package configstore

import (
	"context"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/require"
)

func newTestCacheEntry(modelsJSON, keyStatusesJSON string) *tables.TableModelListCache {
	return &tables.TableModelListCache{
		Provider:        tables.ModelListCacheAll,
		ModelsJSON:      modelsJSON,
		KeyStatusesJSON: keyStatusesJSON,
		UpdatedAt:       time.Now(),
	}
}

func TestModelListCache_CrudRoundTrip(t *testing.T) {
	store := setupRDBTestStore(t)
	ctx := context.Background()

	// Initially a cache miss.
	got, err := store.GetCachedModelList(ctx, tables.ModelListCacheAll)
	require.NoError(t, err)
	require.Nil(t, got)

	modelsJSON := `[{"id":"openai/gpt-5-1","owned_by":"openai"}]`
	keyStatusesJSON := `[{"key_id":"k-1","status":"success","provider":"openai"}]`
	require.NoError(t, store.UpsertCachedModelList(ctx, newTestCacheEntry(modelsJSON, keyStatusesJSON)))

	got, err = store.GetCachedModelList(ctx, tables.ModelListCacheAll)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, modelsJSON, got.ModelsJSON)
	require.Equal(t, keyStatusesJSON, got.KeyStatusesJSON)

	// Upsert overwrites.
	modelsJSON2 := `[{"id":"openai/gpt-5-2"}]`
	require.NoError(t, store.UpsertCachedModelList(ctx, newTestCacheEntry(modelsJSON2, "")))
	got, err = store.GetCachedModelList(ctx, tables.ModelListCacheAll)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, modelsJSON2, got.ModelsJSON)
	require.Equal(t, "", got.KeyStatusesJSON)

	// Delete clears it → cache miss again.
	require.NoError(t, store.DeleteCachedModelList(ctx, tables.ModelListCacheAll))
	got, err = store.GetCachedModelList(ctx, tables.ModelListCacheAll)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestModelListCache_UpsertDefaultsAggregateProvider(t *testing.T) {
	store := setupRDBTestStore(t)
	ctx := context.Background()

	entry := newTestCacheEntry(`[{"id":"m"}]`, "")
	entry.Provider = "" // should default to the aggregate sentinel
	require.NoError(t, store.UpsertCachedModelList(ctx, entry))
	require.Equal(t, tables.ModelListCacheAll, entry.Provider)

	got, err := store.GetCachedModelList(ctx, tables.ModelListCacheAll)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestModelListCache_InvalidatedOnProviderWrite(t *testing.T) {
	store := setupRDBTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.UpsertCachedModelList(ctx, newTestCacheEntry(`[{"id":"m"}]`, "")))

	// Adding a provider invalidates the cache.
	require.NoError(t, store.AddProvider(ctx, schemas.OpenAI, ProviderConfig{NetworkConfig: &schemas.NetworkConfig{BaseURL: "https://api.openai.com"}}))
	got, err := store.GetCachedModelList(ctx, tables.ModelListCacheAll)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestModelListCache_InvalidatedOnKeyWrites(t *testing.T) {
	store := setupRDBTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.AddProvider(ctx, schemas.OpenAI, ProviderConfig{NetworkConfig: &schemas.NetworkConfig{BaseURL: "https://api.openai.com"}}))

	seed := func() {
		require.NoError(t, store.UpsertCachedModelList(ctx, newTestCacheEntry(`[{"id":"m"}]`, "")))
	}
	mustBeNil := func(stage string) {
		got, err := store.GetCachedModelList(ctx, tables.ModelListCacheAll)
		require.NoError(t, err)
		require.Nil(t, got, "cache should be invalidated after %s", stage)
	}

	// CreateProviderKey
	seed()
	require.NoError(t, store.CreateProviderKey(ctx, schemas.OpenAI, schemas.Key{ID: "k-1", Name: "key-1", Models: schemas.WhiteList{"openai/gpt-5-1"}}))
	mustBeNil("CreateProviderKey")

	// UpdateProviderKey
	require.NoError(t, store.UpdateProviderKey(ctx, schemas.OpenAI, "k-1", schemas.Key{ID: "k-1", Name: "key-1", Models: schemas.WhiteList{"openai/gpt-5-2"}}))
	mustBeNil("UpdateProviderKey")

	// DeleteProviderKey
	seed()
	require.NoError(t, store.DeleteProviderKey(ctx, schemas.OpenAI, "k-1"))
	mustBeNil("DeleteProviderKey")

	// UpdateProvider
	seed()
	require.NoError(t, store.UpdateProvider(ctx, schemas.OpenAI, ProviderConfig{NetworkConfig: &schemas.NetworkConfig{BaseURL: "https://api.openai.com/v2"}}))
	mustBeNil("UpdateProvider")

	// DeleteProvider
	seed()
	require.NoError(t, store.DeleteProvider(ctx, schemas.OpenAI))
	mustBeNil("DeleteProvider")

	// UpdateProvidersConfig
	seed()
	require.NoError(t, store.UpdateProvidersConfig(ctx, map[schemas.ModelProvider]ProviderConfig{
		schemas.Anthropic: {NetworkConfig: &schemas.NetworkConfig{BaseURL: "https://api.anthropic.com"}},
	}))
	mustBeNil("UpdateProvidersConfig")
}

func TestModelListCache_GetWithEmptyProviderDefaultsToAll(t *testing.T) {
	store := setupRDBTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.UpsertCachedModelList(ctx, newTestCacheEntry(`[{"id":"m"}]`, "")))
	got, err := store.GetCachedModelList(ctx, "")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, tables.ModelListCacheAll, got.Provider)
}
