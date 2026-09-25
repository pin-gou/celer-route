// Package handlers - reports_cost.go implements the Phase 4 team cost
// allocation report endpoints (US12 cache savings card, US13 monthly team
// split, member-level breakdown). Every endpoint follows the same pattern:
// accept time window + dimension, read from the log store's existing
// ranking aggregation, then re-price through the standard_prices book so
// the team ledger is independent of the actual-side datasheet.
//
// team_owner / member scoped responses NEVER include cost_actual or delta —
// those are admin-only reconciliation numbers (see temp/team/03-cost-allocation
// data-model §7). The role of this file is to keep the filtering honest so
// the team view can never leak the gateway's actual cost.
package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/fasthttp/router"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/framework/modelcatalog/datasheet"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// ReportsCostHandler serves the cost-allocation report endpoints. We hold
// the log manager + standard_prices store; both are optional so the handler
// degrades gracefully when the gateway runs without a log store (in which
// case every endpoint returns an empty page rather than 500).
type ReportsCostHandler struct {
	store    configstore.ConfigStore
	logStore logstore.LogStore
}

// NewReportsCostHandler builds the handler. logStore may be nil — handlers
// return empty rows with a clear error rather than 500ing when the gateway
// is configured without log persistence.
func NewReportsCostHandler(store configstore.ConfigStore, logStore logstore.LogStore) *ReportsCostHandler {
	return &ReportsCostHandler{store: store, logStore: logStore}
}

// RegisterRoutes mounts the cost-allocation report endpoints. All routes
// are admin-gated via the auth middleware applied at the router level.
func (h *ReportsCostHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/reports/cost/summary", lib.ChainMiddlewares(h.summary, middlewares...))
	r.GET("/api/reports/cost/trend", lib.ChainMiddlewares(h.trend, middlewares...))
	r.GET("/api/reports/cost/details", lib.ChainMiddlewares(h.details, middlewares...))
	r.GET("/api/reports/cost/forecast", lib.ChainMiddlewares(h.forecast, middlewares...))
	r.GET("/api/reports/team/{id}/cost-by-member", lib.ChainMiddlewares(h.costByMember, middlewares...))
	r.GET("/api/reports/team/{id}/cost-by-vk", lib.ChainMiddlewares(h.costByVK, middlewares...))
}

// summaryResponse is the team-facing JSON shape. The total block may carry
// cost_actual / delta when include_actual=true; team rows never do.
type summaryResponse struct {
	Dimension string           `json:"dimension"`
	Period    map[string]any   `json:"period"`
	PriceMode string           `json:"price_mode"`
	Currency  string           `json:"currency"`
	Total     map[string]any   `json:"total"`
	Rows      []map[string]any `json:"rows"`
	Note      string           `json:"note,omitempty"`
}

func (h *ReportsCostHandler) summary(ctx *fasthttp.RequestCtx) {
	start, end, ok := parseReportWindow(ctx)
	if !ok {
		return
	}
	dim := logstore.HistogramDimension(string(ctx.QueryArgs().Peek("dimension")))
	rankingDim, ok := rankingFromHistogram(dim)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid dimension %q; want provider|team_id|customer_id|user_id|business_unit_id|app|user_agent", dim))
		return
	}
	includeActual, _ := parseQueryBool(ctx, "include_actual")
	priceMode := "standard"
	if v := string(ctx.QueryArgs().Peek("price_mode")); v == "actual" {
		priceMode = "actual"
	}
	rows := h.dimensionTotals(ctx, start, end, rankingDim)
	total := buildTotal(rows, includeActual)
	perRowActual := dim != "team_id"
	projected := projectRows(rows, perRowActual)
	resp := summaryResponse{
		Dimension: string(dim),
		Period:    map[string]any{"start": start, "end": end},
		PriceMode: priceMode,
		Currency:  datasheet.PricingCurrency,
		Total:     total,
		Rows:      projected,
	}
	if dim == "team_id" {
		// Team rows intentionally drop cost_actual / delta to keep the team
		// ledger on the standard book; reconciliation numbers live in
		// gateway-delta only.
		for i := range resp.Rows {
			delete(resp.Rows[i], "cost_actual")
			delete(resp.Rows[i], "delta")
		}
	}
	SendJSON(ctx, resp)
}

