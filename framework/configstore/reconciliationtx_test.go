package configstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupReconTxTestStore migrates both halves the atomic apply touches: the
// reconciliation batch tables and the datasheet pricing table. They live in
// the same database, which is the precondition that makes a single
// transaction across both possible.
func setupReconTxTestStore(t *testing.T) *RDBConfigStore {
	t.Helper()
	store := setupReportsTestStore(t)
	require.NoError(t, store.DB().AutoMigrate(&tables.TableModelPricing{}))
	return store
}

func seedReconBatch(t *testing.T, store *RDBConfigStore, id, status string) *tables.TableBillingReconciliation {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	batch := &tables.TableBillingReconciliation{
		ID:           id,
		Provider:     "openai",
		Source:       tables.ReconciliationSourceManual,
		PeriodStart:  now.Add(-24 * time.Hour),
		PeriodEnd:    now,
		Status:       status,
		GatewayCost:  100,
		ProviderCost: 110,
		Delta:        10,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.CreateReconciliation(ctx, batch, nil))
	return batch
}

func seedDatasheetRow(t *testing.T, store *RDBConfigStore, model string, in, out float64) {
	t.Helper()
	ctx := context.Background()
	row := &tables.TableModelPricing{
		Provider:           "openai",
		Model:              model,
		Mode:               "chat",
		InputCostPerToken:  &in,
		OutputCostPerToken: &out,
	}
	require.NoError(t, store.UpsertModelPrices(ctx, row))
}

func countDatasheetRows(t *testing.T, store *RDBConfigStore) int64 {
	t.Helper()
	var n int64
	require.NoError(t, store.DB().WithContext(context.Background()).
		Model(&tables.TableModelPricing{}).Count(&n).Error)
	return n
}

func reconBatchStatus(t *testing.T, store *RDBConfigStore, id string) string {
	t.Helper()
	b, err := store.GetReconciliationByID(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, b)
	return b.Status
}

// TestApplyReconciliationTxHappyPath is the positive control: both datasheet
// rows are re-priced and the batch flips to applied in one commit.
func TestApplyReconciliationTxHappyPath(t *testing.T) {
	store := setupReconTxTestStore(t)
	ctx := context.Background()
	batch := seedReconBatch(t, store, "recon-tx-ok", tables.ReconciliationStatusMatched)
	seedDatasheetRow(t, store, "gpt-4o", 0.000002, 0.000008)

	in := 0.0000022
	batch.Status = tables.ReconciliationStatusApplied
	note := "applied calibration"
	batch.Notes = &note
	corrected := []tables.TableModelPricing{{
		Provider:          "openai",
		Model:             "gpt-4o",
		Mode:              "chat",
		InputCostPerToken: &in,
	}}
	require.NoError(t, store.ApplyReconciliationTx(ctx, batch, corrected))

	assert.Equal(t, tables.ReconciliationStatusApplied, reconBatchStatus(t, store, "recon-tx-ok"))
	assert.EqualValues(t, 1, countDatasheetRows(t, store))

	rows, err := store.GetModelPrices(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].InputCostPerToken)
	assert.InDelta(t, 0.0000022, *rows[0].InputCostPerToken, 1e-12, "the corrected rate must be durable")
}

// TestApplyReconciliationTxRollsBackOnDatasheetFailure is the H-2 regression
// pin at the store layer. It injects a hard failure on the SECOND datasheet
// upsert and asserts the first one did not survive.
//
// Pre-fix behaviour: apply() wrote rows one at a time with no transaction, so
// row 1 stayed re-priced while the batch remained `matched` — a retry then
// scaled row 1 again and the price book compounded silently.
func TestApplyReconciliationTxRollsBackOnDatasheetFailure(t *testing.T) {
	store := setupReconTxTestStore(t)
	ctx := context.Background()
	batch := seedReconBatch(t, store, "recon-tx-dsfail", tables.ReconciliationStatusMatched)
	seedDatasheetRow(t, store, "gpt-4o", 0.000002, 0.000008)

	// Fail the second INSERT/UPSERT against the datasheet table.
	var calls int
	cleanup := injectCreateFailureOnNth(t, store, "governance_model_pricing", "fail_pricing_2nd", 2, &calls)
	defer cleanup()

	in1, in2 := 0.0000022, 0.0000011
	batch.Status = tables.ReconciliationStatusApplied
	corrected := []tables.TableModelPricing{
		{Provider: "openai", Model: "gpt-4o", Mode: "chat", InputCostPerToken: &in1},
		{Provider: "openai", Model: "gpt-4o-mini", Mode: "chat", InputCostPerToken: &in2},
	}
	err := store.ApplyReconciliationTx(ctx, batch, corrected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected failure on governance_model_pricing",
		"the surfaced error must be the injected one, or the rollback assertions pass vacuously")

	// Load-bearing: the first row's correction must NOT be durable.
	rows, err := store.GetModelPrices(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1, "the second row must not exist after rollback")
	require.NotNil(t, rows[0].InputCostPerToken)
	assert.InDelta(t, 0.000002, *rows[0].InputCostPerToken, 1e-12,
		"H-2 BUG: the first datasheet row stayed re-priced after the second write failed; a retry would compound it")

	assert.Equal(t, tables.ReconciliationStatusMatched, reconBatchStatus(t, store, "recon-tx-dsfail"),
		"the batch must stay matched so the operator can retry")
}

