import type { ProviderName } from "@/lib/constants/logs";

export type Capability = "chat" | "embed" | "vision" | "speech" | "tools" | "image" | "video" | "transcription" | "rerank";

export const CAPABILITY_ORDER: Capability[] = ["chat", "embed", "vision", "tools", "speech", "image", "video", "transcription", "rerank"];

/**
 * Static capability matrix for built-in providers.
 *
 * Source of truth lives here for built-ins because the provider list API does
 * not surface per-provider capability metadata. Custom providers derive their
 * capabilities from `custom_provider_config.allowed_requests` at runtime.
 */
export const ProviderCapabilities: Record<ProviderName, Capability[]> = {
	openai: ["chat", "embed", "vision", "tools", "speech", "image", "transcription", "rerank"],
	anthropic: ["chat", "vision", "tools"],
	azure: ["chat", "embed", "vision", "tools", "speech", "image", "transcription"],
	bedrock: ["chat", "embed", "vision", "tools", "speech", "image"],
	bedrock_mantle: ["chat", "embed", "vision"],
	cerebras: ["chat"],
	cohere: ["chat", "embed", "rerank"],
	alibaba: ["chat", "embed", "vision", "tools", "image", "speech", "rerank"],
	minimax: ["chat", "embed", "vision", "tools", "speech", "video"],
	deepseek: ["chat", "tools"],
	gemini: ["chat", "embed", "vision", "tools", "speech", "image", "video", "transcription"],
	groq: ["chat", "tools", "speech", "transcription", "vision"],
	huggingface: ["chat", "embed", "image", "speech", "transcription"],
	mistral: ["chat", "embed", "vision", "tools"],
	moonshot: ["chat", "vision", "tools"],
	ollama: ["chat", "embed", "vision"],
	opencode: ["chat"],
	"opencode-go": ["chat"],
	"opencode-zen": ["chat"],
	openrouter: ["chat", "embed", "vision", "tools", "image", "speech"],
	parasail: ["chat", "embed"],
	elevenlabs: ["speech"],
	perplexity: ["chat"],
	sgl: ["chat", "embed"],
	siliconflow: ["chat", "embed", "vision", "image"],
	vertex: ["chat", "embed", "vision", "tools", "speech", "image", "video"],
	nebius: ["chat", "embed"],
	volcengine: ["chat", "embed", "vision", "tools", "image", "speech"],
	xai: ["chat", "vision", "tools"],
	replicate: ["chat", "image", "video", "speech"],
	vllm: ["chat", "embed"],
	runway: ["video"],
	runware: ["image"],
	fireworks: ["chat", "embed", "image"],
	sarvam: ["chat", "speech", "transcription"],
	wafer: ["chat"],
	zhipu: ["chat", "embed", "vision", "tools", "image", "speech"],
	tencent: ["chat", "embed", "vision", "tools", "image", "speech"],
	baidu: ["chat", "embed", "vision", "tools", "image", "speech"],
};

/**
 * Providers commonly used in the China region. Surfaced as the "Recommended"
 * row in the picker to give new users a fast on-ramp without making them
 * scroll a 39-item catalog.
 */
export const RECOMMENDED_PROVIDERS: ProviderName[] = ["deepseek", "moonshot", "alibaba", "zhipu", "volcengine", "openai"];