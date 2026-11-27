package providercooldown

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Chronic long-backoff tests. They are deliberately driven through the public
// state methods (observe / markWithPolicy / PostLLMHook) plus the internal
// isChronicLocked predicate, so the wiring — not just the arithmetic — is
// covered. Every state here runs with health weighting OFF and only
// escalation.chronic ON (unless stated otherwise): observation must be driven by
// either capability, not by health alone.

const chronicTestBaseTTL = 30 * time.Second

// chronicPolicy builds a provider policy with escalation + the given chronic
// block enabled.
func chronicPolicy(chronic *schemas.CooldownChronicConfig) *schemas.CooldownPolicy {
	return &schemas.CooldownPolicy{
		Escalation: &schemas.CooldownEscalationConfig{
			Enabled:       true,
			MaxMultiplier: 4,
			Chronic:       chronic,
		},
	}
}

// newChronicTestState builds a state with the fake clock installed and NO
// plugin-level capability default, so each test's provider policy is the single
// source of truth.
func newChronicTestState(clk *fakeClock) *CooldownState {
	s := NewCooldownState(chronicTestBaseTTL)
	s.now = clk.Now
	return s
}

// chronicSpecOfPolicy resolves the chronic scalars the way the plugin does.
func chronicSpecOfPolicy(t *testing.T, s *CooldownState, policy *schemas.CooldownPolicy) chronicSpec {
	t.Helper()
	spec, ok := s.defaults.chronicSpec(policy)
	if !ok {
		t.Fatalf("chronic spec is not active for policy %+v", policy)
	}
	return spec
}

// chronicNow reports the chronic predicate for (provider, key, model) under the
// state's clock.
func chronicNow(t *testing.T, s *CooldownState, policy *schemas.CooldownPolicy, provider schemas.ModelProvider, keyID, model string) bool {
	t.Helper()
	spec := chronicSpecOfPolicy(t, s, policy)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isChronicLocked(provider, keyID, model, spec, s.nowTime())
}

// observeSteps feeds n identical outcomes, advancing the clock by step between
// them so the EWMA sees realistic spacing.
func observeSteps(s *CooldownState, clk *fakeClock, policy *schemas.CooldownPolicy, provider schemas.ModelProvider, keyID, model string, success bool, n int, step time.Duration) {
	for i := 0; i < n; i++ {
		s.observe(policy, provider, keyID, model, success)
		clk.Advance(step)
	}
}

// chronicSuccessCtx builds a PostLLMHook success context carrying the routing
// info plus the attempt trail — the exact shape both clearEscalationOnSuccess
// and observeSuccess read on the success path.
func chronicSuccessCtx(policy *schemas.CooldownPolicy, keyID string) *schemas.BifrostContext {
	ctx := withPolicy(newPerKeyTrailCtx(keyID), policy)
	ctx.SetValue(schemas.BifrostContextKeyRoutingInfo, schemas.RoutingInfo{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o",
	})
	return ctx
}

// TestChronicPredicateRequiresEvidence pins the statistical trigger: the row
// only becomes chronic once it has min_samples observations AND its EWMA sits
// below the threshold.
func TestChronicPredicateRequiresEvidence(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := chronicPolicy(&schemas.CooldownChronicConfig{
		Enabled:              true,
		SuccessRateThreshold: 0.3,
		MinSamples:           10,
		ChronicMaxTTLSeconds: 600,
	})

	observeSteps(s, clk, policy, schemas.OpenAI, "bad", "gpt-4o", false, 9, 5*time.Second)
	if chronicNow(t, s, policy, schemas.OpenAI, "bad", "gpt-4o") {
		t.Fatal("9 failures (< min_samples=10) must not be chronic")
	}
	// The row must exist even though health weighting is off: chronic relies on
	// the same observation pipeline.
	row, ok := s.healthRowFor(schemas.OpenAI, "bad", "gpt-4o")
	if !ok || row.samples != 9 {
		t.Fatalf("health row = %+v (exists=%t), want 9 samples observed with health weighting off", row, ok)
	}

	s.observe(policy, schemas.OpenAI, "bad", "gpt-4o", false)
	if !chronicNow(t, s, policy, schemas.OpenAI, "bad", "gpt-4o") {
		t.Fatal("10 failures with ewma=0 (threshold 0.3) must be chronic")
	}

	// A healthy key with the same number of samples is not chronic.
	observeSteps(s, clk, policy, schemas.OpenAI, "good", "gpt-4o", true, 10, 5*time.Second)
	if chronicNow(t, s, policy, schemas.OpenAI, "good", "gpt-4o") {
		t.Fatal("10 successes (ewma=1) must not be chronic")
	}
	if row, _ := s.healthRowFor(schemas.OpenAI, "good", "gpt-4o"); row.ewma <= 0.3 {
		t.Fatalf("healthy row ewma = %v, want > threshold 0.3", row.ewma)
	}
}

