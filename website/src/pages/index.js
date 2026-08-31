import React from 'react';
import Layout from '@theme/Layout';
import useBaseUrl from '@docusaurus/useBaseUrl';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import '../css/custom.css';

const t = (locale) => (LOCALES[locale] || LOCALES['zh-CN']);

/* ===== 复用 SVG 图标 ===== */
const Icon = ({ size = 28, children }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    {children}
  </svg>
);

const icons = {
  multi: (
    <>
      <path d="M4 5a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" />
      <path d="M8 9h8M8 13h6M8 17h4" />
    </>
  ),
  grid: (
    <>
      <rect x="3" y="3" width="7" height="7" />
      <rect x="14" y="3" width="7" height="7" />
      <rect x="3" y="14" width="7" height="7" />
      <rect x="14" y="14" width="7" height="7" />
    </>
  ),
  route: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
    </>
  ),
  layers: (
    <>
      <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
    </>
  ),
  wrench: (
    <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />
  ),
  sdk: (
    <>
      <path d="M4 5a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" />
      <path d="M8 9h8M8 13h6M8 17h4" />
    </>
  ),
  stream: <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />,
  lock: (
    <>
      <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </>
  ),
  team: (
    <>
      <path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
      <circle cx="8.5" cy="7" r="4" />
      <polyline points="17 11 19 13 23 9" />
    </>
  ),
};

/* ===== Section 容器 ===== */
function Section({ id, title, lead, children }) {
  return (
    <section id={id} className="pg-section">
      <h1>{title}</h1>
      {lead && <p className="lead">{lead}</p>}
      {children}
    </section>
  );
}

/* ===== 架构图 SVG ===== */
function ArchSvg({ locale }) {
  const L = t(locale).arch;
  return (
    <svg viewBox="0 0 700 360" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <linearGradient id="g1" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stopColor="#047857" />
          <stop offset="100%" stopColor="#10b981" />
        </linearGradient>
        <linearGradient id="g2" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stopColor="#059669" />
          <stop offset="100%" stopColor="#34d399" />
        </linearGradient>
        <linearGradient id="g3" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stopColor="#0d9488" />
          <stop offset="100%" stopColor="#2dd4bf" />
        </linearGradient>
        <marker id="arr" viewBox="0 0 10 10" refX="5" refY="5" markerWidth="5" markerHeight="5" orient="auto">
          <path d="M0,0 L10,5 L0,10 z" fill="#a1a1aa" />
        </marker>
      </defs>

      <rect x="40" y="20" width="620" height="95" rx="10" fill="url(#g1)" opacity="0.12" />
      <rect x="40" y="20" width="620" height="95" rx="10" stroke="url(#g1)" strokeWidth="2" fill="none" />
      <text x="60" y="50" fontSize="14" fontWeight="700" fill="currentColor">{L.l1Title}</text>
      <text x="60" y="72" fontSize="12" fill="#71717b">{L.l1Desc}</text>
      <rect x="60" y="85" width="140" height="24" rx="6" fill="url(#g1)" opacity="0.25" />
      <text x="72" y="102" fontSize="11" fontWeight="600" fill="currentColor">{L.l1Badge}</text>

      <line x1="350" y1="115" x2="350" y2="132" stroke="#a1a1aa" strokeWidth="1.5" markerEnd="url(#arr)" />

      <rect x="40" y="138" width="620" height="110" rx="10" fill="url(#g2)" opacity="0.12" />
      <rect x="40" y="138" width="620" height="110" rx="10" stroke="url(#g2)" strokeWidth="2" fill="none" />
      <text x="60" y="168" fontSize="14" fontWeight="700" fill="currentColor">{L.l2Title}</text>
      <text x="60" y="190" fontSize="12" fill="#71717b">{L.l2Desc}</text>
      <rect x="60" y="205" width="96" height="28" rx="6" fill="url(#g2)" opacity="0.25" />
      <text x="72" y="224" fontSize="11" fontWeight="600" fill="currentColor">{L.l2Plugin}</text>
      <rect x="168" y="205" width="96" height="28" rx="6" fill="url(#g2)" opacity="0.25" />
      <text x="180" y="224" fontSize="11" fontWeight="600" fill="currentColor">{L.l2Key}</text>
      <rect x="276" y="205" width="96" height="28" rx="6" fill="url(#g2)" opacity="0.25" />
      <text x="288" y="224" fontSize="11" fontWeight="600" fill="currentColor">{L.l2Stream}</text>
      <rect x="384" y="205" width="96" height="28" rx="6" fill="url(#g2)" opacity="0.25" />
      <text x="396" y="224" fontSize="11" fontWeight="600" fill="currentColor">{L.l2Route}</text>

      <line x1="350" y1="248" x2="350" y2="265" stroke="#a1a1aa" strokeWidth="1.5" markerEnd="url(#arr)" />

      <rect x="40" y="270" width="620" height="70" rx="10" fill="url(#g3)" opacity="0.12" />
      <rect x="40" y="270" width="620" height="70" rx="10" stroke="url(#g3)" strokeWidth="2" fill="none" />
      <text x="60" y="300" fontSize="14" fontWeight="700" fill="currentColor">{L.l3Title}</text>
      <text x="60" y="322" fontSize="12" fill="#71717b">{L.l3Desc}</text>
      <rect x="60" y="325" width="80" height="22" rx="6" fill="url(#g3)" opacity="0.25" />
      <text x="70" y="340" fontSize="10" fontWeight="600" fill="currentColor">openai</text>
      <rect x="148" y="325" width="80" height="22" rx="6" fill="url(#g3)" opacity="0.25" />
      <text x="158" y="340" fontSize="10" fontWeight="600" fill="currentColor">anthropic</text>
      <rect x="236" y="325" width="80" height="22" rx="6" fill="url(#g3)" opacity="0.25" />
      <text x="246" y="340" fontSize="10" fontWeight="600" fill="currentColor">bedrock</text>
      <rect x="324" y="325" width="80" height="22" rx="6" fill="url(#g3)" opacity="0.25" />
      <text x="334" y="340" fontSize="10" fontWeight="600" fill="currentColor">gemini</text>
      <rect x="412" y="325" width="80" height="22" rx="6" fill="url(#g3)" opacity="0.25" />
      <text x="422" y="340" fontSize="10" fontWeight="600" fill="currentColor">cohere</text>
      <rect x="500" y="325" width="80" height="22" rx="6" fill="url(#g3)" opacity="0.25" />
      <text x="510" y="340" fontSize="10" fontWeight="600" fill="currentColor">+ 25+</text>
    </svg>
  );
}

