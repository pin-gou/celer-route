package providercooldown

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/pg-gateway/core/schemas"
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
func (l *testLogger) Info(msg string, args ...any)  { l.record("info", msg, args) }
func (l *testLogger) Warn(msg string, args ...any)  { l.record("warn", msg, args) }
func (l *testLogger) Error(msg string, args ...any) { l.record("error", msg, args) }

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

func (l *testLogger) Fatal(msg string, args ...any)          { l.record("fatal", msg, args) }
func (l *testLogger) SetLevel(schemas.LogLevel)              {}
func (l *testLogger) SetOutputType(schemas.LoggerOutputType) {}
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

// TestAsMarkerMarksOnRateLimit covers the central reason the marker exists:
// the retry loop rotates past a 429 on key A and succeeds on key B, but A
// is the one that actually failed and we want A — not B — marked in cooldown.
// PostLLMHook would only see the terminal success and skip A entirely; the
// marker is the path that closes that gap.
func TestAsMarkerMarksOnRateLimit(t *testing.T) {
	s := NewCooldownState(time.Minute)
	provider := schemas.OpenAI
	marker := s.AsMarker(nil)

	status := 429
	rateErr := &schemas.BifrostError{
		StatusCode: &status,
		Error: &schemas.ErrorField{
			Message: "Rate limit reached for requests",
			Code:    strPtr("rate_limit_exceeded"),
		},
	}

	marker(nil, provider, "key-A", "A", "gpt-4o", rateErr)

	if !s.IsCoolingDown(provider, "key-A") {
		t.Fatal("expected key-A to be in cooldown after marker observed a rate-limit failure")
	}
	if s.IsCoolingDown(provider, "key-B") {
		t.Fatal("key-B must not be marked — the marker only acts on the failed key")
	}
}

// TestAsMarkerPrefersQuotaOverRateLimit pins the rule precedence: an error
// matching both rules (would only happen with a custom policy that overlaps,
// but we want the behaviour documented) must be attributed to quota — the
// longer, harder-to-recover kind — never to rate_limit.
func TestAsMarkerPrefersQuotaOverRateLimit(t *testing.T) {
	s := NewCooldownState(time.Minute)
	provider := schemas.OpenAI
	// Custom policy that intentionally makes both rules match the same error.
	// The test pins behaviour, not the default policy's narrow rule set.
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyCooldownPolicy, &schemas.CooldownPolicy{
		Quota: &schemas.CooldownPolicyRule{
			MatchMode:  "any",
			TTLSeconds: 600,
			Match: []schemas.CooldownPolicyMatch{
				{MessageContains: []string{"dual-matched"}},
			},
		},
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode:  "any",
			TTLSeconds: 60,
			Match: []schemas.CooldownPolicyMatch{
				{MessageContains: []string{"dual-matched"}},
			},
		},
	})
	marker := s.AsMarker(nil)

	status := 429
	err := &schemas.BifrostError{
		StatusCode: &status,
		Error:      &schemas.ErrorField{Message: "dual-matched signal"},
	}
	marker(ctx, provider, "key-X", "X", "gpt-4o", err)

	if !s.IsCoolingDown(provider, "key-X") {
		t.Fatal("expected key-X to be marked")
	}
	_, kind, ok := s.lookupCooldown(provider, "key-X", "", schemas.CooldownScopeKey)
	if !ok || kind != CooldownKindQuota {
		t.Fatalf("expected kind=quota (precedence over rate_limit), got ok=%v kind=%q", ok, kind)
	}
}

// TestAsMarkerIgnoresPolicyMiss covers the silent no-op path: the per-provider
// CooldownPolicy doesn't match the failure (e.g. sensenova default policy
// doesn't match a bare HTTP 429 without a quota-specific signal). The marker
// must NOT bump counters for those, otherwise we would silently cool every
// key that hits a generic 429.
func TestAsMarkerIgnoresPolicyMiss(t *testing.T) {
	s := NewCooldownState(time.Minute)
	provider := schemas.Sensenova
	marker := s.AsMarker(nil)

	status := 429
	err := &schemas.BifrostError{
		StatusCode: &status,
		Error: &schemas.ErrorField{
			// sensenova rate-limit rule requires message containing
			// "http error 429" or Type "rate_limit_error" — neither of
			// which a generic 400-class error carries, so the policy
			// miss path is exactly what we want to exercise here.
			Message: "validation failed",
			Type:    strPtr("invalid_request_error"),
		},
	}
	before := s.Size()
	marker(nil, provider, "key-Z", "Z", "deepseek-v4-flash", err)

	if s.IsCoolingDown(provider, "key-Z") {
		t.Fatal("policy-miss errors must not be marked")
	}
	if got := s.Size(); got != before {
		t.Fatalf("policy-miss must not change state size: before=%d after=%d", before, got)
	}
}

// TestAsMarkerNilArgsNoPanic pins the guard rails — bifrost can race a
// ctx-cancel or skip-key edge case through the marker, and the marker must
// never panic on missing inputs.
func TestAsMarkerNilArgsNoPanic(t *testing.T) {
	s := NewCooldownState(time.Minute)
	marker := s.AsMarker(nil)

	// nil error: no-op, no panic
	marker(nil, schemas.OpenAI, "k", "k", "gpt-4o", nil)
	// empty keyID: no-op
	marker(nil, schemas.OpenAI, "", "", "gpt-4o", &schemas.BifrostError{StatusCode: intPtr(429)})
	// nil state: caller-side, but we still want the marker closure to be
	// safe even if bifrost hands us a nil receiver somehow.
	nilMarker := (*CooldownState)(nil).AsMarker(nil)
	nilMarker(nil, schemas.OpenAI, "k", "k", "gpt-4o", &schemas.BifrostError{StatusCode: intPtr(429)})
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

	if !s.ClearKey(schemas.OpenAI, "key-a", "") {
		t.Fatal("ClearKey should return true for an existing entry")
	}
	if s.IsCoolingDown(schemas.OpenAI, "key-a") {
		t.Fatal("after ClearKey, the key must no longer be cooling down")
	}
	// Clearing again is a no-op returning false.
	if s.ClearKey(schemas.OpenAI, "key-a", "") {
		t.Fatal("ClearKey on a non-existent entry should return false")
	}
}