// TestChronicMarkUsesLongTTL pins the TTL rule: a chronic row's mark uses
// chronic_max_ttl_seconds and never shortens the base/escalated TTL.
func TestChronicMarkUsesLongTTL(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := chronicPolicy(&schemas.CooldownChronicConfig{
		Enabled:              true,
		SuccessRateThreshold: 0.3,
		MinSamples:           3,
		ChronicMaxTTLSeconds: 600,
		JitterRatio:          0,
	})
	observeSteps(s, clk, policy, schemas.OpenAI, "bad", "gpt-4o", false, 3, time.Second)

	s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel); got != 600*time.Second {
		t.Fatalf("chronic mark TTL = %v, want the exact chronic_max_ttl 600s with jitter_ratio=0", got)
	}

	// A base TTL longer than the chronic maximum is never shortened.
	long := 2 * time.Hour
	s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", long, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel); got != long {
		t.Fatalf("chronic mark TTL = %v, want the longer base %v (chronic only lengthens)", got, long)
	}

	// A healthy key keeps the plain escalation result.
	observeSteps(s, clk, policy, schemas.OpenAI, "good", "gpt-4o", true, 3, time.Second)
	s.markWithPolicy(policy, schemas.OpenAI, "good", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "good", "gpt-4o", schemas.CooldownScopeModel); got != chronicTestBaseTTL {
		t.Fatalf("healthy mark TTL = %v, want the base %v", got, chronicTestBaseTTL)
	}
}

// TestChronicJitter pins the jitter contract: 0 is exact, an injected draw
// produces a deterministic TTL, and the production RNG stays inside the ±ratio
// band.
func TestChronicJitter(t *testing.T) {
	jittered := func(ratio float64) *schemas.CooldownChronicConfig {
		return &schemas.CooldownChronicConfig{
			Enabled:              true,
			SuccessRateThreshold: 0.3,
			MinSamples:           3,
			ChronicMaxTTLSeconds: 1000,
			JitterRatio:          ratio,
		}
	}

	// Injected draw 0.75 → r = 0.5 → 1000s * (1 + 0.2*0.5) = 1100s.
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := chronicPolicy(jittered(0.2))
	observeSteps(s, clk, policy, schemas.OpenAI, "bad", "gpt-4o", false, 3, time.Second)
	s.randFloat = func() float64 { return 0.75 }
	s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel); got != 1100*time.Second {
		t.Fatalf("injected jitter TTL = %v, want exactly 1100s", got)
	}

	// Injected draw 0 → r = -1 → 1000s * (1 - 0.2) = 800s.
	s.randFloat = func() float64 { return 0 }
	s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel); got != 800*time.Second {
		t.Fatalf("lower-bound jitter TTL = %v, want exactly 800s", got)
	}

	// Production RNG (nil randFloat): every draw stays inside the band.
	s.randFloat = nil
	for i := 0; i < 50; i++ {
		s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
		got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel)
		if got < 800*time.Second || got > 1200*time.Second {
			t.Fatalf("random jitter TTL = %v, want within [800s, 1200s]", got)
		}
	}
}

