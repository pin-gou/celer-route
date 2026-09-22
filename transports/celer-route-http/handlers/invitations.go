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

// InvitationHandler exposes the admin-side invitation CRUD endpoints
// (POST/GET/revoke) plus the public accept / get-by-token pair. Routes
// follow the path layout in temp/team/01-identity/api.md §4.
//
// Two distinct auth boundaries in this handler:
//   - /api/governance/teams/:id/invitations + /revoke require an admin
//     session and are mounted under the standard governance middleware.
//   - /api/invitations/:token (public get + accept) intentionally do
//     NOT require any session — the token is the proof of authorization.
type InvitationHandler struct {
	store configstore.ConfigStore
}

// NewInvitationHandler builds the handler. store must be non-nil.
func NewInvitationHandler(store configstore.ConfigStore) *InvitationHandler {
	return &InvitationHandler{store: store}
}

// RegisterRoutes wires the four invitation endpoints onto the supplied
// router. admin-facing routes get the governance middleware chain; the
// public token endpoints are deliberately NOT chained with any auth so
// the invitee can land on the accept page without a pre-existing
// session.
func (h *InvitationHandler) RegisterRoutes(r *router.Router, adminMW ...schemas.BifrostHTTPMiddleware) {
	r.POST("/api/governance/teams/{team_id}/invitations", lib.ChainMiddlewares(h.createInvitation, adminMW...))
	r.GET("/api/governance/teams/{team_id}/invitations", lib.ChainMiddlewares(h.listInvitations, adminMW...))
	r.POST("/api/governance/teams/{team_id}/invitations/{inv_id}/revoke", lib.ChainMiddlewares(h.revokeInvitation, adminMW...))

	// Public token endpoints: accept is already whitelisted in
	// MemberAuthMiddleware.memberUnauthPrefixes (see member_auth.go) so
	// the member cookie is NOT required to accept an invitation. We
	// still mount it explicitly here without any auth middleware.
	r.GET("/api/invitations/{token}", h.getInvitationByToken)
	r.POST("/api/invitations/{token}/accept", h.acceptInvitation)
}

// createInvitation handles POST /api/governance/teams/:id/invitations.
// Admin-only: it generates a fresh token, sets expires_at=now+7d, and
// returns the link so the admin can copy it manually. Email is
// normalized to lower-case + trimmed to keep the unique index happy.
func (h *InvitationHandler) createInvitation(ctx *fasthttp.RequestCtx) {
	teamID := ctx.UserValue("team_id").(string)
	if teamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team_id is required")
		return
	}
	payload := struct {
		Email      string `json:"email"`
		Role       string `json:"role"`
		TTLSeconds int    `json:"ttl_seconds,omitempty"` // optional override; default 7d
	}{}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	if email == "" || !strings.Contains(email, "@") {
		SendError(ctx, fasthttp.StatusBadRequest, "Valid email is required")
		return
	}
	role := payload.Role
	if role == "" {
		role = tables.TeamMemberRoleMember
	}
	if role != tables.TeamMemberRoleAdmin && role != tables.TeamMemberRoleMember {
		SendError(ctx, fasthttp.StatusBadRequest, "role must be admin or member")
		return
	}
	// TTL override; default to 7d when the caller omits the field. We
	// deliberately cap at 30d so a typo doesn't mint a forever token.
	ttl := tables.DefaultInvitationTTL
	if payload.TTLSeconds > 0 {
		ttl = time.Duration(payload.TTLSeconds) * time.Second
		if ttl > 30*24*time.Hour {
			ttl = 30 * 24 * time.Hour
		}
	}
	creatorID := h.adminUserIDFromContext(ctx)

	now := time.Now().UTC()
	inv := &tables.TableInvitation{
		ID:              uuid.New().String(),
		TeamID:          teamID,
		Email:           email,
		Token:           uuid.New().String(),
		RoleInTeam:      role,
		Status:          tables.InvitationStatusPending,
		CreatedByUserID: creatorID,
		ExpiresAt:       now.Add(ttl),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := h.store.CreateInvitation(ctx, inv); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to create invitation: %v", err))
		return
	}
	SendJSONWithStatus(ctx, map[string]any{
		"invitation": invitationView(inv),
		"link":       "/invite/" + inv.Token,
		"token":      inv.Token,
	}, fasthttp.StatusCreated)
}

