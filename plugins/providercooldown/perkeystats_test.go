package providercooldown

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// newPerKeyTrailCtx builds a context that carries an attempt trail for the
// given key — the same shape core populates for a request that reached a
// provider.
func newPerKeyTrailCtx(keyID string) *schemas.BifrostContext {
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{Attempt: 1, KeyID: keyID, KeyName: "k-" + keyID},
	})
	return ctx
}

// newPerKeyResponse builds a successful response whose RoutingInfo names the
// provider that served it.
func newPerKeyResponse(provider schemas.ModelProvider) *schemas.BifrostResponse {
	return &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType: schemas.ChatCompletionRequest,
				RoutingInfo: schemas.RoutingInfo{Provider: provider, Model: "gpt-4o"},
			},
		},
	}
}

// TestPerKeyStats_MarkBumpsPerKeyCounter pins the first leg of the per-key
// picture: a recorded cooldown is attributed to the key it belongs to.
func TestPerKeyStats_MarkBumpsPerKeyCounter(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.OpenAI, "k1")
	s.Mark(schemas.OpenAI, "k1")
	s.Mark(schemas.Anthropic, "k2")

	stats := s.Stats()
	row, ok := stats.PerKey["openai::k1"]
	if !ok {
		t.Fatalf("expected a per-key row for openai::k1, got %v", stats.PerKey)
	}
	if row.MarkCount != 2 {
		t.Errorf("openai::k1 mark_count = %d, want 2", row.MarkCount)
	}
	if row.LastMarkAt == nil {
		t.Error("expected last_mark_at to be set after a mark")
	}
	if row.SuppressedCount != 0 || row.SuccessCount != 0 {
		t.Errorf("expected no suppressed/success counts, got %+v", row)
	}
	if other, ok := stats.PerKey["anthropic::k2"]; !ok || other.MarkCount != 1 {
		t.Errorf("expected anthropic::k2 mark_count = 1, got %+v (present=%t)", other, ok)
	}
}

// TestPerKeyStats_UnclassifiedMarkStillCounts pins that the per-key counter
// is a flat counter: like the aggregate mark_count, it also covers the
// legacy quota_patterns path that classifies no kind.
func TestPerKeyStats_UnclassifiedMarkStillCounts(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.OpenAI, "legacy")

	row, ok := s.Stats().PerKey["openai::legacy"]
	if !ok {
		t.Fatal("expected a per-key row for the unclassified mark")
	}
	if row.MarkCount != 1 {
		t.Errorf("mark_count = %d, want 1", row.MarkCount)
	}
}

// TestPerKeyStats_EmptyKeyIDSkippedForNonKeylessProvider pins that the
// "empty keyID means do not mark" rule still holds — a mark on a non-keyless
// provider with no key must not create a per-key row.
func TestPerKeyStats_EmptyKeyIDSkippedForNonKeylessProvider(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.OpenAI, "")

	stats := s.Stats()
	if len(stats.PerKey) != 0 {
		t.Fatalf("expected no per-key rows, got %v", stats.PerKey)
	}
}

// TestPerKeyStats_SuppressionBumpsPerKeyCounter pins the second leg: a key
// vetoed by the pool filter is credited with the suppression.
func TestPerKeyStats_SuppressionBumpsPerKeyCounter(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "hot", "", time.Minute, CooldownKindRateLimit, schemas.CooldownScopeKey)

	filter := s.AsFilter(nil)
	keys := []schemas.Key{{ID: "hot"}, {ID: "cold"}}
	if _, err := filter(nil, schemas.OpenAI, "gpt-4o", keys); err != nil {
		t.Fatalf("filter: %v", err)
	}
	if _, err := filter(nil, schemas.OpenAI, "gpt-4o", keys); err != nil {
		t.Fatalf("filter: %v", err)
	}

	stats := s.Stats()
	row, ok := stats.PerKey["openai::hot"]
	if !ok {
		t.Fatalf("expected a per-key row for openai::hot, got %v", stats.PerKey)
	}
	if row.SuppressedCount != 2 {
		t.Errorf("openai::hot suppressed_count = %d, want 2", row.SuppressedCount)
	}
	if row.MarkCount != 1 {
		t.Errorf("openai::hot mark_count = %d, want 1", row.MarkCount)
	}
	// The key that was never vetoed must not appear at all.
	if _, ok := stats.PerKey["openai::cold"]; ok {
		t.Error("a key that was never suppressed must not get a per-key row")
	}
}

