// Package jobs — reconciliationjob.go runs the Phase 5 cost calibration
// pipeline. It pulls the provider's usage / invoice for a (provider,
// period) pair, compares against the gateway's Σactual from the log store,
// and writes the result into billing_reconciliations + billing_recon_items
// so the admin "gateway-delta" view can surface the per-model breakdown.
//
// Phase 5 only wires the data path end-to-end for the three first-wave
// providers listed in data-model §6.1 (openai / anthropic / deepseek). For
// any other provider the job returns an "unsupported" batch row rather than
// a hard error — the UI surfaces this as the "please upload invoice"
// affordance, the same handler endpoint that handles invoice_upload takes
// the user from there.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/framework/sidekiq"
)

// ReconciliationSourceKind matches the source column on the persisted batch
// — kept here as a local const so the handler side and the job side agree
// without a transitive import.
const (
	ReconciliationSourceUsageAPI = "usage_api"
	ReconciliationSourceInvoice  = "invoice_upload"
	ReconciliationSourceManual   = "manual"
)

// ReconciliationStore is the configstore surface the calibration job needs.
// Splitting it from the full store keeps the test fake trivial and removes
// any temptation for the job to read pricing rules directly — only the
// outer handler should reconcile with the standard_prices book.
type ReconciliationStore interface {
	CreateReconciliation(ctx context.Context, row *tables.TableBillingReconciliation, items []tables.TableBillingReconItem) error
	UpdateReconciliation(ctx context.Context, row *tables.TableBillingReconciliation) error
}

// ReconciliationLogStore reads the gateway-side Σactual for the period. The
// existing model rankings cover this exact shape (provider × model × cost),
// so the job only needs the log-store rank helper. The interface stays
// narrow to keep the job testable.
type ReconciliationLogStore interface {
	GetModelRankings(ctx context.Context, filters logstore.SearchFilters) (*logstore.ModelRankingResult, error)
}

// UsageAPIClient pulls the provider's ground-truth usage for one
// (provider, period). In Phase 5 we expose only the interface so test
// fakes can simulate the first-wave providers; the production wiring is
// left for the transport layer in a follow-up — keeping the job here
// focused on the data path.
type UsageAPIClient interface {
	FetchUsage(ctx context.Context, provider string, periodStart, periodEnd time.Time) ([]UsageRecord, error)
}

// UsageRecord is one line item from a provider usage API / invoice CSV.
// Model + tokens + cost are the minimum needed to build a recon_item row;
// billing requests and price-deltas are derived downstream.
type UsageRecord struct {
	Model            string
	ProviderRequests int64
	ProviderTokens   int64
	ProviderCost     float64
}

// ReconciliationJob is the sidekiq handler for one calibration batch.
// UsageClient is the optional network seam (nil => unsupported-mode stub).
type ReconciliationJob struct {
	store       ReconciliationStore
	logStore    ReconciliationLogStore
	usageClient UsageAPIClient
	logger      func(format string, args ...any) // optional debug sink
}

// NewReconciliationJob constructs the handler with the dependencies it
// needs. usageClient may be nil — the handler treats that as "we don't have
// a usage API for this provider yet" and writes an unsupported batch.
func NewReconciliationJob(store ReconciliationStore, logStore ReconciliationLogStore, usageClient UsageAPIClient) *ReconciliationJob {
	return &ReconciliationJob{store: store, logStore: logStore, usageClient: usageClient}
}

// Kind is the sidekiq kind string. Handlers reference this when calling
// EnqueueReconciliationNow.
func (j *ReconciliationJob) Kind() string { return "reconciliation" }

