import {
	CooldownStateEntry,
	CooldownStats,
	CooldownStateResponse,
	CooldownStatsResponse,
	CreatePluginRequest,
	Plugin,
	PluginsResponse,
	RtkStatsHistogramResponse,
	RtkStatsResponse,
	UnfreezeCooldownResponse,
	UpdatePluginRequest,
} from "@/lib/types/plugins";
import { baseApi } from "./baseApi";

export const pluginsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		// Get builtin plugin names
		getBuiltinPlugins: builder.query<string[], void>({
			query: () => "/plugins/builtins",
			providesTags: ["Plugins"],
			transformResponse: (response: { plugins: string[] }) => response.plugins || [],
		}),

		// Get the names of all currently loaded plugins (sanitized to match the names
		// embedded in their trace span names). Used by the plugin tracing sheet so it
		// lists every plugin that actually emits spans, including enterprise plugins.
		getLoadedPlugins: builder.query<string[], void>({
			query: () => "/plugins/loaded",
			providesTags: ["Plugins"],
			transformResponse: (response: { plugins: string[] }) => response.plugins || [],
		}),

		// Get all plugins
		getPlugins: builder.query<Plugin[], void>({
			query: () => "/plugins",
			providesTags: ["Plugins"],
			transformResponse: (response: PluginsResponse) => response.plugins || [],
		}),

		// Get a single plugin
		getPlugin: builder.query<Plugin, string>({
			query: (name) => `/plugins/${name}`,
			providesTags: (result, error, name) => [{ type: "Plugins", id: name }],
		}),

		// Create new plugin
		createPlugin: builder.mutation<Plugin, CreatePluginRequest>({
			query: (data) => ({
				url: "/plugins",
				method: "POST",
				body: data,
			}),
			transformResponse: (response: { message: string; plugin: Plugin }) => response.plugin,
			async onQueryStarted(arg, { dispatch, queryFulfilled }) {
				try {
					const { data: newPlugin } = await queryFulfilled;
					dispatch(
						pluginsApi.util.updateQueryData("getPlugins", undefined, (draft) => {
							draft.push(newPlugin);
						}),
					);
					// Also update the individual plugin cache
					dispatch(pluginsApi.util.updateQueryData("getPlugin", newPlugin.name, () => newPlugin));
				} catch {}
			},
		}),

		// Update existing plugin
		updatePlugin: builder.mutation<Plugin, { name: string; data: UpdatePluginRequest }>({
			query: ({ name, data }) => ({
				url: `/plugins/${name}`,
				method: "PUT",
				body: data,
			}),
			transformResponse: (response: { message: string; plugin: Plugin }) => response.plugin,
			async onQueryStarted(arg, { dispatch, queryFulfilled }) {
				try {
					const { data: updatedPlugin } = await queryFulfilled;
					dispatch(
						pluginsApi.util.updateQueryData("getPlugins", undefined, (draft) => {
							const index = draft.findIndex((p) => p.name === arg.name);
							if (index !== -1) {
								draft[index] = updatedPlugin;
							} else {
								draft.push(updatedPlugin);
							}
						}),
					);
					// Also update the individual plugin cache
					dispatch(pluginsApi.util.updateQueryData("getPlugin", arg.name, () => updatedPlugin));
				} catch {}
			},
		}),

		// Delete plugin
		deletePlugin: builder.mutation<Plugin, string>({
			query: (name) => ({
				url: `/plugins/${name}`,
				method: "DELETE",
			}),
			async onQueryStarted(pluginName, { dispatch, queryFulfilled }) {
				try {
					await queryFulfilled;
					dispatch(
						pluginsApi.util.updateQueryData("getPlugins", undefined, (draft) => {
							const index = draft.findIndex((p) => p.name === pluginName);
							if (index !== -1) {
								draft.splice(index, 1);
							}
						}),
					);
				} catch {}
			},
		}),

		// -----------------------------------------------------------------------
		// Provider cooldown monitoring endpoints
		// -----------------------------------------------------------------------

		// GET /api/plugins/provider-cooldown/state — dump active cooldown entries.
		// Backend returns { plugin, count, entries: [...] } — map entries → state
		// for the UI contract.
		getCooldownState: builder.query<CooldownStateResponse, void>({
			query: () => "/plugins/provider-cooldown/state",
			providesTags: ["Plugins"],
			transformResponse: (response: any) => {
				if (response && Array.isArray(response.entries)) {
					return {
						state: response.entries.map((e: any) => ({
							provider: e.provider,
							keyId: e.key_id ?? e.keyId,
							keyName: e.key_name ?? e.keyName,
							model: e.model ?? e.model,
							expireAt: e.expires_at ?? e.expireAt,
							reason: e.reason || "",
						})),
					};
				}
				return response;
			},
		}),

		// GET /api/plugins/provider-cooldown/stats — lifetime counters + active
		// count, plus by_kind (rate_limit vs quota) and per_provider breakdowns.
		// Backend returns:
		//   { plugin, mark_count, suppressed_count, current_active_count,
		//     by_kind: { rate_limit: {mark_count, suppressed_count},
		//               quota:      {mark_count, suppressed_count} },
		//     per_provider: { <provider>: { rate_limit: {...}, quota: {...} } } }
		// — mapped to camelCase for the UI contract.
		getCooldownStats: builder.query<CooldownStatsResponse, void>({
			query: () => "/plugins/provider-cooldown/stats",
			providesTags: ["Plugins"],
			transformResponse: (response: any) => {
				if (response && response.stats) return response;
				if (response && "mark_count" in response) {
					const byKind = response.by_kind
						? {
								rate_limit: {
									markCount: response.by_kind.rate_limit?.mark_count ?? 0,
									suppressedCount: response.by_kind.rate_limit?.suppressed_count ?? 0,
								},
								quota: {
									markCount: response.by_kind.quota?.mark_count ?? 0,
									suppressedCount: response.by_kind.quota?.suppressed_count ?? 0,
								},
							}
						: undefined;
					let perProvider: CooldownStats["perProvider"];
					if (response.per_provider && typeof response.per_provider === "object") {
						perProvider = {};
						for (const [name, counters] of Object.entries(response.per_provider)) {
							const c = counters as { rate_limit?: any; quota?: any };
							perProvider[name] = {
								rate_limit: {
									markCount: c.rate_limit?.mark_count ?? 0,
									suppressedCount: c.rate_limit?.suppressed_count ?? 0,
								},
								quota: {
									markCount: c.quota?.mark_count ?? 0,
									suppressedCount: c.quota?.suppressed_count ?? 0,
								},
							};
						}
					}
					let perProviderModel: CooldownStats["perProviderModel"];
					if (response.per_provider_model && typeof response.per_provider_model === "object") {
						perProviderModel = {};
						for (const [provider, modelMap] of Object.entries(response.per_provider_model)) {
							const models = modelMap as Record<string, any>;
							perProviderModel[provider] = {};
							for (const [model, counters] of Object.entries(models)) {
								const c = counters as { rate_limit?: any; quota?: any };
								perProviderModel[provider][model] = {
									rate_limit: {
										markCount: c.rate_limit?.mark_count ?? 0,
										suppressedCount: c.rate_limit?.suppressed_count ?? 0,
									},
									quota: {
										markCount: c.quota?.mark_count ?? 0,
										suppressedCount: c.quota?.suppressed_count ?? 0,
									},
								};
							}
						}
					}
					return {
						stats: {
							markCount: response.mark_count,
							suppressedCount: response.suppressed_count,
							activeCount: response.current_active_count,
							byKind,
							perProvider,
							perProviderModel,
						},
					};
				}
				return response;
			},
		}),

		// DELETE /api/plugins/provider-cooldown/state/{provider}/{keyId}[?model=…]
		// — manually un-cool a cooldown key. The optional `model` query param
		// targets a model-granularity (provider, key, model) entry; omit it to
		// clear the key-granularity entry. Backend returns { message, provider,
		// key_id, model } — map key_id → keyId for the UI contract.
		unfreezeCooldown: builder.mutation<UnfreezeCooldownResponse, { provider: string; keyId: string; model?: string }>({
			query: ({ provider, keyId, model }) => ({
				url: `/plugins/provider-cooldown/state/${provider}/${keyId}`,
				method: "DELETE",
				...(model ? { params: { model } } : {}),
			}),
			// Trigger an immediate refetch of the cooldown state/stats after a
			// successful unfreeze so the Active Cooldown State list drops the
			// un-frozen entry without waiting for the next polling tick.
			invalidatesTags: ["Plugins"],
			transformResponse: (response: any) => {
				if (response && "key_id" in response && !("keyId" in response)) {
					return { ...response, keyId: response.key_id };
				}
				return response;
			},
		}),

		// -----------------------------------------------------------------------
		// RTK monitoring endpoint
		// -----------------------------------------------------------------------

		// GET /api/context/rtk/stats — process-lifetime compression counters.
		// Backend returns the flat fields directly; map to a { stats: {...} }
		// envelope so the UI has the same shape as the cooldown stats endpoint.
		getRtkStats: builder.query<RtkStatsResponse, void>({
			query: () => "/context/rtk/stats",
			providesTags: ["Plugins"],
			transformResponse: (response: any) => {
				if (response && response.stats) return response;
				if (response && "invocations" in response) {
					return {
						stats: {
							invocations: response.invocations,
							compressedCount: response.compressed_count,
							originalTokens: response.original_tokens,
							compressedTokens: response.compressed_tokens,
							tokensSaved: response.tokens_saved,
							compressionRatio: response.compression_ratio,
						},
					};
				}
				return response;
			},
		}),

		// GET /api/context/rtk/stats/histogram — time-bucketed compression stats
		// for the dashboard chart. Accepts the same time-range parameters as the
		// logs histogram endpoints (start_time, end_time, period).
		getRtkStatsHistogram: builder.query<
			RtkStatsHistogramResponse,
			{ filters: { start_time?: string; end_time?: string; period?: string } }
		>({
			query: ({ filters }) => ({
				url: "/context/rtk/stats/histogram",
				params: filters,
			}),
			providesTags: ["Plugins"],
		}),
	}),
});

export const {
	useGetBuiltinPluginsQuery,
	useGetLoadedPluginsQuery,
	useGetPluginsQuery,
	useGetPluginQuery,
	useCreatePluginMutation,
	useUpdatePluginMutation,
	useDeletePluginMutation,
	useLazyGetPluginsQuery,
	useGetCooldownStateQuery,
	useGetCooldownStatsQuery,
	useUnfreezeCooldownMutation,
	useGetRtkStatsQuery,
	useGetRtkStatsHistogramQuery,
} = pluginsApi;