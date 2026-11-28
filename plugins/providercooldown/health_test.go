package providercooldown

import (
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/keyselectors"
	"github.com/pin-gou/celer-route/core/schemas"
)

// healthOnPolicy returns a policy whose health section is enabled with
// explicit, easy-to-reason-about parameters.
func healthOnPolicy() *schemas.CooldownPolicy {
	return &schemas.CooldownPolicy{
		Health: &schemas.CooldownHealthConfig{
			Enabled:         true,
			HalfLifeSeconds: 120,
			MinSamples:      3,
			MinWeightFactor: 0.05,
		},
	}
}

// newHealthTestState builds a state with the fake clock installed and health
// enabled as the plugin-level default.
func newHealthTestState(clk *fakeClock) *CooldownState {
	s := NewCooldownState(time.Minute)
	s.now = clk.Now
	s.enableHealth(&HealthConfig{
		Enabled:         true,
		HalfLifeSeconds: 120,
		MinSamples:      3,
		MinWeightFactor: 0.05,
	})
	return s
}

// observeN feeds n identical outcomes for the same (provider, key, model).
func observeN(s *CooldownState, policy *schemas.CooldownPolicy, provider schemas.ModelProvider, keyID, model string, success bool, n int) {
	for i := 0; i < n; i++ {
		s.observe(policy, provider, keyID, model, success)
	}
}

// filterWeights runs AsFilter and returns the weights of the returned keys,
// keyed by key ID.
func filterWeights(t *testing.T, s *CooldownState, ctx *schemas.BifrostContext, provider schemas.ModelProvider, model string, keys []schemas.Key) map[string]float64 {
	t.Helper()
	out, err := s.AsFilter(nil)(ctx, provider, model, keys)
	if err != nil {
		t.Fatalf("AsFilter returned an error: %v", err)
	}
	weights := make(map[string]float64, len(out))
	for _, k := range out {
		weights[k.ID] = k.Weight
	}
	return weights
}

// TestHealthWeightedSelection pins the core behaviour: a healthy key keeps
// (close to) its configured weight, a chronically failing key is scaled down,
// and its weight never reaches zero.
func TestHealthWeightedSelection(t *testing.T) {
	clk := newFakeClock()
	s := newHealthTestState(clk)
	pol := healthOnPolicy()

	observeN(s, pol, schemas.OpenAI, "good", "gpt-4o", true, 10)
	// Spread the failures over time so the EWMA actually moves toward zero.
	for i := 0; i < 10; i++ {
		s.observe(pol, schemas.OpenAI, "bad", "gpt-4o", false)
		clk.Advance(30 * time.Second)
	}

	keys := []schemas.Key{
		{ID: "good", Weight: 1},
		{ID: "bad", Weight: 1},
	}
	weights := filterWeights(t, s, nil, schemas.OpenAI, "gpt-4o", keys)

	good, bad := weights["good"], weights["bad"]
	if good <= 0 || bad <= 0 {
		t.Fatalf("weights must stay positive: good=%v bad=%v", good, bad)
	}
	if math.Abs(good-1) > 1e-9 {
		t.Errorf("healthy key weight = %v, want 1 (unchanged, ewma is at its ceiling)", good)
	}
	if bad < 0.05-1e-9 {
		t.Errorf("unhealthy key weight = %v, want >= min_weight_factor 0.05", bad)
	}
	if bad > 0.1 {
		t.Errorf("unhealthy key weight = %v, want clearly below the healthy key", bad)
	}
	if good <= bad*5 {
		t.Errorf("healthy weight %v must dominate unhealthy weight %v", good, bad)
	}

	// The scaled weights must actually steer the existing selector: the good
	// key wins the overwhelming majority of draws, but the bad key is still
	// selected sometimes (exploration is never zero).
	selected := map[string]int{}
	for i := 0; i < 20000; i++ {
		k, err := keyselectors.WeightedRandom(nil, []schemas.Key{{ID: "good", Weight: good}, {ID: "bad", Weight: bad}}, schemas.OpenAI, "gpt-4o")
		if err != nil {
			t.Fatalf("WeightedRandom: %v", err)
		}
		selected[k.ID]++
	}
	if selected["bad"] == 0 {
		t.Fatal("the unhealthy key must still be selected occasionally (non-zero exploration)")
	}
	if selected["good"] <= selected["bad"] {
		t.Fatalf("healthy key selected %d times, unhealthy %d; want the healthy key to dominate", selected["good"], selected["bad"])
	}
}