func TestStateClearKeyEmptyKeyID(t *testing.T) {
	s := NewCooldownState(time.Minute)
	if s.ClearKey(schemas.OpenAI, "", "") {
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
	if !p.ClearKey(schemas.OpenAI, "key-a", "") {
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
	if p.ClearKey(schemas.OpenAI, "k", "") {
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
	if !log.contains("suppressed key openai/hot-key (name=hot, model=gpt-4o") {
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
	// Non-positive TTL must fall back to DefaultCooldownTTL (5 min).
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
	if !log.contains("info [provider-cooldown] suppressed key openai/hot-key (name=, model=gpt-4o") {
		t.Fatalf("expected info-level suppression log, got %d messages: %v", log.count(), log.msgs)
	}
}

// TestPreProviderHookShortCircuitOnAllKeysCooled verifies the new silent
// short-circuit contract: when every key for the targeted provider is in
// cooldown, PreProviderHook returns an LLMPluginShortCircuit with a synthetic
// 503 "no_eligible_keys" BifrostError and Silent=true so the framework can
// skip the worker queue (and the logging plugin can skip the spurious
// "cancelled" row).
func TestPreProviderHookShortCircuitOnAllKeysCooled(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	provider := schemas.OpenAI
	p.State.Mark(provider, "key-1")
	p.State.Mark(provider, "key-2")

	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyProviderKeys, map[schemas.ModelProvider][]schemas.Key{
		provider: {{ID: "key-1"}, {ID: "key-2"}},
	})

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: provider,
			Model:    "gpt-4o",
		},
	}

	gotReq, sc, err := p.PreProviderHook(ctx, req)
	if err != nil {
		t.Fatalf("PreProviderHook must not return an error, got %v", err)
	}
	if sc == nil {
		t.Fatal("expected short-circuit when every key is in cooldown")
	}
	if !sc.Silent {
		t.Fatal("silent short-circuit must be set so logging skips the spurious cancelled row")
	}
	if sc.Error == nil {
		t.Fatal("short-circuit must carry the synthetic 503 BifrostError")
	}
	if sc.Error.StatusCode == nil || *sc.Error.StatusCode != 503 {
		t.Fatalf("expected 503 status code, got %+v", sc.Error.StatusCode)
	}
	if sc.Error.Type == nil || *sc.Error.Type != "no_eligible_keys" {
		t.Fatalf("expected no_eligible_keys error type, got %+v", sc.Error.Type)
	}
	if gotReq != req {
		t.Fatal("PreProviderHook must return the SAME request pointer")
	}
}

// TestPreProviderHookPassesThroughWhenSomeKeysAvailable verifies that with at
// least one eligible key, PreProviderHook is a no-op passthrough (no
// short-circuit) so the request reaches the worker as usual.
func TestPreProviderHookPassesThroughWhenSomeKeysAvailable(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	provider := schemas.OpenAI
	p.State.Mark(provider, "key-1") // only key-1 cooled; key-2 must remain eligible

	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyProviderKeys, map[schemas.ModelProvider][]schemas.Key{
		provider: {{ID: "key-1"}, {ID: "key-2"}},
	})

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: provider,
			Model:    "gpt-4o",
		},
	}

	gotReq, sc, err := p.PreProviderHook(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc != nil {
		t.Fatalf("must not short-circuit when at least one key is available, got %+v", sc)
	}
	if gotReq != req {
		t.Fatal("PreProviderHook must return the same request pointer on passthrough")
	}
}

// TestPreProviderHookPassesThroughWhenNoKeySnapshot ensures the hook is a
// no-op (not a panic, not a short-circuit) when the framework hasn't stamped
// the provider key snapshot — typically because the hook is invoked from a
// caller that bypasses stampProviderKeysOnContext. The worker's KeyPoolFilter
// remains the sole veto authority in that case.
func TestPreProviderHookPassesThroughWhenNoKeySnapshot(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o",
		},
	}

	ctx := schemas.NewBifrostContext(nil, time.Time{})
	gotReq, sc, err := p.PreProviderHook(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc != nil {
		t.Fatalf("must not short-circuit without a provider key snapshot, got %+v", sc)
	}
	if gotReq != req {
		t.Fatal("must return the same request pointer on passthrough")
	}
}

// TestPreProviderHookPassesThroughWhenProviderUnknown covers the case where
// the request's provider has no entry in the snapshot (typically because the
// routing layer chose a custom provider the account doesn't list). The hook
// must not fabricate a short-circuit; the normal pipeline surfaces the
// underlying "no keys configured" error.
func TestPreProviderHookPassesThroughWhenProviderUnknown(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyProviderKeys, map[schemas.ModelProvider][]schemas.Key{
		schemas.OpenAI: {{ID: "key-1"}},
	})

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.Anthropic, // not in snapshot
			Model:    "claude-3",
		},
	}

	gotReq, sc, err := p.PreProviderHook(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc != nil {
		t.Fatalf("must not short-circuit on an unconfigured provider, got %+v", sc)
	}
	if gotReq != req {
		t.Fatal("must return the same request pointer on passthrough")
	}
}

// ---------------------------------------------------------------------------
// Per-kind counters (rate_limit / quota) — see core plugin proposal §3.2.
// The CooldownState is expected to attribute lifetime mark / suppressed
// counters to the CooldownKind that triggered them, so the monitoring
// UI can show "速率限制标记 7 / 抑制 5, 配额标记 5 / 抑制 3" rather than
// a single rolled-up total.
// ---------------------------------------------------------------------------

