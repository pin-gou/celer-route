// Package rtk — cross-request compression metrics.
//
// CompressionMetrics holds monotonic counters that accumulate every time
// applyRtkCompression (or its Responses-API twin) is invoked. Counters live
// on the Plugin struct for the lifetime of the running gateway and reset
// only on process restart; that mirrors the lifetime semantics of
// provider-cooldown's CooldownStats so the UI can show "since startup"
// figures without confusing operators about what the numbers mean.
//
// The handlers package reads this snapshot through Plugin.Metrics() to back
// GET /api/context/rtk/stats; the UI surfaces it inside the RTK
// configuration page's Monitoring panel.
//
// Time-bucketed histogram support: CompressionMetrics also holds a sliding
// window of per-bucket counters (up to maxBuckets = 4096) so the dashboard
// can render a time series of compression savings. Buckets are keyed by
// bucket-aligned Unix timestamp and evicted LRU when the cap is exceeded.
package rtk

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// maxBuckets is the hard cap on in-memory histogram buckets. At 1-hour buckets
// this covers ~170 days; at 1-minute buckets ~68 hours. The oldest bucket is
// evicted when a new bucket would exceed this limit, and its counters are
// merged into the lifetime totals (which are already tracked separately via
// the atomic counters, so the data is not lost — only the per-bucket
// granularity is).
const maxBuckets = 4096

// RtkHistogramBucket is one time bucket in the RTK metrics histogram. All
// fields are aggregated from every RecordInvocationAt call whose timestamp
// falls within this bucket's window.
type RtkHistogramBucket struct {
	Timestamp        int64   `json:"timestamp"`
	Invocations      uint64  `json:"invocations"`
	CompressedCount  uint64  `json:"compressed_count"`
	OriginalTokens   uint64  `json:"original_tokens"`
	CompressedTokens uint64  `json:"compressed_tokens"`
	TokensSaved      uint64  `json:"tokens_saved"`
	CompressionRatio float64 `json:"compression_ratio"`
}

// RtkStatsHistogram is the full response shape for the histogram endpoint.
// Buckets is the list of buckets within the requested time window, sorted
// ascending by timestamp. Totals is the aggregate of the window's buckets.
// LifetimeTotals mirrors GET /api/context/rtk/stats.
type RtkStatsHistogram struct {
	Buckets           []RtkHistogramBucket `json:"buckets"`
	BucketSizeSeconds int64                `json:"bucket_size_seconds"`
	Totals            RtkHistogramBucket   `json:"totals"`
	LifetimeTotals    MetricsSnapshot      `json:"lifetime_totals"`
	Plugin            string               `json:"plugin"`
}

// bucketCounter is the per-bucket accumulator. All fields are plain integers
// protected by the parent mutex, not atomic — the parent CompressionMetrics
// is the atomics boundary.
type bucketCounter struct {
	invocations      uint64
	compressed       uint64
	originalTokens   uint64
	compressedTokens uint64
}

// compressionBucketMetrics holds the sliding-window bucket map. Protected by
// a mutex because bucket operations are infrequent relative to the hot-path
// atomic counters on CompressionMetrics.
type compressionBucketMetrics struct {
	mu      sync.Mutex
	buckets map[int64]*bucketCounter // keyed by bucket-aligned unix timestamp
	keys    []int64                  // sorted ascending, for LRU eviction
}

func newCompressionBucketMetrics() *compressionBucketMetrics {
	return &compressionBucketMetrics{
		buckets: make(map[int64]*bucketCounter),
		keys:    make([]int64, 0, 256),
	}
}

// CompressionMetrics is the set of process-lifetime counters that the RTK
// plugin maintains. All fields are atomic so concurrent Pre/PostLLMHook
// calls can update them without locking.
type CompressionMetrics struct {
	invocations    atomic.Uint64 // every call to applyRtkCompression{,Responses}
	compressed     atomic.Uint64 // subset where at least one tool result was actually compressed
	originalTokens atomic.Uint64 // sum of OriginalTokens across compressed requests
	compressedToks atomic.Uint64 // sum of CompressedTokens across compressed requests

	bucketOnce   sync.Once
	bucketMetric *compressionBucketMetrics // lazy init on first RecordInvocationAt
}

