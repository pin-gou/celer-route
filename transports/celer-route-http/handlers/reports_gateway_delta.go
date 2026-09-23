// Package handlers - reports_gateway_delta.go implements the Phase 5
// "差异层 / gateway delta" view: Σ actual − Σ standard, broken down by
// provider and by (provider, model). This is the admin-only financial
// surface that answers "where did the gateway make or lose money" against
// the stable team ledger (03-cost-allocation data-model §7).
//
// Both sides of the subtraction come from the same window and the same
// filters, so the number is internally consistent:
//
//   - actual  = Σ logs.cost, straight out of the log store's ranking
//     aggregates (GetProviderRankings / GetModelRankings). This is the
//     cost side — datasheet prices × observed usage, tagged per row with
//     cost_accuracy.
//   - standard = the same usage re-priced through the standard_prices book
//     (GetActiveStandardPrice), which is what teams are billed on.
//
// Rows with no standard_prices entry contribute actual only; the response
// carries a `coverage` block so the operator can see how much of the spend
// the price book actually explains rather than silently reading a
// "delta == actual" number as profit.
package handlers

import (
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fasthttp/router"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/framework/modelcatalog/datasheet"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// standardPriceWildcardModel is the provider-default row in standard_prices
// (data-model §1: `model = "*" → 该 provider 的默认行`). A model with no
// exact row falls back to it so the price book can cover a provider with a
// single blended rate.
const standardPriceWildcardModel = "*"

// StandardPriceLookup is the narrow slice of the config store the standard
// pricer needs. Declared as an interface so the delta math is unit-testable
// without standing up the full config store.
type StandardPriceLookup interface {
	GetActiveStandardPrice(ctx context.Context, provider, model string, at time.Time) (*tables.TableStandardPrice, error)
}

// ReportsGatewayDeltaHandler serves /api/reports/gateway-delta. It holds the
// config store for the standard_prices book and the log store for actuals.
// Either may be nil — the handler degrades to an empty/annotated response
// rather than 500, consistent with reports_cost.go.
type ReportsGatewayDeltaHandler struct {
	store    StandardPriceLookup
	logStore logstore.LogStore
}

// NewReportsGatewayDeltaHandler constructs the handler with both deps
// optional. The log store is the same one reports_cost uses so the totals
// always line up with /api/reports/cost/summary?include_actual=true.
func NewReportsGatewayDeltaHandler(store configstore.ConfigStore, logStore logstore.LogStore) *ReportsGatewayDeltaHandler {
	return &ReportsGatewayDeltaHandler{store: store, logStore: logStore}
}

// RegisterRoutes mounts the gateway-delta endpoints. Both are admin-only via
// the router-level auth middleware.
func (h *ReportsGatewayDeltaHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/reports/gateway-delta", lib.ChainMiddlewares(h.gatewayDelta, middlewares...))
	r.GET("/api/reports/gateway-delta/export", lib.ChainMiddlewares(h.gatewayDeltaExport, middlewares...))
}

// gatewayDeltaRow is one provider or (provider, model) line. Delta is signed:
// positive = the gateway kept money (cache hits, off-peak pricing, credit
// discounts), negative = the gateway lost money (peak surcharges, provider
// price rises, plan-allocation skew).
type gatewayDeltaRow struct {
	ID               string  `json:"id"`
	Name             string  `json:"name,omitempty"`
	Provider         string  `json:"provider,omitempty"`
	Model            string  `json:"model,omitempty"`
	Requests         int64   `json:"requests"`
	Tokens           int64   `json:"tokens"`
	CostActual       float64 `json:"cost_actual"`
	CostStandard     float64 `json:"cost_standard"`
	Delta            float64 `json:"delta"`
	SharePct         float64 `json:"share_pct,omitempty"`
	HasStandardPrice bool    `json:"has_standard_price"`
}

// gatewayDeltaCoverage tells the operator how much of the window's actual
// spend the standard price book can explain. Without it a large negative
// delta is ambiguous — it could be a real loss, or just unpriced models.
type gatewayDeltaCoverage struct {
	ModelsPriced       int     `json:"models_priced"`
	ModelsTotal        int     `json:"models_total"`
	PricedActualCost   float64 `json:"priced_actual_cost"`
	ActualCostCoverage float64 `json:"actual_cost_coverage_pct"`
	UnpricedActualCost float64 `json:"unpriced_actual_cost"`
}

// gatewayDeltaResponse is the wire shape.
type gatewayDeltaResponse struct {
	Period    map[string]any       `json:"period"`
	Currency  string               `json:"currency"`
	Accuracy  string               `json:"accuracy"`
	Total     map[string]any       `json:"total"`
	Coverage  gatewayDeltaCoverage `json:"coverage"`
	Providers []gatewayDeltaRow    `json:"providers"`
	Models    []gatewayDeltaRow    `json:"models"`
	Note      string               `json:"note,omitempty"`
}

// gatewayDelta computes the report. One log-store pass per dimension plus one
// token-split pass; the standard price book is read per (provider, model)
// with a small memo so a repeated model doesn't re-hit the DB.
func (h *ReportsGatewayDeltaHandler) gatewayDelta(ctx *fasthttp.RequestCtx) {
	start, end, ok := parseReportWindow(ctx)
	if !ok {
		return
	}
	accuracy, ok := parseCostAccuracy(ctx)
	if !ok {
		return
	}
	if h.logStore == nil {
		SendJSON(ctx, gatewayDeltaResponse{
			Period:    map[string]any{"start": start, "end": end},
			Currency:  datasheet.PricingCurrency,
			Accuracy:  accuracy,
			Total:     map[string]any{},
			Providers: []gatewayDeltaRow{},
			Models:    []gatewayDeltaRow{},
			Note:      "log store not configured; cannot compute gateway delta",
		})
		return
	}

	report, err := h.buildReport(ctx, start, end, accuracy)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "gateway delta: "+err.Error())
		return
	}
	report.resp.Period = map[string]any{"start": start, "end": end}
	report.resp.Currency = datasheet.PricingCurrency
	report.resp.Accuracy = accuracy
	SendJSON(ctx, report.resp)
}

