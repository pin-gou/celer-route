package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/logstore"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeStandardPriceLookup serves GetActiveStandardPrice from a map keyed by
// provider+"\x00"+model, so a test can pin exact-match and wildcard behaviour
// independently.
type fakeStandardPriceLookup struct {
	rows map[string]*tables.TableStandardPrice
	err  error
}

func (f *fakeStandardPriceLookup) GetActiveStandardPrice(_ context.Context, provider, model string, _ time.Time) (*tables.TableStandardPrice, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[provider+"\x00"+model], nil
}

// fakeReconciliationStore is the in-memory ReconciliationStore.
type fakeReconciliationStore struct {
	batch       *tables.TableBillingReconciliation
	items       []tables.TableBillingReconItem
	createCalls []*tables.TableBillingReconciliation
	updateCalls []*tables.TableBillingReconciliation
	lookupErr   error
	// applyErr injects a failure into ApplyReconciliationTx so tests can
	// assert the batch is not marked applied when the commit fails.
	applyErr error
	// applyCalls counts successful atomic applies; appliedRows holds the
	// datasheet rows that actually became durable.
	applyCalls  int
	appliedRows []tables.TableModelPricing
	// persistedStatus mirrors the DURABLE batch status, as opposed to the
	// in-memory `batch` struct the handler mutates in place before it calls
	// ApplyReconciliationTx. The real store re-reads the row inside its
	// transaction, so the already-applied guard must compare against what was
	// committed — reading `batch.Status` here would see the handler's own
	// pending write (same pointer) and refuse every legitimate apply.
	persistedStatus string
}

func (f *fakeReconciliationStore) ListReconciliations(context.Context, configstore.ReconciliationQueryParams) ([]tables.TableBillingReconciliation, int64, error) {
	if f.batch == nil {
		return nil, 0, nil
	}
	return []tables.TableBillingReconciliation{*f.batch}, 1, nil
}

func (f *fakeReconciliationStore) GetReconciliationByID(context.Context, string) (*tables.TableBillingReconciliation, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	if f.batch == nil {
		return nil, configstore.ErrNotFound
	}
	// Snapshot the durable status on first read. From here on the handler owns
	// the `batch` pointer and mutates it pre-commit, so this is the only place
	// the persisted value can be captured faithfully.
	if f.persistedStatus == "" {
		f.persistedStatus = f.batch.Status
	}
	return f.batch, nil
}

func (f *fakeReconciliationStore) ListReconciliationItems(context.Context, string) ([]tables.TableBillingReconItem, error) {
	return f.items, nil
}

func (f *fakeReconciliationStore) CreateReconciliation(_ context.Context, row *tables.TableBillingReconciliation, items []tables.TableBillingReconItem) error {
	if row.ID == "" {
		row.ID = "recon-test"
	}
	f.createCalls = append(f.createCalls, row)
	f.batch = row
	f.items = items
	f.persistedStatus = row.Status
	return nil
}

func (f *fakeReconciliationStore) UpdateReconciliation(_ context.Context, row *tables.TableBillingReconciliation) error {
	f.updateCalls = append(f.updateCalls, row)
	f.batch = row
	f.persistedStatus = row.Status
	return nil
}

// ApplyReconciliationTx models the H-2 atomic commit: the corrected datasheet
// rows and the batch status flip either both land or neither does.
//
// applyErr lets a test inject a commit failure, and appliedRows records the
// rows that actually became durable so a rollback assertion can distinguish
// "staged" from "committed".
func (f *fakeReconciliationStore) ApplyReconciliationTx(_ context.Context, row *tables.TableBillingReconciliation, correctedRows []tables.TableModelPricing) error {
	// Mirrors the store's in-transaction re-read of the durable row. Compare
	// against persistedStatus, NOT row.Status — the handler already flipped
	// that in place before committing.
	if f.persistedStatus == "" && f.batch != nil {
		f.persistedStatus = f.batch.Status
	}
	if f.persistedStatus == tables.ReconciliationStatusApplied {
		return configstore.ErrReconciliationAlreadyApplied
	}
	if f.applyErr != nil {
		return f.applyErr
	}
	f.appliedRows = append(f.appliedRows, correctedRows...)
	f.applyCalls++
	cp := *row
	cp.Status = tables.ReconciliationStatusApplied
	f.batch = &cp
	f.persistedStatus = tables.ReconciliationStatusApplied
	f.updateCalls = append(f.updateCalls, &cp)
	return nil
}

