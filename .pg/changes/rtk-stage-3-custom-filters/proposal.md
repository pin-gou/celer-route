# rtk-stage-3-custom-filters
**关联 issue**：无
**变更类型**：feature

## 背景
oc2-gateway 的 RTK 插件（plugins/rtk/）当前 FilterLoader 仅支持 embed.FS 内置过滤器（53 个 legacy JSON）+ 内存中 Register{Project,Global}Filter API 注入，且 `compression.go` 中 `getFilterLoader()` 硬编码 `defaultConfig = &Config{Enabled: true}`——这意味着**用户/部署方无法通过文件系统提供自定义过滤器**，并且即便新增 `enabled_filters` / `disabled_filters` / `trust_project_filters` 等 Config 字段也会因单例不读 Plugin.config 而失效。这是相对于 OmniRoute RTK 引擎（`open-sse/services/compression/engines/rtk/`）的明显生态差距。

OmniRoute 已实现的三级加载（project > global > builtin）、双格式 Filter JSON（canonical + legacy）、trust.json SHA256 信任模型、enabled/disabled 白/黑名单、TOML 兼容性——本阶段对齐其中三级加载 + 双格式 + trust 校验 + 白/黑名单四项核心能力，TOML 解析留待阶段四。

## 目标
1. FilterLoader 支持三级 source 加载：project (`<app-dir>/.rtk/filters.json`) > global (`<app-dir>/rtk/filters.json`) > builtin (`plugins/rtk/filters/builtin/*.json`)，按 sourceRank + formatRank + priority + id 排序
2. Filter struct 兼容 legacy（name/command/rules/head/tail/max_lines/priority_patterns）+ canonical（id/label/category/priority/match/rules/preserve/tests）双格式，53 个内置 JSON 零迁移
3. project filters 通过 `trust.json` SHA256 校验（OMNIROUTE_RTK_TRUST_PROJECT_FILTERS / BIFROST_RTK_TRUST_PROJECT_FILTERS 任一为 1 即旁路）
4. `enabled_filters` / `disabled_filters` Config 字段按 id（canonical）或 name（legacy）双匹配
5. **修隐式 bug**：`Plugin.Init` 持有 `*FilterLoader`，`compression.go` 通过 `p.loader.Match(...)` 取 filter，Config 字段真正生效

## 范围
### 包含
- `plugins/rtk/filterloader.go` 重写：双格式 struct + UnmarshalJSON 仲裁、三级 source 加载（project/global/builtin）、trust.json SHA256 校验（4 场景）、env var 旁路、TOML 识别 warn 跳过、`Diagnostics()` 接口暴露、`enabled_filters` / `disabled_filters` 白/黑名单过滤、加载后缓存为 loader 实例持有状态
- `plugins/rtk/linefilter.go`：Filter struct 扩展为兼容两格式的胖 struct（id/label/category/priority/match{commands,patterns,outputTypes}/rules{includePatterns,dropPatterns,collapsePatterns,stripAnsi,replace,matchOutput,truncateLineAt,onEmpty,filterStderr,deduplicate,headLines,tailLines,maxLines}/preserve{errorPatterns,summaryPatterns}/tests 等）+ 自定义 `UnmarshalJSON` 仲裁
- `plugins/rtk/config.go`：新增 `CustomFiltersEnabled` / `TrustProjectFilters` / `EnabledFilters` / `DisabledFilters` 4 字段
- `plugins/rtk/rtk.go`：`Init(ctx, config, logger, appDir)` 多收一个 appDir 参数；新增 `*FilterLoader` 字段
- `plugins/rtk/compression.go`：删硬编码 `getFilterLoader()` 包级单例；改通过 `Plugin` 实例 `loader` 字段取 filter；保留 `applyLineFilter` / `scaleFilterForIntensity` 等下游契约
- `plugins/rtk/hooks.go`：`PreLLMHook` / `PostLLMHook` 通过 `p.loader.Match(...)` 取 filter
- `plugins/rtk/filterloader_test.go` / `config_test.go`：新增双格式加载、信任模型、白/黑名单、env var 旁路、ReDoS 拒绝等场景测试；不删改阶段一/二已通过的 27+ 个断言
- `transports/config.schema.json`：rtk 配置块新增 4 字段定义

### 不包含
- TOML 解析实现（OmniRoute `tomlCompatibility.ts` 334 行）——阶段四前置依赖
- 文件 watch / 热重载机制——`plugins/rtk/` 无 reload 架构，超出阶段三
- UI / API 暴露自定义过滤器管理面板——阶段六
- 53 个内置 JSON 文件结构改写——UnmarshalJSON 仲裁兼容 zero migration
- 服务端 live 端到端验证（V-plugins-4 degraded）——单元测试已覆盖 Loader 行为；live E2E 留阶段六