// TestHealthDisabledIsByteIdentical pins the rollback guarantee: with health
// off the filter returns the caller's keys exactly as they were.
func TestHealthDisabledIsByteIdentical(t *testing.T) {
	keys := []schemas.Key{
		{ID: "a", Weight: 1},
		{ID: "b", Weight: 2.5},
		{ID: "c", Weight: 0.37},
	}
	orig := append([]schemas.Key(nil), keys...)

	t.Run("plugin default off", func(t *testing.T) {
		clk := newFakeClock()
		s := NewCooldownState(time.Minute)
		s.now = clk.Now
		observeN(s, nil, schemas.OpenAI, "a", "gpt-4o", false, 10)

		weights := filterWeights(t, s, nil, schemas.OpenAI, "gpt-4o", keys)
		for _, k := range orig {
			if weights[k.ID] != k.Weight {
				t.Fatalf("key %s weight = %v, want the unchanged %v", k.ID, weights[k.ID], k.Weight)
			}
		}
	})

	t.Run("provider override disables the plugin default", func(t *testing.T) {
		clk := newFakeClock()
		s := newHealthTestState(clk)
		pol := &schemas.CooldownPolicy{
			Health: &schemas.CooldownHealthConfig{Enabled: false},
		}
		ctx := policyCtx(pol)
		observeN(s, pol, schemas.OpenAI, "a", "gpt-4o", false, 10)

		weights := filterWeights(t, s, ctx, schemas.OpenAI, "gpt-4o", keys)
		for _, k := range orig {
			if weights[k.ID] != k.Weight {
				t.Fatalf("key %s weight = %v, want the unchanged %v", k.ID, weights[k.ID], k.Weight)
			}
		}
	})

	// The caller's slice must never be mutated.
	for i, k := range keys {
		if !reflect.DeepEqual(k, orig[i]) {
			t.Fatalf("input slice entry %d mutated: %+v → %+v", i, orig[i], k)
		}
	}
}

// TestHealthMinSamplesGatesScaling pins that sparse evidence leaves the weight
// alone: a key with fewer than min_samples observations keeps its weight.
func TestHealthMinSamplesGatesScaling(t *testing.T) {
	clk := newFakeClock()
	s := newHealthTestState(clk) // min_samples = 3
	pol := healthOnPolicy()

	observeN(s, pol, schemas.OpenAI, "sparse", "gpt-4o", false, 2)
	weights := filterWeights(t, s, nil, schemas.OpenAI, "gpt-4o", []schemas.Key{{ID: "sparse", Weight: 1}})
	if weights["sparse"] != 1 {
		t.Fatalf("weight with 2 < min_samples=3 samples = %v, want the unchanged 1", weights["sparse"])
	}

	// The third sample crosses the threshold and scaling engages.
	observeN(s, pol, schemas.OpenAI, "sparse", "gpt-4o", false, 1)
	weights = filterWeights(t, s, nil, schemas.OpenAI, "gpt-4o", []schemas.Key{{ID: "sparse", Weight: 1}})
	if weights["sparse"] >= 1 {
		t.Fatalf("weight after crossing min_samples = %v, want a scaled-down value", weights["sparse"])
	}
}

