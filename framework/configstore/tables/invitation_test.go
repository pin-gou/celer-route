package tables

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTableInvitationBeforeSaveDefaults verifies the hook fills in
// status='pending' and stamps a default 7-day expiry when the caller
// omits them — matches the design contract in
// temp/team/01-identity/api.md §4.
func TestTableInvitationBeforeSaveDefaults(t *testing.T) {
	inv := &TableInvitation{
		TeamID: "team-1",
		Email:  "  Alice@Example.COM  ",
		Token:  "tok-1",
	}
	require.NoError(t, inv.BeforeSave(nil))
	assert.Equal(t, "alice@example.com", inv.Email)
	assert.Equal(t, InvitationStatusPending, inv.Status)
	assert.Equal(t, TeamMemberRoleMember, inv.RoleInTeam)
	assert.WithinDuration(t, time.Now().Add(DefaultInvitationTTL), inv.ExpiresAt, 5*time.Second)
}

// TestTableInvitationBeforeSaveRejectsInvalidStatus covers the enum
// guard. The hook must reject any value outside the 5-state set so a
// typo can't bypass the accept/revoke state machine.
func TestTableInvitationBeforeSaveRejectsInvalidStatus(t *testing.T) {
	inv := &TableInvitation{TeamID: "t", Email: "a@b.co", Token: "x", Status: "unknown"}
	err := inv.BeforeSave(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidInvitationStatus))
}

// TestTableInvitationBeforeSaveRejectsInvalidRole covers the role enum.
// Invitations only carry admin/member — owner is reserved for out-of-band
// provisioning (see Readme D8).
func TestTableInvitationBeforeSaveRejectsInvalidRole(t *testing.T) {
	inv := &TableInvitation{TeamID: "t", Email: "a@b.co", Token: "x", RoleInTeam: "owner"}
	err := inv.BeforeSave(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidInvitationRole))
}

// TestTableInvitationBeforeSaveRejectsMissingTeam verifies the FK
// presence check.
func TestTableInvitationBeforeSaveRejectsMissingTeam(t *testing.T) {
	inv := &TableInvitation{Email: "a@b.co", Token: "x"}
	require.True(t, errors.Is(inv.BeforeSave(nil), errEmptyInvitationTeam))
}

// TestTableInvitationBeforeSaveRejectsEmptyToken covers the token
// presence check. Without a token the row is useless — the accept
// endpoint can never resolve it.
func TestTableInvitationBeforeSaveRejectsEmptyToken(t *testing.T) {
	inv := &TableInvitation{TeamID: "t", Email: "a@b.co"}
	require.True(t, errors.Is(inv.BeforeSave(nil), errEmptyInvitationToken))
}

// TestTableInvitationIsUsable covers the accept-time clock check.
// Pending + unexpired → usable; pending + expired → not; accepted /
// revoked → never usable.
func TestTableInvitationIsUsable(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name string
		inv  *TableInvitation
		want bool
	}{
		{
			name: "pending unexpired",
			inv:  &TableInvitation{Status: InvitationStatusPending, ExpiresAt: now.Add(time.Hour)},
			want: true,
		},
		{
			name: "pending expired",
			inv:  &TableInvitation{Status: InvitationStatusPending, ExpiresAt: now.Add(-time.Hour)},
			want: false,
		},
		{
			name: "accepted",
			inv:  &TableInvitation{Status: InvitationStatusAccepted, ExpiresAt: now.Add(time.Hour)},
			want: false,
		},
		{
			name: "revoked",
			inv:  &TableInvitation{Status: InvitationStatusRevoked, ExpiresAt: now.Add(time.Hour)},
			want: false,
		},
		{
			name: "nil",
			inv:  nil,
			want: false,
		},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, c.inv.IsUsable(now), "%s", c.name)
	}
}

// TestTableKeyRequestBeforeSaveDefaults verifies kind defaults to no
// specific value (we always require it), but status defaults to
// 'pending'. The kinds list comes from the design contract — see
// data-model.md §4.
func TestTableKeyRequestBeforeSaveDefaults(t *testing.T) {
	req := &TableKeyRequest{
		UserID:  "u-1",
		TeamID:  "t-1",
		Kind:    KeyRequestKindJoinTeam,
		Purpose: "  new project  ",
	}
	require.NoError(t, req.BeforeSave(nil))
	assert.Equal(t, "new project", req.Purpose)
	assert.Equal(t, KeyRequestStatusPending, req.Status)
}

// TestTableKeyRequestBeforeSaveRejectsMissingFields exercises the
// required-field set: user_id, team_id, kind, purpose.
func TestTableKeyRequestBeforeSaveRejectsMissingFields(t *testing.T) {
	cases := []struct {
		name string
		req  *TableKeyRequest
		want error
	}{
		{"missing user", &TableKeyRequest{TeamID: "t", Kind: KeyRequestKindJoinTeam, Purpose: "p"}, errEmptyKeyRequestUser},
		{"missing team", &TableKeyRequest{UserID: "u", Kind: KeyRequestKindJoinTeam, Purpose: "p"}, errEmptyKeyRequestTeam},
		{"missing kind", &TableKeyRequest{UserID: "u", TeamID: "t", Purpose: "p"}, errEmptyKeyRequestKind},
		{"missing purpose", &TableKeyRequest{UserID: "u", TeamID: "t", Kind: KeyRequestKindJoinTeam}, errEmptyKeyRequestPurpose},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.True(t, errors.Is(c.req.BeforeSave(nil), c.want))
		})
	}
}

// TestTableKeyRequestBeforeSaveRejectsInvalidKind covers the kind enum.
func TestTableKeyRequestBeforeSaveRejectsInvalidKind(t *testing.T) {
	req := &TableKeyRequest{UserID: "u", TeamID: "t", Kind: "rotate_vk", Purpose: "p"}
	err := req.BeforeSave(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidKeyRequestKind))
}

// TestTableKeyRequestBeforeSaveRejectsInvalidStatus covers the status
// enum (pending/approved/rejected/cancelled).
func TestTableKeyRequestBeforeSaveRejectsInvalidStatus(t *testing.T) {
	req := &TableKeyRequest{UserID: "u", TeamID: "t", Kind: KeyRequestKindJoinTeam, Purpose: "p", Status: "fulfilled"}
	err := req.BeforeSave(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidKeyRequestStatus))
}

// TestTableKeyRequestIsTerminal covers the helper that the approve /
// reject endpoints use to guard against double-decision.
func TestTableKeyRequestIsTerminal(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{KeyRequestStatusPending, false},
		{KeyRequestStatusCancelled, false},
		{KeyRequestStatusApproved, true},
		{KeyRequestStatusRejected, true},
	}
	for _, c := range cases {
		req := &TableKeyRequest{Status: c.status}
		assert.Equalf(t, c.want, req.IsTerminal(), "status=%q", c.status)
	}
	assert.True(t, (*TableKeyRequest)(nil).IsTerminal(), "nil receiver is treated as terminal (no further action possible)")
}

// TestTableNames covers the DB table naming for the two new tables.
// Strings are part of the deployment contract — see migrations.go.
func TestPhase2TableNames(t *testing.T) {
	assert.Equal(t, "invitations", TableInvitation{}.TableName())
	assert.Equal(t, "key_requests", TableKeyRequest{}.TableName())
}
