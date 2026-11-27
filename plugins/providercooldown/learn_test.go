package providercooldown

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newLearnTestPlugin builds a plugin whose state carries the learner, with
// the fake clock installed. It skips NewPlugin's background GC goroutine so
// the state stays stable for the assertions below.
func newLearnTestPlugin(t *testing.T, clk *fakeClock, cfg *LearningConfig) *CooldownPlugin {
	t.Helper()
	s := NewCooldownState(5 * time.Minute)
	s.now = clk.Now
	if n := cfg.Normalized(); n.Active() {
		s.enableLearning(n)
	}
	return &CooldownPlugin{State: s}
}

// learnRateLimitErr builds the kind of 429 the default OpenAI policy
// classifies as rate_limit (not quota).
func learnRateLimitErr() *schemas.BifrostError {
	return &schemas.BifrostError{
		StatusCode: intPtr(429),
		Error: &schemas.ErrorField{
			Message: "Rate limit reached for requests",
			Type:    strPtr("rate_limit_error"),
		},
	}
}

// learnQuotaErr builds the kind of 429 the default OpenAI policy classifies as
// quota (insufficient_quota), i.e. a billing/entitlement failure rather than
// a window limit.
func learnQuotaErr() *schemas.BifrostError {
	return &schemas.BifrostError{
		StatusCode: intPtr(429),
		Error: &schemas.ErrorField{
			Message: "You exceeded your current quota, please check your plan and billing details",
			Type:    strPtr("insufficient_quota"),
		},
	}
}

// learnSpend records real usage into the learner's observer window, the way
// a successful request does through PostLLMHook.
func learnSpend(t *testing.T, p *CooldownPlugin, provider schemas.ModelProvider, model string, tokens int) {
	t.Helper()
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	resp, err, hookErr := p.PostLLMHook(ctx, capacityResp(provider, model, tokens), nil)
	if hookErr != nil {
		t.Fatalf("PostLLMHook returned an error: %v", hookErr)
	}
	if err != nil {
		t.Fatalf("PostLLMHook fabricated an error: %v", err)
	}
	if resp == nil {
		t.Fatal("PostLLMHook dropped the response")
	}
}

// learnReject drives the per-key failure marker — the path core invokes the
// moment a provider refuses an attempt.
func learnReject(t *testing.T, p *CooldownPlugin, provider schemas.ModelProvider, model string, keyID string, bifrostErr *schemas.BifrostError) {
	t.Helper()
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	p.State.AsMarker(nil)(ctx, provider, keyID, keyID, model, bifrostErr)
}

// learnLimitsFor returns the learned entry for (provider, model).
func learnLimitsFor(t *testing.T, p *CooldownPlugin, provider schemas.ModelProvider, model string) LearnedLimit {
	t.Helper()
	for _, l := range p.LearnedLimits() {
		if l.Provider == string(provider) && l.Model == model {
			return l
		}
	}
	t.Fatalf("no learned limit for %s/%s", provider, model)
	return LearnedLimit{}
}

// ---------------------------------------------------------------------------
// 1. disabled => nothing is collected
// ---------------------------------------------------------------------------

