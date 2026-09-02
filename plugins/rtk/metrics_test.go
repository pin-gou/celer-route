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
	m.RecordEngineBreakdown([]EngineBreakdown{{Id: "rtk", InputBytes: 100, OutputBytes: 50}})
	snap := m.Snapshot()
	if snap.Invocations != 0 || snap.CompressedCount != 0 || snap.OriginalTokens != 0 ||
		snap.CompressedTokens != 0 || snap.TokensSaved != 0 || snap.CompressionRatio != 0 ||
		snap.EngineBreakdown != nil {
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
	if snap.Invocations != 0 || snap.CompressedCount != 0 || snap.OriginalTokens != 0 ||
		snap.CompressedTokens != 0 || snap.TokensSaved != 0 || snap.CompressionRatio != 0 ||
		snap.EngineBreakdown != nil {
		t.Errorf("nil Plugin.Stats() = %+v, want zero value", snap)
	}

	q := &Plugin{}
	snap = q.Stats()
	if snap.Invocations != 0 || snap.CompressedCount != 0 || snap.OriginalTokens != 0 ||
		snap.CompressedTokens != 0 || snap.TokensSaved != 0 || snap.CompressionRatio != 0 ||
		snap.EngineBreakdown != nil {
		t.Errorf("uninitialised Plugin.Stats() = %+v, want zero value", snap)
	}
}

// TestCompressionMetrics_HistogramBucketsByWindow verifies that events land in
// the right bucket and that the bucket aggregates match the lifetime snapshot
// for the window.
func TestCompressionMetrics_HistogramBucketsByWindow(t *testing.T) {
	m := &CompressionMetrics{}
	// Two events in the same hour bucket (10:00:00-10:59:59), one in the next.
	hour := int64(3600)
	base := int64(1_600_000_000) // arbitrary aligned-ish timestamp
	m.RecordInvocationAt(base, true, 1000, 400)
	m.RecordInvocationAt(base+60, true, 2000, 500)
	m.RecordInvocationAt(base+hour, false, 0, 0)

	buckets := m.Histogram(base, base+2*hour, hour)
	if len(buckets) != 2 {
		t.Fatalf("Histogram window = %d buckets, want 2", len(buckets))
	}
	b0 := buckets[0]
	if b0.Timestamp != (base/hour)*hour {
		t.Errorf("bucket[0].Timestamp = %d, want %d", b0.Timestamp, (base/hour)*hour)
	}
	if b0.Invocations != 2 || b0.CompressedCount != 2 {
		t.Errorf("bucket[0] Invocations/Compressed = %d/%d, want 2/2", b0.Invocations, b0.CompressedCount)
	}
	if b0.OriginalTokens != 3000 || b0.CompressedTokens != 900 {
		t.Errorf("bucket[0] Original/Compressed tokens = %d/%d, want 3000/900", b0.OriginalTokens, b0.CompressedTokens)
	}
	if b0.TokensSaved != 2100 {
		t.Errorf("bucket[0] TokensSaved = %d, want 2100", b0.TokensSaved)
	}
	wantRatio := 2100.0 / 3000.0
	if b0.CompressionRatio < wantRatio-0.001 || b0.CompressionRatio > wantRatio+0.001 {
		t.Errorf("bucket[0] CompressionRatio = %f, want ~%f", b0.CompressionRatio, wantRatio)
	}

	b1 := buckets[1]
	if b1.Invocations != 1 || b1.CompressedCount != 0 {
		t.Errorf("bucket[1] Invocations/Compressed = %d/%d, want 1/0", b1.Invocations, b1.CompressedCount)
	}
	if b1.OriginalTokens != 0 || b1.TokensSaved != 0 {
		t.Errorf("bucket[1] tokens = %d/%d, want 0/0 (no-op pass)", b1.OriginalTokens, b1.TokensSaved)
	}
}

// TestCompressionMetrics_HistogramWindowBoundaries verifies buckets outside
// [start, end) are excluded and that a window covering only [start,end) with
// no events returns nil.
func TestCompressionMetrics_HistogramWindowBoundaries(t *testing.T) {
	m := &CompressionMetrics{}
	hour := int64(3600)
	base := int64(1_600_000_000)
	m.RecordInvocationAt(base, true, 100, 50)
	m.RecordInvocationAt(base+hour*10, true, 100, 50) // well outside window

	buckets := m.Histogram(base, base+hour, hour)
	if len(buckets) != 1 {
		t.Fatalf("window buckets = %d, want 1", len(buckets))
	}
	if buckets[0].Invocations != 1 {
		t.Errorf("window bucket invocations = %d, want 1", buckets[0].Invocations)
	}

	// A window with no events → nil.
	empty := m.Histogram(base+hour*2, base+hour*3, hour)
	if empty != nil {
		t.Errorf("empty window = %+v, want nil", empty)
	}
}

