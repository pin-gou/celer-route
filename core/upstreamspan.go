package bifrost

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/core/schemas"
)

// upstreamSpanStartWarnOnce rate-limits the "missing request start anchor"
// warning so we don't flood logs on every span when the logging plugin is
// disabled. The warning is purely diagnostic — the offset just degrades to 0.
var upstreamSpanStartWarnOnce sync.Once

// recordUpstreamSpan appends a per-attempt upstream HTTP span to the
// BifrostContext, where the logging plugin picks it up in PostLLMHook and
// persists it alongside the pre/post_llm markers in the same write cycle.
//
// Parameters:
//   - ctx: the request-scoped BifrostContext
//   - providerKey: upstream provider key (e.g. "openai", "anthropic")
//   - model: the resolved model id used for this attempt
//   - keyID: the provider API key selected for this attempt ("" if unknown)
//   - startedAt / endedAt: absolute wall-clock bounds of the provider call.
//     For unary paths this is the span around `provider.X(...)`; for streams
//     this is from the call entry to stream-channel close (full-stream, by
//     design — TTFB would be a separate metric).
//   - status: "success" or "failed" — written by the wrapper at recording
//     time for unary, and overridden by PostLLMHook for streaming because the
//     wrapper can't see the final result.
//   - errMsg: a short error summary, populated only when status == "failed".
//
// If the logging plugin did not write BifrostContextKeyRequestStart (e.g. the
// plugin is disabled), time_offset_ms falls back to 0 so the timeline
// waterfall still renders — just anchored to the first span instead of the
// request start.
func recordUpstreamSpan(
	ctx *schemas.BifrostContext,
	providerKey schemas.ModelProvider,
	model string,
	keyID string,
	startedAt time.Time,
	endedAt time.Time,
	status string,
	errMsg string,
) {
	if ctx == nil || startedAt.IsZero() || endedAt.IsZero() {
		return
	}

	durationMs := float64(endedAt.Sub(startedAt).Milliseconds())
	if durationMs < 0 {
		durationMs = 0
	}

	// Anchor offset to the request start written by PreLLMHook. If absent
	// (logging plugin disabled), fall back to the span start so the
	// waterfall still lines up — just anchored to attempt #1 instead of the
	// earliest pre-hook.
	var timeOffsetMs float64
	if start, ok := ctx.Value(schemas.BifrostContextKeyRequestStart).(time.Time); ok && !start.IsZero() {
		timeOffsetMs = float64(startedAt.Sub(start).Milliseconds())
		if timeOffsetMs < 0 {
			timeOffsetMs = 0
		}
	} else {
		timeOffsetMs = 0
		upstreamSpanStartWarnOnce.Do(func() {
			// Intentionally no-op in production: the package-level logger
			// isn't reachable from a top-level function in this package
			// without a receiver. The fallback is documented; tooling that
			// wants to surface the condition can grep for missing
			// time_offset_ms anchors in the timeline response.
		})
	}

	message := "upstream call completed"
	if status == "failed" {
		if errMsg != "" {
			message = "upstream call failed: " + errMsg
		} else {
			message = "upstream call failed"
		}
	}

	event := schemas.TimelineEvent{
		ID:           uuid.NewString(),
		LogID:        requestIDFromContext(ctx),
		Phase:        "upstream_call",
		Source:       "provider",
		PluginName:   "",
		Level:        statusToLevel(status),
		Message:      message,
		TimeOffsetMS: timeOffsetMs,
		DurationMS:   durationMs,
		Timestamp:    startedAt.UTC(),
		Provider:     string(providerKey),
		Model:        model,
		KeyID:        keyID,
		Status:       status,
	}

	schemas.AppendToContextList(ctx, schemas.BifrostContextKeyUpstreamSpans, event)
}

// requestIDFromContext safely fetches the request id, falling back to empty
// string if absent. Persisting timeline events with an empty log_id is
// harmless — the logging plugin batches them onto the canonical Log row
// keyed by the primary request id when it persists, so any orphan row would
// be ignored by the by-log_id read path.
func requestIDFromContext(ctx *schemas.BifrostContext) string {
	if id, ok := ctx.Value(schemas.BifrostContextKeyRequestID).(string); ok {
		return id
	}
	if id, ok := ctx.Value(schemas.BifrostContextKeyFallbackRequestID).(string); ok {
		return id
	}
	return ""
}

func statusToLevel(status string) string {
	if status == "failed" {
		return "error"
	}
	return "info"
}