// Package rtk — tests for CompressionMetrics.
//
// These tests cover the read/write contract the RTK Monitoring panel
// depends on: RecordInvocation must accumulate atomically under concurrent
// callers, Snapshot must derive tokens-saved / compression-ratio correctly
// (including the empty-state zero ratio), and the nil-receiver guards must
// keep both methods no-op so Plugin.Metrics() callers can fire-and-forget.
package rtk

import (
	"sync"
	"testing"
)

// TestCompressionMetrics_RecordInvocationHappyPath verifies the basic
// aggregation: every invocation increments Invocations, only compressed
// passes bump CompressedCount and the token sums.
func TestCompressionMetrics_RecordInvocationHappyPath(t *testing.T) {
	m := &CompressionMetrics{}
	m.RecordInvocation(false, 0, 0) // passthrough — no compression
	m.RecordInvocation(true, 1000, 400)
	m.RecordInvocation(true, 2000, 500)

	snap := m.Snapshot()
	if snap.Invocations != 3 {
		t.Errorf("Invocations = %d, want 3", snap.Invocations)
	}
	if snap.CompressedCount != 2 {
		t.Errorf("CompressedCount = %d, want 2", snap.CompressedCount)
	}
	if snap.OriginalTokens != 3000 {
		t.Errorf("OriginalTokens = %d, want 3000", snap.OriginalTokens)
	}
	if snap.CompressedTokens != 900 {
		t.Errorf("CompressedTokens = %d, want 900", snap.CompressedTokens)
	}
	if snap.TokensSaved != 2100 {
		t.Errorf("TokensSaved = %d, want 2100", snap.TokensSaved)
	}
	want := 2100.0 / 3000.0
	if snap.CompressionRatio < want-0.001 || snap.CompressionRatio > want+0.001 {
		t.Errorf("CompressionRatio = %f, want ~%f", snap.CompressionRatio, want)
	}
}

// TestCompressionMetrics_EmptySnapshotReturnsZeroRatio guards the
// divide-by-zero edge case the UI relies on for a freshly-started gateway.
func TestCompressionMetrics_EmptySnapshotReturnsZeroRatio(t *testing.T) {
	m := &CompressionMetrics{}
	snap := m.Snapshot()
	if snap.Invocations != 0 || snap.CompressedCount != 0 {
		t.Errorf("expected zero counters on empty metrics, got %+v", snap)
	}
	if snap.CompressionRatio != 0 {
		t.Errorf("CompressionRatio on empty metrics = %f, want 0 (no NaN)", snap.CompressionRatio)
	}
	if snap.TokensSaved != 0 {
		t.Errorf("TokensSaved on empty metrics = %d, want 0", snap.TokensSaved)
	}
}

// TestCompressionMetrics_PassthroughDoesNotAffectTokenSums guards the rule
// that uncompressed passes (anyCompressed=false) must NOT contribute to
// OriginalTokens/CompressedTokens — otherwise the "tokens saved" figure
// would drift on every chat request that had no tool calls.
func TestCompressionMetrics_PassthroughDoesNotAffectTokenSums(t *testing.T) {
	m := &CompressionMetrics{}
	m.RecordInvocation(false, 999_999, 0) // pretend this happened
	snap := m.Snapshot()
	if snap.Invocations != 1 {
		t.Errorf("Invocations = %d, want 1 (passthrough must still count)", snap.Invocations)
	}
	if snap.OriginalTokens != 0 || snap.CompressedTokens != 0 {
		t.Errorf("passthrough leaked into token sums: orig=%d comp=%d", snap.OriginalTokens, snap.CompressedTokens)
	}
}

// TestCompressionMetrics_NegativeTokensClamped ensures the helper doesn't
// produce a phantom negative savings if a future bug feeds compressed >
// original. We treat that as zero rather than going through uint64 wrap.
func TestCompressionMetrics_NegativeTokensClamped(t *testing.T) {
	m := &CompressionMetrics{}
	m.RecordInvocation(true, 100, 200) // degenerate: "compressed" > "original"
	snap := m.Snapshot()
	if snap.TokensSaved != 0 {
		t.Errorf("TokensSaved = %d, want 0 when compressed > original", snap.TokensSaved)
	}
}

// TestCompressionMetrics_NilReceiverSafe makes sure the methods are
// no-ops on a nil metrics handle — Plugin.Metrics() returns nil for
// uninitialised plugins and the caller chains p.metrics.RecordInvocation
// without a guard.
func TestCompressionMetrics_NilReceiverSafe(t *testing.T) {
	var m *CompressionMetrics
	m.RecordInvocation(true, 100, 50)
	snap := m.Snapshot()
	if snap != (MetricsSnapshot{}) {
		t.Errorf("nil Snapshot() = %+v, want zero value", snap)
	}
}

// TestCompressionMetrics_ConcurrentRecordInvocation stresses the read/write
// contract under heavy parallel callers — Invocations and CompressedCount
// must equal the expected totals even when goroutines race on Add.
func TestCompressionMetrics_ConcurrentRecordInvocation(t *testing.T) {
	m := &CompressionMetrics{}
	const goroutines = 32
	const each = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(odd bool) {
			defer wg.Done()
			for j := 0; j < each; j++ {
				if odd {
					m.RecordInvocation(true, 10, 4)
				} else {
					m.RecordInvocation(false, 0, 0)
				}
			}
		}(i%2 == 0)
	}
	wg.Wait()

	snap := m.Snapshot()
	if snap.Invocations != goroutines*each {
		t.Errorf("Invocations = %d, want %d", snap.Invocations, goroutines*each)
	}
	half := goroutines / 2
	wantCompressed := uint64(half * each)
	if snap.CompressedCount != wantCompressed {
		t.Errorf("CompressedCount = %d, want %d", snap.CompressedCount, wantCompressed)
	}
	wantOrig := wantCompressed * 10
	wantComp := wantCompressed * 4
	if snap.OriginalTokens != wantOrig {
		t.Errorf("OriginalTokens = %d, want %d", snap.OriginalTokens, wantOrig)
	}
	if snap.CompressedTokens != wantComp {
		t.Errorf("CompressedTokens = %d, want %d", snap.CompressedTokens, wantComp)
	}
}

// TestPlugin_StatsReturnsZeroOnUninitialised guards the contract that
// *Plugin.Stats() is safe to call on a Plugin pointer that never went
// through Init (tests, scripted tools).
func TestPlugin_StatsReturnsZeroOnUninitialised(t *testing.T) {
	var p *Plugin
	snap := p.Stats()
	if snap != (MetricsSnapshot{}) {
		t.Errorf("nil Plugin.Stats() = %+v, want zero", snap)
	}

	q := &Plugin{}
	snap = q.Stats()
	if snap != (MetricsSnapshot{}) {
		t.Errorf("uninitialised Plugin.Stats() = %+v, want zero", snap)
	}
}