// fakeDatasheetStore is the cost-side slice used by preview/apply.
type fakeDatasheetStore struct {
	rows    []tables.TableModelPricing
	upserts []*tables.TableModelPricing
	err     error
}

func (f *fakeDatasheetStore) GetModelPrices(context.Context) ([]tables.TableModelPricing, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

func (f *fakeDatasheetStore) UpsertModelPrices(_ context.Context, pricing *tables.TableModelPricing, _ ...*gorm.DB) error {
	if f.err != nil {
		return f.err
	}
	f.upserts = append(f.upserts, pricing)
	return nil
}

// fakeCacheStatsProvider returns a fixed tracker snapshot.
type fakeCacheStatsProvider struct{ snap CacheStatsSnapshotShape }

func (f fakeCacheStatsProvider) Stats() CacheStatsSnapshotShape { return f.snap }

// ---------------------------------------------------------------------------
// Gateway delta
// ---------------------------------------------------------------------------

func TestParseCostAccuracy(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", costAccuracyAll, false},
		{"all", costAccuracyAll, false},
		{"provider_reported", "provider_reported", false},
		{"gateway_estimated", "gateway_estimated", false},
		{"unknown", "unknown", false},
		{"provider-reported", "", true},
		{"bogus", "", true},
	}
	for _, tc := range cases {
		ctx := newReportsCtx()
		if tc.in != "" {
			ctx.QueryArgs().Set("accuracy", tc.in)
		}
		got, ok := parseCostAccuracy(ctx)
		if tc.wantErr {
			if ok {
				t.Errorf("accuracy=%q: expected rejection", tc.in)
			}
			if ctx.Response.StatusCode() != http.StatusBadRequest {
				t.Errorf("accuracy=%q: status = %d, want 400", tc.in, ctx.Response.StatusCode())
			}
			continue
		}
		if !ok {
			t.Errorf("accuracy=%q: unexpected rejection", tc.in)
			continue
		}
		if got != tc.want {
			t.Errorf("accuracy=%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestDeltaFilters pins the two properties the report relies on: the accuracy
// band reaches the log-store filter, and the ranking cap is lifted so the long
// tail of unpriced models can't be silently dropped from the loss side.
func TestDeltaFilters(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	all := deltaFilters(start, end, costAccuracyAll)
	if len(all.CostAccuracy) != 0 {
		t.Errorf("accuracy=all must not filter: got %v", all.CostAccuracy)
	}
	if all.RankingLimit == nil || *all.RankingLimit != 0 {
		t.Errorf("RankingLimit must be uncapped (0), got %v", all.RankingLimit)
	}

	banded := deltaFilters(start, end, "provider_reported")
	if len(banded.CostAccuracy) != 1 || banded.CostAccuracy[0] != "provider_reported" {
		t.Errorf("CostAccuracy = %v, want [provider_reported]", banded.CostAccuracy)
	}
}

func TestTokenSplitTableApply(t *testing.T) {
	table := &tokenSplitTable{byProvider: map[string]tokenSplit{"openai": {promptRatio: 0.25}}}

	prompt, completion := table.apply("openai", 1000)
	if prompt != 250 || completion != 750 {
		t.Errorf("openai split = %d/%d, want 250/750", prompt, completion)
	}
	// Unknown provider falls back to an even split rather than dropping tokens.
	prompt, completion = table.apply("mystery", 999)
	if prompt != 499 || completion != 500 {
		t.Errorf("default split = %d/%d, want 499/500", prompt, completion)
	}
	if p, c := table.apply("openai", 0); p != 0 || c != 0 {
		t.Errorf("zero tokens must split to 0/0, got %d/%d", p, c)
	}
}

func TestStandardPricerExactThenWildcard(t *testing.T) {
	exact := &tables.TableStandardPrice{Provider: "openai", Model: "gpt-4o", InputCostPerMillion: 2, OutputCostPerMillion: 8}
	wildcard := &tables.TableStandardPrice{Provider: "openai", Model: standardPriceWildcardModel, InputCostPerMillion: 1, OutputCostPerMillion: 4}
	lookup := &fakeStandardPriceLookup{rows: map[string]*tables.TableStandardPrice{
		"openai\x00gpt-4o":                        exact,
		"openai\x00" + standardPriceWildcardModel: wildcard,
	}}
	h := &ReportsGatewayDeltaHandler{store: lookup}
	pricer := h.newStandardPricer(newReportsCtx(), time.Now())

	// 1M prompt @ $2/M + 1M completion @ $8/M = $10.
	cost, ok := pricer.price("openai", "gpt-4o", 1_000_000, 1_000_000, 0)
	if !ok || cost != 10 {
		t.Fatalf("exact price = %v (ok=%v), want 10", cost, ok)
	}
	// Unknown model falls back to the provider default row: $1 + $4 = $5.
	cost, ok = pricer.price("openai", "gpt-9-unknown", 1_000_000, 1_000_000, 0)
	if !ok || cost != 5 {
		t.Fatalf("wildcard price = %v (ok=%v), want 5", cost, ok)
	}
	// A provider with no row at all reports "unpriced" rather than 0-cost.
	if _, ok := pricer.price("anthropic", "claude", 1_000_000, 0, 0); ok {
		t.Fatal("expected unpriced provider to report ok=false")
	}
}

func TestGatewayDeltaDegradesWithoutLogStore(t *testing.T) {
	h := NewReportsGatewayDeltaHandler(nil, nil)
	ctx := newReportsCtx()
	h.gatewayDelta(ctx)
	if ctx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded)", ctx.Response.StatusCode())
	}
	var body map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if note, _ := body["note"].(string); !strings.Contains(note, "log store not configured") {
		t.Errorf("expected a degraded note, got %#v", body["note"])
	}
}

// ---------------------------------------------------------------------------
// Reconciliation
// ---------------------------------------------------------------------------

func TestParseInvoiceCSV(t *testing.T) {
	body := []byte("model,requests,tokens,cost\n" +
		"gpt-4o,10,1000,$12.50\n" +
		"gpt-4o-mini,20,2000,3\n")
	recs, err := parseInvoiceCSV(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("rows = %d, want 2", len(recs))
	}
	if recs[0].Model != "gpt-4o" || recs[0].Requests != 10 || recs[0].Tokens != 1000 || recs[0].Cost != 12.5 {
		t.Errorf("row 0 = %+v", recs[0])
	}
	if recs[1].Cost != 3 {
		t.Errorf("row 1 cost = %v, want 3", recs[1].Cost)
	}
}

// TestParseInvoiceCSVAcceptsSupplierAliases pins the header aliasing: every
// supplier exports a different column name, and rejecting "amount" would make
// the fallback path useless for exactly the providers it exists for.
func TestParseInvoiceCSVAcceptsSupplierAliases(t *testing.T) {
	body := []byte("\ufeffmodel_name, calls , total_tokens, amount\n" +
		"claude-3-5-sonnet,5,5000,\"$1,234.00\"\n")
	recs, err := parseInvoiceCSV(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("rows = %d, want 1", len(recs))
	}
	if recs[0].Model != "claude-3-5-sonnet" || recs[0].Requests != 5 || recs[0].Tokens != 5000 || recs[0].Cost != 1234 {
		t.Errorf("row = %+v", recs[0])
	}
}

func TestParseInvoiceCSVErrors(t *testing.T) {
	if _, err := parseInvoiceCSV([]byte("model,cost\n")); err == nil {
		t.Error("expected an error for a header with no data rows")
	}
	if _, err := parseInvoiceCSV([]byte("cost\n1.0\n")); err == nil {
		t.Error("expected an error when the model column is missing")
	}
	if _, err := parseInvoiceCSV([]byte("model,cost\ngpt-4o,not-a-number\n")); err == nil {
		t.Error("expected an error for a non-numeric cost")
	}
}

func TestReconciliationRunRequiresProvider(t *testing.T) {
	h := NewReportsReconciliationHandler(&fakeReconciliationStore{}, nil, nil, nil)
	ctx := newReportsCtx()
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBody([]byte(`{"period":"2026-09"}`))
	h.run(ctx)
	if ctx.Response.StatusCode() != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", ctx.Response.StatusCode())
	}
}

func TestReconciliationRunRejectsBadPeriod(t *testing.T) {
	h := NewReportsReconciliationHandler(&fakeReconciliationStore{}, nil, nil, nil)
	for _, body := range []string{
		`{"provider":"openai"}`,                             // no period at all
		`{"provider":"openai","period":"2026"}`,             // not YYYY-MM
		`{"provider":"openai","period_start":"2026-09-01"}`, // half a range
	} {
		ctx := newReportsCtx()
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetBody([]byte(body))
		h.run(ctx)
		if ctx.Response.StatusCode() != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, ctx.Response.StatusCode())
		}
	}
}

