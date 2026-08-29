---
name: refresh-free-tier-catalog
description: 刷新 website/static/recommended-providers/{zh-CN,en}.json 中的免费套餐目录——通过搜索引擎核查现有条目是否仍在提供免费 token（API key 免费额度、keyless 免费接口等），下线已停止活动的供应商，补齐替代或新增条目；编辑完成后用内置 schema 校验脚本（镜像 Go 端 CatalogHandler 的标注规则，确保无效条目不会被运行中的网关接收）兜底校验，最后自动推送到仓库。无参数调用 = 全量刷新；带参数调用 = 聚焦某个提供商或主题。
allowed-tools: Read, Grep, Glob, Bash, WebFetch, Task, TodoWrite, Edit, Write
---

# 刷新免费套餐目录

让 `website/static/recommended-providers/{zh-CN,en}.json` 保持"能点的都是真的免费"。
该目录通过 GitHub Pages 托管，运行中的每个网关都会经 `CatalogHandler`
（见 `transports/celer-route-http/handlers/catalog.go`）拉取，是首页"免费套餐推荐"
卡片的唯一真相源；条目过期或失效会直接表现为用户一键配置失败。

本 skill 单次执行一整套流程：

1. **盘点**：读两个 JSON，列出当前推荐的所有供应商及形态（内建/自定义兜底、有 key/keyless）。
2. **核实**：对每个供应商 + 2–3 个候选，调用 WebFetch 搜索"现在还免费吗"——
   套餐是否还在、apply URL 是否 200、模型清单是否漂移、是否已转收费。
3. **取舍**：把已经停服的条目删掉；字段漂移的条目就地更新；发现新的真正免费渠道则按其
   形态（内建/自定义兜底）补进对应 bundle。**严格外科手术式编辑**，绝不重排无关条目。
4. **同步**：`zh-CN.json` 与 `en.json` 同步编辑（运行时会按请求头选一个文件，
   schema 强制两者 bundle.id + provider 集合一致）。
5. **版本号**：两个文件统一 bump `version`（yyyy-mm-dd）和 `updated_at`（RFC3339）。
6. **校验**（强制，不可跳过）：`scripts/validate_catalog.py`，镜像 Go 端规则——
   base_provider ∈ SupportedBaseProviders、http(s) URL、bundle/provider 不重复、
   双语文件 parity。
7. **汇报 + 等待人工确认**：在 main 分支上原地修改，先把变更摘要（含 diff 关键 diff 块、
   校验器输出）打出来，**等用户明确说"提交"或"推送"后才执行 git add/commit/push**。
   流程内不做无人值守的 commit 或 push。

> **关于分支策略**：本 skill 默认在 `main` 分支直接修改（与仓库约定一致——
   `recommended-providers/*.json` 通过 GitHub Pages 直接服务运行中的网关，
   走 PR 反而会让运行时短暂拿到过期快照）。如有特殊需要走 PR，可在调用时说明。

## 第 1 步 —— 盘点当前目录

```bash
ls website/static/recommended-providers/
python3 .opencode/skills/refresh-free-tier-catalog/scripts/validate_catalog.py
```

读两个文件，输出扁平表 `(bundle_id, provider, base_provider_or_none, is_keyless)`，
这就是本轮工作集。带 `base_provider` 的条目走自定义供应商兜底路径，也要顺手验一下
兜底目标是否还活着（自定义目标 404 和"删了免费套餐"一样破）。

## bundle 分类约定（硬规则，refresh 时不再即兴发挥）

> 个人用户打开 LLM 的主流场景 = bundle 分类的唯一依据。分类反映"用户在哪种工作场景
> 下会点开这个目录"，不是"供应商能干什么"，也不是"价格梯度"。

