package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

// fakeKeyRequestStore is a hermetic in-memory store for the key-request
// handler tests. It implements just enough of configstore.ConfigStore for
// the four endpoints (submit, listMine, listAll, approve, reject) and the
// team-membership lookup the C-2 fix needs.
type fakeKeyRequestStore struct {
	configstore.ConfigStore

	mu          sync.Mutex
	memberships map[string]*tables.TableTeamMember // keyed by team_id+":"+user_id
	keyReqs     map[string]*tables.TableKeyRequest
	usersByID   map[string]*tables.TableUser
	usersByMail map[string]*tables.TableUser
}

func newFakeKeyRequestStore() *fakeKeyRequestStore {
	return &fakeKeyRequestStore{
		memberships: map[string]*tables.TableTeamMember{},
		keyReqs:     map[string]*tables.TableKeyRequest{},
		usersByID:   map[string]*tables.TableUser{},
		usersByMail: map[string]*tables.TableUser{},
	}
}

func (f *fakeKeyRequestStore) seedUser(u *tables.TableUser) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usersByID[u.ID] = u
	f.usersByMail[u.Email] = u
}

func (f *fakeKeyRequestStore) seedMembership(m *tables.TableTeamMember) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := m.TeamID + "|" + m.UserID
	f.memberships[key] = m
}

func (f *fakeKeyRequestStore) GetUserByID(_ context.Context, id string) (*tables.TableUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.usersByID[id], nil
}

func (f *fakeKeyRequestStore) GetUserByEmail(_ context.Context, email string) (*tables.TableUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.usersByMail[email], nil
}

func (f *fakeKeyRequestStore) GetTeamMembership(_ context.Context, teamID, userID string) (*tables.TableTeamMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.memberships[teamID+"|"+userID], nil
}

func (f *fakeKeyRequestStore) CreateTeamMember(_ context.Context, m *tables.TableTeamMember) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := m.TeamID + "|" + m.UserID
	if _, ok := f.memberships[key]; ok {
		return errors.New("duplicate membership")
	}
	f.memberships[key] = m
	return nil
}

func (f *fakeKeyRequestStore) CreateKeyRequest(_ context.Context, r *tables.TableKeyRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.keyReqs[r.ID]; ok {
		return errors.New("duplicate key request")
	}
	cp := *r
	f.keyReqs[r.ID] = &cp
	return nil
}

func (f *fakeKeyRequestStore) GetKeyRequestByID(_ context.Context, id string) (*tables.TableKeyRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.keyReqs[id], nil
}

func (f *fakeKeyRequestStore) UpdateKeyRequest(_ context.Context, r *tables.TableKeyRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *r
	f.keyReqs[r.ID] = &cp
	return nil
}

func (f *fakeKeyRequestStore) ListKeyRequests(_ context.Context, status, userID, teamID string, limit, offset int) ([]tables.TableKeyRequest, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []tables.TableKeyRequest
	for _, r := range f.keyReqs {
		if status != "" && r.Status != status {
			continue
		}
		if userID != "" && r.UserID != userID {
			continue
		}
		if teamID != "" && r.TeamID != teamID {
			continue
		}
		out = append(out, *r)
	}
	return out, int64(len(out)), nil
}

func (f *fakeKeyRequestStore) UpdateTeamMember(_ context.Context, m *tables.TableTeamMember) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *m
	f.memberships[m.TeamID+"|"+m.UserID] = &cp
	return nil
}

// submitCtx builds a member-authed request context with the JSON body
// already attached and the memberUserID stashed where the middleware
// would have put it.
func submitCtx(body any, userID string) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	data, _ := json.Marshal(body)
	ctx.Request.SetBody(data)
	ctx.Request.Header.SetContentType("application/json")
	ctx.SetUserValue(schemas.BifrostContextKeyMemberUserID, userID)
	return ctx
}

