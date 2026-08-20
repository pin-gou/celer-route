---
name: build-and-publish
description: 构建 multi-arch Docker 镜像并推送到 GHCR，然后自动分析 Git 变更生成 CHANGELOG，最后在 GitHub 创建 Release
trigger: slash
---

# /build-and-publish <version>

version: $1

此命令被触发时，执行以下步骤：

1. 使用 Skill tool 加载 `build-and-publish` skill
2. 解析 `$1` 为 version 参数（格式 vX.Y.Z）
3. 按 SKILL 定义的核心流程执行：校验 → 切 main 分支 → git pull → 构建镜像 → 生成 CHANGELOG → 创建 GitHub Release
4. 输出发布报告

**示例**：
```
/build-and-publish v1.5.0
/build-and-publish v2.0.0-rc1
```