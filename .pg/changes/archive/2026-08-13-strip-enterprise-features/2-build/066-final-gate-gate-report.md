# Final Gate Assessment: final-gate

**Track**: final-gate
**Change**: strip-enterprise-features
**Date**: 2026-08-13
**Cycle**: 1

## 整体判定

**FAIL** — gate-score: 81.5 (≥ 80) 但 P0 触发：V-helm-1 FAIL（define-summary 声明 verifiable 但从未验证，enterprise 字段未从 helm-charts 删除）

## 收集的 Gate Assessment

### dev stage (全部 PASS)
| Track | Report | gate-score | 判定 |
|-------|--------|:----------:|:----:|
| dev.core | 005-dev.core-gate-verify.md | 94.25 | ✅ PASS |
| dev.framework | 010-dev.framework-gate-verify.md | 94.25 | ✅ PASS |
| dev.transports | 019-dev.transports-gate-verify.md | 92.50 | ✅ PASS |
| dev.plugins | 024-dev.plugins-gate-verify.md | 95.00 | ✅ PASS |
| dev.ui | 033-dev.ui-gate-verify.md | 95.00 | ✅ PASS |

### int stage (全部 PASS)
| Track | Report | gate-score | 判定 |
|-------|--------|:----------:|:----:|
| int.core | 038-int.core-gate-verify.md | 100.00 | ✅ PASS |
| int.framework | 043-int.framework-gate.md | 100.00 | ✅ PASS |
| int.transports | 048-int.transports-gate.md | 95.00 | ✅ PASS |
| int.plugins | 053-int.plugins-gate.md | 95.00 | ✅ PASS |
| int.ui | 058-int.ui-gate-verify.md | 95.00 | ✅ PASS |

### int.scenario (065)
| Report | 结果 | 判定 |
|--------|:----:|:----:|
| 065-int.scenario-scenario-execute.md | 7/7 Scenario PASS (3 critical + 4 non-critical) | ✅ complete |

## 跨 stage 依赖项检查（任务 52.2）

### ✅ 跨 track 依赖满足
- core 模块删除 enterprise-only BifrostContextKey 后，framework/transports/plugins 编译依赖传播已清理
- framework 删除 EnterpriseOnly/ErrFlagEnterpriseOnly/SyncDelegate 后，transports 引用已清理
- transports 删除 enterprisePlugins/LargePayloadHook 后，上游依赖无断裂
- UI 路由删除后 TanStack Router 自动重生成 routeTree.gen.ts，build 通过
- 无跨 stage 运行依赖冲突（dev 和 int 各自独立通过）

### ❌ 跨 track 交付缺口（3 项 proposal 声明但未分配任务/未实现）

以下 3 项在 proposal.md「包含」章节 + design.md 中明确声明，但未出现在 tasks.md 任何 track 的任务列表中，也从未被实现：

| # | 缺口 | proposal.md 引用 | design.md 引用 | 原因 |
|---|------|-----------------|---------------|------|
| 1 | **helm-charts 部署资产 enterprise 块未删除** | 第 42 行「删除 helm-charts/bifrost/values.yaml、values.schema.json、_helpers.tpl 中的 enterprise 块」 | 部署资产段全文 + design.md 对账 V-helm-1 (verifiable) | 无 track 分配 helm 任务，tasks.md 缺失 |
| 2 | **Makefile cleanup-enterprise 未删除** | 第 44 行「删除 Makefile 的 cleanup-enterprise 目标与 install-ui 对其依赖」 | 不包含（proposal 仅） | 无 track 分配 Makefile 任务 |
| 3 | **tests/e2e/features/mcp-tool-groups 未删除/未清理** | 第 43 行「删除 tests/e2e/features/mcp-tool-groups/spec.ts 中 enterprise 提及」 | E2E 段声明删除 | 无 track 分配 e2e 任务 |

## 加权评分维度

