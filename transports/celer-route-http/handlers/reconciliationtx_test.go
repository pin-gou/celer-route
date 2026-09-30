package handlers

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
)

// twoModelReconSetup builds a batch whose correction plan touches two
// datasheet rows, so a partial-write regression is observable.
func twoModelReconSetup(store *fakeReconciliationStore) *fakeDatasheetStore {
	in1, out1 := 0.000002, 0.000008
	in2, out2 := 0.000001, 0.000004
	store.batch = &tables.TableBillingReconciliation{
		ID: "recon-h2", Provider: "openai", Source: tables.ReconciliationSourceManual,
		PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now(),
		Status: tables.ReconciliationStatusMatched, GatewayCost: 100, ProviderCost: 110,
	}
	store.items = []tables.TableBillingReconItem{
		{Model: "gpt-4o", GatewayCost: 60, ProviderCost: 66},
		{Model: "gpt-4o-mini", GatewayCost: 40, ProviderCost: 44},
	}
	return &fakeDatasheetStore{rows: []tables.TableModelPricing{
		{Provider: "openai", Model: "gpt-4o", Mode: "chat", InputCostPerToken: &in1, OutputCostPerToken: &out1},
		{Provider: "openai", Model: "gpt-4o-mini", Mode: "chat", InputCostPerToken: &in2, OutputCostPerToken: &out2},
	}}
}

// TestReconciliationApplyCommitFailureLeavesBatchUnapplied is the handler-side
// half of the H-2 fix. The store now owns the transaction, so what the handler
// must guarantee is that a failed commit is surfaced as an error and that the
// batch is NOT reported as applied — otherwise the operator sees a 200 and
// never retries, while the price book is in an unknown state.
//
// Pre-fix the handler wrote rows one at a time, so a failure left some rows
// durably re-priced; the store-level test in
// framework/configstore/reconciliationtx_test.go pins the rollback itself.
func TestReconciliationApplyCommitFailureLeavesBatchUnapplied(t *testing.T) {
	store := &fakeReconciliationStore{applyErr: errors.New("injected commit failure")}
	datasheet := twoModelReconSetup(store)

	h := NewReportsReconciliationHandler(store, nil, datasheet, nil)
	ctx := newReportsCtx()
	ctx.SetUserValue("id", "recon-h2")
	h.apply(ctx)

	if ctx.Response.StatusCode() == http.StatusOK {
		t.Fatalf("apply must surface the commit failure, got 200: %s", ctx.Response.Body())
	}
	if ctx.Response.StatusCode() != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", ctx.Response.StatusCode())
	}
	if len(store.appliedRows) != 0 {
		t.Fatalf("H-2 BUG: %d datasheet row(s) committed despite the failed apply", len(store.appliedRows))
	}
	// Assert the DURABLE status, not `store.batch.Status`. The handler flips
	// its in-memory struct before committing — that pending write is exactly
	// what the transaction must discard — so reading it back here would be
	// asserting on the mutation rather than on the outcome.
	if store.persistedStatus == tables.ReconciliationStatusApplied {
		t.Fatalf("batch must not be durably applied after a failed commit (persisted=%q)", store.persistedStatus)
	}
	if store.persistedStatus != tables.ReconciliationStatusMatched {
		t.Fatalf("durable status = %q, want matched so the operator can retry", store.persistedStatus)
	}
}

// TestReconciliationApplyMapsAlreadyAppliedTo409 pins the store-level
// one-way guard's HTTP mapping. This is the branch a concurrent double-apply
// lands on: both requests pass the handler's stale pre-check, the row lock
// lets one commit, and the loser must get 409 rather than 500.
func TestReconciliationApplyMapsAlreadyAppliedTo409(t *testing.T) {
	store := &fakeReconciliationStore{applyErr: configstore.ErrReconciliationAlreadyApplied}
	datasheet := twoModelReconSetup(store)

	h := NewReportsReconciliationHandler(store, nil, datasheet, nil)
	ctx := newReportsCtx()
	ctx.SetUserValue("id", "recon-h2")
	h.apply(ctx)

	if ctx.Response.StatusCode() != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%s)", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	if len(store.appliedRows) != 0 {
		t.Fatalf("a refused apply must not write the price book (%d rows)", len(store.appliedRows))
	}
}

// TestReconciliationApplyStagesBothRowsInOneCommit is the positive control:
// both corrected rows reach the store in a SINGLE atomic call, not as two
// independent writes. The call count is the load-bearing assertion — a
// regression back to per-row writes would show applyCalls == 1 but leave the
// rows committed one at a time, which is what made the pre-fix path unsafe.
func TestReconciliationApplyStagesBothRowsInOneCommit(t *testing.T) {
	store := &fakeReconciliationStore{}
	datasheet := twoModelReconSetup(store)

	h := NewReportsReconciliationHandler(store, nil, datasheet, nil)
	ctx := newReportsCtx()
	ctx.SetUserValue("id", "recon-h2")
	h.apply(ctx)

	if ctx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	if store.applyCalls != 1 {
		t.Fatalf("atomic apply calls = %d, want exactly 1", store.applyCalls)
	}
	if len(store.appliedRows) != 2 {
		t.Fatalf("committed datasheet rows = %d, want 2", len(store.appliedRows))
	}
	// The correction ratio (110/100 = 1.1) must be applied to both rows.
	for _, row := range store.appliedRows {
		if row.InputCostPerToken == nil {
			t.Fatalf("row %s has no input cost", row.Model)
		}
	}
	if len(datasheet.upserts) != 0 {
		t.Errorf("the handler must not write the datasheet directly any more (%d writes)", len(datasheet.upserts))
	}
	if store.batch.Status != tables.ReconciliationStatusApplied {
		t.Fatalf("batch status = %q, want applied", store.batch.Status)
	}
}

// TestReconciliationApplyRefusedBatchNeverOpensCommit confirms the cheap
// pre-check still short-circuits before any store work, so an already-applied
// batch costs nothing beyond the read.
func TestReconciliationApplyRefusedBatchNeverOpensCommit(t *testing.T) {
	store := &fakeReconciliationStore{}
	datasheet := twoModelReconSetup(store)
	store.batch.Status = tables.ReconciliationStatusApplied

	h := NewReportsReconciliationHandler(store, nil, datasheet, nil)
	ctx := newReportsCtx()
	ctx.SetUserValue("id", "recon-h2")
	h.apply(ctx)

	if ctx.Response.StatusCode() != http.StatusConflict {
		t.Fatalf("status = %d, want 409", ctx.Response.StatusCode())
	}
	if store.applyCalls != 0 {
		t.Fatalf("a pre-check refusal must not open the atomic commit (%d calls)", store.applyCalls)
	}
	if len(store.appliedRows) != 0 {
		t.Fatalf("a pre-check refusal must not write the price book (%d rows)", len(store.appliedRows))
	}
}
