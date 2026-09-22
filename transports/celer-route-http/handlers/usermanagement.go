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

// UserManagementHandler exposes the admin-side user CRUD endpoints
// (list / get / update / delete) plus the offboarding helper that
// disables every active VK owned by the targeted user. Routes match
// temp/team/01-identity/api.md §2.
//
// Phase 2 intentionally does NOT expose a "create user" admin endpoint —
// the only legitimate way to onboard a user is via the invitation flow
// (handlers/invitations.go). Manual creation would skip the email-based
// audit trail and would let admin bypass the user-invite state machine.
type UserManagementHandler struct {
	store configstore.ConfigStore
}

// NewUserManagementHandler builds the handler. store must be non-nil.
func NewUserManagementHandler(store configstore.ConfigStore) *UserManagementHandler {
	return &UserManagementHandler{store: store}
}

// RegisterRoutes wires the user-management surface. disable-vks and the
// reset-password-token helper are also admin-only — they share the
// governance middleware chain. delete also cascades offboard so a
// single call replaces the "disable user + disable VKs" sequence that
// flows.md §4 documents.
func (h *UserManagementHandler) RegisterRoutes(r *router.Router, adminMW ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/governance/users", lib.ChainMiddlewares(h.listUsers, adminMW...))
	r.GET("/api/governance/users/{user_id}", lib.ChainMiddlewares(h.getUser, adminMW...))
	r.PUT("/api/governance/users/{user_id}", lib.ChainMiddlewares(h.updateUser, adminMW...))
	r.POST("/api/governance/users/{user_id}/disable", lib.ChainMiddlewares(h.disableUser, adminMW...))
	r.POST("/api/governance/users/{user_id}/disable-vks", lib.ChainMiddlewares(h.disableVks, adminMW...))
	r.POST("/api/governance/users/{user_id}/reset-password-token", lib.ChainMiddlewares(h.resetPasswordToken, adminMW...))
	r.DELETE("/api/governance/users/{user_id}", lib.ChainMiddlewares(h.deleteUser, adminMW...))
}

// listUsers handles GET /api/governance/users. Supports status / role
// filtering + pagination. We never expose password_hash (it's tagged
// json:"-" on the model) so even an unfiltered list cannot leak it.
func (h *UserManagementHandler) listUsers(ctx *fasthttp.RequestCtx) {
	status := string(ctx.QueryArgs().Peek("status"))
	role := string(ctx.QueryArgs().Peek("role"))
	limit := intFromQuery(ctx, "limit", 50)
	offset := intFromQuery(ctx, "offset", 0)
	if limit > 500 {
		limit = 500
	}
	if limit < 1 {
		limit = 50
	}
	rows, total, err := h.store.ListUsers(ctx, status, role, limit, offset)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to list users: %v", err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, memberUserView(&rows[i]))
	}
	SendJSON(ctx, map[string]any{
		"users":  out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// getUser handles GET /api/governance/users/:user_id. Returns the user
// record + their team memberships + their VKs (id + name only — VK
// values and provider key details are intentionally omitted to keep the
// "admin sees no Provider Key metadata in the user detail" contract
// from temp/team/01-identity/data-model.md §5.1).
func (h *UserManagementHandler) getUser(ctx *fasthttp.RequestCtx) {
	userID := ctx.UserValue("user_id").(string)
	if userID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "user_id is required")
		return
	}
	user, err := h.store.GetUserByID(ctx, userID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load user: %v", err))
		return
	}
	if user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "User not found")
		return
	}
	memberships, err := h.store.GetUserTeamMemberships(ctx, userID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load team memberships: %v", err))
		return
	}
	membershipViews := make([]map[string]any, 0, len(memberships))
	for i := range memberships {
		m := memberships[i]
		membershipViews = append(membershipViews, map[string]any{
			"team_id":      m.TeamID,
			"role_in_team": m.RoleInTeam,
			"status":       m.Status,
			"joined_at":    m.JoinedAt.UTC().Format(time.RFC3339),
		})
	}

	// List VKs the user owns. We pull a lightweight summary (no provider
	// configs / no provider key details) so the boundary from data-model
	// §5.1 — "member-side views never expose Provider Key metadata" — is
	// preserved even when admin reads the user-detail page.
	vks, _, err := h.store.ListVirtualKeysByUserID(ctx, userID, 200, 0)
	vkViews := make([]map[string]any, 0, len(vks))
	if err == nil {
		for _, vk := range vks {
			vkViews = append(vkViews, map[string]any{
				"id":         vk.ID,
				"name":       vk.Name,
				"is_active":  vk.IsActiveValue(),
				"team_id":    vk.TeamID,
				"updated_at": vk.UpdatedAt.UTC().Format(time.RFC3339),
			})
		}
	}
	SendJSON(ctx, map[string]any{
		"user":         memberUserView(user),
		"memberships":  membershipViews,
		"virtual_keys": vkViews,
	})
}

