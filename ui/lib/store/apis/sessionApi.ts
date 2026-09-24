import { baseApi, clearAuthStorage } from "./baseApi";

export interface LoginRequest {
	username: string;
	password: string;
}

export interface LoginResponse {
	message: string;
}

export interface IsAuthEnabledResponse {
	is_auth_enabled: boolean;
	has_valid_token: boolean;
	auth_type?: "sso" | "password" | "none";
}

export interface LogoutResponse {
	message: string;
}

// Member session — Phase 1 identity.
// The cookie that backs it is distinct from the admin "token" cookie
// (server cookie name `bf_member_session`); admin and member sessions
// can therefore coexist on the same browser without colliding. The
// `/api/member/auth-status` endpoint intentionally returns 200 (not
// 401) on a missing cookie so the SPA can call it unconditionally
// without surfacing an error in the network panel.
export interface MemberLoginRequest {
	email: string;
	password: string;
}

export interface MemberUserView {
	id: string;
	email: string;
	display_name: string;
	status: "pending" | "active" | "disabled";
	role: "admin" | "member";
	last_login_at: string | null;
	created_at: string;
	updated_at: string;
}

export interface MemberTeamMembership {
	id: string;
	name: string;
	role: string;
}

export interface MemberLoginResponse {
	message: string;
	user: MemberUserView;
}

export interface MemberMeResponse {
	user: MemberUserView;
	teams: MemberTeamMembership[];
	is_admin: boolean;
}

export interface MemberAuthStatusResponse {
	authenticated: boolean;
	auth_type: string;
}

// Phase-2 portal surfaces. Wire shape mirrors
// transports/celer-route-http/handlers/memberportal.go exactly so the
// member portal views can render against the live store without any
// field-by-field rename churn.
export interface MemberVirtualKey {
	id: string;
	name: string;
	team_id: string | null;
	is_active: boolean;
	expires_at: string | null;
	updated_at: string;
}

export interface MemberVirtualKeysResponse {
	virtual_keys: MemberVirtualKey[];
	count: number;
}

export interface MemberVirtualKeyQuotaResponse {
	virtual_key_id: string;
	name: string;
	is_active: boolean;
	team_id: string | null;
	expires_at: string | null;
}

export interface MemberUsageResponse {
	user_id: string;
	email: string;
	last_login_at: string | null;
	// Phase 2 returns an empty histogram map. The detailed numbers
	// land when /api/logs/histogram is delegated to this surface
	// (tracked as a follow-up; the empty shape is intentional so the
	// wire stays stable across that change).
	usage: Record<string, never>;
}

export interface MemberSetupGuideResponse {
	base_url: string;
	compatible_with: "openai";
	available_keys: { id: string; name: string }[];
	example_request: string;
	docs: string;
}

export const sessionApi = baseApi.injectEndpoints({
	overrideExisting: false,
	endpoints: (builder) => ({
		// Check if auth is enabled
		isAuthEnabled: builder.query<IsAuthEnabledResponse, void>({
			query: () => ({
				url: "/session/is-auth-enabled",
				method: "GET",
			}),
			providesTags: ["Sessions"],
		}),
		// Login endpoint
		login: builder.mutation<LoginResponse, LoginRequest>({
			query: (credentials) => ({
				url: "/session/login",
				method: "POST",
				body: credentials,
			}),
			invalidatesTags: ["Sessions"],
		}),

		// Logout endpoint
		logout: builder.mutation<LogoutResponse, void>({
			async queryFn(_arg, _api, _extraOptions, baseQuery) {
				const passwordLogout = await baseQuery({
					url: "/session/logout",
					method: "POST",
				});

				const oauthLogout = await baseQuery({
					url: "/scim/oauth/logout",
					method: "POST",
				});

				if (passwordLogout.error && oauthLogout.error) {
					return { error: oauthLogout.error };
				}

				return { data: { message: "Logout successful" } };
			},
			// After logout, clear token and all cached data
			async onQueryStarted(arg, { dispatch, queryFulfilled }) {
				try {
					await queryFulfilled;
				} catch {
				} finally {
					clearAuthStorage();
					dispatch(baseApi.util.resetApiState());
				}
			},
			invalidatesTags: ["Sessions", "Config", "Providers", "Logs", "VirtualKeys", "Teams", "Customers", "Budgets", "RateLimits"],
		}),

		// ── Member session ───────────────────────────────────────────────
		// Member auth status — returns 200 always; `authenticated` carries the signal.
		memberAuthStatus: builder.query<MemberAuthStatusResponse, void>({
			query: () => ({ url: "/member/auth-status", method: "GET" }),
			providesTags: ["MemberSession"],
		}),
		memberLogin: builder.mutation<MemberLoginResponse, MemberLoginRequest>({
			query: (body) => ({ url: "/member/login", method: "POST", body }),
			invalidatesTags: ["MemberSession"],
		}),
		memberLogout: builder.mutation<{ message: string }, void>({
			query: () => ({ url: "/member/logout", method: "POST" }),
			invalidatesTags: ["MemberSession"],
		}),
		memberMe: builder.query<MemberMeResponse, void>({
			query: () => ({ url: "/member/me", method: "GET" }),
			providesTags: ["MemberSession"],
		}),
		// ── Member portal surfaces (Phase-2 P0 unlock) ───────────────
		// GET /api/member/virtual-keys — list the active, non-expired
		// VKs owned by the current member. Provider-key metadata is
		// stripped server-side; the response never carries a value.
		memberVirtualKeys: builder.query<MemberVirtualKeysResponse, void>({
			query: () => ({ url: "/member/virtual-keys", method: "GET" }),
			providesTags: ["MemberSession"],
		}),
		// GET /api/member/virtual-keys/{vk_id}/quota — single-VK
		// ownership + lifetime. The handler collapses 404 for both
		// "missing" and "not yours" so this client treats anything
		// non-200 as "not visible to me".
		memberVirtualKeyQuota: builder.query<MemberVirtualKeyQuotaResponse, { vkID: string }>({
			query: ({ vkID }) => ({ url: `/member/virtual-keys/${encodeURIComponent(vkID)}/quota`, method: "GET" }),
			providesTags: (_r, _e, { vkID }) => [{ type: "MemberSession", id: `vk-quota:${vkID}` }],
		}),
		// GET /api/member/usage — Phase-2 stub returns counts +
		// last_login; the histogram payload lands in a follow-up
		// (see handler comment). The empty shape is a stable wire
		// contract so the UI does not need a second migration when
		// the histogram lands.
		memberUsage: builder.query<MemberUsageResponse, void>({
			query: () => ({ url: "/member/usage", method: "GET" }),
			providesTags: ["MemberSession"],
		}),
		// GET /api/member/setup-guide — base URL + available VKs +
		// a curl example. Always public-from-the-member's-perspective
		// (still behind memberMiddleware); no provider credentials.
		memberSetupGuide: builder.query<MemberSetupGuideResponse, void>({
			query: () => ({ url: "/member/setup-guide", method: "GET" }),
			providesTags: ["MemberSession"],
		}),
	}),
});

export const {
	useIsAuthEnabledQuery,
	useLoginMutation,
	useLogoutMutation,
	useMemberAuthStatusQuery,
	useMemberLoginMutation,
	useMemberLogoutMutation,
	useMemberMeQuery,
	useMemberVirtualKeysQuery,
	useMemberVirtualKeyQuotaQuery,
	useMemberUsageQuery,
	useMemberSetupGuideQuery,
} = sessionApi;