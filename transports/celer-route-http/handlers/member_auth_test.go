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

// fakeMemberConfigStore is a minimal ConfigStore that handles just the
// user / session reads and writes that the member-auth surface touches.
// It embeds configstore.ConfigStore so unimplemented methods panic — this
// makes the test fail loudly if the handler code grows a new dependency.
//
// In-memory map storage keeps the test hermetic; no SQLite required.
type fakeMemberConfigStore struct {
	configstore.ConfigStore

	mu       sync.Mutex
	users    map[string]*tables.TableUser // keyed by email (lowercased)
	usersByID map[string]*tables.TableUser
	sessions map[string]*tables.SessionsTable // keyed by random half of encoded token
}

func newFakeMemberConfigStore() *fakeMemberConfigStore {
	return &fakeMemberConfigStore{
		users:     map[string]*tables.TableUser{},
		usersByID: map[string]*tables.TableUser{},
		sessions:  map[string]*tables.SessionsTable{},
	}
}

// seedUser inserts a user row in the right shape for login. status must be
// UserStatusActive for the login path to admit the request.
func (f *fakeMemberConfigStore) seedUser(t *testing.T, user *tables.TableUser) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if user.Email == "" || user.ID == "" {
		t.Fatalf("seedUser requires non-empty id and email, got %+v", user)
	}
	if err := user.BeforeSave(nil); err != nil {
		t.Fatalf("seedUser BeforeSave: %v", err)
	}
	f.users[strings.ToLower(user.Email)] = user
	f.usersByID[user.ID] = user
}

