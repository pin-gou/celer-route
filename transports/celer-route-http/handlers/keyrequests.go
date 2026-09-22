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

// KeyRequestHandler exposes member-side "submit request" + admin-side
// "approve / reject" endpoints. Phase 2 keeps approval side-effects
// intentionally narrow: the row flips to approved/rejected and (for
// join_team) we create a team_members row. VK creation is left to the
// existing /api/governance/virtual-keys POST handler so admin remains
// the sole owner of VK lifecycle — see temp/team/01-identity/flows.md
// flow 1.5 + flow 2.
type KeyRequestHandler struct {
	store configstore.ConfigStore
}

// NewKeyRequestHandler builds the handler. store must be non-nil.
func NewKeyRequestHandler(store configstore.ConfigStore) *KeyRequestHandler {
	return &KeyRequestHandler{store: store}
}

// RegisterRoutes wires three endpoint families:
//   - /api/member/key-requests (member session) — submit + list "mine"
//   - /api/governance/key-requests (admin session) — list all + decide
func (h *KeyRequestHandler) RegisterRoutes(r *router.Router, adminMW []schemas.BifrostHTTPMiddleware, memberMW ...schemas.BifrostHTTPMiddleware) {
	r.POST("/api/member/key-requests", lib.ChainMiddlewares(h.submit, memberMW...))
	r.GET("/api/member/key-requests", lib.ChainMiddlewares(h.listMine, memberMW...))
	r.GET("/api/governance/key-requests", lib.ChainMiddlewares(h.listAll, adminMW...))
	r.POST("/api/governance/key-requests/{req_id}/approve", lib.ChainMiddlewares(h.approve, adminMW...))
	r.POST("/api/governance/key-requests/{req_id}/reject", lib.ChainMiddlewares(h.reject, adminMW...))
}

// submit handles POST /api/member/key-requests. kind comes from the
// three values in tables.KeyRequestKind*; requested_models + budget_limit
// are optional and only validated against the relevant kind.
func (h *KeyRequestHandler) submit(ctx *fasthttp.RequestCtx) {
	userID, ok := ctx.UserValue(schemas.BifrostContextKeyMemberUserID).(string)
	if !ok || userID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	payload := struct {
		Kind            string   `json:"kind"`
		Purpose         string   `json:"purpose"`
		TeamID          string   `json:"team_id"`
		RequestedModels []string `json:"requested_models,omitempty"`
		BudgetLimit     *float64 `json:"budget_limit,omitempty"`
	}{}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	purpose := strings.TrimSpace(payload.Purpose)
	if payload.Kind == "" || purpose == "" || payload.TeamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "kind, purpose, and team_id are required")
		return
	}
	switch payload.Kind {
	case tables.KeyRequestKindJoinTeam, tables.KeyRequestKindExtendQuota, tables.KeyRequestKindAddVK:
	default:
		SendError(ctx, fasthttp.StatusBadRequest, "kind must be join_team, extend_quota, or add_vk")
		return
	}

	now := time.Now().UTC()
	req := &tables.TableKeyRequest{
		ID:        uuid.New().String(),
		UserID:    userID,
		TeamID:    payload.TeamID,
		Kind:      payload.Kind,
		Purpose:   purpose,
		Status:    tables.KeyRequestStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if len(payload.RequestedModels) > 0 {
		// Stash as JSON so the admin UI can re-render the requested model
		// list without us having to add a join table.
		raw, _ := json.Marshal(payload.RequestedModels)
		s := string(raw)
		req.RequestedModels = &s
	}
	if payload.BudgetLimit != nil {
		req.BudgetLimit = payload.BudgetLimit
	}
	if err := h.store.CreateKeyRequest(ctx, req); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to create key request: %v", err))
		return
	}
	SendJSONWithStatus(ctx, map[string]any{
		"key_request": keyRequestView(req),
		"message":     "Key request submitted",
	}, fasthttp.StatusCreated)
}

// listMine handles GET /api/member/key-requests. Scoped to the current
// member so the UI can render "my requests" without an admin filter.
func (h *KeyRequestHandler) listMine(ctx *fasthttp.RequestCtx) {
	userID, ok := ctx.UserValue(schemas.BifrostContextKeyMemberUserID).(string)
	if !ok || userID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	status := string(ctx.QueryArgs().Peek("status"))
	limit := intFromQuery(ctx, "limit", 50)
	offset := intFromQuery(ctx, "offset", 0)
	if limit > 200 {
		limit = 200
	}
	if limit < 1 {
		limit = 50
	}
	rows, total, err := h.store.ListKeyRequests(ctx, status, userID, "", limit, offset)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to list key requests: %v", err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, keyRequestView(&rows[i]))
	}
	SendJSON(ctx, map[string]any{
		"key_requests": out,
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	})
}

// listAll handles GET /api/governance/key-requests. Admin can filter by
// status / user / team independently; missing filters default to "any".
func (h *KeyRequestHandler) listAll(ctx *fasthttp.RequestCtx) {
	status := string(ctx.QueryArgs().Peek("status"))
	userID := string(ctx.QueryArgs().Peek("user_id"))
	teamID := string(ctx.QueryArgs().Peek("team_id"))
	limit := intFromQuery(ctx, "limit", 50)
	offset := intFromQuery(ctx, "offset", 0)
	if limit > 500 {
		limit = 500
	}
	if limit < 1 {
		limit = 50
	}
	rows, total, err := h.store.ListKeyRequests(ctx, status, userID, teamID, limit, offset)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to list key requests: %v", err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, keyRequestView(&rows[i]))
	}
	SendJSON(ctx, map[string]any{
		"key_requests": out,
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	})
}

