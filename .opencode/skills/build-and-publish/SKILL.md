---
name: build-and-publish
description: 基于当前分支构建 multi-arch Docker 镜像并推送到 GHCR，然后自动分析 Git 变更生成 CHANGELOG，最后在 GitHub 创建 Release。执行前会就当前分支与相较上一版本的新增 commit 与用户二次确认。接受一个 version 参数（格式 vX.Y.Z）。
license: MIT
compatibility: 需要 gh CLI 已登录、docker login ghcr.io 已配置、当前目录为 Bifrost 仓库根目录
metadata:
  author: pg-spec
  version: "1.0"
---

# build-and-publish

构建 multi-arch Docker 镜像并推送到 GHCR → 自动生成 CHANGELOG → 创建 GitHub Release。

发布**基于当前所在分支**（不强制 `main`）。执行任何构建/发布动作前，必须先与用户二次确认当前发布分支及相较上一版本新增的 commit。

## 前置条件

| 项 | 要求 | 校验失败行为 |
|----|------|------------|
| `gh` CLI 已登录 | `gh auth status` 通过 | 终止并提示登录 |
| `docker login ghcr.io` 已配置 | 可推送至 ghcr.io | 终止并提示登录 |
| 当前目录为仓库根目录 | `Makefile` 存在 | 终止并提示 |
| 当前分支 | 非 detached HEAD（发布基于当前分支，不强制 `main`） | 终止并提示切换到具体分支 |
| git 工作区干净 | `git status --porcelain` 为空 | 终止并提示提交或 stash |
| 参数 `version` | 格式 `vX.Y.Z`（如 `v1.2.3`） | 终止并提示正确格式 |
| 交叉编译工具链 | `bash .github/workflows/scripts/install-cross-compilers.sh` 可自动安装（需 sudo，或已手动装好）；产物校验见步骤 3.5 | 工具链不可用且无法安装时终止——二进制为硬依赖，不发二进制不建 release |

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

### 步骤 2：确认当前发布分支

发布**基于当前所在分支**，不强制切换到 `main`。仅需确保处于具体分支（非 detached HEAD），并在有上游时快进到上游最新：

```bash
branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$branch" == "HEAD" ]]; then
  echo "ERROR: 处于 detached HEAD，无法确定发布分支——请先切换到一个具体分支"
  exit 1
fi

# 若当前分支有上游，快进到上游最新（无上游则跳过，仅使用本地提交）
if git rev-parse --abbrev-ref --symbolic-full-name '@{u}' >/dev/null 2>&1; then
  git pull --ff-only
else
  echo "WARN: 当前分支 ${branch} 无上游，使用本地提交发布"
fi

echo "当前发布分支: ${branch}"
```

### 步骤 2.5：二次确认发布分支与新增提交（必须用户确认）

在**任何构建/发布动作之前**，必须把「当前分支」和「相较上一发布版本新增的 commit」呈现给用户，并取得明确确认；用户未确认或选择终止时立即停止。

```bash
# 上一个发布 tag = 版本序最新、且不等于本次 version 的非预发布 tag
# （本次 tag 可能尚未创建，也可能已存在，两种情况下此写法都正确）
prev_tag=$(git tag --sort=-version:refname \
  | grep -vE '\-(rc|alpha|beta)' \
  | grep -vx "$version" \
  | head -n 1)

if [[ -z "$prev_tag" ]]; then
  log_range="HEAD"
  prev_tag="（首次发布）"
else
  log_range="${prev_tag}..HEAD"
fi

echo "=== 发布分支: ${branch} ==="
echo "=== 对比范围: ${prev_tag} → ${version} ==="
git log --oneline --no-decorate "$log_range"
echo ""
echo "--- commit subjects ---"
git log --format="%s" "$log_range"
```

确认内容必须包含：

- **发布分支**：`${branch}`（tag 将指向该分支当前 HEAD）
- **对比范围**：`${prev_tag} → ${version}`
- **新增 commit 清单**：上述 `git log` 输出

用户确认后，`prev_tag` 与 `log_range` 沿用至步骤 4，不再重新推导。

### 步骤 3：构建 multi-arch Docker 镜像并推送

```bash
make docker-image-multiarch VERSION="$version"
```

### 步骤 3.5：构建并上传发布二进制

构建 5 平台二进制（linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64）并展平命名；上传与 `gh release create` 一起在步骤 5 执行。

#### 3.5.1 构建内嵌 UI 与交叉编译

```bash
# 构建内嵌 UI（go:embed 需要）
make build-ui

# 安装交叉编译工具链（musl / arm-gcc / mingw / osxcross，需 sudo；已装则 apt 幂等、SDK 目录存在即跳过下载）
bash .github/workflows/scripts/install-cross-compilers.sh

# 交叉编译 5 平台（产物在 dist/<os>/<arch>/celer-route-http[.exe] + .sha256）
# build-executables.sh 内部用 `-X main.Version=v${VERSION}` 加 v 前缀，所以这里传入裸版本号
bash .github/workflows/scripts/build-executables.sh "${version#v}"
```