// TestHealthProviderSectionReplacesDefault pins whole-section replacement: a
// provider's own disabled health section turns the feature off for it only,
// while a provider without a section keeps inheriting the plugin default.
func TestHealthProviderSectionReplacesDefault(t *testing.T) {
	clk := newFakeClock()
	s := newHealthTestState(clk)

	disabled := &schemas.CooldownPolicy{
		Health: &schemas.CooldownHealthConfig{Enabled: false, MinSamples: 3, MinWeightFactor: 0.05},
	}
	for i := 0; i < 10; i++ {
		s.observe(disabled, schemas.OpenAI, "k", "gpt-4o", false)
		clk.Advance(30 * time.Second)
	}
	openaiWeights := filterWeights(t, s, policyCtx(disabled), schemas.OpenAI, "gpt-4o", []schemas.Key{{ID: "k", Weight: 1}})
	if openaiWeights["k"] != 1 {
		t.Fatalf("provider with health.enabled=false got weight %v, want the unchanged 1", openaiWeights["k"])
	}

	for i := 0; i < 10; i++ {
		s.observe(nil, schemas.Anthropic, "k", "claude-sonnet-4", false)
		clk.Advance(30 * time.Second)
	}
	anthropicWeights := filterWeights(t, s, nil, schemas.Anthropic, "claude-sonnet-4", []schemas.Key{{ID: "k", Weight: 1}})
	if anthropicWeights["k"] >= 1 {
		t.Fatalf("provider without a health section got weight %v, want the inherited default to scale it down", anthropicWeights["k"])
	}
}

// TestHealthEWMAHalfLife pins the decay formula: after one half-life the
// distance to the target is halved.
func TestHealthEWMAHalfLife(t *testing.T) {
	clk := newFakeClock()
	s := newHealthTestState(clk)
	pol := healthOnPolicy()

	// Seed at 0 with a failure, then let a full half-life elapse.
	s.observe(pol, schemas.OpenAI, "k", "gpt-4o", false)
	clk.Advance(120 * time.Second) // == half_life_seconds

	// alpha = 1 - 2^(-1) = 0.5 → ewma = 0 + 0.5*(1-0) = 0.5.
	s.observe(pol, schemas.OpenAI, "k", "gpt-4o", true)
	row, ok := s.healthRowFor(schemas.OpenAI, "k", "gpt-4o")
	if !ok {
		t.Fatal("expected a health row")
	}
	if math.Abs(row.ewma-0.5) > 1e-9 {
		t.Fatalf("ewma after one half-life = %v, want 0.5", row.ewma)
	}
	if row.samples != 2 {
		t.Fatalf("samples = %d, want 2", row.samples)
	}

	// Another half-life toward 0: alpha = 0.5 → ewma = 0.5 + 0.5*(0-0.5) = 0.25.
	clk.Advance(120 * time.Second)
	s.observe(pol, schemas.OpenAI, "k", "gpt-4o", false)
	row, _ = s.healthRowFor(schemas.OpenAI, "k", "gpt-4o")
	if math.Abs(row.ewma-0.25) > 1e-9 {
		t.Fatalf("ewma after a second half-life = %v, want 0.25", row.ewma)
	}

	// A long idle period below the GC retention must still snap back on the
	// next observation: dt >> half-life makes alpha ≈ 1.
	clk.Advance(time.Hour)
	s.observe(pol, schemas.OpenAI, "k", "gpt-4o", true)
	row, _ = s.healthRowFor(schemas.OpenAI, "k", "gpt-4o")
	if row.ewma < 0.999 {
		t.Fatalf("ewma after a long idle then success = %v, want it to snap back to ~1", row.ewma)
	}
}

// TestHealthGCCleanup pins the bounded-memory guarantee: stale rows are
// dropped on the GC tick while an actively observed row survives.
func TestHealthGCCleanup(t *testing.T) {
	clk := newFakeClock()
	s := newHealthTestState(clk)
	pol := healthOnPolicy()

	s.observe(pol, schemas.OpenAI, "stale", "gpt-4o", false)
	s.observe(pol, schemas.OpenAI, "fresh", "gpt-4o", true)

	clk.Advance(healthStateRetention + time.Second)
	// Keep the "fresh" row alive just before the sweep.
	s.observe(pol, schemas.OpenAI, "fresh", "gpt-4o", true)

	s.mu.Lock()
	s.cleanupHealthLocked(s.nowTime())
	s.mu.Unlock()

	if _, ok := s.healthRowFor(schemas.OpenAI, "stale", "gpt-4o"); ok {
		t.Error("stale row must be pruned by the GC")
	}
	if _, ok := s.healthRowFor(schemas.OpenAI, "fresh", "gpt-4o"); !ok {
		t.Error("actively observed row must survive the GC")
	}
}

