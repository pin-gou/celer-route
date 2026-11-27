package keyinflight

import (
	"fmt"
	"sync"
	"testing"
)

const testProvider = "openai"

func TestAcquireLimitZeroAlwaysAllowsAndTracksNothing(t *testing.T) {
	l := New()

	for i := 0; i < 10; i++ {
		if !l.Acquire(testProvider, "key-a", 0) {
			t.Fatalf("Acquire with limit 0 returned false on call %d", i)
		}
	}
	if got := l.InFlight(testProvider, "key-a"); got != 0 {
		t.Fatalf("InFlight after limit=0 acquires = %d, want 0", got)
	}
	if len(l.counts) != 0 {
		t.Fatalf("limit=0 acquire created %d map entries, want 0", len(l.counts))
	}

	// Negative limits are treated exactly like zero (defensive: a config that
	// slipped past normalization must not start counting).
	if !l.Acquire(testProvider, "key-a", -5) {
		t.Fatal("Acquire with negative limit returned false")
	}
	if got := l.InFlight(testProvider, "key-a"); got != 0 {
		t.Fatalf("InFlight after negative-limit acquire = %d, want 0", got)
	}
}

func TestAcquireRespectsLimit(t *testing.T) {
	l := New()
	const limit = 2

	if !l.Acquire(testProvider, "key-a", limit) {
		t.Fatal("first acquire failed")
	}
	if !l.Acquire(testProvider, "key-a", limit) {
		t.Fatal("second acquire failed")
	}
	if got := l.InFlight(testProvider, "key-a"); got != limit {
		t.Fatalf("InFlight = %d, want %d", got, limit)
	}
	if l.Acquire(testProvider, "key-a", limit) {
		t.Fatal("third acquire succeeded at the limit, want rejection")
	}
	if got := l.InFlight(testProvider, "key-a"); got != limit {
		t.Fatalf("rejected acquire changed InFlight to %d, want %d", got, limit)
	}

	// Releasing one slot frees exactly one acquire.
	l.Release(testProvider, "key-a")
	if got := l.InFlight(testProvider, "key-a"); got != limit-1 {
		t.Fatalf("InFlight after release = %d, want %d", got, limit-1)
	}
	if !l.Acquire(testProvider, "key-a", limit) {
		t.Fatal("acquire after release failed")
	}
	if l.Acquire(testProvider, "key-a", limit) {
		t.Fatal("acquire beyond the limit succeeded after refill")
	}
}

func TestReleaseRemovesEntryAndIgnoresExtras(t *testing.T) {
	l := New()

	if !l.Acquire(testProvider, "key-a", 1) {
		t.Fatal("acquire failed")
	}
	l.Release(testProvider, "key-a")
	if _, ok := l.counts[key{provider: testProvider, keyID: "key-a"}]; ok {
		t.Fatal("releasing the last slot left a zero entry in the map")
	}

	// Extra releases must not panic and must not make the count negative.
	l.Release(testProvider, "key-a")
	l.Release(testProvider, "never-seen")
	if got := l.InFlight(testProvider, "key-a"); got != 0 {
		t.Fatalf("InFlight after extra releases = %d, want 0", got)
	}

	// A fresh acquire after the extra releases still starts from zero.
	if !l.Acquire(testProvider, "key-a", 1) {
		t.Fatal("acquire after extra releases failed")
	}
	if got := l.InFlight(testProvider, "key-a"); got != 1 {
		t.Fatalf("InFlight = %d, want 1", got)
	}
}

func TestProviderNamespacesAreIndependent(t *testing.T) {
	l := New()
	const limit = 1

	if !l.Acquire("openai", "shared-id", limit) {
		t.Fatal("acquire for openai failed")
	}
	// Same key ID under another provider must not be affected by openai's cap.
	if !l.Acquire("anthropic", "shared-id", limit) {
		t.Fatal("acquire for anthropic failed although its own counter is empty")
	}
	if got := l.InFlight("openai", "shared-id"); got != 1 {
		t.Fatalf("openai InFlight = %d, want 1", got)
	}
	if got := l.InFlight("anthropic", "shared-id"); got != 1 {
		t.Fatalf("anthropic InFlight = %d, want 1", got)
	}

	l.Release("openai", "shared-id")
	if got := l.InFlight("anthropic", "shared-id"); got != 1 {
		t.Fatalf("releasing openai changed anthropic InFlight to %d, want 1", got)
	}
}

// TestConcurrentAcquireReleaseNoLeak hammers many keys from many goroutines and
// asserts the limiter does not leak counters once every acquired slot has been
// released. (Over-admission is covered deterministically by
// TestAcquireRespectsLimit; a mirror counter here would over-approximate, since
// Releasing before decrementing a mirror lets the next holder bump it first.)
func TestConcurrentAcquireReleaseNoLeak(t *testing.T) {
	l := New()
	const (
		goroutines = 64
		iterations = 200
		limit      = 4
	)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		acquired int
		released int
		rejected int
	)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			keyID := fmt.Sprintf("key-%d", g%7)
			for i := 0; i < iterations; i++ {
				if !l.Acquire(testProvider, keyID, limit) {
					mu.Lock()
					rejected++
					mu.Unlock()
					continue
				}
				mu.Lock()
				acquired++
				mu.Unlock()
				l.Release(testProvider, keyID)
				mu.Lock()
				released++
				mu.Unlock()
			}
		}(g)
	}
	wg.Wait()

	mu.Lock()
	gotAcquired, gotReleased, gotRejected := acquired, released, rejected
	mu.Unlock()
	if gotAcquired != gotReleased {
		t.Fatalf("acquired %d slots but released %d", gotAcquired, gotReleased)
	}
	if gotAcquired == 0 {
		t.Fatal("no acquire succeeded; the stress test exercised nothing")
	}
	if gotRejected == 0 {
		t.Fatalf("no acquire was ever rejected with %d goroutines racing on %d-slot keys", goroutines, limit)
	}

	for _, keyID := range []string{
		"key-0", "key-1", "key-2", "key-3", "key-4", "key-5", "key-6",
	} {
		if got := l.InFlight(testProvider, keyID); got != 0 {
			t.Fatalf("InFlight(%s) after stress = %d, want 0 (leaked counter)", keyID, got)
		}
	}
	if len(l.counts) != 0 {
		t.Fatalf("limiter map has %d entries after stress, want 0", len(l.counts))
	}
}

// TestConcurrentReleaseOfSameSlot exercises the defensive path: multiple
// goroutines releasing the same slot must not panic or go negative.
func TestConcurrentReleaseOfSameSlot(t *testing.T) {
	l := New()
	if !l.Acquire(testProvider, "key-a", 1) {
		t.Fatal("acquire failed")
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Release(testProvider, "key-a")
		}()
	}
	wg.Wait()

	if got := l.InFlight(testProvider, "key-a"); got != 0 {
		t.Fatalf("InFlight = %d, want 0", got)
	}
}

func TestReset(t *testing.T) {
	l := New()
	if !l.Acquire(testProvider, "key-a", 5) {
		t.Fatal("acquire failed")
	}
	l.Reset()
	if got := l.InFlight(testProvider, "key-a"); got != 0 {
		t.Fatalf("InFlight after Reset = %d, want 0", got)
	}
}
