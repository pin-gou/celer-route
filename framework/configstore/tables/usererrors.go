package tables

import "errors"

// Sentinel errors used by TableUser / TableTeamMember BeforeSave hooks and
// the user-management API. Keeping them in one place makes it easy for
// callers to errors.Is-check the failure mode without parsing strings.
var (
	errEmptyUserEmail          = errors.New("user email is required")
	errInvalidUserEmail        = errors.New("user email is invalid")
	errInvalidUserStatus       = errors.New("user status must be one of pending/active/disabled")
	errInvalidUserRole         = errors.New("user role must be one of admin/member")
	errEmptyUserPassword       = errors.New("user password is required")
	errInvalidMembership       = errors.New("team_member role_in_team must be one of owner/admin/member")
	errInvalidMemberState      = errors.New("team_member status must be one of active/invited/removed")
	errEmptyTeamID             = errors.New("team_member team_id is required")
	errEmptyUserID             = errors.New("team_member user_id is required")
	errEmptyInvitationTeam     = errors.New("invitation team_id is required")
	errEmptyInvitationEmail    = errors.New("invitation email is required")
	errInvalidInvitationEmail  = errors.New("invitation email is invalid")
	errEmptyInvitationToken    = errors.New("invitation token is required")
	errInvalidInvitationStatus = errors.New("invitation status must be one of pending/accepted/declined/expired/revoked")
	errInvalidInvitationRole   = errors.New("invitation role_in_team must be one of admin/member")
	errEmptyKeyRequestUser     = errors.New("key_request user_id is required")
	errEmptyKeyRequestTeam     = errors.New("key_request team_id is required")
	errEmptyKeyRequestKind     = errors.New("key_request kind is required")
	errEmptyKeyRequestPurpose  = errors.New("key_request purpose is required")
	errInvalidKeyRequestKind   = errors.New("key_request kind must be one of join_team/extend_quota/add_vk")
	errInvalidKeyRequestStatus = errors.New("key_request status must be one of pending/approved/rejected/cancelled")
)
