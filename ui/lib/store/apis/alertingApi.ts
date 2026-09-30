import { baseApi } from "./baseApi";

// Admin alerting surface — wraps transports/celer-route-http/handlers/alerting.go.
// Routes go through /api/alert-rules and /api/alert-events; the same router
// also exposes /api/governance/budgets/{id}/projection and
// /api/alert-rules/snapshot-budgets, but Batch C-B only needs the rule
// CRUD + event list, which is what this slice covers.

export type AlertMetric = "cost_per_minute" | "cost_per_request" | "request_error_rate" | "tokens_per_minute" | "budget_consumption";

export type AlertComparison = "gte" | "gt" | "lte" | "lt" | "eq";
export type AlertRuleStatus = "enabled" | "disabled" | "draft";
export type AlertChannelType = "webhook" | "slack" | "smtp";

export interface AlertChannel {
	type: AlertChannelType;
	target: string;
	webhook_id?: string;
}

export interface AlertRule {
	id: string;
	name: string;
	scope_type: "team" | "customer" | "virtual_key" | "global";
	scope_id: string;
	metric: AlertMetric;
	threshold: number;
	comparison: AlertComparison;
	cooldown_minutes: number;
	status: AlertRuleStatus;
	channels: AlertChannel[];
	created_at: string;
	updated_at: string;
}

export interface AlertRulesListParams {
	scope_type?: AlertRule["scope_type"];
	scope_id?: string;
	metric?: AlertMetric;
	status?: AlertRuleStatus;
	search?: string;
	limit?: number;
	offset?: number;
}

export interface AlertRulesListResponse {
	rules: AlertRule[];
	total: number;
	limit: number;
	offset: number;
}

export interface AlertRuleUpsertRequest {
	name: string;
	scope_type: AlertRule["scope_type"];
	scope_id: string;
	metric: AlertMetric;
	threshold: number;
	comparison: AlertComparison;
	cooldown_minutes: number;
	status: AlertRuleStatus;
	channels: AlertChannel[];
}

export interface AlertEvent {
	id: string;
	rule_id: string;
	rule_name: string;
	scope_type: AlertRule["scope_type"];
	scope_id: string;
	metric: AlertMetric;
	value: number;
	threshold: number;
	comparison: AlertComparison;
	status: "firing" | "resolved";
	severity: "warning" | "critical";
	channels: AlertChannel[];
	triggered_at: string;
	resolved_at: string | null;
	message: string;
}

export interface AlertEventsListParams {
	rule_id?: string;
	scope_type?: AlertRule["scope_type"];
	scope_id?: string;
	status?: AlertEvent["status"];
	severity?: AlertEvent["severity"];
	limit?: number;
	offset?: number;
}

export interface AlertEventsListResponse {
	events: AlertEvent[];
	total: number;
	limit: number;
	offset: number;
}

export const alertingApi = baseApi.injectEndpoints({
	overrideExisting: false,
	endpoints: (builder) => ({
		listAlertRules: builder.query<AlertRulesListResponse, AlertRulesListParams | void>({
			query: (params) => {
				const out: Record<string, string | number> = {};
				if (params?.scope_type) out.scope_type = params.scope_type;
				if (params?.scope_id) out.scope_id = params.scope_id;
				if (params?.metric) out.metric = params.metric;
				if (params?.status) out.status = params.status;
				if (params?.search) out.search = params.search;
				if (params?.limit !== undefined) out.limit = params.limit;
				if (params?.offset !== undefined) out.offset = params.offset;
				return { url: "/alert-rules", params: out };
			},
			providesTags: (result) =>
				result
					? [...result.rules.map((r) => ({ type: "AlertRules" as const, id: r.id })), { type: "AlertRules" as const, id: "LIST" }]
					: [{ type: "AlertRules" as const, id: "LIST" }],
		}),

		getAlertRule: builder.query<AlertRule, string>({
			query: (id) => ({ url: `/alert-rules/${encodeURIComponent(id)}`, method: "GET" }),
			providesTags: (_r, _e, id) => [{ type: "AlertRules", id }],
		}),

		createAlertRule: builder.mutation<AlertRule, AlertRuleUpsertRequest>({
			query: (body) => ({ url: "/alert-rules", method: "POST", body }),
			invalidatesTags: [{ type: "AlertRules", id: "LIST" }],
		}),

		updateAlertRule: builder.mutation<AlertRule, { id: string; body: AlertRuleUpsertRequest }>({
			query: ({ id, body }) => ({ url: `/alert-rules/${encodeURIComponent(id)}`, method: "PUT", body }),
			invalidatesTags: (_r, _e, { id }) => [
				{ type: "AlertRules", id },
				{ type: "AlertRules", id: "LIST" },
			],
		}),

		deleteAlertRule: builder.mutation<{ deleted: string }, string>({
			query: (id) => ({ url: `/alert-rules/${encodeURIComponent(id)}`, method: "DELETE" }),
			invalidatesTags: (_r, _e, id) => [
				{ type: "AlertRules", id },
				{ type: "AlertRules", id: "LIST" },
			],
		}),

		testAlertRule: builder.mutation<{ ok: boolean; message?: string }, string>({
			query: (id) => ({ url: `/alert-rules/${encodeURIComponent(id)}/test`, method: "POST" }),
		}),

		listAlertEvents: builder.query<AlertEventsListResponse, AlertEventsListParams | void>({
			query: (params) => {
				const out: Record<string, string | number> = {};
				if (params?.rule_id) out.rule_id = params.rule_id;
				if (params?.scope_type) out.scope_type = params.scope_type;
				if (params?.scope_id) out.scope_id = params.scope_id;
				if (params?.status) out.status = params.status;
				if (params?.severity) out.severity = params.severity;
				if (params?.limit !== undefined) out.limit = params.limit;
				if (params?.offset !== undefined) out.offset = params.offset;
				return { url: "/alert-events", params: out };
			},
			providesTags: (result) =>
				result
					? [...result.events.map((e) => ({ type: "AlertEvents" as const, id: e.id })), { type: "AlertEvents" as const, id: "LIST" }]
					: [{ type: "AlertEvents", id: "LIST" }],
		}),

		getAlertEvent: builder.query<AlertEvent, string>({
			query: (id) => ({ url: `/alert-events/${encodeURIComponent(id)}`, method: "GET" }),
			providesTags: (_r, _e, id) => [{ type: "AlertEvents", id }],
		}),
	}),
});

export const {
	useListAlertRulesQuery,
	useGetAlertRuleQuery,
	useCreateAlertRuleMutation,
	useUpdateAlertRuleMutation,
	useDeleteAlertRuleMutation,
	useTestAlertRuleMutation,
	useListAlertEventsQuery,
	useGetAlertEventQuery,
} = alertingApi;