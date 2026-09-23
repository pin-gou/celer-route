// Package handlers - reports_reconciliation.go implements the Phase 5 账单
// 校准 (billing reconciliation) admin API. A reconciliation batch pairs the
// gateway's Σactual (datasheet prices × observed usage, read from the log
// store) against the provider's Σactual (pulled from a usage API or an
// uploaded invoice), so an operator can see whether the cost side still
// tracks the supplier bill.
//
// Ground-truth channels (03-cost-allocation data-model §6.1):
//
//   - usage_api   — first wave is openai / anthropic / deepseek. Any other
//     provider returns "please upload an invoice" instead.
//   - invoice_upload — the fallback for Azure / Vertex / Bedrock / everything
//     else: POST a CSV to this same collection.
//
// Two invariants drive the write paths:
//
//  1. Calibration only ever touches the **cost side** (the datasheet used to
//     compute logs.cost). standard_prices — the team ledger — is never
//     modified, so a calibration can't silently re-price a customer.
//  2. Applying a batch is one-way: a batch that is already `applied` refuses
//     to run again (409), because the correction is a multiplicative delta on
//     the datasheet and a second pass would compound it.
package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/router"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/framework/modelcatalog/datasheet"
	"github.com/pin-gou/celer-route/framework/sidekiq/jobs"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

// datasheetChatMode is the primary datasheet mode for a text model. The
// preview prefers it when several modes exist for the same (provider, model)
// — the correction ratio is the same for all of them, so picking "chat"
// keeps the before/after numbers comparable across batches.
const datasheetChatMode = "chat"

// ReconciliationStore is the narrow configstore surface this handler needs.
// Declared as an interface so tests can drive the handler without a DB.
type ReconciliationStore interface {
	ListReconciliations(ctx context.Context, params configstore.ReconciliationQueryParams) ([]tables.TableBillingReconciliation, int64, error)
	GetReconciliationByID(ctx context.Context, id string) (*tables.TableBillingReconciliation, error)
	ListReconciliationItems(ctx context.Context, reconciliationID string) ([]tables.TableBillingReconItem, error)
	CreateReconciliation(ctx context.Context, row *tables.TableBillingReconciliation, items []tables.TableBillingReconItem) error
	UpdateReconciliation(ctx context.Context, row *tables.TableBillingReconciliation) error
}

// ReconciliationDatasheetStore is the cost-side slice used by preview/apply.
// Kept separate from ReconciliationStore so a build without a datasheet
// (read-only deployments) still serves list/run/detail.
type ReconciliationDatasheetStore interface {
	GetModelPrices(ctx context.Context) ([]tables.TableModelPricing, error)
	UpsertModelPrices(ctx context.Context, pricing *tables.TableModelPricing, tx ...*gorm.DB) error
}

// ReportsReconciliationHandler serves /api/reports/reconciliations*. The
// store is required; the log store and the datasheet store are optional and
// degrade to empty/annotated responses.
type ReportsReconciliationHandler struct {
	store     ReconciliationStore
	logStore  logstore.LogStore
	datasheet ReconciliationDatasheetStore
	usage     jobs.UsageAPIClient
}

// NewReportsReconciliationHandler builds the handler. logStore supplies the
// gateway-side Σactual; usage may be nil (Phase 5 ships the data path, the
// per-provider usage clients land as they are credentialed) — the run
// endpoint then records the gateway half as a pending batch.
func NewReportsReconciliationHandler(
	store ReconciliationStore,
	logStore logstore.LogStore,
	datasheetStore ReconciliationDatasheetStore,
	usage jobs.UsageAPIClient,
) *ReportsReconciliationHandler {
	return &ReportsReconciliationHandler{store: store, logStore: logStore, datasheet: datasheetStore, usage: usage}
}

// RegisterRoutes mounts the reconciliation surface. All admin-only via the
// router-level auth middleware.
func (h *ReportsReconciliationHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/reports/reconciliations", lib.ChainMiddlewares(h.list, middlewares...))
	r.POST("/api/reports/reconciliations", lib.ChainMiddlewares(h.upload, middlewares...))
	r.POST("/api/reports/reconciliations/run", lib.ChainMiddlewares(h.run, middlewares...))
	r.GET("/api/reports/reconciliations/{id}", lib.ChainMiddlewares(h.detail, middlewares...))
	r.POST("/api/reports/reconciliations/{id}/preview", lib.ChainMiddlewares(h.preview, middlewares...))
	r.POST("/api/reports/reconciliations/{id}/apply", lib.ChainMiddlewares(h.apply, middlewares...))
}

