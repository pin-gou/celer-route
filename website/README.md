# pg-gateway 文档站

本站基于 [Docusaurus 3](https://docusaurus.io/) 构建，部署到 GitHub Pages：
<https://pin-gou.github.io/pg-gateway/>

## 本地开发

```bash
npm install
npm run start    # http://localhost:3000/pg-gateway/
```

## 构建

```bash
npm run build    # 产物在 ./build/
npm run serve    # 本地预览生产构建
```

## 部署

push 到 `master` 分支会触发 GitHub Actions 自动构建并部署到 `gh-pages` 分支。
详见仓库根目录 `.github/workflows/deploy-docs.yml`。

## 内容结构

```
docs/
├── intro.mdx                    # 侧边栏首页入口
├── deployment/
│   ├── sqlite.mdx              # SQLite 部署指南
│   └── postgres.mdx            # PostgreSQL 部署指南
├── features/
│   ├── data-storage.mdx        # 数据存储后端总览
│   ├── dashboard-auth.mdx
│   ├── i18n.mdx
│   └── provider-cooldown.mdx
├── providers/supported-providers/
│   └── ... (7 个 provider)
└── reference/
    └── cooldown-logging.md
```

首页（首页 React 组件，复刻原 `docs/index.html`）位于 `src/pages/index.jsx`。

## 新增文档

1. 在 `docs/` 对应分类下新建 `.md` 或 `.mdx`
2. 如需加入侧边栏，编辑 `sidebars.js`
3. PR 合并后 Actions 自动部署