| 维度 | 权重 | 维度分 | 加权分 | 备注 |
|------|:----:|:------:|:------:|------|
| V-* 验证项通过率 | 50% | 85 | 42.50 | V-helm-1 FAIL（verifiable 但从未验证）；V-scenario-2 未执行；其余 12/14 全部 PASS |
| design.md 一致性 | 20% | 70 | 14.00 | 部署资产段 helm 承诺未实现；E2E 段承诺未实现；core/framework/transports/plugins/ui 段全部实现 |
| scope creep 检查 | 15% | 100 | 15.00 | 无越界修改；反向缺口（该做的没做）已计入设计一致性 |
| 测试质量 | 0% | — | — | V3.x 合并入实现完整性 |
| 实现完整性 (v3.x) | 10% | 100 | 10.00 | 硬标记 0 命中；各 track 内 tasks-V 映射完整 |
| **gate-score** | **100%** | | **81.50** | |
| runtime_boot (B3) | — | PASS | — | 4 步证据齐：启动/就绪/真实 e2e/失败处置（多个 int verify 报告 + scenario 065 含真实服务探针） |

**PASS 条件**: gate-score ≥ 80 AND 无 P0 失败
**P0 触发**: V-helm-1 FAIL（define-summary 声明 verifiable 但企业字段未从 helm-charts 删除，从未验证通过）
**整体判定**: **FAIL**

## 不通过项详细说明

### G-1: helm-charts enterprise 块未删除
- **检查项**: #2 (design.md 一致性) + #1 (V-* 通过率 V-helm-1)
- **预期**: design.md 部署资产段 + design.md 对账 V-helm-1 (verifiable) + 065 dispatch 明确要求 V-scenario-2（helm template 渲染通过）
  - values.yaml 删除 scim_config/access_profiles/vault_store/is_enterprise/alerting/audit_logs/load_balancer_config/large_payload_optimization/guardrails_config/cluster_config/circuit_breaker_config
  - values.schema.json 同步删除对应 schema 字段
  - _helpers.tpl 删除 Access Profiles / Vault Store / is_enterprise 注入段
  - README.md 删除 enterprise 段落与私有仓库示例
  - V-helm-1: helm template 渲染通过, enterprise 字段已删除
- **实际**: git diff master...HEAD -- helm-charts/ = 0 行（helm-charts 完全未变更）
  - values.yaml:584 仍有 `is_enterprise: false`
  - values.yaml:1173 仍有 `access_profiles` 注释块
  - values.yaml:1217-1218 仍有 `alerting` 注释块
  - values.schema.json: 仍有 5 个 enterprise 字段匹配
  - _helpers.tpl:1296-1297 仍有 `is_enterprise` 注入
  - README.md: 仍有 8 处 enterprise 提及
  - V-helm-1: 从未执行验证（065 scenario-execute 只跑了 7 个 API scenario，未执行 helm template）
  - V-scenario-2: 065 dispatch 明确列出但未执行
- **文件位置**: helm-charts/bifrost/values.yaml helm-charts/bifrost/values.schema.json helm-charts/bifrost/templates/_helpers.tpl helm-charts/bifrost/README.md
- **关联 task**: int.scenario:scenario-execute 任务 51.4, 51.5（design.md V-scenario-2 / V-helm-1 应走 scenario 路径但未执行）；实际根因是 tasks.md 无 helm 任务分配
- **修复建议**: 删除 helm-charts/bifrost/values.yaml/values.schema.json/_helpers.tpl/README.md 中 enterprise 段；执行 `helm template helm-charts/bifrost/` 确认渲染通过

### G-2: Makefile cleanup-enterprise 未删除
- **检查项**: #2 (design.md 一致性)
- **预期**: proposal.md「包含」第 44 行 + 方案概述 Makefile 段 + define-summary in_scope 第 9 条
  - 删除 .PHONY cleanup-enterprise
  - 删除 cleanup-enterprise 目标
  - install-ui 移除对 cleanup-enterprise 的依赖
- **实际**: git diff master...HEAD -- Makefile = 0 行（Makefile 完全未变更）
  - Makefile:71 `.PHONY` 列表仍含 `cleanup-enterprise`
  - Makefile:103-106 `cleanup-enterprise` 目标仍存在
  - Makefile:108 `install-ui: cleanup-enterprise` 依赖仍存在
- **文件位置**: Makefile:71,103,108
- **关联 task**: dev.ui:dev 任务 22.17（UI 构建链通过 install-ui 执行 cleanup-enterprise）
- **修复建议**: 删除 Makefile 中 cleanup-enterprise 目标（.PHONY + 目标体），install-ui 依赖改为 `install-ui:`