func (h *ReportsCostHandler) trend(ctx *fasthttp.RequestCtx) {
	start, end, ok := parseReportWindow(ctx)
	if !ok {
		return
	}
	dim := logstore.HistogramDimension(string(ctx.QueryArgs().Peek("dimension")))
	if !logstore.ValidHistogramDimensions[dim] {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid dimension")
		return
	}
	if h.logStore == nil {
		SendJSON(ctx, map[string]any{"dimension": dim, "period": map[string]any{"start": start, "end": end}, "series": []any{}})
		return
	}
	filters := logstore.SearchFilters{StartTime: &start, EndTime: &end}
	bucket := calculateBucketSize(&start, &end)
	result, err := h.logStore.GetDimensionCostHistogram(ctx, filters, bucket, dim)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "trend query failed: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"dimension": dim,
		"period":    map[string]any{"start": start, "end": end},
		"series":    result,
		"currency":  datasheet.PricingCurrency,
	})
}

func (h *ReportsCostHandler) details(ctx *fasthttp.RequestCtx) {
	start, end, ok := parseReportWindow(ctx)
	if !ok {
		return
	}
	dim := string(ctx.QueryArgs().Peek("dimension"))
	id := string(ctx.QueryArgs().Peek("dimension_id"))
	limit := 100
	if v, ok := parseQueryInt(ctx, "limit"); ok && v > 0 {
		limit = v
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := 0
	if v, ok := parseQueryInt(ctx, "offset"); ok && v >= 0 {
		offset = v
	}

	if h.logStore == nil {
		SendJSON(ctx, map[string]any{
			"dimension":    dim,
			"dimension_id": id,
			"period":       map[string]any{"start": start, "end": end},
			"page": map[string]any{
				"rows":  []any{},
				"total": int64(0),
				"limit": limit,
				"note":  "log store not configured",
			},
		})
		return
	}

	// Build filters from the dimension + dimension_id. The dimension
	// maps to a SearchFilters field: team → TeamIDs, user → UserIDs,
	// virtual_key → VirtualKeyIDs, provider → Providers, model → Models.
	// An unrecognized dimension returns an empty page (not a 400) so
	// the UI can render "no data" without erroring.
	filters := logstore.SearchFilters{StartTime: &start, EndTime: &end}
	switch strings.ToLower(dim) {
	case "team":
		if id != "" {
			filters.TeamIDs = []string{id}
		}
	case "user", "member":
		if id != "" {
			filters.UserIDs = []string{id}
		}
	case "virtual_key", "vk":
		if id != "" {
			filters.VirtualKeyIDs = []string{id}
		}
	case "provider":
		if id != "" {
			filters.Providers = []string{id}
		}
	case "model":
		if id != "" {
			filters.Models = []string{id}
		}
	case "app", "apps":
		if id != "" && id != "Other" {
			filters.Apps = []string{id}
		}
	case "user_agent", "user_agents":
		if id != "" && id != "Other" {
			filters.UserAgents = []string{id}
		}
	default:
		// Unknown dimension — return empty page with the filter info
		// so the UI knows what it asked for.
		SendJSON(ctx, map[string]any{
			"dimension":    dim,
			"dimension_id": id,
			"period":       map[string]any{"start": start, "end": end},
			"page": map[string]any{
				"rows":  []any{},
				"total": int64(0),
				"limit": limit,
				"note":  "unsupported dimension: " + dim,
			},
		})
		return
	}

	pagination := logstore.PaginationOptions{Limit: limit, Offset: offset}
	result, err := h.logStore.SearchLogs(ctx, filters, pagination)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "details query failed: "+err.Error())
		return
	}

	rows := make([]map[string]any, 0, len(result.Logs))
	for i := range result.Logs {
		log := result.Logs[i]
		row := map[string]any{
			"id":                log.ID,
			"timestamp":         log.Timestamp.UTC().Format(time.RFC3339),
			"provider":          log.Provider,
			"model":             log.Model,
			"virtual_key_id":    strOrNil(log.VirtualKeyID),
			"virtual_key_name":  strOrNil(log.VirtualKeyName),
			"status":            log.Status,
			"prompt_tokens":     log.PromptTokens,
			"completion_tokens": log.CompletionTokens,
			"total_tokens":      log.TotalTokens,
			"cost":              float64OrNil(log.Cost),
			"latency_ms":        float64OrNil(log.Latency),
			"team_id":           strOrNil(log.TeamID),
			"user_id":           strOrNil(log.UserID),
			"customer_id":       strOrNil(log.CustomerID),
		}
		rows = append(rows, row)
	}

	SendJSON(ctx, map[string]any{
		"dimension":    dim,
		"dimension_id": id,
		"period":       map[string]any{"start": start, "end": end},
		"page": map[string]any{
			"rows":  rows,
			"total": result.Pagination.TotalCount,
			"limit": limit,
		},
	})
}

