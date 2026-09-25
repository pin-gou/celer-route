// Package handlers - reports_idle_keys.go implements the US24 idle-VK report.
//
// US24 is the "定期发现闲置 key" admin story: the admin opens
// /workspace/keys or the dedicated /api/reports/idle-keys page and sees
// every virtual key that has not been used for N days (default 30, capped
// at 365 so a typo never lands on the "everything is idle" branch).
//
// The endpoint is admin-only — it would be a privacy problem to expose
// "this member's VK has been idle for 90 days" to non-admins. Provider
// keys are explicitly out of scope (see temp/team/01-identity/data-model.md
// §5.1).
package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/fasthttp/router"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// IdleKeysHandler serves GET /api/reports/idle-keys. The handler is a
// thin wrapper over ConfigStore.ListIdleVirtualKeys; it never touches the
// log store directly because the idle-VK sidekiq job (US24) is what
// refreshes governance_virtual_keys.last_used_at.
type IdleKeysHandler struct {
	store configstore.ConfigStore
}

// NewIdleKeysHandler builds the handler.
func NewIdleKeysHandler(store configstore.ConfigStore) *IdleKeysHandler {
	return &IdleKeysHandler{store: store}
}

// RegisterRoutes mounts the endpoint.
func (h *IdleKeysHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/reports/idle-keys", lib.ChainMiddlewares(h.listIdle, middlewares...))
}

// idleKeysResponse is the wire shape. We expose the bare minimum the UI
// needs to render the idle list — never the VK value, never the provider
// key the VK references (those live behind separate endpoints that the
// admin's own session can reach).
type idleKeysResponse struct {
	Period    map[string]any     `json:"period"`
	Threshold string             `json:"threshold"`
	Total     int64              `json:"total"`
	Rows      []map[string]any   `json:"rows"`
	Limit     int                `json:"limit"`
	Offset    int                `json:"offset"`
}

func (h *IdleKeysHandler) listIdle(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	threshold := idleKeysThreshold(ctx, 30*24*time.Hour)
	limit := idleKeysInt(ctx, "limit", 100, 1, 500)
	offset := idleKeysInt(ctx, "offset", 0, 0, 100000)
	rows, total, err := h.store.ListIdleVirtualKeys(ctx, threshold, limit, offset)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "list idle virtual keys: "+err.Error())
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		row := map[string]any{
			"id":         rows[i].ID,
			"name":       rows[i].Name,
			"description": rows[i].Description,
			"is_active":  rows[i].IsActiveValue(),
			"team_id":    rows[i].TeamID,
			"user_id":    rows[i].UserID,
			"customer_id": rows[i].CustomerID,
			"last_used_at": timeOrNil(rows[i].LastUsedAt),
			"created_at":  rows[i].CreatedAt.UTC().Format(time.RFC3339),
			"updated_at":  rows[i].UpdatedAt.UTC().Format(time.RFC3339),
		}
		out = append(out, row)
	}
	SendJSON(ctx, idleKeysResponse{
		Period: map[string]any{
			"now":       time.Now().UTC(),
			"threshold": threshold.UTC(),
		},
		Threshold: threshold.UTC().Format(time.RFC3339),
		Total:     total,
		Rows:      out,
		Limit:     limit,
		Offset:    offset,
	})
}

// idleKeysThreshold parses the `idle_days` query param and converts it to
// a "last_used_at must be older than" timestamp. Bounds are intentionally
// wide so an operator can scan "everything idle for at least one day" or
// "everything idle for at least a year". A zero or unparseable value
// falls back to the default.
func idleKeysThreshold(ctx *fasthttp.RequestCtx, def time.Duration) time.Time {
	raw := strings.TrimSpace(string(ctx.QueryArgs().Peek("idle_days")))
	if raw == "" {
		return time.Now().UTC().Add(-def)
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return time.Now().UTC().Add(-def)
	}
	if n > 365 {
		n = 365
	}
	return time.Now().UTC().Add(-time.Duration(n) * 24 * time.Hour)
}

// idleKeysInt parses an integer query param with bounds. Out-of-range
// values are silently clamped — the report is read-only, so refusing the
// request would be heavier than just bounding the input.
func idleKeysInt(ctx *fasthttp.RequestCtx, name string, def, lo, hi int) int {
	raw := strings.TrimSpace(string(ctx.QueryArgs().Peek(name)))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}