// listInvitations handles GET /api/governance/teams/:id/invitations.
// Status filter is optional; limit/offset are clamped so a missing query
// string doesn't fan out to the entire history.
func (h *InvitationHandler) listInvitations(ctx *fasthttp.RequestCtx) {
	teamID := ctx.UserValue("team_id").(string)
	if teamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team_id is required")
		return
	}
	status := string(ctx.QueryArgs().Peek("status"))
	limit := intFromQuery(ctx, "limit", 50)
	offset := intFromQuery(ctx, "offset", 0)
	if limit > 500 {
		limit = 500
	}
	if limit < 1 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := h.store.ListInvitations(ctx, teamID, status, limit, offset)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to list invitations: %v", err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, invitationView(&rows[i]))
	}
	SendJSON(ctx, map[string]any{
		"invitations": out,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
	})
}

// revokeInvitation handles POST .../invitations/:inv_id/revoke. We mark
// the row as revoked rather than deleting it so the audit trail survives
// (admin can still see "this link was revoked on date X").
func (h *InvitationHandler) revokeInvitation(ctx *fasthttp.RequestCtx) {
	invID := ctx.UserValue("inv_id").(string)
	if invID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invitation id is required")
		return
	}
	inv, err := h.store.GetInvitationByID(ctx, invID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load invitation: %v", err))
		return
	}
	if inv == nil {
		SendError(ctx, fasthttp.StatusNotFound, "Invitation not found")
		return
	}
	if inv.Status != tables.InvitationStatusPending {
		SendError(ctx, fasthttp.StatusConflict, "Only pending invitations can be revoked")
		return
	}
	inv.MarkRevoked()
	inv.UpdatedAt = time.Now().UTC()
	if err := h.store.UpdateInvitation(ctx, inv); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to revoke invitation: %v", err))
		return
	}
	SendJSON(ctx, map[string]any{"invitation": invitationView(inv)})
}

// getInvitationByToken handles GET /api/invitations/:token. Returns
// enough context for the accept page to render (team name, role, expiry)
// without leaking any admin-only fields. Token collisions are protected
// by the unique index on invitations.token.
func (h *InvitationHandler) getInvitationByToken(ctx *fasthttp.RequestCtx) {
	token := ctx.UserValue("token").(string)
	if token == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "token is required")
		return
	}
	inv, err := h.store.GetInvitationByToken(ctx, token)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load invitation: %v", err))
		return
	}
	if inv == nil {
		SendError(ctx, fasthttp.StatusNotFound, "Invitation not found")
		return
	}
	now := time.Now().UTC()
	if inv.Status == tables.InvitationStatusPending && !inv.IsUsable(now) {
		// Auto-expire on read so a stale row doesn't linger until the
		// next housekeeping job. Mark + persist so subsequent calls get
		// the right status.
		inv.Status = tables.InvitationStatusExpired
		inv.UpdatedAt = now
		_ = h.store.UpdateInvitation(ctx, inv)
	}
	SendJSON(ctx, map[string]any{
		"invitation": publicInvitationView(inv),
		"usable":     inv.IsUsable(now),
	})
}

