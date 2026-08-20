import type { Plugin } from "@/lib/types/plugins";

const PLUGIN_NAME_KEYS: Record<string, string> = {
	telemetry: "pluginNames.telemetry",
	prompts: "pluginNames.prompts",
	logging: "pluginNames.logging",
	governance: "pluginNames.governance",
	otel: "pluginNames.otel",
	semantic_cache: "pluginNames.semanticCache",
	rtk: "pluginNames.rtk",
	compat: "pluginNames.compat",
	maxim: "pluginNames.maxim",
	"provider-cooldown": "pluginNames.providerCooldown",
};

// formatPluginName title-cases a kebab/snake/space-separated plugin identifier,
// e.g. "my_custom" -> "My Custom". Used as the fallback for unknown plugins.
const formatPluginName = (name: string): string =>
	name
		.split(/[-_\s]+/)
		.filter(Boolean)
		.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
		.join(" ");

export function getPluginDisplayName(plugin: Pick<Plugin, "name" | "isCustom">, t: (key: string) => string): string {
	const key = PLUGIN_NAME_KEYS[plugin.name];
	if (key) return t(key);
	return formatPluginName(plugin.name);
}