// TestReconciliationRunTellsOperatorToUploadInvoice is the first-wave
// contract: a provider outside openai/anthropic/deepseek must come back with
// the "upload an invoice" affordance rather than a silent empty batch.
func TestReconciliationRunTellsOperatorToUploadInvoice(t *testing.T) {
	// logStore is nil here so the handler 503s before reaching the job; that
	// guard is what keeps a half-computed batch out of the DB. Assert the
	// message so the check is meaningful.
	h := NewReportsReconciliationHandler(&fakeReconciliationStore{}, nil, nil, nil)
	ctx := newReportsCtx()
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBody([]byte(`{"provider":"azure","period":"2026-09"}`))
	h.run(ctx)
	if ctx.Response.StatusCode() != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", ctx.Response.StatusCode())
	}
	if !strings.Contains(string(ctx.Response.Body()), "log store not configured") {
		t.Errorf("unexpected body %s", ctx.Response.Body())
	}
}

func TestReconciliationDetailNotFound(t *testing.T) {
	h := NewReportsReconciliationHandler(&fakeReconciliationStore{}, nil, nil, nil)
	ctx := newReportsCtx()
	ctx.SetUserValue("id", "missing")
	h.detail(ctx)
	if ctx.Response.StatusCode() != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", ctx.Response.StatusCode())
	}
}