// TestMarkWithKind_AttributesToByKind pins that a kind-classified mark
// bumps the matching by_kind counter (and the legacy markCount for
// backward compatibility), but an unclassified Mark() bumps ONLY the
// legacy counter.
func TestMarkWithKind_AttributesToByKind(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "k1", "", 60*time.Second, CooldownKindRateLimit, schemas.CooldownScopeKey)
	s.MarkWithTTL(schemas.OpenAI, "k2", "", 60*time.Second, CooldownKindRateLimit, schemas.CooldownScopeKey)
	s.MarkWithTTL(schemas.Anthropic, "k3", "", 60*time.Second, CooldownKindQuota, schemas.CooldownScopeKey)
	s.Mark(schemas.Anthropic, "k4") // unclassified (legacy quota_patterns path)

	stats := s.Stats()

	// Legacy field covers EVERYTHING including unclassified.
	if got, want := stats.MarkCount, uint64(4); got != want {
		t.Fatalf("legacy mark_count = %d, want %d (covers all kinds)", got, want)
	}
	// By-kind only counts classified marks.
	if got, want := stats.ByKind.RateLimit.MarkCount, uint64(2); got != want {
		t.Fatalf("by_kind.rate_limit.mark_count = %d, want %d", got, want)
	}
	if got, want := stats.ByKind.Quota.MarkCount, uint64(1); got != want {
		t.Fatalf("by_kind.quota.mark_count = %d, want %d", got, want)
	}
	if stats.ByKind.RateLimit.SuppressedCount != 0 {
		t.Fatalf("expected no suppressions yet, got %d", stats.ByKind.RateLimit.SuppressedCount)
	}
}

// TestAsFilter_AttributesSuppressionToKind confirms that a key marked as
// rate_limit, when later vetoed by AsFilter, bumps by_kind.rate_limit and
// per_provider[<provider>].rate_limit.suppressed_count — NOT quota's
// counters, even when the request would otherwise have looked like quota
// to a downstream observer.
func TestAsFilter_AttributesSuppressionToKind(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "rl-hot", "", 60*time.Second, CooldownKindRateLimit, schemas.CooldownScopeKey)
	s.MarkWithTTL(schemas.OpenAI, "q-hot", "", 60*time.Second, CooldownKindQuota, schemas.CooldownScopeKey)
	s.Mark(schemas.OpenAI, "u-hot") // unclassified

	keys := []schemas.Key{{ID: "rl-hot"}, {ID: "q-hot"}, {ID: "u-hot"}, {ID: "cold"}}
	filter := s.AsFilter(nil)
	out, err := filter(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(out) != 1 || out[0].ID != "cold" {
		t.Fatalf("expected only cold-key to remain, got %+v", out)
	}

	stats := s.Stats()

	// Legacy suppressed_count = 3 (rl-hot + q-hot + u-hot).
	if got, want := stats.SuppressedCount, uint64(3); got != want {
		t.Fatalf("legacy suppressed_count = %d, want %d", got, want)
	}
	// by_kind only attributes the classified ones.
	if got, want := stats.ByKind.RateLimit.SuppressedCount, uint64(1); got != want {
		t.Fatalf("by_kind.rate_limit.suppressed_count = %d, want %d", got, want)
	}
	if got, want := stats.ByKind.Quota.SuppressedCount, uint64(1); got != want {
		t.Fatalf("by_kind.quota.suppressed_count = %d, want %d", got, want)
	}
	// Unclassified mark should NOT appear in any by_kind.SuppressedCount.
	if total := stats.ByKind.RateLimit.SuppressedCount + stats.ByKind.Quota.SuppressedCount; total != 2 {
		t.Fatalf("expected by_kind totals to sum to 2 (skipping unclassified), got %d", total)
	}

	// per_provider must reflect the per-kind breakdown.
	pp, ok := stats.PerProvider[schemas.OpenAI]
	if !ok {
		t.Fatalf("per_provider[openai] missing, got %+v", stats.PerProvider)
	}
	if pp.RateLimit.SuppressedCount != 1 {
		t.Fatalf("per_provider[openai].rate_limit.suppressed_count = %d, want 1", pp.RateLimit.SuppressedCount)
	}
	if pp.Quota.SuppressedCount != 1 {
		t.Fatalf("per_provider[openai].quota.suppressed_count = %d, want 1", pp.Quota.SuppressedCount)
	}
	if pp.RateLimit.MarkCount != 1 {
		t.Fatalf("per_provider[openai].rate_limit.mark_count = %d, want 1", pp.RateLimit.MarkCount)
	}
	if pp.Quota.MarkCount != 1 {
		t.Fatalf("per_provider[openai].quota.mark_count = %d, want 1", pp.Quota.MarkCount)
	}
}

// TestStats_PerProviderOmitsUnseenProviders confirms that providers with
// no classified events do NOT appear in the per_provider map — keeps the
// payload compact for the UI's per-provider listing.
func TestStats_PerProviderOmitsUnseenProviders(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "k1", "", 60*time.Second, CooldownKindRateLimit, schemas.CooldownScopeKey)	// sensenova never gets a classified event.

	stats := s.Stats()
	if _, ok := stats.PerProvider[schemas.OpenAI]; !ok {
		t.Fatalf("expected openai in per_provider, got %+v", stats.PerProvider)
	}
	if _, ok := stats.PerProvider[schemas.Sensenova]; ok {
		t.Fatalf("sensenova should not appear (no classified events), got %+v", stats.PerProvider)
	}
}

