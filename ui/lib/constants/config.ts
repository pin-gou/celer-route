import { BaseProvider, ConcurrencyAndBufferSize, NetworkConfig } from "@/lib/types/config";
import { ProviderName } from "./logs";

/**
 * Parse a trial expiry string with strict validation.
 * Accepts either a bare date (YYYY-MM-DD) or a full RFC3339 timestamp
 * (YYYY-MM-DDTHH:mm:ssZ) — the Docker build injects the latter, since the Go
 * side needs an RFC3339 value. Only the calendar date is significant for the
 * banner, so any time component is ignored and the date is taken at local
 * midnight. Returns null if the string is empty, malformed, or represents an
 * invalid date.
 */
function parseTrialExpiry(dateStr: string | undefined): Date | null {
	if (!dateStr || !dateStr.trim()) return null;

	// Accept YYYY-MM-DD optionally followed by a time component (e.g. the
	// RFC3339 "T00:00:00Z" suffix), and capture just the date part.
	const dateRegex = /^(\d{4})-(\d{2})-(\d{2})(?:[T ].*)?$/;
	const match = dateRegex.exec(dateStr.trim());
	if (!match) return null;

	const [, yearStr, monthStr, dayStr] = match;
	const year = Number(yearStr);
	const month = Number(monthStr);
	const day = Number(dayStr);
	const date = new Date(year, month - 1, day);

	// Validate the date components match (catches invalid dates like 2024-02-30)
	if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) {
		return null;
	}

	return date;
}

// Model placeholders based on provider type
export const ModelPlaceholders = {
	default: "e.g. gpt-4, gpt-3.5-turbo. Leave blank for all models.",
	alibaba: "e.g. qwen-plus, qwen-turbo, qwen-max",
	anthropic: "e.g. claude-3-haiku, claude-2.1",
	azure: "e.g. gpt-4, gpt-35-turbo (must match alias keys)",
	bedrock: "e.g. claude-v2, titan-text-express-v1, ai21-j2-mid",
	bedrock_mantle: "e.g. anthropic.claude-opus-4-8, openai.gpt-oss-120b",
	cerebras: "e.g. cerebras-2, cerebras-2-vision",
	cohere: "e.g. command-r, command-r-plus",
	deepseek: "e.g. deepseek-v4-flash, deepseek-v4-pro",
	gemini: "e.g. gemini-1.5-pro, gemini-1.5-flash",
	groq: "e.g. llama3-70b-8192, mixtral-8x7b-32768",
	huggingface: "e.g. sambanova/meta-llama/Llama-3.1-8B-Instruct, nebius/Qwen/Qwen3-Embedding-8B",
	minimax: "e.g. minimax-text-01, minimax-abab5.5s",
	mistral: "e.g. mistral-7b-instruct, mixtral-8x7b",
	moonshot: "e.g. moonshot-v1-8k, moonshot-v1-32k",
	openrouter: "e.g. openai/gpt-4, anthropic/claude-3-haiku",
	sgl: "e.g. sgl-2, sgl-vision",
	parasail: "e.g. parasail-2, parasail-vision",
	elevenlabs: "e.g. eleven_multilingual_v2, eleven_turbo_v2",
	perplexity: "e.g. sonar-pro, sonar-deep-research",
	ollama: "e.g. llama3.1, llama2",
	opencode: "e.g. deepseek-v4-flash-free, big-pickle",
	"opencode-go": "e.g. gpt-5.5, claude-sonnet-4-6",
	"opencode-zen": "e.g. gpt-5.5, claude-sonnet-4-6",
	openai: "e.g. gpt-4, gpt-4o, gpt-4o-mini, gpt-3.5-turbo",
	vertex: "e.g. gemini-1.5-pro, text-bison, chat-bison",
	nebius: "e.g. openai/gpt-oss-120b, google/gemma-2-9b-it-fast, Qwen/Qwen2.5-VL-72B-Instruct",
	xai: "e.g. grok-4-0709, grok-3-mini, grok-3, grok-2-vision-1212",
	replicate: "e.g. meta/llama3-1-8b-instruct, black-forest-labs/flux-dev",
	vllm: "e.g. Qwen/Qwen3-0.6B, Qwen/Qwen3-1.5B",
	runway: "e.g. gen4_turbo_image_to_video, gen3a_turbo_image_to_video",
	runware: "e.g. runware:100@1, runware:101@1",
	fireworks: "e.g. accounts/fireworks/models/deepseek-v3p2",
	sarvam: "e.g. sarvam-30b, sarvam-105b",
	siliconflow: "e.g. deepseek-ai/DeepSeek-V3, Qwen/Qwen2.5-72B-Instruct",
	volcengine: "e.g. doubao-pro-32k, doubao-lite-32k",
	wafer: "e.g. glm-5.2, kimi-k2.6",
	zhipu: "e.g. glm-4-plus, glm-4-flash",
	tencent: "e.g. hunyuan-turbos-latest, hunyuan-t1-latest, hunyuan-pro",
	baidu: "e.g. ernie-5.1, ernie-5.0, ernie-4.5-turbo-128k",
	sensenova: "e.g. SenseNova-v5, SenseChat-32K",
};