### G-3: tests/e2e/features/mcp-tool-groups 未删除/未清理
- **检查项**: #2 (design.md 一致性)
- **预期**: proposal.md 方案概述 E2E 段 + design.md 影响面 E2E 段
  - 删除 tests/e2e/features/mcp-tool-groups/ 整个目录（OSS 中无对应组件）
  - 删除 mcp-tool-groups.spec.ts 中 enterprise 注释提及
- **实际**: 目录仍存在，spec.ts 仍有 `// @enterprise` 注释引用
  - tests/e2e/features/mcp-tool-groups/mcp-tool-groups.spec.ts:3-4 含 `@enterprise` 注释
  - tests/e2e/features/mcp-tool-groups/pages/mcp-tool-groups.page.ts 存在
- **文件位置**: tests/e2e/features/mcp-tool-groups/mcp-tool-groups.spec.ts tests/e2e/features/mcp-tool-groups/pages/mcp-tool-groups.page.ts
- **关联 task**: dev.ui:dev 任务 22.4（删除企业版路由的连带产物）
- **修复建议**: `rm -rf tests/e2e/features/mcp-tool-groups/`

## 各维度详细说明

### 维度 1: V-* 验证项通过率 (50% × 85 = 42.50)

| V-* | 定义来源 | 状态 | 判定 |
|-----|---------|:----:|:----:|
| V-ui-1 | define-summary verifiable | grep 0 @enterprise 命中 | ✅ PASS |
| V-ui-2 | define-summary verifiable | npm run build exit=0 | ✅ PASS |
| V-ui-3 | define-summary verifiable | npm run lint + typecheck exit=0 | ✅ PASS |
| V-ui-4 | define-summary degraded | 12 个 enterprise 目录不存在 | ✅ PASS |
| V-ui-5 | define-summary degraded | sidebar IS_ENTERPRISE 0 命中 | ✅ PASS |
| V-core-1 | define-summary verifiable | go build exit=0 | ✅ PASS |
| V-core-2 | define-summary verifiable | go vet exit=0 | ✅ PASS |
| V-core-3 | define-summary verifiable | go test 通过（预存 flaky 豁免） | ✅ PASS |
| V-transports-1 | define-summary verifiable | config schema 删除确认（pre-existing oauth drift 豁免） | ✅ PASS |
| V-transports-2 | define-summary degraded | 14 enterprise 端点 404、OSS 端点 200 | ✅ PASS |
| V-plugins-1 | define-summary verifiable | governance build exit=0；grep 0 代码引用已删键 | ✅ PASS |
| **V-helm-1** | **define-summary verifiable** | **helm template 从未执行；enterprise 字段未从 helm-charts 删除** | **❌ FAIL** |
| V-core-4 | define-summary skipped | 豁免（付费 API key） | ✅ SKIP |
| V-scenario-1 | design.md int scenario VC | 各模块 build 分解验证通过 | ✅ PASS |
| V-scenario-2 | design.md int scenario VC | 065 dispatch 明确要求但未执行 | ❌ FAIL |

**P0 触发**: V-helm-1 FAIL → P0

### 维度 2: design.md 一致性 (20% × 70 = 14.00)

| design.md 章节 | 承诺 | 实现状态 | 判定 |
|---------------|------|---------|:----:|
| 后端架构 - core | 删除 enterprise-only BifrostContextKey, vault hooks, SSEReaderFactory | ✅ core/ 代码已删除 | ✅ |
| 后端架构 - framework | 删除 EnterpriseOnly/ErrFlagEnterpriseOnly/SyncDelegate | ✅ framework/ 代码已删除 | ✅ |
| 后端架构 - transports | 删除 enterprisePlugins, LargePayloadHook, IsEnterprise, schema 字段 | ✅ transports/ 代码已删除 | ✅ |
| 后端架构 - plugins | 注释清理, IsEnterprise 引用删除 | ✅ plugins/ 代码已清理 | ✅ |
| 前端架构 | 删除 fallback 目录, 路由, sidebar, import 重写, scope 收窄 | ✅ ui/ 代码已完成 | ✅ |
| **部署资产** | **values.yaml/values.schema.json/_helpers.tpl/README 删除 enterprise 段** | **❌ 0 diff（未变更）** | **❌ FAIL** |
| E2E 段 | 删除 mcp-tool-groups spec, utils_test 表名, plugins_test/ governanceroutes_test 期望 | utils_test/plugins_test/governanceroutes_test ✅; mcp-tool-groups ❌ 未删除 | ❌ FAIL |
| Verification Criteria V-scenario-2 | helm template 渲染通过 | 065 未执行 | ❌ FAIL |