// TestSnapshot_IncludesKind confirms Snapshot returns entries tagged with
// the kind that triggered each mark, so the GET state endpoint can surface
// "速率限制" vs "配额耗尽" badges on each row.
func TestSnapshot_IncludesKind(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "rl", "", 60*time.Second, CooldownKindRateLimit, schemas.CooldownScopeKey)
	s.MarkWithTTL(schemas.OpenAI, "q", "", 60*time.Second, CooldownKindQuota, schemas.CooldownScopeKey)
	s.Mark(schemas.OpenAI, "u") // unclassified → Kind=""

	entries := s.Snapshot()
	got := map[string]CooldownKind{}
	for _, e := range entries {
		got[e.KeyID] = e.Kind
	}
	if got["rl"] != CooldownKindRateLimit {
		t.Fatalf("entry[rl].Kind = %q, want %q", got["rl"], CooldownKindRateLimit)
	}
	if got["q"] != CooldownKindQuota {
		t.Fatalf("entry[q].Kind = %q, want %q", got["q"], CooldownKindQuota)
	}
	if got["u"] != "" {
		t.Fatalf("entry[u].Kind = %q, want \"\" (unclassified)", got["u"])
	}
}

// TestClassify_ReturnsKind confirms classify attributes the matched rule
// to its kind so PostLLMHook can pass it to MarkWithTTL.
func TestClassify_ReturnsKind(t *testing.T) {
	plugin := NewPlugin(nil)
	policy := &schemas.CooldownPolicy{
		Quota: &schemas.CooldownPolicyRule{
			MatchMode:  "any",
			TTLSeconds: 600,
			Match:      []schemas.CooldownPolicyMatch{{MessageContains: []string{"quota"}}},
		},
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode:  "any",
			TTLSeconds: 60,
			Match:      []schemas.CooldownPolicyMatch{{StatusCode: intPtr(429)}},
		},
	}
	ctx := withTrail(t, policy)

	// Quota rule wins on the "quota" message.
	err := newErr(429, "", "", "quota exceeded")
	_, _, _, _, kind, _, ok := plugin.classify(ctx, err)
	if !ok {
		t.Fatal("expected classify to fire on quota message")
	}
	if kind != CooldownKindQuota {
		t.Fatalf("kind = %q, want %q", kind, CooldownKindQuota)
	}

	// Rate-limit rule wins on a bare 429 with no quota signal.
	err = newErr(429, "", "", "rate exceeded")
	_, _, _, _, kind, _, ok = plugin.classify(ctx, err)
	if !ok {
		t.Fatal("expected classify to fire on rate-limit message")
	}
	if kind != CooldownKindRateLimit {
		t.Fatalf("kind = %q, want %q", kind, CooldownKindRateLimit)
	}

	// No match → kind is empty (unclassified), ok=false.
	err = newErr(500, "", "", "server error")
	_, _, _, _, kind, _, ok = plugin.classify(ctx, err)
	if ok {
		t.Fatal("expected classify to NOT fire on 500")
	}
	if kind != "" {
		t.Fatalf("kind = %q, want \"\" on no match", kind)
	}
}

// --- Keyless provider (e.g. bare opencode) regression tests ---
//
// Keyless providers never carry API credentials; bifrost's GetKeysForProvider
// returns a single []schemas.Key{{}} with empty ID/Name/Value. The retry
// loop routes every attempt through that sentinel. The plugin must therefore
// allow the empty-string keyID to be marked, looked up, and filtered on the
// keyless provider, while keeping the existing "no-op on empty keyID" safety
// net for every other provider (no regression on the per-key model).

func TestCooldownState_KeylessProvider_MarksEmptyKeyID(t *testing.T) {
	s := NewCooldownState(time.Minute)
	// Mark with empty keyID on a keyless provider — must NOT be a no-op.
	s.Mark(schemas.Opencode, "")
	if !s.IsCoolingDown(schemas.Opencode, "") {
		t.Fatal("keyless provider with empty keyID must be markable and lookup-able")
	}
	if s.Size() != 1 {
		t.Fatalf("Size = %d, want 1", s.Size())
	}
}

func TestCooldownState_NonKeylessProvider_EmptyKeyIDStillNoOp(t *testing.T) {
	// Regression guard: the original "empty keyID is a no-op" invariant must
	// still hold for every provider that isn't in schemas.KeylessProviders,
	// so a caller bug cannot accidentally cool the whole provider pool.
	for _, p := range []schemas.ModelProvider{schemas.OpenAI, schemas.Anthropic, schemas.Bedrock, schemas.Gemini, schemas.OpencodeGo, schemas.OpencodeZen, "some-other-provider"} {
		s := NewCooldownState(time.Minute)
		s.Mark(p, "")
		if s.IsCoolingDown(p, "") {
			t.Fatalf("provider %q: empty keyID must remain a no-op", p)
		}
		if s.Size() != 0 {
			t.Fatalf("provider %q: Size = %d, want 0", p, s.Size())
		}
	}
}

func TestAsFilter_KeylessProvider_FiltersSentinelWhenCooled(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.Opencode, "")
	keys := []schemas.Key{{}}
	out, err := s.AsFilter(nil)(nil, schemas.Opencode, "opencode-model", keys)
	if err != nil {
		t.Fatalf("AsFilter returned error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected the keyless sentinel to be filtered out when cooled, got %d keys", len(out))
	}
}

func TestAsFilter_KeylessProvider_PassesThroughWhenNotCooled(t *testing.T) {
	s := NewCooldownState(time.Minute)
	keys := []schemas.Key{{}}
	out, err := s.AsFilter(nil)(nil, schemas.Opencode, "opencode-model", keys)
	if err != nil {
		t.Fatalf("AsFilter returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected the keyless sentinel to pass through when not cooled, got %d keys", len(out))
	}
}

func TestAsFilter_KeylessProvider_NonKeylessEmptyKeyIDNotFiltered(t *testing.T) {
	// Even if a caller mistakenly passes an empty keyID for a non-keyless
	// provider, AsFilter must not treat the empty keyID as "suppressed" —
	// lookupCooldown returns false for that case, so the key passes through.
	s := NewCooldownState(time.Minute)
	keys := []schemas.Key{{ID: ""}}
	out, err := s.AsFilter(nil)(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("AsFilter returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected empty-keyID to pass through on non-keyless provider, got %d keys", len(out))
	}
}