// Note: i18n-aware label lookups are handled at the call site.
export const isKeyRequiredByProvider: Record<ProviderName, boolean> = {
	alibaba: true,
	anthropic: true,
	azure: true,
	bedrock: true,
	bedrock_mantle: true,
	cerebras: true,
	cohere: true,
	deepseek: true,
	gemini: true,
	groq: true,
	huggingface: true,
	minimax: true,
	mistral: true,
	moonshot: true,
	openrouter: true,
	sgl: false,
	parasail: true,
	elevenlabs: true,
	ollama: false,
	opencode: false,
	"opencode-go": true,
	"opencode-zen": true,
	openai: true,
	vertex: true,
	perplexity: true,
	nebius: true,
	xai: true,
	replicate: true,
	runway: true,
	runware: true,
	vllm: false,
	fireworks: true,
	sarvam: true,
	siliconflow: true,
	volcengine: true,
	wafer: true,
	zhipu: true,
	tencent: true,
	baidu: true,
	sensenova: true,
};

// Provider websites (link on the provider detail header) for known providers.
// Custom providers (defined via custom_provider_config at runtime) are not in
// this map and never get a header link.
export const ProviderWebsites: Partial<Record<ProviderName, string>> = {
	alibaba: "https://www.aliyun.com/product/dashscope",
	anthropic: "https://www.anthropic.com",
	azure: "https://azure.microsoft.com/products/ai-services/openai-service",
	bedrock: "https://aws.amazon.com/bedrock/",
	bedrock_mantle: "https://aws.amazon.com/bedrock/mantle/",
	cerebras: "https://www.cerebras.ai",
	cohere: "https://cohere.com",
	deepseek: "https://www.deepseek.com",
	gemini: "https://ai.google.dev",
	groq: "https://groq.com",
	huggingface: "https://huggingface.co",
	minimax: "https://www.minimaxi.com",
	mistral: "https://mistral.ai",
	moonshot: "https://platform.moonshot.cn",
	openrouter: "https://openrouter.ai",
	sgl: "https://sgl-project.github.io",
	parasail: "https://parasail.io",
	elevenlabs: "https://elevenlabs.io",
	perplexity: "https://www.perplexity.ai",
	ollama: "https://ollama.com",
	opencode: "https://opencode.ai",
	"opencode-go": "https://opencode.ai",
	"opencode-zen": "https://opencode.ai",
	openai: "https://platform.openai.com",
	vertex: "https://cloud.google.com/vertex-ai",
	nebius: "https://nebius.com",
	xai: "https://x.ai",
	replicate: "https://replicate.com",
	runway: "https://runwayml.com",
	runware: "https://runware.ai",
	vllm: "https://docs.vllm.ai",
	fireworks: "https://fireworks.ai",
	sarvam: "https://www.sarvam.ai",
	siliconflow: "https://siliconflow.cn",
	volcengine: "https://www.volcengine.com/product/ark",
	wafer: "https://waferhoufy.com",
	zhipu: "https://open.bigmodel.cn",
	tencent: "https://cloud.tencent.com/product/hunyuan",
	baidu: "https://cloud.baidu.com/product/wenxinworkshop",
	sensenova: "https://platform.sensenova.cn",
};

// API-key registration/creation pages per known provider. Rendered as a
// "Get API key" link on the provider detail header; hidden for custom
// providers and for keyless providers.
export const ProviderApiKeyUrls: Partial<Record<ProviderName, string>> = {
	alibaba: "https://www.aliyun.com/product/dashscope",
	anthropic: "https://console.anthropic.com/settings/keys",
	azure: "https://portal.azure.com/#settings/keys",
	bedrock: "https://us-east-1.console.aws.amazon.com/bedrock/home",
	bedrock_mantle: "https://console.aws.amazon.com/bedrock",
	cerebras: "https://inference.cerebras.ai",
	cohere: "https://dashboard.cohere.com/api-keys",
	deepseek: "https://platform.deepseek.com/api_keys",
	gemini: "https://aistudio.google.com/apikey",
	groq: "https://console.groq.com/keys",
	huggingface: "https://huggingface.co/settings/tokens",
	minimax: "https://www.minimaxi.com/user-center/basic-information/interface-key",
	mistral: "https://console.mistral.ai/api-keys",
	moonshot: "https://platform.moonshot.cn/console/api-keys",
	openrouter: "https://openrouter.ai/settings/keys",
	sgl: "",
	parasail: "https://console.parasail.io/api-keys",
	elevenlabs: "https://elevenlabs.io/app/settings/api-keys",
	perplexity: "https://www.perplexity.ai/settings/api",
	ollama: "",
	opencode: "",
	"opencode-go": "https://opencode.ai/settings/keys",
	"opencode-zen": "https://opencode.ai/settings/keys",
	openai: "https://platform.openai.com/api-keys",
	vertex: "https://console.cloud.google.com/apis/credentials",
	nebius: "https://nebius.com/console/keys",
	xai: "https://console.x.ai/api-keys",
	replicate: "https://replicate.com/account/api-tokens",
	runway: "https://app.runwayml.com/api-keys",
	runware: "https://runware.ai/console/api-keys",
	vllm: "",
	fireworks: "https://app.fireworks.ai/api-keys",
	sarvam: "https://console.sarvam.ai/api-keys",
	siliconflow: "https://cloud.siliconflow.cn/account/api-key",
	volcengine: "https://console.volcengine.com/ark/region:ark+cn-beijing/apiKey",
	wafer: "",
	zhipu: "https://open.bigmodel.cn/usercenter/apikeys",
	tencent: "https://console.cloud.tencent.com/hunyuan/api-key",
	baidu: "https://console.bce.baidu.com/qianfan/ais/console/application/list",
	sensenova: "https://platform.sensenova.cn/console/keys",
};

