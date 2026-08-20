/** Provider family mapping per design.md. Shared between the provider list
 * (useProviders2Data) and the AddProviderDialog so both views stay in sync. */
export const FAMILY_MAP: Record<string, string[]> = {
	"OpenAI Family": ["openai", "openai-custom"],
	"Anthropic Family": ["anthropic"],
	"Google Family": ["gemini", "vertex"],
	"Meta-Llama Family": ["groq", "cerebras", "ollama", "perplexity", "openrouter", "parasail", "nebius", "xai", "sgl"],
	"AWS Family": ["bedrock"],
	Other: [],
};

export function getFamilyName(provider: { name: string; custom_provider_config?: { base_provider_type?: string } | null }): string {
	if (provider.custom_provider_config) return "Custom";
	for (const [family, members] of Object.entries(FAMILY_MAP)) {
		if (members.includes(provider.name)) return family;
	}
	return "Other";
}

export const FAMILY_ORDER = [...Object.keys(FAMILY_MAP).filter((k) => k !== "Other"), "Other"];