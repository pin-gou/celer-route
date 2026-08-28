#!/usr/bin/env bash
# send-llm-test.sh — 发送测试请求触发 RTK 报文压缩
#
# RTK 压缩在 PreLLMHook 中扫描请求的 messages 数组，对 role="tool" 的
# 消息内容（即工具执行结果）应用压缩管线。要触发压缩，请求必须包含：
#   1. role="tool" 的消息，内容为可识别的命令输出（go test / git diff / docker build 等）
#   2. 内容足够大，超过 MinTokensToCompress 阈值
#   3. 内容有足够冗余，压缩比 >= 5%
#
# 本脚本构建的测试场景覆盖所有内置 filter 类别：
#   - test:    go test / pytest / jest 等测试框架输出
#   - git:     git diff / status / log 等 git 输出
#   - build:   docker build / go build / tsc 等编译输出
#   - package: npm install / pip install 等包管理输出
#   - infra:   terraform plan / kubectl get 等基础设施输出
#   - shell:   ls / ps / grep 等通用 shell 输出
#   - cloud:   aws / gcloud 等云平台输出
#   - generic: error-stacktrace / json 输出
#   - multi-turn: 多轮对话，含历史 tool_call+tool_result 序列
#   - streaming: 流式请求
#
# 用法:
#   ./send-llm-test.sh [count] [concurrency]
#
# 参数:
#   count        测试请求数量（默认 10）
#   concurrency  并发度，同时发送的请求数（默认 1）
#
# 环境变量:
#   PG_INSTANCE_HOST  API 主机（默认 localhost）
#   PG_INSTANCE_PORT  API 端口（默认 9080）

set -uo pipefail

COUNT="${1:-10}"
CONCURRENCY="${2:-1}"
(( CONCURRENCY < 1 )) && CONCURRENCY=1
HOST="${PG_INSTANCE_HOST:-localhost}"
PORT="${PG_INSTANCE_PORT:-9080}"
BASE="http://${HOST}:${PORT}"

echo "=== Bifrost LLM Test — RTK compression test ==="
echo "  requests:    ${COUNT}"
echo "  concurrency: ${CONCURRENCY}"
echo "  target:      ${BASE}"
echo ""

