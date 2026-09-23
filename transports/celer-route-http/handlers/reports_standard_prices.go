// Package handlers - reports_standard_prices.go implements the admin API for
// the standard_prices price book (Phase 4 cost-allocation D7/D11): list /
// create / delete individual price-book rows, sync from the actual-side
// datasheet so the team ledger never silently goes to zero for a freshly
// shipped model, and the preview endpoint that lets an admin see what a
// rate change would do to last month's costs before committing.
//
// Reads (GET) are admin-only via the auth middleware applied at the router
// level — see transports/celer-route-http/server/server.go. Writes (PUT /
// DELETE) are admin-only too; team owners / members must not see or touch
// the price book (team ledger always reports whatever book the admin has
// configured).
package handlers

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/router"
	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/modelcatalog/datasheet"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// StandardPriceHandler exposes the standard_prices price-book admin API.
// Sync / preview require the actual-side pricing store so we can derive
// the team ledger from the existing datasheet; when the store is nil the
// handler degrades to read + write only.
type StandardPriceHandler struct {
	store configstore.ConfigStore
}

// NewStandardPriceHandler builds the handler. The pricingStore is the
// existing datasheet Store used for actual-cost lookups; pass nil if you
// only want read + CRUD without sync / preview.
func NewStandardPriceHandler(store configstore.ConfigStore) *StandardPriceHandler {
	return &StandardPriceHandler{store: store}
}

// RegisterRoutes mounts the standard_prices + team_pricing_profiles admin
// surface. All routes are admin-gated via the auth middleware applied at
// the router level.
func (h *StandardPriceHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/reports/standard-prices", lib.ChainMiddlewares(h.list, middlewares...))
	r.PUT("/api/reports/standard-prices", lib.ChainMiddlewares(h.upsert, middlewares...))
	r.DELETE("/api/reports/standard-prices/{id}", lib.ChainMiddlewares(h.delete, middlewares...))
	r.POST("/api/reports/standard-prices/sync", lib.ChainMiddlewares(h.sync, middlewares...))
	r.GET("/api/reports/standard-prices/preview", lib.ChainMiddlewares(h.preview, middlewares...))
	r.GET("/api/reports/team-pricing-profiles", lib.ChainMiddlewares(h.listProfiles, middlewares...))
	r.PUT("/api/reports/team-pricing-profiles/{team_id}", lib.ChainMiddlewares(h.upsertProfile, middlewares...))
}

// standardPriceRequest is the caller-editable shape. The id is server-minted
// on insert; effective_from defaults to "now" so an admin can omit it for a
// plain "open the new rate immediately" call.
type standardPriceRequest struct {
	ID                    string     `json:"id,omitempty"`
	Provider              string     `json:"provider"`
	Model                 string     `json:"model"`
	Currency              string     `json:"currency,omitempty"`
	InputCostPerMillion   float64    `json:"input_cost_per_million"`
	OutputCostPerMillion  float64    `json:"output_cost_per_million"`
	CacheReadCostPerMil   *float64   `json:"cache_read_cost_per_million,omitempty"`
	CostPerRequest        *float64   `json:"cost_per_request,omitempty"`
	FXRate                float64    `json:"fx_rate,omitempty"`
	EffectiveFrom         *time.Time `json:"effective_from,omitempty"`
}

func (r *standardPriceRequest) toTable() *tables.TableStandardPrice {
	row := &tables.TableStandardPrice{
		Provider:                strings.ToLower(strings.TrimSpace(r.Provider)),
		Model:                   strings.TrimSpace(r.Model),
		InputCostPerMillion:     r.InputCostPerMillion,
		OutputCostPerMillion:    r.OutputCostPerMillion,
		CacheReadCostPerMillion: r.CacheReadCostPerMil,
		CostPerRequest:          r.CostPerRequest,
		FXRate:                  r.FXRate,
	}
	if r.Currency != "" {
		row.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
	}
	if r.EffectiveFrom != nil {
		row.EffectiveFrom = r.EffectiveFrom.UTC()
	}
	return row
}

// list handles GET /api/reports/standard-prices. Supports provider / model
// filters; default limit 100, max 500 to keep the admin table snappy.
func (h *StandardPriceHandler) list(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	params := configstore.StandardPriceQueryParams{
		Provider: string(ctx.QueryArgs().Peek("provider")),
		Model:    string(ctx.QueryArgs().Peek("model")),
	}
	if v, ok := parseQueryInt(ctx, "limit"); ok {
		params.Limit = v
	}
	if v, ok := parseQueryInt(ctx, "offset"); ok {
		params.Offset = v
	}
	rows, total, err := h.store.ListStandardPrices(ctx, params)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list standard prices: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"rows":  rows,
		"total": total,
	})
}

// upsert handles PUT /api/reports/standard-prices. Editing the rate never
// overwrites history — every save is a new (provider, model, effective_from)
// row, so reports can pick the right snapshot for any past window.
func (h *StandardPriceHandler) upsert(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	var req standardPriceRequest
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload: "+err.Error())
		return
	}
	row := req.toTable()
	if row.ID == "" {
		row.ID = uuid.NewString()
	}
	if err := h.store.CreateStandardPrice(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "failed to save standard price: "+err.Error())
		return
	}
	SendJSON(ctx, row)
}

// delete handles DELETE /api/reports/standard-prices/:id.
func (h *StandardPriceHandler) delete(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	id := ctx.UserValue("id").(string)
	if id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "id is required")
		return
	}
	if err := h.store.DeleteStandardPrice(ctx, id); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "standard price not found")
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete standard price: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"ok": true})
}

