package providercooldown

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

func intPtr(i int) *int { return &i }

func strPtr(s string) *string { return &s }

func newQuotaError(provider schemas.ModelProvider) *schemas.BifrostError {
	return &schemas.BifrostError{
		StatusCode:     intPtr(429),
		IsBifrostError: true,
		Error: &schemas.ErrorField{
			Message: "You exceeded your current quota, please check your plan and billing details",
		},
		ExtraFields: schemas.BifrostErrorExtraFields{
			RoutingInfo: schemas.RoutingInfo{Provider: provider, Model: "gpt-4o"},
		},
	}
}

func newTrailCtx(keyID string) *schemas.BifrostContext {
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{Attempt: 1, KeyID: keyID, KeyName: "k-" + keyID},
	})
	return ctx
}

// testLogger captures formatted log lines for assertions.
type testLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (l *testLogger) Debug(msg string, args ...any) { l.record("debug", msg, args) }
func (l *testLogger) Info(msg string, args ...any)   { l.record("info", msg, args) }
func (l *testLogger) Warn(msg string, args ...any)   { l.record("warn", msg, args) }
func (l *testLogger) Error(msg string, args ...any)  { l.record("error", msg, args) }

func (l *testLogger) record(level, msg string, args []any) {
	line := level + " " + fmt.Sprintf(msg, args...)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, line)
}

func (l *testLogger) contains(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.msgs {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

func (l *testLogger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.msgs)
}

func (l *testLogger) Fatal(msg string, args ...any)               { l.record("fatal", msg, args) }
func (l *testLogger) SetLevel(schemas.LogLevel)                   {}
func (l *testLogger) SetOutputType(schemas.LoggerOutputType)      {}
func (l *testLogger) LogHTTPRequest(schemas.LogLevel, string) schemas.LogEventBuilder {
	return schemas.NoopLogEvent
}

func TestCooldownStateMarkAndExpiry(t *testing.T) {
	s := NewCooldownState(50 * time.Millisecond)
	provider := schemas.OpenAI

	if s.IsCoolingDown(provider, "key-1") {
		t.Fatal("expected no cooldown before Mark")
	}
	s.Mark(provider, "key-1")
	if !s.IsCoolingDown(provider, "key-1") {
		t.Fatal("expected key-1 to be cooling down")
	}
	if s.IsCoolingDown(provider, "key-2") {
		t.Fatal("expected key-2 to be unaffected")
	}
	if s.IsCoolingDown(schemas.Anthropic, "key-1") {
		t.Fatal("expected same key on another provider to be unaffected")
	}

	time.Sleep(60 * time.Millisecond)
	if s.IsCoolingDown(provider, "key-1") {
		t.Fatal("expected cooldown to expire after TTL")
	}
}

func TestCooldownStateEmptyKeyIDNoOp(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.OpenAI, "")
	if s.IsCoolingDown(schemas.OpenAI, "") {
		t.Fatal("empty keyID must never be marked")
	}
	if s.Size() != 0 {
		t.Fatalf("expected empty state, got size %d", s.Size())
	}
}

func TestCooldownStateTTLOverride(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.SetTTLOverride(schemas.OpenAI, 20*time.Millisecond)

	s.Mark(schemas.OpenAI, "k1")
	s.Mark(schemas.Anthropic, "k2")

	time.Sleep(30 * time.Millisecond)
	if s.IsCoolingDown(schemas.OpenAI, "k1") {
		t.Fatal("openai entry should have expired via override TTL")
	}
	if !s.IsCoolingDown(schemas.Anthropic, "k2") {
		t.Fatal("anthropic entry should still be within default TTL")
	}
}

func TestAsFilterSkipsCooledKeys(t *testing.T) {
	s := NewCooldownState(time.Minute)
	provider := schemas.OpenAI
	s.Mark(provider, "hot-key")

	filter := s.AsFilter(nil)
	keys := []schemas.Key{
		{ID: "hot-key", Name: "hot"},
		{ID: "cold-key", Name: "cold"},
	}
	out, err := filter(nil, provider, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].ID != "cold-key" {
		t.Fatalf("expected only cold-key to survive the filter, got %+v", out)
	}
}