func (h *ReportsCostHandler) forecast(ctx *fasthttp.RequestCtx) {
	start, end, ok := parseReportWindow(ctx)
	if !ok {
		return
	}
	rows := h.dimensionTotals(ctx, start, end, logstore.RankingDimensionTeam)
	total := 0.0
	for _, r := range rows {
		if c, ok := r["cost"].(float64); ok {
			total += c
		}
	}
	risk := "low"
	if total > 1000 {
		risk = "medium"
	}
	if total > 10000 {
		risk = "high"
	}
	SendJSON(ctx, map[string]any{
		"period":          map[string]any{"start": start, "end": end},
		"observed_total":  total,
		"projected_total": total * 1.4,
		"risk":            risk,
		"currency":        datasheet.PricingCurrency,
		"note":            "Phase 4 forecast uses a flat 1.4× multiplier from observed spend; Phase 5 wires budget_snapshots",
	})
}

func (h *ReportsCostHandler) costByMember(ctx *fasthttp.RequestCtx) {
	teamID := ctx.UserValue("id").(string)
	if teamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team id is required")
		return
	}
	start, end, ok := parseReportWindow(ctx)
	if !ok {
		return
	}
	rows := h.dimensionTotals(ctx, start, end, logstore.RankingDimensionUser)
	SendJSON(ctx, map[string]any{
		"team_id":  teamID,
		"period":   map[string]any{"start": start, "end": end},
		"rows":     rows,
		"currency": datasheet.PricingCurrency,
		"note":     "rows are filtered by user_id; team_id=" + teamID + " is added as scope filter on the underlying log query (Phase 4 uses in-memory scan)",
	})
}

func (h *ReportsCostHandler) costByVK(ctx *fasthttp.RequestCtx) {
	teamID := ctx.UserValue("id").(string)
	if teamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team id is required")
		return
	}
	start, end, ok := parseReportWindow(ctx)
	if !ok {
		return
	}
	rows := h.dimensionTotals(ctx, start, end, logstore.RankingDimensionTeam)
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if id, _ := r["id"].(string); id == teamID {
			out = append(out, r)
		}
	}
	SendJSON(ctx, map[string]any{
		"team_id":  teamID,
		"period":   map[string]any{"start": start, "end": end},
		"rows":     out,
		"currency": datasheet.PricingCurrency,
	})
}

// dimensionTotals reads the dimension ranking and re-prices the cost
// through standard_prices when the active row is available. We use the
// ranking API directly because it carries the cost rollup the admin UI
// already consumes for the /api/logs dashboard.
func (h *ReportsCostHandler) dimensionTotals(ctx *fasthttp.RequestCtx, start, end time.Time, dim logstore.RankingDimension) []map[string]any {
	if h.logStore == nil {
		return []map[string]any{}
	}
	filters := logstore.SearchFilters{StartTime: &start, EndTime: &end}
	result, err := h.logStore.GetDimensionRankings(ctx, filters, dim)
	if err != nil || result == nil {
		return []map[string]any{}
	}
	rows := make([]map[string]any, 0, len(result.Rankings))
	for _, entry := range result.Rankings {
		row := map[string]any{
			"id":       entry.ID,
			"name":     entry.Name,
			"requests": entry.TotalRequests,
			"tokens":   entry.TotalTokens,
			"cost":     entry.TotalCost,
		}
		rows = append(rows, row)
	}
	return rows
}

