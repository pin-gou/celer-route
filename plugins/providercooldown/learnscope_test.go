package providercooldown

import (
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// ---------------------------------------------------------------------------
// P1: the two learning levels must not be mixed
// ---------------------------------------------------------------------------

// TestLearningKeyLevelEstimatesAreIsolated pins the reason the learner records
// every rejection at both levels: two credentials with different budgets make
// the aggregate number meaningful to neither of them.
//
// key-small is rejected at 1,000 tokens of its own window, key-big at 5,000 of
// its own. The aggregate window then holds 6,000 tokens, so its estimate is the
// median of {1,000, 6,000} = 3,500 — a number no single key would ever hit.
// Adopting that into a key-scoped gate is exactly the "N times the budget"
// mistake the scope field exists to prevent.
func TestLearningKeyLevelEstimatesAreIsolated(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-small", 1000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-small", tpmRateLimitErr())

	learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-big", 5000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-big", tpmRateLimitErr())

	small := learnKeyLimitFor(t, p, schemas.OpenAI, "key-small", "gpt-4o")
	if small.Scope != LearningScopeKeyModel || small.KeyName != "key-small" {
		t.Fatalf("per-key entry identity = %+v, want scope=%q key=key-small", small, LearningScopeKeyModel)
	}
	if small.EstimatedTokenLimit != 1000 {
		t.Fatalf("key-small estimate = %d, want 1000 (its own window only)", small.EstimatedTokenLimit)
	}
	if small.KeyCount != 1 {
		t.Fatalf("key-small key_count = %d, want 1", small.KeyCount)
	}

	big := learnKeyLimitFor(t, p, schemas.OpenAI, "key-big", "gpt-4o")
	if big.EstimatedTokenLimit != 5000 {
		t.Fatalf("key-big estimate = %d, want 5000 (its own window only)", big.EstimatedTokenLimit)
	}

	agg := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if agg.Scope != LearningScopeProviderModel || agg.KeyID != "" {
		t.Fatalf("aggregate entry identity = %+v, want scope=%q with no key", agg, LearningScopeProviderModel)
	}
	if agg.EstimatedTokenLimit != 3500 {
		t.Fatalf("aggregate estimate = %d, want 3500 (median of {1000, 6000})", agg.EstimatedTokenLimit)
	}
	if agg.KeyCount != 2 {
		t.Fatalf("aggregate key_count = %d, want 2", agg.KeyCount)
	}
}

// TestLearningKeyLevelRespectsMinSamplesPerBucket pins that min_samples is
// applied PER BUCKET, not globally: a key with too few rejections of its own
// must not borrow confidence from the aggregate's sample count.
func TestLearningKeyLevelRespectsMinSamplesPerBucket(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 2})

	learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-thin", 1000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-thin", tpmRateLimitErr())

	learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-fat", 2000)
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-fat", tpmRateLimitErr())
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-fat", tpmRateLimitErr())

	thin := learnKeyLimitFor(t, p, schemas.OpenAI, "key-thin", "gpt-4o")
	if thin.SampleCount != 1 {
		t.Fatalf("key-thin sample_count = %d, want 1", thin.SampleCount)
	}
	if thin.EstimatedTokenLimit != 0 || thin.Confidence != LearningConfidenceInsufficient {
		t.Fatalf("key-thin must publish no estimate below min_samples, got %+v", thin)
	}

	fat := learnKeyLimitFor(t, p, schemas.OpenAI, "key-fat", "gpt-4o")
	if fat.EstimatedTokenLimit == 0 || fat.Confidence == LearningConfidenceInsufficient {
		t.Fatalf("key-fat has enough samples of its own and must publish an estimate, got %+v", fat)
	}

	agg := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if agg.SampleCount != 3 {
		t.Fatalf("aggregate sample_count = %d, want 3", agg.SampleCount)
	}
	if agg.EstimatedTokenLimit == 0 {
		t.Fatalf("aggregate has 3 samples and must publish an estimate, got %+v", agg)
	}
}

// ---------------------------------------------------------------------------
// P2: RPM rejections feed a REQUEST-budget estimate
// ---------------------------------------------------------------------------

