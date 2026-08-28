package logging

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/logstore"
)

// captureLogger records WARN messages for assertion in degradation tests.
type captureLogger struct {
	mu     sync.Mutex
	warns  []string
	infos  []string
	debugs []string
}

func (l *captureLogger) Debug(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugs = append(l.debugs, format)
}

func (l *captureLogger) Info(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, format)
}

func (l *captureLogger) Warn(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, format)
}

func (l *captureLogger) Error(format string, args ...any) {}
func (l *captureLogger) Fatal(format string, args ...any) {}
func (l *captureLogger) SetLevel(schemas.LogLevel)        {}
func (l *captureLogger) SetOutputType(schemas.LoggerOutputType) {}

func (l *captureLogger) LogHTTPRequest(level schemas.LogLevel, msg string) schemas.LogEventBuilder {
	return schemas.NoopLogEvent
}

func (l *captureLogger) warnCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.warns)
}

// failTimelineStore wraps a LogStore and fails every CreateTimelineEvent call.
// Used to verify that a timeline insert failure degrades to WARN and does not
// block the main log write.
type failTimelineStore struct {
	logstore.LogStore
}

func (s *failTimelineStore) CreateTimelineEvent(ctx context.Context, event *logstore.TimelineEvent) error {
	return assertAnError
}

// errTimelineStore is a sentinel error for timeline insertion failures.
var assertAnError = &timelineTestError{msg: "simulated timeline insert failure"}

type timelineTestError struct{ msg string }

func (e *timelineTestError) Error() string { return e.msg }

// TestPrePostLLMHookWriteTimelineEvents verifies that PreLLMHook and PostLLMHook
// correctly write timeline_events to the store, associated by log_id, with the
// expected phase/plugin_name/level/message fields matching the design spec.
func TestPrePostLLMHookWriteTimelineEvents(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	clog := &captureLogger{}
	plugin, err := Init(context.Background(), &Config{}, clog, store, nil, nil)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plugin.Cleanup(); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	})

	// Create context with a fixed request ID
	requestID := "req-timeline-e2e-1"
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyRequestID, requestID)
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, map[string]string{
		"user-agent": "test-client/1.0",
	})

	// Execute PreLLMHook — this should build the pre_llm timeline event in pending
	_, _, err = plugin.PreLLMHook(ctx, &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o-mini",
			Params:   &schemas.ChatParameters{},
		},
	})
	if err != nil {
		t.Fatalf("PreLLMHook() error = %v", err)
	}

	// Execute PostLLMHook with a success response — this should build the post_llm
	// event and enqueue both events alongside the log entry.
	latency := int64(42)
	result := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType:          schemas.ChatCompletionRequest,
				Provider:             schemas.OpenAI,
				OriginalModelRequested: "gpt-4o-mini",
				ResolvedModelUsed:      "gpt-4o-mini",
				Latency:              latency,
			},
		},
	}
	_, _, err = plugin.PostLLMHook(ctx, result, nil)
	if err != nil {
		t.Fatalf("PostLLMHook() error = %v", err)
	}

	// Cleanup drains the write queue so timeline events are persisted
	if err := plugin.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Verify the Log row was written
	logEntry, err := store.FindByID(context.Background(), requestID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if logEntry.ID != requestID {
		t.Fatalf("log ID = %q, want %q", logEntry.ID, requestID)
	}
	if logEntry.Status != "success" {
		t.Fatalf("log status = %q, want success", logEntry.Status)
	}

	// Verify timeline events were written, associated by log_id
	events, err := store.ListTimelineEventsByLogID(context.Background(), requestID)
	if err != nil {
		t.Fatalf("ListTimelineEventsByLogID() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 timeline events, got %d", len(events))
	}

	// Find pre_llm and post_llm events
	var preLLM, postLLM *logstore.TimelineEvent
	for i := range events {
		switch events[i].Phase {
		case "pre_llm":
			preLLM = &events[i]
		case "post_llm":
			postLLM = &events[i]
		}
	}

	// Assert pre_llm event fields
	if preLLM == nil {
		t.Fatal("pre_llm timeline event not found")
	}
	if preLLM.LogID != requestID {
		t.Fatalf("pre_llm LogID = %q, want %q", preLLM.LogID, requestID)
	}
	if preLLM.Phase != "pre_llm" {
		t.Fatalf("pre_llm phase = %q, want pre_llm", preLLM.Phase)
	}
	if preLLM.Source != "plugin_logging" {
		t.Fatalf("pre_llm source = %q, want plugin_logging", preLLM.Source)
	}
	if preLLM.PluginName != PluginName {
		t.Fatalf("pre_llm plugin_name = %q, want %q", preLLM.PluginName, PluginName)
	}
	if preLLM.Level != "info" {
		t.Fatalf("pre_llm level = %q, want info", preLLM.Level)
	}
	if preLLM.Message != "pre-llm hook executed" {
		t.Fatalf("pre_llm message = %q, want %q", preLLM.Message, "pre-llm hook executed")
	}
	if preLLM.TimeOffsetMS != 0.0 {
		t.Fatalf("pre_llm time_offset_ms = %f, want 0.0", preLLM.TimeOffsetMS)
	}

	// Assert post_llm event fields
	if postLLM == nil {
		t.Fatal("post_llm timeline event not found")
	}
	if postLLM.LogID != requestID {
		t.Fatalf("post_llm LogID = %q, want %q", postLLM.LogID, requestID)
	}
	if postLLM.Phase != "post_llm" {
		t.Fatalf("post_llm phase = %q, want post_llm", postLLM.Phase)
	}
	if postLLM.Source != "plugin_logging" {
		t.Fatalf("post_llm source = %q, want plugin_logging", postLLM.Source)
	}
	if postLLM.PluginName != PluginName {
		t.Fatalf("post_llm plugin_name = %q, want %q", postLLM.PluginName, PluginName)
	}
	if postLLM.Level != "info" {
		t.Fatalf("post_llm level = %q, want info", postLLM.Level)
	}
	if postLLM.Message != "post-llm hook executed" {
		t.Fatalf("post_llm message = %q, want %q", postLLM.Message, "post-llm hook executed")
	}
	if postLLM.TimeOffsetMS < 0 {
		t.Fatalf("post_llm time_offset_ms = %f, want >= 0", postLLM.TimeOffsetMS)
	}

	// Verify events are ordered by phase (pre_llm before post_llm)
	if preLLM.Timestamp.After(postLLM.Timestamp) {
		t.Fatal("pre_llm event timestamp is after post_llm — events should be ordered")
	}
}

