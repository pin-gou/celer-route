import { baseApi } from "./baseApi";

// Member-side key-request surface — wraps
// transports/celer-route-http/handlers/keyrequests.go. The backend
// supports three kinds: join_team, extend_quota, add_vk. The UI ships
// the join_team flow (US3); extend_quota + add_vk render in the
// "my requests" list once submitted, even if the form itself is not
// yet exposed (the wire is stable).

export type KeyRequestKind = "join_team" | "extend_quota" | "add_vk";
export type KeyRequestStatus = "pending" | "approved" | "rejected";

export interface KeyRequest {
	id: string;
	user_id: string;
	team_id: string;
	kind: KeyRequestKind;
	purpose: string;
	requested_models?: string[] | null;
	budget_limit?: number | null;
	status: KeyRequestStatus;
	admin_notes?: string | null;
	decided_at?: string | null;
	created_at: string;
	updated_at: string;
}

export interface KeyRequestSubmitRequest {
	kind: KeyRequestKind;
	purpose: string;
	team_id: string;
	requested_models?: string[];
	budget_limit?: number;
}

export interface KeyRequestListParams {
	status?: KeyRequestStatus;
	limit?: number;
	offset?: number;
}

export interface KeyRequestListResponse {
	requests: KeyRequest[];
	total: number;
	limit: number;
	offset: number;
}

export const keyRequestsApi = baseApi.injectEndpoints({
	overrideExisting: false,
	endpoints: (builder) => ({
		submitKeyRequest: builder.mutation<{ key_request: KeyRequest; message: string }, KeyRequestSubmitRequest>({
			query: (body) => ({ url: "/member/key-requests", method: "POST", body }),
			invalidatesTags: ["MemberSession"],
		}),
		listMyKeyRequests: builder.query<KeyRequestListResponse, KeyRequestListParams | void>({
			query: (params) => {
				const out: Record<string, string | number> = {};
				if (params?.status) out.status = params.status;
				if (params?.limit !== undefined) out.limit = params.limit;
				if (params?.offset !== undefined) out.offset = params.offset;
				return { url: "/member/key-requests", params: out };
			},
			providesTags: ["MemberSession"],
		}),
	}),
});

export const { useSubmitKeyRequestMutation, useListMyKeyRequestsQuery } = keyRequestsApi;