package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

// fakeInvitationStore is a hermetic in-memory ConfigStore for the
// invitation-accept tests.
//
// It models transactional semantics for AcceptInvitationTx: the three
// mutations (user upsert, membership upsert, invitation burn) are staged
// and only become visible when the whole batch succeeds. That mirrors what
// the real RDB transaction guarantees, so the handler test asserts the
// handler actually delegates to one atomic call instead of issuing three
// independent writes.
type fakeInvitationStore struct {
	configstore.ConfigStore

	mu           sync.Mutex
	users        map[string]*tables.TableUser // by ID
	usersByMail  map[string]*tables.TableUser
	memberships  map[string]*tables.TableTeamMember
	invitations  map[string]*tables.TableInvitation // by token
	failOnMember bool
	failOnInv    bool
	// Calls records the store methods the handler invoked, in order. The
	// atomicity assertions read this to prove the handler stopped doing
	// three separate writes.
	Calls []string
}

func newFakeInvitationStore() *fakeInvitationStore {
	return &fakeInvitationStore{
		users:       map[string]*tables.TableUser{},
		usersByMail: map[string]*tables.TableUser{},
		memberships: map[string]*tables.TableTeamMember{},
		invitations: map[string]*tables.TableInvitation{},
	}
}

func (f *fakeInvitationStore) record(name string) {
	f.Calls = append(f.Calls, name)
}

func (f *fakeInvitationStore) called(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.Calls {
		if c == name {
			return true
		}
	}
	return false
}

func (f *fakeInvitationStore) seedInvitation(inv *tables.TableInvitation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *inv
	f.invitations[inv.Token] = &cp
}

func (f *fakeInvitationStore) seedUser(u *tables.TableUser) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *u
	f.users[u.ID] = &cp
	f.usersByMail[strings.ToLower(u.Email)] = &cp
}

func (f *fakeInvitationStore) seedMembership(m *tables.TableTeamMember) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *m
	f.memberships[m.TeamID+"|"+m.UserID] = &cp
}

func (f *fakeInvitationStore) userByID(id string) *tables.TableUser {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.users[id]
}

func (f *fakeInvitationStore) membershipCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.memberships)
}

func (f *fakeInvitationStore) invitationByToken(token string) *tables.TableInvitation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.invitations[token]
}

func (f *fakeInvitationStore) GetInvitationByToken(_ context.Context, token string) (*tables.TableInvitation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("GetInvitationByToken")
	inv, ok := f.invitations[token]
	if !ok {
		return nil, nil
	}
	cp := *inv
	return &cp, nil
}

