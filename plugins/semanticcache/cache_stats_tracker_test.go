package semanticcache

import (
	"sync"
	"testing"
)

// TestCacheStatsTrackerHitMiss exercises the public Hit/Miss path. The
// saved-token and saved-cost counters must accumulate exactly what callers
// pass in — anything else would corrupt the /api/cache/stats dashboard.
func TestCacheStatsTrackerHitMiss(t *testing.T) {
	tr := NewCacheStatsTracker()
	tr.Hit(100, 0.05)
	tr.Hit(200, 0.10)
	tr.Miss()
	tr.Miss()
	tr.Miss()
	snap := tr.Snapshot()
	if snap.Hits != 2 {
		t.Errorf("hits = %d, want 2", snap.Hits)
	}
	if snap.Misses != 3 {
		t.Errorf("misses = %d, want 3", snap.Misses)
	}
	if snap.SavedInputTokens != 300 {
		t.Errorf("saved_input_tokens = %d, want 300", snap.SavedInputTokens)
	}
	// float arithmetic — compare with epsilon.
	if diff := snap.SavedCost - 0.15; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("saved_cost = %v, want 0.15", snap.SavedCost)
	}
	wantRate := 2.0 / 5.0
	if diff := snap.HitRate - wantRate; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("hit_rate = %v, want %v", snap.HitRate, wantRate)
	}
}

// TestCacheStatsTrackerHitRateZeroWhenEmpty guards against NaN rendering in
// the UI on a freshly-started gateway. Snapshot must return 0, not
// 0/0=NaN, when neither counter has moved.
func TestCacheStatsTrackerHitRateZeroWhenEmpty(t *testing.T) {
	tr := NewCacheStatsTracker()
	snap := tr.Snapshot()
	if snap.HitRate != 0 {
		t.Errorf("empty tracker hit_rate = %v, want 0", snap.HitRate)
	}
}

// TestCacheStatsTrackerRejectsNegativeInputs covers the underflow guard —
// pricing paths occasionally pass negative token counts during a misuse,
// and an underflow would silently wrap to a huge positive.
func TestCacheStatsTrackerRejectsNegativeInputs(t *testing.T) {
	tr := NewCacheStatsTracker()
	tr.Hit(-50, -1.0) // both negative — must be ignored entirely
	snap := tr.Snapshot()
	if snap.Hits != 1 {
		t.Errorf("hits should still increment even with negative payloads, got %d", snap.Hits)
	}
	if snap.SavedInputTokens != 0 {
		t.Errorf("saved_input_tokens = %d, want 0 (negative ignored)", snap.SavedInputTokens)
	}
	if snap.SavedCost != 0 {
		t.Errorf("saved_cost = %v, want 0 (negative ignored)", snap.SavedCost)
	}
}

// TestCacheStatsTrackerConcurrent runs many goroutines hitting / missing to
// confirm the atomic increments sum exactly. Without this we'd discover a
// lost-update regression only under production traffic.
func TestCacheStatsTrackerConcurrent(t *testing.T) {
	tr := NewCacheStatsTracker()
	const goroutines = 32
	const ops = 1000
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				if j%2 == 0 {
					tr.Hit(1, 0.001)
				} else {
					tr.Miss()
				}
			}
		}()
	}
	wg.Wait()
	snap := tr.Snapshot()
	wantHits := uint64(goroutines * ops / 2)
	wantMisses := uint64(goroutines * ops / 2)
	if snap.Hits != wantHits {
		t.Errorf("hits = %d, want %d", snap.Hits, wantHits)
	}
	if snap.Misses != wantMisses {
		t.Errorf("misses = %d, want %d", snap.Misses, wantMisses)
	}
}

// TestCacheStatsTrackerNilSafe guarantees the Hit/Miss/Snapshot methods
// can be called through a nil pointer without panicking. Plugin
// constructors in tests instantiate Plugin{} directly without Init, and
// they reach these methods through the same nil-tolerant paths.
func TestCacheStatsTrackerNilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil receiver should not panic, got %v", r)
		}
	}()
	var tr *CacheStatsTracker
	tr.Hit(1, 0.1)
	tr.Miss()
	snap := tr.Snapshot()
	if snap.Hits != 0 || snap.Misses != 0 {
		t.Errorf("nil tracker must report zero state, got %+v", snap)
	}
}