// gatewayDeltaExport streams the same numbers as CSV for offline accounting.
// The `scope` column keeps provider and model rows in one flat file.
func (h *ReportsGatewayDeltaHandler) gatewayDeltaExport(ctx *fasthttp.RequestCtx) {
	start, end, ok := parseReportWindow(ctx)
	if !ok {
		return
	}
	accuracy, ok := parseCostAccuracy(ctx)
	if !ok {
		return
	}
	if h.logStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "log store not configured")
		return
	}
	report, err := h.buildReport(ctx, start, end, accuracy)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "gateway delta: "+err.Error())
		return
	}

	ctx.Response.Header.Set("Content-Type", "text/csv; charset=utf-8")
	ctx.Response.Header.Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=gateway-delta-%s.csv", time.Now().UTC().Format("20060102")))
	w := csv.NewWriter(ctx)
	_ = w.Write([]string{"scope", "provider", "model", "requests", "tokens", "cost_actual", "cost_standard", "delta", "has_standard_price"})
	writeCSVRows := func(scope string, rows []gatewayDeltaRow) {
		for _, r := range rows {
			_ = w.Write([]string{
				scope,
				r.Provider,
				r.Model,
				strconv.FormatInt(r.Requests, 10),
				strconv.FormatInt(r.Tokens, 10),
				strconv.FormatFloat(r.CostActual, 'f', 10, 64),
				strconv.FormatFloat(r.CostStandard, 'f', 10, 64),
				strconv.FormatFloat(r.Delta, 'f', 10, 64),
				strconv.FormatBool(r.HasStandardPrice),
			})
		}
	}
	writeCSVRows("provider", report.resp.Providers)
	writeCSVRows("model", report.resp.Models)
	_ = w.Write([]string{
		"total", "", "",
		strconv.FormatInt(report.totalRequests, 10),
		strconv.FormatInt(report.totalTokens, 10),
		strconv.FormatFloat(report.totalActual, 'f', 10, 64),
		strconv.FormatFloat(report.totalStandard, 'f', 10, 64),
		strconv.FormatFloat(report.totalActual-report.totalStandard, 'f', 10, 64),
		"",
	})
	w.Flush()
}

