import { baseApi } from "./baseApi";

// Admin-side user management endpoints (Phase 1 identity, surfaced in
// Phase 6 Batch C as /workspace/governance/users). Mirrors the
// transports/celer-route-http/handlers/usermanagement.go surface so
// every CRUD shape on the wire is explicit on the TS side too.
//
// password_hash is intentionally absent (the model tags it
// `json:"-"` and memberUserView strips it), so this struct never
// carries that field even by accident.

export interface AdminUserView {
	id: string;
	email: string;
	display_name: string;
	status: "pending" | "active" | "disabled";
	role: "admin" | "member";
	last_login_at: string | null;
	created_at: string;
	updated_at: string;
}

export interface AdminUserMembership {
	team_id: string;
	role_in_team: string;
	status: string;
	joined_at: string;
}

export interface AdminUserVKRef {
	id: string;
	name: string;
	is_active: boolean;
	team_id: string | null;
	updated_at: string;
}

// GET /api/governance/users/{id} returns this composite shape
// (handlers/usermanagement.go:135). UI pages call useGetUserQuery and
// reach into .user / .memberships / .virtual_keys.
export interface AdminGetUserResponse {
	user: AdminUserView;
	memberships: AdminUserMembership[];
	virtual_keys: AdminUserVKRef[];
}

export interface AdminListUsersParams {
	status?: "pending" | "active" | "disabled";
	role?: "admin" | "member";
	limit?: number;
	offset?: number;
	search?: string;
}

export interface AdminListUsersResponse {
	users: AdminUserView[];
	total: number;
	limit: number;
	offset: number;
}

export interface AdminUpdateUserRequest {
	display_name?: string;
	role?: "admin" | "member";
	status?: "pending" | "active" | "disabled";
}

export const usersApi = baseApi.injectEndpoints({
	overrideExisting: false,
	endpoints: (builder) => ({
		listUsers: builder.query<AdminListUsersResponse, AdminListUsersParams | void>({
			query: (params) => {
				const out: Record<string, string | number> = {};
				if (params?.status) out.status = params.status;
				if (params?.role) out.role = params.role;
				if (params?.search) out.search = params.search;
				if (params?.limit !== undefined) out.limit = params.limit;
				if (params?.offset !== undefined) out.offset = params.offset;
				return { url: "/governance/users", params: out };
			},
			providesTags: (result) =>
				result
					? [...result.users.map((u) => ({ type: "Users" as const, id: u.id })), { type: "Users" as const, id: "LIST" }]
					: [{ type: "Users" as const, id: "LIST" }],
		}),

		getUser: builder.query<AdminGetUserResponse, string>({
			query: (userID) => ({ url: `/governance/users/${encodeURIComponent(userID)}`, method: "GET" }),
			providesTags: (_result, _err, id) => [{ type: "Users", id }],
		}),

		updateUser: builder.mutation<AdminUserView, { userID: string; body: AdminUpdateUserRequest }>({
			query: ({ userID, body }) => ({
				url: `/governance/users/${encodeURIComponent(userID)}`,
				method: "PUT",
				body,
			}),
			invalidatesTags: (_r, _err, { userID }) => [
				{ type: "Users", id: userID },
				{ type: "Users", id: "LIST" },
			],
		}),

		disableUser: builder.mutation<{ message: string }, string>({
			query: (userID) => ({
				url: `/governance/users/${encodeURIComponent(userID)}/disable`,
				method: "POST",
			}),
			invalidatesTags: (_r, _err, userID) => [
				{ type: "Users", id: userID },
				{ type: "Users", id: "LIST" },
			],
		}),

		disableUserVks: builder.mutation<{ message: string; count: number; disabled_virtual_key_ids: string[] }, string>({
			query: (userID) => ({
				url: `/governance/users/${encodeURIComponent(userID)}/disable-vks`,
				method: "POST",
			}),
			invalidatesTags: [
				{ type: "Users", id: "LIST" },
				{ type: "VirtualKeys", id: "LIST" },
			],
		}),

		resetUserPasswordToken: builder.mutation<{ message: string; token: string }, string>({
			query: (userID) => ({
				url: `/governance/users/${encodeURIComponent(userID)}/reset-password-token`,
				method: "POST",
			}),
			invalidatesTags: [{ type: "Users", id: "LIST" }],
		}),

		deleteUser: builder.mutation<{ message: string }, string>({
			query: (userID) => ({
				url: `/governance/users/${encodeURIComponent(userID)}`,
				method: "DELETE",
			}),
			invalidatesTags: (_r, _err, userID) => [
				{ type: "Users", id: userID },
				{ type: "Users", id: "LIST" },
			],
		}),
	}),
});

export const {
	useListUsersQuery,
	useGetUserQuery,
	useUpdateUserMutation,
	useDisableUserMutation,
	useDisableUserVksMutation,
	useResetUserPasswordTokenMutation,
	useDeleteUserMutation,
} = usersApi;