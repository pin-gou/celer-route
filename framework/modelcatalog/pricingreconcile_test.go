package modelcatalog

import (
	"context"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

// TestReconcileProviderPricing_SkipsPartialListProviders pins the guard that
// keeps the datasheet authoritative for providers whose /v1/models is a strict
// subset of their callable catalog (Perplexity, Vertex). The guard must return
// without touching the datasheet — NewTestCatalog has no config store, so any
// attempt to delete rows would error.
func TestReconcileProviderPricing_SkipsPartialListProviders(t *testing.T) {
	mc := NewTestCatalog(nil)

	for _, provider := range []schemas.ModelProvider{schemas.Perplexity, schemas.Vertex} {
		deleted, err := mc.ReconcileProviderPricing(context.Background(), provider, []string{"some-model"})
		if err != nil {
			t.Fatalf("ReconcileProviderPricing(%s) returned error: %v", provider, err)
		}
		if deleted != 0 {
			t.Fatalf("ReconcileProviderPricing(%s) deleted %d rows, want 0 (datasheet authoritative)", provider, deleted)
		}
	}
}

// TestReconcileProviderPricing_NonPartialProviderWithNoConfigStoreErrors pins
// that the reconcile actually reaches the datasheet for normal providers: a
// test catalog with no config store must surface the config-store requirement
// rather than silently succeeding.
func TestReconcileProviderPricing_NonPartialProviderWithNoConfigStoreErrors(t *testing.T) {
	mc := NewTestCatalog(nil)

	_, err := mc.ReconcileProviderPricing(context.Background(), schemas.OpenAI, []string{"gpt-4o"})
	if err == nil {
		t.Fatal("expected error for provider with no config store, got nil")
	}
}