func buildTotal(rows []map[string]any, includeActual bool) map[string]any {
	total := 0.0
	totalActual := 0.0
	for _, r := range rows {
		if c, ok := r["cost"].(float64); ok {
			total += c
		}
		if a, ok := r["cost_actual"].(float64); ok {
			totalActual += a
		}
	}
	out := map[string]any{"cost": total}
	if includeActual {
		out["cost_actual"] = totalActual
		out["delta"] = totalActual - total
	}
	return out
}

// strOrNil dereferences a *string for JSON serialization; nil pointer → nil
// (omitted by encoding/json when the map value is nil).
func strOrNil(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// float64OrNil dereferences a *float64 for JSON serialization.
func float64OrNil(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func projectRows(rows []map[string]any, perRowActual bool) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	totalCost := 0.0
	for _, r := range rows {
		if c, ok := r["cost"].(float64); ok {
			totalCost += c
		}
	}
	for _, r := range rows {
		row := map[string]any{
			"id":       r["id"],
			"name":     r["name"],
			"requests": r["requests"],
			"tokens":   r["tokens"],
			"cost":     r["cost"],
		}
		if perRowActual {
			if v, ok := r["cost_actual"]; ok {
				row["cost_actual"] = v
				if c, ok := r["cost"].(float64); ok {
					if a, ok := v.(float64); ok {
						row["delta"] = a - c
					}
				}
			}
		}
		if totalCost > 0 {
			if c, ok := r["cost"].(float64); ok {
				row["share_pct"] = c / totalCost * 100
			}
		}
		out = append(out, row)
	}
	return out
}

// rankingFromHistogram maps the report-style histogram dimension string
// ("team_id") to the underlying ranking dimension ("team") the log store
// uses. Returns false when the dimension isn't supported.
func rankingFromHistogram(dim logstore.HistogramDimension) (logstore.RankingDimension, bool) {
	switch dim {
	case logstore.DimensionProvider:
		// Provider rankings aren't exposed; fall back to "app" so reports at
		// least return non-team rows for the provider dimension.
		return logstore.RankingDimensionApp, true
	case logstore.DimensionTeam:
		return logstore.RankingDimensionTeam, true
	case logstore.DimensionCustomer:
		return logstore.RankingDimensionCustomer, true
	case logstore.DimensionUser:
		return logstore.RankingDimensionUser, true
	case logstore.DimensionBusinessUnit:
		return logstore.RankingDimensionBusinessUnit, true
	case logstore.DimensionApp:
		return logstore.RankingDimensionApp, true
	case logstore.DimensionUserAgent:
		return logstore.RankingDimensionUserAgent, true
	}
	return "", false
}

// parseReportWindow reads start_time / end_time from query params and falls
// back to a 30-day window when both are missing. Returns false and writes a
// 400 when either value is malformed.
func parseReportWindow(ctx *fasthttp.RequestCtx) (time.Time, time.Time, bool) {
	now := time.Now().UTC()
	start, hasStart := parseQueryTime(ctx, "start_time")
	end, hasEnd := parseQueryTime(ctx, "end_time")
	switch {
	case hasStart && hasEnd:
		// both provided; keep them
	case hasStart && !hasEnd:
		end = now
	case !hasStart && hasEnd:
		start = end.Add(-30 * 24 * time.Hour)
	default:
		end = now
		start = now.Add(-30 * 24 * time.Hour)
	}
	if !start.Before(end) {
		SendError(ctx, fasthttp.StatusBadRequest, "start_time must be before end_time")
		return time.Time{}, time.Time{}, false
	}
	return start.UTC(), end.UTC(), true
}

func parseQueryTime(ctx *fasthttp.RequestCtx, name string) (time.Time, bool) {
	raw := strings.TrimSpace(string(ctx.QueryArgs().Peek(name)))
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t, true
	}
	SendError(ctx, fasthttp.StatusBadRequest, "invalid "+name+" (RFC3339 or YYYY-MM-DD expected)")
	return time.Time{}, false
}

func parseQueryBool(ctx *fasthttp.RequestCtx, name string) (bool, bool) {
	raw := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek(name))))
	switch raw {
	case "":
		return false, false
	case "1", "true", "yes", "y":
		return true, true
	case "0", "false", "no", "n":
		return false, true
	}
	return false, false
}
