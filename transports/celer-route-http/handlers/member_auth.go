package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/valyala/fasthttp"
)

// MemberSessionCookieName is the cookie that backs the member-only login
// path. It is intentionally distinct from the admin cookie name ("token")
// so the two sessions can coexist on the same browser without colliding,
// matching the design contract in temp/team/01-identity/api.md §1.2.
const MemberSessionCookieName = "bf_member_session"

// memberSessionLifetime bounds how long a member session token is valid.
// 30 days matches the existing admin session lifetime so refresh cadence is
// consistent across the two paths. Re-authentication (re-login) refreshes
// the expiry.
const memberSessionLifetime = 30 * 24 * time.Hour

// MemberAuthMiddleware guards member-authenticated endpoints. It is
// deliberately separate from AuthMiddleware so the admin and member
// authentication paths can evolve independently:
//
//   - admin  : password-based AuthConfig.AdminUserName / AdminPassword;
//              sets IsLocalAdminContextKey=true on success.
//   - member : cookie-backed SessionsTable row whose subject is a
//              TableUser.ID; sets BifrostContextKeyMemberUserID.
//
// The two share the underlying sessions table (the same row shape: a
// random token + expiry) but write to different cookie names, so admin
// sessions and member sessions never collide on the same browser.
//
// The middleware never touches IsLocalAdminContextKey. That flag is owned
// exclusively by the admin path so downstream RBAC continues to mean the
// same thing it did before the member path existed.
type MemberAuthMiddleware struct {
	store configstore.ConfigStore
}

// InitMemberAuthMiddleware builds the member-auth middleware. store must
// be non-nil and support the user / session lookups.
func InitMemberAuthMiddleware(store configstore.ConfigStore) (*MemberAuthMiddleware, error) {
	if store == nil {
		return nil, fmt.Errorf("store is not present")
	}
	return &MemberAuthMiddleware{store: store}, nil
}

// APIMiddleware returns a fasthttp middleware that requires a valid member
// session cookie. Routes mounted under /api/member/* (excluding login /
// auth-status) should use this.
//
// Requests are admitted in two equivalent ways:
//   - Cookie: bf_member_session=<token>
//   - Authorization: Bearer <token>  (mirrors admin bearer behavior so SDKs
//     that already speak Bearer can reuse the same plumbing)
//
// On success the middleware writes the authenticated user's ID into the
// request context under BifrostContextKeyMemberUserID. Handlers downstream
// read that to scope queries (member VK list, member usage, etc.) — they
// MUST NOT trust user_id values pulled from request bodies.
func (m *MemberAuthMiddleware) APIMiddleware() schemas.BifrostHTTPMiddleware {
	return m.middleware()
}

// memberUnauthPaths enumerates the routes inside /api/member that must NOT
// require an existing member session — login, auth-status. Login itself
// issues the session; auth-status reports the cookie's validity without
// demanding any caller-supplied body.
var memberUnauthPaths = map[string]struct{}{
	"/api/member/login":      {},
	"/api/member/auth-status": {},
}

// memberUnauthPrefixes matches the open invitation/accept surface
// (Phase 2 will own this; keeping the prefix list here so it lands as soon
// as the first invite-accept route is added without a middleware rewrite).
var memberUnauthPrefixes = []string{
	"/api/member/invitations/",
}

func (m *MemberAuthMiddleware) middleware() schemas.BifrostHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			path := string(ctx.Path())

			// Login + auth-status pass through unauthenticated.
			if _, ok := memberUnauthPaths[path]; ok {
				next(ctx)
				return
			}
			for _, prefix := range memberUnauthPrefixes {
				if strings.HasPrefix(path, prefix) {
					next(ctx)
					return
				}
			}

			token := extractMemberToken(ctx)
			if token == "" {
				SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
				return
			}

			userID, err := m.lookupMemberUserID(ctx, token)
			if err != nil {
				SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to validate member session: %v", err))
				return
			}
			if userID == "" {
				SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
				return
			}

			ctx.SetUserValue(schemas.BifrostContextKeyMemberUserID, userID)
			next(ctx)
		}
	}
}