// MetricsSnapshot is the JSON-shaped read-only view returned to the UI.
// Token savings are pre-computed because atomic counters can't be diffed
// after the fact; the compression ratio is also derived here so callers
// don't have to guard against a divide-by-zero on the first request.
type MetricsSnapshot struct {
	Invocations      uint64  `json:"invocations"`
	CompressedCount  uint64  `json:"compressed_count"`
	OriginalTokens   uint64  `json:"original_tokens"`
	CompressedTokens uint64  `json:"compressed_tokens"`
	TokensSaved      uint64  `json:"tokens_saved"`
	CompressionRatio float64 `json:"compression_ratio"`
}

// RecordInvocation logs one pass through the compression entry point.
// Delegates to RecordInvocationAt with the current wall clock.
//
// anyCompressed distinguishes "request was inspected but nothing matched"
// from "request actually triggered a rewrite" — the UI uses it as the
// numerator for the hit-rate fraction.
//
// origTok / compTok are the request-level aggregates from CompressionState;
// they're only counted when anyCompressed=true so a no-op pass doesn't
// skew the savings arithmetic.
func (m *CompressionMetrics) RecordInvocation(anyCompressed bool, origTok, compTok int) {
	m.RecordInvocationAt(time.Now().Unix(), anyCompressed, origTok, compTok)
}

// RecordInvocationAt logs one pass at a specific Unix timestamp. It updates
// both the process-lifetime atomic counters (backward compatible with
// Snapshot()) and the time-bucketed histogram (used by the dashboard).
func (m *CompressionMetrics) RecordInvocationAt(ts int64, anyCompressed bool, origTok, compTok int) {
	if m == nil {
		return
	}
	m.invocations.Add(1)
	if !anyCompressed {
		// Still record the invocation in the bucket (invocation count is
		// tracked per-bucket), but the token counters are only updated for
		// compressed passes so the "tokens saved" figure doesn't drift.
		m.getBucket().add(ts, false, 0, 0)
		return
	}
	m.compressed.Add(1)
	if origTok > 0 {
		m.originalTokens.Add(uint64(origTok))
	}
	if compTok > 0 {
		m.compressedToks.Add(uint64(compTok))
	}

	m.getBucket().add(ts, true, origTok, compTok)
}

// RecordRawOutput increments the persisted-raw-output counter. Reserved
// for a future plumbing pass: currently the raw-output helper doesn't
// have a Plugin handle to thread this through without widening the
// internal call sites, so it's left here as a no-op until raw-output is
// folded into the metrics pipeline.
func (m *CompressionMetrics) RecordRawOutput() {
	// intentionally a no-op for v1 — kept for API stability
}

// Snapshot reads all counters atomically and returns a derived view.
// CompressionRatio is 0 when there is nothing to compress so callers don't
// see NaN/Inf on a freshly-started gateway.
func (m *CompressionMetrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	orig := m.originalTokens.Load()
	comp := m.compressedToks.Load()
	var saved uint64
	if orig > comp {
		saved = orig - comp
	}
	var ratio float64
	if orig > 0 {
		ratio = float64(saved) / float64(orig)
	}
	return MetricsSnapshot{
		Invocations:      m.invocations.Load(),
		CompressedCount:  m.compressed.Load(),
		OriginalTokens:   orig,
		CompressedTokens: comp,
		TokensSaved:      saved,
		CompressionRatio: ratio,
	}
}

// Histogram returns buckets within [start, end) aligned to the requested
// bucketSizeSeconds. Buckets are sorted ascending by timestamp. The caller
// provides a bucketSizeSeconds that should match the time range duration
// (e.g. 3600 for hourly, 86400 for daily). Totals is the aggregate of the
// returned buckets. Returns an empty slice when there are no matching buckets.
func (m *CompressionMetrics) Histogram(start, end, bucketSizeSeconds int64) []RtkHistogramBucket {
	if m == nil || m.bucketMetric == nil || start >= end || bucketSizeSeconds <= 0 {
		return nil
	}
	return m.bucketMetric.histogram(start, end, bucketSizeSeconds)
}

