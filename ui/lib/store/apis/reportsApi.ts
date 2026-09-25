import { baseApi } from "./baseApi";

// Reports surface — wraps transports/celer-route-http/handlers/reports_*.go.
// Covers: standard-prices (pricing-book admin), gateway-delta (loss/gain
// analysis), cost allocation (summary / trend / details / forecast /
// by-member / by-vk), cache-savings, budget projection, and idle-keys.

// ── Standard prices ─────────────────────────────────────────────────

export interface StandardPriceRow {
	id: string;
	provider: string;
	model: string;
	currency: string;
	input_cost_per_million: number;
	output_cost_per_million: number;
	cache_read_cost_per_million?: number | null;
	cost_per_request?: number | null;
	fx_rate: number;
	effective_from: string;
	created_at: string;
	updated_at: string;
}

export interface StandardPriceUpsertRequest {
	id?: string;
	provider: string;
	model: string;
	currency?: string;
	input_cost_per_million: number;
	output_cost_per_million: number;
	cache_read_cost_per_mil?: number | null;
	cost_per_request?: number | null;
	fx_rate?: number;
	effective_from?: string | null;
}

export interface StandardPriceListParams {
	provider?: string;
	model?: string;
	limit?: number;
	offset?: number;
}

export interface StandardPriceListResponse {
	rows: StandardPriceRow[];
	total: number;
}

export interface StandardPriceSyncResponse {
	created: number;
	as_of: string;
}

// ── Gateway delta ──────────────────────────────────────────────────

export interface GatewayDeltaRow {
	id: string;
	name?: string;
	provider?: string;
	model?: string;
	requests: number;
	tokens: number;
	cost_actual: number;
	cost_standard: number;
	delta: number;
	share_pct?: number;
	has_standard_price: boolean;
}

export interface GatewayDeltaCoverage {
	models_priced: number;
	models_total: number;
	priced_actual_cost: number;
	total_actual_cost: number;
}

export interface GatewayDeltaResponse {
	period: { start: string; end: string };
	currency: string;
	accuracy: string;
	total: {
		requests?: number;
		tokens?: number;
		cost_actual?: number;
		cost_standard?: number;
		delta?: number;
		share_pct?: number;
	};
	coverage: GatewayDeltaCoverage;
	providers: GatewayDeltaRow[];
	models: GatewayDeltaRow[];
	note?: string;
}

export interface GatewayDeltaParams {
	start?: string;
	end?: string;
	accuracy?: string;
	dimension?: "provider" | "model";
}

// ── Cost allocation ───────────────────────────────────────────────

export interface CostSummaryParams {
	start?: string;
	end?: string;
	dimension?: string;
}

export interface CostSummaryResponse {
	period: { start: string; end: string };
	currency: string;
	total: { cost?: number; cost_actual?: number; delta?: number };
	rows: Array<{ id: string; name?: string; requests?: number; tokens?: number; cost?: number; cost_actual?: number; delta?: number }>;
}

export interface CostTrendParams {
	start?: string;
	end?: string;
	dimension?: string;
}

export interface CostTrendResponse {
	dimension: string;
	period: { start: string; end: string };
	series: unknown;
	currency: string;
}

export interface CostDetailsParams {
	start?: string;
	end?: string;
	dimension?: string;
	dimension_id?: string;
	limit?: number;
	offset?: number;
}

export interface CostDetailsResponse {
	dimension: string;
	dimension_id: string;
	period: { start: string; end: string };
	page: {
		rows: Array<Record<string, unknown>>;
		total: number;
		limit: number;
		note?: string;
	};
}

export interface CostForecastResponse {
	period: { start: string; end: string };
	observed_total: number;
	projected_total: number;
	risk: string;
	currency: string;
}

export interface CostByMemberResponse {
	team_id: string;
	period: { start: string; end: string };
	rows: Array<{ id: string; name?: string; requests?: number; tokens?: number; cost?: number }>;
	currency: string;
	note?: string;
}

export interface CostByVKResponse {
	team_id: string;
	period: { start: string; end: string };
	rows: Array<{ id: string; name?: string; requests?: number; tokens?: number; cost?: number }>;
	currency: string;
}

// ── Cache savings ─────────────────────────────────────────────────

export interface CacheSavingsParams {
	start?: string;
	end?: string;
}