// deltaReport is the computed result plus the scalar totals the CSV footer
// needs. The JSON handler serializes resp; keeping the scalars on the same
// struct means the two endpoints can never disagree.
type deltaReport struct {
	resp          gatewayDeltaResponse
	totalRequests int64
	totalTokens   int64
	totalActual   float64
	totalStandard float64
}

// buildReport is the shared computation for the JSON and CSV endpoints.
func (h *ReportsGatewayDeltaHandler) buildReport(ctx *fasthttp.RequestCtx, start, end time.Time, accuracy string) (*deltaReport, error) {
	filters := deltaFilters(start, end, accuracy)

	// Model rankings are the only log-store aggregate that carries both
	// provider and model on the same row, which is what the standard price
	// book is keyed on. Provider rankings then supply the per-provider actual
	// totals (they also cover rows whose model column is empty).
	modelRes, err := h.logStore.GetModelRankings(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("model rankings: %w", err)
	}
	var modelRankings []logstore.ModelRankingWithTrend
	if modelRes != nil {
		modelRankings = modelRes.Rankings
	}
	providerRes, err := h.logStore.GetProviderRankings(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("provider rankings: %w", err)
	}
	var providerRankings []logstore.ProviderRankingWithTrend
	if providerRes != nil {
		providerRankings = providerRes.Rankings
	}
	// The price book is per-token-split, but rankings only expose
	// TotalTokens. Pull the provider-level prompt/completion ratio once and
	// apply it to every model under that provider — a data-driven split
	// rather than an invented constant.
	splits := h.providerTokenSplits(ctx, filters, start, end)

	pricer := h.newStandardPricer(ctx, end)

	modelRows := make([]gatewayDeltaRow, 0, len(modelRankings))
	stdByProvider := make(map[string]float64)
	var modelsPriced int
	var pricedActual, unpricedActual float64
	for _, entry := range modelRankings {
		promptTokens, completionTokens := splits.apply(entry.Provider, entry.TotalTokens)
		std, priced := pricer.price(entry.Provider, entry.Model, promptTokens, completionTokens, entry.TotalRequests)
		row := gatewayDeltaRow{
			ID:               entry.Provider + "/" + entry.Model,
			Name:             entry.Model,
			Provider:         entry.Provider,
			Model:            entry.Model,
			Requests:         entry.TotalRequests,
			Tokens:           entry.TotalTokens,
			CostActual:       entry.TotalCost,
			CostStandard:     std,
			Delta:            entry.TotalCost - std,
			HasStandardPrice: priced,
		}
		modelRows = append(modelRows, row)
		stdByProvider[entry.Provider] += std
		if priced {
			modelsPriced++
			pricedActual += entry.TotalCost
		} else {
			unpricedActual += entry.TotalCost
		}
	}

	providerRows := make([]gatewayDeltaRow, 0, len(providerRankings))
	var totalRequests, totalTokens int64
	var totalActual, totalStandard float64
	for _, entry := range providerRankings {
		std := stdByProvider[entry.Provider]
		providerRows = append(providerRows, gatewayDeltaRow{
			ID:               entry.Provider,
			Name:             entry.Provider,
			Provider:         entry.Provider,
			Requests:         entry.TotalRequests,
			Tokens:           entry.TotalTokens,
			CostActual:       entry.TotalCost,
			CostStandard:     std,
			Delta:            entry.TotalCost - std,
			HasStandardPrice: std > 0,
		})
		totalRequests += entry.TotalRequests
		totalTokens += entry.TotalTokens
		totalActual += entry.TotalCost
		totalStandard += std
	}

	// Share of the gateway's actual spend, so the UI can rank rows by
	// financial weight instead of raw request count.
	if totalActual > 0 {
		for i := range providerRows {
			providerRows[i].SharePct = providerRows[i].CostActual / totalActual * 100
		}
		for i := range modelRows {
			modelRows[i].SharePct = modelRows[i].CostActual / totalActual * 100
		}
	}

	coveragePct := 0.0
	if totalActual > 0 {
		coveragePct = pricedActual / totalActual * 100
	}

	report := &deltaReport{
		resp: gatewayDeltaResponse{
			Providers: providerRows,
			Models:    modelRows,
			Total: map[string]any{
				"requests":      totalRequests,
				"tokens":        totalTokens,
				"cost_actual":   totalActual,
				"cost_standard": totalStandard,
				"delta":         totalActual - totalStandard,
			},
			Coverage: gatewayDeltaCoverage{
				ModelsPriced:       modelsPriced,
				ModelsTotal:        len(modelRows),
				PricedActualCost:   pricedActual,
				ActualCostCoverage: coveragePct,
				UnpricedActualCost: unpricedActual,
			},
		},
		totalRequests: totalRequests,
		totalTokens:   totalTokens,
		totalActual:   totalActual,
		totalStandard: totalStandard,
	}
	if modelsPriced < len(modelRows) {
		report.resp.Note = fmt.Sprintf(
			"standard_prices covers %.1f%% of actual cost (%d/%d models priced); unpriced models contribute actual only, so their delta equals their actual cost",
			coveragePct, modelsPriced, len(modelRows))
	}
	return report, nil
}