// TestReconciliationPlanComputesCorrection pins the datasheet math: the
// correction ratio is provider/gateway cost and both text rates scale by it.
func TestReconciliationPlanComputesCorrection(t *testing.T) {
	in := 0.000002
	out := 0.000008
	store := &fakeReconciliationStore{
		batch: &tables.TableBillingReconciliation{
			ID:           "recon-1",
			Provider:     "openai",
			PeriodStart:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
			Status:       tables.ReconciliationStatusMatched,
			GatewayCost:  100,
			ProviderCost: 125,
		},
		items: []tables.TableBillingReconItem{{Model: "gpt-4o", GatewayCost: 100, ProviderCost: 125}},
	}
	datasheet := &fakeDatasheetStore{rows: []tables.TableModelPricing{
		{Provider: "OpenAI", Model: "gpt-4o", Mode: "chat", InputCostPerToken: &in, OutputCostPerToken: &out},
	}}
	h := NewReportsReconciliationHandler(store, nil, datasheet, nil)
	ctx := newReportsCtx()
	plan, err := h.plan(ctx, "recon-1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !plan.CanApply {
		t.Fatalf("expected a applyable plan, blocked: %s", plan.BlockedReason)
	}
	if plan.CorrectionRatio != 1.25 {
		t.Errorf("ratio = %v, want 1.25", plan.CorrectionRatio)
	}
	if len(plan.Changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(plan.Changes))
	}
	change := plan.Changes[0]
	if change.InputBefore != in || change.OutputBefore != out {
		t.Errorf("before = %v/%v, want %v/%v", change.InputBefore, change.OutputBefore, in, out)
	}
	if change.InputAfter != in*1.25 || change.OutputAfter != out*1.25 {
		t.Errorf("after = %v/%v, want %v/%v", change.InputAfter, change.OutputAfter, in*1.25, out*1.25)
	}
	// Provider is normalized lower-case by indexDatasheet so the batch's
	// normalized provider matches the datasheet's original casing.
	if change.Provider != "openai" {
		t.Errorf("provider = %q, want openai", change.Provider)
	}
}

// TestReconciliationPlanBlocksOnNoProviderCost covers the common state right
// after `run` on a build with no usage client: the batch has a gateway half
// but no provider half, so applying it would be meaningless.
func TestReconciliationPlanBlocksOnNoProviderCost(t *testing.T) {
	store := &fakeReconciliationStore{
		batch: &tables.TableBillingReconciliation{
			ID: "recon-2", Provider: "openai",
			PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now(),
			Status: tables.ReconciliationStatusPending, GatewayCost: 10,
		},
	}
	h := NewReportsReconciliationHandler(store, nil, &fakeDatasheetStore{}, nil)
	plan, err := h.plan(newReportsCtx(), "recon-2")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.CanApply {
		t.Fatal("expected can_apply=false without a provider cost")
	}
	if !strings.Contains(plan.BlockedReason, "provider cost") {
		t.Errorf("blocked reason = %q", plan.BlockedReason)
	}
}

