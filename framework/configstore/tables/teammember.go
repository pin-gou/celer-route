package tables

import (
	"time"

	"gorm.io/gorm"
)

// Team-member lifecycle states. Invited is set when an invitation has been
// sent but the user has not yet accepted; active covers everyone currently
// in the team; removed is the soft-delete state so historical cost / usage
// rows survive even after a member leaves the team.
const (
	TeamMemberStatusInvited = "invited"
	TeamMemberStatusActive  = "active"
	TeamMemberStatusRemoved = "removed"
)

// Roles a user can hold within a single team. owner has full read/write
// access to team-scoped resources (members, model policies, budgets);
// admin can manage keys and team quotas; member is read-only for the team
// and only sees their own VK / usage.
const (
	TeamMemberRoleOwner  = "owner"
	TeamMemberRoleAdmin  = "admin"
	TeamMemberRoleMember = "member"
)

// TableTeamMember associates a user with a team and records their team-
// scoped role. The table is intentionally independent of TableTeam so we
// don't drag a `members` slice onto every team read — that would force every
// existing team listing to either hydrate the join or expose a denormalized
// stub, both of which break callers that today read TableTeam unmolested.
//
// (team_id, user_id) is unique so a single user cannot accidentally be
// inserted into the same team twice. status is set to "removed" on
// offboarding rather than deleting the row, so historical usage stays
// attributable.
type TableTeamMember struct {
	ID          string    `gorm:"primaryKey;type:varchar(255)" json:"id"`
	TeamID      string    `gorm:"type:varchar(255);uniqueIndex:idx_team_members_team_user;not null" json:"team_id"`
	UserID      string    `gorm:"type:varchar(255);uniqueIndex:idx_team_members_team_user;not null" json:"user_id"`
	RoleInTeam  string    `gorm:"type:varchar(20);not null" json:"role_in_team"`
	Status      string    `gorm:"type:varchar(20);default:'active';index" json:"status"`
	JoinedAt    time.Time `gorm:"index;not null" json:"joined_at"`
	CreatedAt   time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"index;not null" json:"updated_at"`
}

// TableName sets the table name.
func (TableTeamMember) TableName() string { return "team_members" }

// BeforeSave enforces enum bounds and ensures joined_at is populated when
// missing. We do not touch the (team_id, user_id) unique constraint here —
// that is a database-level invariant enforced by the index, and GORM will
// surface a duplicate-key error that the caller can convert into a 409.
func (m *TableTeamMember) BeforeSave(tx *gorm.DB) error {
	if m.TeamID == "" {
		return errEmptyTeamID
	}
	if m.UserID == "" {
		return errEmptyUserID
	}
	if m.JoinedAt.IsZero() {
		m.JoinedAt = time.Now().UTC()
	}
	if m.Status == "" {
		m.Status = TeamMemberStatusActive
	}
	switch m.Status {
	case TeamMemberStatusInvited, TeamMemberStatusActive, TeamMemberStatusRemoved:
	default:
		return errInvalidMemberState
	}
	switch m.RoleInTeam {
	case TeamMemberRoleOwner, TeamMemberRoleAdmin, TeamMemberRoleMember:
	default:
		return errInvalidMembership
	}
	return nil
}
