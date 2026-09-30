package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/framework/sidekiq"
)

// fakeReconciliationStore records every CreateReconciliation / UpdateReconciliation
// call so the test can assert on the persisted shape without spinning up a
// real DB.
type fakeReconciliationStore struct {
	createCalls []createCall
	updateCalls []tables.TableBillingReconciliation
	createErr   error
}

type createCall struct {
	row   tables.TableBillingReconciliation
	items []tables.TableBillingReconItem
}

func (f *fakeReconciliationStore) CreateReconciliation(ctx context.Context, row *tables.TableBillingReconciliation, items []tables.TableBillingReconItem) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createCalls = append(f.createCalls, createCall{row: *row, items: append([]tables.TableBillingReconItem(nil), items...)})
	row.ID = "recon-test"
	for i := range items {
		items[i].ReconciliationID = row.ID
	}
	return nil
}

func (f *fakeReconciliationStore) UpdateReconciliation(ctx context.Context, row *tables.TableBillingReconciliation) error {
	f.updateCalls = append(f.updateCalls, *row)
	return nil
}

// fakeLogStore returns a deterministic model ranking per call. Tests set
// Rankings to whatever the scenario needs.
type fakeLogStore struct {
	rankings []logstore.ModelRankingWithTrend
	err      error
}

func (f *fakeLogStore) GetModelRankings(ctx context.Context, filters logstore.SearchFilters) (*logstore.ModelRankingResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &logstore.ModelRankingResult{Rankings: f.rankings}, nil
}

// fakeUsageClient returns whatever UsageRecords the test staged. err
// shortcuts the happy path to exercise the failure branch.
type fakeUsageClient struct {
	records []UsageRecord
	err     error
}

func (f *fakeUsageClient) FetchUsage(ctx context.Context, provider string, periodStart, periodEnd time.Time) ([]UsageRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.records, nil
}

func makeJobMeta(t *testing.T, provider string, start, end time.Time) tables.TableSidekiqJob {
	t.Helper()
	meta, err := json.Marshal(map[string]any{
		"provider":     provider,
		"period_start": start,
		"period_end":   end,
	})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	return tables.TableSidekiqJob{Metadata: string(meta)}
}

// TestReconciliationJobUnsupportedProviderWritesStub covers the "first
// wave mismatch" branch. The job MUST persist a row with status=unsupported
// rather than returning an error — that is what the admin UI relies on to
// render "please upload invoice".
func TestReconciliationJobUnsupportedProviderWritesStub(t *testing.T) {
	store := &fakeReconciliationStore{}
	logStore := &fakeLogStore{}
	job := NewReconciliationJob(store, logStore, &fakeUsageClient{})
	meta := makeJobMeta(t, "azure", time.Now().Add(-time.Hour), time.Now())
	if _, err := job.Handle(context.Background(), meta, nil); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(store.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1", len(store.createCalls))
	}
	if got := store.createCalls[0].row.Status; got != tables.ReconciliationStatusUnsupported {
		t.Errorf("status = %q, want unsupported", got)
	}
	if len(store.createCalls[0].items) != 0 {
		t.Errorf("items should be empty for unsupported batches, got %d", len(store.createCalls[0].items))
	}
}