export const DefaultNetworkConfig = {
	base_url: "",
	default_request_timeout_in_seconds: 300,
	max_retries: 0,
	retry_backoff_initial: 1000,
	retry_backoff_max: 10000,
	insecure_skip_verify: false,
	ca_cert_pem: { value: "", ref: "" },
	stream_idle_timeout_in_seconds: 120,
	keep_alive_timeout_in_seconds: 30,
	max_conns_per_host: 5000,
	enforce_http2: false,
	allow_private_network: false,
} satisfies NetworkConfig;

export const DefaultPerformanceConfig = {
	concurrency: 1000,
	buffer_size: 5000,
} satisfies ConcurrencyAndBufferSize;

export const MCP_STATUS_COLORS: Record<string, string> = {
	healthy: "bg-green-100 text-green-800",
	error: "bg-red-100 text-red-800",
	// Amber, not red/gray: pg-gateway's own connection check most recently
	// failed, but this is purely informational — nothing is gated on it, and
	// it self-heals on the next successful check. Same mild treatment as
	// pending_verification, deliberately distinct from needs_reauth's red
	// ("action required").
	unstable: "bg-yellow-100 text-yellow-800",
	pending_verification: "bg-yellow-100 text-yellow-800",
	disabled: "bg-orange-100 text-orange-800",
	// Same red as `error`: the client's credential has died and it can't be
	// used until a human reauthorizes it, mirroring the "destructive" treatment
	// this status already gets on the MCP sessions table.
	needs_reauth: "bg-red-100 text-red-800",
	// Distinct blue/purple, not amber/red: unlike unstable, this isn't "one
	// instance's check currently failing" — it's "instances disagree with
	// each other about the state," which needs its own visual signal to
	// prompt a look at the per-instance breakdown rather than being read as
	// just another flavor of unhealthy.
	degraded: "bg-blue-100 text-blue-800",
};

// Mapping of what IS supported by each base provider
export const PROVIDER_SUPPORTED_REQUESTS: Record<BaseProvider, string[]> = {
	openai: [
		"list_models",
		"text_completion",
		"text_completion_stream",
		"chat_completion",
		"chat_completion_stream",
		"responses",
		"responses_stream",
		"responses_retrieve",
		"responses_delete",
		"responses_cancel",
		"responses_input_items",
		"embedding",
		"speech",
		"speech_stream",
		"transcription",
		"transcription_stream",
		"image_generation",
		"image_generation_stream",
		"image_edit",
		"image_edit_stream",
		"image_variation",
		"count_tokens",
		"video_generation",
		"video_retrieve",
		"video_download",
		"video_delete",
		"video_list",
		"video_remix",
	],
	anthropic: ["list_models", "chat_completion", "chat_completion_stream", "responses", "responses_stream", "count_tokens"],
	gemini: [
		"list_models",
		"chat_completion",
		"chat_completion_stream",
		"responses",
		"responses_stream",
		"embedding",
		"transcription",
		"transcription_stream",
		"speech",
		"speech_stream",
		"image_generation",
		"image_edit",
		"count_tokens",
		"video_generation",
		"video_retrieve",
		"video_download",
		"video_delete",
		"video_list",
		"video_remix",
	],
	cohere: ["list_models", "chat_completion", "chat_completion_stream", "responses", "responses_stream", "embedding", "count_tokens"],
	bedrock: [
		"list_models",
		"text_completion",
		"chat_completion",
		"chat_completion_stream",
		"responses",
		"responses_stream",
		"embedding",
		"image_generation",
		"image_edit",
		"image_variation",
	],
	replicate: [
		"list_models",
		"text_completion",
		"chat_completion",
		"chat_completion_stream",
		"responses",
		"responses_stream",
		"image_generation",
		"image_generation_stream",
		"image_edit",
		"image_edit_stream",
		"video_generation",
		"video_retrieve",
		"video_download",
		"video_delete",
		"video_list",
		"video_remix",
	],
	fireworks: [
		"list_models",
		"text_completion",
		"text_completion_stream",
		"chat_completion",
		"chat_completion_stream",
		"responses",
		"responses_stream",
		"embedding",
	],
};