// TestCompressionMetrics_HistogramLRUEviction verifies the cap: beyond
// maxBuckets distinct timestamps the oldest buckets are swept (they disappear
// from the histogram, but the lifetime snapshot keeps their numbers).
func TestCompressionMetrics_HistogramLRUEviction(t *testing.T) {
	m := &CompressionMetrics{}
	second := int64(1)
	base := int64(1_600_000_000)
	// Write maxBuckets+5 distinct timestamps.
	for i := 0; i < maxBuckets+5; i++ {
		m.RecordInvocationAt(base+int64(i)*second, true, 10, 4)
	}
	if got := len(m.bucketMetric.buckets); got != maxBuckets {
		t.Fatalf("bucket map size = %d, want cap %d", got, maxBuckets)
	}
	// The 5 oldest were evicted. Querying a window that includes them should
	// return at most maxBuckets buckets, and the first surviving key should
	// be base+5.
	// Note: bucket map keys are raw unix timestamps here (1-second keys), so
	// bucketSizeSeconds=1 keeps keys stable.
	window := m.Histogram(base, base+second*int64(maxBuckets+5), second)
	if len(window) != maxBuckets {
		t.Fatalf("window buckets = %d, want %d", len(window), maxBuckets)
	}
	if window[0].Timestamp != base+5 {
		t.Errorf("oldest surviving bucket = %d, want %d (5 oldest evaporated)", window[0].Timestamp, base+5)
	}
	// Lifetime counters keep everything.
	snap := m.Snapshot()
	wantInvoc := uint64(maxBuckets + 5)
	if snap.Invocations != wantInvoc {
		t.Errorf("lifetime Invocations = %d, want %d", snap.Invocations, wantInvoc)
	}
}

// TestCompressionMetrics_HistogramConcurrent stresses bucket accumulation
// under parallel RecordInvocationAt callers with differing timestamps.
func TestCompressionMetrics_HistogramConcurrent(t *testing.T) {
	m := &CompressionMetrics{}
	const goroutines = 16
	const each = 200
	base := int64(1_600_000_000)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < each; j++ {
				ts := base + int64(n)*1000 + int64(j) // spread across many seconds
				m.RecordInvocationAt(ts, true, 10, 4)
			}
		}(i)
	}
	wg.Wait()

	buckets := m.Histogram(base, base+int64(goroutines)*1000, 1000)
	totalInv := uint64(0)
	for _, b := range buckets {
		totalInv += b.Invocations
	}
	if totalInv != goroutines*each {
		t.Errorf("histogram summed Invocations = %d, want %d", totalInv, goroutines*each)
	}
	// Lifetime must match too.
	if snap := m.Snapshot(); snap.Invocations != goroutines*each {
		t.Errorf("lifetime Invocations = %d, want %d", snap.Invocations, goroutines*each)
	}
}

// TestCompressionMetrics_RecordEngineBreakdownHappyPath verifies the
// per-engine accumulator surfaces in Snapshot.EngineBreakdown with the
// derived compressed_by ratio.
func TestCompressionMetrics_RecordEngineBreakdownHappyPath(t *testing.T) {
	m := &CompressionMetrics{}
	m.RecordEngineBreakdown([]EngineBreakdown{
		{Id: "rtk", InputBytes: 1000, OutputBytes: 400},
	})
	m.RecordEngineBreakdown([]EngineBreakdown{
		{Id: "rtk", InputBytes: 2000, OutputBytes: 500},
		{Id: "caveman", InputBytes: 500, OutputBytes: 100},
	})

	snap := m.Snapshot()
	if len(snap.EngineBreakdown) != 2 {
		t.Fatalf("EngineBreakdown len = %d, want 2", len(snap.EngineBreakdown))
	}
	// Stable ordering: by ID ascending — caveman precedes rtk.
	if snap.EngineBreakdown[0].ID != "caveman" {
		t.Errorf("EngineBreakdown[0].ID = %q, want caveman (sorted)", snap.EngineBreakdown[0].ID)
	}
	if snap.EngineBreakdown[1].ID != "rtk" {
		t.Errorf("EngineBreakdown[1].ID = %q, want rtk (sorted)", snap.EngineBreakdown[1].ID)
	}
	caveman := snap.EngineBreakdown[0]
	if caveman.Invocations != 1 {
		t.Errorf("caveman Invocations = %d, want 1", caveman.Invocations)
	}
	if caveman.InputBytes != 500 || caveman.OutputBytes != 100 {
		t.Errorf("caveman bytes = %d/%d, want 500/100", caveman.InputBytes, caveman.OutputBytes)
	}
	wantRatio := 400.0 / 500.0
	if caveman.CompressedBy < wantRatio-0.001 || caveman.CompressedBy > wantRatio+0.001 {
		t.Errorf("caveman CompressedBy = %f, want ~%f", caveman.CompressedBy, wantRatio)
	}
	rtk := snap.EngineBreakdown[1]
	if rtk.Invocations != 2 {
		t.Errorf("rtk Invocations = %d, want 2", rtk.Invocations)
	}
	if rtk.InputBytes != 3000 || rtk.OutputBytes != 900 {
		t.Errorf("rtk bytes = %d/%d, want 3000/900", rtk.InputBytes, rtk.OutputBytes)
	}
	wantRatio = 2100.0 / 3000.0
	if rtk.CompressedBy < wantRatio-0.001 || rtk.CompressedBy > wantRatio+0.001 {
		t.Errorf("rtk CompressedBy = %f, want ~%f", rtk.CompressedBy, wantRatio)
	}
}

