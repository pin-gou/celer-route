package providercooldown

import (
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// fakeClock is a manually advanced time source for the escalation windows.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// newTestState builds a state with the fake clock installed (before use) and
// escalation enabled or left disabled.
func newTestState(ttl time.Duration, clk *fakeClock, escalationOn bool, maxMultiplier int) *CooldownState {
	s := NewCooldownState(ttl)
	s.now = clk.Now
	if escalationOn {
		s.enableEscalation(&schemas.CooldownEscalationConfig{Enabled: true, MaxMultiplier: maxMultiplier})
	}
	return s
}

// recordedTTL returns the TTL the last mark actually stored for (provider, key,
// model, scope), measured against the state's clock.
func recordedTTL(t *testing.T, s *CooldownState, provider schemas.ModelProvider, keyID, model string, scope schemas.CooldownPolicyScope) time.Duration {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.cooldowns[s.markKey(provider, keyID, model, scope)]
	if !ok {
		t.Fatalf("no cooldown record for %s/%s/%s", provider, keyID, model)
	}
	return rec.expiresAt.Sub(s.nowTime())
}

func TestEscalationDisabledKeepsTTLByteForByte(t *testing.T) {
	clk := newFakeClock()
	s := newTestState(30*time.Second, clk, false, 0)
	provider := schemas.OpenAI

	if s.escalation != nil {
		t.Fatal("escalation must be nil when disabled")
	}

	for i := 1; i <= 6; i++ {
		s.MarkWithTTL(provider, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
		if got := recordedTTL(t, s, provider, "key-1", "gpt-4o", schemas.CooldownScopeModel); got != 30*time.Second {
			t.Fatalf("mark %d: TTL = %v, want 30s (disabled must not change anything)", i, got)
		}
		clk.Advance(5 * time.Second)
	}
}

func TestEscalationDoublesAndCaps(t *testing.T) {
	clk := newFakeClock()
	s := newTestState(10*time.Second, clk, true, 4)
	provider := schemas.OpenAI

	want := []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 40 * time.Second}
	for i, w := range want {
		s.MarkWithTTL(provider, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
		if got := recordedTTL(t, s, provider, "key-1", "gpt-4o", schemas.CooldownScopeModel); got != w {
			t.Fatalf("mark %d: TTL = %v, want %v", i+1, got, w)
		}
		clk.Advance(time.Second)
	}
}

func TestEscalationWindowExpiryResetsCount(t *testing.T) {
	clk := newFakeClock()
	s := newTestState(10*time.Second, clk, true, 4)
	provider := schemas.OpenAI

	s.MarkWithTTL(provider, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, provider, "key-1", "gpt-4o", schemas.CooldownScopeModel); got != 10*time.Second {
		t.Fatalf("first mark: TTL = %v, want 10s", got)
	}

	clk.Advance(10 * time.Second)
	s.MarkWithTTL(provider, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, provider, "key-1", "gpt-4o", schemas.CooldownScopeModel); got != 20*time.Second {
		t.Fatalf("second mark: TTL = %v, want 20s", got)
	}

	// Just past the 300s escalation window since the previous mark: the count
	// must restart at 1, i.e. back to 1x.
	clk.Advance(escalationWindow + time.Second)
	s.MarkWithTTL(provider, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, provider, "key-1", "gpt-4o", schemas.CooldownScopeModel); got != 10*time.Second {
		t.Fatalf("mark after window expiry: TTL = %v, want 10s (count reset)", got)
	}
}

func TestEscalationSuccessClearsCount(t *testing.T) {
	clk := newFakeClock()
	s := newTestState(10*time.Second, clk, true, 4)
	provider := schemas.OpenAI

	s.MarkWithTTL(provider, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	s.MarkWithTTL(provider, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, provider, "key-1", "gpt-4o", schemas.CooldownScopeModel); got != 20*time.Second {
		t.Fatalf("second mark: TTL = %v, want 20s", got)
	}

	// The plugin (not the state) owns the success path; drive it through
	// PostLLMHook so the wiring is covered too. No GC goroutine is started for
	// this struct-literal plugin, matching the direct-construction tests.
	p := &CooldownPlugin{State: s}
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyRoutingInfo, schemas.RoutingInfo{Provider: provider, Model: "gpt-4o"})
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{Attempt: 1, KeyID: "key-1", KeyName: "k-key-1"},
	})
	if _, _, err := p.PostLLMHook(ctx, &schemas.BifrostResponse{}, nil); err != nil {
		t.Fatalf("PostLLMHook success path: %v", err)
	}

	s.MarkWithTTL(provider, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, provider, "key-1", "gpt-4o", schemas.CooldownScopeModel); got != 10*time.Second {
		t.Fatalf("mark after success: TTL = %v, want 10s (count cleared)", got)
	}
}

