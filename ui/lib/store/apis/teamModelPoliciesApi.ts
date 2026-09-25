import { baseApi } from "./baseApi";

// Per-team model ACL surface (Phase 6 / D6). One row per
// (team_id, provider); the wire shape mirrors the table exactly so
// the UI can diff by id/provider/timestamps without any extra layer.
//
// Routes (admin-only via /api/governance middlewares on the server):
//
//	GET    /api/governance/teams/:team_id/model-policies
//	GET    /api/governance/teams/:team_id/model-policies/:provider
//	PUT    /api/governance/teams/:team_id/model-policies/:provider
//	DELETE /api/governance/teams/:team_id/model-policies/:provider

export interface TeamModelPolicy {
	id: string;
	team_id: string;
	provider: string;
	allowed_models: string[];
	blacklisted_models: string[];
	created_by_user_id?: string | null;
	created_at: string;
	updated_at: string;
}

export interface UpsertTeamModelPolicyRequest {
	allowed_models: string[];
	blacklisted_models: string[];
}

export const teamModelPoliciesApi = baseApi.injectEndpoints({
	overrideExisting: false,
	endpoints: (builder) => ({
		listTeamModelPolicies: builder.query<TeamModelPolicy[], string>({
			query: (teamID) => ({ url: `/governance/teams/${encodeURIComponent(teamID)}/model-policies`, method: "GET" }),
			providesTags: (_r, _e, teamID) => [{ type: "TeamModelPolicies", id: teamID }],
		}),
		getTeamModelPolicy: builder.query<TeamModelPolicy, { teamID: string; provider: string }>({
			query: ({ teamID, provider }) => ({
				url: `/governance/teams/${encodeURIComponent(teamID)}/model-policies/${encodeURIComponent(provider)}`,
				method: "GET",
			}),
			providesTags: (_r, _e, { teamID, provider }) => [{ type: "TeamModelPolicies", id: `${teamID}:${provider}` }],
		}),
		upsertTeamModelPolicy: builder.mutation<TeamModelPolicy, { teamID: string; provider: string; body: UpsertTeamModelPolicyRequest }>({
			query: ({ teamID, provider, body }) => ({
				url: `/governance/teams/${encodeURIComponent(teamID)}/model-policies/${encodeURIComponent(provider)}`,
				method: "PUT",
				body,
			}),
			invalidatesTags: (_r, _e, { teamID }) => [
				{ type: "TeamModelPolicies", id: teamID },
				{ type: "TeamModelPolicies", id: "LIST" },
			],
		}),
		deleteTeamModelPolicy: builder.mutation<void, { teamID: string; provider: string }>({
			query: ({ teamID, provider }) => ({
				url: `/governance/teams/${encodeURIComponent(teamID)}/model-policies/${encodeURIComponent(provider)}`,
				method: "DELETE",
			}),
			invalidatesTags: (_r, _e, { teamID }) => [
				{ type: "TeamModelPolicies", id: teamID },
				{ type: "TeamModelPolicies", id: "LIST" },
			],
		}),
	}),
});

export const {
	useListTeamModelPoliciesQuery,
	useGetTeamModelPolicyQuery,
	useUpsertTeamModelPolicyMutation,
	useDeleteTeamModelPolicyMutation,
} = teamModelPoliciesApi;