// TestKeyRequestSubmitJoinTeamRefusesIfAlreadyMember covers the C-2 IDOR
// fix path: a member must NOT be able to file a join_team request for a
// team they already belong to. The bug being pinned: the prior submit()
// accepted the row regardless of membership, which let any member
// create a phantom "join" ticket and (worse) lean on the approval auto-
// side-effect to fabricate membership in teams they were never invited to.
//
// The fix flips the rule: join_team requires that the caller is NOT
// already an active member (the legitimate way in is the invitation
// flow, not a self-service request).
func TestKeyRequestSubmitJoinTeamRefusesIfAlreadyMember(t *testing.T) {
	store := newFakeKeyRequestStore()
	u := &tables.TableUser{ID: "u-1", Email: "a@x.com", Role: tables.UserRoleMember, Status: tables.UserStatusActive}
	store.seedUser(u)
	// Active membership for team-7: the caller should be refused.
	store.seedMembership(&tables.TableTeamMember{
		ID: "m-1", TeamID: "team-7", UserID: "u-1",
		RoleInTeam: tables.TeamMemberRoleMember, Status: tables.TeamMemberStatusActive,
		JoinedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	h := &KeyRequestHandler{store: store}
	ctx := submitCtx(map[string]any{
		"kind":    tables.KeyRequestKindJoinTeam,
		"purpose": "want to join team-7",
		"team_id": "team-7",
	}, "u-1")
	h.submit(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusConflict {
		t.Fatalf("want 409, got %d", ctx.Response.StatusCode())
	}
}

// TestKeyRequestSubmitExtendQuotaRequiresMembership covers the second
// half of the C-2 fix: extend_quota / add_vk requests are only
// legitimate if the caller is already an active member of the target
// team. A member must not be able to file quota-extension or new-VK
// requests against teams they have no relationship with.
func TestKeyRequestSubmitExtendQuotaRequiresMembership(t *testing.T) {
	store := newFakeKeyRequestStore()
	u := &tables.TableUser{ID: "u-1", Email: "a@x.com", Role: tables.UserRoleMember, Status: tables.UserStatusActive}
	store.seedUser(u)
	// No membership for team-7 — caller should be refused.
	h := &KeyRequestHandler{store: store}
	ctx := submitCtx(map[string]any{
		"kind":    tables.KeyRequestKindExtendQuota,
		"purpose": "raise my quota",
		"team_id": "team-7",
	}, "u-1")
	h.submit(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("want 403, got %d", ctx.Response.StatusCode())
	}
}

// TestKeyRequestSubmitAddVKRequiresMembership mirrors ExtendQuota for
// the add_vk kind — same authorization rule, different kind label so the
// regression is pinned for both shapes independently.
func TestKeyRequestSubmitAddVKRequiresMembership(t *testing.T) {
	store := newFakeKeyRequestStore()
	u := &tables.TableUser{ID: "u-1", Email: "a@x.com", Role: tables.UserRoleMember, Status: tables.UserStatusActive}
	store.seedUser(u)
	h := &KeyRequestHandler{store: store}
	ctx := submitCtx(map[string]any{
		"kind":    tables.KeyRequestKindAddVK,
		"purpose": "I need a key",
		"team_id": "team-7",
	}, "u-1")
	h.submit(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("want 403, got %d", ctx.Response.StatusCode())
	}
}

// TestKeyRequestSubmitJoinTeamAllowsNonMember is the happy-path side of
// the C-2 fix: a non-member may file a join_team request. This guards
// against an over-correction that rejects every request.
func TestKeyRequestSubmitJoinTeamAllowsNonMember(t *testing.T) {
	store := newFakeKeyRequestStore()
	u := &tables.TableUser{ID: "u-1", Email: "a@x.com", Role: tables.UserRoleMember, Status: tables.UserStatusActive}
	store.seedUser(u)
	h := &KeyRequestHandler{store: store}
	ctx := submitCtx(map[string]any{
		"kind":    tables.KeyRequestKindJoinTeam,
		"purpose": "want to join team-7",
		"team_id": "team-7",
	}, "u-1")
	h.submit(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Fatalf("want 201, got %d (body=%s)", ctx.Response.StatusCode(), ctx.Response.Body())
	}
}

// TestKeyRequestSubmitExtendQuotaAllowsMember is the symmetric happy
// path for an in-team member filing a quota-extension request.
func TestKeyRequestSubmitExtendQuotaAllowsMember(t *testing.T) {
	store := newFakeKeyRequestStore()
	u := &tables.TableUser{ID: "u-1", Email: "a@x.com", Role: tables.UserRoleMember, Status: tables.UserStatusActive}
	store.seedUser(u)
	store.seedMembership(&tables.TableTeamMember{
		ID: "m-1", TeamID: "team-7", UserID: "u-1",
		RoleInTeam: tables.TeamMemberRoleMember, Status: tables.TeamMemberStatusActive,
		JoinedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	h := &KeyRequestHandler{store: store}
	ctx := submitCtx(map[string]any{
		"kind":    tables.KeyRequestKindExtendQuota,
		"purpose": "raise my quota",
		"team_id": "team-7",
	}, "u-1")
	h.submit(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Fatalf("want 201, got %d (body=%s)", ctx.Response.StatusCode(), ctx.Response.Body())
	}
}

// TestKeyRequestSubmitJoinTeamRefusesPendingMembership is a narrow
// edge-case pin: a member who was previously invited (status=invited)
// but never accepted must NOT file a fresh join_team request either —
// the legitimate path is still the invitation accept flow. Pending is
// not "active", so the same rule as non-member applies.
func TestKeyRequestSubmitJoinTeamRefusesPendingMembership(t *testing.T) {
	store := newFakeKeyRequestStore()
	u := &tables.TableUser{ID: "u-1", Email: "a@x.com", Role: tables.UserRoleMember, Status: tables.UserStatusActive}
	store.seedUser(u)
	store.seedMembership(&tables.TableTeamMember{
		ID: "m-1", TeamID: "team-7", UserID: "u-1",
		RoleInTeam: tables.TeamMemberRoleMember, Status: tables.TeamMemberStatusInvited,
		JoinedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	h := &KeyRequestHandler{store: store}
	ctx := submitCtx(map[string]any{
		"kind":    tables.KeyRequestKindJoinTeam,
		"purpose": "want to join team-7",
		"team_id": "team-7",
	}, "u-1")
	h.submit(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusConflict {
		t.Fatalf("want 409, got %d", ctx.Response.StatusCode())
	}
}
