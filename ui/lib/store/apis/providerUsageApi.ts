import { baseApi } from "./baseApi";

// One model × API key pair's consumption inside the requested window. The
// backend already returns these sorted by total_tokens (desc) and stable, so
// callers must render them in the given order rather than re-sorting.
export interface ProviderUsageEntry {
	model: string;
	key_id: string;
	key_name: string;
	requests: number;
	successes: number;
	errors: number;
	// cancelled is tracked separately from errors — it is not a failure and
	// must not be folded into the error rate.
	cancelled: number;
	prompt_tokens: number;
	completion_tokens: number;
	cached_read_tokens: number;
	total_tokens: number;
	avg_total_tokens: number;
	requests_per_minute: number;
	tokens_per_minute: number;
}

export interface ProviderUsageResponse {
	provider: string;
	window_minutes: number;
	start_time: string;
	end_time: string;
	entries: ProviderUsageEntry[];
}

export const providerUsageApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getProviderUsage: builder.query<ProviderUsageResponse, { provider: string; windowMinutes: number }>({
			query: ({ provider, windowMinutes }) => ({
				url: `/providers/${encodeURIComponent(provider)}/usage`,
				params: { window_minutes: String(windowMinutes) },
			}),
			// No cache tags — usage is an operational snapshot that the card polls
			// on its own cadence and refetches when the user switches windows.
		}),
	}),
});

export const { useGetProviderUsageQuery } = providerUsageApi;