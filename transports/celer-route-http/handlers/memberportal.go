package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/fasthttp/router"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// MemberPortalHandler exposes the member-self-service endpoints from
// temp/team/01-identity/api.md §6. The whole surface sits behind
// MemberAuthMiddleware — every handler pulls the user id from
// BifrostContextKeyMemberUserID and never trusts a user_id supplied
// from the request body. Provider keys are NEVER returned from any of
// these handlers; the boundary from data-model.md §5.1 is enforced
// here at the wire layer, not just at the store.
type MemberPortalHandler struct {
	store    configstore.ConfigStore
	logStore logstore.LogStore
}

// NewMemberPortalHandler builds the handler. store must be non-nil.
// logStore may be nil — when the gateway runs without log persistence,
// the /usage endpoint degrades to counts only (no cost / token histogram).
func NewMemberPortalHandler(store configstore.ConfigStore, logStore logstore.LogStore) *MemberPortalHandler {
	return &MemberPortalHandler{store: store, logStore: logStore}
}

// RegisterRoutes wires the four member-only endpoints. The setup-guide
// endpoint deliberately does NOT call out to a real provider — it just
// renders the openai-compatible base URL so the SDK can talk to the
// gateway.
func (h *MemberPortalHandler) RegisterRoutes(r *router.Router, memberMW ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/member/virtual-keys", lib.ChainMiddlewares(h.listVirtualKeys, memberMW...))
	r.GET("/api/member/virtual-keys/{vk_id}/quota", lib.ChainMiddlewares(h.virtualKeyQuota, memberMW...))
	r.GET("/api/member/usage", lib.ChainMiddlewares(h.usage, memberMW...))
	r.GET("/api/member/setup-guide", lib.ChainMiddlewares(h.setupGuide, memberMW...))
}

// listVirtualKeys handles GET /api/member/virtual-keys. Returns the
// VKs owned by the current member, desensitized (no value / no
// provider key refs). The store's ListVirtualKeysByUserID already
// strips out anything sensitive; we filter to only "active, not
// expired" here so the member portal doesn't show VKs that are
// scheduled to be turned off.
func (h *MemberPortalHandler) listVirtualKeys(ctx *fasthttp.RequestCtx) {
	userID, ok := ctx.UserValue(schemas.BifrostContextKeyMemberUserID).(string)
	if !ok || userID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	vks, _, err := h.store.ListVirtualKeysByUserID(ctx, userID, 200, 0)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to list virtual keys: %v", err))
		return
	}
	now := time.Now().UTC()
	out := make([]map[string]any, 0, len(vks))
	for _, vk := range vks {
		if !vk.IsActiveValue() {
			continue
		}
		if vk.IsExpiredAt(now) {
			continue
		}
		out = append(out, map[string]any{
			"id":         vk.ID,
			"name":       vk.Name,
			"team_id":    vk.TeamID,
			"is_active":  vk.IsActiveValue(),
			"expires_at": timeOrNil(vk.ExpiresAt),
			"updated_at": vk.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	SendJSON(ctx, map[string]any{"virtual_keys": out, "count": len(out)})
}

// virtualKeyQuota handles GET /api/member/virtual-keys/:vk_id/quota.
// Scoped to "the current member owns this VK"; 404 if not yours so the
// endpoint doesn't leak the existence of someone else's VK id. We
// return a lightweight view that calls out ownership + lifetime; the
// full budget-vs-usage breakdown is the same numbers the public
// /api/governance/virtual-keys/quota endpoint returns for the same VK,
// so the UI can fetch that path on demand when it needs the math.
func (h *MemberPortalHandler) virtualKeyQuota(ctx *fasthttp.RequestCtx) {
	userID, ok := ctx.UserValue(schemas.BifrostContextKeyMemberUserID).(string)
	if !ok || userID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	vkID := ctx.UserValue("vk_id").(string)
	if vkID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "vk_id is required")
		return
	}
	vk, err := h.store.GetVirtualKey(ctx, vkID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load virtual key: %v", err))
		return
	}
	if vk == nil || vk.UserID == nil || *vk.UserID != userID {
		// Collapse "missing" and "not yours" into 404 to avoid leaking
		// the existence of someone else's VK id.
		SendError(ctx, fasthttp.StatusNotFound, "Virtual key not found")
		return
	}
	SendJSON(ctx, map[string]any{
		"virtual_key_id": vk.ID,
		"name":           vk.Name,
		"is_active":      vk.IsActiveValue(),
		"team_id":        vk.TeamID,
		"expires_at":     timeOrNil(vk.ExpiresAt),
	})
}

