---
name: build-and-publish
description: 构建 multi-arch Docker 镜像并推送到 GHCR，然后自动分析 Git 变更生成 CHANGELOG，最后在 GitHub 创建 Release。接受一个 version 参数（格式 vX.Y.Z）。
license: MIT
compatibility: 需要 gh CLI 已登录、docker login ghcr.io 已配置、当前目录为 Bifrost 仓库根目录
metadata:
  author: pg-spec
  version: "1.0"
---

# build-and-publish

构建 multi-arch Docker 镜像并推送到 GHCR → 自动生成 CHANGELOG → 创建 GitHub Release。

## 前置条件

| 项 | 要求 | 校验失败行为 |
|----|------|------------|
| `gh` CLI 已登录 | `gh auth status` 通过 | 终止并提示登录 |
| `docker login ghcr.io` 已配置 | 可推送至 ghcr.io | 终止并提示登录 |
| 当前目录为仓库根目录 | `Makefile` 存在 | 终止并提示 |
| git 工作区干净 | `git status --porcelain` 为空 | 终止并提示提交或 stash |
| 参数 `version` | 格式 `vX.Y.Z`（如 `v1.2.3`） | 终止并提示正确格式 |

## 参数

SKILL 接受一个参数 `version`，格式为 `vX.Y.Z`（例如 `v1.5.0`）。

## 核心流程

### 步骤 1：校验参数与前置条件

```bash
version="$1"
if [[ -z "$version" ]]; then
  echo "ERROR: 必须提供 version 参数（格式 vX.Y.Z）"
  exit 1
fi
if ! echo "$version" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "ERROR: version 格式错误，应为 vX.Y.Z（例如 v1.5.0）"
  exit 1
fi
```

### 步骤 2：切换到 main 分支并拉取最新代码

```bash
git checkout main
git pull origin main
```

### 步骤 3：构建 multi-arch Docker 镜像并推送

```bash
make docker-image-multiarch VERSION="$version"
```

### 步骤 4：分析变更并生成 CHANGELOG

获取当前 tag 和上一个 tag，对比 git log 并生成用户视角的 CHANGELOG：

```bash
# 获取当前 tag 和上一个 tag
current_tag="$version"
prev_tag=$(git tag --sort=-version:refname | grep -v '\-rc' | grep -v '\-alpha' | grep -v '\-beta' | head -n 2 | tail -n 1)

# 如果当前 tag 已存在（本地重复执行），则 fallback 到基于 HEAD 的最近 tag
if git rev-parse "$current_tag" >/dev/null 2>&1; then
  current_tag="HEAD"
fi

if [[ -z "$prev_tag" ]]; then
  # 没有上一个 tag，取所有变更
  log_range="HEAD"
  prev_tag="（首次发布）"
else
  log_range="${prev_tag}..HEAD"
fi
```

然后使用 `git log` 提取结构化变更：

```bash
git log --oneline --no-decorate "$log_range"
git log --format="%s" "$log_range"
```

根据 commit message 的 prefix（`feat`、`fix`、`refactor`、`docs`、`test`、`chore`、`style`）分类，提炼为**用户视角**的 CHANGELOG。规则：

- `feat` → **新功能**
- `fix` → **Bug 修复**
- `refactor` → **重构**
- `docs` → **文档**
- `test` → **测试**
- `chore` / `style` → **其他**

每条 commit 提炼为简明的一句话，从用户视角描述。例如：
- "feat: 添加 OpenAI 兼容的流式响应支持" → "支持 OpenAI 兼容的流式响应（SSE 分块传输）"
- "fix: 修复长文本请求时 token 计数溢出问题" → "修复长文本请求时 token 计数溢出的问题"

#### 两级结构：按功能聚合

CHANGELOG 采用**两级结构**：第一级是分类（`### 新功能`、`### Bug 修复` 等），第二级是**功能/模块名**。每个一级分类下，先识别本次变更涉及的「功能主题」，把同主题的多个 commit 聚合到同一个二级条目下，组内逐条罗列子项。

主题识别方法：
- 优先取 commit message 的作用域（Conventional Commits 括号里的部分，如 `feat(rtk): ...`、`fix(rtk): ...`），同作用域的 commit 归入同一主题（如 `rtk`、`docs`）。
- 无作用域时，按 commit 内容关键词人工归类（例如"文档站支持中英双语"和"文档站图片支持点击放大"都属于"文档站"主题）。
- 单条、孤立的 commit 直接作为一条二级条目，主题名就是该条目本身的一句话摘要。

二级条目格式：`**主题名**`（加粗），下面是若干子项，子项用 `-` 列表。最终用户看到的列表是叶子级条目（即"## v1.5.0"下平铺的若干 `- xxx`），但这些叶子按主题聚合展示，例如：

```markdown
### ✨ 新功能

**RTK 压缩**
- 新增 Caveman 自然语言压缩引擎
- 新增各压缩引擎独立统计面板
- 启用免鉴权 URL 恢复原始输出

**文档站**
- 支持中英双语切换
- 站内图片支持点击放大

**仪表板与路由**
- 仪表板提供商用量标签新增供应商排名表格
- 路由规则支持拖拽及上/下按钮调整优先级并持久化
```

#### Bug 修复筛选：只保留针对已发版代码的修复

`fix` 分类下**只保留针对上一个发布版本已存在代码的修复**。新建功能（本次版本首次引入）开发过程中产生的 bug fix，**不列入** "Bug 修复" 分类，而是归入对应新功能的二级条目下，作为该功能的子项之一。