// TestChronicRecoveryHysteresis pins the reset semantics: a single success does
// NOT lift the chronic mark (or clear the escalation counters); only
// recovery_successes consecutive successes do.
func TestChronicRecoveryHysteresis(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := chronicPolicy(&schemas.CooldownChronicConfig{
		Enabled:              true,
		SuccessRateThreshold: 0.3,
		MinSamples:           10,
		ChronicMaxTTLSeconds: 600,
		JitterRatio:          0,
		RecoverySuccesses:    3,
	})
	p := &CooldownPlugin{State: s}
	ctx := chronicSuccessCtx(policy, "bad")
	resp := newPerKeyResponse(schemas.OpenAI)

	observeSteps(s, clk, policy, schemas.OpenAI, "bad", "gpt-4o", false, 10, 5*time.Second)
	// One mark so there is an escalation counter to protect from the success path.
	s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel); got != 600*time.Second {
		t.Fatalf("chronic mark TTL = %v, want 600s", got)
	}

	// Success #1: the EWMA rises and the recovery counter advances, but the
	// chronic latch must survive, and the escalation counter must not be wiped.
	clk.Advance(time.Minute)
	if _, _, err := p.PostLLMHook(ctx, resp, nil); err != nil {
		t.Fatalf("PostLLMHook success: %v", err)
	}
	row, ok := s.healthRowFor(schemas.OpenAI, "bad", "gpt-4o")
	if !ok || !row.chronic || row.consecutiveSuccesses != 1 {
		t.Fatalf("after 1 success: row = %+v (exists=%t), want chronic with 1 consecutive success", row, ok)
	}
	s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel); got != 600*time.Second {
		t.Fatalf("mark after 1 success = %v, want the long 600s TTL", got)
	}

	// Success #2: still chronic.
	clk.Advance(time.Minute)
	if _, _, err := p.PostLLMHook(ctx, resp, nil); err != nil {
		t.Fatalf("PostLLMHook success: %v", err)
	}
	if row, _ := s.healthRowFor(schemas.OpenAI, "bad", "gpt-4o"); !row.chronic || row.consecutiveSuccesses != 2 {
		t.Fatalf("after 2 successes: row = %+v, want chronic with 2 consecutive successes", row)
	}

	// Success #3 reaches recovery_successes: the latch is released AND the
	// escalation counters are cleared by the success path.
	clk.Advance(time.Minute)
	if _, _, err := p.PostLLMHook(ctx, resp, nil); err != nil {
		t.Fatalf("PostLLMHook success: %v", err)
	}
	row, _ = s.healthRowFor(schemas.OpenAI, "bad", "gpt-4o")
	if row.chronic {
		t.Fatalf("after %d consecutive successes: row = %+v, want the chronic latch released", 3, row)
	}
	if chronicNow(t, s, policy, schemas.OpenAI, "bad", "gpt-4o") {
		t.Fatal("recovered key must not be chronic")
	}

	s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel); got != chronicTestBaseTTL {
		t.Fatalf("mark after full recovery = %v, want the base %v (counters cleared, no stale escalation)", got, chronicTestBaseTTL)
	}
}

// TestChronicRecoveryDrill simulates an upstream that comes back: the key is
// chronic, the long backoff expires, the first probe succeeds, and the key is
// fully recovered within 2x chronic_max_ttl of simulated time.
func TestChronicRecoveryDrill(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	const chronicTTL = 600 * time.Second
	policy := chronicPolicy(&schemas.CooldownChronicConfig{
		Enabled:              true,
		SuccessRateThreshold: 0.3,
		MinSamples:           5,
		ChronicMaxTTLSeconds: 600,
		JitterRatio:          0,
		RecoverySuccesses:    3,
	})

	// Build the chronic state through the marker path (mark then observe).
	for i := 0; i < 10; i++ {
		s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
		s.observe(policy, schemas.OpenAI, "bad", "gpt-4o", false)
		clk.Advance(30 * time.Second)
	}
	if !chronicNow(t, s, policy, schemas.OpenAI, "bad", "gpt-4o") {
		t.Fatal("key must be chronic after 10 failures")
	}
	s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel); got != chronicTTL {
		t.Fatalf("chronic mark TTL = %v, want %v", got, chronicTTL)
	}
	start := clk.Now()

	// Half-open probe: the cooldown lapses and the upstream now answers.
	clk.Advance(chronicTTL)
	s.observe(policy, schemas.OpenAI, "bad", "gpt-4o", true)
	if !chronicNow(t, s, policy, schemas.OpenAI, "bad", "gpt-4o") {
		t.Fatal("a single success must not lift the chronic mark")
	}
	// Traffic then flows normally (no cooldown is created on success), so the
	// remaining recovery successes arrive immediately.
	for i := 0; i < 3; i++ {
		s.observe(policy, schemas.OpenAI, "bad", "gpt-4o", true)
		clk.Advance(5 * time.Second)
	}
	if chronicNow(t, s, policy, schemas.OpenAI, "bad", "gpt-4o") {
		t.Fatal("key must be recovered after the successful probe plus recovery_successes")
	}
	if elapsed := clk.Now().Sub(start); elapsed > 2*chronicTTL {
		t.Fatalf("recovery took %v, want <= 2x chronic_max_ttl (%v)", elapsed, 2*chronicTTL)
	}

	s.markWithPolicy(policy, schemas.OpenAI, "bad", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "gpt-4o", schemas.CooldownScopeModel); got >= chronicTTL {
		t.Fatalf("mark after recovery = %v, want the short TTL again", got)
	}
}

