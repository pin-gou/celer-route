package providercooldown

import (
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Storm-signal tests.
//
// The storm signal answers "is this a shared upstream outage or just a few
// broken credentials?". The original rule ("3+ distinct keys failed for the
// same provider/model inside 60s") cannot tell the two apart: a fleet with
// four chronically broken credentials and two healthy ones keeps the signal on
// forever, so every healthy key's plain 429 is inflated to 2x.
//
// The refined rule (active only while the chronic section is on):
//
//   - chronically unhealthy keys do not count towards the distinct-key
//     threshold (the predicate is the very same one the chronic backoff uses);
//   - the signal must be FRESH — the failing-key set has to have grown
//     recently — so a stable split decays instead of holding the signal on.
//
// With chronic off the signal keeps its historical meaning exactly, which the
// regression test at the bottom pins.

// stormPolicy builds a provider policy with escalation + chronic enabled. The
// chronic block is deliberately permissive (3 samples, 0.3 threshold) so a
// test can latch a key with three observations.
func stormPolicy(recoverySuccesses int) *schemas.CooldownPolicy {
	return chronicPolicy(&schemas.CooldownChronicConfig{
		Enabled:              true,
		SuccessRateThreshold: 0.3,
		MinSamples:           3,
		ChronicMaxTTLSeconds: 600,
		JitterRatio:          0,
		RecoverySuccesses:    recoverySuccesses,
	})
}

// markStormKey marks (provider, keyID, model) through the policy-aware path
// and returns the TTL that was recorded.
func markStormKey(t *testing.T, s *CooldownState, policy *schemas.CooldownPolicy, keyID string) time.Duration {
	t.Helper()
	s.markWithPolicy(policy, schemas.OpenAI, keyID, "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	return recordedTTL(t, s, schemas.OpenAI, keyID, "gpt-4o", schemas.CooldownScopeModel)
}

// makeChronic drives enough failures through the observation path to latch
// the chronic mark for (provider, keyID, model).
func makeChronic(t *testing.T, s *CooldownState, clk *fakeClock, policy *schemas.CooldownPolicy, keyID string) {
	t.Helper()
	observeSteps(s, clk, policy, schemas.OpenAI, keyID, "gpt-4o", false, 3, time.Second)
	if !chronicNow(t, s, policy, schemas.OpenAI, keyID, "gpt-4o") {
		t.Fatalf("key %s must be chronic after 3 failures", keyID)
	}
}

// TestStormSignalIgnoresChronicKeys is the production shape: four chronically
// broken credentials keep failing while two healthy ones are fine. The storm
// signal must NOT fire, so a healthy key's plain 429 keeps its normal TTL.
func TestStormSignalIgnoresChronicKeys(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := stormPolicy(3)

	for _, keyID := range []string{"bad-1", "bad-2", "bad-3", "bad-4"} {
		makeChronic(t, s, clk, policy, keyID)
	}
	for _, keyID := range []string{"bad-1", "bad-2", "bad-3", "bad-4", "good-1", "good-2"} {
		markStormKey(t, s, policy, keyID)
	}

	// The distinct count is 2 (the two healthy keys) — below the threshold of
	// 3 — so this mark must NOT be inflated.
	if got := markStormKey(t, s, policy, "victim"); got != chronicTestBaseTTL {
		t.Fatalf("mark after 4 chronic + 2 healthy keys: TTL = %v, want the base %v (chronic keys must not count towards the storm)", got, chronicTestBaseTTL)
	}
}

// TestStormSignalFiresWithoutChronicKeys is the positive control: three
// healthy-but-failing keys inside the window are a genuine group failure, so
// the next mark starts at 2x.
func TestStormSignalFiresWithoutChronicKeys(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := stormPolicy(3)

	for _, keyID := range []string{"a", "b", "c"} {
		if got := markStormKey(t, s, policy, keyID); got != chronicTestBaseTTL {
			t.Fatalf("key %s: TTL = %v, want the base %v before the storm threshold", keyID, got, chronicTestBaseTTL)
		}
	}
	if got := markStormKey(t, s, policy, "d"); got != 2*chronicTestBaseTTL {
		t.Fatalf("key d: TTL = %v, want %v (storm start multiplier 2x)", got, 2*chronicTestBaseTTL)
	}
}

// TestStormSignalCountsRecoveredKey pins the hysteresis interaction: once the
// chronic latch is released the key counts towards the distinct set again, so
// the storm signal can fire once more.
//
// The two halves run on separate states so the first one cannot leave a
// fourth member behind that would push the count over the threshold on its
// own.
func TestStormSignalCountsRecoveredKey(t *testing.T) {
	// Phase 1: the key is still chronic, so it does not count.
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := stormPolicy(3)

	makeChronic(t, s, clk, policy, "bad")
	markStormKey(t, s, policy, "bad")
	markStormKey(t, s, policy, "b")
	markStormKey(t, s, policy, "c")
	// Only two non-chronic members (b, c) — the storm stays off.
	if got := markStormKey(t, s, policy, "d"); got != chronicTestBaseTTL {
		t.Fatalf("mark d: TTL = %v, want the base %v (one chronic member excluded)", got, chronicTestBaseTTL)
	}

	// Phase 2: the same fleet, but the bad key has recovered.
	clk2 := newFakeClock()
	s2 := newChronicTestState(clk2)
	makeChronic(t, s2, clk2, policy, "bad")
	for i := 0; i < 3; i++ {
		clk2.Advance(time.Second)
		s2.observe(policy, schemas.OpenAI, "bad", "gpt-4o", true)
	}
	if chronicNow(t, s2, policy, schemas.OpenAI, "bad", "gpt-4o") {
		t.Fatal("the chronic latch must be released after recovery_successes consecutive successes")
	}

	markStormKey(t, s2, policy, "bad")
	markStormKey(t, s2, policy, "b")
	markStormKey(t, s2, policy, "c")
	// The recovered key counts again: b, c and "bad" are three distinct
	// members, so the next mark is inflated.
	if got := markStormKey(t, s2, policy, "e"); got != 2*chronicTestBaseTTL {
		t.Fatalf("mark e after recovery: TTL = %v, want %v (the recovered key counts again)", got, 2*chronicTestBaseTTL)
	}
}

// TestStormSignalExpiresWithoutGrowth pins the freshness rule: a split whose
// failing-key set stops growing is not a group outage, so the signal decays
// after escalationStormFreshnessWindow.
func TestStormSignalExpiresWithoutGrowth(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := stormPolicy(3)

	for _, keyID := range []string{"a", "b", "c"} {
		markStormKey(t, s, policy, keyID)
	}
	// Still fresh: the set grew a moment ago.
	if got := markStormKey(t, s, policy, "d"); got != 2*chronicTestBaseTTL {
		t.Fatalf("mark d: TTL = %v, want %v (fresh storm)", got, 2*chronicTestBaseTTL)
	}

	// The set stops growing. After the freshness window the signal is stale:
	// the members are still inside the 60s storm window, so only the
	// freshness rule can turn it off.
	clk.Advance(escalationStormFreshnessWindow + time.Second)
	if got := markStormKey(t, s, policy, "e"); got != chronicTestBaseTTL {
		t.Fatalf("mark e after the freshness window: TTL = %v, want the base %v (a stable split must not keep the storm on)", got, chronicTestBaseTTL)
	}
}

// TestStormSignalDisabledChronicIsHistoricalRegression is the rollback
// guarantee: with chronic off the storm signal keeps its exact historical
// meaning — the distinct-key count alone decides, and no freshness window
// applies. This is what makes the whole refinement opt-in.
func TestStormSignalDisabledChronicIsHistoricalRegression(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	// Chronic is enabled on the escalation section but the chronic block
	// itself is absent → the persistent-failure semantics are off.
	policy := chronicPolicy(nil)

	for _, keyID := range []string{"a", "b", "c"} {
		if got := markStormKey(t, s, policy, keyID); got != chronicTestBaseTTL {
			t.Fatalf("key %s: TTL = %v, want the base %v before the storm threshold", keyID, got, chronicTestBaseTTL)
		}
	}
	if got := markStormKey(t, s, policy, "d"); got != 2*chronicTestBaseTTL {
		t.Fatalf("key d: TTL = %v, want %v (historical storm start multiplier 2x)", got, 2*chronicTestBaseTTL)
	}
	// The historical signal does NOT expire with the freshness window: the
	// members are still inside the 60s storm window, so it stays on.
	clk.Advance(escalationStormFreshnessWindow + time.Second)
	if got := markStormKey(t, s, policy, "e"); got != 2*chronicTestBaseTTL {
		t.Fatalf("key e: TTL = %v, want %v (chronic off must keep the historical storm behaviour)", got, 2*chronicTestBaseTTL)
	}
	// It does decay with the storm window, exactly like before.
	clk.Advance(escalationStormWindow)
	if got := markStormKey(t, s, policy, "f"); got != chronicTestBaseTTL {
		t.Fatalf("key f: TTL = %v, want the base %v (the storm window expired)", got, chronicTestBaseTTL)
	}
}

// TestStormSignalConcurrent exercises the refined storm path under -race:
// concurrent marks on chronic and healthy keys must never corrupt the
// bucket, the freshness timestamp or the chronic predicate.
func TestStormSignalConcurrent(t *testing.T) {
	clk := newFakeClock()
	s := newChronicTestState(clk)
	policy := stormPolicy(3)

	makeChronic(t, s, clk, policy, "bad")

	const goroutines = 8
	const marksPerGoroutine = 16
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < marksPerGoroutine; i++ {
				keyID := "k" + string(rune('a'+g))
				if g%2 == 0 {
					keyID = "bad"
				}
				s.markWithPolicy(policy, schemas.OpenAI, keyID, "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
			}
		}(g)
	}
	wg.Wait()

	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket := s.escalation.storm[stormBucketKey{provider: schemas.OpenAI, model: "gpt-4o"}]
	if bucket == nil {
		return
	}
	// One entry per distinct key, and the freshness timestamp is set.
	if len(bucket.members) > goroutines {
		t.Fatalf("storm bucket holds %d keys, want <= %d", len(bucket.members), goroutines)
	}
	if bucket.grewAt.IsZero() {
		t.Fatal("the storm bucket must carry a freshness timestamp")
	}
}
