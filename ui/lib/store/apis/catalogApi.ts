import { BundlesResponse, RecentRoutingRulesResponse } from "@/lib/types/catalog";
import { ModelProvider, ModelProviderKey } from "@/lib/types/config";
import { baseApi } from "./baseApi";

/**
 * RTK Query slice for the home-page free-tier catalog feature.
 *
 * Endpoints:
 *   - GET  /api/catalog/bundles?lang=<lang>          (运营推送的免费套餐)
 *   - GET  /api/logs/recent-routing-rules?limit=<n>  (最近用过的路由规则热度)
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

		// Aggregate the most recently used routing rules from request logs,
		// shown as a "heat" footer under each bundle card.
		getRecentRoutingRules: builder.query<RecentRoutingRulesResponse, { limit?: number }>({
			query: ({ limit }) => ({
				url: "/logs/recent-routing-rules",
				params: { limit: limit ?? 100 },
			}),
		}),

		// One-click provider registration from a bundle row. The backend fills
		// in defaults for every field not present here.
		createProvider: builder.mutation<ModelProvider, { provider: string }>({
			query: ({ provider }) => ({
				url: "/providers",
				method: "POST",
				body: { provider },
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

export const { useGetBundlesQuery, useGetRecentRoutingRulesQuery, useCreateProviderMutation, useCreateProviderKeyMutation } = catalogApi;