// Handle runs the calibration for one batch. The job metadata (set by the
// transport layer when enqueueing) carries the (provider, period_start,
// period_end) triple; this handler ignores any pre-existing job row and
// writes a fresh batch from scratch.
func (j *ReconciliationJob) Handle(ctx context.Context, job tables.TableSidekiqJob, progress sidekiq.ProgressFunc) (string, error) {
	if j.store == nil || j.logStore == nil {
		return "", fmt.Errorf("reconciliation job: missing store/logStore")
	}
	var meta struct {
		Provider    string    `json:"provider"`
		PeriodStart time.Time `json:"period_start"`
		PeriodEnd   time.Time `json:"period_end"`
	}
	if err := json.Unmarshal([]byte(job.Metadata), &meta); err != nil {
		return "", fmt.Errorf("reconciliation job: decode metadata: %w", err)
	}
	if meta.Provider == "" || meta.PeriodStart.IsZero() || meta.PeriodEnd.IsZero() {
		return "", fmt.Errorf("reconciliation job: provider/period_start/period_end required")
	}
	// The gateway side is computable for every provider, so capture it even on
	// the degraded paths below: an operator who later uploads an invoice gets
	// a batch that already carries Σgateway instead of having to re-run.
	gatewayBuckets, err := j.fetchGatewayBuckets(ctx, meta.Provider, meta.PeriodStart, meta.PeriodEnd)
	if err != nil {
		return "", fmt.Errorf("reconciliation job: fetch gateway buckets: %w", err)
	}

	if !tables.IsReconciliationUsageAPISupported(meta.Provider) {
		// data-model §6.1: only the first-wave providers expose a usage API.
		// Everything else (Azure / Vertex / Bedrock / …) must go through
		// invoice_upload — record the attempt so the UI can point at it.
		return j.persistDegraded(ctx, meta.Provider, meta.PeriodStart, meta.PeriodEnd,
			tables.ReconciliationStatusUnsupported,
			"provider has no usage API in the first wave; please upload an invoice CSV via POST /api/reports/reconciliations",
			gatewayBuckets)
	}
	if j.usageClient == nil {
		// First-wave provider, but this build has no usage client wired (no
		// provider org/admin credential configured). The run itself succeeded
		// and the gateway side is captured, so the batch stays pending rather
		// than unsupported — the operator only needs to supply the provider
		// side.
		return j.persistDegraded(ctx, meta.Provider, meta.PeriodStart, meta.PeriodEnd,
			tables.ReconciliationStatusPending,
			"usage API client not configured in this build; gateway side captured — upload an invoice CSV or configure provider usage credentials to complete the calibration",
			gatewayBuckets)
	}
	usage, err := j.usageClient.FetchUsage(ctx, meta.Provider, meta.PeriodStart, meta.PeriodEnd)
	if err != nil {
		return "", fmt.Errorf("reconciliation job: fetch usage: %w", err)
	}
	gatewayByModel := make(map[string]gatewayModelTotal, len(gatewayBuckets))
	for _, b := range gatewayBuckets {
		gatewayByModel[b.Model] = b
	}
	usageByModel := make(map[string]UsageRecord, len(usage))
	for _, u := range usage {
		usageByModel[u.Model] = u
	}
	models := make(map[string]struct{}, len(gatewayByModel)+len(usageByModel))
	for k := range gatewayByModel {
		models[k] = struct{}{}
	}
	for k := range usageByModel {
		models[k] = struct{}{}
	}
	row := &tables.TableBillingReconciliation{
		Provider:    meta.Provider,
		PeriodStart: meta.PeriodStart,
		PeriodEnd:   meta.PeriodEnd,
		Source:      ReconciliationSourceUsageAPI,
	}
	var items []tables.TableBillingReconItem
	var gatewayTotal, providerTotal float64
	var matched int
	for model := range models {
		var gReq, gTok int64
		var gCost float64
		if gb, ok := gatewayByModel[model]; ok {
			gReq, gTok, gCost = gb.Count, gb.Tokens, gb.Cost
		}
		var pReq, pTok int64
		var pCost float64
		if u, ok := usageByModel[model]; ok {
			pReq, pTok, pCost = u.ProviderRequests, u.ProviderTokens, u.ProviderCost
		}
		var tokenDeltaPct, priceDeltaPct *float64
		if gTok > 0 && pTok > 0 {
			d := (float64(pTok-gTok) / float64(gTok)) * 100
			tokenDeltaPct = &d
		}
		if gCost > 0 && pCost > 0 {
			d := (float64(pCost-gCost) / float64(gCost)) * 100
			priceDeltaPct = &d
		}
		items = append(items, tables.TableBillingReconItem{
			Model:            model,
			GatewayRequests:  gReq,
			GatewayTokens:    gTok,
			GatewayCost:      gCost,
			ProviderRequests: pReq,
			ProviderTokens:   pTok,
			ProviderCost:     pCost,
			TokenDeltaPct:    tokenDeltaPct,
			PriceDeltaPct:    priceDeltaPct,
		})
		gatewayTotal += gCost
		providerTotal += pCost
		matched++
	}
	row.GatewayCost = gatewayTotal
	row.ProviderCost = providerTotal
	row.Delta = providerTotal - gatewayTotal
	// Nothing on either side means the window is empty — that is an error, not
	// a match, because there is no evidence to calibrate against. Anything
	// with at least one compared model is "matched": the per-model
	// token_delta_pct / price_delta_pct columns carry the magnitude, and the
	// admin decides whether to apply it.
	if matched == 0 {
		row.Status = tables.ReconciliationStatusError
		notes := "no gateway usage and no provider usage in this period; nothing to calibrate"
		row.Notes = &notes
	} else {
		row.Status = tables.ReconciliationStatusMatched
	}
	if err := j.store.CreateReconciliation(ctx, row, items); err != nil {
		return "", fmt.Errorf("reconciliation job: persist batch: %w", err)
	}
	finalMeta, _ := json.Marshal(map[string]any{
		"id":             row.ID,
		"provider":       meta.Provider,
		"status":         row.Status,
		"matched_models": matched,
		"gateway_cost":   gatewayTotal,
		"provider_cost":  providerTotal,
		"delta":          row.Delta,
	})
	if progress != nil {
		_ = progress(string(finalMeta))
	}
	return string(finalMeta), nil
}