func TestAsMarker_KeylessProvider_MarksOnRateLimit(t *testing.T) {
	s := NewCooldownState(time.Minute)
	policy := &schemas.CooldownPolicy{
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode:  "any",
			TTLSeconds: 60,
			Match:      []schemas.CooldownPolicyMatch{{StatusCode: intPtr(429)}},
		},
	}
	ctx := withTrail(t, policy)
	err := newErr(429, "", "", "rate limit exceeded")

	// Empty keyID on the keyless provider — marker must mark and not skip.
	s.AsMarker(nil)(ctx, schemas.Opencode, "", "", "opencode-model", err)

	if !s.IsCoolingDown(schemas.Opencode, "") {
		t.Fatal("AsMarker must mark the keyless provider even with empty keyID")
	}
	stats := s.Stats()
	if stats.PerProvider[schemas.Opencode].RateLimit.MarkCount != 1 {
		t.Fatalf("PerProvider[opencode].RateLimit.MarkCount = %d, want 1", stats.PerProvider[schemas.Opencode].RateLimit.MarkCount)
	}
}

func TestAsMarker_NonKeylessProvider_EmptyKeyIDStillSkipped(t *testing.T) {
	s := NewCooldownState(time.Minute)
	policy := &schemas.CooldownPolicy{
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode:  "any",
			TTLSeconds: 60,
			Match:      []schemas.CooldownPolicyMatch{{StatusCode: intPtr(429)}},
		},
	}
	ctx := withTrail(t, policy)
	err := newErr(429, "", "", "rate limit exceeded")

	s.AsMarker(nil)(ctx, schemas.OpenAI, "", "", "gpt-4o", err)

	if s.IsCoolingDown(schemas.OpenAI, "") {
		t.Fatal("AsMarker must skip empty keyID on non-keyless provider")
	}
	if s.Size() != 0 {
		t.Fatalf("Size = %d, want 0", s.Size())
	}
}

func TestPostLLMHook_KeylessProvider_MarksOnTerminalQuotaError(t *testing.T) {
	p := NewPlugin(nil)
	policy := &schemas.CooldownPolicy{
		Quota: &schemas.CooldownPolicyRule{
			MatchMode:  "any",
			TTLSeconds: 600,
			Match:      []schemas.CooldownPolicyMatch{{MessageContains: []string{"insufficient_quota"}}},
		},
	}
	ctx := withTrail(t, policy)
	// Stamp the resolved provider as Opencode on the error so the policy
	// lookup targets the keyless default rule set.
	err := newErrOn(429, "", "insufficient_quota", "insufficient_quota: workspace quota", string(schemas.Opencode))
	// Force the trail to record the empty-keyID attempt (mirrors what the
	// retry loop appends for a keyless provider).
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{Attempt: 1, KeyID: "", KeyName: ""},
	})

	_, _, _ = p.PostLLMHook(ctx, nil, err)

	if !p.State.IsCoolingDown(schemas.Opencode, "") {
		t.Fatal("PostLLMHook must mark the keyless provider when terminal error is quota exhaustion")
	}
	stats := p.State.Stats()
	if stats.PerProvider[schemas.Opencode].Quota.MarkCount != 1 {
		t.Fatalf("PerProvider[opencode].Quota.MarkCount = %d, want 1", stats.PerProvider[schemas.Opencode].Quota.MarkCount)
	}
}

func TestCooldownState_KeylessProvider_ExpiresAndRevives(t *testing.T) {
	s := NewCooldownState(20 * time.Millisecond)
	s.Mark(schemas.Opencode, "")
	if !s.IsCoolingDown(schemas.Opencode, "") {
		t.Fatal("expected keyless sentinel to be cooled immediately after Mark")
	}
	// Wait past TTL — cooldown must expire and the sentinel revive.
	time.Sleep(40 * time.Millisecond)
	if s.IsCoolingDown(schemas.Opencode, "") {
		t.Fatal("keyless sentinel must not stay cooled past TTL")
	}
}

func TestClearKey_KeylessProvider_EmptyKeyIDNoOp(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.Opencode, "")
	if !s.IsCoolingDown(schemas.Opencode, "") {
		t.Fatal("setup: expected keyless sentinel to be cooled")
	}
	// ClearKey intentionally rejects empty keyID even for keyless providers
	// — there is no operator path that targets the synthetic sentinel; if
	// an operator wants to lift the cooldown they wait for it to expire.
	if s.ClearKey(schemas.Opencode, "", "") {
		t.Fatal("ClearKey must NOT clear the keyless sentinel via empty keyID")
	}
	if !s.IsCoolingDown(schemas.Opencode, "") {
		t.Fatal("keyless sentinel must remain cooled after a no-op ClearKey")
	}
}

// ---------------------------------------------------------------------------
// Keyless provider — PreProviderHook must still see the cooled sentinel.
//
// Background: schemas.KeylessProviders (e.g. bare `opencode`, the no-auth
// OpenCode Free tier) never get a real key in the user config. The framework
// stamps provider keys into ctx[BifrostContextKeyProviderKeys] before
// PreProviderHook, but stampProviderKeysOnContext (core/bifrost.go:518) only
// writes an entry when len(keys) > 0 — a keyless provider with no configured
// keys is therefore absent from the snapshot, and PreProviderHook used to
// bail out early ("no keys configured") without ever calling AsFilter.
//
// The mark path still fired (PostLLMHook + PerKeyFailureMarker both treat
// empty keyID as the legal sentinel for keyless providers), so cooldowns
// were recorded against "<provider>::" but never observed by the suppression
// path — the UI's "配额耗尽 抑制" counter stayed at 0 even as
// "配额耗尽 标记" climbed on every failed request.
//
// These tests pin the contract that PreProviderHook synthesizes a sentinel
// key for keyless providers when the snapshot is empty/missing, so the
// existing mark entry ("<provider>::") is honored by AsFilter and the
// per-kind suppressed counter increments.
// ---------------------------------------------------------------------------