// TestApplyReconciliationTxRollsBackOnBatchUpdateFailure covers the more
// damaging pre-fix half: every datasheet write succeeds but the batch status
// flip fails. Without a transaction the whole book is re-priced while the
// batch still reads `matched`, so the `applied` guard offers no protection.
func TestApplyReconciliationTxRollsBackOnBatchUpdateFailure(t *testing.T) {
	store := setupReconTxTestStore(t)
	ctx := context.Background()
	batch := seedReconBatch(t, store, "recon-tx-batchfail", tables.ReconciliationStatusMatched)
	seedDatasheetRow(t, store, "gpt-4o", 0.000002, 0.000008)

	cleanup := injectRawSQLFailureOn(t, store, "UPDATE billing_reconciliations", "fail_batch_update")
	defer cleanup()

	in := 0.0000022
	batch.Status = tables.ReconciliationStatusApplied
	corrected := []tables.TableModelPricing{
		{Provider: "openai", Model: "gpt-4o", Mode: "chat", InputCostPerToken: &in},
	}
	err := store.ApplyReconciliationTx(ctx, batch, corrected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected failure on UPDATE billing_reconciliations")

	// The datasheet must be untouched — this is the assertion that fails
	// without the transaction wrapper.
	rows, err := store.GetModelPrices(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].InputCostPerToken)
	assert.InDelta(t, 0.000002, *rows[0].InputCostPerToken, 1e-12,
		"H-2 BUG: datasheet re-priced while the batch flip failed; a retry compounds with no `applied` guard")

	assert.Equal(t, tables.ReconciliationStatusMatched, reconBatchStatus(t, store, "recon-tx-batchfail"))
}

// TestApplyReconciliationTxRefusesDoubleApply pins the one-way guard, now
// enforced inside the transaction against a freshly-read row rather than
// relying on the caller's possibly-stale pre-check.
func TestApplyReconciliationTxRefusesDoubleApply(t *testing.T) {
	store := setupReconTxTestStore(t)
	ctx := context.Background()
	seedReconBatch(t, store, "recon-tx-twice", tables.ReconciliationStatusApplied)
	seedDatasheetRow(t, store, "gpt-4o", 0.000002, 0.000008)

	in := 0.0000022
	corrected := []tables.TableModelPricing{
		{Provider: "openai", Model: "gpt-4o", Mode: "chat", InputCostPerToken: &in},
	}
	batch := &tables.TableBillingReconciliation{ID: "recon-tx-twice", Status: tables.ReconciliationStatusApplied}
	err := store.ApplyReconciliationTx(ctx, batch, corrected)
	require.ErrorIs(t, err, ErrReconciliationAlreadyApplied)

	rows, err := store.GetModelPrices(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].InputCostPerToken)
	assert.InDelta(t, 0.000002, *rows[0].InputCostPerToken, 1e-12,
		"a refused apply must not touch the price book")
}

// TestApplyReconciliationTxUnknownBatch confirms the not-found path so the
// handler can distinguish a deleted batch from a real failure.
func TestApplyReconciliationTxUnknownBatch(t *testing.T) {
	store := setupReconTxTestStore(t)
	ctx := context.Background()
	err := store.ApplyReconciliationTx(ctx, &tables.TableBillingReconciliation{
		ID:     "recon-never-existed",
		Status: tables.ReconciliationStatusApplied,
	}, nil)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestApplyReconciliationTxValidatesInput covers the cheap guard rails.
func TestApplyReconciliationTxValidatesInput(t *testing.T) {
	store := setupReconTxTestStore(t)
	ctx := context.Background()

	require.Error(t, store.ApplyReconciliationTx(ctx, nil, nil))
	require.Error(t, store.ApplyReconciliationTx(ctx, &tables.TableBillingReconciliation{ID: "  "}, nil))
}

// TestApplyReconciliationTxNoRowsStillFlipsStatus confirms that a batch whose
// correction plan matched no datasheet row still records as applied — the
// operator's intent was to close the batch, and leaving it `matched` would
// invite a repeat apply.
func TestApplyReconciliationTxNoRowsStillFlipsStatus(t *testing.T) {
	store := setupReconTxTestStore(t)
	ctx := context.Background()
	seedReconBatch(t, store, "recon-tx-empty", tables.ReconciliationStatusMatched)

	batch := &tables.TableBillingReconciliation{ID: "recon-tx-empty", Status: tables.ReconciliationStatusApplied}
	require.NoError(t, store.ApplyReconciliationTx(ctx, batch, nil))
	assert.Equal(t, tables.ReconciliationStatusApplied, reconBatchStatus(t, store, "recon-tx-empty"))
	assert.EqualValues(t, 0, countDatasheetRows(t, store))
}

// injectRawSQLFailureOn aborts any raw `Exec` whose SQL contains the given
// fragment. ApplyReconciliationTx writes the batch status with a raw UPDATE
// (mirroring UpdateReconciliation), which bypasses GORM's update processor —
// so the failure has to be injected on the "raw" callback instead.
func injectRawSQLFailureOn(t *testing.T, store *RDBConfigStore, sqlFragment, callbackName string) func() {
	t.Helper()
	cb := store.DB().Callback().Raw()
	require.NoError(t, cb.Before("gorm:raw").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil {
			return
		}
		if strings.Contains(tx.Statement.SQL.String(), sqlFragment) {
			_ = tx.AddError(errors.New("injected failure on " + sqlFragment))
		}
	}))
	return func() { _ = cb.Remove(callbackName) }
}

// injectCreateFailureOnNth aborts the Nth INSERT against the named table.
// counter is incremented on every matching attempt so the injection is
// precisely positioned; a plain "fail all" would not distinguish the
// partial-write case this test exists to pin.
func injectCreateFailureOnNth(t *testing.T, store *RDBConfigStore, table, callbackName string, nth int, counter *int) func() {
	t.Helper()
	cb := store.DB().Callback().Create()
	require.NoError(t, cb.Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Table != table {
			return
		}
		*counter++
		if *counter == nth {
			_ = tx.AddError(errors.New("injected failure on " + table))
		}
	}))
	return func() { _ = cb.Remove(callbackName) }
}