// TestHealthConcurrentObserveAndFilter runs observations and filter reads
// concurrently to pin the locking (run with -race).
func TestHealthConcurrentObserveAndFilter(t *testing.T) {
	s := newHealthTestState(newFakeClock())
	pol := healthOnPolicy()
	keys := []schemas.Key{{ID: "a", Weight: 1}, {ID: "b", Weight: 1}}

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				s.observe(pol, schemas.OpenAI, "a", "gpt-4o", i%2 == 0)
				s.observe(pol, schemas.OpenAI, "b", "gpt-4o", i%3 == 0)
			}
		}(w)
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				if _, err := s.AsFilter(nil)(nil, schemas.OpenAI, "gpt-4o", keys); err != nil {
					t.Errorf("AsFilter: %v", err)
					return
				}
				_ = s.healthSize()
			}
		}()
	}
	wg.Wait()
}

// TestHealthPostLLMHookSuccessFeedsEWMA pins the success-path wiring: a
// successful terminal response raises the serving key's EWMA and sample count.
func TestHealthPostLLMHookSuccessFeedsEWMA(t *testing.T) {
	cfg := &Config{Health: &HealthConfig{Enabled: true, HalfLifeSeconds: 120, MinSamples: 3, MinWeightFactor: 0.05}}
	p := &CooldownPlugin{State: cfg.AsState(nil)}

	ctx := newPerKeyTrailCtx("k1")
	resp := newPerKeyResponse(schemas.OpenAI)
	if _, _, err := p.PostLLMHook(ctx, resp, nil); err != nil {
		t.Fatalf("PostLLMHook: %v", err)
	}

	row, ok := p.State.healthRowFor(schemas.OpenAI, "k1", "gpt-4o")
	if !ok {
		t.Fatal("expected a health row for the successful key")
	}
	if row.samples != 1 || math.Abs(row.ewma-1) > 1e-9 {
		t.Fatalf("health row = %+v, want samples=1 ewma=1", row)
	}
}

// TestHealthMarkerFailureFeedsEWMA pins the failure-path wiring: a
// policy-matched per-key rejection lowers the key's EWMA.
func TestHealthMarkerFailureFeedsEWMA(t *testing.T) {
	cfg := &Config{Health: &HealthConfig{Enabled: true, HalfLifeSeconds: 120, MinSamples: 3, MinWeightFactor: 0.05}}
	p := &CooldownPlugin{State: cfg.AsState(nil)}

	marker := p.State.AsMarker(nil)
	marker(nil, schemas.OpenAI, "k1", "k1", "gpt-4o", learnRateLimitErr())

	row, ok := p.State.healthRowFor(schemas.OpenAI, "k1", "gpt-4o")
	if !ok {
		t.Fatal("expected a health row for the failing key")
	}
	if row.samples != 1 || row.ewma != 0 {
		t.Fatalf("health row = %+v, want samples=1 ewma=0", row)
	}
}

// TestHealthKeylessProviderSentinel pins that the empty-keyID sentinel is a
// legal identity for keyless providers, mirroring the mark path.
func TestHealthKeylessProviderSentinel(t *testing.T) {
	clk := newFakeClock()
	s := newHealthTestState(clk)
	pol := healthOnPolicy()

	s.observe(pol, schemas.Opencode, "", "some-model", false)
	if _, ok := s.healthRowFor(schemas.Opencode, "", "some-model"); !ok {
		t.Fatal("keyless providers must record health against the empty sentinel key")
	}

	// Any other provider must reject the empty keyID.
	s.observe(pol, schemas.OpenAI, "", "gpt-4o", false)
	if _, ok := s.healthRowFor(schemas.OpenAI, "", "gpt-4o"); ok {
		t.Fatal("non-keyless providers must not record health for an empty keyID")
	}
}