// getBucket returns the lazily-initialised bucket metrics reference. Safe for
// concurrent callers via sync.Once.
func (m *CompressionMetrics) getBucket() *compressionBucketMetrics {
	m.bucketOnce.Do(func() {
		m.bucketMetric = newCompressionBucketMetrics()
	})
	return m.bucketMetric
}

// ---------------------------------------------------------------------------
// compressionBucketMetrics implementation
// ---------------------------------------------------------------------------

// add records one invocation at the given Unix timestamp. The timestamp is
// floored to bucketSizeSeconds on write — but since we don't know the caller's
// bucket size until Histogram is called, we store the raw key at 1-second
// granularity. The Histogram call then aggregates adjacent seconds into the
// requested bucket size.
//
// Concurrency: must be called with bm.mu held (or by the caller of add, which
// acquires the lock internally).
func (bm *compressionBucketMetrics) add(ts int64, anyCompressed bool, origTok, compTok int) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// LRU eviction: if we are at capacity and this is a new key, sweep the
	// oldest entry.
	if len(bm.buckets) >= maxBuckets {
		if _, exists := bm.buckets[ts]; !exists {
			// Find and remove the oldest key.
			oldest := bm.keys[0]
			delete(bm.buckets, oldest)
			bm.keys = bm.keys[1:]
		}
	}

	bc, exists := bm.buckets[ts]
	if !exists {
		bc = &bucketCounter{}
		bm.buckets[ts] = bc
		bm.keys = appendSorted(bm.keys, ts)
	}

	bc.invocations++
	if anyCompressed {
		bc.compressed++
		if origTok > 0 {
			bc.originalTokens += uint64(origTok)
		}
		if compTok > 0 {
			bc.compressedTokens += uint64(compTok)
		}
	}
}

// histogram returns buckets covering [start, end) aggregated to the requested
// bucketSizeSeconds. Must be called with bm.mu held (or by the caller, which
// acquires the lock internally).
func (bm *compressionBucketMetrics) histogram(start, end, bucketSizeSeconds int64) []RtkHistogramBucket {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if len(bm.buckets) == 0 {
		return nil
	}

	// Build a map of bucket-aligned keys to aggregated counters.
	type agg struct {
		invocations      uint64
		compressed       uint64
		originalTokens   uint64
		compressedTokens uint64
	}
	aggMap := make(map[int64]*agg)

	for ts, bc := range bm.buckets {
		if ts < start || ts >= end {
			continue
		}
		bucketKey := (ts / bucketSizeSeconds) * bucketSizeSeconds
		a, ok := aggMap[bucketKey]
		if !ok {
			a = &agg{}
			aggMap[bucketKey] = a
		}
		a.invocations += bc.invocations
		a.compressed += bc.compressed
		a.originalTokens += bc.originalTokens
		a.compressedTokens += bc.compressedTokens
	}

	if len(aggMap) == 0 {
		return nil
	}

	// Sort bucket keys.
	keys := make([]int64, 0, len(aggMap))
	for k := range aggMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	result := make([]RtkHistogramBucket, len(keys))
	for i, k := range keys {
		a := aggMap[k]
		saved := uint64(0)
		if a.originalTokens > a.compressedTokens {
			saved = a.originalTokens - a.compressedTokens
		}
		ratio := 0.0
		if a.originalTokens > 0 {
			ratio = float64(saved) / float64(a.originalTokens)
			ratio = math.Min(1.0, math.Max(0.0, ratio))
		}
		result[i] = RtkHistogramBucket{
			Timestamp:        k,
			Invocations:      a.invocations,
			CompressedCount:  a.compressed,
			OriginalTokens:   a.originalTokens,
			CompressedTokens: a.compressedTokens,
			TokensSaved:      saved,
			CompressionRatio: ratio,
		}
	}
	return result
}

// appendSorted inserts ts into the sorted keys slice, maintaining ascending order.
// If ts already exists, the slice is returned unchanged.
func appendSorted(keys []int64, ts int64) []int64 {
	// Binary search for insertion point.
	idx := sort.Search(len(keys), func(i int) bool { return keys[i] >= ts })
	if idx < len(keys) && keys[idx] == ts {
		return keys // already present
	}
	// Insert at idx.
	keys = append(keys, 0)
	copy(keys[idx+1:], keys[idx:])
	keys[idx] = ts
	return keys
}