func (f *fakeInvitationStore) GetUserByEmail(_ context.Context, email string) (*tables.TableUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("GetUserByEmail")
	u, ok := f.usersByMail[strings.ToLower(email)]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (f *fakeInvitationStore) GetTeamMembership(_ context.Context, teamID, userID string) (*tables.TableTeamMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("GetTeamMembership")
	m, ok := f.memberships[teamID+"|"+userID]
	if !ok {
		return nil, nil
	}
	cp := *m
	return &cp, nil
}

// AcceptInvitationTx applies all three mutations atomically. Any injected
// failure discards the whole batch, which is exactly the guarantee the
// handler previously lacked.
func (f *fakeInvitationStore) AcceptInvitationTx(ctx context.Context, in configstore.AcceptInvitationInput) (*configstore.AcceptInvitationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("AcceptInvitationTx")

	inv, ok := f.invitations[in.Token]
	if !ok {
		return nil, configstore.ErrInvitationNotFound
	}
	if !inv.IsUsable(in.Now) {
		return nil, configstore.ErrInvitationNotUsable
	}

	user, exists := f.usersByMail[strings.ToLower(inv.Email)]
	if !exists {
		user = &tables.TableUser{
			ID:          "u-new-" + in.Token[:4],
			Email:       strings.ToLower(inv.Email),
			DisplayName: in.DisplayName,
			CreatedAt:   in.Now,
		}
		if user.DisplayName == "" {
			user.DisplayName = user.Email
		}
	}
	hash := in.PasswordHash
	user.PasswordHash = &hash
	user.Status = tables.UserStatusActive
	user.UpdatedAt = in.Now

	member := f.memberships[inv.TeamID+"|"+user.ID]
	if member == nil {
		member = &tables.TableTeamMember{
			ID:         "m-" + user.ID,
			TeamID:     inv.TeamID,
			UserID:     user.ID,
			RoleInTeam: inv.RoleInTeam,
			JoinedAt:   in.Now,
			CreatedAt:  in.Now,
		}
	}
	member.Status = tables.TeamMemberStatusActive
	member.RoleInTeam = inv.RoleInTeam
	member.UpdatedAt = in.Now

	// Injected failures abort before anything is staged-visible.
	if f.failOnMember {
		return nil, errors.New("injected team_members failure")
	}
	if f.failOnInv {
		return nil, errors.New("injected invitation failure")
	}

	inv.MarkAccepted(in.Now)
	inv.UpdatedAt = in.Now
	f.users[user.ID] = user
	f.usersByMail[strings.ToLower(user.Email)] = user
	f.memberships[inv.TeamID+"|"+user.ID] = member

	userOut := *user
	memberOut := *member
	invOut := *inv
	return &configstore.AcceptInvitationOutput{
		User:       &userOut,
		TeamMember: &memberOut,
		Invitation: &invOut,
	}, nil
}

// acceptCtx builds a public (unauthenticated) request context for
// POST /api/invitations/:token/accept with the JSON body attached.
func acceptCtx(token string, body any) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	data, _ := json.Marshal(body)
	ctx.Request.SetBody(data)
	ctx.Request.Header.SetContentType("application/json")
	ctx.SetUserValue("token", token)
	_ = schemas.BifrostContextKeyMemberUserID // keep import used if refactored
	return ctx
}