产物校验：`ls dist/*/*/celer-route-http*` 应含 5 平台二进制 + 5 个 `.sha256`。

#### 3.5.2 展平二进制命名

`gh release upload` 的资产名 = basename，linux/amd64 与 linux/arm64 的 `celer-route-http` 会重名覆盖。上传前按 `celer-route-http-<os>-<arch>[.exe]` 展平并重新生成校验和（与 `release-celer-route-http-finalize.sh` 的做法保持一致）：

```bash
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
while IFS= read -r -d '' asset; do
  rel="${asset#dist/}"
  plat="${rel%%/*}"
  arch_dir="$(dirname "$rel" | cut -d/ -f2)"
  base="$(basename "$asset")"
  stem="${base%.exe}"
  ext="${base#$stem}"
  cp "$asset" "$STAGE/celer-route-http-${plat}-${arch_dir}${ext}"
done < <(find dist -type f -name "celer-route-http*" ! -name "*.sha256" -print0)
(cd "$STAGE" && for f in celer-route-http-*; do [ -f "$f" ] && shasum -a 256 "$f" > "$f.sha256"; done)
[ "$(ls "$STAGE" | wc -l)" -eq 10 ] || { echo "ERROR: 展平产物数量异常（应为 5 二进制 + 5 校验和）"; exit 1; }
```

- 展平后的 `$STAGE` 目录在步骤 5 中用于 `gh release upload`
- `$STAGE` 由 EXIT trap 自动清理

### 步骤 4：分析变更并生成 CHANGELOG

对比范围 `${prev_tag} → ${version}`（`log_range`）已在步骤 2.5 确定并经用户确认，直接据此提取结构化变更：

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

#### 排除规则：内部目录变更不进入 CHANGELOG

以下目录的变更属于内部维护或开发工作流配置，**不体现在 release notes 中**（不进入任何分类，包括「其他」）：

| 目录 | 说明 |
|------|------|
| `website/static/recommended-providers/` | 免费套餐目录维护（刷新、增删供应商） |
| `.opencode/` | opencode 技能与配置 |
| `.pg/` | pg-skills 工作流配置 |

判定方法：
- 若 commit 的**全部**变更文件都落在上述目录内，整条 commit 从 CHANGELOG 中排除。
- 若 commit 同时改动产品代码（Go/UI）与上述目录，仍按正常规则提炼产品部分，忽略目录部分。
- 典型信号：`chore: 刷新免费套餐目录 ...`、`.pg`/`.opencode` 下的 bump/配置提交。

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
# 将 CHANGELOG 内容写入临时文件（CHANGELOG 前加 Installation 段）
cat > /tmp/changelog-${version}.md << 'CHANGELOG_EOF'
### Installation

#### Single Binary

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/pin-gou/celer-route/main/scripts/install.sh | bash
\`\`\`

Pre-built binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 and windows/amd64 are attached to this release, each with a \`.sha256\` checksum.

#### Docker

\`\`\`bash
docker run -p 8080:8080 ghcr.io/pin-gou/celer-route:v<version>
\`\`\`

{{CHANGELOG_CONTENT}}
CHANGELOG_EOF

# 创建 release（不含资产），再单独上传——二进制总量约 660MB，分离执行保证上传中断时只需重跑 upload
gh release create "$version" \
  --title "$version" \
  --notes-file /tmp/changelog-${version}.md

gh release upload "$version" "$STAGE"/celer-route-http-* --clobber

# 校验资产齐全（5 二进制 + 5 校验和）
COUNT=$(gh release view "$version" --json assets --jq '.assets | length')
if [[ "$COUNT" -ne 10 ]]; then
  echo "ERROR: 资产数量异常（期望 10，实际 $COUNT）——重跑 gh release upload 补传"
  exit 1
fi

rm -f /tmp/changelog-${version}.md
```

- 上传中断：release 已创建，重跑 `gh release upload "$version" "$STAGE"/celer-route-http-* --clobber` 即可，不要重复 `gh release create`（tag 已存在时会报错，属预期）
- `"$STAGE"/celer-route-http-*` 会被 shell 展开为全部二进制与校验和文件
- `$STAGE` 由步骤 3.5.2 的 EXIT trap 清理

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

# 步骤 2：确认当前发布分支（不切换分支）
branch="$(git rev-parse --abbrev-ref HEAD)"
[[ "$branch" != "HEAD" ]] || { echo "ERROR: detached HEAD"; exit 1; }
if git rev-parse --abbrev-ref --symbolic-full-name '@{u}' >/dev/null 2>&1; then
  git pull --ff-only
else
  echo "WARN: 当前分支 ${branch} 无上游，使用本地提交发布"
fi

# 步骤 2.5：二次确认（展示分支与新增 commit，等待用户明确确认后才继续）
prev_tag=$(git tag --sort=-version:refname \
  | grep -vE '\-(rc|alpha|beta)' \
  | grep -vx "$version" \
  | head -n 1)
if [[ -z "$prev_tag" ]]; then
  log_range="HEAD"; prev_tag="（首次发布）"
else
  log_range="${prev_tag}..HEAD"