/* ===== Feature Card ===== */
function FeatureCard({ iconKey, title, children }) {
  return (
    <div className="pg-feature-card">
      <div className="icon">
        <Icon size={28}>{icons[iconKey]}</Icon>
      </div>
      <h3>{title}</h3>
      <p>{children}</p>
    </div>
  );
}

/* ===== Step Card ===== */
function StepCard({ num, title, children }) {
  return (
    <div className="pg-step-card">
      <h3>
        <span className="pg-step-num">{num}</span>
        {title}
      </h3>
      {children}
    </div>
  );
}

/* ===== 双语文案 ===== */
const LOCALES = {
  'zh-CN': {
    metaTitle: 'celer-route · 高性能个人 LLM 网关',
    metaDesc: 'celer-route 是一个高性能的个人 LLM 接口网关，将 20+ 主流 LLM 提供商统一为 OpenAI 兼容 API',
    tagline: '高性能个人 LLM 网关 · 一套 API 串联 30+ 模型',
    cta: '快速开始 →',
    sec1: '1. 这是什么',
    sec1Lead: '',
    sec1Intro: 'celer-route 是一个高性能的个人 LLM 接口网关，将 20+ 主流 LLM 提供商统一为 OpenAI 兼容 API。你只需学会一套 API，就能在 30+ 模型间自由切换、组合、路由。',
    features1: [
      ['multi', '多提供商统一接入', '一套 API 访问 OpenAI、Anthropic、AWS Bedrock、Google Gemini、Azure、Cohere、Mistral、Ollama、Groq、DeepSeek、Fireworks 等 30+ 提供商'],
      ['grid', 'Web 管理界面', '仪表盘实时监控请求量 / 延迟 / Token 用量，提供请求日志查询与详情追溯，支持中文 / 英文界面'],
      ['route', '智能路由', '路由规则与权重路由，支持自动故障转移和跨 API Key 负载均衡，一台网关串联所有模型'],
      ['layers', '插件系统', '内置提供商冷却、语义缓存、请求日志、模拟响应、提示词管理等插件，按需启停，扩展灵活'],
      ['wrench', '灵活配置', '客户端设置、兼容模式、缓存策略、安全配置、API Key 管理，全部通过 Web UI 或 API 完成'],
      ['grid', '双数据库后端', '同一镜像同时支持 SQLite（单机零依赖）与 PostgreSQL（生产 HA），部署时通过 config.json 切换'],
    ],
    sec2: '2. 架构全景',
    sec2Lead: '所有客户端走同一网关；网关只对提供商做适配差异；中间层用插件串接治理、缓存、日志。',
    arch: {
      l1Title: 'L1 · 客户端层',
      l1Desc: 'OpenAI SDK / Anthropic SDK / Bedrock SDK / GenAI SDK / LangChain / LiteLLM / 标准 HTTP 客户端',
      l1Badge: '统一 API 入口',
      l2Title: 'L2 · 网关引擎层',
      l2Desc: 'celer-route Core · 请求排队 · 推理路由 · 故障转移 · 中间件管道',
      l2Plugin: '插件系统',
      l2Key: 'Key 选择器',
      l2Stream: '流式处理',
      l2Route: '路由规则',
      l3Title: 'L3 · 提供商适配层',
      l3Desc: '30+ 适配器 · 统一接口 · 各提供商独立 worker 池',
    },
    archCore: '核心原则',
    archCoreDesc: '提供商隔离——每个提供商独立 worker 池和队列，一个提供商故障不会级联到其他。通道基异步，Go 通道 + 原子标志，零锁争用。',
    sec3: '3. 快速开始',
    step1: '获取代码',
    step2: '启动服务',
    dockerRec: '推荐',
    docker: 'Docker（推荐）',
    srcBuild: '源码构建',
    srcAlt: '备选',
    step3: '首次调用',
    step3Desc: '浏览器打开 ',
    step3Desc2: ' 进入 Web 管理界面，配置 Provider Key 后即可调用 API：',
    tip: '提示',
    tipDesc: '首次启动后先到 Web UI 配置你的 Provider API Key，再调用路由。',
    sec4: '4. 核心能力深潜',
    features4: [
      ['sdk', '多 SDK 兼容', 'OpenAI / Anthropic / Bedrock / GenAI / LangChain / Cohere / LiteLLM / PydanticAI / Cursor 全兼容，drop-in 替换，无需改一行客户端代码'],
      ['stream', '流式响应', 'SSE 流式全支持，NewIdleTimeoutReader 按 chunk 超时而非整请求超时，长对话不卡死；框架层积累器支持流中 pause / resume'],
      ['layers', '插件流水线', 'Pre-Hook / Post-Hook 对称管道，注册顺序执行，错误可恢复、可短路。LLM / MCP / HTTP Transport / Observability 四种插件接口'],
      ['route', '智能路由', '按权重 / CEL 规则选择 provider，自动故障转移，多 API Key 负载均衡，回退时完整重跑插件管道'],
      ['lock', '虚拟密钥治理', '虚拟 Key / 团队 / 客户 / 预算 / 限流 / RBAC 全链路治理，路由规则与复杂性路由，每请求鉴权审计'],
      ['team', '语义缓存', '基于向量存储（Weaviate / Qdrant / Redis / Pinecone）的语义命中复用，节约调用成本与延迟'],
    ],
    sec5: '5. 仓库结构',
    dirs: [
      ['core/', 'Go 核心引擎 — 请求排队、推理路由、故障转移、30+ 提供商实现', 'bifrost.go / inference.go / providers/'],
      ['framework/', '数据持久化、流式处理、加密、路由规则、Webhook、向量存储', 'streaming/ / configstore/ / logstore/ / vectorstore/'],
      ['transports/', 'HTTP 网关 — 92 个 endpoint，SDK 兼容层，中间件，Server 生命周期', 'handlers/ / integrations/ / server/'],
      ['ui/', 'React + Vite 管理界面，23 个 workspace 页面，i18n 支持中/英文', 'app/workspace/ / components/ui/ / locales/'],
      ['plugins/', '9 个 Go 插件：governance / telemetry / logging / semanticcache / otel / mocker / prompts / compat / providercooldown', 'governance/ / telemetry/ / logging/ / semanticcache/'],
      ['cli/', '命令行工具 — 配置管理、Secrets、MCP 管理、运行时管理、更新', 'internal/app/ / internal/config/ / internal/mcp/'],
    ],
    sec6: '6. 构建与开发',
    cmdHeader: ['命令', '功能'],
    cmds: [
      ['make build', '构建 celer-route-http 二进制'],
      ['make dev', '本地开发（UI + API + air 热重载）'],
      ['make fmt', '代码格式化'],
      ['make lint', '代码 lint'],
      ['make test-core PROVIDER=openai', '单提供商 LLM 集成测试'],
      ['make test-core PROVIDER=openai TESTCASE=TestSimpleChat', '指定测试用例'],
      ['make test-mcp', 'MCP / Agent 测试（mock 基，无需 live API）'],
      ['make test-framework', '框架层测试（需 docker compose up）'],
      ['make test-plugins', '插件测试'],
      ['make test-governance', 'Governance 插件测试'],
      ['make test-integrations-py', 'Python SDK 集成测试'],
      ['make test-integrations-ts', 'TypeScript SDK 集成测试'],
      ['make run-e2e FLOW=providers', 'Playwright E2E 测试'],
    ],
    prereqTitle: '前置要求',
    prereqDesc: 'Go 1.26.1+ · 多模块 Go workspace · 部分测试需要 docker compose 启动 postgres / weaviate / qdrant / redis 等依赖服务',
    footerApache: 'Apache 2.0 · celer-route · 基于 ',
    bifrost: 'Bifrost',
    footerBuilt: ' 构建',
  },
  en: {
    metaTitle: 'celer-route · High-Performance Personal LLM Gateway',
    metaDesc: 'celer-route is a high-performance personal LLM gateway that unifies 20+ mainstream LLM providers behind one OpenAI-compatible API.',
    tagline: 'High-performance personal LLM gateway · one API, 30+ models',
    cta: 'Get Started →',
    sec1: '1. What is it',
    sec1Lead: '',
    sec1Intro: 'celer-route is a high-performance personal LLM gateway that unifies 20+ mainstream LLM providers behind one OpenAI-compatible API. Learn one API and switch, compose, and route freely across 30+ models.',
    features1: [
      ['multi', 'Multi-provider unified access', 'One API to reach OpenAI, Anthropic, AWS Bedrock, Google Gemini, Azure, Cohere, Mistral, Ollama, Groq, DeepSeek, Fireworks and 20+ more providers'],
      ['grid', 'Web admin dashboard', 'Real-time monitoring of request volume, latency, and token usage; full request log query and traceback; bilingual UI (Chinese / English)'],
      ['route', 'Intelligent routing', 'Rule-based and weighted routing with automatic failover and cross-API-key load balancing — one gateway for all models'],
      ['layers', 'Plugin system', 'Built-in plugins for provider cooldown, semantic cache, request logging, mock responses, prompt management — enable what you need'],
      ['wrench', 'Flexible configuration', 'Client settings, compatibility modes, cache policies, security, API keys — all via the Web UI or API'],
      ['grid', 'Dual database backend', 'Same image supports SQLite (zero-dep single-node) and PostgreSQL (production HA); switch via config.json at deploy time'],
    ],
    sec2: '2. Architecture at a Glance',
    sec2Lead: 'All clients hit one gateway; the gateway adapts to provider differences; the middle layer is wired by plugins for governance, cache, and logging.',
    arch: {
      l1Title: 'L1 · Client Layer',
      l1Desc: 'OpenAI SDK / Anthropic SDK / Bedrock SDK / GenAI SDK / LangChain / LiteLLM / standard HTTP clients',
      l1Badge: 'Unified API entry',
      l2Title: 'L2 · Gateway Engine',
      l2Desc: 'celer-route Core · request queue · inference routing · failover · middleware pipeline',
      l2Plugin: 'Plugins',
      l2Key: 'Key selector',
      l2Stream: 'Streaming',
      l2Route: 'Routing rules',
      l3Title: 'L3 · Provider Adapters',
      l3Desc: '30+ adapters · unified interface · per-provider isolated worker pools',
    },
    archCore: 'Core principle',
    archCoreDesc: 'Provider isolation — each provider has its own worker pool and queue, so one provider going down never cascades to others. Channel-based async via Go channels + atomic flags, zero lock contention.',
    sec3: '3. Quick Start',
    step1: 'Get the code',
    step2: 'Launch the service',
    dockerRec: 'Recommended',
    docker: 'Docker (recommended)',
    srcBuild: 'Build from source',
    srcAlt: 'Alternative',
    step3: 'First call',
    step3Desc: 'Open ',
    step3Desc2: ' in your browser for the Web admin, configure a provider key, and you are ready to call the API:',
    tip: 'Tip',
    tipDesc: 'On first launch, configure your provider API keys in the Web UI before invoking the gateway.',
    sec4: '4. Capabilities in Depth',
    features4: [
      ['sdk', 'Multi-SDK compatibility', 'Drop-in compatible with OpenAI / Anthropic / Bedrock / GenAI / LangChain / Cohere / LiteLLM / PydanticAI / Cursor — no client-side code changes'],
      ['stream', 'Streaming responses', 'Full SSE streaming support; NewIdleTimeoutReader enforces per-chunk idle timeout rather than whole-response, so long sessions never stall; the framework accumulator supports pause/resume mid-stream'],
      ['layers', 'Plugin pipeline', 'Symmetric Pre-Hook / Post-Hook pipelines in registration order, with recoverable errors and short-circuit support. Four plugin interfaces: LLM / MCP / HTTP Transport / Observability'],
      ['route', 'Intelligent routing', 'Weighted / CEL-rule provider selection, automatic failover, multi-API-key load balancing; fallback re-runs the full plugin pipeline'],
      ['lock', 'Virtual key governance', 'Virtual keys / teams / customers / budgets / rate limits / RBAC end-to-end; routing rules and complexity routing; per-request auth audit'],
      ['team', 'Semantic cache', 'Vector-store-backed (Weaviate / Qdrant / Redis / Pinecone) semantic hit reuse to cut cost and latency'],
    ],
    sec5: '5. Repository Layout',
    dirs: [
      ['core/', 'Go core engine — request queue, inference routing, failover, 30+ provider implementations', 'bifrost.go / inference.go / providers/'],
      ['framework/', 'Data persistence, streaming, encryption, routing rules, webhooks, vector stores', 'streaming/ / configstore/ / logstore/ / vectorstore/'],
      ['transports/', 'HTTP gateway — 92 endpoints, SDK compatibility layer, middleware, server lifecycle', 'handlers/ / integrations/ / server/'],
      ['ui/', 'React + Vite admin UI, 23 workspace pages, bilingual i18n (zh-CN / en)', 'app/workspace/ / components/ui/ / locales/'],
      ['plugins/', '9 Go plugins: governance / telemetry / logging / semanticcache / otel / mocker / prompts / compat / providercooldown', 'governance/ / telemetry/ / logging/ / semanticcache/'],
      ['cli/', 'Command-line tool — config management, secrets, MCP management, runtime, updates', 'internal/app/ / internal/config/ / internal/mcp/'],
    ],
    sec6: '6. Build & Development',
    cmdHeader: ['Command', 'Purpose'],
    cmds: [
      ['make build', 'Build the celer-route-http binary'],
      ['make dev', 'Local dev (UI + API + air hot reload)'],
      ['make fmt', 'Format code'],
      ['make lint', 'Lint code'],
      ['make test-core PROVIDER=openai', 'Single-provider LLM integration tests'],
      ['make test-core PROVIDER=openai TESTCASE=TestSimpleChat', 'Specific test case'],
      ['make test-mcp', 'MCP / Agent tests (mock-based, no live API)'],
      ['make test-framework', 'Framework tests (requires docker compose up)'],
      ['make test-plugins', 'Plugin tests'],
      ['make test-governance', 'Governance plugin tests'],
      ['make test-integrations-py', 'Python SDK integration tests'],
      ['make test-integrations-ts', 'TypeScript SDK integration tests'],
      ['make run-e2e FLOW=providers', 'Playwright E2E tests'],
    ],
    prereqTitle: 'Prerequisites',
    prereqDesc: 'Go 1.26.1+ · multi-module Go workspace · some tests require docker compose for postgres / weaviate / qdrant / redis and other backing services',
    footerApache: 'Apache 2.0 · celer-route · built on ',
    bifrost: 'Bifrost',
    footerBuilt: '',
  },
};