| `bundle.id`（稳定 slug，不改名） | 中文标题 | 英文标题 | 用户场景 | 典型成员 |
|---|---|---|---|---|
| `coding` | 编程开发 | Coding & Development | 代码补全、bug 定位、Code Review、写脚本、shell/SQL 小段 | OpenAI、Anthropic、DeepSeek-Coder、opencode、together、sambanova |
| `data-science` | 数据科学 | Data Science | 数据分析、自然语言→SQL/Pandas、报表解读、图表 | deepseek（chat/reasoner）、pollinations、智谱 GLM-4-Flash、Qwen-Long |
| `image-generation` | 图像生成 | Image Generation | 文生图、配图、icon、海报、头像、图改图、扩图 | runware、Stability、Pollinations image、Together Imagen |
| `speech` *(按需启用)* | 语音 / TTS STT | Speech | 配音、会议纪要、字幕、有声书 | ElevenLabs（keyless 试用）、edge-tts（keyless 自定义） |
| `video` *(按需启用)* | 视频生成 | Video Generation | 短宣传片、产品演示、视频总结、文生视频 | runway、Replicate（keyless 试用）、Pika |

**硬规则**：

1. **`bundle.id` 是稳定 slug，不改名**。首页、i18n、UI 测试、GitHub Pages URL
   全都按 `coding` / `data-science` / `image-generation` 锚定。重命名会引入
   大范围连带改动——只允许新增 bundle，不允许改名 / 合并 / 拆分。
2. **同语义条目可跨 bundle 复用**。如果某供应商同时在编程 + 数据科学都好用，
   按"主用场景"放在最相关的 bundle 里，**不重复出现**。
3. **`description` 一句话说清"为什么点这个 bundle"**。这是用户在 header 看到
   的那行字，必须能用一句话定位"我是为了 X 才点开的"。
4. **新增 bundle 的触发条件**：该场景下 ≥ 2 个真实可用的免费供应商。**不预留**
   空 bundle，也不在只有 1 个供应商时新增（UI 留空一格视觉负担大）。
5. **删除 bundle 的触发条件**：该场景下所有供应商全部停服。**不允许保留空壳**
   ——首页显示 0 个 provider 比隐藏 bundle 更让人困惑。
6. **keyless 与否不是分类维度**。`is_keyless: true` 是单条目属性，不是 bundle
   维度：同一 bundle 里 keyless 与非 keyless 共存即可（对话窗已自动分流 UI）。
7. **判断新条目归属的提问**："用户在哪种场景下会点开这个供应商？"——场景 = bundle，
   不是"它能干什么"。

新增条目时，按下表判断 bundle 选择（决策树）：

```
用户在哪种场景下会点开这个供应商？
├─ 写代码 / 调试 / 写脚本 → coding
├─ 跑数据分析 / 写 SQL / 处理表格 → data-science
├─ 出图 / 改图 / 配图 → image-generation
├─ 出声音 / 转文字 / 字幕 → speech（前提：本类已有 ≥1 个供应商）
├─ 出视频 / 视频总结 → video（前提：本类已有 ≥1 个供应商）
└─ 其它（写作 / 学习 / 翻译 / 角色扮演 / RAG 长文档 / 信息查询）
   └─ 默认归 coding（大多数编程模型都擅长通用对话，且 coding 是用户
       点开频率最高的入口，避免分散到无名 bundle）
```

## 第 2 步 —— 核实免费状态

对工作集中的每个供应商，并行跑 WebFetch 搜索（中文条目加 `国内` 关键字）：

- 关键词模板：`<provider> free tier 2026` / `<provider> 免费额度 2026` /
  `<provider> discontinued free tier` / `<provider> free quota removed 2026` /
  `<provider> 停止免费` / `<provider> 转收费`
- 图像/视频专长：`<provider> free credits monthly 2026`
- keyless 选项：`free OpenAI-compatible API no key 2026` / `免 Key OpenAI 兼容 API`
- 中国区：`<provider> 国内 免费 2026`、优先采信中文社区与官方公告

每个供应商产出以下记录：