// reconciliationRunRequest is the POST …/run body. `period` is a calendar
// month ("2026-09") or an RFC3339 / YYYY-MM-DD range via period_start +
// period_end. Exactly one of the two forms is required.
type reconciliationRunRequest struct {
	Provider    string `json:"provider"`
	Period      string `json:"period,omitempty"`
	PeriodStart string `json:"period_start,omitempty"`
	PeriodEnd   string `json:"period_end,omitempty"`
}

func (h *ReportsReconciliationHandler) list(ctx *fasthttp.RequestCtx) {
	params := configstore.ReconciliationQueryParams{
		Provider: string(ctx.QueryArgs().Peek("provider")),
		Status:   string(ctx.QueryArgs().Peek("status")),
		Source:   string(ctx.QueryArgs().Peek("source")),
	}
	if v, ok := parseQueryInt(ctx, "limit"); ok {
		params.Limit = v
	}
	if v, ok := parseQueryInt(ctx, "offset"); ok {
		params.Offset = v
	}
	if params.Limit <= 0 || params.Limit > 500 {
		params.Limit = 100
	}
	if start, ok := parseQueryTime(ctx, "period_start"); ok {
		params.PeriodStart = start
	}
	if end, ok := parseQueryTime(ctx, "period_end"); ok {
		params.PeriodEnd = end
	}
	rows, total, err := h.store.ListReconciliations(ctx, params)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "list reconciliations: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"reconciliations": rows,
		"total":           total,
		"limit":           params.Limit,
		"offset":          params.Offset,
		"currency":        datasheet.PricingCurrency,
	})
}

// run triggers one calibration. It executes the job inline (rather than
// enqueueing) so the response carries the freshly written batch — an
// operator triggering a recalibration by hand needs the delta, not a queue
// receipt.
func (h *ReportsReconciliationHandler) run(ctx *fasthttp.RequestCtx) {
	var req reconciliationRunRequest
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "provider is required")
		return
	}
	start, end, ok := h.parseRunPeriod(ctx, req)
	if !ok {
		return
	}
	if h.logStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "log store not configured; cannot compute the gateway side")
		return
	}

	metaJSON, err := jobs.RunReconciliationNow(ctx, h.store, h.logStore, h.usage, provider, start, end)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "run reconciliation: "+err.Error())
		return
	}
	meta := map[string]any{}
	_ = json.Unmarshal([]byte(metaJSON), &meta)
	batchID, _ := meta["id"].(string)
	batch, _ := h.store.GetReconciliationByID(ctx, batchID)

	status, _ := meta["status"].(string)
	resp := map[string]any{
		"id":             batchID,
		"provider":       provider,
		"period":         map[string]any{"start": start, "end": end},
		"status":         status,
		"source":         tables.ReconciliationSourceUsageAPI,
		"result":         meta,
		"currency":       datasheet.PricingCurrency,
		"reconciliation": batch,
	}
	if status == tables.ReconciliationStatusUnsupported {
		// The whole point of the first-wave list: an operator asking for a
		// provider we can't pull from must be told to upload the invoice,
		// not handed a silent empty batch.
		resp["requires_invoice_upload"] = true
		resp["message"] = fmt.Sprintf(
			"provider %q has no usage API in the first wave; please upload an invoice CSV via POST /api/reports/reconciliations",
			provider)
	}
	SendJSON(ctx, resp)
}

