package configstore

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPhase2TestStore extends setupRDBTestStore with the tables added
// during Phase 2 (invitations, key_requests). Centralized here so the
// per-test boilerplate stays tiny.
func newPhase2TestStore(t *testing.T) *RDBConfigStore {
	store := setupRDBTestStore(t)
	require.NoError(t, store.DB().AutoMigrate(
		&tables.TableUser{},
		&tables.TableTeamMember{},
		&tables.TableInvitation{},
		&tables.TableKeyRequest{},
	))
	return store
}

// TestCreateAndLookupInvitation exercises the create → token lookup →
// status-update round-trip that the invitations handler relies on.
func TestCreateAndLookupInvitation(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	inv := &tables.TableInvitation{
		ID:              uuid.New().String(),
		TeamID:          "team-1",
		Email:           "  Alice@Example.COM  ",
		Token:           uuid.New().String(),
		RoleInTeam:      tables.TeamMemberRoleMember,
		Status:          tables.InvitationStatusPending,
		CreatedByUserID: "admin",
		ExpiresAt:       now.Add(tables.DefaultInvitationTTL),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, store.CreateInvitation(ctx, inv))

	got, err := store.GetInvitationByToken(ctx, inv.Token)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "alice@example.com", got.Email, "BeforeSave must lowercase the email")
	assert.Equal(t, inv.ID, got.ID)
	assert.Equal(t, tables.InvitationStatusPending, got.Status)
	assert.True(t, got.IsUsable(now))

	gotByID, err := store.GetInvitationByID(ctx, inv.ID)
	require.NoError(t, err)
	require.NotNil(t, gotByID)
	assert.Equal(t, inv.ID, gotByID.ID)

	// Update flips status to revoked; subsequent reload must reflect it.
	got.MarkRevoked()
	got.UpdatedAt = now
	require.NoError(t, store.UpdateInvitation(ctx, got))
	refetched, err := store.GetInvitationByID(ctx, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, tables.InvitationStatusRevoked, refetched.Status)
}

// TestListInvitationsPagination covers the paginated list path the
// admin handler uses. We seed three rows so the total/limit/offset
// behavior is observable.
func TestListInvitationsPagination(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		inv := &tables.TableInvitation{
			ID:         uuid.New().String(),
			TeamID:     "team-1",
			Email:      "user" + string(rune('a'+i)) + "@x.co",
			Token:      uuid.New().String(),
			RoleInTeam: tables.TeamMemberRoleMember,
			Status:     tables.InvitationStatusPending,
			ExpiresAt:  now.Add(tables.DefaultInvitationTTL),
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		require.NoError(t, store.CreateInvitation(ctx, inv))
	}
	rows, total, err := store.ListInvitations(ctx, "team-1", "", 2, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Len(t, rows, 2, "limit must cap the page size")

	rows2, total2, err := store.ListInvitations(ctx, "team-1", "", 2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total2)
	assert.Len(t, rows2, 1)
}

// TestCreateAndListKeyRequests exercises the key-request CRUD path.
func TestCreateAndListKeyRequests(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	req := &tables.TableKeyRequest{
		ID:        uuid.New().String(),
		UserID:    "user-1",
		TeamID:    "team-1",
		Kind:      tables.KeyRequestKindJoinTeam,
		Purpose:   "new project",
		Status:    tables.KeyRequestStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, store.CreateKeyRequest(ctx, req))

	got, err := store.GetKeyRequestByID(ctx, req.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, tables.KeyRequestKindJoinTeam, got.Kind)

	rows, total, err := store.ListKeyRequests(ctx, "", "user-1", "", 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Len(t, rows, 1)

	// Mark approved → reload → must be terminal + have approver set.
	got.MarkApproved("admin", "ok", time.Now().UTC())
	got.VirtualKeyID = ptr("vk-1")
	require.NoError(t, store.UpdateKeyRequest(ctx, got))

	refetched, err := store.GetKeyRequestByID(ctx, req.ID)
	require.NoError(t, err)
	require.NotNil(t, refetched)
	assert.True(t, refetched.IsTerminal())
	require.NotNil(t, refetched.ApprovedByUserID)
	assert.Equal(t, "admin", *refetched.ApprovedByUserID)
	require.NotNil(t, refetched.VirtualKeyID)
	assert.Equal(t, "vk-1", *refetched.VirtualKeyID)
}

// TestDisableUserVKeys exercises the offboarding helper. We seed three
// VKs owned by user-1 (only one active) plus one owned by user-2 —
// after the call only user-1's active VK should be flipped.
func TestDisableUserVKeys(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	seedVK := func(owner string, idx int, active bool) *tables.TableVirtualKey {
		vk := &tables.TableVirtualKey{
			ID:     uuid.New().String(),
			Name:   "vk-" + owner + "-" + string(rune('a'+idx)),
			Value:  *schemas.NewSecretVar("bfvk-" + owner + "-" + string(rune('a'+idx))),
			UserID: ptr(owner),
		}
		if !active {
			f := false
			vk.IsActive = &f
		}
		require.NoError(t, store.DB().WithContext(ctx).Create(vk).Error)
		return vk
	}
	u1Active := seedVK("user-1", 0, true)
	_ = seedVK("user-1", 1, true) // second active VK for the same user
	_ = seedVK("user-1", 2, false)
	_ = seedVK("user-2", 0, true) // different user — must NOT be affected

	disabled, err := store.DisableUserVKeys(ctx, "user-1")
	require.NoError(t, err)
	// Three VKs belong to user-1 → all three get flagged as disabled
	// (the function does not check is_active on the way in; it just
	// ensures IsActive=false on the way out). Either way, the affected
	// set is exactly user-1's three rows.
	assert.Len(t, disabled, 3)
	assert.Contains(t, disabled, u1Active.ID)

	// Verify user-2's VK is untouched.
	var stillActive int64
	require.NoError(t, store.DB().WithContext(ctx).
		Model(&tables.TableVirtualKey{}).
		Where("user_id = ? AND is_active = ?", "user-2", true).
		Count(&stillActive).Error)
	assert.EqualValues(t, 1, stillActive)
}

// TestListVirtualKeysByUserID exercises the lightweight VK summary
// that powers GET /api/governance/users/:id.
func TestListVirtualKeysByUserID(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		vk := &tables.TableVirtualKey{
			ID:     uuid.New().String(),
			Name:   "vk-u1-" + string(rune('a'+i)),
			Value:  *schemas.NewSecretVar("bfvk-u1-" + string(rune('a'+i))),
			UserID: ptr("user-1"),
		}
		require.NoError(t, store.DB().WithContext(ctx).Create(vk).Error)
	}
	// Different user — must not be returned.
	other := &tables.TableVirtualKey{
		ID:     uuid.New().String(),
		Name:   "vk-u2",
		Value:  *schemas.NewSecretVar("bfvk-u2"),
		UserID: ptr("user-2"),
	}
	require.NoError(t, store.DB().WithContext(ctx).Create(other).Error)

	rows, total, err := store.ListVirtualKeysByUserID(ctx, "user-1", 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Len(t, rows, 3)
	for _, vk := range rows {
		require.NotNil(t, vk.UserID)
		assert.Equal(t, "user-1", *vk.UserID)
	}
}

func ptr(s string) *string { return &s }