// TestChronicEmptyModelAggregates pins the key-scope branch: a mark without a
// model aggregates the key's model rows instead of falling back to "never
// chronic", and a single bad model is enough.
func TestChronicEmptyModelAggregates(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := chronicPolicy(&schemas.CooldownChronicConfig{
		Enabled:              true,
		SuccessRateThreshold: 0.3,
		MinSamples:           5,
		ChronicMaxTTLSeconds: 600,
		JitterRatio:          0,
	})

	// Model-scoped evidence (the marker path always carries the model), then a
	// key-scoped mark with no model.
	observeSteps(s, clk, policy, schemas.OpenAI, "bad", "gpt-4o", false, 5, time.Second)
	s.markWithPolicy(policy, schemas.OpenAI, "bad", "", 0, CooldownKindRateLimit, schemas.CooldownScopeKey)
	if got := recordedTTL(t, s, schemas.OpenAI, "bad", "", schemas.CooldownScopeKey); got != 600*time.Second {
		t.Fatalf("key-scope mark TTL = %v, want the chronic 600s from the model rows", got)
	}

	// A healthy key keeps the base TTL for key-scope marks.
	observeSteps(s, clk, policy, schemas.OpenAI, "good", "gpt-4o", true, 5, time.Second)
	s.markWithPolicy(policy, schemas.OpenAI, "good", "", 0, CooldownKindRateLimit, schemas.CooldownScopeKey)
	if got := recordedTTL(t, s, schemas.OpenAI, "good", "", schemas.CooldownScopeKey); got != chronicTestBaseTTL {
		t.Fatalf("healthy key-scope mark TTL = %v, want the base %v", got, chronicTestBaseTTL)
	}

	// One chronically bad model among a healthy sibling still makes the key
	// chronic at key scope.
	observeSteps(s, clk, policy, schemas.OpenAI, "mixed", "bad-model", false, 5, time.Second)
	observeSteps(s, clk, policy, schemas.OpenAI, "mixed", "good-model", true, 20, time.Second)
	s.markWithPolicy(policy, schemas.OpenAI, "mixed", "", 0, CooldownKindRateLimit, schemas.CooldownScopeKey)
	if got := recordedTTL(t, s, schemas.OpenAI, "mixed", "", schemas.CooldownScopeKey); got != 600*time.Second {
		t.Fatalf("mixed key-scope mark TTL = %v, want the chronic 600s (a bad model must not hide)", got)
	}
}

