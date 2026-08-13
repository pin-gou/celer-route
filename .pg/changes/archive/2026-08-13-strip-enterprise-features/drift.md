# Design Drift Log

## Drift #1
- **发现阶段**: scenario-fix
- **场景**: S-bifrost-api-config-still-works
- **位置**: scenario-scenario.yaml:then assertion
- **原因**: design asserts response.body matches "config": but actual API contract uses client_config key (pre-existing, handler unmodified by this change)
- **决策**: ACCEPT

## Drift #2
- **发现阶段**: scenario-fix
- **场景**: S-bifrost-api-version-after-strip
- **位置**: scenario-scenario.yaml:then assertion
- **原因**: design asserts response.body matches "version":\s*".+" but actual API contract returns bare JSON string "v1.0.0" (getVersion=SendJSON(string), handler unmodified by this change)
- **决策**: ACCEPT

## Drift #3
- **发现阶段**: scenario-fix
- **场景**: S-bifrost-api-no-enterprise-endpoints-registered,S-bifrost-api-routing-rules-list
- **位置**: scenario-scenario.yaml:when/then assertions
- **原因**: scenario 文件假设 teams/customers 应 404（实际为 OSS 保留功能 200）且 routing-rules 路径缺 /governance 前缀；已由编排器 escalate 提交 c7565b6 修正，代码行为与修正后契约一致
- **决策**: ACCEPT

