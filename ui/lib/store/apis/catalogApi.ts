import { BundlesResponse } from "@/lib/types/catalog";
import { CustomProviderConfig, ModelProvider, ModelProviderKey, NetworkConfig } from "@/lib/types/config";
import { baseApi } from "./baseApi";

/**
 * RTK Query slice for the home-page free-tier catalog feature.
 *
 * Endpoints:
 *   - GET  /api/catalog/bundles?lang=<lang>          (运营推送的免费套餐)
 *   - POST /api/providers                            (一键接入 provider)
 *   - POST /api/providers/{provider}/keys            (一键填入 API key)
 */
export const catalogApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		// List free-tier bundles for the current UI language. The backend
		// always answers 200 (possibly with an empty bundle list) so the
		// frontend renders its empty/degraded state from data alone.
		getBundles: builder.query<BundlesResponse, { lang?: string }>({
			query: ({ lang }) => ({
				url: "/catalog/bundles",
				params: lang ? { lang } : {},
			}),
			providesTags: ["CatalogBundles"],
		}),

		// One-click provider registration from a bundle row. The backend fills
		// in defaults for every field not present here. Entries whose provider
		// is not built into this gateway build additionally carry the
		// custom-provider fallback (custom_provider_config + network_config),
		// which the server annotated into the catalog snapshot.
		createProvider: builder.mutation<
			ModelProvider,
			{
				provider: string;
				custom_provider_config?: CustomProviderConfig;
				network_config?: NetworkConfig;
			}
		>({
			query: (body) => ({
				url: "/providers",
				method: "POST",
				body,
			}),
			invalidatesTags: ["Providers", "CatalogBundles"],
		}),

		// One-click API key registration for an already-known provider.
		// `key` is the raw API key string; the request body adapts it to the
		// provider-keys contract (value/secret-var, wildcard models).
		createProviderKey: builder.mutation<ModelProviderKey, { provider: string; key: string }>({
			query: ({ provider, key }) => ({
				url: `/providers/${encodeURIComponent(provider)}/keys`,
				method: "POST",
				body: {
					name: provider,
					value: { value: key },
					models: ["*"],
					weight: 1.0,
					enabled: true,
				},
			}),
			invalidatesTags: ["Providers", "CatalogBundles"],
		}),
	}),
});

export const { useGetBundlesQuery, useCreateProviderMutation, useCreateProviderKeyMutation } = catalogApi;