func TestEscalationStormRaisesStartingMultiplier(t *testing.T) {
	clk := newFakeClock()
	s := newTestState(10*time.Second, clk, true, 4)
	provider := schemas.OpenAI

	// Three distinct keys fail inside the storm window. None of these marks is
	// escalated: the threshold key is counted only for *subsequent* marks.
	for _, keyID := range []string{"key-a", "key-b", "key-c"} {
		s.MarkWithTTL(provider, keyID, "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
		if got := recordedTTL(t, s, provider, keyID, "gpt-4o", schemas.CooldownScopeModel); got != 10*time.Second {
			t.Fatalf("key %s: TTL = %v, want 10s before the storm threshold", keyID, got)
		}
	}

	// Fourth distinct key: the storm signal is active, so even a first mark
	// starts at 2x.
	s.MarkWithTTL(provider, "key-d", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, provider, "key-d", "gpt-4o", schemas.CooldownScopeModel); got != 20*time.Second {
		t.Fatalf("key-d: TTL = %v, want 20s (storm start multiplier 2x)", got)
	}

	// Let the storm window lapse, then a brand-new key must be back to 1x.
	clk.Advance(escalationStormWindow + time.Second)
	s.MarkWithTTL(provider, "key-e", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, provider, "key-e", "gpt-4o", schemas.CooldownScopeModel); got != 10*time.Second {
		t.Fatalf("key-e: TTL = %v, want 10s after storm window expired", got)
	}
}

func TestEscalationTTLCeiling(t *testing.T) {
	clk := newFakeClock()
	s := newTestState(10*time.Minute, clk, true, 16)
	provider := schemas.OpenAI

	var last time.Duration
	for i := 0; i < 10; i++ {
		s.MarkWithTTL(provider, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
		last = recordedTTL(t, s, provider, "key-1", "gpt-4o", schemas.CooldownScopeModel)
		if last > escalationMaxTTL {
			t.Fatalf("mark %d: TTL = %v exceeds 1h ceiling", i+1, last)
		}
	}
	if last != escalationMaxTTL {
		t.Fatalf("final TTL = %v, want %v (10min * 16x capped)", last, escalationMaxTTL)
	}
}

func TestParseConfigEscalation(t *testing.T) {
	t.Run("absent leaves nil", func(t *testing.T) {
		c, err := ParseConfig(map[string]any{})
		if err != nil {
			t.Fatalf("ParseConfig: %v", err)
		}
		if c.Escalation != nil {
			t.Fatalf("Escalation = %+v, want nil", c.Escalation)
		}
	})

	t.Run("parses enabled and default multiplier", func(t *testing.T) {
		c, err := ParseConfig(map[string]any{
			"escalation": map[string]any{
				"enabled":        true,
				"max_multiplier": float64(0),
			},
		})
		if err != nil {
			t.Fatalf("ParseConfig: %v", err)
		}
		if c.Escalation == nil || !c.Escalation.Enabled {
			t.Fatalf("Escalation = %+v, want enabled", c.Escalation)
		}
		if c.Escalation.MaxMultiplier != 4 {
			t.Fatalf("MaxMultiplier = %d, want default 4", c.Escalation.MaxMultiplier)
		}
	})

	t.Run("clamps multiplier above bound", func(t *testing.T) {
		c, err := ParseConfig(map[string]any{
			"escalation": map[string]any{
				"enabled":        false,
				"max_multiplier": float64(999),
			},
		})
		if err != nil {
			t.Fatalf("ParseConfig: %v", err)
		}
		if c.Escalation == nil || c.Escalation.Enabled {
			t.Fatalf("Escalation = %+v, want disabled", c.Escalation)
		}
		if c.Escalation.MaxMultiplier != 16 {
			t.Fatalf("MaxMultiplier = %d, want clamp 16", c.Escalation.MaxMultiplier)
		}
	})

	t.Run("rejects bad shapes", func(t *testing.T) {
		cases := []map[string]any{
			{"escalation": "on"},
			{"escalation": map[string]any{"enabled": "yes"}},
			{"escalation": map[string]any{"max_multiplier": "many"}},
			{"escalation": map[string]any{"max_multiplers": float64(4)}},
		}
		for _, raw := range cases {
			if _, err := ParseConfig(raw); err == nil {
				t.Fatalf("ParseConfig(%v) succeeded, want error", raw)
			}
		}
	})

	t.Run("AsState only enables when requested", func(t *testing.T) {
		off := (&Config{Escalation: &EscalationConfig{Enabled: false, MaxMultiplier: 8}}).AsState(nil)
		if off.escalation != nil {
			t.Fatal("disabled escalation must leave the state inert")
		}
		on := (&Config{Escalation: &EscalationConfig{Enabled: true, MaxMultiplier: 0}}).AsState(nil)
		if on.escalation == nil {
			t.Fatal("enabled escalation must build the state")
		}
		if on.escalation.maxMultiplier != 4 {
			t.Fatalf("maxMultiplier = %d, want normalized default 4", on.escalation.maxMultiplier)
		}
	})
}

func TestEscalationConcurrentMarks(t *testing.T) {
	clk := newFakeClock()
	s := newTestState(10*time.Second, clk, true, 4)
	provider := schemas.OpenAI

	const goroutines = 16
	const marksPerGoroutine = 32
	const distinctKeys = 8

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < marksPerGoroutine; i++ {
				// Half the goroutines hammer one shared key, the rest spread
				// across distinct keys, so both the per-key counter and the
				// storm bucket are exercised concurrently.
				if g%2 == 0 {
					s.MarkWithTTL(provider, "shared", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
					continue
				}
				keyID := "key-" + string(rune('a'+g%distinctKeys))
				s.MarkWithTTL(provider, keyID, "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
			}
		}(g)
	}
	wg.Wait()

	// Bounded: at most one escalation entry per distinct (key, model, scope)
	// tuple, and the storm bucket holds at most the distinct keys seen.
	s.mu.RLock()
	defer s.mu.RUnlock()
	if got := len(s.escalation.entries); got > distinctKeys+1 {
		t.Fatalf("escalation entries = %d, want <= %d", got, distinctKeys+1)
	}
	bucket := s.escalation.storm[stormBucketKey{provider: provider, model: "gpt-4o"}]
	if got := len(bucket.members); got > distinctKeys+1 {
		t.Fatalf("storm bucket = %d keys, want <= %d", got, distinctKeys+1)
	}
}