// TestTimelineEventInsertFailureDegradesToWarn verifies that when a timeline
// event insert fails (DB write failure), the main request is NOT blocked — the
// Log row should still be written, and the failure should degrade to a WARN log
// message rather than aborting the request.
func TestTimelineEventInsertFailureDegradesToWarn(t *testing.T) {
	baseStore := newTestStore(t)
	defer baseStore.Close(context.Background())

	store := &failTimelineStore{LogStore: baseStore}
	clog := &captureLogger{}
	plugin, err := Init(context.Background(), &Config{}, clog, store, nil, nil)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plugin.Cleanup(); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	})

	requestID := "req-timeline-degraded-1"
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyRequestID, requestID)
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, map[string]string{
		"user-agent": "test-client/1.0",
	})

	// Execute PreLLMHook
	_, _, err = plugin.PreLLMHook(ctx, &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o-mini",
			Params:   &schemas.ChatParameters{},
		},
	})
	if err != nil {
		t.Fatalf("PreLLMHook() error = %v", err)
	}

	// Execute PostLLMHook — the timeline insert will fail, but the main request
	// should not be blocked.
	latency := int64(42)
	result := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType:          schemas.ChatCompletionRequest,
				Provider:             schemas.OpenAI,
				OriginalModelRequested: "gpt-4o-mini",
				ResolvedModelUsed:      "gpt-4o-mini",
				Latency:              latency,
			},
		},
	}
	_, _, err = plugin.PostLLMHook(ctx, result, nil)
	if err != nil {
		t.Fatalf("PostLLMHook() error = %v", err)
	}

	// Cleanup drains the write queue — the timeline insert failure should be
	// logged as WARN but the log entry itself should persist.
	if err := plugin.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Verify the Log row WAS written despite the timeline insert failure
	logEntry, err := baseStore.FindByID(context.Background(), requestID)
	if err != nil {
		t.Fatalf("FindByID() error = %v — the log row should have been written despite timeline insert failure", err)
	}
	if logEntry == nil {
		t.Fatal("log entry is nil — the log row should have been written despite timeline insert failure")
	}
	if logEntry.ID != requestID {
		t.Fatalf("log ID = %q, want %q", logEntry.ID, requestID)
	}
	if logEntry.Status != "success" {
		t.Fatalf("log status = %q, want success", logEntry.Status)
	}

	// Verify that a WARN was logged about the timeline insert failure
	if clog.warnCount() == 0 {
		t.Fatal("expected at least one WARN log about timeline event insert failure, got 0 — the failure should degrade to WARN")
	}

	// Verify timeline events were NOT written (the store rejects them)
	events, err := baseStore.ListTimelineEventsByLogID(context.Background(), requestID)
	if err != nil {
		t.Fatalf("ListTimelineEventsByLogID() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 timeline events (insert was rejected), got %d", len(events))
	}
}

