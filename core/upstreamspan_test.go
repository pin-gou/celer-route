package bifrost

import (
	"context"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// setupSpanTestCtx returns a request-scoped BifrostContext with RequestStart
// anchored at base so recordUpstreamSpan can compute non-zero time_offset_ms.
func setupSpanTestCtx(t *testing.T, base time.Time, withRequestStart bool) *schemas.BifrostContext {
	t.Helper()
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyRequestID, "req-span-test")
	if withRequestStart {
		ctx.SetValue(schemas.BifrostContextKeyRequestStart, base)
	}
	return ctx
}

// TestRecordUpstreamSpan_AppendsToContext verifies a successful unary call
// appends exactly one span to BifrostContextKeyUpstreamSpans with correct
// duration/offset/provider/model/status.
func TestRecordUpstreamSpan_AppendsToContext(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Millisecond)
	ctx := setupSpanTestCtx(t, base, true)

	start := base.Add(100 * time.Millisecond)
	end := start.Add(4500 * time.Millisecond)
	recordUpstreamSpan(ctx, schemas.OpenAI, "gpt-4o", "key-1", start, end, "success", "")

	spans := ctx.Value(schemas.BifrostContextKeyUpstreamSpans)
	got, ok := spans.([]schemas.TimelineEvent)
	if !ok {
		t.Fatalf("spans value = %T, want []schemas.TimelineEvent", spans)
	}
	if len(got) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(got))
	}
	s := got[0]
	if s.Phase != "upstream_call" {
		t.Errorf("phase = %q, want upstream_call", s.Phase)
	}
	if s.Source != "provider" {
		t.Errorf("source = %q, want provider", s.Source)
	}
	if s.TimeOffsetMS != 100.0 {
		t.Errorf("time_offset_ms = %v, want 100", s.TimeOffsetMS)
	}
	if s.DurationMS != 4500.0 {
		t.Errorf("duration_ms = %v, want 4500", s.DurationMS)
	}
	if s.Provider != string(schemas.OpenAI) {
		t.Errorf("provider = %q, want %q", s.Provider, schemas.OpenAI)
	}
	if s.Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", s.Model)
	}
	if s.KeyID != "key-1" {
		t.Errorf("key_id = %q, want key-1", s.KeyID)
	}
	if s.Status != "success" {
		t.Errorf("status = %q, want success", s.Status)
	}
	if s.LogID != "req-span-test" {
		t.Errorf("log_id = %q, want req-span-test", s.LogID)
	}
	if s.ID == "" {
		t.Error("id is empty")
	}
}

// TestRecordUpstreamSpan_FailureStatus verifies a failed call records
// status=failed, level=error and the message carries the error summary.
func TestRecordUpstreamSpan_FailureStatus(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Millisecond)
	ctx := setupSpanTestCtx(t, base, true)

	start := base.Add(1 * time.Second)
	recordUpstreamSpan(ctx, schemas.Anthropic, "claude-3", "key-2", start, start.Add(2*time.Second), "failed", "invalid_request_error HTTP 429")

	got := ctx.Value(schemas.BifrostContextKeyUpstreamSpans).([]schemas.TimelineEvent)
	if len(got) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(got))
	}
	if got[0].Status != "failed" {
		t.Errorf("status = %q, want failed", got[0].Status)
	}
	if got[0].Level != "error" {
		t.Errorf("level = %q, want error", got[0].Level)
	}
	if got[0].Message == "upstream call completed" {
		t.Errorf("message should carry error summary for failed call")
	}
}

// TestRecordUpstreamSpan_NoRequestStartAnchor verifies the degradation path:
// when the logging plugin is disabled and BifrostContextKeyRequestStart is
// absent, the span still records with time_offset_ms=0 (anchored to itself)
// instead of panicking or producing a negative offset.
func TestRecordUpstreamSpan_NoRequestStartAnchor(t *testing.T) {
	ctx := setupSpanTestCtx(t, time.Now(), false)

	start := time.Now().UTC()
	recordUpstreamSpan(ctx, schemas.OpenAI, "gpt-4o", "key-1", start, start.Add(100*time.Millisecond), "success", "")

	got := ctx.Value(schemas.BifrostContextKeyUpstreamSpans).([]schemas.TimelineEvent)
	if len(got) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(got))
	}
	if got[0].TimeOffsetMS != 0 {
		t.Errorf("time_offset_ms = %v, want 0 (anchored at span start)", got[0].TimeOffsetMS)
	}
	if got[0].DurationMS != 100.0 {
		t.Errorf("duration_ms = %v, want 100", got[0].DurationMS)
	}
}

// TestRecordUpstreamSpan_AppendToContextListSetSemantics verifies that
// AppendToContextList's duplicate-skip behavior doesn't drop two distinct
// spans from different attempts (they must have different IDs, so both land).
func TestRecordUpstreamSpan_TwoAttemptsBothAppend(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Millisecond)
	ctx := setupSpanTestCtx(t, base, true)

	recordUpstreamSpan(ctx, schemas.Sensenova, "deepseek-v4-flash", "k1", base.Add(0), base.Add(4*time.Second), "failed", "HTTP 429")
	recordUpstreamSpan(ctx, schemas.Sensenova, "deepseek-v4-flash", "k2", base.Add(5*time.Second), base.Add(8*time.Second), "success", "")

	got := ctx.Value(schemas.BifrostContextKeyUpstreamSpans).([]schemas.TimelineEvent)
	if len(got) != 2 {
		t.Fatalf("len(spans) = %d, want 2", len(got))
	}
	if got[0].KeyID != "k1" || got[1].KeyID != "k2" {
		t.Errorf("key order wrong: %q, %q", got[0].KeyID, got[1].KeyID)
	}
	if got[1].TimeOffsetMS != 5000.0 {
		t.Errorf("second time_offset_ms = %v, want 5000", got[1].TimeOffsetMS)
	}
}

// TestRecordUpstreamSpan_NilContextIsNoop verifies nil ctx doesn't panic.
func TestRecordUpstreamSpan_NilContextIsNoop(t *testing.T) {
	start := time.Now()
	recordUpstreamSpan(nil, schemas.OpenAI, "gpt-4o", "k", start, start.Add(time.Second), "success", "")
}