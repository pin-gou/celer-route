import {
	CapacityUsageResponse,
	CooldownStateEntry,
	CooldownStats,
	CooldownStateResponse,
	CooldownStatsResponse,
	CreatePluginRequest,
	LearnedLimitConfidence,
	LearnedLimitEntry,
	LearnedLimitsResponse,
	Plugin,
	PluginsResponse,
	RtkStatsHistogramResponse,
	RtkStatsResponse,
	UnfreezeCooldownResponse,
	UpdatePluginRequest,
} from "@/lib/types/plugins";
import { baseApi } from "./baseApi";

// Wire shapes for the provider-cooldown capacity/learning endpoints. Declared
// locally and kept private: the UI only ever sees the camelCase contract, so
// these exist purely to type `transformResponse` without falling back to
// `any`.
interface CapacityUsageWireEntry {
	provider: string;
	model?: string;
	key_id?: string;
	key_name?: string;
	scope?: string;
	window_seconds?: number;
	used_tokens?: number;
	used_requests?: number;
	token_limit?: number;
	request_limit?: number;
	headroom?: number;
	threshold_tokens?: number;
	threshold_requests?: number;
	retry_after_seconds?: number;
	source?: string;
}

interface CapacityUsageWireResponse {
	plugin?: string;
	count?: number;
	entries?: CapacityUsageWireEntry[];
}

interface LearnedLimitWireEntry {
	provider: string;
	model?: string;
	scope?: string;
	key_name?: string | null;
	sample_count?: number;
	key_count?: number;
	estimated_token_limit?: number | null;
	window_seconds?: number;
	lower_bound?: number | null;
	upper_bound?: number | null;
	confidence?: string;
	request_sample_count?: number;
	estimated_request_limit?: number | null;
	request_lower_bound?: number | null;
	request_upper_bound?: number | null;
	request_confidence?: string;
	limiter_dimension?: string;
	dimension_sample_counts?: Record<string, number>;
	first_seen?: string;
	last_seen?: string;
}

interface LearnedLimitsWireResponse {
	plugin?: string;
	count?: number;
	window_seconds?: number;
	limits?: LearnedLimitWireEntry[];
}

function normalizeLearnedConfidence(value: string | undefined): LearnedLimitConfidence {
	switch (value) {
		case "low":
		case "medium":
		case "high":
			return value;
		default:
			return "insufficient";
	}
}

function normalizeLearnedDimension(value: string | undefined): LearnedLimitEntry["limiterDimension"] {
	switch (value) {
		case "rpm":
		case "tpm":
		case "unknown":
			return value;
		default:
			return null;
	}
}

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
					// per_key stays keyed by "<provider>::<keyID>" (the
					// lookup key the plugin uses) and only its row fields
					// are camelCased.
					let perKey: CooldownStats["perKey"];
					if (response.per_key && typeof response.per_key === "object") {
						perKey = {};
						for (const [key, row] of Object.entries(response.per_key)) {
							const r = row as {
								mark_count?: number;
								suppressed_count?: number;
								success_count?: number;
								last_mark_at?: string;
								last_success_at?: string;
								cooldown_remaining?: number;
							};
							perKey[key] = {
								markCount: r.mark_count ?? 0,
								suppressedCount: r.suppressed_count ?? 0,
								successCount: r.success_count ?? 0,
								lastMarkAt: r.last_mark_at,
								lastSuccessAt: r.last_success_at,
								cooldownRemaining: r.cooldown_remaining,
							};
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
							perKey,
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

		// GET /api/plugins/provider-cooldown/capacity-usage — live capacity
		// accounting per (provider, model): how much of the rolling window's
		// budget is spent and whether the gate is currently engaged. Backend
		// returns snake_case; mapped to camelCase for the UI contract.
		getCapacityUsage: builder.query<CapacityUsageResponse, void>({
			query: () => "/plugins/provider-cooldown/capacity-usage",
			providesTags: ["Plugins"],
			transformResponse: (response: CapacityUsageWireResponse): CapacityUsageResponse => ({
				plugin: response.plugin ?? "provider-cooldown",
				count: response.count ?? response.entries?.length ?? 0,
				entries: (response.entries ?? []).map((entry) => ({
					provider: entry.provider,
					model: entry.model ?? "",
					keyId: entry.key_id,
					keyName: entry.key_name,
					scope: entry.scope === "provider_model" ? "provider_model" : entry.scope === "key" ? "key" : "provider",
					windowSeconds: entry.window_seconds ?? 0,
					usedTokens: entry.used_tokens ?? 0,
					usedRequests: entry.used_requests ?? 0,
					tokenLimit: entry.token_limit ?? 0,
					requestLimit: entry.request_limit ?? 0,
					headroom: entry.headroom ?? 0,
					thresholdTokens: entry.threshold_tokens ?? 0,
					thresholdRequests: entry.threshold_requests ?? 0,
					retryAfterSeconds: entry.retry_after_seconds ?? 0,
					source: entry.source === "provider" ? "provider" : "default",
				})),
			}),
		}),

		// GET /api/plugins/provider-cooldown/learned-limits — the read-only
		// learner's per-(provider, model) estimate of the effective window
		// limit. Same snake_case → camelCase mapping as capacity-usage.
		getLearnedLimits: builder.query<LearnedLimitsResponse, void>({
			query: () => "/plugins/provider-cooldown/learned-limits",
			providesTags: ["Plugins"],
			transformResponse: (response: LearnedLimitsWireResponse): LearnedLimitsResponse => ({
				plugin: response.plugin ?? "provider-cooldown",
				count: response.count ?? response.limits?.length ?? 0,
				windowSeconds: response.window_seconds ?? 0,
				limits: (response.limits ?? []).map((limit) => ({
					provider: limit.provider,
					model: limit.model ?? "",
					// Older servers omit the scope; their only level was the
					// aggregate one.
					scope: limit.scope === "key_model" ? "key_model" : "provider_model",
					keyName: limit.key_name ?? null,
					sampleCount: limit.sample_count ?? 0,
					keyCount: limit.key_count ?? 0,
					estimatedTokenLimit: limit.estimated_token_limit ?? null,
					windowSeconds: limit.window_seconds ?? response.window_seconds ?? 0,
					lowerBound: limit.lower_bound ?? null,
					upperBound: limit.upper_bound ?? null,
					confidence: normalizeLearnedConfidence(limit.confidence),
					requestSampleCount: limit.request_sample_count ?? 0,
					estimatedRequestLimit: limit.estimated_request_limit ?? null,
					requestLowerBound: limit.request_lower_bound ?? null,
					requestUpperBound: limit.request_upper_bound ?? null,
					requestConfidence: normalizeLearnedConfidence(limit.request_confidence),
					limiterDimension: normalizeLearnedDimension(limit.limiter_dimension),
					dimensionSampleCounts: limit.dimension_sample_counts ?? null,
					firstSeen: limit.first_seen ?? "",
					lastSeen: limit.last_seen ?? "",
				})),
			}),
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
	useGetCapacityUsageQuery,
	useGetLearnedLimitsQuery,
	useGetRtkStatsQuery,
	useGetRtkStatsHistogramQuery,
} = pluginsApi;