## 方案概述
**Filter struct 双格式兼容**：在 `plugins/rtk/linefilter.go` 把现有 `Filter` struct 扩展为胖 struct，含 legacy 7 字段 + canonical 27 字段（id/label/category/priority/match/rules/preserve/tests），所有字段带 `omitempty` JSON tag。自定义 `UnmarshalJSON` 实现仲裁逻辑：先 `json.Unmarshal` 进 struct，然后若 canonical `head_lines`/`tail_lines`/`max_lines` 任一非零则覆盖 legacy `head`/`tail`/`max_lines`；反之若 legacy 有值而 canonical 缺则拷贝过去，保证 53 个内置 JSON 零迁移。

**FilterLoader 三级加载**：新增 `Load(appDir string)` 方法，按 `<app-dir>/.rtk/filters.{json,toml}` → `<app-dir>/rtk/filters.{json,toml}` → embed.FS `filters/builtin/*.json` 顺序收集 sources，按 `(sourceRank desc, formatRank desc, priority desc, id asc)` 排序。TOML 文件被识别后写 warn 并跳过（阶段四补解析）。`Match()` 在已加载 slice 上找最长前缀匹配。

**Trust 模型**：`projectFiltersTrusted(filtersPath, trustProjectFilters)` 返回 `bool|"changed"`：
- `trustProjectFilters=true` 或 `OMNIROUTE_RTK_TRUST_PROJECT_FILTERS=1` 或 `BIFROST_RTK_TRUST_PROJECT_FILTERS=1` → `true`（旁路）
- 否则读 `<project-dir>/trust.json`，字段优先顺序：`filtersSha256` > `trustedFiltersSha256`（兼容 OmniRoute 旧字段）；SHA256 匹配 → `true`，不匹配 → `"changed"`，缺失 → `false`
- `false` / `"changed"` 都跳过该 source，写 warn diagnostic，Loader 仍能继续加载其他 source

**Plugin 持有 Loader**：删 `compression.go` 中包级 `globalLoader` + `getFilterLoader()` + `defaultConfig` 硬编码；改 `Plugin` struct 持有 `*FilterLoader` + `appDir string`；`Init(ctx, config, logger, appDir)` 构造时调 `NewFilterLoader(config).Load(appDir)`；`hooks.go` 通过 `p.loader.Match(...)` 取 filter。`Cleanup()` 释放（loader 无显式资源，目前仅清 stateStore）。

**Config 字段**：4 个新字段带 `Validate()` 校验：枚举 / 字符串数组允许空值；`EnabledFilters` 与 `DisabledFilters` 同时非空时记录 warn（白/黑名单交集可能为空）。

**transports/config.schema.json**：rtk 配置块 `additionalProperties: false` 下新增 4 字段 schema，附 description。

## 风险和注意事项
- **53 个内置 JSON 兼容性**：新胖 struct 字段全部带 `omitempty`，53 个内置 legacy JSON 反序列化时多余字段为 nil，下游 `applyLineFilter` / `smarttruncate.go` 仅读 legacy 字段，行为不变。已通过 `TestFilterLoaderBuiltin` 等 4 个 fixture test 隐式验证。
- **trust.json 静默跳过**：project filters 在 trust 校验失败/缺失时仅写 warn diagnostic，用户可能未察觉。阶段六 UI 接入 `Loader.Diagnostics()` 时把 diagnostics 接到前端（本次不做）。
- **多 Plugin 实例内存成本**：每个 `Plugin` 持有独立 `*FilterLoader` 缓存（53 builtin + N project + N global filters），内存 = `O(单 Plugin filter 数)`。无共享缓存设计；DB-backed 多 provider 多 config 切换场景下若内存压力明显，可后续引入 LRU。当前阶段三不引入共享机制。
- **env var 双名**：`OMNIROUTE_RTK_TRUST_PROJECT_FILTERS` 与 `BIFROST_RTK_TRUST_PROJECT_FILTERS` 任一为 1 即生效（兼容 OmniRoute 历史命名），不引入优先级判定。
- **TOML 临时 warn**：阶段三识别 `.toml` 但不解析，写 warn "TOML support planned for stage 4, skipping"，用户可能误以为配置丢失。阶段四补 parser 后即移除。
- **Plugin.Init 签名变更**：`Init(ctx, config, logger, appDir string)` 多收一个参数。下游调用方（PG plugin 注册器 / DB-backed plugin loader）需同步更新；本次修改面仅 `plugins/rtk/`，无外部调用方。

**约束满足**：上述 6 条风险对应 `define-summary.yaml` 中 4 个 V-*（V-plugins-1/2/3/4）；其中 V-plugins-1/2/3 可在 `go test ./plugins/rtk/... -count=1` 中验证；V-plugins-4 (服务端 live) degraded，design.md「环境限制与验证策略」段记录降级路径。