// TestChronicDisabledIsExactEscalationRegression is the rollback guarantee:
// with escalation.chronic disabled (or absent), the recorded TTLs and the
// success-clears-counters behaviour are EXACTLY the pre-chronic values, even
// while failures are being observed.
func TestChronicDisabledIsExactEscalationRegression(t *testing.T) {
	type scenario struct {
		baseTTL       time.Duration
		maxMultiplier int
		marks         int
	}
	scenarios := []scenario{
		{10 * time.Second, 4, 6},
		{30 * time.Second, 4, 6},
		{30 * time.Second, 1, 4},
		{10 * time.Minute, 16, 6},
		{5 * time.Minute, 2, 5},
	}
	for _, sc := range scenarios {
		baseline := func() []time.Duration {
			clk := newFakeClock()
			s := newTestState(sc.baseTTL, clk, true, sc.maxMultiplier)
			out := make([]time.Duration, 0, sc.marks)
			for i := 0; i < sc.marks; i++ {
				s.markWithPolicy(nil, schemas.OpenAI, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
				out = append(out, recordedTTL(t, s, schemas.OpenAI, "key-1", "gpt-4o", schemas.CooldownScopeModel))
				clk.Advance(5 * time.Second)
			}
			return out
		}()

		clk := newFakeClock()
		s := newTestState(sc.baseTTL, clk, true, sc.maxMultiplier)
		policy := &schemas.CooldownPolicy{
			Escalation: &schemas.CooldownEscalationConfig{
				Enabled:       true,
				MaxMultiplier: sc.maxMultiplier,
				Chronic: &schemas.CooldownChronicConfig{
					Enabled:              false,
					SuccessRateThreshold: 0.3,
					MinSamples:           3,
					ChronicMaxTTLSeconds: 600,
					JitterRatio:          0.5,
					RecoverySuccesses:    3,
				},
			},
		}
		for i, want := range baseline {
			s.markWithPolicy(policy, schemas.OpenAI, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
			got := recordedTTL(t, s, schemas.OpenAI, "key-1", "gpt-4o", schemas.CooldownScopeModel)
			if got != want {
				t.Fatalf("scenario %+v mark %d: TTL = %v, want the pre-chronic %v", sc, i+1, got, want)
			}
			// The marker path also observes the failure; with chronic off it
			// must have no effect on the TTL.
			s.observe(policy, schemas.OpenAI, "key-1", "gpt-4o", false)
			clk.Advance(5 * time.Second)
		}
	}

	// Success-path clearing is unchanged when chronic is disabled: the next
	// mark after a success is back to 1x.
	run := func(policy *schemas.CooldownPolicy) time.Duration {
		clk := newFakeClock()
		s := newTestState(10*time.Second, clk, true, 4)
		p := &CooldownPlugin{State: s}
		for i := 0; i < 2; i++ {
			s.markWithPolicy(policy, schemas.OpenAI, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
			clk.Advance(5 * time.Second)
		}
		ctx := chronicSuccessCtx(policy, "key-1")
		if _, _, err := p.PostLLMHook(ctx, newPerKeyResponse(schemas.OpenAI), nil); err != nil {
			t.Fatalf("PostLLMHook success: %v", err)
		}
		s.markWithPolicy(policy, schemas.OpenAI, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
		return recordedTTL(t, s, schemas.OpenAI, "key-1", "gpt-4o", schemas.CooldownScopeModel)
	}
	disabled := run(chronicPolicy(&schemas.CooldownChronicConfig{Enabled: false, MinSamples: 3}))
	if disabled != 10*time.Second {
		t.Fatalf("mark after success with chronic disabled = %v, want the pre-chronic 10s", disabled)
	}
	if absent := run(nil); absent != disabled {
		t.Fatalf("chronic absent gave %v, chronic disabled gave %v; they must be identical", absent, disabled)
	}
}

// TestChronicConcurrentMarkObserveFilter exercises the shared health /
// cooldown tables from several goroutines (run with -race).
func TestChronicConcurrentMarkObserveFilter(t *testing.T) {
	s := newChronicTestState(newFakeClock())
	policy := chronicPolicy(&schemas.CooldownChronicConfig{
		Enabled:              true,
		SuccessRateThreshold: 0.3,
		MinSamples:           2,
		ChronicMaxTTLSeconds: 60,
		JitterRatio:          0.2,
		RecoverySuccesses:    2,
	})
	keys := []schemas.Key{{ID: "a", Weight: 1}, {ID: "b", Weight: 1}}

	var wg sync.WaitGroup
	for w := 0; w < 6; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				s.observe(policy, schemas.OpenAI, keys[i%2].ID, "gpt-4o", i%3 == 0)
				s.markWithPolicy(policy, schemas.OpenAI, keys[i%2].ID, "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
			}
		}(w)
	}
	for r := 0; r < 3; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				if _, err := s.AsFilter(nil)(policyCtx(policy), schemas.OpenAI, "gpt-4o", keys); err != nil {
					t.Errorf("AsFilter: %v", err)
					return
				}
				_ = s.Stats()
			}
		}()
	}
	wg.Wait()
}

