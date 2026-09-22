package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fasthttp/router"
	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// MemberSessionHandler exposes the member-only login endpoints. Each route
// is documented in temp/team/01-identity/api.md §1.2; the structure mirrors
// the existing SessionHandler (admin login) so future contributors can
// reason about both paths side-by-side.
type MemberSessionHandler struct {
	store configstore.ConfigStore
}

// NewMemberSessionHandler creates a new member session handler. store must
// be non-nil and support the user / session CRUD added in Phase 1.
func NewMemberSessionHandler(store configstore.ConfigStore) *MemberSessionHandler {
	return &MemberSessionHandler{store: store}
}

// RegisterRoutes wires the four member endpoints onto the supplied router.
// No middleware is appended here because:
//   - login + auth-status must remain reachable without a session;
//   - logout + me sit behind MemberAuthMiddleware, which the server
//     configures when wiring this handler in (see server.go).
//
// The middleware separation keeps this handler focused on the request
// shapes; the server owns the security boundary.
func (h *MemberSessionHandler) RegisterRoutes(r *router.Router, memberMiddleware ...schemas.BifrostHTTPMiddleware) {
	// Login + auth-status: no auth required.
	r.POST("/api/member/login", wrap(h.login))
	r.GET("/api/member/auth-status", wrap(h.authStatus))

	// logout + me require a valid member session.
	for _, m := range memberMiddleware {
		_ = m
	}
	r.POST("/api/member/logout", lib.ChainMiddlewares(h.logout, memberMiddleware...))
	r.GET("/api/member/me", lib.ChainMiddlewares(h.me, memberMiddleware...))
}

// login handles POST /api/member/login. It looks up the user by lowercased
// email, verifies the bcrypt password_hash, and refuses login when the
// user is anything other than status='active'. On success it issues a
// session cookie with the user_id encoded into the token so the
// middleware can resolve the subject on every subsequent request without
// needing a new column on the sessions table.
func (h *MemberSessionHandler) login(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Member authentication is not available")
		return
	}
	payload := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	if email == "" || payload.Password == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Email and password are required")
		return
	}
	user, err := h.store.GetUserByEmail(ctx, email)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to look up user: %v", err))
		return
	}
	if user == nil || !user.IsActive() || user.PasswordHash == nil {
		// We deliberately collapse "no such user", "disabled user", and
		// "pending user" into a single 401 so an attacker can't probe the
		// user table by status. The same applies to a wrong password.
		SendError(ctx, fasthttp.StatusUnauthorized, "Invalid email or password")
		return
	}
	ok, err := user.VerifyPassword(payload.Password)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to verify password")
		return
	}
	if !ok {
		SendError(ctx, fasthttp.StatusUnauthorized, "Invalid email or password")
		return
	}

	// Compose the cookie token: "<user_id>:<random>". The random half is
	// what we persist in the sessions table (the BeforeSave hook hashes
	// it for indexed lookup). The full token is what we send back in the
	// cookie; on each request the middleware extracts the user_id half
	// without hitting the DB.
	random := uuid.New().String()
	session := &tables.SessionsTable{
		Token:     random,
		ExpiresAt: time.Now().Add(memberSessionLifetime),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := h.store.CreateSession(ctx, session); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to create session: %v", err))
		return
	}
	if err := h.store.UpdateUserLastLoginAt(ctx, user.ID, time.Now()); err != nil {
		// Non-fatal: a missed last_login_at write shouldn't block login.
		logger.Warn("failed to update last_login_at for user %s: %v", user.ID, err)
	}

	MemberLoginCookie(ctx, encodeMemberToken(user.ID, random))
	SendJSON(ctx, map[string]any{
		"message": "Login successful",
		"user":    memberUserView(user),
	})
}