// TestReconciliationJobMissingUsageClientWritesPending covers a first-wave
// provider (openai) on a build with no usage client wired. The run itself
// succeeded and the gateway side was captured, so the batch must land as
// pending — NOT unsupported — otherwise the admin UI would tell the operator
// to upload an invoice for a provider that does have a usage API.
func TestReconciliationJobMissingUsageClientWritesPending(t *testing.T) {
	store := &fakeReconciliationStore{}
	logStore := &fakeLogStore{
		rankings: []logstore.ModelRankingWithTrend{
			{ModelRankingEntry: logstore.ModelRankingEntry{Model: "gpt-4o", TotalRequests: 10, TotalTokens: 1000, TotalCost: 12.5}},
		},
	}
	job := NewReconciliationJob(store, logStore, nil)
	meta := makeJobMeta(t, "openai", time.Now().Add(-time.Hour), time.Now())
	if _, err := job.Handle(context.Background(), meta, nil); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(store.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1", len(store.createCalls))
	}
	row := store.createCalls[0].row
	if row.Status != tables.ReconciliationStatusPending {
		t.Errorf("status = %q, want pending", row.Status)
	}
	// The degraded path must still persist Σgateway so a later invoice upload
	// lands on a batch that already has the gateway half filled in.
	if row.GatewayCost != 12.5 {
		t.Errorf("gateway_cost = %v, want 12.5", row.GatewayCost)
	}
	if row.Delta != -12.5 {
		t.Errorf("delta = %v, want -12.5 (fully unaccounted until the provider side arrives)", row.Delta)
	}
	if len(store.createCalls[0].items) != 1 {
		t.Errorf("items = %d, want 1 gateway-side row", len(store.createCalls[0].items))
	}
}

// TestReconciliationJobUnsupportedProviderCapturesGatewaySide pins that the
// "please upload invoice" path is still a useful batch: the gateway half is
// computed even though no provider figure exists.
func TestReconciliationJobUnsupportedProviderCapturesGatewaySide(t *testing.T) {
	store := &fakeReconciliationStore{}
	logStore := &fakeLogStore{
		rankings: []logstore.ModelRankingWithTrend{
			{ModelRankingEntry: logstore.ModelRankingEntry{Model: "gpt-4o", TotalRequests: 4, TotalTokens: 400, TotalCost: 3}},
		},
	}
	job := NewReconciliationJob(store, logStore, &fakeUsageClient{})
	meta := makeJobMeta(t, "azure", time.Now().Add(-time.Hour), time.Now())
	if _, err := job.Handle(context.Background(), meta, nil); err != nil {
		t.Fatalf("handle: %v", err)
	}
	row := store.createCalls[0].row
	if row.Status != tables.ReconciliationStatusUnsupported {
		t.Fatalf("status = %q, want unsupported", row.Status)
	}
	if row.GatewayCost != 3 || len(store.createCalls[0].items) != 1 {
		t.Errorf("gateway side not captured: cost=%v items=%d", row.GatewayCost, len(store.createCalls[0].items))
	}
}

// TestReconciliationJobHappyPathWritesBatchAndItems is the end-to-end
// happy path. Verifies (a) one item per union-of-models, (b) token & price
// deltas computed correctly, (c) totals add up.
func TestReconciliationJobHappyPathWritesBatchAndItems(t *testing.T) {
	store := &fakeReconciliationStore{}
	logStore := &fakeLogStore{
		rankings: []logstore.ModelRankingWithTrend{
			{ModelRankingEntry: logstore.ModelRankingEntry{Model: "gpt-4o", TotalRequests: 100, TotalTokens: 1000, TotalCost: 80}},
			{ModelRankingEntry: logstore.ModelRankingEntry{Model: "gpt-4o-mini", TotalRequests: 200, TotalTokens: 2000, TotalCost: 20}},
		},
	}
	usage := &fakeUsageClient{records: []UsageRecord{
		{Model: "gpt-4o", ProviderRequests: 100, ProviderTokens: 1100, ProviderCost: 82},
	}}
	job := NewReconciliationJob(store, logStore, usage)
	start := time.Now().Add(-time.Hour)
	end := time.Now()
	meta := makeJobMeta(t, "openai", start, end)
	if _, err := job.Handle(context.Background(), meta, nil); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(store.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1", len(store.createCalls))
	}
	row := store.createCalls[0].row
	if row.Status != tables.ReconciliationStatusMatched {
		t.Errorf("status = %q, want matched", row.Status)
	}
	if row.GatewayCost != 100 {
		t.Errorf("gateway_cost = %v, want 100", row.GatewayCost)
	}
	if row.ProviderCost != 82 {
		t.Errorf("provider_cost = %v, want 82", row.ProviderCost)
	}
	if row.Delta != -18 {
		t.Errorf("delta = %v, want -18", row.Delta)
	}
	if len(store.createCalls[0].items) != 2 {
		t.Fatalf("items = %d, want 2 (gpt-4o + gpt-4o-mini)", len(store.createCalls[0].items))
	}
	// The gpt-4o-mini row must show 0 gateway match (provider-only).
	for _, it := range store.createCalls[0].items {
		if it.Model == "gpt-4o-mini" && it.ProviderCost != 0 {
			t.Errorf("gpt-4o-mini provider cost = %v, want 0 (no usage line)", it.ProviderCost)
		}
	}
}