// approve handles POST /api/governance/key-requests/:req_id/approve.
// Side effects by kind:
//   - join_team: create the team_members row (status=active).
//   - extend_quota / add_vk: admin still has to press the "create VK"
//     button separately — we only stamp approved_by + decision_note;
//     the admin then POSTs /api/governance/virtual-keys and the
//     downstream call back-fills virtual_key_id via the
//     backfillVK helper below.
//
// Optional body: { virtual_key_id?: string, decision_note?: string }.
// virtual_key_id is only relevant for extend_quota / add_vk.
func (h *KeyRequestHandler) approve(ctx *fasthttp.RequestCtx) {
	reqID := ctx.UserValue("req_id").(string)
	if reqID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "request id is required")
		return
	}
	payload := struct {
		VirtualKeyID string `json:"virtual_key_id,omitempty"`
		DecisionNote string `json:"decision_note,omitempty"`
	}{}
	if len(ctx.PostBody()) > 0 {
		if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
			return
		}
	}

	req, err := h.store.GetKeyRequestByID(ctx, reqID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load key request: %v", err))
		return
	}
	if req == nil {
		SendError(ctx, fasthttp.StatusNotFound, "Key request not found")
		return
	}
	if req.IsTerminal() {
		SendError(ctx, fasthttp.StatusConflict, "Key request already decided")
		return
	}

	approver := h.adminUserIDFromContext(ctx)
	now := time.Now().UTC()
	req.MarkApproved(approver, payload.DecisionNote, now)
	if payload.VirtualKeyID != "" {
		req.VirtualKeyID = &payload.VirtualKeyID
	}

	// join_team side effect: create the membership row if it doesn't
	// already exist. This is the only "automatic" side-effect of
	// approval — VK creation remains a separate, deliberate admin
	// action. We deliberately do not flip existing removed memberships
	// back to active here; if the user was removed, a fresh
	// invitation is the right path.
	if req.Kind == tables.KeyRequestKindJoinTeam {
		existing, err := h.store.GetTeamMembership(ctx, req.TeamID, req.UserID)
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load team membership: %v", err))
			return
		}
		if existing == nil {
			member := &tables.TableTeamMember{
				ID:         uuid.New().String(),
				TeamID:     req.TeamID,
				UserID:     req.UserID,
				RoleInTeam: tables.TeamMemberRoleMember,
				Status:     tables.TeamMemberStatusActive,
				JoinedAt:   now,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := h.store.CreateTeamMember(ctx, member); err != nil {
				SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to create team member: %v", err))
				return
			}
		}
	}

	if err := h.store.UpdateKeyRequest(ctx, req); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to update key request: %v", err))
		return
	}
	SendJSON(ctx, map[string]any{
		"key_request": keyRequestView(req),
		"message":     "Key request approved",
	})
}

// reject handles POST /api/governance/key-requests/:req_id/reject. We
// require a non-empty reason so the audit log carries the rationale; the
// applicant will see this reason on the member portal.
func (h *KeyRequestHandler) reject(ctx *fasthttp.RequestCtx) {
	reqID := ctx.UserValue("req_id").(string)
	if reqID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "request id is required")
		return
	}
	payload := struct {
		DecisionNote string `json:"decision_note"`
	}{}
	if len(ctx.PostBody()) > 0 {
		if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
			return
		}
	}
	reason := strings.TrimSpace(payload.DecisionNote)
	if reason == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "decision_note is required for rejection")
		return
	}
	req, err := h.store.GetKeyRequestByID(ctx, reqID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load key request: %v", err))
		return
	}
	if req == nil {
		SendError(ctx, fasthttp.StatusNotFound, "Key request not found")
		return
	}
	if req.IsTerminal() {
		SendError(ctx, fasthttp.StatusConflict, "Key request already decided")
		return
	}
	approver := h.adminUserIDFromContext(ctx)
	now := time.Now().UTC()
	req.MarkRejected(approver, reason, now)
	if err := h.store.UpdateKeyRequest(ctx, req); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to update key request: %v", err))
		return
	}
	SendJSON(ctx, map[string]any{
		"key_request": keyRequestView(req),
		"message":     "Key request rejected",
	})
}

// keyRequestView is the JSON shape for both the admin and member views.
// We expose requested_models as a decoded slice (the raw column is JSON).
func keyRequestView(r *tables.TableKeyRequest) map[string]any {
	if r == nil {
		return nil
	}
	var models []string
	if r.RequestedModels != nil && *r.RequestedModels != "" {
		_ = json.Unmarshal([]byte(*r.RequestedModels), &models)
	}
	return map[string]any{
		"id":                  r.ID,
		"user_id":             r.UserID,
		"team_id":             r.TeamID,
		"kind":                r.Kind,
		"purpose":             r.Purpose,
		"requested_models":    models,
		"budget_limit":        r.BudgetLimit,
		"status":              r.Status,
		"approved_by_user_id": r.ApprovedByUserID,
		"decision_note":       r.DecisionNote,
		"virtual_key_id":      r.VirtualKeyID,
		"created_at":          r.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":          r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// adminUserIDFromContext returns the canonical admin identifier for the
// current request. Same semantics as invitations.go: read the
// IsLocalAdminContextKey flag, then resolve AuthConfig.AdminUserName
// from the config store. Empty string on missing flag or missing
// config so callers can persist "no actor recorded".
func (h *KeyRequestHandler) adminUserIDFromContext(ctx *fasthttp.RequestCtx) string {
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
