// Package handlers - reports_cache.go implements the Phase 5 cache-savings
// report (03-cost-allocation api.md §1: GET /api/reports/cache/savings).
//
// It answers "how much did the semantic cache save us in this window?" from
// two independent sources, and reports both rather than picking one:
//
//   - The **log store** gives the window-scoped view: how many completed
//     requests were served from cache, how many input tokens those requests
//     carried, and an estimate of what those tokens would have cost.
//   - The **in-process tracker** (semanticcache.CacheStatsTracker) gives the
//     process-lifetime view — the same numbers /api/cache/stats exposes, so
//     the two endpoints can never disagree about "since boot".
//
// The window estimate is deliberately conservative: the missed-request cost
// per input token is derived from the same window, so a cache that is
// absorbing the cheap traffic doesn't get credited with the expensive
// traffic's savings.
package handlers

import (
	"fmt"

	"github.com/fasthttp/router"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/framework/modelcatalog/datasheet"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// cacheHitTypes are the cache_debug hit_type values the semantic cache
// writes. Mirrors SearchFilters.CacheHitTypes' accepted set — the store
// drops anything else.
var cacheHitTypes = []string{"direct", "semantic"}

// ReportsCacheHandler serves the cache-savings report. logStore is required
// for the window view; the stats resolver is optional (a build without the
// semantic cache plugin loaded simply omits the tracker block).
type ReportsCacheHandler struct {
	logStore logstore.LogStore
	resolve  CacheStatsResolver
}

// NewReportsCacheHandler builds the handler. resolve may be nil.
func NewReportsCacheHandler(logStore logstore.LogStore, resolve CacheStatsResolver) *ReportsCacheHandler {
	return &ReportsCacheHandler{logStore: logStore, resolve: resolve}
}

// RegisterRoutes mounts the cache-savings report.
func (h *ReportsCacheHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/reports/cache/savings", lib.ChainMiddlewares(h.savings, middlewares...))
}

// cacheSavingsResponse is the wire shape.
type cacheSavingsResponse struct {
	Period       map[string]any `json:"period"`
	Currency     string         `json:"currency"`
	Hits         int64          `json:"hits"`
	DirectHits   int64          `json:"direct_hits"`
	SemanticHits int64          `json:"semantic_hits"`
	Requests     int64          `json:"requests"`
	HitRate      float64        `json:"hit_rate"`

	SavedInputTokens   int64   `json:"saved_input_tokens"`
	EstimatedSavedCost float64 `json:"estimated_saved_cost"`

	// ActualCost and PromptTokens for the window are echoed so the estimate
	// is auditable: an operator can recompute estimated_saved_cost from
	// these two plus saved_input_tokens.
	ActualCost   float64 `json:"actual_cost"`
	PromptTokens int64   `json:"prompt_tokens"`

	Tracker map[string]any `json:"tracker,omitempty"`
	Note    string         `json:"note,omitempty"`
}

func (h *ReportsCacheHandler) savings(ctx *fasthttp.RequestCtx) {
	start, end, ok := parseReportWindow(ctx)
	if !ok {
		return
	}
	resp := cacheSavingsResponse{
		Period:   map[string]any{"start": start, "end": end},
		Currency: datasheet.PricingCurrency,
	}
	if h.logStore != nil {
		all, err := h.logStore.GetStats(ctx, logstore.SearchFilters{StartTime: &start, EndTime: &end})
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "cache savings stats: "+err.Error())
			return
		}
		hits, err := h.logStore.GetStats(ctx, logstore.SearchFilters{
			StartTime:     &start,
			EndTime:       &end,
			CacheHitTypes: cacheHitTypes,
		})
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "cache savings hit stats: "+err.Error())
			return
		}
		applyCacheSavings(&resp, all, hits)
	} else {
		resp.Note = "log store not configured; only the process-lifetime tracker is available"
	}

	if h.resolve != nil {
		if provider := h.resolve(); provider != nil {
			snap := provider.Stats()
			resp.Tracker = map[string]any{
				"hits":               snap.Hits,
				"misses":             snap.Misses,
				"hit_rate":           snap.HitRate,
				"saved_input_tokens": snap.SavedInputTokens,
				"saved_cost":         snap.SavedCost,
			}
		}
	}
	if resp.Note == "" {
		resp.Note = "estimated_saved_cost prices the cached input tokens at the window's missed-request cost per input token (actual_cost / non-cached prompt tokens); tracker counters are process-lifetime"
	}
	SendJSON(ctx, resp)
}

// applyCacheSavings folds the two log-store aggregates into the response.
//
// The hit count comes from the cache_debug-derived counters on the full
// window aggregate (DirectCacheHits / SemanticCacheHits) so it stays
// consistent with the dashboard's cache-hit card; the token figure needs a
// second, cache-filtered aggregate because SearchStats does not expose
// prompt tokens per hit type.
func applyCacheSavings(resp *cacheSavingsResponse, all, hits *logstore.SearchStats) {
	if all == nil || hits == nil {
		return
	}
	resp.Requests = all.TotalRequests
	resp.PromptTokens = all.PromptTokens
	resp.ActualCost = all.TotalCost

	if all.DirectCacheHits != nil {
		resp.DirectHits = *all.DirectCacheHits
	}
	if all.SemanticCacheHits != nil {
		resp.SemanticHits = *all.SemanticCacheHits
	}
	resp.Hits = resp.DirectHits + resp.SemanticHits
	if resp.Hits == 0 {
		// Fall back to the filtered aggregate when the counters are absent —
		// a store that predates the cache_debug counters still knows how many
		// rows matched the hit filter.
		resp.Hits = hits.TotalRequests
	}
	resp.SavedInputTokens = hits.PromptTokens

	denominator := all.TotalRequests
	if all.CacheHitRateTotalRequests != nil && *all.CacheHitRateTotalRequests > 0 {
		denominator = *all.CacheHitRateTotalRequests
	}
	if denominator > 0 {
		resp.HitRate = float64(resp.Hits) / float64(denominator)
	}

	// Cost per input token on the *missed* requests. Using the whole window's
	// actual cost over the whole window's prompt tokens would understate the
	// price of a miss, because hits contribute tokens but no cost.
	missedPromptTokens := all.PromptTokens - hits.PromptTokens
	if missedPromptTokens > 0 && resp.SavedInputTokens > 0 {
		costPerPromptToken := all.TotalCost / float64(missedPromptTokens)
		resp.EstimatedSavedCost = float64(resp.SavedInputTokens) * costPerPromptToken
	}
	if resp.Hits > 0 && missedPromptTokens <= 0 {
		resp.Note = fmt.Sprintf(
			"every prompt token in this window came from cache (%d hits), so there is no missed-request rate to price the savings against; estimated_saved_cost is 0",
			resp.Hits)
	}
}