- **状态**：alive / degraded / sunset / unknown
- **免费额度**：还在吗？（"$5 credit" → "$0 credit" 是高危信号）
- **apply URL**：还 200 吗（用 WebFetch 探活）
- **模型**：免费模型清单是否变化
- **动作**：stay（含编辑）/ drop / replace-with-X

遇到供应商状态有歧义且删除成本高时，用 AskUserQuestion 让用户拍板
（如：官网 502 但社区说临时维护）。

## 第 3 步 —— 编辑（外科手术式）

每个改动只动需要动的字段。规则：

- **绝不**重排、改名、reformat 无关条目；不要调整 JSON 风格、缩进、键顺序
  （这是审查 diff 的人类共识）。
- **删除**：删掉 `providers` 数组里的整个 provider 对象；保留 bundle（即使只剩 1 个
  provider，UI 仍要 bundle header）。
- **新增**：往对应 bundle 的 `providers` 数组里插一个对象。非内建条目必须填
  `base_provider` + `base_url`（服务器端无兜底字段会被标 unsupported=disabled）。
- **就地编辑**：只改漂移的字段；`provider` 键保持稳定（改键名会让已经缓存该行的
  客户端错位）。
- **不要翻转 `is_keyless`**：这会改变对话框 UI（api-key 输入框出现/消失），仅在
  多个来源交叉确认后才改。

编辑完两个文件，统一 bump：

```json
"version": "<today yyyy-mm-dd>",
"updated_at": "<today yyyy-mm-dd>T08:00:00Z"
```

## 第 4 步 —— 校验（强制，不可跳过）

```bash
python3 .opencode/skills/refresh-free-tier-catalog/scripts/validate_catalog.py
```

脚本镜像 Go 端 `CatalogHandler` 的全部规则：base_provider 白名单、http(s) URL、
bundle/provider 不重复、双语 parity。**任何错误必须修完再继续**——不要把运行中
网关会拒收的目录推上去。

## 第 5 步 —— 提交 + 推送（仅在用户明确确认后执行）

**在用户未确认前不要做任何 git 写操作。**

修改全部落在 `main` 分支（GitHub Pages 服务的就是 main HEAD；走 feature 分支
会让运行中的网关短暂拿到过期快照，反而违背"目录保持新鲜"的初衷）。

确认流程：

1. 校验通过后，先打印一份 **变更摘要**（见第 6 步），用 AskUserQuestion 或文字
   询问"是否提交并推送"。**等用户明确说"提交"/"推送"才继续**。
2. 用户同意后执行：

   ```bash
   git add website/static/recommended-providers/
   git commit -m "chore: 刷新免费套餐目录 (drop <p1>, <p2>; add <p3>)"
   git push origin main
   ```

3. 用户拒绝或要求调整：回到第 3 步继续编辑，重新校验，重新询问。

## 第 6 步 —— 汇报 + 等待确认

输出变更摘要，**提交前必须打印并停下等待用户确认**。摘要至少包含：

- 新增条目（写明 bundle + base_provider）
- 删除条目（写明核实的证据/出处）
- 就地编辑条目（写明哪个字段漂移、漂移成什么）
- 版本号：`version` / `updated_at` 旧值 → 新值
- 校验器输出（必须是 `OK:`）

然后用 AskUserQuestion 或文字明确问：

```
确认以上变更后，是否提交并推送到 origin/main？
[ ] 提交并推送
[ ] 调整后再确认
[ ] 放弃本次改动
```

只在用户选 1 时才执行第 5 步的 git 命令。

## 失败模式

- **WebFetch 超时 / 失败**：换关键词二次尝试；仍失败则保留原条目，汇报中标注
  "could not verify"，**不要在不确定时删除**。
- **只改了 zh-CN 漏掉 en**：parity 校验会报错。永远同 pass 同改两个文件。
- **参数只指定一个提供商**：聚焦核验该提供商 + 同主题 2 个替代，但**仍要跑全量
  校验和 parity 检查**。
- **用户撤回 / 没看到确认请求**：不要做任何 commit / push；保持工作树 dirty，
  把当前状态汇报给用户。