// parseRunPeriod resolves the run body's period into a half-open UTC window.
// `period: "2026-09"` means [2026-09-01, 2026-10-01); the explicit
// period_start/period_end pair wins when both are present.
func (h *ReportsReconciliationHandler) parseRunPeriod(ctx *fasthttp.RequestCtx, req reconciliationRunRequest) (time.Time, time.Time, bool) {
	if strings.TrimSpace(req.PeriodStart) != "" || strings.TrimSpace(req.PeriodEnd) != "" {
		if strings.TrimSpace(req.PeriodStart) == "" || strings.TrimSpace(req.PeriodEnd) == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "period_start and period_end must be provided together")
			return time.Time{}, time.Time{}, false
		}
		start, err := parseReportTime(req.PeriodStart)
		if err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "invalid period_start: "+err.Error())
			return time.Time{}, time.Time{}, false
		}
		end, err := parseReportTime(req.PeriodEnd)
		if err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "invalid period_end: "+err.Error())
			return time.Time{}, time.Time{}, false
		}
		if !start.Before(end) {
			SendError(ctx, fasthttp.StatusBadRequest, "period_start must be before period_end")
			return time.Time{}, time.Time{}, false
		}
		return start.UTC(), end.UTC(), true
	}
	period := strings.TrimSpace(req.Period)
	if period == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "period (YYYY-MM) or period_start + period_end is required")
		return time.Time{}, time.Time{}, false
	}
	month, err := time.Parse("2006-01", period)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid period %q; want YYYY-MM or period_start + period_end")
		return time.Time{}, time.Time{}, false
	}
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0), true
}

func (h *ReportsReconciliationHandler) detail(ctx *fasthttp.RequestCtx) {
	id, ok := reconciliationIDFromCtx(ctx)
	if !ok {
		return
	}
	batch, err := h.store.GetReconciliationByID(ctx, id)
	if err != nil {
		writeReconciliationLookupError(ctx, err)
		return
	}
	items, err := h.store.ListReconciliationItems(ctx, id)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "list reconciliation items: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"reconciliation": batch,
		"items":          items,
		"currency":       datasheet.PricingCurrency,
	})
}

// upload accepts a supplier invoice CSV as the ground-truth side of a batch.
// This is the only path for providers outside the first wave (Azure / Vertex
// / Bedrock / …).
//
// CSV shape: a header row plus one row per (model, requests, tokens, cost).
// Header aliases are accepted for the cost column (`cost`, `cost_usd`,
// `amount`, `usd`) because every supplier exports a slightly different name.
func (h *ReportsReconciliationHandler) upload(ctx *fasthttp.RequestCtx) {
	provider := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("provider"))))
	period := strings.TrimSpace(string(ctx.QueryArgs().Peek("period")))
	if form, err := ctx.MultipartForm(); err == nil && form != nil {
		if v := firstFormValue(form.Value["provider"]); v != "" {
			provider = strings.ToLower(strings.TrimSpace(v))
		}
		if v := firstFormValue(form.Value["period"]); v != "" {
			period = strings.TrimSpace(v)
		}
	}
	if provider == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "provider is required (form field or ?provider=)")
		return
	}
	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "file is required for invoice upload")
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "open upload")
		return
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "read upload")
		return
	}
	records, err := parseInvoiceCSV(body)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid invoice CSV: "+err.Error())
		return
	}
	if len(records) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "invoice CSV has no data rows")
		return
	}

	start, end, ok := h.periodForUpload(ctx, period)
	if !ok {
		return
	}

	row := &tables.TableBillingReconciliation{
		Provider:    provider,
		PeriodStart: start,
		PeriodEnd:   end,
		Source:      tables.ReconciliationSourceInvoice,
	}
	// The gateway side is recomputed here so the uploaded invoice is compared
	// against the same window the usage-API path would have used.
	gatewayByModel, err := h.gatewayByModel(ctx, provider, start, end)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "read gateway side: "+err.Error())
		return
	}

	items := make([]tables.TableBillingReconItem, 0, len(records))
	var matched int
	for _, rec := range records {
		item := tables.TableBillingReconItem{
			Model:            rec.Model,
			ProviderRequests: rec.Requests,
			ProviderTokens:   rec.Tokens,
			ProviderCost:     rec.Cost,
		}
		if g, ok := gatewayByModel[rec.Model]; ok {
			item.GatewayRequests = g.Count
			item.GatewayTokens = g.Tokens
			item.GatewayCost = g.Cost
			matched++
		}
		if item.GatewayTokens > 0 && item.ProviderTokens > 0 {
			d := (float64(item.ProviderTokens-item.GatewayTokens) / float64(item.GatewayTokens)) * 100
			item.TokenDeltaPct = &d
		}
		if item.GatewayCost > 0 && item.ProviderCost > 0 {
			d := ((item.ProviderCost - item.GatewayCost) / item.GatewayCost) * 100
			item.PriceDeltaPct = &d
		}
		row.ProviderCost += item.ProviderCost
		row.GatewayCost += item.GatewayCost
		items = append(items, item)
	}
	row.Delta = row.ProviderCost - row.GatewayCost
	if matched == 0 {
		// Nothing on the invoice matched a gateway model: the batch is
		// recorded but flagged, because applying it would be meaningless.
		row.Status = tables.ReconciliationStatusError
		notes := "no invoice row matched a gateway model in this period; check the model column and the period"
		row.Notes = &notes
	} else {
		row.Status = tables.ReconciliationStatusMatched
	}
	if err := h.store.CreateReconciliation(ctx, row, items); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "persist reconciliation: "+err.Error())
		return
	}
	SendJSONWithStatus(ctx, map[string]any{
		"reconciliation": row,
		"items":          items,
		"matched_models": matched,
		"rows":           len(records),
		"currency":       datasheet.PricingCurrency,
	}, fasthttp.StatusCreated)
}

