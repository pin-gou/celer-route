package datasheet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/require"
)

// storeWithSQLite builds a Store backed by a real SQLite config store so the
// Delete/Rename reload paths (which read from the DB) work end-to-end.
func storeWithSQLite(t *testing.T) *Store {
	t.Helper()
	store, err := configstore.NewConfigStore(context.Background(), &configstore.Config{
		Enabled: true,
		Type:    configstore.ConfigStoreTypeSQLite,
		Config: &configstore.SQLiteConfig{
			Path: filepath.Join(t.TempDir(), "pricing.db"),
		},
	}, noOpLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	s := New(store, noOpLogger{}, Config{})
	require.NoError(t, s.LoadFromDB(context.Background()))
	return s
}

func seedCustomPricing(t *testing.T, s *Store, model string, provider schemas.ModelProvider, mode string, isCustom bool) {
	t.Helper()
	require.NoError(t, s.configStore.UpsertModelPrices(context.Background(), &configstoreTables.TableModelPricing{
		Model:        model,
		Provider:     string(provider),
		Mode:         mode,
		IsDeprecated: false,
		IsCustom:     isCustom,
	}))
	require.NoError(t, s.LoadFromDB(context.Background()))
}

func TestIsCustomModel_ExactMatchOnly(t *testing.T) {
	s := storeWithSQLite(t)
	seedCustomPricing(t, s, "my-model", schemas.OpenAI, "chat", true)
	seedCustomPricing(t, s, "gpt-4o-2024-08-06", schemas.OpenAI, "chat", false)

	require.True(t, s.IsCustomModel("my-model", schemas.OpenAI), "custom model must report is_custom")
	require.False(t, s.IsCustomModel("gpt-4o-2024-08-06", schemas.OpenAI), "datasheet model must not report is_custom")
	require.False(t, s.IsCustomModel("missing-model", schemas.OpenAI), "unknown model must not report is_custom")
}

func TestDeleteModelPricing_RemovesAndReloads(t *testing.T) {
	s := storeWithSQLite(t)
	seedCustomPricing(t, s, "my-model", schemas.OpenAI, "chat", true)
	seedCustomPricing(t, s, "other-model", schemas.OpenAI, "chat", true)

	require.Contains(t, s.DatasheetModelsForProvider(schemas.OpenAI), "my-model")

	rows, err := s.DeleteModelPricing(context.Background(), "my-model", schemas.OpenAI)
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	models := s.DatasheetModelsForProvider(schemas.OpenAI)
	require.NotContains(t, models, "my-model", "deleted model must leave the datasheet view")
	require.Contains(t, models, "other-model")
	require.False(t, s.IsCustomModel("my-model", schemas.OpenAI))
}

func TestDeleteModelPricing_UnknownModelReturnsZero(t *testing.T) {
	s := storeWithSQLite(t)
	rows, err := s.DeleteModelPricing(context.Background(), "ghost", schemas.OpenAI)
	require.NoError(t, err)
	require.Equal(t, int64(0), rows)
}

func TestRenameModelPricing_RenamesAndReloads(t *testing.T) {
	s := storeWithSQLite(t)
	seedCustomPricing(t, s, "my-model", schemas.OpenAI, "chat", true)

	rows, err := s.RenameModelPricing(context.Background(), "my-model", schemas.OpenAI, "my-model-v2")
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	models := s.DatasheetModelsForProvider(schemas.OpenAI)
	require.NotContains(t, models, "my-model")
	require.Contains(t, models, "my-model-v2", "renamed model must appear under the new name")
	require.True(t, s.IsCustomModel("my-model-v2", schemas.OpenAI), "custom flag must survive rename")
}

func TestRenameModelPricing_RejectsInvalidTargets(t *testing.T) {
	s := storeWithSQLite(t)
	seedCustomPricing(t, s, "my-model", schemas.OpenAI, "chat", true)
	seedCustomPricing(t, s, "taken-model", schemas.OpenAI, "chat", false)

	_, err := s.RenameModelPricing(context.Background(), "my-model", schemas.OpenAI, "")
	require.Error(t, err, "empty new name must be rejected")

	_, err = s.RenameModelPricing(context.Background(), "my-model", schemas.OpenAI, "my-model")
	require.Error(t, err, "identical new name must be rejected")

	_, err = s.RenameModelPricing(context.Background(), "my-model", schemas.OpenAI, "taken-model")
	require.Error(t, err, "conflicting target must be rejected")
}

func TestRenameModelPricing_UnknownModelReturnsZero(t *testing.T) {
	s := storeWithSQLite(t)
	rows, err := s.RenameModelPricing(context.Background(), "ghost", schemas.OpenAI, "ghost-v2")
	require.NoError(t, err)
	require.Equal(t, int64(0), rows)
}

func TestReconcileProviderPricing_DeletesStaleSyncedKeepsCustomAndLatest(t *testing.T) {
	s := storeWithSQLite(t)
	// Custom row always survives even though it is absent from latest.
	seedCustomPricing(t, s, "my-custom", schemas.OpenAI, "chat", true)
	// Datasheet rows: one present in latest, one stale.
	seedCustomPricing(t, s, "gpt-4o", schemas.OpenAI, "chat", false)
	seedCustomPricing(t, s, "gpt-stale", schemas.OpenAI, "chat", false)
	// Another provider's rows must be untouched.
	seedCustomPricing(t, s, "claude-3", schemas.Anthropic, "chat", false)

	deleted, err := s.ReconcileProviderPricing(context.Background(), schemas.OpenAI, []string{"gpt-4o"})
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted, "only the stale non-custom row must be deleted")

	models := s.DatasheetModelsForProvider(schemas.OpenAI)
	require.Contains(t, models, "my-custom", "custom rows must survive reconcile")
	require.Contains(t, models, "gpt-4o", "latest rows must survive reconcile")
	require.NotContains(t, models, "gpt-stale", "stale synced rows must be deleted")

	models = s.DatasheetModelsForProvider(schemas.Anthropic)
	require.Contains(t, models, "claude-3", "other providers must be untouched")
}

