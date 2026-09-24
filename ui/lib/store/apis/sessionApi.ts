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
} = sessionApi;