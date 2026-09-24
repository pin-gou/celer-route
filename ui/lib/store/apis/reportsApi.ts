import { baseApi } from "./baseApi";

// Phase 5 reports surface — wraps transports/celer-route-http/handlers/reports_*.go.
// Batch C-C covers three of the seven reports: standard-prices (the
// pricing-book admin), gateway-delta (loss/gain analysis), and the
// landing page. Cost summary / trend / details / forecast / cache-savings /
// team-by-member / team-by-vk all stay backend-only for this round.

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
} = reportsApi;