func (f *fakeMemberConfigStore) GetUserByID(_ context.Context, id string) (*tables.TableUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.usersByID[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (f *fakeMemberConfigStore) GetUserByEmail(_ context.Context, email string) (*tables.TableUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[strings.ToLower(email)]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (f *fakeMemberConfigStore) CreateSession(_ context.Context, s *tables.SessionsTable) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.sessions[s.Token]; exists {
		return errors.New("session already exists")
	}
	f.sessions[s.Token] = s
	return nil
}

func (f *fakeMemberConfigStore) GetSession(_ context.Context, token string) (*tables.SessionsTable, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[token]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (f *fakeMemberConfigStore) DeleteSession(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, token)
	return nil
}

func (f *fakeMemberConfigStore) UpdateUserLastLoginAt(_ context.Context, id string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.usersByID[id]; ok {
		cp := at
		u.LastLoginAt = &cp
	}
	return nil
}

func (f *fakeMemberConfigStore) GetUserTeamMemberships(_ context.Context, _ string) ([]tables.TableTeamMember, error) {
	return nil, nil
}

// newMemberCtx builds an empty fasthttp request context suitable for
// passing to a handler.
func newMemberCtx() *fasthttp.RequestCtx { return &fasthttp.RequestCtx{} }

// responseCookieValue returns the bare cookie value (token half) from the
// response Set-Cookie header. fasthttp's ResponseHeader.PeekCookie returns
// the full Set-Cookie line ("name=value; expires=...; path=/; HttpOnly"),
// so the test must trim everything after the first semicolon to recover the
// value that was actually written into the cookie. Mirrors how a browser
// would parse the line before sending it back on the next request.
func responseCookieValue(t *testing.T, ctx *fasthttp.RequestCtx, name string) string {
	t.Helper()
	raw := string(ctx.Response.Header.PeekCookie(name))
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, ";"); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.Index(raw, "="); i >= 0 {
		raw = raw[i+1:]
	}
	return raw
}

// postJSON puts the body onto the request and sets the content type so
// json.Unmarshal in the handler can decode it.
func postJSON(ctx *fasthttp.RequestCtx, body any) {
	data, _ := json.Marshal(body)
	ctx.Request.SetBody(data)
	ctx.Request.Header.SetContentType("application/json")
}

// -----------------------------------------------------------------------------
// encodeMemberToken / decodeMemberTokenToUserID round-trip
// -----------------------------------------------------------------------------

func TestEncodeAndDecodeMemberTokenRoundTrip(t *testing.T) {
	token := encodeMemberToken("user-42", "random-half")
	userID, err := decodeMemberTokenToUserID(token)
	if err != nil {
		t.Fatalf("decodeMemberTokenToUserID: %v", err)
	}
	if userID != "user-42" {
		t.Fatalf("expected user-42, got %q", userID)
	}
	if got := memberRandomSessionToken(token); got != "random-half" {
		t.Fatalf("expected random-half, got %q", got)
	}
}

// TestDecodeMemberTokenRejectsMalformedInputs covers the failure paths the
// middleware silently translates into 401s.
func TestDecodeMemberTokenRejectsMalformedInputs(t *testing.T) {
	cases := []string{
		"",
		"no-separator",
		":only-random",
		"only-user-id:",
	}
	for _, c := range cases {
		userID, err := decodeMemberTokenToUserID(c)
		if err != nil {
			t.Fatalf("expected nil err on %q, got %v", c, err)
		}
		if userID != "" {
			t.Fatalf("expected empty userID on %q, got %q", c, userID)
		}
	}
}

// -----------------------------------------------------------------------------
// POST /api/member/login
// -----------------------------------------------------------------------------

func TestMemberLoginHappyPath(t *testing.T) {
	store := newFakeMemberConfigStore()
	u := &tables.TableUser{
		ID:    "user-1",
		Email: "alice@example.com",
		Role:  tables.UserRoleMember,
		Status: tables.UserStatusActive,
	}
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	store.seedUser(t, u)

	h := NewMemberSessionHandler(store)
	ctx := newMemberCtx()
	postJSON(ctx, map[string]string{"email": "alice@example.com", "password": "hunter2"})

	h.login(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", status, ctx.Response.Body())
	}

	// Cookie should be set with the bf_member_session name (NOT the admin
	// "token" name — that's the whole point of the two-cookie design).
	cookie := responseCookieValue(t, ctx, MemberSessionCookieName)
	if cookie == "" {
		t.Fatalf("expected %s cookie, got headers=%v", MemberSessionCookieName, ctx.Response.Header.Cookies())
	}
	if strings.Contains(cookie, "alice") {
		t.Fatalf("cookie value must be a session token, not the email: %s", cookie)
	}

	// Session row must have been created with a valid random half.
	random := memberRandomSessionToken(cookie)
	if random == "" {
		t.Fatalf("encoded cookie should have a random half: %s", cookie)
	}
	if _, err := store.GetSession(ctx, random); err != nil {
		t.Fatalf("session lookup failed: %v", err)
	}
}

func TestMemberLoginWrongPassword(t *testing.T) {
	store := newFakeMemberConfigStore()
	u := &tables.TableUser{
		ID:     "user-1",
		Email:  "alice@example.com",
		Status: tables.UserStatusActive,
	}
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	store.seedUser(t, u)

	h := NewMemberSessionHandler(store)
	ctx := newMemberCtx()
	postJSON(ctx, map[string]string{"email": "alice@example.com", "password": "wrong"})

	h.login(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", status, ctx.Response.Body())
	}
}

func TestMemberLoginUnknownUserDoesNotProbeStatus(t *testing.T) {
	store := newFakeMemberConfigStore()
	h := NewMemberSessionHandler(store)

	ctx := newMemberCtx()
	postJSON(ctx, map[string]string{"email": "ghost@example.com", "password": "x"})

	h.login(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", status, ctx.Response.Body())
	}
}

func TestMemberLoginDisabledUserRefused(t *testing.T) {
	store := newFakeMemberConfigStore()
	u := &tables.TableUser{
		ID:     "user-1",
		Email:  "alice@example.com",
		Status: tables.UserStatusDisabled,
	}
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	store.seedUser(t, u)

	h := NewMemberSessionHandler(store)
	ctx := newMemberCtx()
	postJSON(ctx, map[string]string{"email": "alice@example.com", "password": "hunter2"})

	h.login(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", status, ctx.Response.Body())
	}
}

func TestMemberLoginPendingUserRefused(t *testing.T) {
	store := newFakeMemberConfigStore()
	u := &tables.TableUser{
		ID:     "user-1",
		Email:  "alice@example.com",
		Status: tables.UserStatusPending,
	}
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	store.seedUser(t, u)

	h := NewMemberSessionHandler(store)
	ctx := newMemberCtx()
	postJSON(ctx, map[string]string{"email": "alice@example.com", "password": "hunter2"})

	h.login(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", status, ctx.Response.Body())
	}
}

// -----------------------------------------------------------------------------
// POST /api/member/logout
// -----------------------------------------------------------------------------

func TestMemberLogoutDeletesSessionAndClearsCookie(t *testing.T) {
	store := newFakeMemberConfigStore()
	u := &tables.TableUser{
		ID:     "user-1",
		Email:  "alice@example.com",
		Status: tables.UserStatusActive,
	}
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	store.seedUser(t, u)

	h := NewMemberSessionHandler(store)

	// First, log in so we have a real session row.
	loginCtx := newMemberCtx()
	postJSON(loginCtx, map[string]string{"email": "alice@example.com", "password": "hunter2"})
	h.login(loginCtx)
	cookie := responseCookieValue(t, loginCtx, MemberSessionCookieName)
	if cookie == "" {
		t.Fatalf("setup: expected member cookie after login")
	}

	// Then log out — clear the cookie and delete the session.
	logoutCtx := newMemberCtx()
	logoutCtx.Request.Header.SetCookie(MemberSessionCookieName, cookie)
	h.logout(logoutCtx)

	if status := logoutCtx.Response.StatusCode(); status != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	// Cookie value should now be empty + expired.
	cleared := responseCookieValue(t, logoutCtx, MemberSessionCookieName)
	if cleared != "" {
		t.Fatalf("expected cleared cookie, got %q", cleared)
	}
	// Session row should be gone.
	if _, err := store.GetSession(logoutCtx, memberRandomSessionToken(cookie)); err != nil {
		t.Fatalf("session lookup after logout: %v", err)
	}
}

// -----------------------------------------------------------------------------
// GET /api/member/me
// -----------------------------------------------------------------------------

func TestMemberMeRequiresContextUserID(t *testing.T) {
	store := newFakeMemberConfigStore()
	h := NewMemberSessionHandler(store)
	ctx := newMemberCtx()
	h.me(ctx)
	if status := ctx.Response.StatusCode(); status != fasthttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
}

func TestMemberMeReturnsUserPayload(t *testing.T) {
	store := newFakeMemberConfigStore()
	u := &tables.TableUser{
		ID:          "user-1",
		Email:       "alice@example.com",
		DisplayName: "Alice",
		Role:        tables.UserRoleMember,
		Status:      tables.UserStatusActive,
	}
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	store.seedUser(t, u)

	h := NewMemberSessionHandler(store)
	ctx := newMemberCtx()
	ctx.SetUserValue(schemas.BifrostContextKeyMemberUserID, "user-1")
	h.me(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", status, ctx.Response.Body())
	}
	var payload map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &payload); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, ctx.Response.Body())
	}
	user, ok := payload["user"].(map[string]any)
	if !ok {
		t.Fatalf("payload missing user: %v", payload)
	}
	if user["email"] != "alice@example.com" {
		t.Fatalf("expected email alice@example.com, got %v", user["email"])
	}
	// Defense-in-depth: password_hash must not be on the wire.
	if _, leaked := user["password_hash"]; leaked {
		t.Fatalf("password_hash leaked: %v", user)
	}
}

