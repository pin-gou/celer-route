import { baseApi } from "./baseApi";

export interface ErrorPattern {
	rank: number;
	count: number;
	first_seen?: string;
	last_seen?: string;
	status_code?: number;
	error_type?: string;
	error_code?: string;
	sample_message?: string;
	example_request_id?: string;
	/**
	 * What the cooldown classifier would decide for this error shape:
	 * "rpm" (request rate), "tpm" (token rate) or "unknown". Computed
	 * server-side from the same fields the mark path reads, so the echo cannot
	 * drift from the real decision.
	 */
	limiter_dimension?: "rpm" | "tpm" | "unknown";
}

export interface ErrorPatternsResponse {
	provider: string;
	window: string;
	total_errors: number;
	patterns: ErrorPattern[];
}

export const errorPatternsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getErrorPatterns: builder.query<ErrorPatternsResponse, { provider: string; window: string; limit?: number }>({
			query: ({ provider, window, limit = 20 }) => ({
				url: "/logs/error-patterns",
				params: { provider, window, limit: String(limit) },
			}),
			// No cache tags — error patterns change as new errors arrive, and the
			// component only fetches when the user selects a provider/window.
		}),
	}),
});

export const { useGetErrorPatternsQuery } = errorPatternsApi;