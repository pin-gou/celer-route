# Final Gate Assessment: final-gate

**Track**: final-gate
**Change**: strip-enterprise-features
**Date**: 2026-08-13
**Cycle**: 2 (retry of 066)

## 整体判定

**FAIL** — gate-score: 84.1 (≥ 80) 但 P0 触发：V-helm-1 FAIL（vaultStore 残留 + 从未执行 helm template 验证）
实现完整性维度：硬标记 0 命中，但 vaultStore 遗漏属设计承诺实现不完整

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

### ✅ 066 修复确认（修复提交 0b72c9d）
066 报告的 3 个缺口修复情况：

| # | 缺口 | 修复状态 | 说明 |
|---|------|:--------:|------|
| 1 | **helm-charts 部署资产 enterprise 块未删除** | ⚠️ 部分修复 | scim/cluster/alerting/auditLogs/loadbalancer/guardrails/is_enterprise/circuit_breaker 已删除；**vaultStore 的 schema 字段 + _helpers.tpl 注入段未删除** |
| 2 | **Makefile cleanup-enterprise 未删除** | ✅ 已修复 | .PHONY 已清理，目标已删除，install-ui 依赖已移除 |
| 3 | **tests/e2e/features/mcp-tool-groups 未删除** | ✅ 已修复 | 目录与 spec 已完全删除，ls 确认不存在 |

## 加权评分维度

| 维度 | 权重 | 维度分 | 加权分 | 备注 |
|------|:----:|:------:|:------:|------|
| V-* 验证项通过率 | 50% | 85 | 42.50 | V-helm-1 FAIL（vaultStore 残留 + 从未 helm template 验证）；V-scenario-2 未执行；其余 12/14 全部 PASS |
| design.md 一致性 | 20% | 88 | 17.60 | 部署资产段 vaultStore 未删除（values.schema.json schema + _helpers.tpl 注入段）；其余部分（scim/cluster/alerting/auditLogs/guardrails/largePayload/circuitBreaker/is_enterprise）已修复；E2E/Makefile 已补齐 |
| scope creep 检查 | 15% | 100 | 15.00 | 修复提交 0b72c9d 只改动 helm-charts/Makefile/tests/e2e，均在 proposal 声明范围内 |
| 测试质量 | 0% | — | — | V3.x 合并入实现完整性 |
| 实现完整性 (v3.x) | 10% | 90 | 9.00 | 硬标记 0 命中；但 vaultStore 遗漏——design.md 承诺了删除但未实现，属"设计承诺未完整实现" |
| **gate-score** | **100%** | | **84.10** | |
| runtime_boot (B3) | — | PASS | — | 4 步证据齐：065 scenario 含真实服务启动（9080/health 200）+ 7 个真实 API 调用 + 探针轮询 OK |

**PASS 条件**: gate-score ≥ 80 AND 无 P0 失败
**P0 触发**: V-helm-1 FAIL（verifiable 但 enterprise 字段 vaultStore 未从 helm-charts 完全删除，且 helm template 从未验证通过）
**整体判定**: **FAIL**

## 不通过项详细说明

### final-gate:G-1 — vaultStore 未从 helm-charts 完全删除
- **检查项**: #1 (V-* 通过率 V-helm-1) + #2 (design.md 一致性)
- **预期**: design.md 部署资产段 + proposal.md:101-102 明确要求：
  - `values.schema.json` 同步删除 vaultStore schema 字段
  - `templates/_helpers.tpl` 删除 Vault Store 注入段（"Access Profiles, Vault Store, is_enterprise 注入段"）
  - V-helm-1: helm template 渲染通过, enterprise 字段已删除