// TestPerKeyStats_SuccessBumpsPerKeyCounter pins the third leg: a request
// that succeeded is credited to the key that served it.
func TestPerKeyStats_SuccessBumpsPerKeyCounter(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	ctx := newPerKeyTrailCtx("k1")
	if _, _, err := p.PostLLMHook(ctx, newPerKeyResponse(schemas.OpenAI), nil); err != nil {
		t.Fatalf("PostLLMHook: %v", err)
	}

	stats := p.Stats()
	row, ok := stats.PerKey["openai::k1"]
	if !ok {
		t.Fatalf("expected a per-key row for openai::k1, got %v", stats.PerKey)
	}
	if row.SuccessCount != 1 {
		t.Errorf("openai::k1 success_count = %d, want 1", row.SuccessCount)
	}
	if row.LastSuccessAt == nil {
		t.Error("expected last_success_at to be set after a success")
	}
}

// TestPerKeyStats_SuccessWithoutKeyIsNotCounted pins that a success whose
// key cannot be resolved is dropped instead of being attributed to some
// arbitrary key (or to the whole provider).
func TestPerKeyStats_SuccessWithoutKeyIsNotCounted(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	// No attempt trail at all → no key identity.
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	if _, _, err := p.PostLLMHook(ctx, newPerKeyResponse(schemas.OpenAI), nil); err != nil {
		t.Fatalf("PostLLMHook: %v", err)
	}

	if stats := p.Stats(); len(stats.PerKey) != 0 {
		t.Fatalf("expected no per-key rows, got %v", stats.PerKey)
	}
}

// TestPerKeyStats_KeylessProviderUsesProviderSentinel pins that a keyless
// provider (whose sentinel key has an empty ID) still gets a per-key row,
// keyed "<provider>::" exactly like the cooldown map's sentinel entry.
func TestPerKeyStats_KeylessProviderUsesProviderSentinel(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.Mark(schemas.Opencode, "")

	stats := s.Stats()
	if _, ok := stats.PerKey["opencode::"]; !ok {
		t.Fatalf("expected a per-key row for opencode::, got %v", stats.PerKey)
	}
}

// TestPerKeyStats_StatsJSONContainsPerKey pins the wire shape: the stats
// document carries a "per_key" object whose keys are "<provider>::<keyID>"
// and whose rows expose the three counters.
func TestPerKeyStats_StatsJSONContainsPerKey(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.MarkWithTTL(schemas.OpenAI, "k1", "", time.Minute, CooldownKindRateLimit, schemas.CooldownScopeKey)

	filter := s.AsFilter(nil)
	if _, err := filter(nil, schemas.OpenAI, "gpt-4o", []schemas.Key{{ID: "k1"}}); err != nil {
		t.Fatalf("filter: %v", err)
	}

	raw, err := json.Marshal(s.Stats())
	if err != nil {
		t.Fatalf("marshal stats: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"per_key":{`) {
		t.Fatalf("expected a per_key block in %s", body)
	}
	if !strings.Contains(body, `"openai::k1":{`) {
		t.Fatalf("expected the openai::k1 row in %s", body)
	}
	for _, want := range []string{
		`"mark_count":1`,
		`"suppressed_count":1`,
		`"success_count":0`,
		`"last_mark_at":`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in %s", want, body)
		}
	}
}

// TestPerKeyStats_CooldownRemainingMatchesSnapshot pins that the remaining
// cooldown reported per key is the one the state snapshot reports — the
// per-key view must not disagree with GET /state.
func TestPerKeyStats_CooldownRemainingMatchesSnapshot(t *testing.T) {
	s := NewCooldownState(time.Minute)
	// Pin the clock so Snapshot() and Stats() observe the same instant; the
	// point of the assertion is their agreement, not clock arithmetic.
	fixed := time.Now()
	s.now = func() time.Time { return fixed }

	s.MarkWithTTL(schemas.OpenAI, "k1", "", 90*time.Second, CooldownKindRateLimit, schemas.CooldownScopeKey)

	snapshot := s.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("expected 1 snapshot entry, got %d", len(snapshot))
	}
	row, ok := s.Stats().PerKey["openai::k1"]
	if !ok {
		t.Fatal("expected a per-key row for openai::k1")
	}
	if row.CooldownRemaining != snapshot[0].Remaining {
		t.Fatalf("cooldown_remaining = %v, want %v (the snapshot's remaining)", row.CooldownRemaining, snapshot[0].Remaining)
	}
}

