import { baseApi } from "./baseApi";

// Team-detail surface for /workspace/governance/teams/[id]. Re-uses the
// existing teams types from governanceApi for read paths; only the
// team-scoped endpoints Batch C-D needs (members list + invitation CRUD)
// live here. Members list is a new handler added in Batch C-D
// (handlers/invitations.go:listTeamMembers).

export interface TeamMember {
	id: string;
	team_id: string;
	user_id: string;
	role_in_team: string;
	status: "invited" | "active" | "removed";
	joined_at: string;
	email?: string;
	display_name?: string;
	user_status?: "pending" | "active" | "disabled";
}

export interface TeamMemberListResponse {
	members: TeamMember[];
	total: number;
}

export interface TeamInvitation {
	id: string;
	team_id: string;
	email: string;
	role_in_team: string;
	status: "pending" | "accepted" | "revoked" | "expired";
	token?: string;
	expires_at: string;
	created_at: string;
	accepted_at?: string | null;
}

export interface TeamInvitationListResponse {
	invitations: TeamInvitation[];
	total: number;
	limit: number;
	offset: number;
}

export interface CreateTeamInvitationRequest {
	email: string;
	role?: "admin" | "member";
	ttl_seconds?: number;
}

export interface CreateTeamInvitationResponse {
	invitation: TeamInvitation;
	link: string;
	token: string;
}

export const teamsApi = baseApi.injectEndpoints({
	overrideExisting: false,
	endpoints: (builder) => ({
		listTeamMembers: builder.query<TeamMemberListResponse, string>({
			query: (teamID) => ({ url: `/governance/teams/${encodeURIComponent(teamID)}/members`, method: "GET" }),
			providesTags: (_r, _e, teamID) => [{ type: "TeamMembers", id: teamID }],
		}),

		listTeamInvitations: builder.query<TeamInvitationListResponse, { teamID: string; status?: string }>({
			query: ({ teamID, status }) => ({
				url: `/governance/teams/${encodeURIComponent(teamID)}/invitations`,
				method: "GET",
				params: status ? { status } : {},
			}),
			providesTags: (_r, _e, { teamID }) => [{ type: "TeamInvitations", id: teamID }],
		}),

		createTeamInvitation: builder.mutation<CreateTeamInvitationResponse, { teamID: string; body: CreateTeamInvitationRequest }>({
			query: ({ teamID, body }) => ({
				url: `/governance/teams/${encodeURIComponent(teamID)}/invitations`,
				method: "POST",
				body,
			}),
			invalidatesTags: (_r, _e, { teamID }) => [
				{ type: "TeamInvitations", id: teamID },
				{ type: "TeamMembers", id: teamID },
			],
		}),

		revokeTeamInvitation: builder.mutation<{ invitation: TeamInvitation }, { teamID: string; invitationID: string }>({
			query: ({ teamID, invitationID }) => ({
				url: `/governance/teams/${encodeURIComponent(teamID)}/invitations/${encodeURIComponent(invitationID)}/revoke`,
				method: "POST",
			}),
			invalidatesTags: (_r, _e, { teamID }) => [{ type: "TeamInvitations", id: teamID }],
		}),
	}),
});

export const { useListTeamMembersQuery, useListTeamInvitationsQuery, useCreateTeamInvitationMutation, useRevokeTeamInvitationMutation } =
	teamsApi;