// TestHealthZeroFactorIsImpossible pins the hard guardrail: a configured
// min_weight_factor of zero is normalized to the default rather than starving
// the pool.
func TestHealthZeroFactorIsImpossible(t *testing.T) {
	got := (&HealthConfig{Enabled: true, MinWeightFactor: 0}).Normalized()
	if got.MinWeightFactor <= 0 {
		t.Fatalf("min_weight_factor normalized to %v, want a positive default", got.MinWeightFactor)
	}

	clk := newFakeClock()
	s := newHealthTestState(clk)
	pol := &schemas.CooldownPolicy{Health: &schemas.CooldownHealthConfig{Enabled: true, MinSamples: 1, MinWeightFactor: 0}}
	for i := 0; i < 20; i++ {
		s.observe(pol, schemas.OpenAI, "bad", "gpt-4o", false)
		clk.Advance(time.Minute)
	}
	weights := filterWeights(t, s, policyCtx(pol), schemas.OpenAI, "gpt-4o", []schemas.Key{{ID: "bad", Weight: 1}})
	if weights["bad"] <= 0 {
		t.Fatalf("weight = %v, want a strictly positive floor", weights["bad"])
	}
}

// TestHealthParseConfig pins the strict parse + normalization of the plugin
// config's health section: unknown keys are rejected and unset bounds take the
// documented defaults.
func TestHealthParseConfig(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{
		"health": map[string]any{
			"enabled":           true,
			"half_life_seconds": 0,
			"min_samples":       0,
			"min_weight_factor": 0,
		},
	})
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Health == nil || !cfg.Health.Enabled {
		t.Fatalf("health section = %+v, want enabled", cfg.Health)
	}
	if cfg.Health.HalfLifeSeconds != schemas.DefaultHealthHalfLifeSeconds ||
		cfg.Health.MinSamples != schemas.DefaultHealthMinSamples ||
		cfg.Health.MinWeightFactor != schemas.DefaultHealthMinWeightFactor {
		t.Fatalf("health defaults = %+v, want %d/%d/%v",
			cfg.Health, schemas.DefaultHealthHalfLifeSeconds, schemas.DefaultHealthMinSamples, schemas.DefaultHealthMinWeightFactor)
	}

	// A typed value flows through and is clamped into the documented bounds.
	cfg, err = ParseConfig(map[string]any{
		"health": map[string]any{"enabled": true, "half_life_seconds": 5, "min_weight_factor": 2.5},
	})
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Health.HalfLifeSeconds != schemas.MinHealthHalfLifeSeconds {
		t.Errorf("half_life_seconds = %d, want clamped to %d", cfg.Health.HalfLifeSeconds, schemas.MinHealthHalfLifeSeconds)
	}
	if cfg.Health.MinWeightFactor != schemas.MaxHealthMinWeightFactor {
		t.Errorf("min_weight_factor = %v, want clamped to %v", cfg.Health.MinWeightFactor, schemas.MaxHealthMinWeightFactor)
	}

	if _, err := ParseConfig(map[string]any{
		"health": map[string]any{"enabled": true, "unknown": 1},
	}); err == nil {
		t.Fatal("unknown health key must be rejected")
	}
}

// TestHealthKeyCopiesNotMutated guards the "return a copy" contract at the
// struct level: the returned slice must equal the input except for Weight.
func TestHealthKeyCopiesNotMutated(t *testing.T) {
	clk := newFakeClock()
	s := newHealthTestState(clk)
	pol := healthOnPolicy()
	for i := 0; i < 5; i++ {
		s.observe(pol, schemas.OpenAI, "bad", "gpt-4o", false)
		clk.Advance(30 * time.Second)
	}

	in := []schemas.Key{{ID: "bad", Name: "Bad Key", Weight: 1}}
	out, err := s.AsFilter(nil)(nil, schemas.OpenAI, "gpt-4o", in)
	if err != nil {
		t.Fatalf("AsFilter: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d keys, want 1", len(out))
	}
	want := in[0]
	want.Weight = out[0].Weight
	if !reflect.DeepEqual(out[0], want) {
		t.Fatalf("filter changed fields other than Weight: got %+v want %+v", out[0], want)
	}
}
