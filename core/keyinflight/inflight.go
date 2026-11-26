// Package keyinflight tracks how many requests are currently in flight for each
// (provider, API key) pair, so core can apply an optional per-key concurrency
// cap (schemas.ProviderConfig.MaxConcurrentRequestsPerKey).
//
// The whole feature is opt-in and disabled by default: every method takes the
// configured limit and returns immediately when it is <= 0, without reading or
// writing any state. A gateway that never sets the cap therefore pays no
// locking, no map lookup, and no allocation on the hot path.
//
// This is a concurrency gate, not a cooldown/health signal: a saturated key is
// merely less attractive for the next request's key selection, and a request
// that finds every candidate saturated is still allowed through (fail-open).
// Turning key-pool saturation into a user-visible error would be worse than
// briefly exceeding the cap.
package keyinflight

import "sync"

// key identifies one (provider, API key) counter. Keeping both fields means the
// same key ID used by two providers (e.g. two custom providers wrapping the same
// upstream) is counted independently.
type key struct {
	provider string
	keyID    string
}

// Limiter is a concurrency-safe in-flight counter per (provider, key).
// The zero value is not usable; construct it with New.
type Limiter struct {
	mu     sync.Mutex
	counts map[key]int
}

// New returns an empty Limiter.
func New() *Limiter {
	return &Limiter{counts: make(map[key]int)}
}

// Acquire increments the in-flight count for (provider, keyID) when it is below
// limit and reports whether a slot was taken.
//
// limit <= 0 disables tracking for this call: it returns true without touching
// any state (so a caller can use the return value uniformly while provably
// leaving the map untouched). When the count is already at or above limit it
// returns false and does not increment.
func (l *Limiter) Acquire(provider string, keyID string, limit int) bool {
	if limit <= 0 {
		return true
	}

	k := key{provider: provider, keyID: keyID}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[k] >= limit {
		return false
	}
	l.counts[k]++
	return true
}

// Release decrements the in-flight count for (provider, keyID). The entry is
// deleted when the count reaches zero so the map cannot grow without bound as
// keys are added and removed.
//
// Extra releases are ignored defensively: they never panic and never drive a
// counter negative. The limiter deliberately has no logger dependency, so the
// caller keeps full control over log volume.
func (l *Limiter) Release(provider string, keyID string) {
	k := key{provider: provider, keyID: keyID}
	l.mu.Lock()
	defer l.mu.Unlock()
	n, ok := l.counts[k]
	if !ok {
		return
	}
	if n <= 1 {
		delete(l.counts, k)
		return
	}
	l.counts[k] = n - 1
}

// InFlight reports the current number of in-flight requests for
// (provider, keyID). Intended for tests, debug endpoints, and key-selection
// preference; it is not on any hot path that runs when the cap is disabled.
func (l *Limiter) InFlight(provider string, keyID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[key{provider: provider, keyID: keyID}]
}

// Reset clears all counters. Test helper only: production never resets a live
// limiter, because dropping counters would let extra requests through that the
// cap was meant to hold back.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	clear(l.counts)
}