func TestLearningDisabledCollectsNothing(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: false, WindowSeconds: 60, MaxSamples: 50, MinSamples: 5})

	if p.State.learner() != nil {
		t.Fatal("a disabled learning config must not install a learner")
	}
	if got := p.LearningWindowSeconds(); got != 0 {
		t.Fatalf("LearningWindowSeconds() = %d, want 0 when disabled", got)
	}

	// Blast the would-be sampling path: nothing may be collected.
	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 1000)
	for i := 0; i < 10; i++ {
		learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())
	}
	if got := p.LearnedLimits(); len(got) != 0 {
		t.Fatalf("disabled learning must report no limits, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// 2. collection
// ---------------------------------------------------------------------------

func TestLearningCollectsSamplesPerProviderModel(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 5})

	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 1000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())
	learnSpend(t, p, schemas.Anthropic, "claude-sonnet-4", 700)
	learnReject(t, p, schemas.Anthropic, "claude-sonnet-4", "key-2", learnRateLimitErr())

	limits := p.LearnedLimits()
	if len(limits) != 2 {
		t.Fatalf("expected one entry per (provider, model), got %d: %+v", len(limits), limits)
	}
	// Sorted by provider then model: anthropic comes first.
	if limits[0].Provider != string(schemas.Anthropic) || limits[0].Model != "claude-sonnet-4" {
		t.Fatalf("unexpected first entry: %+v", limits[0])
	}
	if limits[0].SampleCount != 1 {
		t.Fatalf("anthropic sample count = %d, want 1", limits[0].SampleCount)
	}
	if limits[1].Provider != string(schemas.OpenAI) || limits[1].Model != "gpt-4o" {
		t.Fatalf("unexpected second entry: %+v", limits[1])
	}
	if limits[1].SampleCount != 1 {
		t.Fatalf("openai sample count = %d, want 1", limits[1].SampleCount)
	}
	if limits[1].FirstSeen.IsZero() || limits[1].LastSeen.IsZero() {
		t.Fatalf("expected first/last seen timestamps, got %+v", limits[1])
	}
}

// ---------------------------------------------------------------------------
// 3. estimate lands inside the reported bounds
// ---------------------------------------------------------------------------

func TestLearningEstimateWithinBounds(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 3})

	// Three rejections at 1000 / 3000 / 6000 tokens of window consumption.
	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 1000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())
	clk.Advance(time.Second)
	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 2000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())
	clk.Advance(time.Second)
	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 3000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount != 3 {
		t.Fatalf("sample count = %d, want 3", l.SampleCount)
	}
	if l.EstimatedTokenLimit <= 0 {
		t.Fatalf("expected a positive estimate, got %+v", l)
	}
	if l.LowerBound > l.EstimatedTokenLimit || l.EstimatedTokenLimit > l.UpperBound {
		t.Fatalf("estimate %d outside bounds [%d, %d]", l.EstimatedTokenLimit, l.LowerBound, l.UpperBound)
	}
	// The median of {1000, 3000, 6000} is 3000.
	if l.EstimatedTokenLimit != 3000 {
		t.Fatalf("estimate = %d, want 3000 (median of the observed rejection points)", l.EstimatedTokenLimit)
	}
	if l.LowerBound != 1000 {
		t.Fatalf("lower bound = %d, want 1000 (smallest observed rejection point)", l.LowerBound)
	}
	if l.UpperBound != 6000 {
		t.Fatalf("upper bound = %d, want 6000 (largest observed would-be total)", l.UpperBound)
	}
	if l.Confidence != LearningConfidenceLow {
		t.Fatalf("confidence = %q, want %q", l.Confidence, LearningConfidenceLow)
	}
}

// ---------------------------------------------------------------------------
// 4. not enough samples => no estimate, but the count is still reported
// ---------------------------------------------------------------------------

func TestLearningInsufficientSamplesReportsCountWithoutEstimate(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 5})

	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 1000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount != 2 {
		t.Fatalf("sample count = %d, want 2", l.SampleCount)
	}
	if l.EstimatedTokenLimit != 0 || l.LowerBound != 0 || l.UpperBound != 0 {
		t.Fatalf("expected no estimate below min_samples, got %+v", l)
	}
	if l.Confidence != LearningConfidenceInsufficient {
		t.Fatalf("confidence = %q, want %q", l.Confidence, LearningConfidenceInsufficient)
	}
}

// ---------------------------------------------------------------------------
// 5. sample cap
// ---------------------------------------------------------------------------