// deltaFilters builds the shared log-store filter set. RankingLimit is
// explicitly uncapped: a delta report that silently drops the long tail of
// models would understate the loss side, which is exactly the number an
// operator is auditing.
func deltaFilters(start, end time.Time, accuracy string) logstore.SearchFilters {
	uncapped := 0
	filters := logstore.SearchFilters{
		StartTime:    &start,
		EndTime:      &end,
		RankingLimit: &uncapped,
	}
	if accuracy != "" && accuracy != costAccuracyAll {
		filters.CostAccuracy = []string{accuracy}
	}
	return filters
}

// costAccuracyAll is the "no band filter" sentinel accepted by the
// `accuracy` query param.
const costAccuracyAll = "all"

// parseCostAccuracy reads and validates the `accuracy` query param. Returns
// false (after writing a 400) when the value isn't a band we can produce.
func parseCostAccuracy(ctx *fasthttp.RequestCtx) (string, bool) {
	accuracy := strings.TrimSpace(string(ctx.QueryArgs().Peek("accuracy")))
	if accuracy == "" {
		return costAccuracyAll, true
	}
	if accuracy != costAccuracyAll && !logstore.IsValidCostAccuracy(accuracy) {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf(
			"invalid accuracy %q; want %s|%s|%s|%s", accuracy,
			logstore.CostAccuracyProviderReported,
			logstore.CostAccuracyGatewayEstimated,
			logstore.CostAccuracyUnknown,
			costAccuracyAll))
		return "", false
	}
	return accuracy, true
}

// tokenSplit holds a provider's observed prompt/completion ratio, used to
// apportion a model's TotalTokens across the two standard-price rates.
type tokenSplit struct {
	promptRatio float64
}