**P0 失败条件**: 缺核心 API/DTO 改动 → 不触发（helm 非 API/DTO）

### 维度 3: scope creep 检查 (15% × 100 = 15.00)

- 所有变更文件在各 track 模块根内（core/framework/transports/plugins/ui）
- 无越界修改，无新增功能，无未声明文件改动
- 反向缺口（该做的没做）不属于 scope creep 维度，已计入设计一致性维度

### 维度 4: 实现完整性 (v3.x, 10% × 100 = 10.00)

**硬标记扫描**（git diff master...HEAD 排除 .pg/ 和 .opencode/）：
```
$ git diff master...HEAD -- ':!.pg' ':!.opencode' | grep -E '^\+' | grep -nE '//\s*(TODO|FIXME|XXX|stub|not implemented)|@todo|UnsupportedOperationException' | grep -v '\[skip-review\]'
(0 命中)
```
✅ 生产代码 0 个新增 TODO/FIXME/XXX/stub/not-implemented

**tasks-V 映射核查**（各 track 内部）：
- dev.core: 5/5 任务映射完整 ✅
- dev.framework: 5/5 任务映射完整 ✅
- dev.transports: 10/10 任务映射完整 ✅
- dev.plugins: 4/4 任务映射完整 ✅
- dev.ui: 18/18 任务映射完整 ✅
- int.core/framework/transports/plugins/ui: 各 track 全部映射完整 ✅

**tasks-V 缺口**: helm/Makefile/e2e 无 track 分配任务，属于 task 分配缺口（非 stub/占位实现）

### runtime_boot (B3 修复，P0 硬约束)

| 步骤 | 证据 | 判定 |
|------|------|:----:|
| 1. 启动服务 | int verify 报告：restart_all_instances 原始输出（bifrost-api ready on 9080, ui-dev ready on 3008） | ✅ |
| 2. 等待就绪 | int verify 报告：health probe 30×3s 轮询 → OK | ✅ |
| 3. 真实 e2e | scenario 065: 7 个真实 API 调用（version/config/rbac/teams/metrics/virtual-keys/routing-rules）；int verify: 14 enterprise 端点 404 | ✅ |
| 4. 失败处置 | int verify 报告：无启动失败/探针超时 | ✅ |

**runtime_boot PASS** — 4 步证据齐全，服务实际启动并响应真实 HTTP 请求，无"代码存在但装配失败"风险。

## 实现完整性核查段

### grep 输出

```bash
# 生产代码（非 .pg/ 非 .opencode/ 非 test）硬标记扫描
$ git diff master...HEAD -- ':!.pg' ':!.opencode' | grep -E '^\+' | grep -nE '//\s*(TODO|FIXME|XXX|stub|not implemented)|@todo|UnsupportedOperationException' | grep -v '\[skip-review\]'
(0 命中 — 无输出)
```

### 豁免列表

无。全 bugdiff 中 31 行命中均在 `.pg/` 变更管理文档文本中（grep 命令本身作为文档内容），非生产代码，无需豁免。

### tasks-V 映射核查结论

各 track 内部 tasks-V 映射完整（共 41 个 task 条目全部对应代码变更）。但 proposal 声明 + design.md 承诺的 3 项交付物（helm-charts/Makefile/e2e mcp-tool-groups）未出现在 tasks.md 任何任务中，属任务分配缺口，not stub/占位实现。

## 维度分明细

| 维度 | 权重 | 维度分 | 加权分 | P0 触发 |
|------|:----:|:------:|:------:|:-------:|
| V-* 验证项通过率 | 50% | 85 | 42.50 | ✅ V-helm-1 FAIL |
| design.md 一致性 | 20% | 70 | 14.00 | ❌ |
| scope creep 检查 | 15% | 100 | 15.00 | ❌ |
| 测试质量 | 0% | — | — | — |
| 实现完整性 (v3.x) | 10% | 100 | 10.00 | ❌ |
| **gate-score** | **100%** | | **81.50** | **P0 触发 → FAIL** |

**PASS 条件**: gate-score ≥ 80 AND 无 P0 失败
**P0 条件**: V-helm-1 FAIL → **整体 FAIL**
