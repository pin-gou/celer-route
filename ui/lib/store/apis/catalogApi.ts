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
		//
		// NOTE: The endpoint is intentionally named `oneClickCreateProvider` (not
		// `createProvider`) so it does not collide with the `createProvider`
		// endpoint registered by `providersApi` on the same `baseApi`. Both APIs
		// happen to dispatch from hooks named `useCreateProviderMutation`, but
		// RTK Query's `injectEndpoints` overrides the prior endpoint definition
		// when names collide — sharing the name caused the provider-detail form
		// to send `providersApi`-shaped payloads through the `catalogApi`
		// body wrapper.
		oneClickCreateProvider: builder.mutation<
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
		//
		// See `oneClickCreateProvider` above for why this endpoint is named
		// distinctly from `providersApi.createProviderKey`.
		oneClickCreateProviderKey: builder.mutation<ModelProviderKey, { provider: string; key: string }>({
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

// Re-export under the legacy hook names so the home-page call sites
// (`freeTierOneKeyConfigDialog.tsx` / its test) keep importing
// `useCreateProviderMutation` / `useCreateProviderKeyMutation` from this
// module. The underlying endpoints were renamed to `oneClickCreateProvider` /
// `oneClickCreateProviderKey` (see comments above) to stop colliding with the
// matching endpoints registered by `providersApi` on the same `baseApi` —
// RTK Query's `injectEndpoints` overrides the prior endpoint definition when
// names collide, which used to make the provider-detail form dispatch its
// full Key shape through the catalogApi's body wrapper.
export const { useGetBundlesQuery, useOneClickCreateProviderMutation, useOneClickCreateProviderKeyMutation } = catalogApi;