// providerTokenSplits reads GetProviderTokenHistogram over the same window
// and collapses the buckets into one ratio per provider. Falls back to an
// even split for providers the histogram has no rows for (e.g. a window
// where the bucket table hasn't caught up).
func (h *ReportsGatewayDeltaHandler) providerTokenSplits(ctx *fasthttp.RequestCtx, filters logstore.SearchFilters, start, end time.Time) *tokenSplitTable {
	table := &tokenSplitTable{byProvider: make(map[string]tokenSplit)}
	bucket := calculateBucketSize(&start, &end)
	res, err := h.logStore.GetProviderTokenHistogram(ctx, filters, bucket)
	if err != nil || res == nil {
		// Non-fatal: the split only refines the standard side. Degrade to an
		// even split rather than failing the whole report.
		return table
	}
	type acc struct{ prompt, completion int64 }
	totals := make(map[string]*acc)
	for _, b := range res.Buckets {
		for provider, stats := range b.ByProvider {
			a := totals[provider]
			if a == nil {
				a = &acc{}
				totals[provider] = a
			}
			a.prompt += stats.PromptTokens
			a.completion += stats.CompletionTokens
		}
	}
	for provider, a := range totals {
		sum := a.prompt + a.completion
		if sum <= 0 {
			continue
		}
		table.byProvider[provider] = tokenSplit{promptRatio: float64(a.prompt) / float64(sum)}
	}
	return table
}

// tokenSplitTable maps provider → observed prompt ratio.
type tokenSplitTable struct {
	byProvider map[string]tokenSplit
}

// apply splits totalTokens into (prompt, completion) using the provider's
// observed ratio, defaulting to 50/50 when the provider has no histogram
// rows. Rounding goes to prompt so the two halves always sum back to total.
func (t *tokenSplitTable) apply(provider string, totalTokens int64) (int64, int64) {
	if totalTokens <= 0 {
		return 0, 0
	}
	ratio := 0.5
	if t != nil {
		if s, ok := t.byProvider[provider]; ok {
			ratio = s.promptRatio
		}
	}
	prompt := int64(float64(totalTokens) * ratio)
	if prompt > totalTokens {
		prompt = totalTokens
	}
	return prompt, totalTokens - prompt
}

// standardPricer resolves and memoizes standard_prices rows for one report
// pass. `at` is the window end so a mid-window price change prices the whole
// window at the rate in force when the report is run — the same convention
// reports_cost uses for the team ledger.
type standardPricer struct {
	store StandardPriceLookup
	ctx   *fasthttp.RequestCtx
	at    time.Time
	memo  map[string]*tables.TableStandardPrice
}

func (h *ReportsGatewayDeltaHandler) newStandardPricer(ctx *fasthttp.RequestCtx, at time.Time) *standardPricer {
	return &standardPricer{
		store: h.store,
		ctx:   ctx,
		at:    at,
		memo:  make(map[string]*tables.TableStandardPrice),
	}
}

// price re-prices one (provider, model) bucket at the standard book. Returns
// (cost, true) when a row was found — exact match first, then the provider's
// `*` default row — and (0, false) when the book has no entry at all.
//
// Cache-read tokens are deliberately not priced separately: the ranking
// aggregate doesn't expose them, so folding them into the prompt half at the
// input rate is the documented simplification (standard_prices treats a nil
// cache_read_cost_per_million the same way — see TableStandardPrice.
// CacheReadTokenCost).
func (p *standardPricer) price(provider, model string, promptTokens, completionTokens int64, requests int64) (float64, bool) {
	if p == nil || p.store == nil || provider == "" || model == "" {
		return 0, false
	}
	row := p.lookup(provider, model)
	if row == nil {
		row = p.lookup(provider, standardPriceWildcardModel)
	}
	if row == nil {
		return 0, false
	}
	cost := float64(promptTokens)*row.TokenCost(false) + float64(completionTokens)*row.TokenCost(true)
	if row.CostPerRequest != nil && requests > 0 {
		cost += float64(requests) * *row.CostPerRequest
	}
	return cost, true
}

// lookup reads one active price-book row, memoized per (provider, model). A
// nil result is memoized too so an unpriced model costs one DB round-trip for
// the whole report rather than one per occurrence.
func (p *standardPricer) lookup(provider, model string) *tables.TableStandardPrice {
	key := provider + "\x00" + model
	if row, ok := p.memo[key]; ok {
		return row
	}
	row, err := p.store.GetActiveStandardPrice(p.ctx, provider, model, p.at)
	if err != nil {
		row = nil
	}
	p.memo[key] = row
	return row
}