// TestPreProviderHook_KeylessProviderSynthesizesSentinelKey reproduces the
// real-world bug: a keyless provider (Opencode) has no keys in the snapshot
// (because no API keys are configured for it) but state already holds a
// quota-mark from an earlier failed attempt. PreProviderHook must
// short-circuit AND bump the byKind.quota.suppressed counter, mirroring the
// behavior of providers with real keys.
//
// Before the fix this test fails: PreProviderHook returns (req, nil, nil)
// because the snapshot lacks the provider, AsFilter never runs, and the
// suppressed counter stays at 0 even though the user can see the cooldown
// entry on the monitoring panel.
func TestPreProviderHook_KeylessProviderSynthesizesSentinelKey(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	provider := schemas.Opencode
	// Pre-condition: a quota mark already exists for the keyless sentinel
	// (empty keyID is the legal sentinel for schemas.IsKeylessProvider).
	p.State.MarkWithTTL(provider, "", "", time.Minute, CooldownKindQuota, schemas.CooldownScopeKey)
	if !p.State.IsCoolingDown(provider, "") {
		t.Fatal("setup: expected keyless sentinel to be cooled")
	}

	// Snapshot intentionally lacks the keyless provider — this matches what
	// stampProviderKeysOnContext produces when config has no keys for the
	// provider (the real-world scenario for OpenCode Free).
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyProviderKeys, map[schemas.ModelProvider][]schemas.Key{
		schemas.OpenAI: {{ID: "key-1"}}, // some other provider, not Opencode
	})

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: provider,
			Model:    "hermes-operator",
		},
	}

	preStats := p.State.Stats()
	gotReq, sc, err := p.PreProviderHook(ctx, req)
	if err != nil {
		t.Fatalf("PreProviderHook must not return an error, got %v", err)
	}
	if gotReq != req {
		t.Fatal("PreProviderHook must return the SAME request pointer")
	}

	// Expectation: the only key (the synthesized sentinel) is in cooldown,
	// so the hook must short-circuit exactly like a regular all-cooled case.
	if sc == nil {
		t.Fatal("expected short-circuit for keyless provider with cooldown on the sentinel")
	}
	if !sc.Silent {
		t.Fatal("short-circuit must be Silent=true (logging plugin skips the spurious cancelled row)")
	}
	if sc.Error == nil || sc.Error.StatusCode == nil || *sc.Error.StatusCode != 503 {
		t.Fatalf("expected 503 short-circuit error, got %+v", sc.Error)
	}
	if sc.Error.Type == nil || *sc.Error.Type != "no_eligible_keys" {
		t.Fatalf("expected no_eligible_keys type, got %+v", sc.Error.Type)
	}

	// Expectation: the per-kind suppressed counter for quota advanced by 1.
	postStats := p.State.Stats()
	if got, want := postStats.ByKind.Quota.SuppressedCount, preStats.ByKind.Quota.SuppressedCount+1; got != want {
		t.Fatalf("byKind.quota.suppressed = %d, want %d", got, want)
	}
	if got, want := postStats.SuppressedCount, preStats.SuppressedCount+1; got != want {
		t.Fatalf("legacy suppressed_count = %d, want %d", got, want)
	}
}

// TestPreProviderHook_KeylessProviderNoCooldown_NoSuppress covers the
// negative case for keyless providers: when the snapshot lacks the
// keyless provider AND no cooldown is currently active for it, the hook
// must not short-circuit and must not bump suppressed counters — a
// keyless provider with no cooldown is exactly the same as a regular
// provider that simply has no keys configured.
func TestPreProviderHook_KeylessProviderNoCooldown_NoSuppress(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	provider := schemas.Opencode

	ctx := schemas.NewBifrostContext(nil, time.Time{})
	// Snapshot intentionally lacks the keyless provider.
	ctx.SetValue(schemas.BifrostContextKeyProviderKeys, map[schemas.ModelProvider][]schemas.Key{
		schemas.OpenAI: {{ID: "key-1"}},
	})

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: provider,
			Model:    "hermes-operator",
		},
	}

	preStats := p.State.Stats()
	gotReq, sc, err := p.PreProviderHook(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc != nil {
		t.Fatalf("must not short-circuit when keyless sentinel is not cooled, got %+v", sc)
	}
	if gotReq != req {
		t.Fatal("must return the same request pointer on passthrough")
	}

	postStats := p.State.Stats()
	if postStats.SuppressedCount != preStats.SuppressedCount {
		t.Fatalf("suppressed_count must not advance when there is no cooldown, got %d (was %d)",
			postStats.SuppressedCount, preStats.SuppressedCount)
	}
	if postStats.ByKind.Quota.SuppressedCount != preStats.ByKind.Quota.SuppressedCount {
		t.Fatalf("byKind.quota.suppressed must not advance when there is no cooldown, got %d (was %d)",
			postStats.ByKind.Quota.SuppressedCount, preStats.ByKind.Quota.SuppressedCount)
	}
}

