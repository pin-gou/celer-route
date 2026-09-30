package tables

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Invitation lifecycle states. Phase 2 only exposes the four below:
// pending is the active state when an admin generates a link; accepted
// fires when the invitee finishes the password step; declined/expired/
// revoked are terminal states that block re-use of the same token.
const (
	InvitationStatusPending  = "pending"
	InvitationStatusAccepted = "accepted"
	InvitationStatusDeclined = "declined"
	InvitationStatusExpired  = "expired"
	InvitationStatusRevoked  = "revoked"
)

// DefaultInvitationTTL bounds how long an invitation link stays valid.
// Matches the design contract in temp/team/01-identity/api.md §4 (7 days).
// Stored on the row as expires_at so the value is auditable per row rather
// than a global config knob.
const DefaultInvitationTTL = 7 * 24 * time.Hour

// TableInvitation is the audit + token carrier for "invite a teammate" and
// "invite to reset password". One row per invitation; token is the
// unique-indexed random half of the link the admin hands to the invitee.
//
// Phase 2 keeps the surface intentionally small: an admin creates a row
// (status=pending, expires_at=now+7d), the invitee posts a password to the
// accept endpoint which flips status to accepted, and the row remains as a
// permanent audit trail. We never store plaintext passwords; the password
// the invitee submits at accept time is bcrypt-hashed into the matching
// TableUser row (which the invitation row references by email + optional
// user_id link).
type TableInvitation struct {
	ID              string     `gorm:"primaryKey;type:varchar(255)" json:"id"`
	TeamID          string     `gorm:"type:varchar(255);index:idx_invitations_team;not null" json:"team_id"`
	Email           string     `gorm:"type:varchar(320);index:idx_invitations_email;not null" json:"email"`
	Token           string     `gorm:"type:varchar(255);uniqueIndex:idx_invitations_token;not null" json:"token"`
	RoleInTeam      string     `gorm:"type:varchar(20);not null" json:"role_in_team"`
	Status          string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	CreatedByUserID string     `gorm:"type:varchar(255);index" json:"created_by_user_id"`
	ExpiresAt       time.Time  `gorm:"index;not null" json:"expires_at"`
	AcceptedAt      *time.Time `gorm:"type:timestamp;null" json:"accepted_at,omitempty"`
	CreatedAt       time.Time  `gorm:"index;not null" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"index;not null" json:"updated_at"`
}

// TableName sets the table name.
func (TableInvitation) TableName() string { return "invitations" }

// BeforeSave normalizes the email and enforces enum bounds. Token is a
// high-entropy opaque string; we do not validate its shape (it is generated
// upstream by the handler) but we do require it to be present so an
// empty token never reaches the database.
func (i *TableInvitation) BeforeSave(tx *gorm.DB) error {
	if i.TeamID == "" {
		return errEmptyInvitationTeam
	}
	if i.Email != "" {
		i.Email = strings.ToLower(strings.TrimSpace(i.Email))
	}
	if i.Email == "" {
		return errEmptyInvitationEmail
	}
	if !strings.Contains(i.Email, "@") {
		return errInvalidInvitationEmail
	}
	if i.Token == "" {
		return errEmptyInvitationToken
	}
	if i.RoleInTeam == "" {
		i.RoleInTeam = TeamMemberRoleMember
	}
	if i.Status == "" {
		i.Status = InvitationStatusPending
	}
	switch i.Status {
	case InvitationStatusPending, InvitationStatusAccepted, InvitationStatusDeclined, InvitationStatusExpired, InvitationStatusRevoked:
	default:
		return errInvalidInvitationStatus
	}
	switch i.RoleInTeam {
	case TeamMemberRoleAdmin, TeamMemberRoleMember:
		// Invitations only support admin/member at the team scope today;
		// team owner is provisioned out-of-band (see Readme D8). Reject
		// any other value (notably "owner") so the audit log cannot be
		// silently mis-shouldered.
	default:
		return errInvalidInvitationRole
	}
	if i.ExpiresAt.IsZero() {
		i.ExpiresAt = time.Now().UTC().Add(DefaultInvitationTTL)
	}
	return nil
}

// IsUsable reports whether the invitation can still be accepted: pending
// status + unexpired clock. Centralized here so handlers do not race on
// the conditions in three different places.
func (i *TableInvitation) IsUsable(now time.Time) bool {
	if i == nil {
		return false
	}
	return i.Status == InvitationStatusPending && now.UTC().Before(i.ExpiresAt.UTC())
}

// MarkAccepted flips the row to its terminal accepted state and stamps the
// accepted_at timestamp. Callers must persist the change via the store
// (the BeforeSave hook validates the new state).
func (i *TableInvitation) MarkAccepted(now time.Time) {
	i.Status = InvitationStatusAccepted
	i.AcceptedAt = &now
}

// MarkRevoked flips the row to revoked. Same caveat as MarkAccepted.
func (i *TableInvitation) MarkRevoked() {
	i.Status = InvitationStatusRevoked
}