// previewCorrection is the dry-run answer: which datasheet rows would move,
// by how much, and how many historical log rows the change implies. It
// applies nothing.
type previewCorrection struct {
	ReconciliationID string                `json:"reconciliation_id"`
	DryRun           bool                  `json:"dry_run"`
	CanApply         bool                  `json:"can_apply"`
	BlockedReason    string                `json:"blocked_reason,omitempty"`
	CorrectionRatio  float64               `json:"correction_ratio"`
	Changes          []datasheetCorrection `json:"datasheet_changes"`
	LogsAffected     int64                 `json:"logs_affected"`
	Note             string                `json:"note,omitempty"`
}

// datasheetCorrection is one (provider, model, mode) row's before/after. The
// costs are per-token (the datasheet's unit), not per-million.
type datasheetCorrection struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	Mode          string  `json:"mode"`
	InputBefore   float64 `json:"input_cost_per_token_before"`
	InputAfter    float64 `json:"input_cost_per_token_after"`
	OutputBefore  float64 `json:"output_cost_per_token_before"`
	OutputAfter   float64 `json:"output_cost_per_token_after"`
	GatewayCost   float64 `json:"gateway_cost"`
	ProviderCost  float64 `json:"provider_cost"`
	PriceDeltaPct float64 `json:"price_delta_pct"`
}

func (h *ReportsReconciliationHandler) preview(ctx *fasthttp.RequestCtx) {
	id, ok := reconciliationIDFromCtx(ctx)
	if !ok {
		return
	}
	plan, err := h.plan(ctx, id)
	if err != nil {
		writeReconciliationLookupError(ctx, err)
		return
	}
	plan.DryRun = true
	plan.Note = correctionNote()
	SendJSON(ctx, plan)
}

