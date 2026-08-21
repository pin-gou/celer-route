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
package rtk

import "sync/atomic"

// CompressionMetrics is the set of process-lifetime counters that the RTK
// plugin maintains. All fields are atomic so concurrent Pre/PostLLMHook
// calls can update them without locking.
type CompressionMetrics struct {
	invocations    atomic.Uint64 // every call to applyRtkCompression{,Responses}
	compressed     atomic.Uint64 // subset where at least one tool result was actually compressed
	originalTokens atomic.Uint64 // sum of OriginalTokens across compressed requests
	compressedToks atomic.Uint64 // sum of CompressedTokens across compressed requests
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
//
// anyCompressed distinguishes "request was inspected but nothing matched"
// from "request actually triggered a rewrite" — the UI uses it as the
// numerator for the hit-rate fraction.
//
// origTok / compTok are the request-level aggregates from CompressionState;
// they're only counted when anyCompressed=true so a no-op pass doesn't
// skew the savings arithmetic.
func (m *CompressionMetrics) RecordInvocation(anyCompressed bool, origTok, compTok int) {
	if m == nil {
		return
	}
	m.invocations.Add(1)
	if !anyCompressed {
		return
	}
	m.compressed.Add(1)
	if origTok > 0 {
		m.originalTokens.Add(uint64(origTok))
	}
	if compTok > 0 {
		m.compressedToks.Add(uint64(compTok))
	}
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