// lookupMemberUserID resolves a session token to the owning user's ID. We
// intentionally refuse the lookup if the resolved user is not active (e.g.
// a disabled user whose session row was somehow not cleaned up) so a
// deprovisioned user can't continue calling /api/member/* after offboarding.
func (m *MemberAuthMiddleware) lookupMemberUserID(ctx *fasthttp.RequestCtx, token string) (string, error) {
	// The encoded cookie token is "<user_id>:<random>"; only the random
	// half is persisted in the sessions table, so we strip it before
	// calling GetSession. decodeMemberTokenToUserID is called after the
	// session row confirms validity (and expiry) — that order matters so a
	// deleted or expired session cannot leak the user_id through this path.
	random := memberRandomSessionToken(token)
	if random == "" {
		return "", nil
	}
	session, err := m.store.GetSession(ctx, random)
	if err != nil || session == nil {
		return "", err
	}
	if session.ExpiresAt.Before(time.Now()) {
		return "", nil
	}
	userID, _ := decodeMemberTokenToUserID(token)
	if userID == "" {
		return "", nil
	}
	// Belt-and-braces: confirm the user still exists and is active so a
	// session whose owner has since been disabled is refused.
	user, err := m.store.GetUserByID(ctx, userID)
	if err != nil || user == nil || !user.IsActive() {
		return "", err
	}
	return userID, nil
}

// extractMemberToken pulls the member token from the request, preferring
// the cookie (the canonical browser path) and falling back to the
// Authorization header for SDKs that already speak Bearer.
func extractMemberToken(ctx *fasthttp.RequestCtx) string {
	if cookie := string(ctx.Request.Header.Cookie(MemberSessionCookieName)); cookie != "" {
		return cookie
	}
	if auth := string(ctx.Request.Header.Peek("Authorization")); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// MemberLoginCookie writes the member session cookie onto the response.
// Centralized here so the cookie attributes (HttpOnly, SameSite, secure
// detection) stay in one place and match the admin cookie's flags.
func MemberLoginCookie(ctx *fasthttp.RequestCtx, token string) {
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey(MemberSessionCookieName)
	cookie.SetValue(token)
	cookie.SetExpire(time.Now().Add(memberSessionLifetime))
	cookie.SetPath("/")
	cookie.SetHTTPOnly(true)
	cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	if string(ctx.Request.Header.Peek("X-Forwarded-Proto")) == "https" {
		cookie.SetSecure(true)
	}
	ctx.Response.Header.SetCookie(cookie)
}

// MemberClearCookie expires the member session cookie on logout so the
// browser drops it immediately. Centralized for the same reason as
// MemberLoginCookie.
func MemberClearCookie(ctx *fasthttp.RequestCtx) {
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey(MemberSessionCookieName)
	cookie.SetValue("")
	cookie.SetExpire(time.Now().Add(-time.Hour * 24 * 30))
	cookie.SetPath("/")
	cookie.SetHTTPOnly(true)
	cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	if string(ctx.Request.Header.Peek("X-Forwarded-Proto")) == "https" {
		cookie.SetSecure(true)
	}
	ctx.Response.Header.SetCookie(cookie)
}

// memberTokenPrefix / memberTokenSeparator encode the user_id into the
// session token itself so we don't need to add a user_id column to the
// shared sessions table. The format is "<user_id>:<random-token>"; the
// random tail is what gets hashed and stored in token_hash.
//
// Centralized here so the format has exactly one definition site.
const (
	memberTokenSeparator = ":"
)

// encodeMemberToken composes the cookie token from a user_id and a random
// session token. Random is the half that gets hashed in the sessions row.
func encodeMemberToken(userID, random string) string {
	return userID + memberTokenSeparator + random
}

// decodeMemberTokenToUserID parses the user_id half of a member session
// token. Returns "" on any parse failure — callers should treat that as a
// 401 rather than a 500.
func decodeMemberTokenToUserID(token string) (string, error) {
	idx := strings.Index(token, memberTokenSeparator)
	if idx <= 0 || idx >= len(token)-1 {
		return "", nil
	}
	return token[:idx], nil
}

// memberRandomSessionToken extracts the random half of the encoded token
// for lookup against the sessions table.
func memberRandomSessionToken(token string) string {
	idx := strings.Index(token, memberTokenSeparator)
	if idx < 0 || idx >= len(token)-1 {
		return ""
	}
	return token[idx+1:]
}

// memberContextLookup is the convenience wrapper used by member handlers
// to fetch the current user from context. Returns (nil, false) when no
// MemberAuthMiddleware has been mounted on the route, or when the
// middleware wrote a non-string value (defense-in-depth).
func memberContextLookup(ctx context.Context, store configstore.ConfigStore) (*MemberSession, bool) {
	raw := ctx.Value(schemas.BifrostContextKeyMemberUserID)
	userID, ok := raw.(string)
	if !ok || userID == "" || store == nil {
		return nil, false
	}
	user, err := store.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, false
	}
	return &MemberSession{User: user}, true
}

// MemberSession is the shape handed to member handlers from
// memberContextLookup. It bundles the user record with any future fields
// (team memberships, last_login_at, etc.) so handlers have a single object
// to reach for instead of chasing multiple ctx lookups.
type MemberSession struct {
	User any // *tables.TableUser — kept as any here to avoid an import cycle; member_session.go narrows it
}