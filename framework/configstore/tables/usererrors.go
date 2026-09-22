package tables

import "errors"

// Sentinel errors used by TableUser / TableTeamMember BeforeSave hooks and
// the user-management API. Keeping them in one place makes it easy for
// callers to errors.Is-check the failure mode without parsing strings.
var (
	errEmptyUserEmail     = errors.New("user email is required")
	errInvalidUserEmail   = errors.New("user email is invalid")
	errInvalidUserStatus  = errors.New("user status must be one of pending/active/disabled")
	errInvalidUserRole    = errors.New("user role must be one of admin/member")
	errEmptyUserPassword  = errors.New("user password is required")
	errInvalidMembership  = errors.New("team_member role_in_team must be one of owner/admin/member")
	errInvalidMemberState = errors.New("team_member status must be one of active/invited/removed")
	errEmptyTeamID        = errors.New("team_member team_id is required")
	errEmptyUserID        = errors.New("team_member user_id is required")
)