func TestLearningSampleCapDropsOldestSamples(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 5, MinSamples: 1})

	// Eight observations of 100 tokens each, one per second: the window
	// consumption grows 100, 200, ... 800.
	for i := 0; i < 8; i++ {
		clk.Advance(time.Second)
		learnSpend(t, p, schemas.OpenAI, "gpt-4o", 100)
		learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())
	}

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount != 5 {
		t.Fatalf("sample count = %d, want it capped at 5", l.SampleCount)
	}
	// Only the newest five observations survive: {400, 500, 600, 700, 800}
	// → median 600. Keeping all eight would have produced 450.
	if l.EstimatedTokenLimit != 600 {
		t.Fatalf("estimate = %d, want 600 (the oldest samples must have been dropped)", l.EstimatedTokenLimit)
	}
	if l.LowerBound != 400 {
		t.Fatalf("lower bound = %d, want 400 (oldest retained sample)", l.LowerBound)
	}
}

// ---------------------------------------------------------------------------
// 6. the window slides: old consumption stops counting
// ---------------------------------------------------------------------------

func TestLearningWindowExpiryDropsOldUsage(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 5000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.EstimatedTokenLimit != 5000 {
		t.Fatalf("estimate = %d, want 5000 (the usage is still inside the window)", l.EstimatedTokenLimit)
	}

	// Push past the window: the 5000 tokens must no longer count.
	clk.Advance(61 * time.Second)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())

	l = learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount != 2 {
		t.Fatalf("sample count = %d, want 2", l.SampleCount)
	}
	// The retained samples are {5000, 0} → median 2500, envelope [0, 5000].
	if l.LowerBound != 0 {
		t.Fatalf("lower bound = %d, want 0 (the expired usage must not be counted)", l.LowerBound)
	}
}

// ---------------------------------------------------------------------------
// 7. only rate_limit rejections are sampled
// ---------------------------------------------------------------------------

func TestLearningIgnoresQuotaClassifiedRejections(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 1000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnQuotaErr())

	if got := p.LearnedLimits(); len(got) != 0 {
		t.Fatalf("quota-classified rejections must not produce samples, got %+v", got)
	}

	// The same provider/model with a rate-limit rejection does produce one.
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())
	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount != 1 {
		t.Fatalf("sample count = %d, want 1", l.SampleCount)
	}
	if l.EstimatedTokenLimit != 1000 {
		t.Fatalf("estimate = %d, want 1000 (the window usage that preceded the rejection)", l.EstimatedTokenLimit)
	}
}

// TestLearningNoMeasuredUsagePublishesNoEstimate pins the "nothing to
// estimate" case: the pair has enough samples, but none of them saw any real
// usage in the window, so publishing a 0 would be meaningless.
func TestLearningNoMeasuredUsagePublishesNoEstimate(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	// No learnSpend: the window is empty when the 429 arrives.
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount != 1 {
		t.Fatalf("sample count = %d, want 1", l.SampleCount)
	}
	if l.EstimatedTokenLimit != 0 || l.LowerBound != 0 || l.UpperBound != 0 {
		t.Fatalf("expected no estimate without measured usage, got %+v", l)
	}
	if l.Confidence != LearningConfidenceInsufficient {
		t.Fatalf("confidence = %q, want %q", l.Confidence, LearningConfidenceInsufficient)
	}
}

// ---------------------------------------------------------------------------
// 7b. the terminal-outcome fallback path
// ---------------------------------------------------------------------------

// learnTerminalCtx builds a request context carrying an attempt trail, the way
// core hands it to PostLLMHook after a provider attempt failed.
func learnTerminalCtx(provider schemas.ModelProvider, keyID string) *schemas.BifrostContext {
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{Attempt: 0, KeyID: keyID, KeyName: keyID},
	})
	return ctx
}

// learnTerminalErr builds a terminal provider error carrying routing info.
func learnTerminalErr(provider schemas.ModelProvider, model string, bifrostErr *schemas.BifrostError) *schemas.BifrostError {
	bifrostErr.ExtraFields.RoutingInfo = schemas.RoutingInfo{Provider: provider, Model: model}
	bifrostErr.ExtraFields.RequestType = schemas.ChatCompletionRequest
	return bifrostErr
}