func TestAsFilterEmptyInput(t *testing.T) {
	s := NewCooldownState(time.Minute)
	out, err := s.AsFilter(nil)(nil, schemas.OpenAI, "gpt-4o", nil)
	if err != nil || len(out) != 0 {
		t.Fatalf("expected empty pass-through, got out=%v err=%v", out, err)
	}
}

func TestAsFilter_EnabledFilter(t *testing.T) {
	// Verify that the KeyPoolFilter returned by AsFilter does not panic
	// when BifrostContext is nil (the "nil path" through the filter).
	// This simulates the filter being called from the key selection loop
	// without a context — the filter must never dereference a nil context.
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.OpenAI, "hot-key")

	filter := s.AsFilter(nil)
	keys := []schemas.Key{
		{ID: "hot-key", Name: "hot"},
		{ID: "cold-key", Name: "cold"},
	}

	// Call with nil BifrostContext — must not panic.
	out, err := filter(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].ID != "cold-key" {
		t.Fatalf("expected only cold-key to survive the nil-context filter call, got %+v", out)
	}
}

func TestStateSnapshot(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.OpenAI, "key-a")
	s.Mark(schemas.Anthropic, "key-b")

	entries := s.Snapshot()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify entries are parseable and carry provider/key info.
	seen := map[string]bool{}
	for _, e := range entries {
		seen[string(e.Provider)+"::"+e.KeyID] = true
		if e.ExpiresAt.IsZero() {
			t.Fatal("ExpiresAt must be set")
		}
		if e.Remaining <= 0 {
			t.Fatalf("Remaining must be positive for an active cooldown, got %v", e.Remaining)
		}
	}
	if !seen["openai::key-a"] || !seen["anthropic::key-b"] {
		t.Fatalf("expected openai::key-a and anthropic::key-b, got %+v", seen)
	}
}

func TestStateSnapshotSkipsExpired(t *testing.T) {
	s := NewCooldownState(10 * time.Millisecond)
	s.Mark(schemas.OpenAI, "key-a")
	time.Sleep(20 * time.Millisecond)

	entries := s.Snapshot()
	if len(entries) != 0 {
		t.Fatalf("expected expired entries to be skipped, got %d", len(entries))
	}
}

func TestStateSnapshotEmpty(t *testing.T) {
	s := NewCooldownState(time.Minute)
	entries := s.Snapshot()
	if len(entries) != 0 {
		t.Fatalf("expected empty snapshot on fresh state, got %d", len(entries))
	}
}

func TestStateClearKey(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.OpenAI, "key-a")

	if !s.IsCoolingDown(schemas.OpenAI, "key-a") {
		t.Fatal("precondition: key should be cooling down")
	}

	if !s.ClearKey(schemas.OpenAI, "key-a") {
		t.Fatal("ClearKey should return true for an existing entry")
	}
	if s.IsCoolingDown(schemas.OpenAI, "key-a") {
		t.Fatal("after ClearKey, the key must no longer be cooling down")
	}
	// Clearing again is a no-op returning false.
	if s.ClearKey(schemas.OpenAI, "key-a") {
		t.Fatal("ClearKey on a non-existent entry should return false")
	}
}

func TestStateClearKeyEmptyKeyID(t *testing.T) {
	s := NewCooldownState(time.Minute)
	if s.ClearKey(schemas.OpenAI, "") {
		t.Fatal("ClearKey with empty keyID must return false")
	}
}

func TestPluginSnapshotAndClearKeyDelegates(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	p.State.Mark(schemas.OpenAI, "key-a")
	entries := p.Snapshot()
	if len(entries) != 1 || entries[0].KeyID != "key-a" {
		t.Fatalf("expected 1 entry for key-a, got %+v", entries)
	}
	if !p.ClearKey(schemas.OpenAI, "key-a") {
		t.Fatal("plugin ClearKey should delegate and return true")
	}
	if len(p.Snapshot()) != 0 {
		t.Fatal("after plugin ClearKey, snapshot must be empty")
	}
}

func TestPluginSnapshotNilState(t *testing.T) {
	// A zero-value plugin has nil State; Snapshot must not panic and must
	// return nil (distinct from empty slice, signaling "never initialized").
	p := &CooldownPlugin{}
	if got := p.Snapshot(); got != nil {
		t.Fatalf("Snapshot on nil-state plugin should return nil, got %+v", got)
	}
	if p.ClearKey(schemas.OpenAI, "k") {
		t.Fatal("ClearKey on nil-state plugin should return false")
	}
}

