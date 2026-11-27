import { useGetPluginQuery } from "@/lib/store/apis/pluginsApi";
import { PROVIDER_COOLDOWN_PLUGIN, type ProviderCooldownCapabilityDefaults } from "@/lib/types/plugins";

/**
 * Reads the provider-cooldown plugin's capability sections — the "global
 * defaults" a provider-level override inherits from. Returns an empty object
 * while the plugin is loading or not installed, in which case the callers fall
 * back to the frontend constants in `defaults.ts`.
 */
export function useCooldownCapabilityDefaults(): ProviderCooldownCapabilityDefaults {
	const { data } = useGetPluginQuery(PROVIDER_COOLDOWN_PLUGIN);
	const config = data?.config as ProviderCooldownCapabilityDefaults | undefined;
	return {
		escalation: config?.escalation,
		capacity: config?.capacity,
		learning: config?.learning,
	};
}