// apply commits a batch: it scales the datasheet rates for every affected
// (provider, model) by the batch's implied correction ratio, marks the batch
// `applied`, and records the pre-change rates in the batch notes.
//
// The historical log rows are NOT rewritten here. Re-pricing a month of
// logs belongs to the existing cost-recalc job (plugins/logging), which
// already reads the datasheet and updates logs.cost; folding that work into
// a request handler would mean paging millions of rows inline. The response
// says so explicitly, and the batch note keeps the original rates so a
// repeated calibration can't compound.
func (h *ReportsReconciliationHandler) apply(ctx *fasthttp.RequestCtx) {
	id, ok := reconciliationIDFromCtx(ctx)
	if !ok {
		return
	}
	plan, err := h.plan(ctx, id)
	if err != nil {
		writeReconciliationLookupError(ctx, err)
		return
	}
	if !plan.CanApply {
		SendError(ctx, fasthttp.StatusConflict, plan.BlockedReason)
		return
	}
	if h.datasheet == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "datasheet not configured; cannot apply calibration")
		return
	}
	batch, err := h.store.GetReconciliationByID(ctx, id)
	if err != nil {
		writeReconciliationLookupError(ctx, err)
		return
	}

	// Write each corrected row back. UpsertModelPrices takes the full row, so
	// we start from the current one and only move the two text cost columns.
	rows, err := h.datasheet.GetModelPrices(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "load datasheet: "+err.Error())
		return
	}
	index := indexDatasheet(rows)
	var changed int
	for _, change := range plan.Changes {
		row, ok := index[datasheetKey{provider: change.Provider, model: change.Model, mode: change.Mode}]
		if !ok {
			continue
		}
		in := change.InputAfter
		out := change.OutputAfter
		row.InputCostPerToken = &in
		row.OutputCostPerToken = &out
		if err := h.datasheet.UpsertModelPrices(ctx, &row); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "write datasheet row "+change.Provider+"/"+change.Model+": "+err.Error())
			return
		}
		changed++
	}

	// Snapshot the pre-change rates into the batch so the audit trail carries
	// what the correction was applied on top of.
	snapshot := appliedSnapshot{
		AppliedAt: time.Now().UTC(),
		Ratio:     plan.CorrectionRatio,
		Changes:   plan.Changes,
	}
	snapshotJSON, _ := json.Marshal(snapshot)
	note := "applied calibration: datasheet scaled by " +
		strconv.FormatFloat(plan.CorrectionRatio, 'f', 6, 64) +
		"; historical log costs are re-priced by the cost-recalc job. snapshot=" + string(snapshotJSON)
	batch.Status = tables.ReconciliationStatusApplied
	batch.Notes = &note
	batch.Delta = batch.ProviderCost - batch.GatewayCost
	if err := h.store.UpdateReconciliation(ctx, batch); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "mark reconciliation applied: "+err.Error())
		return
	}

	plan.DryRun = false
	plan.CanApply = false
	plan.BlockedReason = ""
	plan.Note = correctionNote() + fmt.Sprintf(" applied %d datasheet row(s).", changed)
	SendJSON(ctx, map[string]any{
		"reconciliation":         batch,
		"datasheet_rows_written": changed,
		"result":                 plan,
	})
}

// appliedSnapshot is the audit payload stored in the batch notes on apply.
type appliedSnapshot struct {
	AppliedAt time.Time             `json:"applied_at"`
	Ratio     float64               `json:"ratio"`
	Changes   []datasheetCorrection `json:"changes"`
}

// plan builds the correction plan shared by preview and apply. It returns
// ErrNotFound for an unknown batch id, and sets CanApply=false with a reason
// when the batch can't be applied (already applied / no price signal).
func (h *ReportsReconciliationHandler) plan(ctx *fasthttp.RequestCtx, id string) (*previewCorrection, error) {
	batch, err := h.store.GetReconciliationByID(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := h.store.ListReconciliationItems(ctx, id)
	if err != nil {
		return nil, err
	}
	plan := &previewCorrection{
		ReconciliationID: id,
		Changes:          []datasheetCorrection{},
	}
	if batch.Status == tables.ReconciliationStatusApplied {
		plan.BlockedReason = "batch is already applied; re-applying would compound the correction"
		return plan, nil
	}
	if batch.GatewayCost <= 0 {
		plan.BlockedReason = "batch has no gateway cost; nothing to correct against"
		return plan, nil
	}
	if h.datasheet == nil {
		plan.BlockedReason = "datasheet not configured; cannot compute a correction"
		return plan, nil
	}
	if batch.ProviderCost <= 0 {
		plan.BlockedReason = "batch has no provider cost yet; upload an invoice or configure a usage API before applying"
		return plan, nil
	}

	ratio := batch.ProviderCost / batch.GatewayCost
	plan.CorrectionRatio = ratio

	rows, err := h.datasheet.GetModelPrices(ctx)
	if err != nil {
		return nil, err
	}
	index := indexDatasheet(rows)

	for _, item := range items {
		row, ok := index[datasheetKey{provider: batch.Provider, model: item.Model, mode: datasheetChatMode}]
		if !ok {
			// Fall back to any mode for this model — the delta is a scale
			// factor and applies the same way to embeddings / batch rows.
			for key, candidate := range index {
				if key.provider == strings.ToLower(batch.Provider) && key.model == item.Model {
					row, ok = candidate, true
					break
				}
			}
		}
		if !ok {
			continue
		}
		var inBefore, outBefore float64
		if row.InputCostPerToken != nil {
			inBefore = *row.InputCostPerToken
		}
		if row.OutputCostPerToken != nil {
			outBefore = *row.OutputCostPerToken
		}
		deltaPct := 0.0
		if item.PriceDeltaPct != nil {
			deltaPct = *item.PriceDeltaPct
		}
		plan.Changes = append(plan.Changes, datasheetCorrection{
			// Lower-case the provider: the batch normalizes to lower case and
			// apply() looks the correction back up in indexDatasheet, whose
			// keys are lower-cased. Emitting the datasheet's original casing
			// ("OpenAI") would make apply silently skip the row.
			Provider:      strings.ToLower(strings.TrimSpace(row.Provider)),
			Model:         row.Model,
			Mode:          row.Mode,
			InputBefore:   inBefore,
			InputAfter:    inBefore * ratio,
			OutputBefore:  outBefore,
			OutputAfter:   outBefore * ratio,
			GatewayCost:   item.GatewayCost,
			ProviderCost:  item.ProviderCost,
			PriceDeltaPct: deltaPct,
		})
	}
	if len(plan.Changes) == 0 {
		plan.BlockedReason = "no datasheet row matches this batch's models; nothing to correct"
		return plan, nil
	}
	plan.CanApply = true
	plan.LogsAffected = h.countLogsAffected(ctx, batch, items)
	return plan, nil
}

// countLogsAffected reports how many historical log rows the correction
// implies. It is an estimate for the preview ("this touches ~N rows"), read
// from the log store's own aggregate so it costs one query rather than a
// scan.
func (h *ReportsReconciliationHandler) countLogsAffected(ctx *fasthttp.RequestCtx, batch *tables.TableBillingReconciliation, items []tables.TableBillingReconItem) int64 {
	if h.logStore == nil {
		return 0
	}
	models := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Model) != "" {
			models = append(models, item.Model)
		}
	}
	start, end := batch.PeriodStart, batch.PeriodEnd
	stats, err := h.logStore.GetStats(ctx, logstore.SearchFilters{
		Providers: []string{batch.Provider},
		Models:    models,
		StartTime: &start,
		EndTime:   &end,
	})
	if err != nil || stats == nil {
		return 0
	}
	return stats.TotalRequests
}