// TestLearningSamplesTerminalRejectionWithoutMarker covers the fallback
// sampling path: the per-key marker is not invoked (keyless provider / SDK
// deployment without the marker wired), so the terminal rejection observed by
// PostLLMHook must still produce a sample.
func TestLearningSamplesTerminalRejectionWithoutMarker(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 900)
	ctx := learnTerminalCtx(schemas.OpenAI, "key-1")
	err := learnTerminalErr(schemas.OpenAI, "gpt-4o", learnRateLimitErr())
	if _, _, hookErr := p.PostLLMHook(ctx, nil, err); hookErr != nil {
		t.Fatalf("PostLLMHook returned an error: %v", hookErr)
	}

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount != 1 {
		t.Fatalf("sample count = %d, want 1", l.SampleCount)
	}
	if l.EstimatedTokenLimit != 900 {
		t.Fatalf("estimate = %d, want 900 (the window usage that preceded the rejection)", l.EstimatedTokenLimit)
	}
}

// TestLearningDoesNotDoubleSampleMarkerAndPostHook pins the dedup: the same
// terminal rejection is observed twice (once by the per-key marker inside the
// retry loop, once by PostLLMHook) but must produce a single sample.
func TestLearningDoesNotDoubleSampleMarkerAndPostHook(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	learnSpend(t, p, schemas.OpenAI, "gpt-4o", 900)
	ctx := learnTerminalCtx(schemas.OpenAI, "key-1")
	err := learnTerminalErr(schemas.OpenAI, "gpt-4o", learnRateLimitErr())

	// The marker fires first, inside the retry loop.
	p.State.AsMarker(nil)(ctx, schemas.OpenAI, "key-1", "key-1", "gpt-4o", err)
	// Then the terminal outcome reaches PostLLMHook.
	if _, _, hookErr := p.PostLLMHook(ctx, nil, err); hookErr != nil {
		t.Fatalf("PostLLMHook returned an error: %v", hookErr)
	}

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount != 1 {
		t.Fatalf("sample count = %d, want 1 (the same rejection must not be sampled twice)", l.SampleCount)
	}
}

// TestLearningPostHookIgnoresQuotaRejections mirrors the marker rule on the
// terminal path: quota-classified rejections never produce a sample.
func TestLearningPostHookIgnoresQuotaRejections(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	ctx := learnTerminalCtx(schemas.OpenAI, "key-1")
	err := learnTerminalErr(schemas.OpenAI, "gpt-4o", learnQuotaErr())
	if _, _, hookErr := p.PostLLMHook(ctx, nil, err); hookErr != nil {
		t.Fatalf("PostLLMHook returned an error: %v", hookErr)
	}

	if got := p.LearnedLimits(); len(got) != 0 {
		t.Fatalf("quota-classified rejections must not produce samples, got %+v", got)
	}
}

// TestLearningReportsDistinctKeyCount pins the multi-key caveat: the estimate
// aggregates every key of the provider/model, so the report says how many
// keys contributed.
func TestLearningReportsDistinctKeyCount(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-1", learnRateLimitErr())
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-2", learnRateLimitErr())

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount != 3 {
		t.Fatalf("sample count = %d, want 3", l.SampleCount)
	}
	if l.KeyCount != 2 {
		t.Fatalf("key count = %d, want 2 (key-1 and key-2)", l.KeyCount)
	}
}

// ---------------------------------------------------------------------------
// 8. concurrency
// ---------------------------------------------------------------------------
func TestLearningConcurrentRejectionsStayBounded(t *testing.T) {
	clk := newFakeClock()
	const maxSamples = 10
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: maxSamples, MinSamples: 1})

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			keyID := fmt.Sprintf("key-%d", i%4)
			learnReject(t, p, schemas.OpenAI, "gpt-4o", keyID, learnRateLimitErr())
		}(i)
	}
	wg.Wait()

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.SampleCount > maxSamples {
		t.Fatalf("sample count = %d, want at most %d", l.SampleCount, maxSamples)
	}
	if l.SampleCount == 0 {
		t.Fatal("expected the concurrent rejections to be collected")
	}
}

// ---------------------------------------------------------------------------
// 9. config parsing
// ---------------------------------------------------------------------------

