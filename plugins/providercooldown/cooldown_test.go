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

func TestIsQuotaExhausted(t *testing.T) {
	cases := []struct {
		name string
		err  *schemas.BifrostError
		want bool
	}{
		{"nil", nil, false},
		{"402 billing", &schemas.BifrostError{StatusCode: intPtr(402)}, true},
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
		{"402 via message only", &schemas.BifrostError{Error: &schemas.ErrorField{Message: "payment required"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsQuotaExhausted(tc.err); got != tc.want {
				t.Fatalf("IsQuotaExhausted(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
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

func TestPluginNameAndCleanup(t *testing.T) {
	p := NewPlugin(nil)
	if p.GetName() == "" {
		t.Fatal("plugin name must be non-empty")
	}
	if err := p.Cleanup(); err != nil {
		t.Fatalf("Cleanup should be a no-op, got %v", err)
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

func TestPluginLogsOnMark(t *testing.T) {
	log := &testLogger{}
	plugin := NewPlugin(log)
	ctx := newTrailCtx("key-1")
	plugin.PostLLMHook(ctx, nil, newQuotaError(schemas.OpenAI))

	if !plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("expected (openai, key-1) to be in cooldown")
	}
	if !log.contains("marked key openai/key-1") {
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
	if !log.contains("suppressed key openai/hot-key") {
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
	plugin.PostLLMHook(newTrailCtx("k1"), nil, newQuotaError(schemas.OpenAI))
	if !plugin.State.IsCoolingDown(schemas.OpenAI, "k1") {
		t.Fatal("nil logger must not affect cooldown behavior")
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