fi
echo "=== 发布分支: ${branch} ==="
echo "=== 对比范围: ${prev_tag} → ${version} ==="
git log --oneline --no-decorate "$log_range"
# >>> agent 必须在此处向用户展示「分支 + 新增 commit 清单」并取得明确确认，未确认则终止 <<<

# 步骤 3：构建镜像
make docker-image-multiarch VERSION="$version"

# 步骤 3.5：构建发布二进制（内嵌 UI + 交叉编译 5 平台 + 展平命名）
make build-ui
bash .github/workflows/scripts/install-cross-compilers.sh
bash .github/workflows/scripts/build-executables.sh "${version#v}"
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
while IFS= read -r -d '' asset; do
  rel="${asset#dist/}"
  plat="${rel%%/*}"
  arch_dir="$(dirname "$rel" | cut -d/ -f2)"
  base="$(basename "$asset")"
  stem="${base%.exe}"
  ext="${base#$stem}"
  cp "$asset" "$STAGE/celer-route-http-${plat}-${arch_dir}${ext}"
done < <(find dist -type f -name "celer-route-http*" ! -name "*.sha256" -print0)
(cd "$STAGE" && for f in celer-route-http-*; do [ -f "$f" ] && shasum -a 256 "$f" > "$f.sha256"; done)
[ "$(ls "$STAGE" | wc -l)" -eq 10 ] || { echo "ERROR: 展平产物数量异常（应为 5 二进制 + 5 校验和）"; exit 1; }

# 步骤 4：分析变更（log_range / prev_tag 已在步骤 2.5 确定）
git log --oneline --no-decorate "$log_range"
echo ""

# agent 在此处手动分析并生成 CHANGELOG markdown
# ...

# 步骤 5：创建 Release（正文含 Installation 段；create 与 upload 分离）
git tag "$version"
git push origin "$version"
cat > /tmp/changelog-${version}.md << 'CHANGELOG_EOF'
### Installation

#### Single Binary

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/pin-gou/celer-route/main/scripts/install.sh | bash
\`\`\`

Pre-built binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 and windows/amd64 are attached to this release, each with a \`.sha256\` checksum.

#### Docker

\`\`\`bash
docker run -p 8080:8080 ghcr.io/pin-gou/celer-route:v<version>
\`\`\`

CHANGELOG_EOF
# agent 将生成的 CHANGELOG 内容追加入 /tmp/changelog-${version}.md（Installation 段之后）
gh release create "$version" --title "$version" --notes-file /tmp/changelog-${version}.md
gh release upload "$version" "$STAGE"/celer-route-http-* --clobber
COUNT=$(gh release view "$version" --json assets --jq '.assets | length')
[[ "$COUNT" -eq 10 ]] || { echo "ERROR: 资产数量异常（期望 10，实际 $COUNT）——重跑 gh release upload 补传"; exit 1; }
rm -f /tmp/changelog-${version}.md
```

## 报告格式

### 成功时

```
## 构建与发布完成

**版本：** {{version}}
**发布分支：** {{branch}}
**工作流：** build-and-publish

### 构建产物
- **Docker 镜像：** ghcr.io/pin-gou/celer-route:{{version}}（multi-arch: linux/amd64, linux/arm64）
- **GitHub Release：** https://github.com/{{owner}}/{{repo}}/releases/tag/{{version}}
- **二进制资产（上传至 Release）：**
  - celer-route-http-linux-amd64 / celer-route-http-linux-arm64
  - celer-route-http-darwin-amd64 / celer-route-http-darwin-arm64
  - celer-route-http-windows-amd64.exe
  - 以上均附 `.sha256`

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

- 发布基于当前所在分支（不强制 `main`），但必须处于具体分支（非 detached HEAD）
- 执行任何构建/发布动作前，必须向用户二次确认「发布分支 + 相较上一版本新增的 commit」，用户未确认不得继续
- tag 将指向当前分支 HEAD；若该提交尚未推送到远端，`git push origin "$version"` 会随 tag 一并推送该提交对象
- 本地 tag 创建后立即 `git push origin`，避免本地残留
- 工作区不干净时拒绝执行，防止误提交未完成的变更
- `gh release create` 使用 `--notes-file` 而非 `--notes`，避免 shell 转义问题
- 临时 changelog 文件在 release 创建后立即删除
- `gh release create` 与 `gh release upload` 分离执行：上传中断时只重跑 `gh release upload --clobber` 覆盖重传，不要重复 `gh release create`（tag 已存在时会报错，属预期）
- 上传后必须校验资产数量（`gh release view` 应为 10），不足时补传，不静默放行
- 交叉编译工具链为硬依赖：`install-cross-compilers.sh` 失败且无法手动补齐时终止，不发无二进制资产的 release

## 明确不做的事

- 不做代码审查或测试——发布前应已通过 CI
- 不修改 `AGENTS.md` 或版本文件——由发布流程独立管理
- 不推送 `latest` tag 以外的 Docker 标签——`make docker-image-multiarch` 已经处理
- 不创建 PR——发布流程直接从当前分支进行
- 不重复创建 `transports/vX` release（避免与 CI 路径冲突）——二进制只挂仓库级 `vX`