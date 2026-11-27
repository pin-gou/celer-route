package providercooldown

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// ---------------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------------

// rpmRateLimitErr builds the shape observed on the SenseNova deployment for a
// request-rate rejection: the provider's rate_limit_error type with an
// EndpointRPMExceeded code / "rpm exhausted" message, and no retry-after
// header (only X-Request-Id), which is why the dimension has to be derived
// from the body rather than a hint.
func rpmRateLimitErr() *schemas.BifrostError {
	return &schemas.BifrostError{
		StatusCode: intPtr(429),
		Error: &schemas.ErrorField{
			Message: "rpm exhausted",
			Type:    strPtr("rate_limit_error"),
			Code:    strPtr("EndpointRPMExceeded"),
		},
	}
}

// tpmRateLimitErr is the token-rate counterpart of rpmRateLimitErr.
func tpmRateLimitErr() *schemas.BifrostError {
	return &schemas.BifrostError{
		StatusCode: intPtr(429),
		Error: &schemas.ErrorField{
			Message: "tpm exhausted",
			Type:    strPtr("rate_limit_error"),
			Code:    strPtr("EndpointTPMExceeded"),
		},
	}
}

// ---------------------------------------------------------------------------
// 1. B3 red test: RPM rejections must not pollute the token estimate
// ---------------------------------------------------------------------------

// TestLearningRPMRejectionsDoNotPolluteTokenEstimate is the B3 regression pin:
// an RPM rejection says nothing about the token budget, so it must not enter
// the token-limit estimate. Before the fix, a bad key rejected on RPM at
// usedTokens≈0 dragged the median (and the lower bound) to 0, which is what
// the production learned-limits report showed.
func TestLearningRPMRejectionsDoNotPolluteTokenEstimate(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 100, MinSamples: 5})

	// Five genuine token-budget rejections at 3k..7k tokens consumed in the
	// window. Advancing past the window before each spend makes the measured
	// consumption exactly the amount just spent.
	for i := 0; i < 5; i++ {
		clk.Advance(61 * time.Second)
		learnSpend(t, p, schemas.OpenAI, "gpt-4o", (i+3)*1000)
		learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-tpm", tpmRateLimitErr())
	}

	// Twenty RPM rejections on an idle token window (usedTokens≈0).
	for i := 0; i < 20; i++ {
		clk.Advance(61 * time.Second)
		learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-rpm", rpmRateLimitErr())
	}

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")

	// The estimate must rest on the five token-rate samples only; the median of
	// {3000,4000,5000,6000,7000} is 5000. With the RPM zeros included the
	// median of the 25 samples is 0 (the pre-fix behaviour).
	if l.EstimatedTokenLimit != 5000 {
		t.Fatalf("estimated_token_limit = %d, want 5000 (RPM rejections must not enter the token-limit estimate; got bounds [%d,%d], %d samples)",
			l.EstimatedTokenLimit, l.LowerBound, l.UpperBound, l.SampleCount)
	}
	if l.LowerBound != 3000 {
		t.Fatalf("lower_bound = %d, want 3000 (RPM rejections at usedTokens=0 must not pull it to 0)", l.LowerBound)
	}

	// The report must still expose every observed sample, split by dimension,
	// so an operator can see the RPM noise and the token evidence separately.
	if l.SampleCount != 5 {
		t.Fatalf("sample_count = %d, want 5 (samples participating in the estimate)", l.SampleCount)
	}
	if got := l.DimensionSampleCounts["rpm"]; got != 20 {
		t.Fatalf("dimension_sample_counts[rpm] = %d, want 20", got)
	}
	if got := l.DimensionSampleCounts["tpm"]; got != 5 {
		t.Fatalf("dimension_sample_counts[tpm] = %d, want 5", got)
	}
	if l.LimiterDimension != string(dimensionTPM) {
		t.Fatalf("limiter_dimension = %q, want %q", l.LimiterDimension, dimensionTPM)
	}
	if l.KeyCount != 1 {
		t.Fatalf("key_count = %d, want 1 (only key-tpm carries the estimate)", l.KeyCount)
	}
}

