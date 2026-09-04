package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

// fakeModelListCacheConfigStore is a ConfigStore stub whose only live methods
// are the cached-model-list read/write path. Everything else panics if reached.
type fakeModelListCacheConfigStore struct {
	configstore.ConfigStore
	entry *configstoreTables.TableModelListCache
	err   error
}

func (f *fakeModelListCacheConfigStore) GetCachedModelList(_ context.Context, _ string) (*configstoreTables.TableModelListCache, error) {
	return f.entry, f.err
}

func (f *fakeModelListCacheConfigStore) UpsertCachedModelList(_ context.Context, entry *configstoreTables.TableModelListCache, _ ...*gorm.DB) error {
	f.entry = entry
	return nil
}

func newListModelsCacheHandler(store configstore.ConfigStore) *CompletionHandler {
	cfg := &lib.Config{ConfigStore: store}
	return &CompletionHandler{config: cfg}
}

func TestTryServeListModelsFromCache_Hit(t *testing.T) {
	h := newListModelsCacheHandler(&fakeModelListCacheConfigStore{
		entry: &configstoreTables.TableModelListCache{
			Provider:   configstoreTables.ModelListCacheAll,
			ModelsJSON: `[{"id":"openai/gpt-5-1"},{"id":"anthropic/claude-sonnet-4"}]`,
		},
	})
	ctx := &fasthttp.RequestCtx{}
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if served := h.tryServeListModelsFromCache(ctx, bifrostCtx, 0, ""); !served {
		t.Fatalf("expected cache hit to be served")
	}
	if got := string(ctx.Response.Header.Peek("Content-Type")); got != "application/json" {
		t.Fatalf("expected JSON response, got content-type %q", got)
	}
	body := string(ctx.Response.Body())
	if !strings.Contains(body, "openai/gpt-5-1") || !strings.Contains(body, "anthropic/claude-sonnet-4") {
		t.Fatalf("expected cached models in response body, got %s", body)
	}
}

func TestTryServeListModelsFromCache_Miss(t *testing.T) {
	h := newListModelsCacheHandler(&fakeModelListCacheConfigStore{})
	ctx := &fasthttp.RequestCtx{}
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if served := h.tryServeListModelsFromCache(ctx, bifrostCtx, 0, ""); served {
		t.Fatalf("expected cache miss to return false so caller fans out")
	}
}

func TestTryServeListModelsFromCache_SkipsVKScoped(t *testing.T) {
	h := newListModelsCacheHandler(&fakeModelListCacheConfigStore{
		entry: &configstoreTables.TableModelListCache{ModelsJSON: `[{"id":"openai/gpt-5-1"}]`},
	})
	ctx := &fasthttp.RequestCtx{}
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	bifrostCtx.SetValue(schemas.BifrostContextKeyAvailableProviders, []schemas.ModelProvider{schemas.OpenAI})

	if served := h.tryServeListModelsFromCache(ctx, bifrostCtx, 0, ""); served {
		t.Fatalf("expected VK-scoped request to skip cache (would leak providers)")
	}
}

func TestTryServeListModelsFromCache_SkipsUnresolvableVKHeader(t *testing.T) {
	h := newListModelsCacheHandler(&fakeModelListCacheConfigStore{
		entry: &configstoreTables.TableModelListCache{ModelsJSON: `[{"id":"openai/gpt-5-1"}]`},
	})
	ctx := &fasthttp.RequestCtx{}
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	// An unresolvable VK (never stored on the context as a resolved row) must
	// still skip the cache — governance returns an empty list for it, so the
	// unscoped aggregate cache would leak providers.
	bifrostCtx.SetValue(schemas.BifrostContextKeyVirtualKey, "sk-bf-does-not-exist")

	if served := h.tryServeListModelsFromCache(ctx, bifrostCtx, 0, ""); served {
		t.Fatalf("expected unresolvable-VK request to skip cache (would leak providers)")
	}
}

func TestTryServeListModelsFromCache_StoreErrorIsMiss(t *testing.T) {
	h := newListModelsCacheHandler(&fakeModelListCacheConfigStore{err: errors.New("db unavailable")})
	ctx := &fasthttp.RequestCtx{}
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if served := h.tryServeListModelsFromCache(ctx, bifrostCtx, 0, ""); served {
		t.Fatalf("expected store error to behave as cache miss")
	}
}

func TestPersistListModelsCache_EmptyDataNotCached(t *testing.T) {
	store := &fakeModelListCacheConfigStore{}
	h := newListModelsCacheHandler(store)
	ctx := &fasthttp.RequestCtx{}

	h.persistListModelsCache(ctx, &schemas.BifrostListModelsResponse{Data: nil})
	if store.entry != nil {
		t.Fatalf("expected empty fan-out result not to be persisted, got %v", store.entry)
	}

	h.persistListModelsCache(ctx, &schemas.BifrostListModelsResponse{Data: []schemas.Model{
		{ID: "openai/gpt-5-1"},
	}})
	if store.entry == nil {
		t.Fatalf("expected non-empty fan-out result to be persisted")
	}
}