// usage handles GET /api/member/usage. Returns the current month's cost
// and token histograms for the member's own virtual keys. The handler
// fetches the member's VKs from the config store, then queries the log
// store filtered by those VK IDs — so a member can never see another
// member's usage, and the data comes from the same source the admin
// /api/logs/histogram uses (no separate aggregation pipeline).
//
// When the log store is nil (gateway running without log persistence),
// the handler degrades to just the user profile + VK count; the UI
// renders an empty state instead of erroring.
func (h *MemberPortalHandler) usage(ctx *fasthttp.RequestCtx) {
	userID, ok := ctx.UserValue(schemas.BifrostContextKeyMemberUserID).(string)
	if !ok || userID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	user, err := h.store.GetUserByID(ctx, userID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to load user: %v", err))
		return
	}
	if user == nil {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}

	// Get the member's VKs — we filter the log query by these IDs so
	// the member never sees traffic from VKs they don't own.
	vks, vkCount, err := h.store.ListVirtualKeysByUserID(ctx, userID, 100, 0)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to list virtual keys: %v", err))
		return
	}

	// Build the "current calendar month" window for the histogram.
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	filters := logstore.SearchFilters{
		StartTime: &monthStart,
		EndTime:   &now,
	}
	vkIDs := make([]string, 0, len(vks))
	for _, vk := range vks {
		vkIDs = append(vkIDs, vk.ID)
	}
	filters.VirtualKeyIDs = vkIDs

	usage := map[string]any{
		"vk_count":     vkCount,
		"period_start": monthStart.Format(time.RFC3339),
		"period_end":   now.Format(time.RFC3339),
	}

	if h.logStore != nil && len(vkIDs) > 0 {
		bucket := calculateBucketSize(&monthStart, &now)

		// Cost histogram: total cost per bucket with model breakdown.
		costHist, err := h.logStore.GetCostHistogram(ctx, filters, bucket)
		if err == nil && costHist != nil {
			totalCost := 0.0
			for _, b := range costHist.Buckets {
				totalCost += b.TotalCost
			}
			usage["total_cost"] = totalCost
			usage["cost_buckets"] = costHist.Buckets
			usage["bucket_size_seconds"] = costHist.BucketSizeSeconds
			usage["models"] = costHist.Models
		}

		// Token histogram: prompt / completion / total per bucket.
		tokenHist, err := h.logStore.GetTokenHistogram(ctx, filters, bucket)
		if err == nil && tokenHist != nil {
			var prompt, completion, total int64
			for _, b := range tokenHist.Buckets {
				prompt += b.PromptTokens
				completion += b.CompletionTokens
				total += b.TotalTokens
			}
			usage["prompt_tokens"] = prompt
			usage["completion_tokens"] = completion
			usage["total_tokens"] = total
			usage["token_buckets"] = tokenHist.Buckets
		}

		// Request count histogram.
		reqHist, err := h.logStore.GetHistogram(ctx, filters, bucket)
		if err == nil && reqHist != nil {
			var requestCount int64
			for _, b := range reqHist.Buckets {
				requestCount += b.Count
			}
			usage["request_count"] = requestCount
		}
	}

	SendJSON(ctx, map[string]any{
		"user_id":       user.ID,
		"email":         user.Email,
		"last_login_at": timeOrNil(user.LastLoginAt),
		"usage":         usage,
	})
}

// setupGuide handles GET /api/member/setup-guide. Returns the openai-
// compatible base URL plus an SDK example; never includes provider
// credentials of any kind.
func (h *MemberPortalHandler) setupGuide(ctx *fasthttp.RequestCtx) {
	userID, ok := ctx.UserValue(schemas.BifrostContextKeyMemberUserID).(string)
	if !ok || userID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	host := string(ctx.Request.Host())
	scheme := "http"
	if string(ctx.Request.Header.Peek("X-Forwarded-Proto")) == "https" {
		scheme = "https"
	}
	base := scheme + "://" + strings.TrimSpace(host)
	vks, _, err := h.store.ListVirtualKeysByUserID(ctx, userID, 50, 0)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to list virtual keys: %v", err))
		return
	}
	keys := make([]map[string]any, 0, len(vks))
	for _, vk := range vks {
		if !vk.IsActiveValue() || vk.IsExpiredAt(time.Now().UTC()) {
			continue
		}
		keys = append(keys, map[string]any{
			"id":   vk.ID,
			"name": vk.Name,
		})
	}
	SendJSON(ctx, map[string]any{
		"base_url":        base + "/v1",
		"compatible_with": "openai",
		"available_keys":  keys,
		"example_request": fmt.Sprintf("curl %s/v1/chat/completions -H 'Authorization: Bearer <your_vk_value>' -H 'Content-Type: application/json' -d '{\"model\":\"...\",\"messages\":[...]}'", base),
		"docs":            "https://github.com/pin-gou/celer-route",
	})
}
