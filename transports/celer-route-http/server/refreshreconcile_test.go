package server

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	bifrost "github.com/pin-gou/celer-route/core"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/modelcatalog"
	"github.com/pin-gou/celer-route/framework/modelcatalog/datasheet"
	"github.com/pin-gou/celer-route/framework/modelcatalog/live"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
)

// refreshReconcileTestConfig builds a lib.Config backed by a real SQLite config
// store and a ModelCatalog loaded from it, seeded with pricing rows for OpenAI:
// one synced model present in the fresh results, one stale synced model, and
// one custom (manually added) model.
func refreshReconcileTestConfig(t *testing.T) *lib.Config {
	t.Helper()
	ctx := context.Background()
	store, err := configstore.NewConfigStore(ctx, &configstore.Config{
		Enabled: true,
		Type:    configstore.ConfigStoreTypeSQLite,
		Config:  &configstore.SQLiteConfig{Path: filepath.Join(t.TempDir(), "reconcile.db")},
	}, bifrost.NewNoOpLogger())
	if err != nil {
		t.Fatalf("create config store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close(ctx) })

	seed := func(model string, isCustom bool) {
		if err := store.UpsertModelPrices(ctx, &configstoreTables.TableModelPricing{
			Model:    model,
			Provider: string(schemas.OpenAI),
			Mode:     "chat",
			IsCustom: isCustom,
		}); err != nil {
			t.Fatalf("seed pricing %s: %v", model, err)
		}
	}
	seed("gpt-4o", false)   // synced and present in the fresh results
	seed("stale-1", false)  // synced but absent from the fresh results
	seed("my-custom", true) // custom — must always survive

	ds := datasheet.New(store, bifrost.NewNoOpLogger(), datasheet.Config{})
	if err := ds.LoadFromDB(ctx); err != nil {
		t.Fatalf("load pricing: %v", err)
	}
	// The canonical datasheet only contains gpt-4o — stale-1 is an orphan and
	// my-custom is a manual registration.
	ds.SetDatasheetKeysForTest(map[string]struct{}{"gpt-4o|openai|chat": {}})
	return &lib.Config{ConfigStore: store, ModelCatalog: modelcatalog.NewTestCatalogWithDatasheet(ds)}
}

func TestReconcileLiveModelsAfterSync_PrunesStaleLiveAndPricing(t *testing.T) {
	SetLogger(noopTestLogger{})
	cfg := refreshReconcileTestConfig(t)
	s := &BifrostHTTPServer{Config: cfg}
	mc := cfg.ModelCatalog

	// A previous pass left two keys cached; this pass only k1 succeeded.
	mc.UpsertLive(schemas.OpenAI, "k1", false, []string{"gpt-4o"})
	mc.UpsertLive(schemas.OpenAI, "k1", true, []string{"gpt-4o"})
	mc.UpsertLive(schemas.OpenAI, "k2", true, []string{"stale-live"})

	results := newLiveRefreshResults()
	results.record("k1", false, nil)
	results.record("k1", true, []string{"gpt-4o"})

	s.reconcileLiveModelsAfterSync(context.Background(), schemas.OpenAI, results)

	// Union (live + datasheet view): stale-live and stale-1 gone; gpt-4o and
	// my-custom survive.
	got := mc.GetUnfilteredModelsForProvider(schemas.OpenAI)
	want := []string{"gpt-4o", "my-custom"}
	if !slices.Equal(got, want) {
		t.Fatalf("unfiltered union after reconcile = %v, want %v", got, want)
	}

	// Filtered live is pruned to the fresh key's models only. (my-custom is not
	// in the filtered view: it is not served by any live key and the datasheet
	// backfill only re-adds deprecated models.)
	gotFiltered := mc.GetModelsForProvider(schemas.OpenAI)
	if !slices.Equal(gotFiltered, []string{"gpt-4o"}) {
		t.Fatalf("filtered live after reconcile = %v, want [gpt-4o]", gotFiltered)
	}

	// Pricing rows: stale-1 deleted from the DB, custom + latest kept.
	rows, err := cfg.ConfigStore.GetModelPrices(context.Background())
	if err != nil {
		t.Fatalf("get model prices: %v", err)
	}
	names := make(map[string]bool, len(rows))
	for _, r := range rows {
		names[r.Model] = true
	}
	if names["stale-1"] {
		t.Fatalf("stale-1 pricing row should have been deleted, rows=%v", names)
	}
	if !names["gpt-4o"] || !names["my-custom"] {
		t.Fatalf("gpt-4o and my-custom pricing rows must survive, rows=%v", names)
	}
}

func TestReconcileLiveModelsAfterSync_FailedPassKeepsDatasheetAndCustom(t *testing.T) {
	SetLogger(noopTestLogger{})
	cfg := refreshReconcileTestConfig(t)
	s := &BifrostHTTPServer{Config: cfg}
	mc := cfg.ModelCatalog

	mc.UpsertLive(schemas.OpenAI, "k1", false, []string{"gpt-4o"})
	mc.UpsertLive(schemas.OpenAI, "k1", true, []string{"gpt-4o"})
	mc.UpsertLive(schemas.OpenAI, "k2", true, []string{"stale-live"})

	// A fully-failed pass records no unfiltered success → live is untouched
	// (last-known-good), but the orphan cleanup still removes rows the
	// datasheet never contained.
	results := newLiveRefreshResults()
	results.record("k1", false, nil) // only the filtered fetch succeeded

	s.reconcileLiveModelsAfterSync(context.Background(), schemas.OpenAI, results)

	got := mc.GetUnfilteredModelsForProvider(schemas.OpenAI)
	// gpt-4o (datasheet), my-custom (manual) survive; stale-1 (orphan) is
	// pruned; stale-live stays because no fresh unfiltered result authorized
	// the live prune.
	want := []string{"gpt-4o", "my-custom", "stale-live"}
	if !slices.Equal(got, want) {
		t.Fatalf("unfiltered union after failed pass = %v, want %v", got, want)
	}
}

func TestReconcileLiveModelsAfterSync_EmptyLatestKeepsDatasheetAndCustom(t *testing.T) {
	SetLogger(noopTestLogger{})
	cfg := refreshReconcileTestConfig(t)
	s := &BifrostHTTPServer{Config: cfg}
	mc := cfg.ModelCatalog

	mc.UpsertLive(schemas.OpenAI, "k1", false, []string{"gpt-4o"})
	mc.UpsertLive(schemas.OpenAI, "k1", true, []string{"gpt-4o"})

	// The unfiltered fetch "succeeded" but returned no models — treated like a
	// transient failure for the live layer, while the orphan cleanup still runs.
	results := newLiveRefreshResults()
	results.record("k1", true, nil)

	s.reconcileLiveModelsAfterSync(context.Background(), schemas.OpenAI, results)

	got := mc.GetUnfilteredModelsForProvider(schemas.OpenAI)
	want := []string{"gpt-4o", "my-custom"}
	if !slices.Equal(got, want) {
		t.Fatalf("unfiltered union after empty latest = %v, want %v", got, want)
	}
}

func TestLiveRefreshResults_RetainedFillsProvider(t *testing.T) {
	r := newLiveRefreshResults()
	r.record("k1", true, []string{"gpt-4o", "o1"})
	r.record("k2", false, nil)

	if !r.anyUnfiltered() {
		t.Fatal("anyUnfiltered must be true when one unfiltered fetch succeeded")
	}
	if got := r.latestModels(); !slices.Equal(got, []string{"gpt-4o", "o1"}) {
		t.Fatalf("latestModels = %v, want [gpt-4o o1]", got)
	}

	retained := r.retained(schemas.OpenAI)
	if _, ok := retained[live.Key{Provider: schemas.OpenAI, KeyID: "k1", Unfiltered: true}]; !ok {
		t.Fatal("retained must include (k1, unfiltered) with provider filled")
	}
	if _, ok := retained[live.Key{Provider: schemas.OpenAI, KeyID: "k2", Unfiltered: false}]; !ok {
		t.Fatal("retained must include (k2, filtered) with provider filled")
	}
	if len(retained) != 2 {
		t.Fatalf("retained has %d entries, want 2", len(retained))
	}
}