// TestReconciliationJobUsageErrorSurfaces covers the failure mode where
// the usage API call itself fails. The handler returns the error so the
// runner marks the job as failed and retries up to MaxAttempts.
func TestReconciliationJobUsageErrorSurfaces(t *testing.T) {
	store := &fakeReconciliationStore{}
	logStore := &fakeLogStore{}
	usage := &fakeUsageClient{err: errors.New("network down")}
	job := NewReconciliationJob(store, logStore, usage)
	meta := makeJobMeta(t, "openai", time.Now().Add(-time.Hour), time.Now())
	if _, err := job.Handle(context.Background(), meta, nil); err == nil {
		t.Fatalf("expected error to surface")
	}
	if len(store.createCalls) != 0 {
		t.Errorf("no batch should be written on failure, got %d", len(store.createCalls))
	}
}

// TestReconciliationJobEnqueueNowRequiresRunner ensures the convenience
// helper refuses a nil runner rather than panicking — the same guard the
// snapshot job uses.
func TestReconciliationJobEnqueueNowRequiresRunner(t *testing.T) {
	if err := EnqueueReconciliationNow(nil, nil, nil, nil, "openai", time.Now(), time.Now()); err == nil {
		t.Fatalf("expected nil runner to error")
	}
	// The runner's nil path is the only thing we can exercise without a
	// full sidekiq runner. Smoke-test the registry path is sound by
	// constructing a bare struct and ensuring the kind matches the
	// registered name.
	job := NewReconciliationJob(nil, nil, nil)
	if job.Kind() != "reconciliation" {
		t.Errorf("kind = %q, want reconciliation", job.Kind())
	}
	_ = sidekiq.ProgressFunc(nil) // silence unused import linter
}

// TestRunReconciliationNowRequiresStores guards the synchronous helper the
// HTTP handler uses: a nil store/logStore must error out rather than panic
// while the handler is already inside a request.
func TestRunReconciliationNowRequiresStores(t *testing.T) {
	if _, err := RunReconciliationNow(context.Background(), nil, nil, nil, "openai", time.Now(), time.Now()); err == nil {
		t.Fatalf("expected nil stores to error")
	}
}

// TestRunReconciliationNowReturnsBatchMeta pins the inline path the handler
// depends on: the metadata JSON must carry the persisted batch id and status
// so POST …/reconciliations/run can echo the created batch back to the caller.
func TestRunReconciliationNowReturnsBatchMeta(t *testing.T) {
	store := &fakeReconciliationStore{}
	logStore := &fakeLogStore{
		rankings: []logstore.ModelRankingWithTrend{
			{ModelRankingEntry: logstore.ModelRankingEntry{Model: "gpt-4o", TotalRequests: 1, TotalTokens: 10, TotalCost: 1}},
		},
	}
	out, err := RunReconciliationNow(context.Background(), store, logStore, nil, "openai", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(out), &meta); err != nil {
		t.Fatalf("metadata is not JSON: %v (%q)", err, out)
	}
	if meta["status"] != tables.ReconciliationStatusPending {
		t.Errorf("status = %v, want pending", meta["status"])
	}
	if _, ok := meta["id"]; !ok {
		t.Errorf("metadata must carry the persisted batch id, got %q", out)
	}
}