// gatewayByModel reads the gateway side for one (provider, window). Mirrors
// the reconciliation job's fetchGatewayBuckets so the invoice path and the
// usage-API path compare against exactly the same numbers.
func (h *ReportsReconciliationHandler) gatewayByModel(ctx *fasthttp.RequestCtx, provider string, start, end time.Time) (map[string]gatewayModel, error) {
	if h.logStore == nil {
		return map[string]gatewayModel{}, nil
	}
	uncapped := 0
	res, err := h.logStore.GetModelRankings(ctx, logstore.SearchFilters{
		Providers:    []string{provider},
		StartTime:    &start,
		EndTime:      &end,
		RankingLimit: &uncapped,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]gatewayModel)
	if res == nil {
		return out, nil
	}
	for _, m := range res.Rankings {
		out[m.Model] = gatewayModel{Count: m.TotalRequests, Tokens: m.TotalTokens, Cost: m.TotalCost}
	}
	return out, nil
}

type gatewayModel struct {
	Count  int64
	Tokens int64
	Cost   float64
}

// periodForUpload resolves the period for an invoice upload. It accepts the
// `period` form field / query param (YYYY-MM) or falls back to the current
// calendar month, because an admin uploading last month's invoice usually
// omits it.
func (h *ReportsReconciliationHandler) periodForUpload(ctx *fasthttp.RequestCtx, period string) (time.Time, time.Time, bool) {
	if period != "" {
		month, err := time.Parse("2006-01", period)
		if err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "invalid period %q; want YYYY-MM")
			return time.Time{}, time.Time{}, false
		}
		start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), true
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0), true
}

// invoiceRecord is one parsed CSV data row.
type invoiceRecord struct {
	Model    string
	Requests int64
	Tokens   int64
	Cost     float64
}