// updateUser handles PUT /api/governance/users/:user_id. We allow
// display_name / role / status edits; email is intentionally NOT
// editable (changing email mid-lifecycle would break the
// invitation-by-email contract).
func (h *UserManagementHandler) updateUser(ctx *fasthttp.RequestCtx) {
	userID := ctx.UserValue("user_id").(string)
	if userID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "user_id is required")
		return
	}
	user, err := h.store.GetUserByID(ctx, userID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load user: %v", err))
		return
	}
	if user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "User not found")
		return
	}
	payload := struct {
		DisplayName *string `json:"display_name,omitempty"`
		Role        *string `json:"role,omitempty"`
		Status      *string `json:"status,omitempty"`
	}{}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	if payload.DisplayName != nil {
		user.DisplayName = strings.TrimSpace(*payload.DisplayName)
	}
	if payload.Role != nil {
		switch *payload.Role {
		case tables.UserRoleAdmin, tables.UserRoleMember:
			user.Role = *payload.Role
		default:
			SendError(ctx, fasthttp.StatusBadRequest, "role must be admin or member")
			return
		}
	}
	if payload.Status != nil {
		switch *payload.Status {
		case tables.UserStatusPending, tables.UserStatusActive, tables.UserStatusDisabled:
			user.Status = *payload.Status
		default:
			SendError(ctx, fasthttp.StatusBadRequest, "status must be pending/active/disabled")
			return
		}
	}
	user.UpdatedAt = time.Now().UTC()
	if err := h.store.UpdateUser(ctx, user); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to update user: %v", err))
		return
	}
	SendJSON(ctx, map[string]any{"user": memberUserView(user)})
}

// disableUser handles POST /api/governance/users/:user_id/disable. This
// flips status to 'disabled' but leaves VKs intact; admin can then
// separately call /disable-vks if they want to revoke the user's VKs
// too. Splitting the two actions lets admin stage an offboarding
// (disable first, then disable-vks after a grace period) without
// building a state machine into a single endpoint.
func (h *UserManagementHandler) disableUser(ctx *fasthttp.RequestCtx) {
	userID := ctx.UserValue("user_id").(string)
	if userID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "user_id is required")
		return
	}
	user, err := h.store.GetUserByID(ctx, userID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load user: %v", err))
		return
	}
	if user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "User not found")
		return
	}
	user.Status = tables.UserStatusDisabled
	user.UpdatedAt = time.Now().UTC()
	if err := h.store.UpdateUser(ctx, user); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to disable user: %v", err))
		return
	}
	SendJSON(ctx, map[string]any{"user": memberUserView(user)})
}

