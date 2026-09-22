package tables

import (
	"strings"
	"time"

	"github.com/pin-gou/celer-route/framework/encrypt"
	"gorm.io/gorm"
)

// UserStatus enumerates the lifecycle states a member account can be in.
//   - pending  : the user was invited but has not yet set a password; login is
//     blocked even if a password_hash happens to be present.
//   - active   : the user can authenticate via POST /api/member/login.
//   - disabled : the user has been deprovisioned (resign/offboard); login is
//     refused and all their VKs are flipped to is_active=false.
const (
	UserStatusPending  = "pending"
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// UserRole enumerates the gateway-level role a user holds. Phase 1 only
// recognizes "admin" (the legacy single-admin path; no new admins can be
// created from the user-management UI yet) and "member" (the team-member
// login path). Phase 2+ may add team-owner / team-admin at the team_members
// layer (role_in_team), not here.
const (
	UserRoleAdmin  = "admin"
	UserRoleMember = "member"
)

// TableUser represents a person who can authenticate to the gateway. The
// AdminUserName/AdminPassword pair on AuthConfig continues to back the
// dashboard admin path; this table powers the separate member-only login
// path. password_hash is nullable so admins can invite a member without
// setting a password upfront — the invite-accept flow backfills it.
//
// The lifecycle is intentionally minimal: status drives login gating
// (login is allowed only for status=active) and the user-management API can
// move a user between pending/active/disabled.
type TableUser struct {
	ID           string     `gorm:"primaryKey;type:varchar(255)" json:"id"`
	Email        string     `gorm:"type:varchar(320);uniqueIndex:idx_users_email_unique;not null" json:"email"`
	DisplayName  string     `gorm:"type:varchar(255)" json:"display_name"`
	PasswordHash *string    `gorm:"type:text" json:"-"` // bcrypt; nil until invite accept
	Status       string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	Role         string     `gorm:"type:varchar(20);default:'member';index" json:"role"`
	LastLoginAt  *time.Time `gorm:"type:timestamp;null" json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `gorm:"index;not null" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"index;not null" json:"updated_at"`
}

// TableName sets the table name.
func (TableUser) TableName() string { return "users" }

// BeforeSave normalizes the email and rejects malformed input before GORM
// persists the row. We keep validation here (rather than only at the API
// layer) because invitations and config-import paths also create rows
// directly, and we want a single source of truth for "what is a valid user".
func (u *TableUser) BeforeSave(tx *gorm.DB) error {
	if u.Email != "" {
		u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	}
	if u.DisplayName != "" {
		u.DisplayName = strings.TrimSpace(u.DisplayName)
	}
	if u.Email == "" {
		return errEmptyUserEmail
	}
	if !strings.Contains(u.Email, "@") {
		return errInvalidUserEmail
	}
	if u.Status == "" {
		u.Status = UserStatusPending
	}
	if u.Role == "" {
		u.Role = UserRoleMember
	}
	switch u.Status {
	case UserStatusPending, UserStatusActive, UserStatusDisabled:
	default:
		return errInvalidUserStatus
	}
	switch u.Role {
	case UserRoleAdmin, UserRoleMember:
	default:
		return errInvalidUserRole
	}
	return nil
}

// IsActive reports whether the user is allowed to authenticate. Pending and
// disabled users both fail this check — pending users must complete an
// invitation flow to be promoted to active.
func (u *TableUser) IsActive() bool {
	return u != nil && u.Status == UserStatusActive
}

// SetPassword hashes the plaintext and stores the bcrypt digest on the
// struct. Callers must follow up with a CreateUser/UpdateUser so the hook
// persists it. We never persist plaintext passwords.
func (u *TableUser) SetPassword(plaintext string) error {
	if plaintext == "" {
		return errEmptyUserPassword
	}
	hashed, err := encrypt.Hash(plaintext)
	if err != nil {
		return err
	}
	u.PasswordHash = &hashed
	return nil
}

// VerifyPassword returns true iff the stored bcrypt hash matches the
// supplied plaintext. A nil hash or a non-active user always fails closed.
func (u *TableUser) VerifyPassword(plaintext string) (bool, error) {
	if u == nil || u.PasswordHash == nil || plaintext == "" {
		return false, nil
	}
	return encrypt.CompareHash(*u.PasswordHash, plaintext)
}