// TestChronicParseConfig pins the strict parse, normalization and the
// "chronic requires escalation" rule of the plugin config section.
func TestChronicParseConfig(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{
		"escalation": map[string]any{
			"enabled":        true,
			"max_multiplier": float64(4),
			"chronic": map[string]any{
				"enabled":                 true,
				"success_rate_threshold":  0.0,
				"min_samples":             float64(0),
				"chronic_max_ttl_seconds": float64(0),
				"jitter_ratio":            0.0,
				"recovery_successes":      float64(0),
			},
		},
	})
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	ch := cfg.Escalation.Chronic
	if ch == nil || !ch.Enabled {
		t.Fatalf("escalation.chronic = %+v, want parsed and enabled", ch)
	}
	if ch.SuccessRateThreshold != schemas.DefaultChronicSuccessRateThreshold ||
		ch.MinSamples != schemas.DefaultChronicMinSamples ||
		ch.ChronicMaxTTLSeconds != schemas.DefaultChronicMaxTTLSeconds ||
		ch.RecoverySuccesses != schemas.DefaultChronicRecoverySuccesses {
		t.Fatalf("chronic defaults = %+v, want %v/%d/%d/%d", ch,
			schemas.DefaultChronicSuccessRateThreshold, schemas.DefaultChronicMinSamples,
			schemas.DefaultChronicMaxTTLSeconds, schemas.DefaultChronicRecoverySuccesses)
	}
	// 0 is meaningful for jitter ("no jitter") and must survive normalization.
	if ch.JitterRatio != 0 {
		t.Fatalf("jitter_ratio = %v, want 0 preserved", ch.JitterRatio)
	}

	// Out-of-range values are clamped.
	cfg, err = ParseConfig(map[string]any{
		"escalation": map[string]any{
			"enabled": true,
			"chronic": map[string]any{
				"enabled":                 true,
				"success_rate_threshold":  float64(2),
				"min_samples":             float64(-3),
				"chronic_max_ttl_seconds": float64(1 << 20),
				"jitter_ratio":            float64(5),
				"recovery_successes":      float64(-1),
			},
		},
	})
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	ch = cfg.Escalation.Chronic
	if ch.SuccessRateThreshold != schemas.MaxChronicSuccessRateThreshold ||
		ch.MinSamples != schemas.DefaultChronicMinSamples ||
		ch.ChronicMaxTTLSeconds != schemas.MaxChronicMaxTTLSeconds ||
		ch.JitterRatio != schemas.MaxChronicJitterRatio ||
		ch.RecoverySuccesses != schemas.DefaultChronicRecoverySuccesses {
		t.Fatalf("chronic clamps = %+v, want %v/%d/%d/%v/%d", ch,
			schemas.MaxChronicSuccessRateThreshold, schemas.DefaultChronicMinSamples,
			schemas.MaxChronicMaxTTLSeconds, schemas.MaxChronicJitterRatio,
			schemas.DefaultChronicRecoverySuccesses)
	}

	// Unknown keys inside chronic are rejected.
	if _, err := ParseConfig(map[string]any{
		"escalation": map[string]any{
			"enabled": true,
			"chronic": map[string]any{"enabled": true, "jiter_ratio": 0.2},
		},
	}); err == nil {
		t.Fatal("unknown escalation.chronic key must be rejected")
	}

	// An enabled chronic block under a disabled escalation section is rejected:
	// it could never do anything.
	if _, err := ParseConfig(map[string]any{
		"escalation": map[string]any{
			"enabled": false,
			"chronic": map[string]any{"enabled": true},
		},
	}); err == nil {
		t.Fatal("escalation.chronic.enabled with escalation.enabled=false must be rejected")
	}

	// A disabled chronic block under a disabled escalation is fine.
	cfg, err = ParseConfig(map[string]any{
		"escalation": map[string]any{
			"enabled": false,
			"chronic": map[string]any{"enabled": false},
		},
	})
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Escalation.Chronic == nil || cfg.Escalation.Chronic.Enabled {
		t.Fatalf("chronic = %+v, want present and disabled", cfg.Escalation.Chronic)
	}
}