// parseInvoiceCSV reads a supplier invoice. The header row is required; the
// model column is mandatory and the rest default to 0 when absent. Aliases
// are accepted for the cost and token columns because every supplier names
// them differently.
func parseInvoiceCSV(body []byte) ([]invoiceRecord, error) {
	reader := csv.NewReader(strings.NewReader(string(body)))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, errors.New("expected a header row and at least one data row")
	}
	header := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		header[i] = normalizeCSVHeader(h)
	}
	modelCol := indexOfHeader(header, "model", "model_name", "modelname")
	if modelCol < 0 {
		return nil, errors.New("header must contain a `model` column")
	}
	requestsCol := indexOfHeader(header, "requests", "request_count", "calls", "count")
	tokensCol := indexOfHeader(header, "tokens", "total_tokens", "token_count", "usage_tokens")
	costCol := indexOfHeader(header, "cost", "cost_usd", "amount", "usd", "total_cost")

	out := make([]invoiceRecord, 0, len(rows)-1)
	for i, r := range rows[1:] {
		if len(r) == 0 {
			continue
		}
		rec := invoiceRecord{}
		if modelCol < len(r) {
			rec.Model = strings.TrimSpace(r[modelCol])
		}
		if rec.Model == "" {
			continue
		}
		rec.Requests = parseCSVInt(cellAt(r, requestsCol))
		rec.Tokens = parseCSVInt(cellAt(r, tokensCol))
		cost, err := parseCSVFloat(cellAt(r, costCol))
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid cost %q", i+2, cellAt(r, costCol))
		}
		rec.Cost = cost
		out = append(out, rec)
	}
	return out, nil
}

func normalizeCSVHeader(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	h = strings.TrimPrefix(h, "\ufeff")
	return strings.ReplaceAll(h, " ", "_")
}

func indexOfHeader(header []string, names ...string) int {
	for i, h := range header {
		for _, name := range names {
			if h == name {
				return i
			}
		}
	}
	return -1
}

func cellAt(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func parseCSVInt(v string) int64 {
	if v == "" {
		return 0
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return int64(f)
	}
	return 0
}

func parseCSVFloat(v string) (float64, error) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "$"))
	v = strings.ReplaceAll(v, ",", "")
	if v == "" {
		return 0, nil
	}
	return strconv.ParseFloat(v, 64)
}

// datasheetKey identifies one datasheet row in the in-memory index.
type datasheetKey struct {
	provider string
	model    string
	mode     string
}

// indexDatasheet keys the datasheet rows for the preview/apply pass. Provider
// and model are lower-cased because reconciliation rows normalize on write
// (TableBillingReconciliation.BeforeSave) while the datasheet preserves
// whatever casing the sync wrote.
func indexDatasheet(rows []tables.TableModelPricing) map[datasheetKey]tables.TableModelPricing {
	out := make(map[datasheetKey]tables.TableModelPricing, len(rows))
	for _, r := range rows {
		out[datasheetKey{
			provider: strings.ToLower(strings.TrimSpace(r.Provider)),
			model:    strings.TrimSpace(r.Model),
			mode:     r.Mode,
		}] = r
	}
	return out
}

// correctionNote documents the two deliberate scope decisions on the apply
// path so the operator isn't surprised by either.
func correctionNote() string {
	return "calibration only touches the cost side (datasheet); standard_prices — the team ledger — is never modified. " +
		"Historical logs are re-priced by the cost-recalc job on its next run, not inline by this endpoint."
}

// writeReconciliationLookupError maps the configstore's sentinel errors to
// status codes. A missing batch is a 404, everything else a 500.
func writeReconciliationLookupError(ctx *fasthttp.RequestCtx, err error) {
	if errors.Is(err, configstore.ErrNotFound) {
		SendError(ctx, fasthttp.StatusNotFound, "reconciliation not found")
		return
	}
	SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
}

// firstFormValue returns the first non-empty entry of a multipart value
// slice, or "".
func firstFormValue(values []string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// parseReportTime parses RFC3339 or YYYY-MM-DD without writing to the
// response — the caller decides how to report the failure (reports_cost.go's
// parseQueryTime writes the 400 inline, which is right for query params but
// wrong for a JSON body where the field name belongs in the message).
func parseReportTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%q is not RFC3339 or YYYY-MM-DD", raw)
}

// reconciliationIDFromCtx reads the {id} path param. The router guarantees it
// is present for these routes, but a defensive type-assert keeps a malformed
// request from panicking the handler goroutine.
func reconciliationIDFromCtx(ctx *fasthttp.RequestCtx) (string, bool) {
	v, _ := ctx.UserValue("id").(string)
	if strings.TrimSpace(v) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "reconciliation id is required")
		return "", false
	}
	return v, true
}
