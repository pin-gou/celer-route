# celer-route

> 基于 Bifrost 裁剪的个人 LLM 接口网关，统一主流 LLM 提供商为 OpenAI 兼容 API。你只需学会一套 API，就能在不同提供商/模型间自由切换、组合、路由。

## 主要功能

- **多提供商统一接入** — 一套 API 访问 OpenAI、Anthropic、AWS Bedrock、Google Gemini、Azure、Cohere、Mistral、Ollama、Groq、DeepSeek、Fireworks 等主流提供商，每个提供商独立 worker 池与队列，单点故障不级联
- **Web 管理界面** — 仪表盘实时监控请求量 / 延迟 / Token 用量，提供请求日志查询与详情追溯，支持中文 / 英文界面
- **智能路由** — 路由规则与权重路由，支持自动故障转移和跨 API Key 负载均衡，一台网关串联所有模型
- **插件系统** — 提供商冷却、语义缓存、请求日志、模拟响应、提示词管理、RTK 上下文压缩等插件按需启停，扩展灵活
- **多 SDK 兼容** — OpenAI / Anthropic / Bedrock / GenAI / LangChain / Cohere / LiteLLM / PydanticAI / Cursor 全兼容，drop-in 替换，无需改一行客户端代码
- **流式响应** — SSE 流式全支持，按 chunk 空闲超时而非整请求超时，长对话不卡死；框架层积累器支持流中 pause / resume
- **虚拟密钥治理** — 虚拟 Key / 团队 / 客户 / 预算 / 限流 / RBAC 全链路治理，每请求鉴权审计
- **双数据库后端** — 同一镜像同时支持 SQLite（单机零依赖）与 PostgreSQL（生产 HA），部署时通过 config.json 切换

## Quick Start

```bash
# 拉取并启动服务
docker pull ghcr.io/pin-gou/celer-route:latest
docker run -d \
  --name celer-route \
  --restart unless-stopped \
  --user 0:0 \
  -p 8080:8080 \
  -v ~/celer-route-data:/app/data \
  ghcr.io/pin-gou/celer-route:latest

# 打开 Web 界面，按引导完成配置
open http://localhost:8080

# 调用 API
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/gpt-4o-mini",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

> 提示：首次启动后先到 Web UI 配置你的 Provider API Key，再调用路由。

## License

Apache 2.0