func usableInvitation(token, teamID, email string) *tables.TableInvitation {
	now := time.Now().UTC()
	return &tables.TableInvitation{
		ID:         "inv-" + token,
		TeamID:     teamID,
		Email:      email,
		Token:      token,
		RoleInTeam: tables.TeamMemberRoleMember,
		Status:     tables.InvitationStatusPending,
		ExpiresAt:  now.Add(tables.DefaultInvitationTTL),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// TestAcceptInvitationRollsBackWhenMembershipWriteFails is the C-3
// regression pin. The pre-fix handler issued CreateUser →
// CreateTeamMember → UpdateInvitation as three independent writes, so a
// failure on the middle step left a live, password-set user row that
// belonged to no team and an invitation still marked pending (re-accept
// would then create a second membership).
//
// The assertion that goes red before the fix: after the injected
// membership failure the user row must NOT exist and the invitation must
// still be pending.
func TestAcceptInvitationRollsBackWhenMembershipWriteFails(t *testing.T) {
	store := newFakeInvitationStore()
	store.seedInvitation(usableInvitation("tok-rollback", "team-7", "alice@example.com"))
	store.failOnMember = true

	h := &InvitationHandler{store: store}
	ctx := acceptCtx("tok-rollback", map[string]any{
		"password": "correct-horse-battery",
	})
	h.acceptInvitation(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusInternalServerError {
		t.Fatalf("want 500, got %d (body=%s)", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	if store.membershipCount() != 0 {
		t.Fatalf("membership must not persist after rollback, got %d rows", store.membershipCount())
	}
	// The load-bearing assertion: no orphan user row.
	if u, err := store.GetUserByEmail(context.Background(), "alice@example.com"); err != nil || u != nil {
		t.Fatalf("user row must be rolled back, got %+v err=%v", u, err)
	}
	inv := store.invitationByToken("tok-rollback")
	if inv.Status != tables.InvitationStatusPending {
		t.Fatalf("invitation must stay pending after rollback, got %q", inv.Status)
	}
}

// TestAcceptInvitationRollsBackWhenInvitationBurnFails covers the third
// step failing: the membership write must not survive either, otherwise a
// retry creates a duplicate membership row.
func TestAcceptInvitationRollsBackWhenInvitationBurnFails(t *testing.T) {
	store := newFakeInvitationStore()
	store.seedInvitation(usableInvitation("tok-burnfail", "team-7", "bob@example.com"))
	store.failOnInv = true

	h := &InvitationHandler{store: store}
	ctx := acceptCtx("tok-burnfail", map[string]any{"password": "correct-horse-battery"})
	h.acceptInvitation(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusInternalServerError {
		t.Fatalf("want 500, got %d", ctx.Response.StatusCode())
	}
	if store.membershipCount() != 0 {
		t.Fatalf("membership must be rolled back, got %d rows", store.membershipCount())
	}
	if u, _ := store.GetUserByEmail(context.Background(), "bob@example.com"); u != nil {
		t.Fatalf("user row must be rolled back, got %+v", u)
	}
}

// TestAcceptInvitationHappyPath is the positive control: with no injected
// failure all three mutations land and the token is burned.
func TestAcceptInvitationHappyPath(t *testing.T) {
	store := newFakeInvitationStore()
	store.seedInvitation(usableInvitation("tok-ok", "team-7", "carol@example.com"))

	h := &InvitationHandler{store: store}
	ctx := acceptCtx("tok-ok", map[string]any{
		"password":     "correct-horse-battery",
		"display_name": "Carol",
	})
	h.acceptInvitation(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Fatalf("want 201, got %d (body=%s)", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	if store.membershipCount() != 1 {
		t.Fatalf("want 1 membership row, got %d", store.membershipCount())
	}
	u, err := store.GetUserByEmail(context.Background(), "carol@example.com")
	if err != nil || u == nil {
		t.Fatalf("user must exist, got %+v err=%v", u, err)
	}
	if u.Status != tables.UserStatusActive {
		t.Fatalf("user must be active, got %q", u.Status)
	}
	if u.DisplayName != "Carol" {
		t.Fatalf("display_name not applied, got %q", u.DisplayName)
	}
	inv := store.invitationByToken("tok-ok")
	if inv.Status != tables.InvitationStatusAccepted {
		t.Fatalf("invitation must be accepted, got %q", inv.Status)
	}
}

// TestAcceptInvitationRejectsExpiredToken pins the 410 path.
func TestAcceptInvitationRejectsExpiredToken(t *testing.T) {
	store := newFakeInvitationStore()
	inv := usableInvitation("tok-expired", "team-7", "dave@example.com")
	inv.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	store.seedInvitation(inv)

	h := &InvitationHandler{store: store}
	ctx := acceptCtx("tok-expired", map[string]any{"password": "correct-horse-battery"})
	h.acceptInvitation(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusGone {
		t.Fatalf("want 410, got %d", ctx.Response.StatusCode())
	}
	if store.membershipCount() != 0 {
		t.Fatalf("no membership must be created for an expired token")
	}
}

// TestAcceptInvitationRequiresPassword pins the 400 path and, more
// importantly, that no write happens when validation fails.
func TestAcceptInvitationRequiresPassword(t *testing.T) {
	store := newFakeInvitationStore()
	store.seedInvitation(usableInvitation("tok-nopw", "team-7", "erin@example.com"))

	h := &InvitationHandler{store: store}
	ctx := acceptCtx("tok-nopw", map[string]any{"password": ""})
	h.acceptInvitation(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Fatalf("want 400, got %d", ctx.Response.StatusCode())
	}
	if store.called("AcceptInvitationTx") {
		t.Fatalf("validation failure must not reach the store")
	}
}

// TestAcceptInvitationUnknownToken pins the 410 path for a token that was
// never issued — the same response as an expired one, so token validity is
// not an oracle.
func TestAcceptInvitationUnknownToken(t *testing.T) {
	store := newFakeInvitationStore()
	h := &InvitationHandler{store: store}
	ctx := acceptCtx("tok-never-existed", map[string]any{"password": "correct-horse-battery"})
	h.acceptInvitation(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusGone {
		t.Fatalf("want 410, got %d", ctx.Response.StatusCode())
	}
}