func TestReconcileProviderPricing_NothingToDeleteIsNoop(t *testing.T) {
	s := storeWithSQLite(t)
	seedCustomPricing(t, s, "gpt-4o", schemas.OpenAI, "chat", false)
	seedCustomPricing(t, s, "my-custom", schemas.OpenAI, "chat", true)

	deleted, err := s.ReconcileProviderPricing(context.Background(), schemas.OpenAI, []string{"gpt-4o", "gpt-4o-mini"})
	require.NoError(t, err)
	require.Equal(t, int64(0), deleted)
	require.Contains(t, s.DatasheetModelsForProvider(schemas.OpenAI), "gpt-4o")
	require.Contains(t, s.DatasheetModelsForProvider(schemas.OpenAI), "my-custom")
}

func TestPruneOrphanPricingForProvider_RemovesOnlyNonDatasheetNonCustomRows(t *testing.T) {
	s := storeWithSQLite(t)
	s.SetDatasheetKeysForTest(map[string]struct{}{
		"gpt-4o|openai|chat": {},
	})
	seedCustomPricing(t, s, "gpt-4o", schemas.OpenAI, "chat", false)      // in datasheet → kept
	seedCustomPricing(t, s, "orphan", schemas.OpenAI, "chat", false)      // orphan → pruned
	seedCustomPricing(t, s, "my-custom", schemas.OpenAI, "chat", true)    // custom → kept
	seedCustomPricing(t, s, "claude-3", schemas.Anthropic, "chat", false) // other provider → untouched

	deleted, err := s.PruneOrphanPricingForProvider(context.Background(), schemas.OpenAI)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted, "only the orphan row must be pruned")

	models := s.DatasheetModelsForProvider(schemas.OpenAI)
	require.Contains(t, models, "gpt-4o")
	require.Contains(t, models, "my-custom")
	require.NotContains(t, models, "orphan")
	require.Contains(t, s.DatasheetModelsForProvider(schemas.Anthropic), "claude-3")
}

func TestPruneOrphanPricingForProvider_NoDatasheetMembershipPrunesAllSynced(t *testing.T) {
	s := storeWithSQLite(t)
	seedCustomPricing(t, s, "gpt-4o", schemas.OpenAI, "chat", false)
	seedCustomPricing(t, s, "my-custom", schemas.OpenAI, "chat", true)

	// datasheetKeys empty (e.g. never synced from URL): every non-custom row
	// is an orphan, custom rows survive.
	deleted, err := s.PruneOrphanPricingForProvider(context.Background(), schemas.OpenAI)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	require.NotContains(t, s.DatasheetModelsForProvider(schemas.OpenAI), "gpt-4o")
	require.Contains(t, s.DatasheetModelsForProvider(schemas.OpenAI), "my-custom")
}

func TestSyncFromURL_PrunesOrphanPricingRows(t *testing.T) {
	s := storeWithSQLite(t)
	seedCustomPricing(t, s, "ghost", schemas.OpenAI, "chat", false)    // orphan, not in the datasheet file
	seedCustomPricing(t, s, "my-custom", schemas.OpenAI, "chat", true) // custom survives

	pricingPath := filepath.Join(t.TempDir(), "pricing.json")
	require.NoError(t, os.WriteFile(pricingPath, []byte(`{"gpt-4o": {"provider":"openai","mode":"chat"}}`), 0o600))
	s.UpdateSyncConfig(Config{URL: "file://" + pricingPath})

	require.NoError(t, s.SyncFromURL(context.Background()))

	models := s.DatasheetModelsForProvider(schemas.OpenAI)
	require.Contains(t, models, "gpt-4o", "datasheet row must be present")
	require.Contains(t, models, "my-custom", "custom row must survive")
	require.NotContains(t, models, "ghost", "orphan row must be pruned by the datasheet sync")
}