func TestReconciliationApplyRefusesAlreadyApplied(t *testing.T) {
	in := 0.000002
	store := &fakeReconciliationStore{
		batch: &tables.TableBillingReconciliation{
			ID: "recon-3", Provider: "openai",
			PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now(),
			Status: tables.ReconciliationStatusApplied, GatewayCost: 100, ProviderCost: 120,
		},
		items: []tables.TableBillingReconItem{{Model: "gpt-4o"}},
	}
	datasheet := &fakeDatasheetStore{rows: []tables.TableModelPricing{
		{Provider: "openai", Model: "gpt-4o", Mode: "chat", InputCostPerToken: &in},
	}}
	h := NewReportsReconciliationHandler(store, nil, datasheet, nil)
	ctx := newReportsCtx()
	ctx.SetUserValue("id", "recon-3")
	h.apply(ctx)
	if ctx.Response.StatusCode() != http.StatusConflict {
		t.Fatalf("status = %d, want 409", ctx.Response.StatusCode())
	}
	if len(datasheet.upserts) != 0 {
		t.Errorf("a refused apply must not write the datasheet (%d writes)", len(datasheet.upserts))
	}
	if store.applyCalls != 0 {
		t.Errorf("a refused apply must not open the atomic commit (%d calls)", store.applyCalls)
	}
	if len(store.updateCalls) != 0 {
		t.Errorf("a refused apply must not touch the batch (%d updates)", len(store.updateCalls))
	}
}

// TestReconciliationApplyWritesDatasheetAndMarksApplied is the happy path:
// the datasheet moves, the batch flips to applied, and the pre-change rates
// are snapshotted into the notes so a re-run can't compound.
func TestReconciliationApplyWritesDatasheetAndMarksApplied(t *testing.T) {
	in := 0.000002
	out := 0.000008
	store := &fakeReconciliationStore{
		batch: &tables.TableBillingReconciliation{
			ID: "recon-4", Provider: "openai",
			PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now(),
			Status: tables.ReconciliationStatusMatched, GatewayCost: 100, ProviderCost: 110,
		},
		items: []tables.TableBillingReconItem{{Model: "gpt-4o", GatewayCost: 100, ProviderCost: 110}},
	}
	datasheet := &fakeDatasheetStore{rows: []tables.TableModelPricing{
		{Provider: "openai", Model: "gpt-4o", Mode: "chat", InputCostPerToken: &in, OutputCostPerToken: &out},
	}}
	h := NewReportsReconciliationHandler(store, nil, datasheet, nil)
	ctx := newReportsCtx()
	ctx.SetUserValue("id", "recon-4")
	h.apply(ctx)

	if ctx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	// Since H-2 the datasheet writes go through the atomic apply, so the
	// corrected rows land on the store rather than on the datasheet fake.
	if len(store.appliedRows) != 1 {
		t.Fatalf("committed datasheet rows = %d, want 1", len(store.appliedRows))
	}
	if store.applyCalls != 1 {
		t.Errorf("atomic apply calls = %d, want exactly 1", store.applyCalls)
	}
	if got := *store.appliedRows[0].InputCostPerToken; got != in*1.1 {
		t.Errorf("input after = %v, want %v", got, in*1.1)
	}
	if got := *store.appliedRows[0].OutputCostPerToken; got != out*1.1 {
		t.Errorf("output after = %v, want %v", got, out*1.1)
	}
	if len(store.updateCalls) != 1 || store.updateCalls[0].Status != tables.ReconciliationStatusApplied {
		t.Fatalf("batch not marked applied: %+v", store.updateCalls)
	}
	notes := ""
	if store.updateCalls[0].Notes != nil {
		notes = *store.updateCalls[0].Notes
	}
	if !strings.Contains(notes, "snapshot=") {
		t.Errorf("notes must carry the pre-change snapshot, got %q", notes)
	}
}

// ---------------------------------------------------------------------------
// Cache savings
// ---------------------------------------------------------------------------

