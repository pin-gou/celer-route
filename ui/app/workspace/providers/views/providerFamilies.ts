import type { ProviderName } from "@/lib/constants/logs";

/** Provider family categories used by the Providers page list and the
 * AddProviderDialog picker. Four buckets:
 *   - 国内 (domestic): China-region providers
 *   - 国外 (overseas): non-China first-party providers
 *   - 网关 (gateway): OpenAI/Anthropic-compatible API gateways & routers
 *   - 其他 (other): everything else (image/video/speech specialists, etc.)
 */
export type FamilyKey = "domestic" | "overseas" | "gateway" | "other";

export const FAMILY_MAP: Record<FamilyKey, string[]> = {
	domestic: ["deepseek", "moonshot", "alibaba", "zhipu", "volcengine", "tencent", "baidu", "siliconflow", "minimax", "sarvam"],
	overseas: ["openai", "anthropic", "gemini", "vertex", "bedrock", "mistral", "cohere", "groq", "huggingface", "xai", "replicate"],
	gateway: [
		"openrouter",
		"groq",
		"cerebras",
		"ollama",
		"perplexity",
		"parasail",
		"nebius",
		"sgl",
		"fireworks",
		"vllm",
		"wafer",
		"opencode",
		"opencode-go",
		"opencode-zen",
		"azure",
		"bedrock_mantle",
	],
	other: ["elevenlabs", "runway", "runware"],
};

const FAMILY_LABEL_KEY: Record<FamilyKey, string> = {
	domestic: "providers2.family.domestic",
	overseas: "providers2.family.overseas",
	gateway: "providers2.family.gateway",
	other: "providers2.family.other",
};

export function getFamilyLabelKey(family: string): string {
	if (family === "Custom") return "providers2.family.custom";
	return FAMILY_LABEL_KEY[family as FamilyKey] ?? family;
}

export function getFamilyName(provider: {
	name: string;
	custom_provider_config?: { base_provider_type?: string } | null;
}): FamilyKey | "Custom" {
	if (provider.custom_provider_config) return "Custom";
	for (const [family, members] of Object.entries(FAMILY_MAP) as [FamilyKey, string[]][]) {
		if (members.includes(provider.name)) return family;
	}
	return "other";
}

export const FAMILY_ORDER: FamilyKey[] = ["domestic", "overseas", "gateway", "other"];

export const DISPLAY_FAMILY_ORDER = ["Custom", ...FAMILY_ORDER] as const;

/** Providers commonly surfaced in the "Recommended" row of the picker.
 * The order here is the display order. */
export const RECOMMENDED_PROVIDERS: ProviderName[] = ["deepseek", "alibaba", "minimax"];