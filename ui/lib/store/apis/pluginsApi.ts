import {
	CooldownStateEntry,
	CooldownStats,
	CooldownStateResponse,
	CooldownStatsResponse,
	CreatePluginRequest,
	Plugin,
	PluginsResponse,
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
							expireAt: e.expires_at ?? e.expireAt,
							reason: e.reason || "cooldown",
						})),
					};
				}
				return response;
			},
		}),

		// GET /api/plugins/provider-cooldown/stats — lifetime counters + active
		// count. Backend returns { plugin, mark_count, suppressed_count,
		// current_active_count } — map to camelCase for the UI contract.
		getCooldownStats: builder.query<CooldownStatsResponse, void>({
			query: () => "/plugins/provider-cooldown/stats",
			providesTags: ["Plugins"],
			transformResponse: (response: any) => {
				if (response && response.stats) return response;
				if (response && "mark_count" in response) {
					return {
						stats: {
							markCount: response.mark_count,
							suppressedCount: response.suppressed_count,
							activeCount: response.current_active_count,
						},
					};
				}
				return response;
			},
		}),

		// DELETE /api/plugins/provider-cooldown/state/{provider}/{keyId} —
		// manually un-cool a cooldown key. Backend returns { message, provider,
		// key_id } — map key_id → keyId for the UI contract.
		unfreezeCooldown: builder.mutation<UnfreezeCooldownResponse, { provider: string; keyId: string }>({
			query: ({ provider, keyId }) => ({
				url: `/plugins/provider-cooldown/state/${provider}/${keyId}`,
				method: "DELETE",
			}),
			transformResponse: (response: any) => {
				if (response && "key_id" in response && !("keyId" in response)) {
					return { ...response, keyId: response.key_id };
				}
				return response;
			},
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
} = pluginsApi;