# AGENTS.md — celer-route 文档站（website/）

> 本文件为 AI agent 在 `website/` 目录工作时的约定补充，请与仓库根目录 `AGENTS.md` 配合阅读。

## 图片存放约定

文档（`docs/**/*.mdx`）中引用的图片，**不要**放进 `website/static/img/`，而是存放在该 mdx 文件同目录下、与 mdx **同名**的 `.assets/` 子文件夹里。

### 规则

- 文件夹名 = mdx 文件名去掉扩展名后加 `.assets`。例如 `provider-cooldown.mdx` → `provider-cooldown.assets/`
- 文件夹与 mdx 位于**同一目录**：`docs/features/provider-cooldown.mdx` 与 `docs/features/provider-cooldown.assets/`
- mdx 中用**相对路径**引用：`![alt](./provider-cooldown.assets/xxx.png)`
- Docusaurus 在构建时会自动将相对路径的图片资源打包到 `build/assets/images/` 并加内容哈希，无需手工处理

### 示例

```
docs/features/
├── provider-cooldown.mdx
└── provider-cooldown.assets/
    ├── provider-cooldown-plugin.png
    └── provider-cooldown-policy.png
```

```mdx
![提供商冷却插件页](./provider-cooldown.assets/provider-cooldown-plugin.png)
```

### 说明

- `website/static/img/` 仅用于全站通用资源（logo、favicon、social-card 等非文档专属图片）。文档专属截图一律走 `.assets/` 同目录方案，保证「文档与其配图就近聚合、随文档一起增删」。
- 如果一张图被多篇文章共用，再考虑放入 `static/img/`。

## 编写文档的通用要求

以下要求提炼自 `docs/features/provider-cooldown.mdx` 的编写过程，适用于新增或重写任意 `docs/**` 文档。

### 1. 语言

- 正文用**中文**；`docs/features/` 等面向用户的文档全篇中文。
- 保留原文不译：代码块、API 端点、配置字段名 / JSON key、CLI 命令、类名、文件路径、枚举值（如 `rate_limit`、`any`）。
- frontmatter 的 `title` / `description` 也写中文。

### 2. 视角：面向使用，而非实现

- 写"用户能做什么、在哪点、看到什么、怎么决策"，**不**写 `config.json` 字段表、JSON 片段、REST 端点示例这类实现细节。
- 用操作路径组织内容（启用 → 监控 → 配置 → 常见场景），而非按数据结构/接口组织。
- 技术字段只在"用户在 UI 里确实要填它"时才出现，且解释用户视角的含义。

### 3. 内容必须源于真实代码扫描

- 动笔前先用 `codegraph_explore` / Read 扫描：UI 片段组件、`ui/locales/zh-CN/*.json` 文案、后端 schema 与默认值、相关常量。
- 文档里的字段名、默认值、界面标签、行为语义都要与代码一致，**不得凭空编造**。例：默认 TTL 300s、OpenAI 限流 60s/配额 600s 来自 `core/schemas/provider.go` 的 `DefaultCooldownPolicy`。

### 4. 界面元素引用真实中文标签

- UI 中的按钮 / 标题 / 开关名，用界面实际显示的中文，取自 `ui/locales/zh-CN/<domain>.json`，而非自行英译中。
- 例：写"解除冻结""启用提供商冷却""清除自定义策略"，不写"Unfreeze""Enable"。

### 5. 截图验证（涉及 UI 的文档）

- 通过 hooks 协议启动 celer-route（`pg-invoke-hook.py`，见根 AGENTS.md §环境生命周期），**不要**直接 `make dev`。
- 用 Chrome DevTools MCP 截图；截图前**先把界面切到简体中文**（侧边栏语言开关 → 简体中文）。
- 截图存入该文档的 `.assets/` 目录（见上文「图片存放约定」），mdx 用相对路径引用。

### 6. 结构模板

推荐章节顺序（按需裁剪）：

1. frontmatter：`title` / `description` / `sidebar_position`
2. 一句话简介：这是什么 + 用户能得到什么
3. 工作原理 / 概念：用**表格**讲触发→影响→恢复这类流程语义
4. 操作步骤：按用户操作路径分节（启用 → 监控 → 配置 …），每节配 UI 截图
5. 默认行为：开箱即有的保护，无需配置
6. 常见场景：把典型问题映射到具体操作

### 7. 构建验证

- 写完跑 `npx docusaurus build`，**双语言（zh-CN / en）都要通过**。
- 确认图片被正确内嵌（`build/assets/images/` 下出现带哈希的产物，对应 html 引用到了图片）。