func TestApplyCacheSavingsEstimator(t *testing.T) {
	all := &logstore.SearchStats{
		DirectCacheHits:   ptr(int64(30)),
		SemanticCacheHits: ptr(int64(20)),
		PromptTokens:      1_000_000,
		TotalCost:         100,
		TotalRequests:     100,
	}
	hitsStats := &logstore.SearchStats{PromptTokens: 400_000}

	resp := &cacheSavingsResponse{}
	applyCacheSavings(resp, all, hitsStats)

	if resp.Hits != 50 {
		t.Errorf("hits = %d, want 50", resp.Hits)
	}
	if resp.DirectHits != 30 || resp.SemanticHits != 20 {
		t.Errorf("direct/semantic = %d/%d, want 30/20", resp.DirectHits, resp.SemanticHits)
	}
	if resp.SavedInputTokens != 400_000 {
		t.Errorf("saved tokens = %d, want 400000", resp.SavedInputTokens)
	}
	// Missed prompt tokens = 1_000_000 - 400_000 = 600_000, so the missed
	// rate is 100/600_000 and the savings estimate is 400_000 * that = 66.67.
	want := 400_000 * (100.0 / 600_000)
	if diff := resp.EstimatedSavedCost - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("estimated saved cost = %v, want %v", resp.EstimatedSavedCost, want)
	}
	if got := resp.HitRate; got != 0.5 {
		t.Errorf("hit rate = %v, want 0.5", got)
	}
	if resp.ActualCost != 100 || resp.PromptTokens != 1_000_000 {
		t.Errorf("echoed actual_cost/prompt_tokens = %v/%d, want 100/1000000", resp.ActualCost, resp.PromptTokens)
	}
}

func TestApplyCacheSavingsNoMissedTraffic(t *testing.T) {
	// Every prompt token was served from cache — there is no missed-request
	// rate, so the estimate must stay 0 rather than dividing by zero.
	all := &logstore.SearchStats{PromptTokens: 1000, TotalCost: 5, TotalRequests: 10, DirectCacheHits: ptr(int64(10))}
	hitsStats := &logstore.SearchStats{PromptTokens: 1000}
	resp := &cacheSavingsResponse{}
	applyCacheSavings(resp, all, hitsStats)
	if resp.EstimatedSavedCost != 0 {
		t.Errorf("estimated saved cost = %v, want 0", resp.EstimatedSavedCost)
	}
	if !strings.Contains(resp.Note, "came from cache") {
		t.Errorf("expected an explanatory note, got %q", resp.Note)
	}
}

func TestCacheSavingsDegradesWithoutLogStore(t *testing.T) {
	h := NewReportsCacheHandler(nil, func() CacheStatsProvider {
		return fakeCacheStatsProvider{snap: CacheStatsSnapshotShape{Hits: 3, Misses: 7, HitRate: 0.3, SavedInputTokens: 42, SavedCost: 1.5}}
	})
	ctx := newReportsCtx()
	h.savings(ctx)
	if ctx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want 200", ctx.Response.StatusCode())
	}
	var body map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	tracker, ok := body["tracker"].(map[string]any)
	if !ok {
		t.Fatalf("expected a tracker block, got %#v", body["tracker"])
	}
	if tracker["hits"].(float64) != 3 || tracker["misses"].(float64) != 7 {
		t.Errorf("tracker = %#v", tracker)
	}
}

func TestCacheStatsHandlerReturnsSnapshot(t *testing.T) {
	h := NewCacheStatsHandler(func() CacheStatsProvider {
		return fakeCacheStatsProvider{snap: CacheStatsSnapshotShape{Hits: 9, Misses: 1, HitRate: 0.9, SavedInputTokens: 100, SavedCost: 2.25}}
	})
	ctx := newReportsCtx()
	h.stats(ctx)
	if ctx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want 200", ctx.Response.StatusCode())
	}
	var body map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body["hits"].(float64) != 9 || body["saved_cost"].(float64) != 2.25 {
		t.Errorf("body = %#v", body)
	}
}

func TestCacheStatsHandlerWithoutPlugin(t *testing.T) {
	h := NewCacheStatsHandler(func() CacheStatsProvider { return nil })
	ctx := newReportsCtx()
	h.stats(ctx)
	if ctx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want 200", ctx.Response.StatusCode())
	}
	var body map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body["hits"].(float64) != 0 {
		t.Errorf("expected zeroed snapshot, got %#v", body)
	}
}
