package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

// TeamModelPoliciesHandler serves the per-team model ACL endpoints
// (Phase 6 / D6). One row per (team_id, provider); the resolver composes
// this with the per-VK allowlist on every model check (intersection).
//
// Routes:
//
//	GET    /api/governance/teams/:team_id/model-policies
//	GET    /api/governance/teams/:team_id/model-policies/:provider
//	PUT    /api/governance/teams/:team_id/model-policies/:provider
//	DELETE /api/governance/teams/:team_id/model-policies/:provider
type TeamModelPoliciesHandler struct {
	configStore configstore.ConfigStore
}

// NewTeamModelPoliciesHandler wires the handler. The config store must be
// non-nil — model ACL has no in-memory fallback because a stale local view
// would silently leak models the team has been told to drop.
func NewTeamModelPoliciesHandler(configStore configstore.ConfigStore) *TeamModelPoliciesHandler {
	return &TeamModelPoliciesHandler{configStore: configStore}
}

// teamModelPolicyPayload is the JSON wire shape for one row. Mirrors the
// table exactly so the response carries the same id/provider/created_at the
// UI uses for diffs.
type teamModelPolicyPayload struct {
	ID                string    `json:"id"`
	TeamID            string    `json:"team_id"`
	Provider          string    `json:"provider"`
	AllowedModels     []string  `json:"allowed_models"`
	BlacklistedModels []string  `json:"blacklisted_models"`
	CreatedByUserID   *string   `json:"created_by_user_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func teamModelPolicyToPayload(p *configstoreTables.TableTeamModelPolicy) teamModelPolicyPayload {
	if p == nil {
		return teamModelPolicyPayload{}
	}
	allowed := append([]string(nil), p.AllowedModels...)
	blacklisted := append([]string(nil), p.BlacklistedModels...)
	return teamModelPolicyPayload{
		ID:                p.ID,
		TeamID:            p.TeamID,
		Provider:          p.Provider,
		AllowedModels:     allowed,
		BlacklistedModels: blacklisted,
		CreatedByUserID:   p.CreatedByUserID,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

// teamModelPolicyWritePayload is the request body for PUT. Kept separate
// from the response so the handler never echoes back client-supplied IDs
// (the server assigns UUIDs on create).
type teamModelPolicyWritePayload struct {
	AllowedModels     []string `json:"allowed_models"`
	BlacklistedModels []string `json:"blacklisted_models"`
}

// RegisterRoutes wires the four endpoints. Pass admin auth middlewares the
// same way the rest of the governance CRUD does — these endpoints are
// admin-only because they decide what models an entire team may call.
func (h *TeamModelPoliciesHandler) RegisterRoutes(r fasthttp.RequestHandler, middlewares ...schemas.BifrostHTTPMiddleware) {
	// No-op stub; the routes are wired through GovernanceHandler.RegisterRoutes
	// alongside the rest of /api/governance. Kept here so a downstream caller
	// can wire a standalone router if it wants to.
	_ = r
}

func (h *TeamModelPoliciesHandler) ListTeamModelPolicies(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	teamID, ok := h.readTeamID(ctx)
	if !ok {
		return
	}
	rows, err := h.configStore.ListTeamModelPolicies(ctx, teamID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to list team model policies: %v", err))
		return
	}
	payload := make([]teamModelPolicyPayload, 0, len(rows))
	for i := range rows {
		payload = append(payload, teamModelPolicyToPayload(&rows[i]))
	}
	SendJSON(ctx, payload)
}

func (h *TeamModelPoliciesHandler) GetTeamModelPolicy(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	teamID, provider, ok := h.readTeamAndProvider(ctx)
	if !ok {
		return
	}
	row, err := h.configStore.GetTeamModelPolicy(ctx, teamID, provider)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to get team model policy: %v", err))
		return
	}
	if row == nil {
		SendError(ctx, fasthttp.StatusNotFound, "team model policy not found")
		return
	}
	SendJSON(ctx, teamModelPolicyToPayload(row))
}

func (h *TeamModelPoliciesHandler) UpsertTeamModelPolicy(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	teamID, provider, ok := h.readTeamAndProvider(ctx)
	if !ok {
		return
	}
	body := ctx.PostBody()
	if len(body) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "request body required")
		return
	}
	var payload teamModelPolicyWritePayload
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid request payload: %v", err))
		return
	}

	// D6 invariant: VK allowlist is the lower bound and the team allowlist is
	// the upper bound. When the team policy carries an explicit allowlist,
	// every active VK under this team must already intersect it — otherwise
	// admins will create VKs that can never be called. Refuse the write so
	// the admin fixes the VK side first; the error message points them at
	// the offending VKs by ID.
	if len(payload.AllowedModels) > 0 && !isAllWildcard(payload.AllowedModels) {
		if err := h.validateVKAllowlistsAgainstTeam(ctx, teamID, payload.AllowedModels); err != nil {
			SendError(ctx, fasthttp.StatusUnprocessableEntity, err.Error())
			return
		}
	}

	row := &configstoreTables.TableTeamModelPolicy{
		TeamID:            teamID,
		Provider:          provider,
		AllowedModels:     append([]string(nil), payload.AllowedModels...),
		BlacklistedModels: append([]string(nil), payload.BlacklistedModels...),
	}
	if err := h.configStore.UpsertTeamModelPolicy(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to upsert team model policy: %v", err))
		return
	}
	// Reload so the response reflects server-assigned IDs and timestamps.
	loaded, err := h.configStore.GetTeamModelPolicy(ctx, teamID, provider)
	if err != nil || loaded == nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to reload team model policy after write")
		return
	}
	SendJSON(ctx, teamModelPolicyToPayload(loaded))
}

func (h *TeamModelPoliciesHandler) DeleteTeamModelPolicy(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	teamID, provider, ok := h.readTeamAndProvider(ctx)
	if !ok {
		return
	}
	if err := h.configStore.DeleteTeamModelPolicy(ctx, teamID, provider); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "team model policy not found")
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to delete team model policy: %v", err))
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

// readTeamID extracts the team_id path param and rejects empty values.
func (h *TeamModelPoliciesHandler) readTeamID(ctx *fasthttp.RequestCtx) (string, bool) {
	teamID := strings.TrimSpace(valueFromUserValue(ctx.UserValue("team_id")))
	if teamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team_id is required")
		return "", false
	}
	return teamID, true
}

// readTeamAndProvider extracts both path params and rejects empty values.
// Empty provider is reported as 400 because the policy is keyed by
// (team_id, provider) and a missing provider would silently widen to "all".
func (h *TeamModelPoliciesHandler) readTeamAndProvider(ctx *fasthttp.RequestCtx) (string, string, bool) {
	teamID, ok := h.readTeamID(ctx)
	if !ok {
		return "", "", false
	}
	provider := strings.TrimSpace(valueFromUserValue(ctx.UserValue("provider")))
	if provider == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "provider is required")
		return "", "", false
	}
	return teamID, strings.ToLower(provider), true
}

// valueFromUserValue normalizes the `any` returned by fasthttp's UserValue to
// a plain string. The router stores path params as strings but the API
// surface returns `any`, so we centralize the assertion here.
func valueFromUserValue(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// isAllWildcard reports whether the allowlist is the ["*"] unrestricted
// sentinel — in that case every VK under the team keeps calling any model
// the team's VKs were already configured for, so the D6 cross-check below
// is a no-op.
func isAllWildcard(allowed []string) bool {
	return len(allowed) == 1 && allowed[0] == "*"
}

// validateVKAllowlistsAgainstTeam implements the "VK allowlist ⊆ Team ACL"
// check from model-acl.md §D6. We do a one-shot fetch of the team's VKs
// (paginated), then compare each VK's per-provider allowlist against the
// requested team allowlist; any VK whose allowlist is no longer a subset
// produces an error that lists the offending VK IDs.
//
// The check is intentionally permissive about empty/wildcard VK allowlists:
// an unrestricted VK ("*") always intersects a non-empty team allowlist
// (every team-allowed model is reachable), and an empty VK allowlist
// already denies everything — also a subset of any team allowlist.
//
// Returns nil on success; the returned error string is API-shaped (joined
// comma list of VK IDs) and surfaces directly to the admin UI.
func (h *TeamModelPoliciesHandler) validateVKAllowlistsAgainstTeam(ctx *fasthttp.RequestCtx, teamID string, teamAllowed []string) error {
	vks, _, err := h.configStore.GetVirtualKeysPaginated(ctx, configstore.VirtualKeyQueryParams{TeamID: teamID, Limit: 1000})
	if err != nil {
		return fmt.Errorf("failed to load team virtual keys: %w", err)
	}
	teamSet := make(map[string]struct{}, len(teamAllowed))
	for _, m := range teamAllowed {
		teamSet[strings.ToLower(strings.TrimSpace(m))] = struct{}{}
	}
	var badIDs []string
	for i := range vks {
		vk := &vks[i]
		if vk == nil || vk.ProviderConfigs == nil {
			continue
		}
		hasNonIntersecting := false
		for j := range vk.ProviderConfigs {
			pc := &vk.ProviderConfigs[j]
			allow := pc.AllowedModels
			if len(allow) == 0 || isAllWildcard(allow) {
				continue
			}
			for _, m := range allow {
				if _, ok := teamSet[strings.ToLower(strings.TrimSpace(m))]; !ok {
					hasNonIntersecting = true
					break
				}
			}
			if hasNonIntersecting {
				break
			}
		}
		if hasNonIntersecting {
			badIDs = append(badIDs, vk.ID)
		}
	}
	if len(badIDs) > 0 {
		return fmt.Errorf("team allowlist is narrower than the following virtual keys: %s", strings.Join(badIDs, ","))
	}
	return nil
}