// logout handles POST /api/member/logout. It deletes the underlying
// session row (so the token can't be replayed from another device) and
// expires the cookie. Safe to call when no session is present — the
// caller just gets the cleared cookie back either way.
func (h *MemberSessionHandler) logout(ctx *fasthttp.RequestCtx) {
	token := extractMemberToken(ctx)
	if token != "" {
		if random := memberRandomSessionToken(token); random != "" {
			if err := h.store.DeleteSession(ctx, random); err != nil {
				logger.Warn("failed to delete member session during logout: %v", err)
			}
		}
	}
	MemberClearCookie(ctx)
	SendJSON(ctx, map[string]any{
		"message": "Logout successful",
	})
}

// me handles GET /api/member/me. The MemberAuthMiddleware has already
// resolved the authenticated user_id and written it to context under
// BifrostContextKeyMemberUserID; here we hydrate the full user record
// and attach their team memberships for the portal rendering.
func (h *MemberSessionHandler) me(ctx *fasthttp.RequestCtx) {
	raw, ok := ctx.UserValue(schemas.BifrostContextKeyMemberUserID).(string)
	if !ok || raw == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	user, err := h.store.GetUserByID(ctx, raw)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load user: %v", err))
		return
	}
	if user == nil {
		// The middleware shouldn't have admitted this case (the session
		// must have resolved to an existing user), but be defensive.
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	memberships, err := h.store.GetUserTeamMemberships(ctx, user.ID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load team memberships: %v", err))
		return
	}
	SendJSON(ctx, map[string]any{
		"user":    memberUserView(user),
		"teams":   memberships,
		"is_admin": user.Role == tables.UserRoleAdmin,
	})
}

// authStatus handles GET /api/member/auth-status. It is the member-side
// counterpart to the admin's /api/session/is-auth-enabled — it tells the
// browser whether a valid member cookie is present so the SPA can branch
// to the right login page on first paint.
//
// Note: we deliberately return 200 (not 401) on a missing/invalid cookie
// so the SPA can call this endpoint unconditionally without surfacing an
// error in the network panel. The boolean carries the real signal.
func (h *MemberSessionHandler) authStatus(ctx *fasthttp.RequestCtx) {
	token := extractMemberToken(ctx)
	authenticated := false
	if token != "" {
		if random := memberRandomSessionToken(token); random != "" {
			session, err := h.store.GetSession(ctx, random)
			if err == nil && session != nil && session.ExpiresAt.After(time.Now()) {
				if userID, perr := decodeMemberTokenToUserID(token); perr == nil && userID != "" {
					user, uerr := h.store.GetUserByID(ctx, userID)
					if uerr == nil && user != nil && user.IsActive() {
						authenticated = true
					}
				}
			}
		}
	}
	SendJSON(ctx, map[string]any{
		"authenticated": authenticated,
		"auth_type":     "member_session",
	})
}

// memberUserView is the JSON shape returned to the browser. We strip
// password_hash (json:"-") on the model itself, but explicitly enumerate
// the safe fields here so the wire shape doesn't accidentally widen if a
// new column lands on TableUser.
func memberUserView(u *tables.TableUser) map[string]any {
	if u == nil {
		return nil
	}
	return map[string]any{
		"id":           u.ID,
		"email":        u.Email,
		"display_name": u.DisplayName,
		"status":       u.Status,
		"role":         u.Role,
		"last_login_at": func() any {
			if u.LastLoginAt == nil {
				return nil
			}
			return u.LastLoginAt.UTC().Format(time.RFC3339)
		}(),
		"created_at": u.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": u.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// wrap is a tiny helper that adapts a fasthttp.RequestHandler into a
// router-compatible closure. The router's r.POST signature expects a
// fasthttp.RequestHandler already, so this is mostly a no-op — but it
// keeps the RegisterRoutes method above uniform against the lib.ChainMiddlewares
// calls below, which want a fasthttp.RequestHandler too.
func wrap(h fasthttp.RequestHandler) fasthttp.RequestHandler { return h }