// TestLearningAllRPMReportsUnusableEstimate pins the all-RPM shape: the entry
// is still listed so the operator can see the rejections, but there is no
// token estimate and the dominant dimension says why.
func TestLearningAllRPMReportsUnusableEstimate(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 2})

	for i := 0; i < 5; i++ {
		learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-rpm", rpmRateLimitErr())
	}

	l := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if l.EstimatedTokenLimit != 0 || l.LowerBound != 0 || l.UpperBound != 0 {
		t.Fatalf("expected no token estimate from RPM-only samples, got %+v", l)
	}
	if l.Confidence != LearningConfidenceInsufficient {
		t.Fatalf("confidence = %q, want %q", l.Confidence, LearningConfidenceInsufficient)
	}
	if l.SampleCount != 0 {
		t.Fatalf("sample_count = %d, want 0 (no sample participates in the estimate)", l.SampleCount)
	}
	if l.LimiterDimension != string(dimensionRPM) {
		t.Fatalf("limiter_dimension = %q, want %q", l.LimiterDimension, dimensionRPM)
	}
	if got := l.DimensionSampleCounts["rpm"]; got != 5 {
		t.Fatalf("dimension_sample_counts[rpm] = %d, want 5", got)
	}
}

// ---------------------------------------------------------------------------
// 2. dimension mapping (table driven)
// ---------------------------------------------------------------------------