func TestParseConfigLearningDefaults(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{
		"learning": map[string]any{"enabled": true},
	})
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	if cfg.Learning == nil {
		t.Fatal("expected a learning config")
	}
	if !cfg.Learning.Enabled {
		t.Fatal("expected learning to be enabled")
	}
	if cfg.Learning.WindowSeconds != defaultLearningWindowSeconds {
		t.Fatalf("window_seconds = %d, want %d", cfg.Learning.WindowSeconds, defaultLearningWindowSeconds)
	}
	if cfg.Learning.MaxSamples != defaultLearningMaxSamples {
		t.Fatalf("max_samples = %d, want %d", cfg.Learning.MaxSamples, defaultLearningMaxSamples)
	}
	if cfg.Learning.MinSamples != defaultLearningMinSamples {
		t.Fatalf("min_samples = %d, want %d", cfg.Learning.MinSamples, defaultLearningMinSamples)
	}
}

func TestParseConfigLearningClamps(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{
		"learning": map[string]any{
			"enabled":        true,
			"window_seconds": 5,
			"max_samples":    1,
			"min_samples":    1000,
		},
	})
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	if cfg.Learning.WindowSeconds != minLearningWindowSeconds {
		t.Fatalf("window_seconds = %d, want it clamped to %d", cfg.Learning.WindowSeconds, minLearningWindowSeconds)
	}
	if cfg.Learning.MaxSamples != minLearningMaxSamples {
		t.Fatalf("max_samples = %d, want it clamped to %d", cfg.Learning.MaxSamples, minLearningMaxSamples)
	}
	// min_samples is range-clamped to 100 first, then pulled down to the
	// (clamped) max_samples: a minimum above the cap could never be
	// reached and would silently disable the learner forever.
	if cfg.Learning.MinSamples != minLearningMaxSamples {
		t.Fatalf("min_samples = %d, want it pulled down to max_samples (%d)", cfg.Learning.MinSamples, minLearningMaxSamples)
	}

	cfg, err = ParseConfig(map[string]any{
		"learning": map[string]any{
			"enabled":        true,
			"window_seconds": 100000,
			"max_samples":    100000,
		},
	})
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	if cfg.Learning.WindowSeconds != maxLearningWindowSeconds {
		t.Fatalf("window_seconds = %d, want it clamped to %d", cfg.Learning.WindowSeconds, maxLearningWindowSeconds)
	}
	if cfg.Learning.MaxSamples != maxLearningMaxSamples {
		t.Fatalf("max_samples = %d, want it clamped to %d", cfg.Learning.MaxSamples, maxLearningMaxSamples)
	}
}

func TestParseConfigLearningRejectsBadTypes(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
	}{
		{"non object", map[string]any{"learning": "yes"}},
		{"non bool enabled", map[string]any{"learning": map[string]any{"enabled": "yes"}}},
		{"non int window", map[string]any{"learning": map[string]any{"window_seconds": "60"}}},
		{"non int max samples", map[string]any{"learning": map[string]any{"max_samples": "many"}}},
		{"non int min samples", map[string]any{"learning": map[string]any{"min_samples": true}}},
		{"unknown key", map[string]any{"learning": map[string]any{"nope": 1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseConfig(tc.raw); err == nil {
				t.Fatalf("expected ParseConfig to reject %s", tc.name)
			}
		})
	}
}

func TestConfigAsStateInstallsLearnerOnlyWhenEnabled(t *testing.T) {
	off, err := ParseConfig(map[string]any{"learning": map[string]any{"enabled": false}})
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	if off.AsState(nil).learner() != nil {
		t.Fatal("a disabled learning config must not install a learner")
	}

	on, err := ParseConfig(map[string]any{"learning": map[string]any{"enabled": true}})
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	state := on.AsState(nil)
	if state.learner() == nil {
		t.Fatal("an enabled learning config must install a learner")
	}
	if got := state.learner().window(); got != int64(defaultLearningWindowSeconds) {
		t.Fatalf("learner window = %d, want %d", got, defaultLearningWindowSeconds)
	}
}
