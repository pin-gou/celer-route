package tables

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTableUserBeforeSaveDefaults verifies that an empty user struct
// round-trips through BeforeSave with sensible defaults (status=pending,
// role=member) and that email gets trimmed+lowercased.
func TestTableUserBeforeSaveDefaults(t *testing.T) {
	u := &TableUser{Email: "  Alice@Example.COM  ", DisplayName: "  Alice  "}
	require.NoError(t, u.BeforeSave(nil))
	assert.Equal(t, "alice@example.com", u.Email)
	assert.Equal(t, "Alice", u.DisplayName)
	assert.Equal(t, UserStatusPending, u.Status)
	assert.Equal(t, UserRoleMember, u.Role)
}

// TestTableUserBeforeSaveRejectsInvalidStatus makes sure the enum guard
// catches typos at write time rather than surfacing as silent filter
// misses later.
func TestTableUserBeforeSaveRejectsInvalidStatus(t *testing.T) {
	u := &TableUser{Email: "a@b.co", Status: "ACTIVE-ish"}
	err := u.BeforeSave(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidUserStatus), "expected errInvalidUserStatus, got %v", err)
}

// TestTableUserBeforeSaveRejectsInvalidRole covers the role enum.
func TestTableUserBeforeSaveRejectsInvalidRole(t *testing.T) {
	u := &TableUser{Email: "a@b.co", Role: "superuser"}
	err := u.BeforeSave(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidUserRole), "expected errInvalidUserRole, got %v", err)
}

// TestTableUserBeforeSaveRejectsEmptyEmail covers the email presence + shape
// checks. Both errors should be returned by the hook so a missing email
// doesn't slip through.
func TestTableUserBeforeSaveRejectsEmptyEmail(t *testing.T) {
	u := &TableUser{Email: ""}
	require.True(t, errors.Is(u.BeforeSave(nil), errEmptyUserEmail))
	u.Email = "no-at-sign"
	require.True(t, errors.Is(u.BeforeSave(nil), errInvalidUserEmail))
}

// TestTableUserSetPasswordAndVerifyPassword exercises the bcrypt round-trip
// via the convenience methods. We don't pin the exact hash because bcrypt
// embeds a random salt; we just check the public methods behave correctly
// for the matching plaintext and reject any other.
func TestTableUserSetPasswordAndVerifyPassword(t *testing.T) {
	u := &TableUser{Email: "u@x.co"}
	require.NoError(t, u.SetPassword("hunter2"))
	require.NotNil(t, u.PasswordHash)
	ok, err := u.VerifyPassword("hunter2")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = u.VerifyPassword("wrong")
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestTableUserVerifyPasswordNilHashFailsClosed guarantees a user with no
// password_hash (pending invite state) cannot be logged in via the verify
// helper, regardless of the supplied plaintext.
func TestTableUserVerifyPasswordNilHashFailsClosed(t *testing.T) {
	u := &TableUser{Email: "u@x.co"}
	ok, err := u.VerifyPassword("anything")
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestTableUserIsActive verifies the single-bit status check that the
// member-login path uses to gate authentication.
func TestTableUserIsActive(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{UserStatusPending, false},
		{UserStatusActive, true},
		{UserStatusDisabled, false},
		{"", false},
	}
	for _, c := range cases {
		u := &TableUser{Status: c.status}
		assert.Equalf(t, c.want, u.IsActive(), "status=%q", c.status)
	}
}

// TestTableTeamMemberBeforeSaveDefaults stamps JoinedAt when missing and
// fills in active status; useful for the create path where the caller
// doesn't always have a clock handy.
func TestTableTeamMemberBeforeSaveDefaults(t *testing.T) {
	m := &TableTeamMember{TeamID: "team-1", UserID: "user-1", RoleInTeam: TeamMemberRoleMember}
	require.NoError(t, m.BeforeSave(nil))
	assert.Equal(t, TeamMemberStatusActive, m.Status)
	assert.False(t, m.JoinedAt.IsZero(), "JoinedAt must be stamped when missing")
	assert.True(t, time.Since(m.JoinedAt) < 5*time.Second)
}

// TestTableTeamMemberBeforeSaveRequiresIDs covers the FK presence checks.
func TestTableTeamMemberBeforeSaveRequiresIDs(t *testing.T) {
	require.True(t, errors.Is((&TableTeamMember{UserID: "u"}).BeforeSave(nil), errEmptyTeamID))
	require.True(t, errors.Is((&TableTeamMember{TeamID: "t"}).BeforeSave(nil), errEmptyUserID))
}

// TestTableTeamMemberBeforeSaveRejectsInvalidRole covers the role enum.
func TestTableTeamMemberBeforeSaveRejectsInvalidRole(t *testing.T) {
	m := &TableTeamMember{TeamID: "t", UserID: "u", RoleInTeam: "viewer"}
	err := m.BeforeSave(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidMembership))
}

// TestTableTeamMemberBeforeSaveRejectsInvalidStatus covers the status enum.
func TestTableTeamMemberBeforeSaveRejectsInvalidStatus(t *testing.T) {
	m := &TableTeamMember{TeamID: "t", UserID: "u", Status: "left"}
	err := m.BeforeSave(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidMemberState))
}

// TestTableTableNames covers the DB table naming. These strings are part of
// the deployment contract (DB migrations + dashboard URLs depend on them);
// renaming would silently break existing installs.
func TestTableTableNames(t *testing.T) {
	assert.Equal(t, "users", TableUser{}.TableName())
	assert.Equal(t, "team_members", TableTeamMember{}.TableName())
}