// TestLearningRequestBudgetFromRPMRejections pins the second estimate: an RPM
// rejection carries no token evidence, but it says exactly how many requests
// the window had already counted when the provider refused.
func TestLearningRequestBudgetFromRPMRejections(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	// Ten requests already accounted for this key in the window.
	for i := 0; i < 10; i++ {
		learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-a", 10)
	}
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-a", rpmRateLimitErr())

	l := learnKeyLimitFor(t, p, schemas.OpenAI, "key-a", "gpt-4o")
	if l.SampleCount != 0 || l.EstimatedTokenLimit != 0 {
		t.Fatalf("RPM samples must not feed the token estimate, got %+v", l)
	}
	if l.RequestSampleCount != 1 {
		t.Fatalf("request_sample_count = %d, want 1", l.RequestSampleCount)
	}
	if l.EstimatedRequestLimit != 10 {
		t.Fatalf("estimated_request_limit = %d, want 10 (requests counted at rejection)", l.EstimatedRequestLimit)
	}
	if l.RequestLowerBound != 10 || l.RequestUpperBound != 11 {
		t.Fatalf("request bounds = [%d,%d], want [10,11]", l.RequestLowerBound, l.RequestUpperBound)
	}
	if l.RequestConfidence != LearningConfidenceLow {
		t.Fatalf("request confidence = %q, want %q", l.RequestConfidence, LearningConfidenceLow)
	}
	// The token confidence keeps its historical meaning: no token evidence.
	if l.Confidence != LearningConfidenceInsufficient {
		t.Fatalf("token confidence = %q, want %q", l.Confidence, LearningConfidenceInsufficient)
	}
}

// TestLearningRequestBudgetIsMedianOfObservedPoints pins the estimator itself
// (median, bracketed by the observed rejection points).
func TestLearningRequestBudgetIsMedianOfObservedPoints(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	spend := func(n int) {
		for i := 0; i < n; i++ {
			learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-a", 1)
		}
	}
	spend(2) // window request count reaches 2
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-a", rpmRateLimitErr())
	spend(3) // 5
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-a", rpmRateLimitErr())
	spend(5) // 10
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-a", rpmRateLimitErr())

	l := learnKeyLimitFor(t, p, schemas.OpenAI, "key-a", "gpt-4o")
	if l.RequestSampleCount != 3 {
		t.Fatalf("request_sample_count = %d, want 3", l.RequestSampleCount)
	}
	if l.EstimatedRequestLimit != 5 {
		t.Fatalf("estimated_request_limit = %d, want 5 (median of {2,5,10})", l.EstimatedRequestLimit)
	}
	if l.RequestLowerBound != 2 {
		t.Fatalf("request lower bound = %d, want 2", l.RequestLowerBound)
	}
	if l.RequestUpperBound != 11 {
		t.Fatalf("request upper bound = %d, want 11 (largest would-be total)", l.RequestUpperBound)
	}
}

// TestLearningRequestBudgetAggregatesAcrossKeys is the request-side twin of
// TestLearningKeyLevelEstimatesAreIsolated: the aggregate request estimate is
// the sum of every key's traffic, so it must be reported separately too.
func TestLearningRequestBudgetAggregatesAcrossKeys(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	for i := 0; i < 4; i++ {
		learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-a", 1)
	}
	for i := 0; i < 6; i++ {
		learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-b", 1)
	}
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-a", rpmRateLimitErr())
	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-b", rpmRateLimitErr())

	keyA := learnKeyLimitFor(t, p, schemas.OpenAI, "key-a", "gpt-4o")
	if keyA.EstimatedRequestLimit != 4 {
		t.Fatalf("key-a request estimate = %d, want 4", keyA.EstimatedRequestLimit)
	}
	keyB := learnKeyLimitFor(t, p, schemas.OpenAI, "key-b", "gpt-4o")
	if keyB.EstimatedRequestLimit != 6 {
		t.Fatalf("key-b request estimate = %d, want 6", keyB.EstimatedRequestLimit)
	}
	agg := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if agg.EstimatedRequestLimit != 10 {
		t.Fatalf("aggregate request estimate = %d, want 10 (4 + 6 requests across both keys)", agg.EstimatedRequestLimit)
	}
	if agg.KeyCount != 2 {
		t.Fatalf("aggregate key_count = %d, want 2", agg.KeyCount)
	}
}

// TestLearningKeyLevelWindowIsPerCredential pins that the two levels measure
// their OWN window: a key that has been idle past the learning window must see
// an empty window even when another key kept the aggregate busy.
func TestLearningKeyLevelWindowIsPerCredential(t *testing.T) {
	clk := newFakeClock()
	p := newLearnTestPlugin(t, clk, &LearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1})

	learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-idle", 5000)
	clk.Advance(61 * time.Second)
	learnSpendForKey(t, p, schemas.OpenAI, "gpt-4o", "key-busy", 700)

	learnReject(t, p, schemas.OpenAI, "gpt-4o", "key-idle", tpmRateLimitErr())

	idle := learnKeyLimitFor(t, p, schemas.OpenAI, "key-idle", "gpt-4o")
	if idle.EstimatedTokenLimit != 0 {
		t.Fatalf("key-idle estimate = %d, want 0 (its usage slid out of the window)", idle.EstimatedTokenLimit)
	}
	agg := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if agg.EstimatedTokenLimit != 700 {
		t.Fatalf("aggregate estimate = %d, want 700 (only key-busy is still inside the window)", agg.EstimatedTokenLimit)
	}
}