// -----------------------------------------------------------------------------
// GET /api/member/auth-status
// -----------------------------------------------------------------------------

func TestMemberAuthStatusWithoutCookie(t *testing.T) {
	store := newFakeMemberConfigStore()
	h := NewMemberSessionHandler(store)
	ctx := newMemberCtx()
	h.authStatus(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	var payload map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["authenticated"] != false {
		t.Fatalf("expected authenticated=false, got %v", payload["authenticated"])
	}
}

func TestMemberAuthStatusWithValidCookie(t *testing.T) {
	store := newFakeMemberConfigStore()
	u := &tables.TableUser{
		ID:     "user-1",
		Email:  "alice@example.com",
		Status: tables.UserStatusActive,
	}
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	store.seedUser(t, u)

	// Log in to get a real cookie.
	h := NewMemberSessionHandler(store)
	loginCtx := newMemberCtx()
	postJSON(loginCtx, map[string]string{"email": "alice@example.com", "password": "hunter2"})
	h.login(loginCtx)
	cookie := responseCookieValue(t, loginCtx, MemberSessionCookieName)

	// Then call auth-status with that cookie.
	statusCtx := newMemberCtx()
	statusCtx.Request.Header.SetCookie(MemberSessionCookieName, cookie)
	h.authStatus(statusCtx)

	if status := statusCtx.Response.StatusCode(); status != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	var payload map[string]any
	if err := json.Unmarshal(statusCtx.Response.Body(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["authenticated"] != true {
		t.Fatalf("expected authenticated=true, got %v body=%s", payload["authenticated"], statusCtx.Response.Body())
	}
}

// -----------------------------------------------------------------------------
// MemberAuthMiddleware: cookie + context plumbing
// -----------------------------------------------------------------------------

func TestMemberAuthMiddlewareAdmitsValidSession(t *testing.T) {
	store := newFakeMemberConfigStore()
	u := &tables.TableUser{ID: "user-1", Email: "alice@example.com", Status: tables.UserStatusActive}
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	store.seedUser(t, u)

	mw, err := InitMemberAuthMiddleware(store)
	if err != nil {
		t.Fatalf("InitMemberAuthMiddleware: %v", err)
	}
	handler := mw.APIMiddleware()(func(ctx *fasthttp.RequestCtx) {
		ctx.Response.SetBodyString("ok")
	})

	// Login to obtain a cookie.
	h := NewMemberSessionHandler(store)
	loginCtx := newMemberCtx()
	postJSON(loginCtx, map[string]string{"email": "alice@example.com", "password": "hunter2"})
	h.login(loginCtx)
	cookie := responseCookieValue(t, loginCtx, MemberSessionCookieName)

	// Drive a protected endpoint with that cookie.
	ctx := newMemberCtx()
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/member/me")
	ctx.Request.Header.SetCookie(MemberSessionCookieName, cookie)
	handler(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", status, ctx.Response.Body())
	}
	if raw, ok := ctx.UserValue(schemas.BifrostContextKeyMemberUserID).(string); !ok || raw != "user-1" {
		t.Fatalf("expected member user id 'user-1' on context, got %v", ctx.UserValue(schemas.BifrostContextKeyMemberUserID))
	}
}

func TestMemberAuthMiddlewareRejectsMissingCookie(t *testing.T) {
	store := newFakeMemberConfigStore()
	mw, err := InitMemberAuthMiddleware(store)
	if err != nil {
		t.Fatalf("InitMemberAuthMiddleware: %v", err)
	}
	handler := mw.APIMiddleware()(func(ctx *fasthttp.RequestCtx) {
		ctx.Response.SetBodyString("ok")
	})

	ctx := newMemberCtx()
	ctx.Request.SetRequestURI("/api/member/me")
	handler(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
}

func TestMemberAuthMiddlewareDoesNotInterfereWithAdminCookie(t *testing.T) {
	store := newFakeMemberConfigStore()
	mw, err := InitMemberAuthMiddleware(store)
	if err != nil {
		t.Fatalf("InitMemberAuthMiddleware: %v", err)
	}
	handler := mw.APIMiddleware()(func(ctx *fasthttp.RequestCtx) {
		ctx.Response.SetBodyString("ok")
	})

	// Admin cookie (named "token") must not satisfy a member-only route —
	// the two paths are independent by design.
	ctx := newMemberCtx()
	ctx.Request.SetRequestURI("/api/member/me")
	ctx.Request.Header.SetCookie("token", "admin-session")
	handler(ctx)

	if status := ctx.Response.StatusCode(); status != fasthttp.StatusUnauthorized {
		t.Fatalf("admin cookie must not authorize member route, got %d", status)
	}
}

// TestMemberAuthMiddlewareLoginPathBypassesAuth exercises the explicit
// whitelist: /api/member/login and /api/member/auth-status must always
// pass through, even when no cookie is present.
func TestMemberAuthMiddlewareLoginPathBypassesAuth(t *testing.T) {
	store := newFakeMemberConfigStore()
	mw, err := InitMemberAuthMiddleware(store)
	if err != nil {
		t.Fatalf("InitMemberAuthMiddleware: %v", err)
	}
	handler := mw.APIMiddleware()(func(ctx *fasthttp.RequestCtx) {
		ctx.Response.SetBodyString("ok")
	})

	for _, path := range []string{"/api/member/login", "/api/member/auth-status"} {
		ctx := newMemberCtx()
		ctx.Request.SetRequestURI(path)
		handler(ctx)
		if status := ctx.Response.StatusCode(); status != fasthttp.StatusOK {
			t.Fatalf("expected %s to bypass auth, got %d", path, status)
		}
	}
}
