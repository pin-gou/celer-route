# Design Drift Log

## Drift #1
- **发现阶段**: scenario-fix
- **场景**: S-i18n-login-via-api-then-set-zh-CN
- **位置**: scenario-scr.yaml:16 登录 URL + fixature_config.json auth_config.is_enabled
- **原因**: scenario 使用 /api/auth/login 但实际后端路由为 /api/session/login；fixture 中 auth_config.is_enabled=false 导致即使正确 URL 也返回 403
- **决策**: FIXED
- **修复**: scenario-scr.yaml:16 URL 改为 /api/session/login；fixature_config.json 启用 auth（is_enabled=true, admin/bifrost123 bcrypt hash）；design.md 修正 vite-plugin-i18next-typescript（不存在于 npm registry）引用和 auth 描述