// TestChronicNormalizedDeepCopies pins that normalizing an escalation section
// never aliases (or mutates) the caller's nested chronic block, and that the
// plugin default is inherited only by providers without their own section.
func TestChronicNormalizedDeepCopies(t *testing.T) {
	ch := &schemas.CooldownChronicConfig{Enabled: true, MinSamples: 7}
	esc := &schemas.CooldownEscalationConfig{Enabled: true, MaxMultiplier: 0, Chronic: ch}
	norm := esc.Normalized()
	if norm.Chronic == ch {
		t.Fatal("Normalized aliased the caller's chronic block; it must be deep-copied")
	}
	if norm.Chronic.MinSamples != ch.MinSamples {
		t.Fatalf("normalized chronic MinSamples = %d, want the input's %d", norm.Chronic.MinSamples, ch.MinSamples)
	}
	if norm.MaxMultiplier != schemas.DefaultEscalationMaxMultiplier {
		t.Fatalf("normalized MaxMultiplier = %d, want the default", norm.MaxMultiplier)
	}

	// Whole-section replacement: the plugin default (with chronic) is inherited
	// by a provider without its own escalation section, and dropped entirely by
	// a provider whose own section omits chronic.
	s := (&Config{Escalation: &EscalationConfig{
		Enabled:       true,
		MaxMultiplier: 4,
		Chronic:       &ChronicConfig{Enabled: true, MinSamples: 5},
	}}).AsState(nil)
	spec, ok := s.defaults.chronicSpec(nil)
	if !ok || spec.minSamples != 5 {
		t.Fatalf("inherited chronic spec = %+v (ok=%t), want minSamples=5", spec, ok)
	}
	if _, ok := s.defaults.chronicSpec(&schemas.CooldownPolicy{
		Escalation: &schemas.CooldownEscalationConfig{Enabled: true, MaxMultiplier: 4},
	}); ok {
		t.Fatal("a provider escalation section without chronic must not inherit the plugin chronic block")
	}

	// Validate rejects an enabled chronic under a disabled escalation section.
	if err := (&schemas.CooldownEscalationConfig{
		Enabled: false,
		Chronic: &schemas.CooldownChronicConfig{Enabled: true},
	}).Validate(); err == nil {
		t.Fatal("Validate must reject chronic.enabled with escalation.enabled=false")
	}
}

// TestChronicStatsCount pins the minimal observability: the stats snapshot
// reports how many distinct keys are latched chronic.
func TestChronicStatsCount(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := chronicPolicy(&schemas.CooldownChronicConfig{
		Enabled:              true,
		SuccessRateThreshold: 0.3,
		MinSamples:           3,
		ChronicMaxTTLSeconds: 600,
		JitterRatio:          0,
	})
	observeSteps(s, clk, policy, schemas.OpenAI, "bad-a", "gpt-4o", false, 3, time.Second)
	observeSteps(s, clk, policy, schemas.OpenAI, "bad-b", "gpt-4o", false, 3, time.Second)
	observeSteps(s, clk, policy, schemas.OpenAI, "good", "gpt-4o", true, 3, time.Second)

	if got := s.Stats().ChronicKeyCount; got != 2 {
		t.Fatalf("ChronicKeyCount = %d, want 2", got)
	}
	if got := s.healthSize(); got != 3 {
		t.Fatalf("health rows = %d, want 3", got)
	}
	// No health weighting was enabled, so the filter must leave weights alone
	// even though chronic is observing.
	out, err := s.AsFilter(nil)(policyCtx(policy), schemas.OpenAI, "gpt-4o", []schemas.Key{{ID: "bad-a", Weight: 1}})
	if err != nil {
		t.Fatalf("AsFilter: %v", err)
	}
	if len(out) != 1 || math.Abs(out[0].Weight-1) > 1e-9 {
		t.Fatalf("weight = %v, want the unchanged 1 (health weighting is off)", out[0].Weight)
	}
}