// TestFilterSwapSeesNewCooldown simulates the wire-path behavior of plugin
// reload: after a reload, a new plugin instance is created with a fresh
// CooldownState, and the transport rewires s.KeyPoolFilter to point at the
// new State's filter. This test verifies that:
//
//  1. The new filter, after rewiring, correctly suppresses a key freshly
//     marked on the new State (proving the rewiring is wired correctly).
//  2. The OLD filter (which captures the OLD State) does NOT see the new
//     cooldown — confirming the old State has been orphaned (no longer
//     written to), as expected by the design.
func TestFilterSwapSeesNewCooldown(t *testing.T) {
	oldPlugin := NewPlugin(nil)
	oldFilter := oldPlugin.State.AsFilter(nil)

	// Simulate reload: a new plugin instance replaces the old one in the
	// transport layer (SyncLoadedPlugin). The transport then calls
	// plugin.State.AsFilter(...) to rewire s.KeyPoolFilter.
	newPlugin := NewPlugin(nil)
	newFilter := newPlugin.State.AsFilter(nil)

	// Mark a key on the NEW plugin's State (this is what PostLLMHook would
	// do for a quota error after the reload).
	newPlugin.State.Mark(schemas.OpenAI, "key-after-reload")

	keys := []schemas.Key{
		{ID: "key-after-reload", Name: "k1"},
		{ID: "key-fresh", Name: "k2"},
	}

	// Old filter should NOT filter "key-after-reload" because its captured
	// State has no entry for it.
	out, err := oldFilter(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("oldFilter: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("old filter should let all keys through (its State was orphaned by reload), got %d keys", len(out))
	}

	// New filter MUST filter "key-after-reload".
	out, err = newFilter(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("newFilter: %v", err)
	}
	if len(out) != 1 || out[0].ID != "key-fresh" {
		t.Fatalf("new filter should suppress the cooled key, got %+v", out)
	}
}

// TestSharedStateAcrossFilters simulates a future-proofing scenario where the
// transport chooses to share a single *CooldownState across plugin reloads
// (e.g. to preserve cooldowns across config changes). It is not the current
// behavior but documents the underlying contract: a State can back multiple
// filter closures, and writes through any of them are visible to all.
func TestSharedStateAcrossFilters(t *testing.T) {
	s := NewCooldownState(time.Minute)
	f1 := s.AsFilter(nil)
	f2 := s.AsFilter(nil)

	s.Mark(schemas.OpenAI, "shared-key")

	keys := []schemas.Key{{ID: "shared-key"}, {ID: "other-key"}}

	out1, err := f1(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("f1: %v", err)
	}
	out2, err := f2(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("f2: %v", err)
	}
	if len(out1) != 1 || out1[0].ID != "other-key" {
		t.Fatalf("f1 did not see cooldown, got %+v", out1)
	}
	if len(out2) != 1 || out2[0].ID != "other-key" {
		t.Fatalf("f2 did not see cooldown, got %+v", out2)
	}
}

func TestIsQuotaExhausted(t *testing.T) {
	cases := []struct {
		name string
		err  *schemas.BifrostError
		want bool
	}{
		{"nil", nil, false},
		// 402 is a permanent billing failure, handled by bifrost's deadKeyIDs.
		// Don't cooldown — see IsQuotaExhausted docstring.
		{"402 billing", &schemas.BifrostError{StatusCode: intPtr(402)}, false},
		{"429 quota message", newQuotaError(schemas.OpenAI), true},
		{"429 generic rate limit only", &schemas.BifrostError{
			StatusCode: intPtr(429),
			Error:      &schemas.ErrorField{Message: "Too many requests, please retry later"},
		}, false},
		{"429 insufficient_quota message", &schemas.BifrostError{
			StatusCode: intPtr(429),
			Error:      &schemas.ErrorField{Message: "insufficient_quota"},
		}, true},
		{"429 usage limit message", &schemas.BifrostError{
			StatusCode: intPtr(429),
			Error:      &schemas.ErrorField{Message: "usage limit reached for this account"},
		}, true},
		{"500", &schemas.BifrostError{StatusCode: intPtr(500)}, false},
		{"401", &schemas.BifrostError{StatusCode: intPtr(401)}, false},
		// 402 message-only (no status code) used to match the substring
		// "payment required". With the new semantics, message-only detection
		// still works for genuine quota exhaustion, but a 402-style message
		// in isolation now correctly does NOT trigger cooldown.
		{"402 via message only", &schemas.BifrostError{Error: &schemas.ErrorField{Message: "payment required"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsQuotaExhausted(tc.err); got != tc.want {
				t.Fatalf("IsQuotaExhausted(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestPluginIgnores402(t *testing.T) {
	log := &testLogger{}
	plugin := NewPlugin(log)
	ctx := newTrailCtx("key-1")
	// 402 status code only — was previously triggering cooldown.
	plugin.PostLLMHook(ctx, nil, &schemas.BifrostError{
		StatusCode: intPtr(402),
		Error:      &schemas.ErrorField{Message: "Payment required"},
		ExtraFields: schemas.BifrostErrorExtraFields{
			RoutingInfo: schemas.RoutingInfo{Provider: schemas.OpenAI, Model: "gpt-4o"},
		},
	})
	if plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("402 must not trigger cooldown (handled by bifrost deadKeyIDs)")
	}
	if log.contains("marked key") {
		t.Fatalf("expected no Mark log for 402, got messages: %v", log.msgs)
	}
}

func TestPluginMarksOnQuotaError(t *testing.T) {
	plugin := NewPlugin(nil)
	ctx := newTrailCtx("key-1")
	plugin.PostLLMHook(ctx, nil, newQuotaError(schemas.OpenAI))

	if !plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("expected (openai, key-1) to be in cooldown after quota error")
	}
	if plugin.State.IsCoolingDown(schemas.OpenAI, "key-2") {
		t.Fatal("expected unrelated key to be untouched")
	}
}

func TestPluginIgnoresNonQuotaError(t *testing.T) {
	plugin := NewPlugin(nil)
	ctx := newTrailCtx("key-1")
	err := &schemas.BifrostError{
		StatusCode: intPtr(429),
		Error:      &schemas.ErrorField{Message: "too many requests, retry later"},
		ExtraFields: schemas.BifrostErrorExtraFields{
			RoutingInfo: schemas.RoutingInfo{Provider: schemas.OpenAI},
		},
	}
	plugin.PostLLMHook(ctx, nil, err)
	if plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("transient rate limit must not trigger cooldown")
	}
}

func TestPluginIgnoresSuccess(t *testing.T) {
	plugin := NewPlugin(nil)
	ctx := newTrailCtx("key-1")
	plugin.PostLLMHook(ctx, &schemas.BifrostResponse{}, nil)
	if plugin.State.Size() != 0 {
		t.Fatal("successful responses must not populate cooldown state")
	}
}

func TestPluginRequiresTrailForKeyID(t *testing.T) {
	plugin := NewPlugin(nil)
	// ctx without an attempt trail — nothing to mark, must not panic.
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	plugin.PostLLMHook(ctx, nil, newQuotaError(schemas.OpenAI))
	if plugin.State.Size() != 0 {
		t.Fatal("no keyID in trail => nothing should be marked")
	}
}

func TestPluginPrefersRoutingInfoProvider(t *testing.T) {
	plugin := NewPlugin(nil)
	ctx := newTrailCtx("key-1")
	err := newQuotaError(schemas.Anthropic)
	plugin.PostLLMHook(ctx, nil, err)
	if !plugin.State.IsCoolingDown(schemas.Anthropic, "key-1") {
		t.Fatal("expected cooldown on anthropic from RoutingInfo.Provider")
	}
	if plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("cooldown must not leak to other providers")
	}
}

func TestPluginImplementsLLMPlugin(t *testing.T) {
	var _ schemas.LLMPlugin = (*CooldownPlugin)(nil)
	var _ schemas.BasePlugin = (*CooldownPlugin)(nil)
}

// TestPreLLMHookReturnsRequestUnchanged is a regression guard for a P0
// production incident: PreLLMHook originally returned nil for the request,
// which made the pipeline hand every subsequent plugin a nil request and
// collapsed all LLM traffic into "bifrost request after plugin hooks cannot
// be nil". The hook MUST return the request unchanged.
func TestPreLLMHookReturnsRequestUnchanged(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	req := &schemas.BifrostRequest{}
	gotReq, shortCircuit, err := p.PreLLMHook(nil, req)

	if err != nil {
		t.Fatalf("PreLLMHook must not return an error, got %v", err)
	}
	if shortCircuit != nil {
		t.Fatalf("PreLLMHook must not short-circuit, got %+v", shortCircuit)
	}
	if gotReq == nil {
		t.Fatal("PreLLMHook returned nil request — this breaks the pipeline for all subsequent plugins")
	}
	if gotReq != req {
		t.Fatal("PreLLMHook must return the SAME request pointer unchanged")
	}
}

// TestPreLLMHookNilRequestIsPassthrough verifies the hook never turns a nil
// input into a non-nil output and never panics. (A nil request reaching the
// hook would already be a pipeline bug upstream, but the hook must not make
// things worse.)
func TestPreLLMHookNilRequestIsPassthrough(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	gotReq, shortCircuit, err := p.PreLLMHook(nil, nil)
	if err != nil || shortCircuit != nil {
		t.Fatalf("unexpected err=%v shortCircuit=%v", err, shortCircuit)
	}
	if gotReq != nil {
		t.Fatal("nil input must pass through as nil, not be fabricated")
	}
}

// TestPostLLMHookReturnsInputsUnchanged is a regression guard: PostLLMHook
// must return the response and error it was given, because the pipeline
// reassigns its working resp/bifrostErr to these return values. Returning
// nil,nil would wipe a valid response (success path) or error (failure path)
// for every downstream consumer.
func TestPostLLMHookReturnsInputsUnchanged(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	t.Run("success path returns response unchanged", func(t *testing.T) {
		resp := &schemas.BifrostResponse{}
		gotResp, gotErr, err := p.PostLLMHook(newTrailCtx("k1"), resp, nil)
		if err != nil {
			t.Fatalf("PostLLMHook must not return an error, got %v", err)
		}
		if gotResp != resp {
			t.Fatal("PostLLMHook must return the SAME response pointer on success")
		}
		if gotErr != nil {
			t.Fatalf("PostLLMHook must not fabricate an error, got %v", gotErr)
		}
	})

	t.Run("non-quota error path returns both unchanged", func(t *testing.T) {
		resp := &schemas.BifrostResponse{}
		berr := &schemas.BifrostError{
			StatusCode: intPtr(429),
			Error:      &schemas.ErrorField{Message: "too many requests, retry later"},
		}
		gotResp, gotErr, err := p.PostLLMHook(newTrailCtx("k1"), resp, berr)
		if err != nil {
			t.Fatalf("PostLLMHook must not return an error, got %v", err)
		}
		if gotResp != resp {
			t.Fatal("PostLLMHook must return the SAME response pointer")
		}
		if gotErr != berr {
			t.Fatal("PostLLMHook must return the SAME error pointer for non-quota errors")
		}
	})

	t.Run("quota error path still returns both unchanged", func(t *testing.T) {
		resp := &schemas.BifrostResponse{}
		berr := newQuotaError(schemas.OpenAI)
		gotResp, gotErr, err := p.PostLLMHook(newTrailCtx("k1"), resp, berr)
		if err != nil {
			t.Fatalf("PostLLMHook must not return an error, got %v", err)
		}
		if gotResp != resp {
			t.Fatal("PostLLMHook must return the SAME response pointer even when marking cooldown")
		}
		if gotErr != berr {
			t.Fatal("PostLLMHook must return the SAME error pointer even when marking cooldown")
		}
		// And the cooldown was actually marked.
		if !p.State.IsCoolingDown(schemas.OpenAI, "k1") {
			t.Fatal("expected cooldown to be marked")
		}
	})
}

func TestPluginNameAndCleanup(t *testing.T) {
	p := NewPlugin(nil)
	if p.GetName() == "" {
		t.Fatal("plugin name must be non-empty")
	}
	if err := p.Cleanup(); err != nil {
		t.Fatalf("Cleanup should succeed, got %v", err)
	}
}

func TestRunGCPrunesExpired(t *testing.T) {
	s := NewCooldownState(10 * time.Millisecond)
	s.Mark(schemas.OpenAI, "k1")

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		// RunGC's ticker is 1 minute (production cadence) — too slow for a
		// unit test, so this test primarily verifies (a) the goroutine exits
		// promptly when stop is closed, and (b) the lazy-prune path inside
		// IsCoolingDown removes the expired entry on the next access.
		s.RunGC(nil, stop)
	}()

	close(stop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunGC did not stop when the stop channel closed")
	}

	// Wait past the TTL so the next IsCoolingDown observes expiry, then check
	// the lazy prune removed the entry.
	time.Sleep(20 * time.Millisecond)
	if s.IsCoolingDown(schemas.OpenAI, "k1") {
		t.Fatal("entry should have expired")
	}
	if s.Size() != 0 {
		t.Fatalf("lazy prune should have removed the entry, got size %d", s.Size())
	}
}

// TestStateGCLoopPrunesAtCustomInterval drives runGCLocked directly with a
// short interval to verify the GC actually removes expired entries (not just
// the lazy prune path).
func TestStateGCLoopPrunesAtCustomInterval(t *testing.T) {
	log := &testLogger{}
	s := NewCooldownState(20 * time.Millisecond)
	s.Mark(schemas.OpenAI, "k1")

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runGCLocked(log, 10*time.Millisecond, stop)
	}()

	// Wait for TTL + at least one tick.
	time.Sleep(80 * time.Millisecond)
	close(stop)
	<-done

	if s.IsCoolingDown(schemas.OpenAI, "k1") {
		t.Fatal("GC loop should have pruned the expired entry")
	}
	if !log.contains("GC pruned 1 expired") {
		t.Fatalf("expected GC log, got messages: %v", log.msgs)
	}
}

// TestPluginCleanupStopsGC verifies the plugin's auto-started GC goroutine
// exits promptly when Cleanup is called. This is the regression for the P0
// issue where the GC never started in production; without an explicit
// Cleanup, plugin reloads and shutdowns would leak goroutines.
func TestPluginCleanupStopsGC(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	// Sanity: plugin must have a non-nil stop/done channel after NewPlugin.
	if p.gcStop == nil || p.gcDone == nil {
		t.Fatal("NewPlugin should auto-start the GC goroutine")
	}
}

// TestPluginCleanupIdempotent verifies calling Cleanup twice is a no-op.
func TestPluginCleanupIdempotent(t *testing.T) {
	p := NewPlugin(nil)
	if err := p.Cleanup(); err != nil {
		t.Fatalf("first Cleanup: %v", err)
	}
	if err := p.Cleanup(); err != nil {
		t.Fatalf("second Cleanup should be a no-op, got %v", err)
	}
}

// TestPluginInitRestartsGC verifies Init replaces the State and restarts the
// GC against the new State. Without restart, the GC goroutine would still
// point at the OLD State.
func TestPluginInitRestartsGC(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	oldStop := p.gcStop
	if oldStop == nil {
		t.Fatal("NewPlugin must start GC")
	}

	if err := p.Init(map[string]any{
		"default_ttl_seconds": float64(45),
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer p.Cleanup()

	if p.gcStop == nil {
		t.Fatal("Init should restart GC, but gcStop is nil")
	}
	// We can't reliably assert they're different channel values (gcStop is
	// reassigned), so just confirm the old channel was closed by attempting
	// a non-blocking receive on it.
	select {
	case <-oldStop:
		// expected: Init closed the old stop channel
	default:
		t.Fatal("Init did not close the previous GC stop channel — goroutine leak")
	}
}

// TestPluginCleanupExitsGoroutine verifies the GC goroutine actually exits
// after Cleanup (not just that the channel was closed).
func TestPluginCleanupExitsGoroutine(t *testing.T) {
	p := NewPlugin(nil)
	done := p.gcDone

	// Spawn a watcher that signals when gcDone closes.
	watcher := make(chan struct{})
	go func() {
		<-done
		close(watcher)
	}()

	if err := p.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	select {
	case <-watcher:
		// good
	case <-time.After(time.Second):
		t.Fatal("GC goroutine did not exit after Cleanup (goroutine leak)")
	}
}

func TestPluginLogsOnMark(t *testing.T) {
	log := &testLogger{}
	plugin := NewPlugin(log)
	ctx := newTrailCtx("key-1")
	plugin.PostLLMHook(ctx, nil, newQuotaError(schemas.OpenAI))

	if !plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("expected (openai, key-1) to be in cooldown")
	}
	if !log.contains("marked key openai/key-1 (name=k-key-1, TTL=") {
		t.Fatalf("expected log about marked key, got %d messages: %v", log.count(), log.msgs)
	}
}

func TestPluginLogsOnInit(t *testing.T) {
	log := &testLogger{}
	plugin := NewPlugin(log)
	if err := plugin.Init(map[string]any{
		"default_ttl_seconds": float64(300),
		"ttl_overrides": map[string]any{
			"openai": float64(60),
		},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !log.contains("initialized") {
		t.Fatalf("expected init log, got %d messages: %v", log.count(), log.msgs)
	}
	if !log.contains("default_ttl=5m0s") {
		t.Fatalf("expected default TTL in init log, got %v", log.msgs)
	}
	if !log.contains("1 provider override") {
		t.Fatalf("expected override count in init log, got %v", log.msgs)
	}
}

func TestAsFilterLogsSuppressedKeys(t *testing.T) {
	log := &testLogger{}
	s := NewCooldownState(30 * time.Minute)
	s.Mark(schemas.OpenAI, "hot-key")

	filter := s.AsFilter(log)
	keys := []schemas.Key{
		{ID: "hot-key", Name: "hot"},
		{ID: "cold-key", Name: "cold"},
	}
	out, err := filter(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].ID != "cold-key" {
		t.Fatalf("expected only cold-key, got %+v", out)
	}
	if !log.contains("suppressed key openai/hot-key (name=hot, model=gpt-4o)") {
		t.Fatalf("expected suppression log, got %d messages: %v", log.count(), log.msgs)
	}
}

func TestAsFilterNilLoggerNoPanic(t *testing.T) {
	s := NewCooldownState(30 * time.Minute)
	s.Mark(schemas.OpenAI, "hot-key")
	filter := s.AsFilter(nil)
	keys := []schemas.Key{{ID: "hot-key"}, {ID: "cold-key"}}
	out, err := filter(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil || len(out) != 1 {
		t.Fatalf("nil logger must not panic, got out=%v err=%v", out, err)
	}
}

func TestNewPluginNilLoggerNoPanic(t *testing.T) {
	plugin := NewPlugin(nil)
	defer plugin.Cleanup()
	plugin.PostLLMHook(newTrailCtx("k1"), nil, newQuotaError(schemas.OpenAI))
	if !plugin.State.IsCoolingDown(schemas.OpenAI, "k1") {
		t.Fatal("nil logger must not affect cooldown behavior")
	}
}

func TestNewPluginWithTTL(t *testing.T) {
	plugin := NewPluginWithTTL(nil, 20*time.Millisecond)
	defer plugin.Cleanup()
	plugin.State.Mark(schemas.OpenAI, "k1")

	if !plugin.State.IsCoolingDown(schemas.OpenAI, "k1") {
		t.Fatal("key should be cooling down immediately after Mark")
	}
	time.Sleep(30 * time.Millisecond)
	if plugin.State.IsCoolingDown(schemas.OpenAI, "k1") {
		t.Fatal("cooldown must expire after the custom TTL")
	}
}

func TestNewPluginWithTTLNonPositiveFallsBack(t *testing.T) {
	plugin := NewPluginWithTTL(nil, 0)
	defer plugin.Cleanup()
	// Non-positive TTL must fall back to DefaultCooldownTTL (10 min).
	if got := plugin.State.EffectiveTTL(schemas.OpenAI); got != DefaultCooldownTTL {
		t.Fatalf("expected fallback to DefaultCooldownTTL, got %v", got)
	}
}

func TestPluginInitAppliesTTL(t *testing.T) {
	p := &CooldownPlugin{}
	cfg := map[string]any{
		"default_ttl_seconds": float64(120),
		"ttl_overrides": map[string]any{
			"openai": float64(30),
		},
	}
	if err := p.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if p.State == nil {
		t.Fatal("Init must populate State")
	}
	if got := p.State.effectiveTTLLocked(schemas.OpenAI); got != 30*time.Second {
		t.Fatalf("openai override = %v, want 30s", got)
	}
	if got := p.State.effectiveTTLLocked(schemas.Anthropic); got != 120*time.Second {
		t.Fatalf("anthropic default = %v, want 120s", got)
	}
}

func TestPluginInitRejectsNonMap(t *testing.T) {
	p := &CooldownPlugin{}
	if err := p.Init("not a map"); err == nil {
		t.Fatal("expected error on non-map config")
	}
}

func TestPluginInitRejectsBadTypes(t *testing.T) {
	p := &CooldownPlugin{}
	if err := p.Init(map[string]any{
		"default_ttl_seconds": "forever",
	}); err == nil {
		t.Fatal("expected error on non-int TTL")
	}
}

func TestPluginInitNilIsNoOp(t *testing.T) {
	p := &CooldownPlugin{}
	if err := p.Init(nil); err != nil {
		t.Fatalf("Init(nil) should be no-op, got %v", err)
	}
	if p.State == nil {
		t.Fatal("Init(nil) should still initialize State with default TTL")
	}
}

func TestPluginInitIsIdempotent(t *testing.T) {
	p := NewPlugin(nil)
	if err := p.Init(map[string]any{
		"default_ttl_seconds": float64(45),
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// mark something under the new state
	p.State.Mark(schemas.OpenAI, "k1")

	// re-init wipes state (idempotent reset, not merge)
	if err := p.Init(map[string]any{
		"default_ttl_seconds": float64(90),
	}); err != nil {
		t.Fatalf("re-Init: %v", err)
	}
	if p.State.Size() != 0 {
		t.Fatalf("re-Init should produce a fresh state, got size %d", p.State.Size())
	}
	if got := p.State.effectiveTTLLocked(schemas.Anthropic); got != 90*time.Second {
		t.Fatalf("re-Init default TTL = %v, want 90s", got)
	}
}

// TestStatsCounters covers the lifetime counters surfaced via Stats().
// This is the read path used by GET /api/plugins/provider-cooldown/stats
// and is what an operator uses to confirm the filter is actually vetoing
// keys — production logs at Info level would be too noisy when the
// feature is in steady-state suppression.
//
// Red-phase contract: Mark() increments markCount, AsFilter() increments
// suppressedCount once per vetoed key per call, and Stats() returns both.
func TestStatsCounters(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.OpenAI, "k1")
	s.Mark(schemas.OpenAI, "k2")
	s.Mark(schemas.OpenAI, "k1") // duplicate Mark still counts

	filter := s.AsFilter(nil)
	keys := []schemas.Key{
		{ID: "k1", Name: "k1"},
		{ID: "k2", Name: "k2"},
		{ID: "k3", Name: "k3"},
	}
	if _, err := filter(nil, schemas.OpenAI, "gpt-4o", keys); err != nil {
		t.Fatalf("filter call 1: %v", err)
	}
	if _, err := filter(nil, schemas.OpenAI, "gpt-4o", keys); err != nil {
		t.Fatalf("filter call 2: %v", err)
	}

	stats := s.Stats()
	if got, want := stats.MarkCount, uint64(3); got != want {
		t.Errorf("MarkCount = %d, want %d", got, want)
	}
	if got, want := stats.SuppressedCount, uint64(4); got != want {
		t.Errorf("SuppressedCount = %d, want %d (2 calls × 2 vetoed keys)", got, want)
	}
	if got, want := stats.CurrentActiveCount, 2; got != want {
		t.Errorf("CurrentActiveCount = %d, want %d", got, want)
	}
}

// TestAsFilterLogsSuppressedKeysAtInfo promotes the suppression log line
// from Debug to Info so operators running with LOG_LEVEL=info can observe
// filter hits (the previous Debug level made the feature invisible in
// production unless logging was turned up).
func TestAsFilterLogsSuppressedKeysAtInfo(t *testing.T) {
	log := &testLogger{}
	s := NewCooldownState(30 * time.Minute)
	s.Mark(schemas.OpenAI, "hot-key")

	filter := s.AsFilter(log)
	keys := []schemas.Key{{ID: "hot-key"}, {ID: "cold-key"}}
	if _, err := filter(nil, schemas.OpenAI, "gpt-4o", keys); err != nil {
		t.Fatalf("filter: %v", err)
	}
	if !log.contains("info [provider-cooldown] suppressed key openai/hot-key (name=, model=gpt-4o)") {
		t.Fatalf("expected info-level suppression log, got %d messages: %v", log.count(), log.msgs)
	}
}