// fetchGatewayBuckets calls the log store's per-model cost ranking
// narrowed to provider + period. The result is a flat per-model list which
// is exactly the shape the reconciliation item rows need.
func (j *ReconciliationJob) fetchGatewayBuckets(ctx context.Context, provider string, start, end time.Time) ([]gatewayModelTotal, error) {
	filters := logstore.SearchFilters{
		Providers: []string{provider},
		StartTime: &start,
		EndTime:   &end,
	}
	res, err := j.logStore.GetModelRankings(ctx, filters)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	out := make([]gatewayModelTotal, 0, len(res.Rankings))
	for _, m := range res.Rankings {
		out = append(out, gatewayModelTotal{
			Model:  m.Model,
			Count:  m.TotalRequests,
			Tokens: m.TotalTokens,
			Cost:   m.TotalCost,
		})
	}
	return out, nil
}

// gatewayModelTotal is the trimmed view of a log-store ranking entry.
// Keeping it local to this file avoids leaking the logstore type into the
// recon job's public signature.
type gatewayModelTotal struct {
	Model  string
	Count  int64
	Tokens int64
	Cost   float64
}

// EnqueueReconciliationNow schedules a calibration tick. The handler returns
// immediately; the runner takes the job on its next slot.
func EnqueueReconciliationNow(r *sidekiq.Runner, store ReconciliationStore, logStore ReconciliationLogStore, usageClient UsageAPIClient, provider string, start, end time.Time) error {
	if r == nil {
		return fmt.Errorf("sidekiq runner is required")
	}
	id := "reconciliation-" + uuid.NewString()
	meta, _ := json.Marshal(map[string]any{
		"provider":     provider,
		"period_start": start,
		"period_end":   end,
	})
	job := NewReconciliationJob(store, logStore, usageClient)
	r.Register(job.Kind(), job.Handle)
	return r.Enqueue(context.Background(), id, job.Kind(), string(meta), "")
}

// persistDegraded writes a batch for the two paths where the provider side
// cannot be fetched (unsupported provider / no usage client wired). The
// gateway side and its per-model items are still persisted so a later invoice
// upload lands on a batch that already carries Σgateway, and the note tells
// the operator exactly which affordance to use next.
//
// Delta is set to −Σgateway: with no provider figure the whole gateway spend
// is unaccounted for, which is the conservative reading (the batch shows as a
// loss until the provider side arrives rather than as a break-even).
func (j *ReconciliationJob) persistDegraded(
	ctx context.Context,
	provider string,
	periodStart, periodEnd time.Time,
	status string,
	note string,
	gatewayBuckets []gatewayModelTotal,
) (string, error) {
	row := &tables.TableBillingReconciliation{
		Provider:    provider,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Source:      ReconciliationSourceUsageAPI,
		Status:      status,
		Notes:       &note,
	}
	items := make([]tables.TableBillingReconItem, 0, len(gatewayBuckets))
	for _, b := range gatewayBuckets {
		items = append(items, tables.TableBillingReconItem{
			Model:           b.Model,
			GatewayRequests: b.Count,
			GatewayTokens:   b.Tokens,
			GatewayCost:     b.Cost,
		})
		row.GatewayCost += b.Cost
	}
	row.Delta = -row.GatewayCost
	if err := j.store.CreateReconciliation(ctx, row, items); err != nil {
		return "", fmt.Errorf("reconciliation job: persist %s batch: %w", status, err)
	}
	out, _ := json.Marshal(map[string]any{
		"id":               row.ID,
		"provider":         provider,
		"status":           status,
		"source":           row.Source,
		"gateway_cost":     row.GatewayCost,
		"provider_cost":    row.ProviderCost,
		"delta":            row.Delta,
		"note":             note,
		"requires_invoice": status == tables.ReconciliationStatusUnsupported,
	})
	return string(out), nil
}

// RunReconciliationNow executes one calibration batch inline and returns the
// job's metadata JSON. The HTTP handler uses this instead of Enqueue so the
// POST …/reconciliations/run response can carry the freshly written batch
// (id, status, delta, note) — an operator triggering a calibration by hand
// needs the result, not a queue receipt. The sidekiq path above stays for
// scheduled runs.
func RunReconciliationNow(ctx context.Context, store ReconciliationStore, logStore ReconciliationLogStore, usageClient UsageAPIClient, provider string, start, end time.Time) (string, error) {
	if store == nil || logStore == nil {
		return "", fmt.Errorf("reconciliation requires a config store and a log store")
	}
	meta, err := json.Marshal(map[string]any{
		"provider":     provider,
		"period_start": start,
		"period_end":   end,
	})
	if err != nil {
		return "", fmt.Errorf("reconciliation: encode metadata: %w", err)
	}
	job := NewReconciliationJob(store, logStore, usageClient)
	return job.Handle(ctx, tables.TableSidekiqJob{Metadata: string(meta)}, nil)
}