// disableVks handles POST /api/governance/users/:user_id/disable-vks.
// This is the offboarding step from flows.md §4. Provider keys are NOT
// touched — admin retains centralized control over the Provider Key
// pool, matching the boundary spelled out in data-model.md §5.1.
func (h *UserManagementHandler) disableVks(ctx *fasthttp.RequestCtx) {
	userID := ctx.UserValue("user_id").(string)
	if userID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "user_id is required")
		return
	}
	ids, err := h.store.DisableUserVKeys(ctx, userID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to disable VKs: %v", err))
		return
	}
	SendJSON(ctx, map[string]any{
		"disabled_virtual_key_ids": ids,
		"count":                    len(ids),
		"message":                  "All VKs owned by this user have been disabled",
	})
}

// resetPasswordToken handles POST /api/governance/users/:user_id/reset-password-token.
// Because OSS has no email service we can't email the link, so we return
// the freshly minted invitation token in the response body and rely on
// admin to copy it manually (Readme D8 says password reset goes admin
// → token → member through the same /invite/{token} accept surface).
func (h *UserManagementHandler) resetPasswordToken(ctx *fasthttp.RequestCtx) {
	userID := ctx.UserValue("user_id").(string)
	if userID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "user_id is required")
		return
	}
	user, err := h.store.GetUserByID(ctx, userID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load user: %v", err))
		return
	}
	if user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "User not found")
		return
	}
	payload := struct {
		TeamID string `json:"team_id"`
	}{}
	if len(ctx.PostBody()) > 0 {
		if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
			return
		}
	}
	// Pick any existing team membership as the team_id default — admin
	// can override if the user is in multiple teams. Falls back to the
	// first membership in the user's list.
	if payload.TeamID == "" {
		memberships, err := h.store.GetUserTeamMemberships(ctx, userID)
		if err == nil && len(memberships) > 0 {
			payload.TeamID = memberships[0].TeamID
		}
	}
	if payload.TeamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team_id is required (user has no team memberships)")
		return
	}
	now := time.Now().UTC()
	creator := h.adminUserIDFromContext(ctx)
	inv := &tables.TableInvitation{
		ID:              uuid.New().String(),
		TeamID:          payload.TeamID,
		Email:           user.Email,
		Token:           uuid.New().String(),
		RoleInTeam:      tables.TeamMemberRoleMember,
		Status:          tables.InvitationStatusPending,
		CreatedByUserID: creator,
		ExpiresAt:       now.Add(tables.DefaultInvitationTTL),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := h.store.CreateInvitation(ctx, inv); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to create reset token: %v", err))
		return
	}
	SendJSONWithStatus(ctx, map[string]any{
		"token":      inv.Token,
		"link":       "/invite/" + inv.Token,
		"expires_at": inv.ExpiresAt.UTC().Format(time.RFC3339),
		"message":    "Password reset token issued; share the link with the user",
	}, fasthttp.StatusCreated)
}

// deleteUser handles DELETE /api/governance/users/:user_id. We cascade
// disable VKs as part of the delete so the offboarding contract from
// flows.md §4 holds even when admin chooses the "drop the row" path.
// Hard-delete is intended for test data + accidental duplicates; real
// offboardings should prefer disable + disable-vks so audit history
// survives.
func (h *UserManagementHandler) deleteUser(ctx *fasthttp.RequestCtx) {
	userID := ctx.UserValue("user_id").(string)
	if userID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "user_id is required")
		return
	}
	if _, err := h.store.DisableUserVKeys(ctx, userID); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to disable VKs: %v", err))
		return
	}
	if err := h.store.DeleteUser(ctx, userID); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to delete user: %v", err))
		return
	}
	SendJSON(ctx, map[string]any{"deleted": userID})
}

// adminUserIDFromContext returns the canonical admin identifier for the
// current request. Same semantics as invitations.go / keyrequests.go:
// resolve AuthConfig.AdminUserName when the IsLocalAdminContextKey
// flag is set.
func (h *UserManagementHandler) adminUserIDFromContext(ctx *fasthttp.RequestCtx) string {
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
