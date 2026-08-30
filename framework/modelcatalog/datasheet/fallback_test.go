package datasheet

import (
	"context"
	"errors"
	"testing"
	"time"

	bifrost "github.com/pin-gou/celer-route/core"
	"github.com/pin-gou/celer-route/core/schemas"
)

// TestBundledPricingLoads verifies the embedded pricing datasheet parses into
// valid entries. This pins the fallback copy so a build never ships a bundled
// file that fails to load at runtime.
func TestBundledPricingLoads(t *testing.T) {
	data, err := loadBundledPricing()
	if err != nil {
		t.Fatalf("loadBundledPricing: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("bundled pricing datasheet is empty")
	}
	// gpt-4o is a well-known entry; assert it parsed so the bundle isn't a
	// truncated/stale snapshot.
	if _, ok := data["gpt-4o"]; !ok {
		t.Fatalf("bundled pricing datasheet missing gpt-4o (got %d entries)", len(data))
	}
}

// TestBundledModelParamsGzipRoundtrip verifies the embedded model-parameters
// datasheet decompresses from gzip and parses into valid entries.
func TestBundledModelParamsGzipRoundtrip(t *testing.T) {
	data, err := loadBundledModelParams()
	if err != nil {
		t.Fatalf("loadBundledModelParams: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("bundled model parameters datasheet is empty")
	}
	if _, ok := data["gpt-4o"]; !ok {
		t.Fatalf("bundled model parameters datasheet missing gpt-4o (got %d entries)", len(data))
	}
}

// TestShouldFallbackToBundled covers the 4-way decision matrix: fallback only
// for the default URL and only when no existing data can serve instead.
func TestShouldFallbackToBundled(t *testing.T) {
	cases := []struct {
		name            string
		rawURL          string
		defaultURL      string
		hasExistingData bool
		want            bool
	}{
		{"default URL, no existing data -> fallback", DefaultURL, DefaultURL, false, true},
		{"default URL, existing data -> keep existing", DefaultURL, DefaultURL, true, false},
		{"custom URL, no existing data -> no fallback", "https://custom.example/datasheet", DefaultURL, false, false},
		{"custom URL, existing data -> no fallback", "https://custom.example/datasheet", DefaultURL, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFallbackToBundled(tc.rawURL, tc.defaultURL, tc.hasExistingData); got != tc.want {
				t.Fatalf("shouldFallbackToBundled(%q, %q, %v) = %v, want %v",
					tc.rawURL, tc.defaultURL, tc.hasExistingData, got, tc.want)
			}
		})
	}
}

// TestLoadFromURLIntoMemoryFallsBackToBundled simulates a download failure
// (via the fetchURL seam) at the default pricing URL with no config store, and
// asserts the store boots from the bundled datasheet instead of erroring.
func TestLoadFromURLIntoMemoryFallsBackToBundled(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	store := New(nil, logger, Config{
		URL:                DefaultURL,
		ModelParametersURL: DefaultModelParametersURL,
	})
	store.fetchURL = func(ctx context.Context, rawURL string) ([]byte, error) {
		return nil, errors.New("simulated network failure")
	}

	// A short-lived context makes withRetries bail at the first backoff wait
	// instead of sleeping 1s+2s+4s between retries, keeping the test fast.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := store.LoadFromURLIntoMemory(ctx); err != nil {
		t.Fatalf("LoadFromURLIntoMemory with default URL + download failure should fall back to bundled data, got error: %v", err)
	}
	if len(store.DatasheetProviders()) == 0 {
		t.Fatal("expected pricing data to be loaded from bundled datasheet after fallback")
	}
}

// TestSyncFromURLFallsBackToBundled exercises the configStore==nil path of
// SyncFromURL: a download failure at the default URL must not surface an error
// because the bundled datasheet covers startup.
func TestSyncFromURLFallsBackToBundled(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	store := New(nil, logger, Config{
		URL:                DefaultURL,
		ModelParametersURL: DefaultModelParametersURL,
	})
	store.fetchURL = func(ctx context.Context, rawURL string) ([]byte, error) {
		return nil, errors.New("simulated network failure")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := store.SyncFromURL(ctx); err != nil {
		t.Fatalf("SyncFromURL with default URL + download failure should fall back to bundled data, got error: %v", err)
	}
	if len(store.DatasheetProviders()) == 0 {
		t.Fatal("expected pricing data to be loaded from bundled datasheet after fallback")
	}
}

// TestLoadFromURLIntoMemoryNoFallbackForCustomURL asserts a custom URL failure
// still surfaces as an error — the bundled copy is only a fallback for the
// default URL, never for an operator's own datasheet source.
func TestLoadFromURLIntoMemoryNoFallbackForCustomURL(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	store := New(nil, logger, Config{
		URL:                "https://custom.example/datasheet",
		ModelParametersURL: "https://custom.example/model-parameters",
	})
	store.fetchURL = func(ctx context.Context, rawURL string) ([]byte, error) {
		return nil, errors.New("simulated network failure")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := store.LoadFromURLIntoMemory(ctx); err == nil {
		t.Fatal("expected error when custom pricing URL download fails (bundled fallback must not apply)")
	}
}

// TestSyncModelParamsFromURLFallsBackToBundled exercises the model-parameters
// datasheet fallback (gzip roundtrip through the real load path).
func TestSyncModelParamsFromURLFallsBackToBundled(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelError)
	store := New(nil, logger, Config{
		URL:                DefaultURL,
		ModelParametersURL: DefaultModelParametersURL,
	})
	store.fetchURL = func(ctx context.Context, rawURL string) ([]byte, error) {
		return nil, errors.New("simulated network failure")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := store.SyncModelParamsFromURL(ctx); err != nil {
		t.Fatalf("SyncModelParamsFromURL with default URL + download failure should fall back to bundled data, got error: %v", err)
	}
	if !store.IsRequestTypeSupported("gpt-4o", schemas.ChatCompletionRequest) {
		t.Fatal("expected model parameters to be loaded from bundled datasheet after fallback")
	}
}

// TestDownloadTimeoutsAre5Seconds pins the user-requested download timeout.
func TestDownloadTimeoutsAre5Seconds(t *testing.T) {
	if DefaultPricingTimeout != 5*time.Second {
		t.Fatalf("DefaultPricingTimeout = %v, want 5s", DefaultPricingTimeout)
	}
	if DefaultModelParametersTimeout != 5*time.Second {
		t.Fatalf("DefaultModelParametersTimeout = %v, want 5s", DefaultModelParametersTimeout)
	}
}