- **实际**: 修复提交 0b72c9d 修复了大部分 enterprise 字段（scim/cluster/alerting/auditLogs/loadbalancer/guardrails/is_enterprise/circuit_breaker），但 **vaultStore 完全未处理**：
  - `values.schema.json:2402-2460` 仍保留完整 vaultStore schema 对象（60 行，含 aws/gcp/hashicorp 子配置）
  - `_helpers.tpl:661-698` 仍保留完整 Vault Store 注入段（9 处引用，第 698 行 `set $cs "vault_store" $vaultStore` 会渲染 vault_store 到 config.json）
  - `values.yaml:977-1000` 仍保留 vaultStore 注释示例块（含 enterprise 文档）
  - helm template 渲染从未执行验证（无任何证据文件在 2-build/ 目录）
- **文件位置**:
  - `helm-charts/bifrost/values.schema.json:2402-2460`
  - `helm-charts/bifrost/templates/_helpers.tpl:661-698`
  - `helm-charts/bifrost/values.yaml:977-1000`
- **关联 task**: int.scenario:scenario-execute 任务 51.4, 51.5；实际根因是 tasks.md 无 helm 任务分配（fix-gate 也未补 task）
- **修复建议**: 删除 values.schema.json 中 vaultStore 对象（~60 行 schema）；删除 _helpers.tpl 中 Vault Store 注入段（661-698 行）；删除 values.yaml 中 vaultStore 注释块（977-1000 行）；执行 `helm template helm-charts/bifrost/` 确认渲染通过且无 vault_store 键

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
| **V-helm-1** | **define-summary verifiable** | **helm template 从未执行；vaultStore schema+注入段残留** | **❌ FAIL** |
| V-core-4 | define-summary skipped | 豁免（付费 API key） | ✅ SKIP |
| V-scenario-1 | design.md int scenario VC | 各模块 build 分解验证通过 | ✅ PASS |
| V-scenario-2 | design.md int scenario VC | 065 dispatch 明确要求但未执行 | ❌ FAIL |

**P0 触发**: V-helm-1 FAIL → P0

### 维度 2: design.md 一致性 (20% × 88 = 17.60)

| design.md 章节 | 承诺 | 实现状态 | 判定 |
|---------------|------|---------|:----:|
| 后端架构 - core | 删除 enterprise-only BifrostContextKey, vault hooks, SSEReaderFactory | ✅ core/ 代码已删除 | ✅ |
| 后端架构 - framework | 删除 EnterpriseOnly/ErrFlagEnterpriseOnly/SyncDelegate | ✅ framework/ 代码已删除 | ✅ |
| 后端架构 - transports | 删除 enterprisePlugins, LargePayloadHook, IsEnterprise, schema 字段 | ✅ transports/ 代码已删除 | ✅ |
| 后端架构 - plugins | 注释清理, IsEnterprise 引用删除 | ✅ plugins/ 代码已清理 | ✅ |
| 前端架构 | 删除 fallback 目录, 路由, sidebar, import 重写, scope 收窄 | ✅ ui/ 代码已完成 | ✅ |
| **部署资产** | **values.yaml/values.schema.json/_helpers.tpl/README 删除 enterprise 段** | **大部分已实现（scim/cluster/alerting/auditLogs/loadbalancer/guardrails/circuit_breaker 已删）；vaultStore schema+注入段残留 ⚠️** | **❌ FAIL** |
| E2E 段 | 删除 mcp-tool-groups spec, utils_test 表名, plugins_test/ governanceroutes_test 期望 | utils_test/plugins_test/governanceroutes_test ✅; mcp-tool-groups ✅ 已删除 | ✅ PASS |

**P0 失败条件**: 缺核心 API/DTO 改动 → 不触发（vaultStore 非 API/DTO 但属于部署资产段承诺）

### 维度 3: scope creep 检查 (15% × 100 = 15.00)

- 所有变更文件在各 track 模块根内（core/framework/transports/plugins/ui/helm-charts/Makefile/tests/e2e）
- 修复提交 0b72c9d 仅改动 helm-charts/Makefile/tests/e2e，均在 proposal 声明范围内
- 无越界修改，无新增功能，无未声明文件改动

