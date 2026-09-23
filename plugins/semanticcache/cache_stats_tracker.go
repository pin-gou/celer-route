// Package semanticcache — cache_stats_tracker.go implements the in-process
// counter the Phase 5 cache-observability endpoint reads from. It is a
// deliberately narrow atomic-counter type (no histograms, no rolling
// buckets) for two reasons:
//
//  1. The /api/cache/stats endpoint asks for "how much has the cache saved
//     since boot" — not a time series. Histograms would multiply work for
//     no UI benefit until the cache_hit_counts table is added later.
//  2. The semantic cache plugin is already hot on the request path; a
//     mutex-guarded tracker would be a regression. atomic.Uint64 keeps the
//     hit/miss hook cost at one CAS per call.
//
// The plugin embeds *CacheStatsTracker; if it is nil the helpers degrade to
// no-ops so existing tests that construct Plugin{} without Init don't break.
package semanticcache

import (
	"sync/atomic"
)

// CacheStatsTracker holds process-lifetime counters for the semantic cache.
// All increments are atomic so PreLLMHook / PostLLMHook can update from
// concurrent requests without a lock. The /api/cache/stats endpoint reads a
// snapshot via Snapshot — callers should treat the result as best-effort
// across the read boundary (a hit increment that lands between two field
// reads shows up on the next call).
type CacheStatsTracker struct {
	hits             atomic.Uint64
	misses           atomic.Uint64
	savedInputTokens atomic.Uint64
	savedCostMicros  atomic.Uint64 // USD * 1_000_000 to keep float64 atomic
}

// NewCacheStatsTracker returns a fresh tracker with all counters at zero.
func NewCacheStatsTracker() *CacheStatsTracker {
	return &CacheStatsTracker{}
}

// Hit records a successful cache lookup. savedInputTokens is the input
// token count of the hit request — these are the tokens the cache kept off
// the upstream bill. savedCostMicros is USD × 1e6, rounded, so the
// Prometheus exporter can emit a float without dropping precision.
func (t *CacheStatsTracker) Hit(savedInputTokens int64, savedCostUSD float64) {
	if t == nil {
		return
	}
	t.hits.Add(1)
	if savedInputTokens > 0 {
		// uint64 underflow guard — negative tokens come from a misconfigured
		// pricing path and would corrupt the counter silently otherwise.
		t.savedInputTokens.Add(uint64(savedInputTokens))
	}
	if savedCostUSD > 0 {
		t.savedCostMicros.Add(uint64(savedCostUSD * 1_000_000))
	}
}

// Miss records a cache miss. The hit/miss symmetry is intentional: a miss
// against a non-cacheable request still counts (so hit_rate stays
// meaningful even when semantic cache is misconfigured to no-op).
func (t *CacheStatsTracker) Miss() {
	if t == nil {
		return
	}
	t.misses.Add(1)
}

// Snapshot is a read-only view onto the current counter state. Returned
// values are independent of any future increment so handlers can pass the
// struct out without worrying about a concurrent update race.
//
// HitRate is 0 when both counters are zero (no traffic yet) to avoid
// rendering NaN in the UI. The endpoint's "all" period hides this entirely.
type CacheStatsSnapshot struct {
	Hits             uint64  `json:"hits"`
	Misses           uint64  `json:"misses"`
	HitRate          float64 `json:"hit_rate"`
	SavedInputTokens uint64  `json:"saved_input_tokens"`
	SavedCost        float64 `json:"saved_cost"`
}

// Snapshot reads the current counter state. Safe to call concurrently with
// Hit / Miss — the worst case is observing one increment on hits and a
// later one on savedCost, which is fine for a /stats response.
func (t *CacheStatsTracker) Snapshot() CacheStatsSnapshot {
	if t == nil {
		return CacheStatsSnapshot{}
	}
	hits := t.hits.Load()
	misses := t.misses.Load()
	savedTokens := t.savedInputTokens.Load()
	savedCostMicros := t.savedCostMicros.Load()
	rate := 0.0
	if total := hits + misses; total > 0 {
		rate = float64(hits) / float64(total)
	}
	return CacheStatsSnapshot{
		Hits:             hits,
		Misses:           misses,
		HitRate:          rate,
		SavedInputTokens: savedTokens,
		SavedCost:        float64(savedCostMicros) / 1_000_000,
	}
}