// TestPreProviderHook_NonKeylessProviderEmptySnapshotStillPassesThrough
// pins the unchanged contract for regular (non-keyless) providers: when the
// snapshot lacks the provider, the hook must still pass through. We do NOT
// synthesize a sentinel for non-keyless providers because that would mark
// the whole provider on the next mark — exactly the operator-bug we want to
// avoid (see lookupCooldown: empty keyID on non-keyless is treated as a
// no-op, but synthesizing a sentinel would route future marks there).
func TestPreProviderHook_NonKeylessProviderEmptySnapshotStillPassesThrough(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	provider := schemas.Anthropic // non-keyless

	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyProviderKeys, map[schemas.ModelProvider][]schemas.Key{
		schemas.OpenAI: {{ID: "key-1"}},
	})

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: provider,
			Model:    "claude-3",
		},
	}

	preStats := p.State.Stats()
	gotReq, sc, err := p.PreProviderHook(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc != nil {
		t.Fatalf("non-keyless provider with empty snapshot must NOT short-circuit, got %+v", sc)
	}
	if gotReq != req {
		t.Fatal("must return the same request pointer on passthrough")
	}
	if post := p.State.Stats(); post.SuppressedCount != preStats.SuppressedCount {
		t.Fatalf("suppressed_count must not advance for non-keyless empty snapshot, got %d (was %d)",
			post.SuppressedCount, preStats.SuppressedCount)
	}
}

// ---------------------------------------------------------------------------
// Scope (model-granularity cooldown) tests
// ---------------------------------------------------------------------------

func TestScopeModel_MarkAndLookup(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	if !s.IsCoolingDownForModel(schemas.OpenAI, "key-a", "gpt-4o") {
		t.Fatal("scope=model: key-a/gpt-4o should be cooling down")
	}
	// key-granularity must NOT be affected by a model-granularity mark.
	if s.IsCoolingDown(schemas.OpenAI, "key-a") {
		t.Fatal("scope=model: key-granularity must NOT be suppressed by a model-only mark")
	}
}

func TestScopeModel_DifferentModelNotSuppressed(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	if s.IsCoolingDownForModel(schemas.OpenAI, "key-a", "claude-3") {
		t.Fatal("scope=model: key-a/claude-3 must NOT be suppressed when only gpt-4o is marked")
	}
}

func TestScopeModel_EmptyModelNotMarked(t *testing.T) {
	s := NewCooldownState(time.Minute)
	// scope=model with empty model must be a no-op.
	s.MarkWithTTL(schemas.OpenAI, "key-a", "", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	if s.Size() != 0 {
		t.Fatal("scope=model with empty model must not record an entry")
	}
}

func TestScopeKey_SuppressesAllModels(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "", time.Minute, CooldownKindQuota, schemas.CooldownScopeKey)

	if !s.IsCoolingDown(schemas.OpenAI, "key-a") {
		t.Fatal("scope=key: key-a should be cooling down at key granularity")
	}
	// lookupSuppressed must also see it regardless of model.
	_, _, _, suppressed := s.lookupSuppressed(schemas.OpenAI, "key-a", "gpt-4o")
	if !suppressed {
		t.Fatal("scope=key: lookupSuppressed must see key-granularity mark even with model")
	}
}

func TestAsFilter_ScopeModelSuppressesMatchingModel(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	keys := []schemas.Key{{ID: "key-a"}}
	filtered, err := s.AsFilter(nil)(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("AsFilter error: %v", err)
	}
	if len(filtered) != 0 {
		t.Fatal("scope=model: key-a must be suppressed when requesting gpt-4o")
	}
}

func TestAsFilter_ScopeModelSkipsDifferentModel(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	keys := []schemas.Key{{ID: "key-a"}}
	filtered, err := s.AsFilter(nil)(nil, schemas.OpenAI, "claude-3", keys)
	if err != nil {
		t.Fatalf("AsFilter error: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatal("scope=model: key-a must NOT be suppressed when requesting a different model")
	}
}

func TestAsFilter_ScopeModelEmptyModelNotSuppressed(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	keys := []schemas.Key{{ID: "key-a"}}
	// Empty model (ListModels etc.) must NOT be suppressed by model-granularity marks.
	filtered, err := s.AsFilter(nil)(nil, schemas.OpenAI, "", keys)
	if err != nil {
		t.Fatalf("AsFilter error: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatal("scope=model: key-a must NOT be suppressed when the runtime model is empty")
	}
}

func TestAsFilter_ScopeKeySuppressesAllModels(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "", time.Minute, CooldownKindQuota, schemas.CooldownScopeKey)

	keys := []schemas.Key{{ID: "key-a"}}
	// key-granularity mark suppresses every model.
	filtered, err := s.AsFilter(nil)(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("AsFilter error: %v", err)
	}
	if len(filtered) != 0 {
		t.Fatal("scope=key: key-a must be suppressed even when a model is requested")
	}
}

func TestClearKey_WithModel(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	if !s.IsCoolingDownForModel(schemas.OpenAI, "key-a", "gpt-4o") {
		t.Fatal("precondition: key-a/gpt-4o should be cooling down")
	}
	// Clear with matching model.
	if !s.ClearKey(schemas.OpenAI, "key-a", "gpt-4o") {
		t.Fatal("ClearKey with model should return true")
	}
	if s.IsCoolingDownForModel(schemas.OpenAI, "key-a", "gpt-4o") {
		t.Fatal("after ClearKey with model, entry must be removed")
	}
	// Clear with empty model should NOT affect the model-granularity entry.
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)
	if s.ClearKey(schemas.OpenAI, "key-a", "") {
		t.Fatal("ClearKey with empty model must NOT clear a model-granularity entry")
	}
	if !s.IsCoolingDownForModel(schemas.OpenAI, "key-a", "gpt-4o") {
		t.Fatal("model-granularity entry must survive ClearKey with empty model")
	}
}

func TestSnapshot_IncludesModel(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)
	s.MarkWithTTL(schemas.OpenAI, "key-b", "", time.Minute, CooldownKindRateLimit, schemas.CooldownScopeKey)

	snap := s.Snapshot()
	var foundModel, foundKey bool
	for _, e := range snap {
		if e.KeyID == "key-a" && e.Model == "gpt-4o" {
			foundModel = true
		}
		if e.KeyID == "key-b" && e.Model == "" {
			foundKey = true
		}
	}
	if !foundModel {
		t.Fatal("Snapshot should include model for model-granularity entries")
	}
	if !foundKey {
		t.Fatal("Snapshot should include empty model for key-granularity entries")
	}
}