### 维度 4: 实现完整性 (v3.x, 10% × 90 = 9.00)

**硬标记扫描**（git diff master...HEAD 排除 .pg/ 和 .opencode/）：
```bash
$ git diff master...HEAD -- ':!.pg' ':!.opencode' | grep -E '^\+' | grep -nE '//\s*(TODO|FIXME|XXX|stub|not implemented)|@todo|UnsupportedOperationException' | grep -v '\[skip-review\]'
(0 命中)
```
✅ 生产代码 0 个新增 TODO/FIXME/XXX/stub/not-implemented

**tasks-V 映射核查**：
- 各 track 内部 tasks-V 映射完整 ✅
- 但 vaultStore 删除属于 design.md 承诺但 tasks.md 无对应任务分配 → 任务分配缺口（与 066 同根因）

**vaultStore 遗漏分析**：修复提交 0b72c9d 删除了 _helpers.tpl 中 Cluster Config、SCIM 等 enterprise 注入段，但完全未触碰 Vault Store 注入段（661-698 行 9 处引用 + 700 行 `set $cs "vault_store"`）。这属于"代码存在但未实质实现"的反面——不是 stub，而是设计承诺的删除未执行。

### runtime_boot (B3 修复，P0 硬约束)

| 步骤 | 证据 | 判定 |
|------|------|:----:|
| 1. 启动服务 | int verify 报告：restart_all_instances 原始输出（bifrost-api ready on 9080, ui-dev ready on 3008） | ✅ |
| 2. 等待就绪 | int verify 报告：health probe 30×3s 轮询 → OK | ✅ |
| 3. 真实 e2e | scenario 065: 7 个真实 API 调用（version/config/rbac/teams/metrics/virtual-keys/routing-rules）；int verify: 14 enterprise 端点 404 | ✅ |
| 4. 失败处置 | int verify 报告：无启动失败/探针超时 | ✅ |

**runtime_boot PASS** — 4 步证据齐全。修复提交仅改动 helm-charts/Makefile/e2e 静态资产，不影响运行时行为，065 scenario 验证的服务状态仍有效。

## 实现完整性核查段

### grep 输出

```bash
# 生产代码（非 .pg/ 非 .opencode/ 非 test）硬标记扫描
$ git diff master...HEAD -- ':!.pg' ':!.opencode' | grep -E '^\+' | grep -nE '//\s*(TODO|FIXME|XXX|stub|not implemented)|@todo|UnsupportedOperationException' | grep -v '\[skip-review\]'
(0 命中 — 无输出)
```

### 豁免列表

无。全 bugdiff 中所有 TODO/FIXME 命中均在 `.pg/` 变更管理文档文本中，非生产代码，无需豁免。

### tasks-V 映射核查结论

各 track 内部 tasks-V 映射完整（共 41 个 task 条目全部对应代码变更）。但 vaultStore 删除属于 design.md 承诺（design.md:179 列 vault_store 在删除清单 + 部署资产段明确"_helpers.tpl 删除 Vault Store 注入段"）但 tasks.md 无对应任务分配，属任务分配缺口。

## 维度分明细

| 维度 | 权重 | 维度分 | 加权分 | P0 触发 |
|------|:----:|:------:|:------:|:-------:|
| V-* 验证项通过率 | 50% | 85 | 42.50 | ✅ V-helm-1 FAIL |
| design.md 一致性 | 20% | 88 | 17.60 | ❌ |
| scope creep 检查 | 15% | 100 | 15.00 | ❌ |
| 测试质量 | 0% | — | — | — |
| 实现完整性 (v3.x) | 10% | 90 | 9.00 | ❌ |
| **gate-score** | **100%** | | **84.10** | **P0 触发 → FAIL** |

**PASS 条件**: gate-score ≥ 80 AND 无 P0 失败
**P0 条件**: V-helm-1 FAIL（vaultStore 残留 + 从未 helm template 验证）→ **整体 FAIL**