package configstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedInvitationTxTest inserts one usable pending invitation and returns it.
func seedInvitationTxTest(t *testing.T, store *RDBConfigStore, token, teamID, email string) *tables.TableInvitation {
	t.Helper()
	now := time.Now().UTC()
	inv := &tables.TableInvitation{
		ID:              uuid.New().String(),
		TeamID:          teamID,
		Email:           email,
		Token:           token,
		RoleInTeam:      tables.TeamMemberRoleMember,
		Status:          tables.InvitationStatusPending,
		CreatedByUserID: "admin",
		ExpiresAt:       now.Add(tables.DefaultInvitationTTL),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, store.CreateInvitation(context.Background(), inv))
	return inv
}

// injectCreateFailureOn registers a GORM callback that aborts any INSERT
// against the named table, simulating the mid-flight DB failure the C-3 fix
// has to survive. Returns a cleanup func so the callback can't leak into
// other tests in the package.
//
// The callback is anchored before gorm:create so the abort happens while the
// enclosing transaction is still open and therefore still rollback-able.
func injectCreateFailureOn(t *testing.T, store *RDBConfigStore, table, callbackName string) func() {
	t.Helper()
	cb := store.DB().Callback().Create()
	require.NoError(t, cb.Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == table {
			_ = tx.AddError(errors.New("injected failure on " + table))
		}
	}))
	return func() { _ = cb.Remove(callbackName) }
}

// injectUpdateFailureOn is the UPDATE-side counterpart of
// injectCreateFailureOn, used to fail the final "burn the token" step.
func injectUpdateFailureOn(t *testing.T, store *RDBConfigStore, table, callbackName string) func() {
	t.Helper()
	cb := store.DB().Callback().Update()
	require.NoError(t, cb.Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == table {
			_ = tx.AddError(errors.New("injected failure on " + table))
		}
	}))
	return func() { _ = cb.Remove(callbackName) }
}