// sync handles POST /api/reports/standard-prices/sync. Derives new
// standard-price rows from the actual-side datasheet with a configurable
// multiplier (default 1.0) so the team ledger never silently zeros out a
// freshly shipped model.
func (h *StandardPriceHandler) sync(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	multiplier := 1.0
	if v, ok := parseQueryFloat(ctx, "multiplier"); ok {
		multiplier = v
	}
	if multiplier <= 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "multiplier must be > 0")
		return
	}
	pricing, err := h.store.GetModelPrices(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load actual-side pricing: "+err.Error())
		return
	}
	now := time.Now().UTC()
	rows := make([]tables.TableStandardPrice, 0, len(pricing))
	for i := range pricing {
		p := pricing[i]
		if p.InputCostPerToken == nil && p.OutputCostPerToken == nil {
			continue
		}
		// Some upstream datasheet entries (notably OpenRouter) use -1 as a
		// sentinel for "not priced". Filter those out so the team ledger
		// never inherits a negative rate.
		input := 0.0
		if p.InputCostPerToken != nil && *p.InputCostPerToken >= 0 {
			input = *p.InputCostPerToken * 1_000_000 * multiplier
		}
		output := 0.0
		if p.OutputCostPerToken != nil && *p.OutputCostPerToken >= 0 {
			output = *p.OutputCostPerToken * 1_000_000 * multiplier
		}
		cacheRead := (*float64)(nil)
		if p.CacheReadInputTokenCost != nil && *p.CacheReadInputTokenCost >= 0 {
			v := *p.CacheReadInputTokenCost * 1_000_000 * multiplier
			cacheRead = &v
		}
		costPerRequest := (*float64)(nil)
		if p.CostPerRequest != nil && *p.CostPerRequest >= 0 {
			v := *p.CostPerRequest * multiplier
			costPerRequest = &v
		}
		rows = append(rows, tables.TableStandardPrice{
			Provider:                p.Provider,
			Model:                   p.Model,
			Currency:                "USD",
			InputCostPerMillion:     input,
			OutputCostPerMillion:    output,
			CacheReadCostPerMillion: cacheRead,
			CostPerRequest:          costPerRequest,
			FXRate:                  1.0,
			EffectiveFrom:           now,
		})
	}
	if err := h.store.BulkCreateStandardPrices(ctx, rows); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to sync standard prices: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"created": len(rows),
		"as_of":   now,
	})
}

// preview handles GET /api/reports/standard-prices/preview. Lets an admin
// see what a proposed rate would have cost each team in the lookback window
// (default 30 days). We don't have access to log aggregation here without
// pulling in the log manager, so the preview returns the proposed rates
// plus the current active row and a structured shape the admin UI can fill
// in once log-aggregated estimates are wired in (Phase 5 hook).
func (h *StandardPriceHandler) preview(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	provider := string(ctx.QueryArgs().Peek("provider"))
	model := string(ctx.QueryArgs().Peek("model"))
	if provider == "" || model == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "provider and model are required")
		return
	}
	inputRate, ok1 := parseQueryFloat(ctx, "input_cost_per_million")
	outputRate, ok2 := parseQueryFloat(ctx, "output_cost_per_million")
	if !ok1 || !ok2 {
		SendError(ctx, fasthttp.StatusBadRequest, "input_cost_per_million and output_cost_per_million are required")
		return
	}
	window := 30 * 24 * time.Hour
	if v, ok := parseQueryInt(ctx, "lookback_days"); ok && v > 0 {
		window = time.Duration(v) * 24 * time.Hour
	}
	end := time.Now().UTC()
	start := end.Add(-window)

	active, _ := h.store.GetActiveStandardPrice(ctx, provider, model, end)
	SendJSON(ctx, map[string]any{
		"provider":      provider,
		"model":         model,
		"currency":      datasheet.PricingCurrency,
		"proposed":      map[string]float64{"input": inputRate, "output": outputRate},
		"active_row":    active,
		"window":        map[string]any{"start": start, "end": end},
		"preview_note":  "preview surfaces the proposed vs active rates; full cost-impact requires log-aggregated lookups wired in Phase 5",
	})
}

// listProfiles handles GET /api/reports/team-pricing-profiles.
func (h *StandardPriceHandler) listProfiles(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	rows, err := h.store.ListTeamPricingProfiles(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list team pricing profiles: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"rows": rows})
}

// upsertProfile handles PUT /api/reports/team-pricing-profiles/:team_id.
func (h *StandardPriceHandler) upsertProfile(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store unavailable")
		return
	}
	teamID := ctx.UserValue("team_id").(string)
	if teamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team_id is required")
		return
	}
	var req struct {
		Mode             string  `json:"mode"`
		MarginMultiplier float64 `json:"margin_multiplier"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload: "+err.Error())
		return
	}
	row := &tables.TableTeamPricingProfile{
		TeamID:           teamID,
		Mode:             req.Mode,
		MarginMultiplier: req.MarginMultiplier,
	}
	if err := h.store.UpsertTeamPricingProfile(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "failed to upsert team pricing profile: "+err.Error())
		return
	}
	SendJSON(ctx, row)
}

// parseQueryInt reads a query parameter as int; returns (0, false) when the
// param is absent or malformed. Shared with alerting.go so the parsing
// pattern stays consistent across handlers.
func parseQueryInt(ctx *fasthttp.RequestCtx, name string) (int, bool) {
	raw := string(ctx.QueryArgs().Peek(name))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseQueryFloat reads a query parameter as float64.
func parseQueryFloat(ctx *fasthttp.RequestCtx, name string) (float64, bool) {
	raw := string(ctx.QueryArgs().Peek(name))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
