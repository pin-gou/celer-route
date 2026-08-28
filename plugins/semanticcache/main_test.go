package semanticcache

import (
	"context"
	"os"
	"testing"
	"time"

	bifrost "github.com/pin-gou/celer-route/core"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/vectorstore"
)

// TestMain drops the shared test namespace BEFORE the run starts (in case a
// previous run was interrupted and left stale entries) AND once after — both
// matter: tests share one namespace + one cache_key prefix per t.Name(),
// so stale writes from a prior interrupted run would surface as spurious
// cache hits on the first request of the next run.
func TestMain(m *testing.M) {
	dropSharedTestNamespace() // pre-run sweep
	code := m.Run()
	dropSharedTestNamespace() // post-run sweep
	os.Exit(code)
}

// dropSharedTestNamespace removes the shared test namespace from EVERY vector
// store backend the suite exercises - not just Weaviate. Redis, Qdrant, and
// Pinecone are persistent external services, so a deterministic per-t.Name()
// cache_key written by one run is still present on the next run (within TTL)
// and surfaces as a spurious cache hit on the first request. Sweeping all
// backends here is the suite's only cleanup, since clearTestKeysWithStore is a
// no-op. Stores that aren't configured/reachable in this environment are
// skipped silently.
func dropSharedTestNamespace() {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	for _, tc := range getVectorStoreTestCases() {
		storeConfig, ok := storeConfigForType(tc.StoreType)
		if !ok {
			continue
		}
		func() {
			store, err := vectorstore.NewVectorStore(context.Background(), &vectorstore.Config{
				Type:    tc.StoreType,
				Config:  storeConfig,
				Enabled: true,
			}, logger)
			if err != nil {
				return // backend not configured/available in this environment
			}
			defer store.Close(context.Background(), SharedTestNamespace)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = store.DeleteNamespace(ctx, SharedTestNamespace)
		}()
	}
}

// mockStore implements the vectorstore.VectorStore interface with no-op methods
// for unit testing Init validation without requiring a real vector store backend.
type mockStore struct{}

func (m *mockStore) Ping(ctx context.Context) error { return nil }
func (m *mockStore) CreateNamespace(ctx context.Context, namespace string, dimension int, properties map[string]vectorstore.VectorStoreProperties) error { return nil }
func (m *mockStore) DeleteNamespace(ctx context.Context, namespace string) error { return nil }
func (m *mockStore) GetChunk(ctx context.Context, namespace string, id string) (vectorstore.SearchResult, error) { return vectorstore.SearchResult{}, nil }
func (m *mockStore) GetChunks(ctx context.Context, namespace string, ids []string) ([]vectorstore.SearchResult, error) { return nil, nil }
func (m *mockStore) GetAll(ctx context.Context, namespace string, queries []vectorstore.Query, selectFields []string, cursor *string, limit int64) ([]vectorstore.SearchResult, *string, error) { return nil, nil, nil }
func (m *mockStore) GetNearest(ctx context.Context, namespace string, vector []float32, queries []vectorstore.Query, selectFields []string, threshold float64, limit int64) ([]vectorstore.SearchResult, error) { return nil, nil }
func (m *mockStore) RequiresVectors() bool { return false }
func (m *mockStore) Add(ctx context.Context, namespace string, id string, embedding []float32, metadata map[string]interface{}) error { return nil }
func (m *mockStore) Delete(ctx context.Context, namespace string, id string) error { return nil }
func (m *mockStore) DeleteAll(ctx context.Context, namespace string, queries []vectorstore.Query) ([]vectorstore.DeleteResult, error) { return nil, nil }
func (m *mockStore) Close(ctx context.Context, namespace string) error { return nil }

func TestInit_Dimension_Negative(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	store := &mockStore{}
	_, err := Init(context.Background(), &Config{Dimension: -1}, logger, store)
	if err == nil {
		t.Error("expected error for Dimension=-1")
	}
}

func TestInit_Dimension_Zero_WithProvider(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	store := &mockStore{}
	_, err := Init(context.Background(), &Config{Provider: "openai", Dimension: 0}, logger, store)
	if err == nil {
		t.Error("expected error for Dimension=0 with Provider set")
	}
}

func TestInit_Dimension_Positive_WithProvider(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	store := &mockStore{}
	_, err := Init(context.Background(), &Config{Provider: "openai", Dimension: 1536}, logger, store)
	if err != nil {
		t.Errorf("expected no error for Dimension=1536 with Provider, got: %v", err)
	}
}

func TestInit_Dimension_Zero_WithoutProvider(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	store := &mockStore{}
	_, err := Init(context.Background(), &Config{Dimension: 0}, logger, store)
	if err != nil {
		t.Errorf("expected no error for Dimension=0 with no Provider, got: %v", err)
	}
}

func TestInit_NilConfig(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	store := &mockStore{}
	_, err := Init(context.Background(), nil, logger, store)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestInit_NilStore(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	_, err := Init(context.Background(), &Config{Dimension: 1536}, logger, nil)
	if err == nil {
		t.Error("expected error for nil store")
	}
}

func TestUnmarshalJSON_TTL_Negative(t *testing.T) {
	// Negative TTL should produce an error
	var cfg Config
	err := cfg.UnmarshalJSON([]byte(`{"dimension": 1536, "ttl": -300}`))
	if err == nil {
		t.Error("expected error for negative TTL numeric value")
	}
}

func TestUnmarshalJSON_TTL_NegativeString(t *testing.T) {
	var cfg Config
	err := cfg.UnmarshalJSON([]byte(`{"dimension": 1536, "ttl": "-5m"}`))
	if err == nil {
		t.Error("expected error for negative TTL string value")
	}
}

func TestUnmarshalJSON_TTL_Zero(t *testing.T) {
	// TTL=0 is valid (means "use default" in Init)
	var cfg Config
	if err := cfg.UnmarshalJSON([]byte(`{"dimension": 1536, "ttl": 0}`)); err != nil {
		t.Fatalf("unexpected error for TTL=0: %v", err)
	}
	if cfg.TTL != 0 {
		t.Errorf("TTL: expected 0, got %v", cfg.TTL)
	}
}