func TestLimiterDimensionOf(t *testing.T) {
	cases := []struct {
		name string
		err  *schemas.BifrostError
		want limiterDimension
	}{
		{
			name: "EndpointRPMExceeded code",
			err:  newErr(429, "rate_limit_error", "EndpointRPMExceeded", "rpm exhausted"),
			want: dimensionRPM,
		},
		{
			name: "ModelAccountRpmRateLimitExceeded code",
			err:  newErr(429, "quota_exceeded_error", "ModelAccountRpmRateLimitExceeded", "rate limit"),
			want: dimensionRPM,
		},
		{
			name: "rpm exhausted message",
			err:  newErr(429, "", "8", "rpm exhausted"),
			want: dimensionRPM,
		},
		{
			name: "rps exhausted message folds into rpm",
			err:  newErr(429, "", "8", "rps exhausted"),
			want: dimensionRPM,
		},
		{
			name: "RequestBurstTooFast type",
			err:  newErr(429, "RequestBurstTooFast", "", ""),
			want: dimensionRPM,
		},
		{
			name: "Throttling.BurstRate type",
			err:  newErr(429, "Throttling.BurstRate", "", ""),
			want: dimensionRPM,
		},
		{
			name: "generic rate limit exceeded without token wording",
			err:  newErr(429, "", "", "rate limit exceeded"),
			want: dimensionRPM,
		},
		{
			name: "status-only 429 renders rate limit exceeded",
			err:  newErr(429, "", "", ""),
			want: dimensionRPM,
		},
		{
			name: "rate limit exceeded plus token wording resolves to tpm",
			err:  newErr(429, "", "", "rate limit exceeded: tokens per minute budget reached"),
			want: dimensionTPM,
		},
		{
			name: "EndpointTPMExceeded code is not misfiled as rpm",
			err:  newErr(429, "rate_limit_error", "EndpointTPMExceeded", ""),
			want: dimensionTPM,
		},
		{
			name: "ModelAccountTpmRateLimitExceeded code is not misfiled as rpm",
			err:  newErr(429, "rate_limit_error", "ModelAccountTpmRateLimitExceeded", ""),
			want: dimensionTPM,
		},
		{
			name: "tpm exhausted message",
			err:  newErr(429, "", "8", "tpm exhausted"),
			want: dimensionTPM,
		},
		{
			name: "token rate limit message",
			err:  newErr(429, "", "", "token rate limit reached"),
			want: dimensionTPM,
		},
		{
			name: "tokens per minute message",
			err:  newErr(429, "", "", "limit: tokens per minute"),
			want: dimensionTPM,
		},
		{
			name: "tpm code wins over a generic message",
			err:  newErr(429, "rate_limit_error", "EndpointTPMExceeded", "rate limit exceeded"),
			want: dimensionTPM,
		},
		{
			name: "unrelated 429 body is unknown",
			err:  newErr(429, "invalid_request_error", "", "boom"),
			want: dimensionUnknown,
		},
		{
			name: "nil error is unknown",
			err:  nil,
			want: dimensionUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := limiterDimensionOf(tc.err); got != tc.want {
				t.Fatalf("limiterDimensionOf() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. dimension-specific TTL on the mark path
// ---------------------------------------------------------------------------

func TestClassifyForMarkerDimensionTTL(t *testing.T) {
	policy := &schemas.CooldownPolicy{
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode:     "any",
			TTLSeconds:    30,
			RPMTTLSeconds: 90,
			TPMTTLSeconds: 40,
			Match:         []schemas.CooldownPolicyMatch{{StatusCode: intPtr(429)}},
		},
	}
	ctx := makeCtx(t, policy)

	cases := []struct {
		name string
		err  *schemas.BifrostError
		want time.Duration
	}{
		{"rpm", rpmRateLimitErr(), 90 * time.Second},
		{"tpm", tpmRateLimitErr(), 40 * time.Second},
		{"unknown falls back to ttl_seconds", newErr(429, "", "", "boom"), 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ttl, _, _, kind, _, ok := classifyForMarker(ctx, schemas.OpenAI, "key-1", "key-1", tc.err)
			if !ok || kind != CooldownKindRateLimit {
				t.Fatalf("expected a rate_limit match, got ok=%v kind=%q", ok, kind)
			}
			if ttl != tc.want {
				t.Fatalf("ttl = %v, want %v", ttl, tc.want)
			}
		})
	}
}

// TestClassifyForMarkerQuotaIgnoresDimensionTTL pins that the dimension split
// is rate_limit-only: even a policy that reached the runtime without
// validation cannot make a quota mark take an rpm/tpm TTL.
func TestClassifyForMarkerQuotaIgnoresDimensionTTL(t *testing.T) {
	policy := &schemas.CooldownPolicy{
		Quota: &schemas.CooldownPolicyRule{
			MatchMode:     "any",
			TTLSeconds:    600,
			RPMTTLSeconds: 90,
			TPMTTLSeconds: 40,
			Match:         []schemas.CooldownPolicyMatch{{MessageContains: []string{"quota exceeded"}}},
		},
	}
	ctx := makeCtx(t, policy)
	err := newErr(429, "", "EndpointRPMExceeded", "quota exceeded")
	ttl, _, _, kind, _, ok := classifyForMarker(ctx, schemas.OpenAI, "key-1", "key-1", err)
	if !ok || kind != CooldownKindQuota {
		t.Fatalf("expected a quota match, got ok=%v kind=%q", ok, kind)
	}
	if ttl != 600*time.Second {
		t.Fatalf("quota ttl = %v, want 600s (dimension TTLs are rate_limit-only)", ttl)
	}
}

// ---------------------------------------------------------------------------
// 4. retry_after_mode interaction (table driven)
// ---------------------------------------------------------------------------

func TestEffectiveCooldownTTLDimensionAndRetryAfter(t *testing.T) {
	rule := &schemas.CooldownPolicyRule{
		TTLSeconds:    30,
		RPMTTLSeconds: 90,
		TPMTTLSeconds: 40,
	}
	errWithHint := func(base *schemas.BifrostError, hint string) *schemas.BifrostError {
		base.ExtraFields.ProviderResponseHeaders = map[string]string{"Retry-After": hint}
		return base
	}

	cases := []struct {
		name string
		mode string
		err  func() *schemas.BifrostError
		want time.Duration
	}{
		// rpm base = 90s.
		{"rpm off no hint", "", func() *schemas.BifrostError { return rpmRateLimitErr() }, 90 * time.Second},
		{"rpm off hint below base", "", func() *schemas.BifrostError { return errWithHint(rpmRateLimitErr(), "20") }, 90 * time.Second},
		{"rpm off hint above base", "", func() *schemas.BifrostError { return errWithHint(rpmRateLimitErr(), "120") }, 120 * time.Second},
		{"rpm off hint above 24h ceiling is rejected", "", func() *schemas.BifrostError { return errWithHint(rpmRateLimitErr(), "90000") }, 90 * time.Second},
		{"rpm cap hint below base", "cap", func() *schemas.BifrostError { return errWithHint(rpmRateLimitErr(), "20") }, 20 * time.Second},
		{"rpm cap hint above base", "cap", func() *schemas.BifrostError { return errWithHint(rpmRateLimitErr(), "120") }, 90 * time.Second},
		{"rpm cap no hint", "cap", func() *schemas.BifrostError { return rpmRateLimitErr() }, 90 * time.Second},
		{"rpm exact hint", "exact", func() *schemas.BifrostError { return errWithHint(rpmRateLimitErr(), "20") }, 20 * time.Second},
		{"rpm exact no hint", "exact", func() *schemas.BifrostError { return rpmRateLimitErr() }, 90 * time.Second},

		// tpm base = 40s.
		{"tpm off no hint", "", func() *schemas.BifrostError { return tpmRateLimitErr() }, 40 * time.Second},
		{"tpm off hint above base", "", func() *schemas.BifrostError { return errWithHint(tpmRateLimitErr(), "60") }, 60 * time.Second},
		{"tpm cap hint below base", "cap", func() *schemas.BifrostError { return errWithHint(tpmRateLimitErr(), "10") }, 10 * time.Second},
		{"tpm exact hint", "exact", func() *schemas.BifrostError { return errWithHint(tpmRateLimitErr(), "10") }, 10 * time.Second},

		// unknown base = ttl_seconds = 30s.
		{"unknown off no hint", "", func() *schemas.BifrostError { return newErr(429, "", "", "boom") }, 30 * time.Second},
		{"unknown off hint above base", "", func() *schemas.BifrostError { return errWithHint(newErr(429, "", "", "boom"), "45") }, 45 * time.Second},
		{"unknown cap hint above base", "cap", func() *schemas.BifrostError { return errWithHint(newErr(429, "", "", "boom"), "45") }, 30 * time.Second},
		{"unknown exact hint", "exact", func() *schemas.BifrostError { return errWithHint(newErr(429, "", "", "boom"), "45") }, 45 * time.Second},
		{"unknown exact no hint", "exact", func() *schemas.BifrostError { return newErr(429, "", "", "boom") }, 30 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := *rule
			r.RetryAfterMode = tc.mode
			if got := effectiveCooldownTTL(&r, tc.err()); got != tc.want {
				t.Fatalf("effectiveCooldownTTL() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEffectiveCooldownTTLPerFieldFallback pins that each dimension opts in
// independently: rpm set + tpm unset means a TPM rejection uses ttl_seconds,
// and vice versa.
func TestEffectiveCooldownTTLPerFieldFallback(t *testing.T) {
	rpmOnly := &schemas.CooldownPolicyRule{TTLSeconds: 30, RPMTTLSeconds: 90}
	if got := effectiveCooldownTTL(rpmOnly, rpmRateLimitErr()); got != 90*time.Second {
		t.Fatalf("rpm ttl = %v, want 90s", got)
	}
	if got := effectiveCooldownTTL(rpmOnly, tpmRateLimitErr()); got != 30*time.Second {
		t.Fatalf("tpm ttl = %v, want 30s (tpm_ttl_seconds unset falls back to ttl_seconds)", got)
	}

	tpmOnly := &schemas.CooldownPolicyRule{TTLSeconds: 30, TPMTTLSeconds: 40}
	if got := effectiveCooldownTTL(tpmOnly, rpmRateLimitErr()); got != 30*time.Second {
		t.Fatalf("rpm ttl = %v, want 30s (rpm_ttl_seconds unset falls back to ttl_seconds)", got)
	}
	if got := effectiveCooldownTTL(tpmOnly, tpmRateLimitErr()); got != 40*time.Second {
		t.Fatalf("tpm ttl = %v, want 40s", got)
	}
}

// ---------------------------------------------------------------------------
// 5. disabled regression: byte-for-byte identical to the pre-B3 behaviour
// ---------------------------------------------------------------------------

// legacyEffectiveCooldownTTL is a copy of the pre-B3 helper. The regression
// test asserts the new implementation produces exactly the same duration for
// every combination once rpm_ttl_seconds / tpm_ttl_seconds are left unset.
func legacyEffectiveCooldownTTL(rule *schemas.CooldownPolicyRule, bifrostErr *schemas.BifrostError) time.Duration {
	if rule == nil {
		return 0
	}
	ttl := time.Duration(rule.TTLSeconds) * time.Second
	mode := rule.RetryAfterModeOrDefault()
	if mode != "cap" && mode != "exact" {
		return ttl
	}
	if bifrostErr == nil {
		return ttl
	}
	hint := providerRetryAfterHint(bifrostErr.ExtraFields.ProviderResponseHeaders)
	if hint <= 0 {
		return ttl
	}
	if mode == "cap" && hint > ttl {
		return ttl
	}
	return hint
}

func TestEffectiveCooldownTTLDisabledMatchesLegacy(t *testing.T) {
	modes := []string{"", "off", "cap", "exact", "CAP"}
	ttls := []int{1, 30, 60, 600}
	hints := []string{"", "0", "5", "30", "120", "90000", "not-a-date"}
	errs := []func() *schemas.BifrostError{
		func() *schemas.BifrostError { return rpmRateLimitErr() },
		func() *schemas.BifrostError { return tpmRateLimitErr() },
		func() *schemas.BifrostError { return newErr(429, "", "", "boom") },
		func() *schemas.BifrostError { return nil },
	}

	for _, mode := range modes {
		for _, ttl := range ttls {
			for _, hint := range hints {
				for i, makeErr := range errs {
					rule := &schemas.CooldownPolicyRule{TTLSeconds: ttl, RetryAfterMode: mode}
					bifrostErr := makeErr()
					if bifrostErr != nil && hint != "" {
						bifrostErr.ExtraFields.ProviderResponseHeaders = map[string]string{"Retry-After": hint}
					}
					want := legacyEffectiveCooldownTTL(rule, bifrostErr)
					got := effectiveCooldownTTL(rule, bifrostErr)
					if got != want {
						t.Fatalf("mode=%q ttl=%d hint=%q errCase=%d: got %v, want legacy %v", mode, ttl, hint, i, got, want)
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 6. concurrency
// ---------------------------------------------------------------------------

func TestLearningDimensionConcurrentObserveAndLimits(t *testing.T) {
	clk := newFakeClock()
	l := newLearner(&LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 64, MinSamples: 3}, clk.Now)
	dims := []limiterDimension{dimensionRPM, dimensionTPM, dimensionUnknown}

	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				l.observe(schemas.OpenAI, "gpt-4o", fmt.Sprintf("key-%d", i%4), int64(i%7)*1000, 0, dims[(w+i)%3], clk.Now())
			}
		}(w)
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = l.limits()
			}
		}()
	}
	wg.Wait()

	got := l.limits()
	if len(got) != 1 {
		t.Fatalf("expected one (provider, model) entry, got %d", len(got))
	}
	total := 0
	for _, n := range got[0].DimensionSampleCounts {
		total += n
	}
	if total > l.maxSamples {
		t.Fatalf("retained samples = %d, want at most %d", total, l.maxSamples)
	}
	if total == 0 {
		t.Fatal("expected the concurrent observations to be collected")
	}
}

// TestMarkerAppliesDimensionTTL pins the end-to-end path: the per-key failure
// marker classifies the rejection, derives the dimension and records a mark
// whose remaining time matches the configured dimension TTL.
func TestMarkerAppliesDimensionTTL(t *testing.T) {
	clk := newFakeClock()
	s := NewCooldownState(5 * time.Minute)
	s.now = clk.Now
	policy := &schemas.CooldownPolicy{
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode:     "any",
			TTLSeconds:    30,
			RPMTTLSeconds: 90,
			Match:         []schemas.CooldownPolicyMatch{{StatusCode: intPtr(429)}},
		},
	}
	ctx := makeCtx(t, policy)

	s.AsMarker(nil)(ctx, schemas.OpenAI, "key-rpm", "key-rpm", "gpt-4o", rpmRateLimitErr())

	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one cooldown entry, got %d", len(snap))
	}
	if snap[0].Remaining != 90*time.Second {
		t.Fatalf("cooldown remaining = %v, want 90s (the rpm_ttl_seconds value)", snap[0].Remaining)
	}
}