// TestCompressionMetrics_RecordEngineBreakdownEmptyGuards covers the
// nil/empty fast-path: both the receiver guard and the empty-slice guard
// must no-op so callers don't need to check before invoking.
func TestCompressionMetrics_RecordEngineBreakdownEmptyGuards(t *testing.T) {
	var m *CompressionMetrics
	m.RecordEngineBreakdown([]EngineBreakdown{{Id: "rtk", InputBytes: 100, OutputBytes: 50}}) // nil receiver
	snap := m.Snapshot()
	if snap.EngineBreakdown != nil {
		t.Errorf("nil receiver Snapshot.EngineBreakdown = %+v, want nil", snap.EngineBreakdown)
	}

	live := &CompressionMetrics{}
	live.RecordEngineBreakdown(nil)
	live.RecordEngineBreakdown([]EngineBreakdown{}) // explicit empty
	snap = live.Snapshot()
	if snap.EngineBreakdown != nil {
		t.Errorf("empty RecordEngineBreakdown produced %+v, want nil", snap.EngineBreakdown)
	}
}

// TestCompressionMetrics_RecordEngineBreakdownIgnoresEmptyID guards against
// a pipeline returning an entry with no engine id (defensive — the runner
// already filters these but the public surface accepts arbitrary slices).
func TestCompressionMetrics_RecordEngineBreakdownIgnoresEmptyID(t *testing.T) {
	m := &CompressionMetrics{}
	m.RecordEngineBreakdown([]EngineBreakdown{
		{Id: "", InputBytes: 999, OutputBytes: 999},
		{Id: "rtk", InputBytes: 100, OutputBytes: 50},
	})
	snap := m.Snapshot()
	if len(snap.EngineBreakdown) != 1 {
		t.Fatalf("EngineBreakdown len = %d, want 1 (empty id ignored)", len(snap.EngineBreakdown))
	}
	if snap.EngineBreakdown[0].ID != "rtk" {
		t.Errorf("EngineBreakdown[0].ID = %q, want rtk", snap.EngineBreakdown[0].ID)
	}
	if snap.EngineBreakdown[0].InputBytes != 100 {
		t.Errorf("InputBytes = %d, want 100 (empty-id entry must not leak)", snap.EngineBreakdown[0].InputBytes)
	}
}

// TestCompressionMetrics_SnapshotEngineBreakdownNilWhenNoActivity keeps
// the contract that an idle gateway returns nil (not an empty slice) so
// the UI can short-circuit rendering without inspecting the array.
func TestCompressionMetrics_SnapshotEngineBreakdownNilWhenNoActivity(t *testing.T) {
	m := &CompressionMetrics{}
	snap := m.Snapshot()
	if snap.EngineBreakdown != nil {
		t.Errorf("Snapshot().EngineBreakdown = %+v on idle metrics, want nil", snap.EngineBreakdown)
	}
}

// TestCompressionMetrics_ConcurrentRecordEngineBreakdown stresses the
// engineMu path under parallel callers — every entry must be accounted
// for in the final snapshot, mirroring the aggregate atomic-counter
// concurrency contract.
func TestCompressionMetrics_ConcurrentRecordEngineBreakdown(t *testing.T) {
	m := &CompressionMetrics{}
	const goroutines = 16
	const each = 500

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < each; j++ {
				// Alternate engine id so both buckets get hit.
				id := "rtk"
				if (n+j)%2 == 0 {
					id = "caveman"
				}
				m.RecordEngineBreakdown([]EngineBreakdown{{
					Id:          id,
					InputBytes:  100,
					OutputBytes: 40,
				}})
			}
		}(i)
	}
	wg.Wait()

	snap := m.Snapshot()
	if len(snap.EngineBreakdown) != 2 {
		t.Fatalf("EngineBreakdown len = %d, want 2", len(snap.EngineBreakdown))
	}
	total := uint64(0)
	for _, e := range snap.EngineBreakdown {
		total += e.Invocations
	}
	if total != goroutines*each {
		t.Errorf("sum of engine Invocations = %d, want %d", total, goroutines*each)
	}
}