export interface CacheSavingsResponse {
	period: { start: string; end: string };
	currency: string;
	window: {
		cache_hit_requests: number;
		cache_hit_input_tokens: number;
		estimated_savings: number;
	};
	process: {
		cache_hits: number;
		cache_misses: number;
		hit_rate: number;
	} | null;
}

// ── Budget projection ─────────────────────────────────────────────

export interface BudgetProjectionResponse {
	budget_id: string;
	used_amount: number;
	max_amount: number;
	usage_percent: number;
	risk_level: string;
	has_projection: boolean;
	predicted_exhaustion?: string;
	reason?: string;
}

// ── Idle keys ─────────────────────────────────────────────────────

export interface IdleKeysParams {
	idle_days?: number;
	limit?: number;
	offset?: number;
}

export interface IdleKeyRow {
	id: string;
	name: string;
	description?: string;
	is_active: boolean;
	team_id?: string | null;
	user_id?: string | null;
	customer_id?: string | null;
	last_used_at?: string | null;
	created_at: string;
	updated_at: string;
}

export interface IdleKeysResponse {
	period: { now: string; threshold: string };
	threshold: string;
	total: number;
	rows: IdleKeyRow[];
	limit: number;
	offset: number;
}

export const reportsApi = baseApi.injectEndpoints({
	overrideExisting: false,
	endpoints: (builder) => ({
		listStandardPrices: builder.query<StandardPriceListResponse, StandardPriceListParams | void>({
			query: (params) => {
				const out: Record<string, string | number> = {};
				if (params?.provider) out.provider = params.provider;
				if (params?.model) out.model = params.model;
				if (params?.limit !== undefined) out.limit = params.limit;
				if (params?.offset !== undefined) out.offset = params.offset;
				return { url: "/reports/standard-prices", params: out };
			},
			providesTags: (result) =>
				result
					? [...result.rows.map((r) => ({ type: "StandardPrices" as const, id: r.id })), { type: "StandardPrices" as const, id: "LIST" }]
					: [{ type: "StandardPrices" as const, id: "LIST" }],
		}),

		upsertStandardPrice: builder.mutation<StandardPriceRow, StandardPriceUpsertRequest>({
			query: (body) => ({ url: "/reports/standard-prices", method: "PUT", body }),
			invalidatesTags: [{ type: "StandardPrices", id: "LIST" }],
		}),

		deleteStandardPrice: builder.mutation<{ ok: boolean }, string>({
			query: (id) => ({ url: `/reports/standard-prices/${encodeURIComponent(id)}`, method: "DELETE" }),
			invalidatesTags: (_r, _e, id) => [
				{ type: "StandardPrices", id },
				{ type: "StandardPrices", id: "LIST" },
			],
		}),

		syncStandardPrices: builder.mutation<StandardPriceSyncResponse, { multiplier?: number } | void>({
			query: (arg) => ({ url: "/reports/standard-prices/sync", method: "POST", params: { multiplier: arg?.multiplier ?? 1.0 } }),
			invalidatesTags: [{ type: "StandardPrices", id: "LIST" }],
		}),

		getGatewayDelta: builder.query<GatewayDeltaResponse, GatewayDeltaParams | void>({
			query: (params) => {
				const out: Record<string, string> = {};
				if (params?.start) out.start = params.start;
				if (params?.end) out.end = params.end;
				if (params?.accuracy) out.accuracy = params.accuracy;
				return { url: "/reports/gateway-delta", params: out };
			},
			providesTags: ["GatewayDelta"],
		}),

		// ── Cost allocation ───────────────────────────────────────
		getCostSummary: builder.query<CostSummaryResponse, CostSummaryParams | void>({
			query: (params) => {
				const out: Record<string, string> = {};
				if (params?.start) out.start = params.start;
				if (params?.end) out.end = params.end;
				if (params?.dimension) out.dimension = params.dimension;
				return { url: "/reports/cost/summary", params: out };
			},
			providesTags: ["GatewayDelta"],
		}),

		getCostTrend: builder.query<CostTrendResponse, CostTrendParams | void>({
			query: (params) => {
				const out: Record<string, string> = {};
				if (params?.start) out.start = params.start;
				if (params?.end) out.end = params.end;
				if (params?.dimension) out.dimension = params.dimension;
				return { url: "/reports/cost/trend", params: out };
			},
			providesTags: ["GatewayDelta"],
		}),

		getCostDetails: builder.query<CostDetailsResponse, CostDetailsParams | void>({
			query: (params) => {
				const out: Record<string, string | number> = {};
				if (params?.start) out.start = params.start;
				if (params?.end) out.end = params.end;
				if (params?.dimension) out.dimension = params.dimension;
				if (params?.dimension_id) out.dimension_id = params.dimension_id;
				if (params?.limit) out.limit = params.limit;
				if (params?.offset) out.offset = params.offset;
				return { url: "/reports/cost/details", params: out };
			},
			providesTags: ["GatewayDelta"],
		}),

		getCostForecast: builder.query<CostForecastResponse, CostSummaryParams | void>({
			query: (params) => {
				const out: Record<string, string> = {};
				if (params?.start) out.start = params.start;
				if (params?.end) out.end = params.end;
				return { url: "/reports/cost/forecast", params: out };
			},
			providesTags: ["GatewayDelta"],
		}),

		getCostByMember: builder.query<CostByMemberResponse, { teamId: string; start?: string; end?: string }>({
			query: ({ teamId, ...rest }) => {
				const out: Record<string, string> = {};
				if (rest.start) out.start = rest.start;
				if (rest.end) out.end = rest.end;
				return { url: `/reports/cost/teams/${encodeURIComponent(teamId)}/by-member`, params: out };
			},
			providesTags: ["GatewayDelta"],
		}),

		getCostByVK: builder.query<CostByVKResponse, { teamId: string; start?: string; end?: string }>({
			query: ({ teamId, ...rest }) => {
				const out: Record<string, string> = {};
				if (rest.start) out.start = rest.start;
				if (rest.end) out.end = rest.end;
				return { url: `/reports/cost/teams/${encodeURIComponent(teamId)}/by-vk`, params: out };
			},
			providesTags: ["GatewayDelta"],
		}),

		// ── Cache savings ─────────────────────────────────────────
		getCacheSavings: builder.query<CacheSavingsResponse, CacheSavingsParams | void>({
			query: (params) => {
				const out: Record<string, string> = {};
				if (params?.start) out.start = params.start;
				if (params?.end) out.end = params.end;
				return { url: "/reports/cache/savings", params: out };
			},
			providesTags: ["GatewayDelta"],
		}),

		// ── Budget projection ──────────────────────────────────────
		getBudgetProjection: builder.query<BudgetProjectionResponse, string>({
			query: (budgetId) => ({ url: `/governance/budgets/${encodeURIComponent(budgetId)}/projection` }),
			providesTags: ["GatewayDelta"],
		}),

		// ── Idle keys ─────────────────────────────────────────────
		listIdleKeys: builder.query<IdleKeysResponse, IdleKeysParams | void>({
			query: (params) => {
				const out: Record<string, string | number> = {};
				if (params?.idle_days) out.idle_days = params.idle_days;
				if (params?.limit) out.limit = params.limit;
				if (params?.offset) out.offset = params.offset;
				return { url: "/reports/idle-keys", params: out };
			},
			providesTags: ["GatewayDelta"],
		}),
	}),
});

// exportGatewayDeltaUrl is a helper, not an RTK Query endpoint — moving it
// outside the injectEndpoints() block keeps the auto-generated hook types
// happy. The window.open() call below uses the same path so callers
// don't need it at runtime; we keep it as a pure URL builder for tests
// and headless export flows.
export const buildGatewayDeltaExportUrl = (params: GatewayDeltaParams | void): string => {
	const search = new URLSearchParams();
	if (params?.start) search.set("start", params.start);
	if (params?.end) search.set("end", params.end);
	if (params?.accuracy) search.set("accuracy", params.accuracy);
	const q = search.toString();
	return `/api/reports/gateway-delta/export${q ? `?${q}` : ""}`;
};

export const {
	useListStandardPricesQuery,
	useUpsertStandardPriceMutation,
	useDeleteStandardPriceMutation,
	useSyncStandardPricesMutation,
	useGetGatewayDeltaQuery,
	useGetCostSummaryQuery,
	useGetCostTrendQuery,
	useGetCostDetailsQuery,
	useGetCostForecastQuery,
	useGetCostByMemberQuery,
	useGetCostByVKQuery,
	useGetCacheSavingsQuery,
	useGetBudgetProjectionQuery,
	useListIdleKeysQuery,
} = reportsApi;