// TestAcceptInvitationTxHappyPath is the positive control: all three
// mutations land in one commit and the returned copies reflect them.
func TestAcceptInvitationTxHappyPath(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	seedInvitationTxTest(t, store, "tok-tx-ok", "team-1", "Alice@Example.com")

	out, err := store.AcceptInvitationTx(ctx, AcceptInvitationInput{
		Token:        "tok-tx-ok",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuv",
		DisplayName:  "Alice",
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, tables.UserStatusActive, out.User.Status)
	assert.Equal(t, "alice@example.com", out.User.Email, "email must be normalized")
	assert.Equal(t, "Alice", out.User.DisplayName)
	require.NotNil(t, out.User.PasswordHash)
	assert.Equal(t, tables.TeamMemberStatusActive, out.TeamMember.Status)
	assert.Equal(t, "team-1", out.TeamMember.TeamID)
	assert.Equal(t, tables.InvitationStatusAccepted, out.Invitation.Status)

	// Persisted state matches the returned copies.
	gotUser, err := store.GetUserByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	require.NotNil(t, gotUser)
	assert.Equal(t, out.User.ID, gotUser.ID)

	gotMember, err := store.GetTeamMembership(ctx, "team-1", out.User.ID)
	require.NoError(t, err)
	require.NotNil(t, gotMember)
	assert.Equal(t, tables.TeamMemberStatusActive, gotMember.Status)
}

// TestAcceptInvitationTxRollsBackOnMembershipFailure is the C-3 regression
// pin at the store layer. It injects a hard failure on the team_members
// INSERT and asserts that the user row written one step earlier is rolled
// back rather than left behind as an orphan.
//
// Pre-fix behaviour this replaces: three independent statements, so the user
// row survived with status=active and a set password while belonging to no
// team, and the invitation stayed pending — a retry then created a second
// membership.
func TestAcceptInvitationTxRollsBackOnMembershipFailure(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	seedInvitationTxTest(t, store, "tok-tx-rollback", "team-1", "bob@example.com")

	cleanup := injectCreateFailureOn(t, store, "team_members", "fail_team_members")
	defer cleanup()

	out, err := store.AcceptInvitationTx(ctx, AcceptInvitationInput{
		Token:        "tok-tx-rollback",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuv",
		DisplayName:  "Bob",
		Now:          time.Now().UTC(),
	})
	require.Error(t, err, "the injected failure must surface to the caller")
	// Assert the SURFACED error is the injected one. Without this the
	// rollback assertions below could pass vacuously — e.g. if the call
	// bailed out earlier for an unrelated schema reason, nothing would have
	// been written and the "rolled back" counts would be zero anyway.
	assert.Contains(t, err.Error(), "injected failure on team_members")
	assert.Nil(t, out)

	// Load-bearing: no orphan user row.
	var userCount int64
	require.NoError(t, store.DB().WithContext(ctx).Model(&tables.TableUser{}).
		Where("email = ?", "bob@example.com").Count(&userCount).Error)
	assert.Zero(t, userCount, "user row must be rolled back when the membership write fails")

	var memberCount int64
	require.NoError(t, store.DB().WithContext(ctx).Model(&tables.TableTeamMember{}).Count(&memberCount).Error)
	assert.Zero(t, memberCount)

	// The invitation must still be pending so the invitee can retry once the
	// underlying DB problem is resolved.
	inv, err := store.GetInvitationByToken(ctx, "tok-tx-rollback")
	require.NoError(t, err)
	require.NotNil(t, inv)
	assert.Equal(t, tables.InvitationStatusPending, inv.Status)
	assert.Nil(t, inv.AcceptedAt)
}

// TestAcceptInvitationTxRollsBackOnInvitationBurnFailure covers the last step
// failing: neither the user nor the membership may survive, otherwise a retry
// duplicates the membership row.
func TestAcceptInvitationTxRollsBackOnInvitationBurnFailure(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	seedInvitationTxTest(t, store, "tok-tx-burn", "team-1", "carol@example.com")

	cleanup := injectUpdateFailureOn(t, store, "invitations", "fail_invitations")
	defer cleanup()

	out, err := store.AcceptInvitationTx(ctx, AcceptInvitationInput{
		Token:        "tok-tx-burn",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuv",
		Now:          time.Now().UTC(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected failure on invitations")
	assert.Nil(t, out)

	var userCount int64
	require.NoError(t, store.DB().WithContext(ctx).Model(&tables.TableUser{}).Count(&userCount).Error)
	assert.Zero(t, userCount, "user must be rolled back when the token burn fails")

	var memberCount int64
	require.NoError(t, store.DB().WithContext(ctx).Model(&tables.TableTeamMember{}).Count(&memberCount).Error)
	assert.Zero(t, memberCount, "membership must be rolled back when the token burn fails")
}

// TestAcceptInvitationTxSingleUse pins that a spent token cannot be accepted
// twice — the second call returns ErrInvitationNotUsable and creates no
// additional membership row.
func TestAcceptInvitationTxSingleUse(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	seedInvitationTxTest(t, store, "tok-tx-once", "team-1", "dave@example.com")

	in := AcceptInvitationInput{
		Token:        "tok-tx-once",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuv",
		Now:          time.Now().UTC(),
	}
	first, err := store.AcceptInvitationTx(ctx, in)
	require.NoError(t, err)
	require.NotNil(t, first)

	_, err = store.AcceptInvitationTx(ctx, in)
	require.ErrorIs(t, err, ErrInvitationNotUsable, "a spent token must be refused")

	var memberCount int64
	require.NoError(t, store.DB().WithContext(ctx).Model(&tables.TableTeamMember{}).
		Where("user_id = ?", first.User.ID).Count(&memberCount).Error)
	assert.EqualValues(t, 1, memberCount, "double-accept must not create a second membership")
}

// TestAcceptInvitationTxUnknownToken confirms the not-found sentinel so the
// handler can map it to 410 rather than 500.
func TestAcceptInvitationTxUnknownToken(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()

	_, err := store.AcceptInvitationTx(ctx, AcceptInvitationInput{
		Token:        "tok-never-issued",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuv",
		Now:          time.Now().UTC(),
	})
	require.ErrorIs(t, err, ErrInvitationNotFound)
}

// TestAcceptInvitationTxExpiredToken covers the expiry branch separately from
// the unknown-token branch — same HTTP mapping, different sentinel.
func TestAcceptInvitationTxExpiredToken(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	inv := seedInvitationTxTest(t, store, "tok-tx-expired", "team-1", "erin@example.com")

	inv.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	inv.UpdatedAt = time.Now().UTC()
	require.NoError(t, store.UpdateInvitation(ctx, inv))

	_, err := store.AcceptInvitationTx(ctx, AcceptInvitationInput{
		Token:        "tok-tx-expired",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuv",
		Now:          time.Now().UTC(),
	})
	require.ErrorIs(t, err, ErrInvitationNotUsable)

	var userCount int64
	require.NoError(t, store.DB().WithContext(ctx).Model(&tables.TableUser{}).Count(&userCount).Error)
	assert.Zero(t, userCount, "an expired token must not provision a user")
}

// TestAcceptInvitationTxReactivatesRemovedMember covers the upsert branch: a
// previously-removed member accepting a fresh invitation is promoted back to
// active on the SAME row, not duplicated.
func TestAcceptInvitationTxReactivatesRemovedMember(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Pre-existing user + removed membership.
	user := &tables.TableUser{
		ID:          uuid.New().String(),
		Email:       "frank@example.com",
		DisplayName: "Frank",
		Status:      tables.UserStatusDisabled,
		Role:        tables.UserRoleMember,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, store.CreateUser(ctx, user))
	require.NoError(t, store.CreateTeamMember(ctx, &tables.TableTeamMember{
		ID:         uuid.New().String(),
		TeamID:     "team-1",
		UserID:     user.ID,
		RoleInTeam: tables.TeamMemberRoleMember,
		Status:     tables.TeamMemberStatusRemoved,
		JoinedAt:   now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}))
	seedInvitationTxTest(t, store, "tok-tx-reactivate", "team-1", "frank@example.com")

	out, err := store.AcceptInvitationTx(ctx, AcceptInvitationInput{
		Token:        "tok-tx-reactivate",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuv",
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, user.ID, out.User.ID, "must reuse the existing user row, not create a new one")
	assert.Equal(t, tables.UserStatusActive, out.User.Status)
	assert.Equal(t, tables.TeamMemberStatusActive, out.TeamMember.Status)

	var memberCount int64
	require.NoError(t, store.DB().WithContext(ctx).Model(&tables.TableTeamMember{}).
		Where("team_id = ? AND user_id = ?", "team-1", user.ID).Count(&memberCount).Error)
	assert.EqualValues(t, 1, memberCount, "reactivation must not duplicate the membership row")
}

// TestAcceptInvitationTxRejectsEmptyHash guards the invariant that keeps
// bcrypt out of the transaction: the caller must supply a hash.
func TestAcceptInvitationTxRejectsEmptyHash(t *testing.T) {
	store := newPhase2TestStore(t)
	ctx := context.Background()
	seedInvitationTxTest(t, store, "tok-tx-nohash", "team-1", "gina@example.com")

	_, err := store.AcceptInvitationTx(ctx, AcceptInvitationInput{
		Token: "tok-tx-nohash",
		Now:   time.Now().UTC(),
	})
	require.Error(t, err)

	// The invitation must be untouched — validation happens before the tx.
	inv, err := store.GetInvitationByToken(ctx, "tok-tx-nohash")
	require.NoError(t, err)
	require.NotNil(t, inv)
	assert.Equal(t, tables.InvitationStatusPending, inv.Status)
}