func TestKeylessProvider_ScopeModelAndLookup(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.Opencode, "", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	if !s.IsCoolingDownForModel(schemas.Opencode, "", "gpt-4o") {
		t.Fatal("keyless scope=model: gpt-4o should be cooling down")
	}
	if s.IsCoolingDownForModel(schemas.Opencode, "", "claude-3") {
		t.Fatal("keyless scope=model: claude-3 must NOT be cooled when only gpt-4o is marked")
	}
	// Key-granularity must not be affected.
	if s.IsCoolingDown(schemas.Opencode, "") {
		t.Fatal("keyless scope=model: key-granularity must NOT be suppressed")
	}
	// Snapshot should show empty keyID and the model.
	snap := s.Snapshot()
	var found bool
	for _, e := range snap {
		if e.Provider == schemas.Opencode && e.KeyID == "" && e.Model == "gpt-4o" {
			found = true
		}
	}
	if !found {
		t.Fatal("keyless scope=model: Snapshot should include the model")
	}
}

// ---------------------------------------------------------------------------
// PerProviderModel stats tests
// ---------------------------------------------------------------------------

func TestPerProviderModel_ScopeModelMarkUpdatesModelCounters(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)
	s.MarkWithTTL(schemas.OpenAI, "key-b", "gpt-4o", time.Minute, CooldownKindRateLimit, schemas.CooldownScopeModel)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "claude-3", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	stats := s.Stats()
	if stats.PerProviderModel == nil {
		t.Fatal("PerProviderModel should be non-nil after model-granularity marks")
	}
	// openai::gpt-4o: quota mark=1, rate_limit mark=1
	gptKey := "openai::gpt-4o"
	gpt, ok := stats.PerProviderModel[gptKey]
	if !ok {
		t.Fatalf("PerProviderModel should contain %q", gptKey)
	}
	if gpt.Quota.MarkCount != 1 {
		t.Fatalf("%s quota.mark_count = %d, want 1", gptKey, gpt.Quota.MarkCount)
	}
	if gpt.RateLimit.MarkCount != 1 {
		t.Fatalf("%s rate_limit.mark_count = %d, want 1", gptKey, gpt.RateLimit.MarkCount)
	}
	// openai::claude-3: quota mark=1 only
	claudeKey := "openai::claude-3"
	claude, ok := stats.PerProviderModel[claudeKey]
	if !ok {
		t.Fatalf("PerProviderModel should contain %q", claudeKey)
	}
	if claude.Quota.MarkCount != 1 {
		t.Fatalf("%s quota.mark_count = %d, want 1", claudeKey, claude.Quota.MarkCount)
	}
	if claude.RateLimit.MarkCount != 0 {
		t.Fatalf("%s rate_limit.mark_count = %d, want 0", claudeKey, claude.RateLimit.MarkCount)
	}
}

func TestPerProviderModel_ScopeKeyMarkDoesNotUpdateModelCounters(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "", time.Minute, CooldownKindQuota, schemas.CooldownScopeKey)
	s.MarkWithTTL(schemas.OpenAI, "key-b", "", time.Minute, CooldownKindRateLimit, schemas.CooldownScopeKey)

	stats := s.Stats()
	if stats.PerProviderModel != nil && len(stats.PerProviderModel) > 0 {
		t.Fatal("PerProviderModel must be empty after key-granularity marks only")
	}
}

func TestPerProviderModel_AsFilterModelSuppressionBumpsSuppressed(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "key-a", "gpt-4o", time.Minute, CooldownKindQuota, schemas.CooldownScopeModel)

	// Suppress key-a on gpt-4o — model-granularity match.
	keys := []schemas.Key{{ID: "key-a"}}
	_, err := s.AsFilter(nil)(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("AsFilter error: %v", err)
	}

	stats := s.Stats()
	gptKey := "openai::gpt-4o"
	gpt, ok := stats.PerProviderModel[gptKey]
	if !ok {
		t.Fatalf("PerProviderModel should contain %q after model suppression", gptKey)
	}
	if gpt.Quota.SuppressedCount != 1 {
		t.Fatalf("%s quota.suppressed_count = %d, want 1", gptKey, gpt.Quota.SuppressedCount)
	}

	// Suppress on a different model — must NOT bump suppressed for gpt-4o.
	_, err = s.AsFilter(nil)(nil, schemas.OpenAI, "claude-3", keys)
	if err != nil {
		t.Fatalf("AsFilter error: %v", err)
	}

	stats = s.Stats()
	gpt = stats.PerProviderModel[gptKey]
	if gpt.Quota.SuppressedCount != 1 {
		t.Fatalf("different model suppression must NOT bump %s suppressed, got %d", gptKey, gpt.Quota.SuppressedCount)
	}
}

func TestPerProviderModel_AsFilterKeyGranularityDoesNotBumpModel(t *testing.T) {
	s := NewCooldownState(time.Minute)
	// Key-granularity mark.
	s.MarkWithTTL(schemas.OpenAI, "key-a", "", time.Minute, CooldownKindQuota, schemas.CooldownScopeKey)

	keys := []schemas.Key{{ID: "key-a"}}
	_, err := s.AsFilter(nil)(nil, schemas.OpenAI, "gpt-4o", keys)
	if err != nil {
		t.Fatalf("AsFilter error: %v", err)
	}

	// PerProviderModel must remain empty — key-granularity suppression does
	// NOT attribute to model buckets.
	stats := s.Stats()
	if stats.PerProviderModel != nil && len(stats.PerProviderModel) > 0 {
		t.Fatal("PerProviderModel must be empty after key-granularity suppression only")
	}
}