判定方法：
- 看 commit message 的作用域和上下文——若修复的是**本次版本内新引入**的功能（典型信号：commit 与某个 `feat(<scope>): ...` 出现在同一批变更中，修复的代码路径在最近几次 feat 提交里被新建），则归入「新功能」中对应主题的子项。
- 若修复的对象在上一个发布版本已存在（典型信号：修复的是旧有的公共路径、核心组件、或上一个版本就存在的 UI 行为），则单独列入「Bug 修复」。
- 难以判定时，**倾向不列入 Bug 修复**——保守地把修复归入对应新功能主题，避免把开发期的修整污染用户视角的修复清单。

#### 完整 CHANGELOG 格式示例

```markdown
## v1.5.0 (2026-08-20)

### ✨ 新功能

**RTK 压缩**
- 新增 Caveman 自然语言压缩引擎
- 新增各压缩引擎独立统计面板
- 启用免鉴权 URL 恢复原始输出

**文档站**
- 支持中英双语切换
- 站内图片支持点击放大

**仪表板与路由**
- 仪表板提供商用量标签新增供应商排名表格
- 路由规则支持拖拽及上/下按钮调整优先级并持久化

### 🐛 Bug 修复

**核心组件**
- 修复长文本请求时 token 计数溢出的问题
- 修复流式响应偶尔丢失最后一段数据的问题

**仪表板**
- 路由切换后图片点击放大失效的问题

### 🔄 重构
- 提取 HTTP 客户端为共享工具函数，减少各 provider 重复代码

### 📖 文档
- 更新 OpenAI 兼容 API 的配置示例
```

### 步骤 5：创建 GitHub Release

```bash
# 创建本地 tag
git tag "$version"
git push origin "$version"

# 创建 release
# 将 CHANGELOG 内容写入临时文件
cat > /tmp/changelog-${version}.md << 'CHANGELOG_EOF'
{{CHANGELOG_CONTENT}}
CHANGELOG_EOF

gh release create "$version" \
  --title "$version" \
  --notes-file /tmp/changelog-${version}.md

rm -f /tmp/changelog-${version}.md
```

## 完整执行脚本

以下脚本由 SKILL 加载后逐步骤执行（不可直接复制粘贴，需根据实际输出调整 CHANGELOG）：

```bash
# 由 agent 手动执行，每步暂停确认
set -e

version="$1"

# 步骤 1：校验
[[ -z "$version" ]] && { echo "ERROR: 需要 version 参数"; exit 1; }
echo "$version" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$' || { echo "ERROR: 格式错误"; exit 1; }
gh auth status
docker info
[[ -f Makefile ]] || { echo "ERROR: 不在仓库根目录"; exit 1; }
[[ -z "$(git status --porcelain)" ]] || { echo "ERROR: 工作区不干净"; exit 1; }

# 步骤 2：切换分支
git checkout main
git pull origin main

# 步骤 3：构建镜像
make docker-image-multiarch VERSION="$version"

# 步骤 4：分析变更
current_tag="$version"
prev_tag=$(git tag --sort=-version:refname | grep -vE '\-(rc|alpha|beta)' | head -n 2 | tail -n 1)
if git rev-parse "$current_tag" >/dev/null 2>&1; then
  current_tag="HEAD"
fi
if [[ -z "$prev_tag" ]]; then
  log_range="HEAD"
  prev_tag="（首次发布）"
else
  log_range="${prev_tag}..HEAD"
fi

echo "=== 对比范围: ${prev_tag} → ${version} ==="
git log --oneline --no-decorate "$log_range"
echo ""

# agent 在此处手动分析并生成 CHANGELOG markdown
# ...

# 步骤 5：创建 Release
git tag "$version"
git push origin "$version"
gh release create "$version" --title "$version" --notes-file /tmp/changelog-${version}.md
```

## 报告格式

### 成功时

```
## 构建与发布完成

**版本：** {{version}}
**工作流：** build-and-publish

### 构建产物
- **Docker 镜像：** ghcr.io/pin-gou/celer-route:{{version}}（multi-arch: linux/amd64, linux/arm64）
- **GitHub Release：** https://github.com/{{owner}}/{{repo}}/releases/tag/{{version}}

### CHANGELOG 摘要

（此处展示生成的 CHANGELOG 内容）

### 下一步
- 验证 Docker 镜像已推送：`docker pull ghcr.io/pin-gou/celer-route:{{version}}`
- 验证 Release 已创建：`gh release view {{version}}`
```

### 失败时

```
## 构建与发布失败

**版本：** {{version}}
**工作流：** build-and-publish
**状态：** FAILED

### 失败原因
- **失败步骤：** {{步骤名}}
- **失败详情：** {{描述}}

### 未执行的步骤
- {{未执行的步骤列表}}
```

## 安全规则

- 仅在 `main` 分支上执行发布流程
- 本地 tag 创建后立即 `git push origin`，避免本地残留
- 工作区不干净时拒绝执行，防止误提交未完成的变更
- `gh release create` 使用 `--notes-file` 而非 `--notes`，避免 shell 转义问题
- 临时 changelog 文件在 release 创建后立即删除

## 明确不做的事

- 不做代码审查或测试——发布前应已通过 CI
- 不修改 `AGENTS.md` 或版本文件——由发布流程独立管理
- 不推送 `latest` tag 以外的 Docker 标签——`make docker-image-multiarch` 已经处理
- 不创建 PR——发布流程直接从 main 分支进行