/* ===== Home Component ===== */
export default function Home() {
  const { i18n } = useDocusaurusContext();
  const locale = i18n.currentLocale;
  const L = t(locale);

  return (
    <Layout title={L.metaTitle} description={L.metaDesc}>
      <div className="pg-home">
        {/* Hero */}
        <header className="pg-hero">
          <h1>celer-route</h1>
          <p className="tagline">{L.tagline}</p>
          <div className="cta">
            <a className="pg-btn pg-btn-primary" href={useBaseUrl('deployment/sqlite')}>
              {L.cta}
            </a>
            <a
              className="pg-btn pg-btn-secondary"
              href="https://github.com/pin-gou/celer-route"
              target="_blank"
              rel="noopener noreferrer"
            >
              GitHub
            </a>
          </div>
        </header>

        {/* Section 1 */}
        <Section id="sec1" title={L.sec1}>
          <p>{L.sec1Intro}</p>
          <div className="pg-feature-grid">
            {L.features1.map(([k, title, body], i) => (
              <FeatureCard key={`f1-${i}`} iconKey={k} title={title}>{body}</FeatureCard>
            ))}
          </div>
        </Section>

        {/* Section 2 */}
        <Section id="sec2" title={L.sec2} lead={L.sec2Lead}>
          <div className="pg-svg-container">
            <ArchSvg locale={locale} />
          </div>
          <div className="pg-info-card">
            <p>
              <strong>{L.archCore}</strong>: {L.archCoreDesc}
            </p>
          </div>
        </Section>

        {/* Section 3 */}
        <Section id="sec3" title={L.sec3}>
          <StepCard num="1" title={L.step1}>
            <pre>
              <code>
                git clone https://github.com/pin-gou/celer-route.git{'\n'}
                cd celer-route
              </code>
            </pre>
          </StepCard>

          <StepCard num="2" title={L.step2}>
            <div className="pg-info-card">
              <p>
                <strong>Docker</strong>
                <span className="pg-tag pg-tag-green">{L.dockerRec}</span>
              </p>
            </div>
            <pre>
              <code>
                docker pull ghcr.io/pin-gou/celer-route:latest{'\n'}
                docker run -d \{'\n'}
                {'  '}--name celer-route \{'\n'}
                {'  '}--restart unless-stopped \{'\n'}
                {'  '}--user 0:0 \{'\n'}
                {'  '}-p 8080:8080 \{'\n'}
                {'  '}-v ~/celer-route-data:/app/data \{'\n'}
                {'  '}ghcr.io/pin-gou/celer-route:latest
              </code>
            </pre>
            <div className="pg-info-card">
              <p>
                <strong>{L.srcBuild}</strong>
                <span className="pg-tag pg-tag-blue">{L.srcAlt}</span>
              </p>
            </div>
            <pre>
              <code>
                # Go 1.26.1+ required{'\n'}
                make build     # build binary{'\n'}
                make dev       # start dev environment (with hot reload)
              </code>
            </pre>
          </StepCard>

          <StepCard num="3" title={L.step3}>
            <p>
              {L.step3Desc}<a href="http://localhost:8080">http://localhost:8080</a>{L.step3Desc2}
            </p>
            <pre>
              <code>
                curl http://localhost:8080/v1/chat/completions \{'\n'}
                {'  '}-H "Content-Type: application/json" \{'\n'}
                {'  '}-d '&#123;{'\n'}
                {'    '}"model": "openai/gpt-4o-mini",{'\n'}
                {'    '}"messages": [&#123;"role": "user", "content": "hello"&#125;]{'\n'}
                {'  '}&#125;'
              </code>
            </pre>
            <div className="pg-info-card">
              <p>
                💡 <strong>{L.tip}</strong>: {L.tipDesc}
              </p>
            </div>
          </StepCard>
        </Section>

        {/* Section 4 */}
        <Section id="sec4" title={L.sec4}>
          <div className="pg-feature-grid">
            {L.features4.map(([k, title, body], i) => (
              <FeatureCard key={`f4-${i}`} iconKey={k} title={title}>{body}</FeatureCard>
            ))}
          </div>
        </Section>

        {/* Section 5 */}
        <Section id="sec5" title={L.sec5}>
          <div className="pg-dir-grid">
            {L.dirs.map(([name, desc, path], i) => (
              <div className="pg-dir-card" key={`dir-${i}`}>
                <div className="pg-dir-name">{name}</div>
                <p>{desc}</p>
                <div className="pg-dir-path">{path}</div>
              </div>
            ))}
          </div>
        </Section>

        {/* Section 6 */}
        <Section id="sec6" title={L.sec6}>
          <table className="pg-cmd-table">
            <thead>
              <tr>
                <th>{L.cmdHeader[0]}</th>
                <th>{L.cmdHeader[1]}</th>
              </tr>
            </thead>
            <tbody>
              {L.cmds.map(([cmd, desc], i) => (
                <tr key={`cmd-${i}`}>
                  <td><code>{cmd}</code></td>
                  <td>{desc}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="pg-info-card">
            <p>
              <strong>{L.prereqTitle}</strong>: {L.prereqDesc}
            </p>
          </div>
        </Section>

        {/* Footer */}
        <div className="pg-footer">
          <p>
            {L.footerApache}
            <a href="https://github.com/maximhq/bifrost" target="_blank" rel="noopener noreferrer">
              {L.bifrost}
            </a>
            {L.footerBuilt}
          </p>
        </div>
      </div>
    </Layout>
  );
}
