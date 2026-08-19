# pg-gateway

> 基于 Bifrost 裁剪的个人 LLM 接口网关，统一 20+ LLM 提供商为 OpenAI 兼容 API。

## 主要功能

- **多提供商统一接入** — 支持 OpenAI、Anthropic、AWS Bedrock、Google Gemini/Vertex、Azure、Cohere、Mistral、Ollama、Groq、DeepSeek、Fireworks 等 30+ 提供商，一套 API 访问所有模型
- **Web 管理界面** — 内置仪表盘实时监控请求量/延迟/Token 用量，提供请求日志查询与详情追溯
- **智能路由** — 路由规则与复杂性路由，支持自动故障转移和跨 API Key 负载均衡
- **插件系统** — 内置提供商冷却、语义缓存、请求日志、模拟响应、提示词管理等插件，可按需启停
- **灵活配置** — 客户端设置、兼容模式、缓存策略、安全配置、API Key 管理、性能调优，全部通过 Web UI 或 API 完成

## Quick Start

> Docker 镜像即将发布，敬请期待。

```bash
# 启动服务（即将支持）
docker run  -d \
  --name pg-gateway \
  --restart unless-stopped \
  --user 0:0 \
  -p 8080:8080 \
  -v ~/pg-gateway-data:/app/data \
  ghcr.io/pin-gou/pg-gateway:latest

# 打开 Web 界面
open http://localhost:8080

# 调用 API
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/gpt-4o-mini",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

## 仓库结构

```
pg-gateway/
├── core/           # 核心引擎与提供商实现
├── framework/      # 数据持久化与流式处理
├── transports/     # HTTP 网关
├── ui/             # Web 管理界面
├── plugins/        # 插件系统
└── cli/            # 命令行工具
```

## 构建

```bash
make build          # 构建二进制
make dev            # 启动开发环境（含热重载）
```

## License

Apache 2.0