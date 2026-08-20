import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { ProviderLabels } from "@/lib/constants/logs";
import { useMemo } from "react";
import type { ProviderCardProvider } from "./ProviderCard";
import { FAMILY_ORDER, getFamilyName } from "./providerFamilies";

export function useProviders2Data() {
	const { data: providers, isLoading, error, refetch } = useGetProvidersQuery();

	const groupedProviders = useMemo(() => {
		if (!providers) return [];

		const groups = new Map<string, ProviderCardProvider[]>();

		for (const p of providers) {
			const family = getFamilyName(p);
			if (!groups.has(family)) {
				groups.set(family, []);
			}
			const cardProvider: ProviderCardProvider = {
				name: p.name,
				provider_status: p.provider_status,
				keys_count: p.keys_count ?? 0,
				models_count: p.models_count ?? 0,
				keys_health_status: p.keys_health_status ?? "unknown",
				keys_enabled: p.keys_enabled ?? true,
				custom_provider_config: p.custom_provider_config,
				is_key_less: p.is_key_less === true,
				status: p.status,
			};
			groups.get(family)!.push(cardProvider);
		}

		// Sort families: Custom first, then known families, then Other
		const familyOrder = ["Custom", ...FAMILY_ORDER];
		return Array.from(groups.entries())
			.sort(([a], [b]) => {
				const ai = familyOrder.indexOf(a);
				const bi = familyOrder.indexOf(b);
				return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi);
			})
			.map(([family, members]) => ({
				family,
				providers: members.sort((a, b) => a.name.localeCompare(b.name)),
			}));
	}, [providers]);

	return { groupedProviders, isLoading, error, refetch };
}