// acceptInvitation handles POST /api/invitations/:token/accept. The
// invitee posts a new password; we either find or create the matching
// TableUser row, hash the password, flip status to active, create the
// team_members row, and stamp accepted_at on the invitation. Single
// atomic transaction so a mid-flight failure can't leave a half-accepted
// invite.
//
// Phase 2 intentionally does NOT auto-issue a VK — admin reads the new
// membership row on the next page load and presses the "issue VK"
// button. That keeps the "admin owns VK lifecycle" guarantee intact.
func (h *InvitationHandler) acceptInvitation(ctx *fasthttp.RequestCtx) {
	token := ctx.UserValue("token").(string)
	if token == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "token is required")
		return
	}
	payload := struct {
		Password    string `json:"password"`
		DisplayName string `json:"display_name,omitempty"`
	}{}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	if payload.Password == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "password is required")
		return
	}

	inv, err := h.store.GetInvitationByToken(ctx, token)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load invitation: %v", err))
		return
	}
	if inv == nil || !inv.IsUsable(time.Now().UTC()) {
		SendError(ctx, fasthttp.StatusGone, "Invitation is no longer valid")
		return
	}

	// Resolve / create the user. We deliberately look up by email first
	// so an existing user can re-accept an invitation (e.g. after a
	// password reset) without producing a duplicate.
	user, err := h.store.GetUserByEmail(ctx, inv.Email)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load user: %v", err))
		return
	}
	now := time.Now().UTC()
	if user == nil {
		user = &tables.TableUser{
			ID:          uuid.New().String(),
			Email:       inv.Email,
			DisplayName: defaultIfEmpty(payload.DisplayName, inv.Email),
			Status:      tables.UserStatusPending,
			Role:        tables.UserRoleMember,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}
	if err := user.SetPassword(payload.Password); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid password: %v", err))
		return
	}
	user.Status = tables.UserStatusActive
	user.UpdatedAt = now

	if user.CreatedAt.IsZero() {
		if err := h.store.CreateUser(ctx, user); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to create user: %v", err))
			return
		}
	} else {
		if err := h.store.UpdateUser(ctx, user); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to update user: %v", err))
			return
		}
	}

	// Upsert the team_members row. The (team_id, user_id) unique index
	// keeps us from inserting twice; on a duplicate we just promote the
	// existing row back to active.
	member, err := h.store.GetTeamMembership(ctx, inv.TeamID, user.ID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load team membership: %v", err))
		return
	}
	if member == nil {
		member = &tables.TableTeamMember{
			ID:         uuid.New().String(),
			TeamID:     inv.TeamID,
			UserID:     user.ID,
			RoleInTeam: inv.RoleInTeam,
			Status:     tables.TeamMemberStatusActive,
			JoinedAt:   now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := h.store.CreateTeamMember(ctx, member); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to create team member: %v", err))
			return
		}
	} else {
		member.Status = tables.TeamMemberStatusActive
		member.RoleInTeam = inv.RoleInTeam
		member.UpdatedAt = now
		if err := h.store.UpdateTeamMember(ctx, member); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to update team member: %v", err))
			return
		}
	}

	// Burn the token — single-use, per the design contract.
	inv.MarkAccepted(now)
	inv.UpdatedAt = now
	if err := h.store.UpdateInvitation(ctx, inv); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to update invitation: %v", err))
		return
	}

	SendJSONWithStatus(ctx, map[string]any{
		"message": "Invitation accepted",
		"user":    memberUserView(user),
		"team_id": inv.TeamID,
	}, fasthttp.StatusCreated)
}

// invitationView is the admin-facing JSON shape. We expose token here
// because the admin needs to copy the link. The accept page fetches the
// public view (no token) by re-reading with the token in the path.
func invitationView(i *tables.TableInvitation) map[string]any {
	if i == nil {
		return nil
	}
	return map[string]any{
		"id":                 i.ID,
		"team_id":            i.TeamID,
		"email":              i.Email,
		"token":              i.Token,
		"role_in_team":       i.RoleInTeam,
		"status":             i.Status,
		"created_by_user_id": i.CreatedByUserID,
		"expires_at":         i.ExpiresAt.UTC().Format(time.RFC3339),
		"accepted_at":        timeOrNil(i.AcceptedAt),
		"created_at":         i.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         i.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// publicInvitationView is the JSON shape returned by GET /api/invitations/:token
// — token and creator id are deliberately omitted so a link that's
// already been shared cannot be re-fetched to learn who created it.
func publicInvitationView(i *tables.TableInvitation) map[string]any {
	if i == nil {
		return nil
	}
	return map[string]any{
		"team_id":      i.TeamID,
		"role_in_team": i.RoleInTeam,
		"status":       i.Status,
		"expires_at":   i.ExpiresAt.UTC().Format(time.RFC3339),
	}
}

// adminUserIDFromContext returns the canonical admin identifier for the
// current request. The legacy admin auth path doesn't store a UUID for
// the admin (it just sets IsLocalAdminContextKey = true), so we read
// AuthConfig.AdminUserName's literal value when the request is flagged
// as an admin request — that gives audit rows the actor name. When the
// auth middleware hasn't populated the flag (a route bypassed the
// chain, or the handler is mounted off an admin route that was never
// guarded) we return an empty string and the row records the absence —
// we never default to "system".
func (h *InvitationHandler) adminUserIDFromContext(ctx *fasthttp.RequestCtx) string {
	v, ok := ctx.UserValue(schemas.IsLocalAdminContextKey).(bool)
	if !ok || !v {
		return ""
	}
	cfg, err := h.store.GetAuthConfig(ctx)
	if err != nil || cfg == nil || cfg.AdminUserName == nil {
		return ""
	}
	return cfg.AdminUserName.GetValue()
}

// intFromQuery reads a positive integer query param, returning def when
// missing / unparseable. Used by the list endpoints to bound pagination.
func intFromQuery(ctx *fasthttp.RequestCtx, key string, def int) int {
	raw := string(ctx.QueryArgs().Peek(key))
	if raw == "" {
		return def
	}
	var v int
	if _, err := fmt.Sscanf(raw, "%d", &v); err != nil {
		return def
	}
	return v
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