// TestPerKeyStats_CooldownRemainingZeroWhenNotCooling pins that a key with
// counters but no active cooldown reports no remaining time.
func TestPerKeyStats_CooldownRemainingZeroWhenNotCooling(t *testing.T) {
	s := NewCooldownState(20 * time.Millisecond)
	s.Mark(schemas.OpenAI, "k1")
	time.Sleep(40 * time.Millisecond)

	row, ok := s.Stats().PerKey["openai::k1"]
	if !ok {
		t.Fatal("expected a per-key row for openai::k1")
	}
	if row.CooldownRemaining != 0 {
		t.Fatalf("cooldown_remaining = %v, want 0", row.CooldownRemaining)
	}
}

// TestPerKeyStats_ConcurrentUpdatesAreRaceFree exercises the three counter
// paths concurrently. Run under -race it pins that the shared map is guarded
// correctly; the counts pin that no update is lost.
func TestPerKeyStats_ConcurrentUpdatesAreRaceFree(t *testing.T) {
	p := NewPlugin(nil)
	defer p.Cleanup()

	const workers = 16
	const rounds = 20

	// Phase 1 — concurrent marks on the same key.
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				p.State.MarkWithTTL(schemas.OpenAI, "k1", "", time.Minute, CooldownKindRateLimit, schemas.CooldownScopeKey)
			}
		}()
	}
	wg.Wait()

	// Phase 2 — concurrent suppressions of the cooled key.
	filter := p.State.AsFilter(nil)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				if _, err := filter(nil, schemas.OpenAI, "gpt-4o", []schemas.Key{{ID: "k1"}}); err != nil {
					t.Errorf("filter: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	// Phase 3 — concurrent successes on the same key.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				ctx := newPerKeyTrailCtx("k1")
				if _, _, err := p.PostLLMHook(ctx, newPerKeyResponse(schemas.OpenAI), nil); err != nil {
					t.Errorf("PostLLMHook: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	row, ok := p.Stats().PerKey["openai::k1"]
	if !ok {
		t.Fatalf("expected a per-key row for openai::k1, got %v", p.Stats().PerKey)
	}
	want := uint64(workers * rounds)
	if row.MarkCount != want {
		t.Errorf("mark_count = %d, want %d", row.MarkCount, want)
	}
	if row.SuppressedCount != want {
		t.Errorf("suppressed_count = %d, want %d", row.SuppressedCount, want)
	}
	if row.SuccessCount != want {
		t.Errorf("success_count = %d, want %d", row.SuccessCount, want)
	}
}

// TestPerKeyStats_GCPrunesIdleRows pins that the per-key table is bounded:
// a row with no activity for longer than the retention window is dropped by
// the same GC tick that prunes the escalation tables.
func TestPerKeyStats_GCPrunesIdleRows(t *testing.T) {
	s := NewCooldownState(time.Minute)

	var fakeNow atomic.Int64
	fakeNow.Store(time.Now().UnixNano())
	s.now = func() time.Time { return time.Unix(0, fakeNow.Load()) }

	s.Mark(schemas.OpenAI, "k1")
	if _, ok := s.Stats().PerKey["openai::k1"]; !ok {
		t.Fatal("expected a per-key row for openai::k1 before the GC runs")
	}

	// Move past the retention window so the row becomes prunable.
	fakeNow.Store(fakeNow.Load() + int64(perKeyStatsRetention+time.Minute))

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runGCLocked(nil, 5*time.Millisecond, stop)
	}()
	time.Sleep(60 * time.Millisecond)
	close(stop)
	<-done

	if stats := s.Stats(); len(stats.PerKey) != 0 {
		t.Fatalf("expected the idle per-key row to be pruned, got %v", stats.PerKey)
	}
}

// TestPerKeyStats_GCKeepsActiveRows pins the other half of the retention
// rule: a row that is still active is never dropped, no matter how long the
// GC has been running.
func TestPerKeyStats_GCKeepsActiveRows(t *testing.T) {
	s := NewCooldownState(time.Minute)

	var fakeNow atomic.Int64
	fakeNow.Store(time.Now().UnixNano())
	s.now = func() time.Time { return time.Unix(0, fakeNow.Load()) }

	s.Mark(schemas.OpenAI, "k1")

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runGCLocked(nil, 5*time.Millisecond, stop)
	}()
	time.Sleep(40 * time.Millisecond)
	close(stop)
	<-done

	row, ok := s.Stats().PerKey["openai::k1"]
	if !ok {
		t.Fatal("expected the active per-key row to survive the GC")
	}
	if row.MarkCount != 1 {
		t.Fatalf("mark_count = %d, want 1", row.MarkCount)
	}
}