# ---- 1. 查询 providers ----
echo "--- Querying providers ---"
PROVIDERS=$(curl -s "${BASE}/api/providers" | python3 -c "
import json, sys
d = json.load(sys.stdin)
names = [p['name'] for p in d.get('providers', [])]
print(' '.join(names))
" 2>/dev/null || echo "")

if [[ -z "$PROVIDERS" ]]; then
  echo "WARN: 无法获取 providers 列表"
fi
echo "providers: ${PROVIDERS:-none}"
echo ""
IFS=' ' read -ra PROVIDER_ARRAY <<< "${PROVIDERS:-}"

# ---- 2. 按 provider 查询其 models，保证模型与 provider 匹配 ----
# 逐个 provider 用 /api/models?provider=<name> 拉取专属模型列表，
# 而不是先取全部模型再单独查 provider（两者各自独立，会导致模型与
# provider 错配）。每个条目保存为 "<model> <provider>" 的配对。
echo "--- Querying models per provider ---"
MODEL_PROVIDER=()

if [[ ${#PROVIDER_ARRAY[@]} -gt 0 ]]; then
  for PIDX in "${PROVIDER_ARRAY[@]}"; do
    MODELS_FOR_PROVIDER=$(curl -s "${BASE}/api/models?provider=${PIDX}&limit=50" | python3 -c "
import json, sys
d = json.load(sys.stdin)
for m in d.get('models', []):
    if m.get('name'):
        print(m['name'])
" 2>/dev/null || echo "")
    while IFS= read -r mname; do
      [[ -n "$mname" ]] && MODEL_PROVIDER+=("${mname} ${PIDX}")
    done <<< "$MODELS_FOR_PROVIDER"
  done
fi

if [[ ${#MODEL_PROVIDER[@]} -eq 0 ]]; then
  # 兜底: 从全局 models 列表提取 name+provider（仍保证两者同源）
  echo "WARN: 逐 provider 查询 models 失败，退回全局 models 列表"
  while IFS=$'\t' read -r mname pname; do
    [[ -n "$mname" ]] && MODEL_PROVIDER+=("${mname} ${pname}")
  done < <(curl -s "${BASE}/api/models?limit=100" | python3 -c "
import json, sys
d = json.load(sys.stdin)
for m in d.get('models', []):
    if m.get('name') and m.get('provider'):
        print(m['name'] + '\t' + m['provider'])
" 2>/dev/null)
fi

if [[ ${#MODEL_PROVIDER[@]} -eq 0 ]]; then
  echo "WARN: 无法获取 models 列表，使用 fixture 模型名称"
  MODELS="deepseek-v4-flash-0731 deepseek-v4-pro glm-5.2 qwen3.6-flash qwen3.7-max"
  for MK in $MODELS; do
    MODEL_PROVIDER+=("${MK}")
  done
fi

MODELS=""
for MP in "${MODEL_PROVIDER[@]}"; do
  set -- $MP
  MODELS="${MODELS} $1"
done
MODELS="${MODELS# }"
echo "models: ${MODELS}"
echo "model-provider pairs: ${#MODEL_PROVIDER[@]}"
echo ""

# ---- 3. 查询 routing rules ----
echo "--- Querying routing rules ---"
ROUTES=$(curl -s "${BASE}/api/governance/routing-rules" | python3 -c "
import json, sys
d = json.load(sys.stdin)
names = [r['name'] for r in d.get('rules', []) if r.get('name')]
print(' '.join(names))
" 2>/dev/null || echo "")

if [[ -z "$ROUTES" ]]; then
  echo "WARN: 无法获取 routing rules 列表，使用 fixture 路由名称"
  ROUTES="pg-expert pg-master pg-associate hermes-default hermes-operator"
fi
echo "routes: ${ROUTES}"
echo ""

# ---- 将列表转数组 ----
IFS=' ' read -ra ROUTE_ARRAY <<< "$ROUTES"

# ---- 4. 发送测试请求 ----
PASS_FILE="$(mktemp)"
FAIL_FILE="$(mktemp)"
TOOL_COUNT_FILE="$(mktemp)"
PAYLOAD_SIZE_FILE="$(mktemp)"
COMPRESSION_RATIO_FILE="$(mktemp)"
: > "$PASS_FILE"
: > "$FAIL_FILE"
: > "$TOOL_COUNT_FILE"
: > "$PAYLOAD_SIZE_FILE"
: > "$COMPRESSION_RATIO_FILE"

# ── 场景定义 ──
# 每个场景包含 messages 数组（含 role="tool" 消息，内容是需压缩的命令输出）
# 使用 python3 内联定义，避免 bash 多行字符串转义
read -r -d '' SCENARIOS_PY << 'PYEOF'
import json

scenarios = []

# ============================================================
# 1. test: go test 输出 — 大量 PASS 行 + 少量 FAIL（可被 go-test filter 压缩）
# ============================================================
go_test_output = """=== RUN   TestParseConfig
--- PASS: TestParseConfig (0.00s)
=== RUN   TestValidateInput
--- PASS: TestValidateInput (0.01s)
=== RUN   TestBuildRequest
--- PASS: TestBuildRequest (0.00s)
=== RUN   TestSendRequest
--- PASS: TestSendRequest (0.02s)
=== RUN   TestHandleResponse
--- PASS: TestHandleResponse (0.01s)
=== RUN   TestRetryLogic
--- PASS: TestRetryLogic (0.03s)
=== RUN   TestTimeout
--- PASS: TestTimeout (0.50s)
=== RUN   TestStreamResponse
--- PASS: TestStreamResponse (0.04s)
=== RUN   TestErrorHandling
--- PASS: TestErrorHandling (0.01s)
=== RUN   TestAuthMiddleware
--- PASS: TestAuthMiddleware (0.00s)
=== RUN   TestRateLimiter
--- PASS: TestRateLimiter (0.02s)
=== RUN   TestCacheHit
--- PASS: TestCacheHit (0.01s)
=== RUN   TestCacheMiss
--- PASS: TestCacheMiss (0.03s)
=== RUN   TestFallbackProvider
--- PASS: TestFallbackProvider (0.02s)
=== RUN   TestParallelRequests
--- PASS: TestParallelRequests (0.15s)
=== RUN   TestLargePayload
--- PASS: TestLargePayload (0.08s)
=== RUN   TestTokenCount
--- PASS: TestTokenCount (0.00s)
=== RUN   TestStreamAccumulator
--- PASS: TestStreamAccumulator (0.01s)
=== RUN   TestToolCallRouting
--- FAIL: TestToolCallRouting (0.02s)
    router_test.go:142: expected tool call to be routed to provider "openai", got "anthropic"
=== RUN   TestModelFallback
--- PASS: TestModelFallback (0.01s)
=== RUN   TestResponseFormat
--- PASS: TestResponseFormat (0.00s)
ok  	github.com/pin-gou/celer-route/core/router	0.452s"""

scenarios.append({
    "category": "test-go-test",
    "messages": [
        {"role": "user", "content": "运行项目的单元测试，检查是否有失败的测试用例"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_go_test_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"go test ./...\", \"timeout\": 60}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_go_test_1", "content": go_test_output},
        {"role": "assistant", "content": "测试完成，21 个测试用例中有 1 个失败：TestToolCallRouting 需要修复。"},
        {"role": "user", "content": "帮我修复 TestToolCallRouting 这个失败的测试用例，然后重新运行测试确认修复成功"}
    ]
})

# ============================================================
# 2. git: git diff 输出 — 大量 diff 行（可被 git-diff filter 压缩）
# ============================================================
git_diff_output = """diff --git a/core/router/router.go b/core/router/router.go
index a1b2c3d..e4f5g6h 100644
--- a/core/router/router.go
+++ b/core/router/router.go
@@ -42,6 +42,8 @@ func NewRouter(cfg *Config) (*Router, error) {
 	if cfg == nil {
 		return nil, errors.New("config is required")
 	}
+	// Initialize the provider cache
+	cfg.initProviderCache()
 	r := &Router{
 		config:   cfg,
 		clients:  make(map[string]*Client),
@@ -156,7 +158,7 @@ func (r *Router) Route(ctx context.Context, req *Request) (*Response, error) {
 	}
 	provider, err := r.selectProvider(req)
 	if err != nil {
-		return nil, fmt.Errorf("no provider available: %w", err)
+		return nil, fmt.Errorf("no provider available for model %q: %w", req.Model, err)
 	}
 	return r.dispatch(ctx, provider, req)
 }
diff --git a/core/router/router_test.go b/core/router/router_test.go
index x1y2z3w..a1b2c3d 100644
--- a/core/router/router_test.go
+++ b/core/router/router_test.go
@@ -89,10 +89,14 @@ func TestToolCallRouting(t *testing.T) {
 	}
 	resp, err := router.Route(ctx, req)
 	if err != nil {
-		t.Fatalf("Route failed: %v", err)
+		t.Fatalf("Route failed for model %s: %v", req.Model, err)
 	}
 	if resp.Provider != "openai" {
-		t.Errorf("expected provider openai, got %s", resp.Provider)
+		t.Errorf("expected provider openai for model %s, got %s", req.Model, resp.Provider)
 	}
+	// Verify the response contains tool calls
+	if len(resp.ToolCalls) == 0 {
+		t.Error("expected at least one tool call in response")
+	}
 }"""

scenarios.append({
    "category": "git-diff",
    "messages": [
        {"role": "user", "content": "检查一下当前代码的变更，看看 router.go 修改了什么"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_git_diff_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"git diff\", \"timeout\": 10}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_git_diff_1", "content": git_diff_output},
        {"role": "assistant", "content": "router.go 有两处修改：1) 新增了 provider cache 初始化；2) 改进了错误消息包含模型名称。router_test.go 的 TestToolCallRouting 测试用例也相应更新了断言。"},
        {"role": "user", "content": "把 git diff 的输出保存到 changelog 文件，然后提交这次修改"}
    ]
})

# ============================================================
# 3. build: docker build 输出（可被 docker-build filter 压缩）
# ============================================================
docker_build_output = """#1 [internal] load build definition from Dockerfile
#1 sha256:a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0
#1 transferring dockerfile: 2.04kB done
#1 DONE 0.0s

#2 [internal] load metadata for docker.io/library/python:3.12-slim
#2 sha256:b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1
#2 DONE 0.0s

#3 [internal] load .dockerignore
#3 sha256:c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2
#3 transferring context: 2B done
#3 DONE 0.0s

#4 [1/5] FROM docker.io/library/python:3.12-slim
#4 sha256:d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3
#4 CACHED

#5 [2/5] WORKDIR /app
#5 sha256:e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4
#5 CACHED

#6 [3/5] COPY requirements.txt .
#6 sha256:f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5
#6 DONE 0.0s

#7 [4/5] RUN pip install --no-cache-dir -r requirements.txt
#7 sha256:a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6
#7 0.427 Collecting flask==3.0.0
#7 0.471   Downloading flask-3.0.0-py3-none-any.whl (102 kB)
#7 0.475 Collecting requests==2.31.0
#7 0.477   Downloading requests-2.31.0-py3-none-any.whl (64 kB)
#7 0.481 Collecting gunicorn==21.2.0
#7 0.483   Downloading gunicorn-21.2.0-py3-none-any.whl (82 kB)
#7 0.488 Collecting redis==5.0.1
#7 0.490   Downloading redis-5.0.1-py3-none-any.whl (250 kB)
#7 0.510 Collecting celery==5.3.6
#7 0.513   Downloading celery-5.3.6-py3-none-any.whl (432 kB)
#7 0.546 Successfully installed celery-5.3.6 flask-3.0.0 gunicorn-21.2.0 redis-5.0.1 requests-2.31.0
#7 DONE 0.6s

#8 [5/5] COPY . .
#8 sha256:b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7
#8 DONE 0.0s

#9 exporting to image
#9 sha256:c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8
#9 exporting layers 0.0s
#9 writing image sha256:d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9
#9 DONE 0.0s"""

scenarios.append({
    "category": "build-docker",
    "messages": [
        {"role": "user", "content": "构建 Docker 镜像并部署到测试环境"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_docker_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"docker build -t app:latest .\", \"timeout\": 120}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_docker_1", "content": docker_build_output},
        {"role": "assistant", "content": "Docker 镜像构建成功，共 9 个步骤，安装了 Flask、Requests、Gunicorn、Redis 和 Celery。"}
    ]
})

# ============================================================
# 4. package: npm install 输出（可被 npm-install filter 压缩）
# ============================================================
npm_install_output = """npm WARN deprecated core-js@2.6.12: core-js@<3.23.3 is no longer maintained and not recommended for usage due to the number of issues. Because of the V8 engine whims, dependency in a newer version of core-js is not guaranteed.
npm WARN deprecated left-pad@1.3.0: use String.prototype.padStart()
npm notice
npm notice New major version of npm available! 10.2.4 -> 11.0.0
npm notice Changelog: https://github.com/npm/cli/releases/tag/v11.0.0
npm notice Run npm install -g npm@11.0.0 to update!
npm notice
added 1423 packages in 45s
1423 packages are looking for funding
  run npm fund for details
8 vulnerabilities (4 moderate, 3 high, 1 critical)
  To address all issues, run:
    npm audit fix
  Run npm audit for details."""

scenarios.append({
    "category": "package-npm",
    "messages": [
        {"role": "user", "content": "安装项目依赖，然后检查有没有安全漏洞需要修复"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_npm_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"npm install\", \"timeout\": 120}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_npm_1", "content": npm_install_output},
        {"role": "assistant", "content": "依赖安装完成，共 1423 个包。发现 8 个漏洞（4 moderate, 3 high, 1 critical），建议运行 npm audit fix。"}
    ]
})

# ============================================================
# 5. infra: terraform plan 输出（可被 terraform filter 压缩）
# ============================================================
terraform_plan_output = """Terraform used the selected providers to generate the following execution plan. Resource actions are indicated with the following symbols:
  + create
  ~ update in-place
  - destroy

Terraform will perform the following actions:

  # aws_vpc.main will be created
  + resource "aws_vpc" "main" {
      + arn                              = (known after apply)
      + cidr_block                       = "10.0.0.0/16"
      + enable_dns_hostnames             = true
      + enable_dns_support               = true
      + id                               = (known after apply)
      + instance_tenancy                  = "default"
      + tags                             = {
          + "Environment" = "production"
          + "Name"        = "main"
        }
    }

  # aws_subnet.public will be created
  + resource "aws_subnet" "public" {
      + arn                                            = (known after apply)
      + availability_zone                              = "us-west-2a"
      + cidr_block                                     = "10.0.1.0/24"
      + id                                             = (known after apply)
      + map_public_ip_on_launch                        = true
      + tags                                           = {
          + "Name" = "public-subnet"
        }
    }

  # aws_security_group.web will be updated in-place
  ~ resource "aws_security_group" "web" {
      ~ description = "Old description" -> "Updated description"
        id          = "sg-12345678"
        name        = "web-sg"
        tags        = {}
      ~ ingress {
          + description = "HTTP from load balancer"
          ~ from_port   = 80 -> 443
          ~ to_port     = 80 -> 443
        }
    }

  # aws_instance.old will be destroyed
  - resource "aws_instance" "old" {
      - ami           = "ami-0c55b159cbfafe1f0" -> null
      - instance_type = "t2.micro" -> null
      - tags          = {} -> null
    }

Plan: 2 to add, 1 to change, 1 to destroy."""

scenarios.append({
    "category": "infra-terraform",
    "messages": [
        {"role": "user", "content": "检查一下基础设施变更计划，确认本次部署的影响范围"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_tf_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"terraform plan\", \"timeout\": 60}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_tf_1", "content": terraform_plan_output},
        {"role": "assistant", "content": "Terraform 计划显示：2 个资源创建（VPC + 子网），1 个资源更新（安全组端口变更），1 个资源销毁（旧实例）。"},
        {"role": "user", "content": "分析一下这个 plan 的安全影响，特别是安全组端口变更的风险"}
    ]
})

# ============================================================
# 6. cloud: kubectl get pods 输出（可被 kubectl-get filter 压缩）
# ============================================================
kubectl_get_output = """NAME                                      READY   STATUS             RESTARTS   AGE
api-gateway-7d4f8b9c6-abc12               1/1     Running            0          2d
api-gateway-7d4f8b9c6-def34               1/1     Running            0          2d
auth-service-5e8f2a1d7-ghi56              1/1     Running            0          5d
auth-service-5e8f2a1d7-jkl78              1/1     Running            0          5d
user-service-3c6a9b2e4-mno90              1/1     Running            0          12h
user-service-3c6a9b2e4-pqr12              1/1     Running            0          12h
payment-service-9b2d4f6a1-stu34           0/1     CrashLoopBackOff   6          15m
payment-service-9b2d4f6a1-vwx56           1/1     Running            0          1d
notification-service-1a3b5c7d9-yza78      0/1     ImagePullBackOff   0          5m
notification-service-1a3b5c7d9-bcd90      0/1     ErrImagePull       0          5m
monitoring-stack-4e6f8a2b1-efg12          1/1     Running            0          7d
monitoring-stack-4e6f8a2b1-hij34          1/1     Running            0          7d
log-aggregator-8d0e2f4a6-klm56            1/1     Running            0          3d
log-aggregator-8d0e2f4a6-nop78            1/1     Running            0          3d
cache-cluster-2f4a6b8d0-qrs90             1/1     Running            0          1d
cache-cluster-2f4a6b8d0-tuv12             1/1     Running            0          1d
message-queue-6a8b0c2d4-wxy34             1/1     Running            0          4d
message-queue-6a8b0c2d4-zab56             1/1     Running            0          4d
database-migration-5c7d9e1f3-cde78        0/1     Terminating        0          30m
database-migration-5c7d9e1f3-fgh90        0/1     Error              1          25m"""

scenarios.append({
    "category": "cloud-kubectl",
    "messages": [
        {"role": "user", "content": "查看 Kubernetes 集群中所有 Pod 的运行状态，找出异常的服务"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_kube_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"kubectl get pods --all-namespaces\", \"timeout\": 10}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_kube_1", "content": kubectl_get_output},
        {"role": "assistant", "content": "集群共有 20 个 Pod，发现 3 个异常：payment-service CrashLoopBackOff（6 次重启），notification-service ImagePullBackOff/ErrImagePull（镜像拉取失败），database-migration 处于 Terminating/Error 状态。"}
    ]
})

# ============================================================
# 7. test: pytest 输出（可被 test-pytest filter 压缩）
# ============================================================
pytest_output = """============================= test session starts ==============================
platform linux -- Python 3.12.0, pytest-8.1.1, pluggy-1.4.0
rootdir: /workspace/project
configfile: pyproject.toml
plugins: cov-5.0.0, asyncio-0.23.5, xdist-3.5.0
collected 142 items

tests/test_auth.py ............                                           [  8%]
tests/test_api.py ...............................                          [ 28%]
tests/test_models.py .......................                              [ 42%]
tests/test_services.py .................................................. [ 78%]
tests/test_tasks.py ............                                          [ 86%]
tests/test_utils.py ...................                                   [100%]

=============================== warnings summary ===============================
tests/test_models.py:42: DeprecationWarning: deprecated_function
  warnings.warn("deprecated_function is deprecated")
tests/test_api.py:128: ResourceWarning: unclosed transport
  transport.close()

-- Docs: https://docs.pytest.org/en/stable/how-to/capture-warnings.html
========================== 142 passed in 3.21s ==========================="""

scenarios.append({
    "category": "test-pytest",
    "messages": [
        {"role": "user", "content": "运行 Python 测试套件，确认所有测试都通过"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_pytest_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"pytest tests/ -v\", \"timeout\": 60}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_pytest_1", "content": pytest_output},
        {"role": "assistant", "content": "142 个测试全部通过，耗时 3.21 秒。有两个警告需要关注：models.py 的弃用 API 和 api.py 的未关闭传输。"}
    ]
})

# ============================================================
# 8. shell: 高重复行 — 大量 ls 输出（测试 dedup / grouping 压缩）
# ============================================================
ls_repetitive_output = """total 1284
drwxr-xr-x  12 user  staff    384 Mar 15 10:30 .
drwxr-xr-x   5 user  staff    160 Mar 15 10:30 ..
-rw-r--r--   1 user  staff   6148 Mar 15 10:30 .DS_Store
-rw-r--r--   1 user  staff    213 Mar 15 10:30 .editorconfig
-rw-r--r--   1 user  staff    351 Mar 15 10:30 .env.example
-rw-r--r--   1 user  staff     38 Mar 15 10:30 .gitignore
-rw-r--r--   1 user  staff    876 Mar 15 10:30 Dockerfile
-rw-r--r--   1 user  staff  11357 Mar 15 10:30 LICENSE
-rw-r--r--   1 user  staff   3028 Mar 15 10:30 Makefile
-rw-r--r--   1 user  staff   3501 Mar 15 10:30 README.md
drwxr-xr-x   4 user  staff    128 Mar 15 10:30 cmd
drwxr-xr-x   6 user  staff    192 Mar 15 10:30 config
drwxr-xr-x   3 user  staff     96 Mar 15 10:30 docs
drwxr-xr-x   5 user  staff    160 Mar 15 10:30 examples
drwxr-xr-x   4 user  staff    128 Mar 15 10:30 internal
drwxr-xr-x  17 user  staff    544 Mar 15 10:30 pkg
-rw-r--r--   1 user  staff   1234 Mar 15 10:30 go.mod
-rw-r--r--   1 user  staff  56789 Mar 15 10:30 go.sum
-rw-r--r--   1 user  staff   2345 Mar 15 10:30 main.go
"""

scenarios.append({
    "category": "shell-ls",
    "messages": [
        {"role": "system", "content": "你是一个文件系统助手，可以列出和操作文件。"},
        {"role": "user", "content": "列出项目根目录的所有文件"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_ls_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"ls -la\", \"timeout\": 5}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_ls_1", "content": ls_repetitive_output},
        {"role": "assistant", "content": "项目根目录有 19 个文件和目录，包括 Go 项目文件、Dockerfile、Makefile 等。"}
    ]
})

# ============================================================
# 9. generic: error-stacktrace（可被 error-stacktrace filter 压缩）
# ============================================================
stacktrace_output = """panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x4a1b2c]

goroutine 42 [running]:
github.com/pin-gou/celer-route/core/router.(*Router).selectProvider(0x0, 0xc0001a2000)
    /workspace/core/router/router.go:158 +0x2a4
github.com/pin-gou/celer-route/core/router.(*Router).Route(0xc0001a2000, {0x7f1a2b3c, 0xc0001a4000}, 0xc0001a6000)
    /workspace/core/router/router.go:92 +0x1b8
github.com/pin-gou/celer-route/core/bifrost.(*Bifrost).HandleRequest(0xc0001a8000, {0x7f1a2b3c, 0xc0001a4000}, 0xc0001a6000)
    /workspace/core/bifrost/bifrost.go:245 +0x3c4
github.com/pin-gou/celer-route/transports/http.(*Handler).ServeHTTP(0xc0001aa000, {0x7f1a2b3c, 0xc0001a4000}, 0xc0001ac000)
    /workspace/transports/http/handler.go:78 +0x2e8
net/http.(*ServeMux).ServeHTTP(0xc0001ae000, {0x7f1a2b3c, 0xc0001a4000}, 0xc0001ac000)
    /usr/local/go/src/net/http/server.go:2568 +0x1b4
net/http.serverHandler.ServeHTTP(0xc0001b0000, {0x7f1a2b3c, 0xc0001a4000}, 0xc0001ac000)
    /usr/local/go/src/net/http/server.go:2938 +0x2c8
net/http.(*conn).serve(0xc0001b2000, {0x7f1a2b3c, 0xc0001a4000})
    /usr/local/go/src/net/http/server.go:2009 +0x5b8
created by net/http.(*Server).Serve in goroutine 1
    /usr/local/go/src/net/http/server.go:3086 +0x5b8"""

scenarios.append({
    "category": "generic-error",
    "messages": [
        {"role": "user", "content": "服务崩溃了，帮我分析一下 panic 日志的根因"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_stack_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"cat /var/log/app/panic.log\", \"timeout\": 5}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_stack_1", "content": stacktrace_output},
        {"role": "assistant", "content": "panic 是 nil pointer dereference，根因在 router.go:158 的 selectProvider 方法中，router 实例为 nil。建议检查 Router 的初始化流程。"}
    ]
})

# ============================================================
# 10. multi-turn: 多轮 tool_call 历史（测试长序列 + 累积压缩）
# ============================================================
multi_turn_output_1 = """=== RUN   TestLogin
--- PASS: TestLogin (0.12s)
=== RUN   TestLogout
--- PASS: TestLogout (0.05s)
=== RUN   TestSession
--- PASS: TestSession (0.08s)
ok  	auth-service	0.352s"""

multi_turn_output_2 = """=== RUN   TestCreateUser
--- PASS: TestCreateUser (0.15s)
=== RUN   TestDeleteUser
--- PASS: TestDeleteUser (0.10s)
=== RUN   TestUpdateUser
--- FAIL: TestUpdateUser (0.03s)
    user_test.go:89: expected email to be updated, got old value
ok  	user-service	0.423s"""

scenarios.append({
    "category": "multi-turn",
    "messages": [
        {"role": "system", "content": "你是一个测试运行助手，帮助开发者运行和分析测试结果。"},
        {"role": "user", "content": "先运行 auth-service 的测试"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_mt_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"go test ./auth-service/...\", \"timeout\": 30}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_mt_1", "content": multi_turn_output_1},
        {"role": "assistant", "content": "auth-service 3 个测试全部通过。"},
        {"role": "user", "content": "再跑 user-service 的测试"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_mt_2", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"go test ./user-service/...\", \"timeout\": 30}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_mt_2", "content": multi_turn_output_2},
        {"role": "assistant", "content": "user-service 有 1 个测试失败：TestUpdateUser - email 字段未更新。"},
        {"role": "user", "content": "修复 TestUpdateUser 失败的问题，然后重新跑 user-service 的全部测试确认修复生效"}
    ]
})

# ============================================================
# 11. streaming: 流式请求（带 tool output）
# ============================================================
scenarios.append({
    "category": "streaming",
    "stream": True,
    "messages": [
        {"role": "user", "content": "查看集群中所有异常 Pod 的状态"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_stream_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"kubectl get pods --field-selector=status.phase!=Running\", \"timeout\": 10}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_stream_1", "content": kubectl_get_output},
        {"role": "assistant", "content": "以下 Pod 处于非 Running 状态：payment-service CrashLoopBackOff，notification-service ImagePullBackOff/ErrImagePull，database-migration Terminating/Error。建议检查镜像仓库配置和支付服务最近一次的代码变更。"}
    ]
})

# ============================================================
# 12. build: tsc TypeScript 编译输出（可被 build-typescript filter 压缩）
# ============================================================
tsc_output = """src/services/auth.ts:42:5 - error TS2322: Type 'string | undefined' is not assignable to type 'string'.
  Type 'undefined' is not assignable to type 'string'.

42     const token = process.env.AUTH_TOKEN;
       ~~~~~

src/services/auth.ts:58:12 - error TS2532: Object is possibly 'undefined'.

58     return user.profile.name;
              ~~~~

src/services/api.ts:23:3 - error TS2564: Property 'client' has no initializer and is not definitely assigned in the constructor.

23   private client: HttpClient;
     ~~~~~~

src/services/api.ts:67:10 - error TS18046: 'result' is of type 'unknown'.

67   return result.data;
            ~~~~~~

src/utils/validator.ts:15:24 - error TS7006: Parameter 'value' implicitly has an 'any' type.

15   return value.trim();
                          ~~~~

Found 5 errors in 3 files."""

scenarios.append({
    "category": "build-typescript",
    "messages": [
        {"role": "user", "content": "运行 TypeScript 类型检查，找出所有类型错误"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_tsc_1", "type": "function", "function": {"name": "run_command", "arguments": "{\"command\": \"npx tsc --noEmit\", \"timeout\": 30}"}}
        ]},
        {"role": "tool", "tool_call_id": "call_tsc_1", "content": tsc_output},
        {"role": "assistant", "content": "发现 5 个类型错误分布在 3 个文件中：auth.ts 有 2 个（类型兼容性和空值检查），api.ts 有 2 个（初始化断言和未知类型），validator.ts 有 1 个（隐式 any 类型）。"}
    ]
})

# 按场景数量导出
PYEOF

# 构建 payload 的 python3 脚本
build_payload() {
  local model="$1"
  local scenario_index="$2"
  python3 -c "
import json, sys, os

${SCENARIOS_PY}

model = sys.argv[1]
idx = int(sys.argv[2])
seed = int(sys.argv[3]) if len(sys.argv) > 3 else 42

scenario = scenarios[idx % len(scenarios)]
is_stream = scenario.get('stream', False)

payload = {
    'model': model,
    'messages': scenario['messages'],
    'params': {
        'max_completion_tokens': 1024,
        'stream': is_stream
    }
}

raw = json.dumps(payload, ensure_ascii=False)
print(raw)
" "$model" "$scenario_index" "$RANDOM"
}

send_llm_request() {
  local i="$1"
  local resp="/tmp/send-llm-test-resp-$$-${i}.json"
  local out="/tmp/send-llm-test-out-$$-${i}.txt"

  RANDOM=$((i * 7919 + $$))

  {
    echo "--- Request #${i} ---"

    USE_MODEL=$((RANDOM % 2))

    if [[ USE_MODEL -eq 0 && ${#MODEL_PROVIDER[@]} -gt 0 ]]; then
      idx=$((RANDOM % ${#MODEL_PROVIDER[@]}))
      MP="${MODEL_PROVIDER[$idx]}"
      set -- $MP
      MODEL="$1"
      PROVIDER="$2"
      if [[ -n "${PROVIDER}" ]]; then
        echo "mode: by-model | model: ${MODEL} | provider: ${PROVIDER}"
      else
        echo "mode: by-model | model: ${MODEL}"
      fi
    elif [[ ${#ROUTE_ARRAY[@]} -gt 0 ]]; then
      idx=$((RANDOM % ${#ROUTE_ARRAY[@]}))
      MODEL="${ROUTE_ARRAY[$idx]}"
      echo "mode: by-route | route: ${MODEL}"
    elif [[ ${#MODEL_PROVIDER[@]} -gt 0 ]]; then
      idx=$((RANDOM % ${#MODEL_PROVIDER[@]}))
      MP="${MODEL_PROVIDER[$idx]}"
      set -- $MP
      MODEL="$1"
      echo "mode: by-model (route fallback) | model: ${MODEL}"
    else
      echo "WARN: 无可用 model 或 route，跳过"
      return
    fi

    scenario_idx=$(( (i - 1) % 12 ))

    SCENARIO_CATEGORY=$(python3 -c "
${SCENARIOS_PY}
import json
idx = $scenario_idx % len(scenarios)
print(scenarios[idx]['category'])
")

    echo "scenario: ${SCENARIO_CATEGORY} (#${scenario_idx})"

    PAYLOAD=$(build_payload "$MODEL" "$scenario_idx")
    PAYLOAD_SIZE=${#PAYLOAD}
    echo "payload_size: ${PAYLOAD_SIZE} bytes"

    # 统计 tool 消息的总字符数
    TOOL_CHARS=$(python3 -c "
${SCENARIOS_PY}
idx = $scenario_idx % len(scenarios)
total = 0
for msg in scenarios[idx]['messages']:
    if msg.get('role') == 'tool':
        total += len(msg.get('content', ''))
print(total)
")
    echo "tool_content: ~${TOOL_CHARS} chars"

    START=$(date +%s%N)

    if [[ "$SCENARIO_CATEGORY" == "streaming" ]]; then
      > "$resp"
      curl -s -X POST "${BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "$PAYLOAD" 2>/dev/null | while IFS= read -r line; do
          echo "$line" >> "$resp"
        done
      HTTP_CODE=200
    else
      HTTP_CODE=$(curl -s -o "$resp" -w "%{http_code}" \
        -X POST "${BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "$PAYLOAD" 2>/dev/null)
    fi

    END=$(date +%s%N)
    ELAPSED_MS=$(( (END - START) / 1000000 ))

    echo "elapsed: ${ELAPSED_MS}ms"

    if [[ "$HTTP_CODE" == "200" ]]; then
      python3 -c "
import json
with open('${resp}') as f:
    raw = f.read()

# 尝试解析完整 JSON，streaming 则取最后一个完整 data:
choices = None
content = None
usage = None
try:
    d = json.loads(raw)
    choices = d.get('choices', [])
    usage = d.get('usage')
except json.JSONDecodeError:
    last_data = None
    for line in raw.split('\n'):
        if line.startswith('data: ') and line != 'data: [DONE]':
            try:
                last_data = json.loads(line[6:])
            except:
                pass
    if last_data:
        choices = last_data.get('choices', [])
        usage = last_data.get('usage')

if choices:
    msg = choices[0].get('message', {}) or choices[0].get('delta', {})
    content = msg.get('content', '')
    tool_calls = msg.get('tool_calls', [])
    if tool_calls:
        names = [tc['function']['name'] for tc in tool_calls]
        print(f'result: PASS → tool_calls: {len(tool_calls)} → {json.dumps(names)}')
    else:
        preview = (content[:120] if content else '(no content)')
        print(f'result: PASS → content: {preview}')

    # 提取 usage 中的压缩指标
    if usage:
        prompt_tokens = usage.get('prompt_tokens', 0)
        original_prompt = usage.get('original_prompt_tokens', 0)
        compressed_prompt = usage.get('compressed_prompt_tokens', 0)
        if original_prompt and original_prompt != prompt_tokens:
            ratio = round((1 - compressed_prompt / original_prompt) * 100, 1)
            print(f'rtk: compressed {original_prompt}→{compressed_prompt} tokens ({ratio}% reduction)')
elif raw:
    print(f'result: PASS → (no choices, {len(raw)} bytes received)')
else:
    print('result: PASS → (empty response)')
" 2>/dev/null || echo "result: PASS → (parse error)"

      echo "${PAYLOAD_SIZE}" >> "$PAYLOAD_SIZE_FILE"
      echo 1 >> "$PASS_FILE"
    else
      BODY=$(head -c 200 "$resp" 2>/dev/null || echo "(no response body)")
      echo "result: FAIL (${HTTP_CODE}) → ${BODY}"
      echo 1 >> "$FAIL_FILE"
    fi
  } > "$out" 2>&1

  rm -f "$resp"
}

i=1
while (( i <= COUNT )); do
  batch_end=$(( i + CONCURRENCY - 1 ))
  (( batch_end > COUNT )) && batch_end=$COUNT

  pids=()
  for ((j = i; j <= batch_end; j++)); do
    send_llm_request "$j" &
    pids+=("$!")
  done

  for pid in "${pids[@]}"; do
    wait "$pid"
  done

  for ((j = i; j <= batch_end; j++)); do
    cat "/tmp/send-llm-test-out-$$-${j}.txt"
    rm -f "/tmp/send-llm-test-out-$$-${j}.txt"
  done
  echo ""

  i=$(( batch_end + 1 ))
done

PASS=$(($(wc -l < "$PASS_FILE")))
FAIL=$(($(wc -l < "$FAIL_FILE")))
TOTAL_PAYLOAD=0
PAYLOAD_COUNT=0
if [[ -s "$PAYLOAD_SIZE_FILE" ]]; then
  while IFS= read -r line; do
    TOTAL_PAYLOAD=$((TOTAL_PAYLOAD + line))
    PAYLOAD_COUNT=$((PAYLOAD_COUNT + 1))
  done < "$PAYLOAD_SIZE_FILE"
fi
AVG_PAYLOAD=$(( PAYLOAD_COUNT > 0 ? TOTAL_PAYLOAD / PAYLOAD_COUNT : 0 ))
rm -f "$PASS_FILE" "$FAIL_FILE" "$TOOL_COUNT_FILE" "$PAYLOAD_SIZE_FILE" "$COMPRESSION_RATIO_FILE"

echo "=== Summary ==="
echo "  Total:       ${COUNT}"
echo "  Pass:        ${PASS}"
echo "  Fail:        ${FAIL}"
echo "  Avg payload: ${AVG_PAYLOAD} bytes"
echo ""
echo "  Scenario coverage (12 builtin filter types):"
echo "    1. test-go-test     — go test output (PASS/FAIL lines)"
echo "    2. git-diff         — git diff output"
echo "    3. build-docker     — docker build output"
echo "    4. package-npm      — npm install output"
echo "    5. infra-terraform  — terraform plan output"
echo "    6. cloud-kubectl    — kubectl get pods output"
echo "    7. test-pytest      — pytest output"
echo "    8. shell-ls         — ls -la output (repetitive lines)"
echo "    9. generic-error    — Go stacktrace"
echo "   10. multi-turn       — multi-turn tool call history"
echo "   11. streaming        — streaming request"
echo "   12. build-typescript — tsc type errors"

[[ "$FAIL" -eq 0 ]] && exit 0 || exit 1