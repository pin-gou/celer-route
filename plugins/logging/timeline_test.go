package logging

import (
	"context"
	"sync"
	"testing"

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