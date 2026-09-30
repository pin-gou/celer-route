// Package handlers - cache_stats.go serves the Phase 5 cache-observability
// endpoint that the cache-tracker writes into. It returns the current
// process-lifetime counters (hits / misses / saved tokens / saved cost)
// so the admin "cache savings" card can render without waiting for the
// per-hour cache_hit_counts table that lands in a later phase.
//
// /api/cache/stats is admin-only and intentionally read-only: there is no
// reset endpoint because the counter only rolls on process restart and
// admin actions should never force a partial sample to be lost.
package handlers

import (
	"strings"

	"github.com/fasthttp/router"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// CacheStatsProvider is the narrow contract this handler reads from the
// semantic cache plugin. Kept interface-sized so tests can stub the tracker
// without depending on the plugin package.
type CacheStatsProvider interface {
	Stats() CacheStatsSnapshotShape
}

// CacheStatsSnapshotShape is the JSON shape the handler returns. Defined
// here (and copied from semanticcache) to keep the handler decoupled from
// the plugin package — the admin UI only cares about the JSON contract.
type CacheStatsSnapshotShape struct {
	Hits             uint64  `json:"hits"`
	Misses           uint64  `json:"misses"`
	HitRate          float64 `json:"hit_rate"`
	SavedInputTokens uint64  `json:"saved_input_tokens"`
	SavedCost        float64 `json:"saved_cost"`
}

// CacheStatsResolver returns the currently-loaded cache plugin's stats
// surface, or nil when no plugin is loaded. Same lazy-resolve pattern as
// CacheClearerResolver so plugin lifecycle (POST /api/plugins) is honored.
type CacheStatsResolver func() CacheStatsProvider

// CacheStatsHandler is the read-only endpoint that surfaces the in-memory
// cache counters. No configuration — the snapshot is cheap and the handler
// is safe to wire unconditionally.
type CacheStatsHandler struct {
	resolve CacheStatsResolver
}

// NewCacheStatsHandler builds the handler with the resolver injected. The
// resolver may return nil — the handler returns an empty stats payload
// rather than 500 in that scenario, matching the reports handlers' degraded
// behavior.
func NewCacheStatsHandler(resolve CacheStatsResolver) *CacheStatsHandler {
	return &CacheStatsHandler{resolve: resolve}
}

// RegisterRoutes mounts GET /api/cache/stats. Admin-only via the same
// middleware chain as other reports endpoints.
func (h *CacheStatsHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/cache/stats", lib.ChainMiddlewares(h.stats, middlewares...))
}

// stats returns the in-memory tracker snapshot. period is accepted for
// future-compat (cache_hit_counts table) but currently every value is
// process-lifetime; the field is echoed back so the UI can render
// "since boot" without a separate schema migration.
func (h *CacheStatsHandler) stats(ctx *fasthttp.RequestCtx) {
	period := strings.TrimSpace(string(ctx.QueryArgs().Peek("period")))
	if period == "" {
		period = "all"
	}
	var snapshot CacheStatsSnapshotShape
	provider := h.resolve()
	if provider != nil {
		snapshot = provider.Stats()
	}
	SendJSON(ctx, map[string]any{
		"period":             period,
		"hits":               snapshot.Hits,
		"misses":             snapshot.Misses,
		"hit_rate":           snapshot.HitRate,
		"saved_input_tokens": snapshot.SavedInputTokens,
		"saved_cost":         snapshot.SavedCost,
		"note":               "counters are process-lifetime; reset on restart",
	})
}