// TestFinalTimelineEventsFallbackUsesStableSpanIDs verifies that when a request
// chains PreLLMHook → PostLLMHook across multiple fallback attempts (each
// attempt has its own LogID), the per-attempt upstream spans recorded by
// core/upstreamspan.go are persisted exactly once — each tied to the LogID of
// the attempt that produced them.
//
// The historical bug: finalTimelineEvents mutated `spans[i].LogID = logID` on
// the slice stored on BifrostContextKeyUpstreamSpans (a slice shared across
// all fallback attempts of the same request). The first attempt stamped the
// span's LogID to fb-1 and persisted it; the second attempt then re-stamped the
// same span to fb-2 and tried to persist it again. SQLite/Postgres both
// rejected the second attempt with "UNIQUE constraint failed: timeline_events.id"
// / "duplicate key value violates unique constraint timeline_events_pkey" —
// the row already exists, just under a different log_id, and the WARN
// "failed to insert timeline event" surfaced per duplicate.
//
// The fix replaces the in-place mutation with a value copy before adjusting
// LogID/Status, and clears the context key so each attempt claims its spans
// instead of stealing them from later attempts.
func TestFinalTimelineEventsFallbackUsesStableSpanIDs(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	clog := &captureLogger{}
	plugin, err := Init(context.Background(), &Config{}, clog, store, nil, nil)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plugin.Cleanup(); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	})

	// Simulate the upstream spans core/upstreamspan.go writes. They share a
	// backing slice via BifrostContext (each AppendToContextList appends to
	// the same slice). Each span has a fresh UUID and an initial LogID
	// stamped at the time the attempt ran.
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyRequestID, "primary-id")
	spanID1 := uuid.NewString()
	spanID2 := uuid.NewString()
	spans := []schemas.TimelineEvent{
		{ID: spanID1, Phase: "upstream_call", Source: "provider", Status: "success", LogID: "fb-1"},
		{ID: spanID2, Phase: "upstream_call", Source: "provider", Status: "failed", LogID: "fb-2"},
	}
	ctx.SetValue(schemas.BifrostContextKeyUpstreamSpans, spans)

	// Fallback attempt 1: pending.RequestID = fb-1.
	fb1Pending := &PendingLogData{
		RequestID: "fb-1",
		Timestamp: time.Now().Add(-1 * time.Second),
		InitialData: &InitialLogData{
			Object: string(schemas.ChatCompletionRequest),
		},
		TimelineEvents: []*logstore.TimelineEvent{
			{ID: uuid.NewString(), LogID: "fb-1", Phase: "pre_llm"},
		},
		Ctx: ctx,
	}

	// Fallback attempt 2: pending.RequestID = fb-2. Same ctx, same spans
	// slice (with both span IDs already set). A new pre_llm marker.
	fb2Pending := &PendingLogData{
		RequestID: "fb-2",
		Timestamp: time.Now().Add(-1 * time.Second),
		InitialData: &InitialLogData{
			Object: string(schemas.ChatCompletionRequest),
		},
		TimelineEvents: []*logstore.TimelineEvent{
			{ID: uuid.NewString(), LogID: "fb-2", Phase: "pre_llm"},
		},
		Ctx: ctx,
	}

	enqueue := func(p *LoggerPlugin, pending *PendingLogData) {
		// Materialize a real Log row first (BatchUpsert is the parent
		// log write, and CreateTimelineEvent tolerates orphan rows but
		// having the parent present matches the production flow).
		entry := &logstore.Log{
			ID:        pending.RequestID,
			Provider:  "openai",
			Object:    string(schemas.ChatCompletionRequest),
			Status:    "error",
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
		}
		if err := store.BatchUpsert(context.Background(), []*logstore.Log{entry}); err != nil {
			t.Fatalf("BatchUpsert log %s: %v", entry.ID, err)
		}
		bifrostErr := &schemas.BifrostError{
			Error:          &schemas.ErrorField{Message: "synthesized 429"},
			IsBifrostError: true,
		}
		events := p.finalTimelineEvents(pending, entry.ID, bifrostErr)
		p.enqueueLogEntry(entry, nil, events...)
	}

	enqueue(plugin, fb1Pending)
	enqueue(plugin, fb2Pending)

	if err := plugin.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// No duplicate-PK warnings should have been logged by processBatch.
	for _, msg := range clog.warns {
		if strings.Contains(msg, "duplicate key") ||
			strings.Contains(msg, "timeline_events_pkey") ||
			strings.Contains(msg, "failed to insert timeline event") {
			t.Errorf("unexpected duplicate-key warning from processBatch: %s", msg)
		}
	}
}