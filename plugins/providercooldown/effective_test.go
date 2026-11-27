package providercooldown

import (
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// policyCtx returns a context carrying the per-provider CooldownPolicy, the
// way core stamps it before every attempt.
func policyCtx(policy *schemas.CooldownPolicy) *schemas.BifrostContext {
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyCooldownPolicy, policy)
	return ctx
}

// withPolicy adds the per-provider policy stamp to an existing context.
func withPolicy(ctx *schemas.BifrostContext, policy *schemas.CooldownPolicy) *schemas.BifrostContext {
	ctx.SetValue(schemas.BifrostContextKeyCooldownPolicy, policy)
	return ctx
}

// spend posts one successful terminal request through PostLLMHook so the
// capacity gate / learner observer account its usage for the provider the
// stamped policy belongs to.
func spend(t *testing.T, p *CooldownPlugin, policy *schemas.CooldownPolicy, provider schemas.ModelProvider, model string, tokens int) {
	t.Helper()
	if _, _, err := p.PostLLMHook(policyCtx(policy), capacityResp(provider, model, tokens), nil); err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}
}

// checkCapacity runs PreProviderHook for (provider, model) under the given
// policy and returns the short-circuit it produced (nil = allowed).
func checkCapacity(p *CooldownPlugin, policy *schemas.CooldownPolicy, provider schemas.ModelProvider, model string) *schemas.LLMPluginShortCircuit {
	ctx := withPolicy(capacityCtx(provider, schemas.Key{ID: "key-1"}), policy)
	_, sc, _ := p.PreProviderHook(ctx, capacityReq(provider, model))
	return sc
}

func TestCapabilityResolutionMatrix(t *testing.T) {
	s := NewCooldownState(time.Minute)
	s.enableCapacity(&CapacityConfig{Enabled: true, TokenLimit: 1000, RequestLimit: 7, WindowSeconds: 120})
	s.enableEscalation(&EscalationConfig{Enabled: true, MaxMultiplier: 2})
	s.enableLearning(&LearningConfig{Enabled: true, WindowSeconds: 120, MaxSamples: 40, MinSamples: 3})

	override := &schemas.CooldownPolicy{
		Capacity:   &schemas.CooldownCapacityConfig{Enabled: true, TokenLimit: 5, WindowSeconds: 30},
		Escalation: &schemas.CooldownEscalationConfig{Enabled: true, MaxMultiplier: 4},
		Learning:   &schemas.CooldownLearningConfig{Enabled: true, WindowSeconds: 600, MaxSamples: 20, MinSamples: 1},
	}
	disabled := &schemas.CooldownPolicy{
		Capacity:   &schemas.CooldownCapacityConfig{Enabled: false, TokenLimit: 9999},
		Escalation: &schemas.CooldownEscalationConfig{Enabled: false, MaxMultiplier: 8},
		Learning:   &schemas.CooldownLearningConfig{Enabled: false, WindowSeconds: 60, MinSamples: 1},
	}

	t.Run("absent section inherits the plugin default", func(t *testing.T) {
		g := s.gateFor(schemas.OpenAI, nil)
		if g == nil || g.tokenLimit != 1000 || g.requestLimit != 7 || g.windowSeconds != 120 {
			t.Fatalf("default gate = %+v, want token=1000 request=7 window=120", g)
		}
		if mm, ok := s.defaults.escalationMaxMultiplier(nil); !ok || mm != 2 {
			t.Fatalf("default escalation = (%d, %v), want (2, true)", mm, ok)
		}
		l := s.learnerFor(schemas.OpenAI, nil)
		if l == nil || l.windowSeconds != 120 || l.maxSamples != 40 || l.minSamples != 3 {
			t.Fatalf("default learner = %+v, want window=120 max=40 min=3", l)
		}

		// An explicit provider with no relevant section still inherits.
		g = s.gateFor(schemas.Anthropic, &schemas.CooldownPolicy{
			RateLimit: &schemas.CooldownPolicyRule{TTLSeconds: 5},
		})
		if g == nil || g.tokenLimit != 1000 {
			t.Fatalf("unrelated policy must inherit the default capacity, got %+v", g)
		}
	})

	t.Run("own section replaces the default wholesale", func(t *testing.T) {
		g := s.gateFor(schemas.OpenAI, override)
		if g == nil || g.tokenLimit != 5 || g.windowSeconds != 30 {
			t.Fatalf("override gate = %+v, want token=5 window=30", g)
		}
		// No field-level merge: the default's request_limit must be gone.
		if g.requestLimit != 0 {
			t.Fatalf("override requestLimit = %d, want 0 (whole-section replacement)", g.requestLimit)
		}
		if mm, ok := s.defaults.escalationMaxMultiplier(override); !ok || mm != 4 {
			t.Fatalf("override escalation = (%d, %v), want (4, true)", mm, ok)
		}
		l := s.learnerFor(schemas.OpenAI, override)
		if l == nil || l.windowSeconds != 600 || l.maxSamples != 20 || l.minSamples != 1 {
			t.Fatalf("override learner = %+v, want window=600 max=20 min=1", l)
		}
	})

	t.Run("enabled false explicitly disables regardless of the default", func(t *testing.T) {
		if g := s.gateFor(schemas.OpenAI, disabled); g != nil {
			t.Fatalf("disabled capacity must resolve to nil, got %+v", g)
		}
		if mm, ok := s.defaults.escalationMaxMultiplier(disabled); ok {
			t.Fatalf("disabled escalation resolved to multiplier %d, want off", mm)
		}
		if l := s.learnerFor(schemas.OpenAI, disabled); l != nil {
			t.Fatalf("disabled learning must resolve to nil, got %+v", l)
		}
	})

	t.Run("resolution depends only on the provider's policy", func(t *testing.T) {
		if g := s.gateFor(schemas.Anthropic, nil); g == nil || g.tokenLimit != 1000 {
			t.Fatalf("anthropic without a section = %+v, want the default 1000", g)
		}
		if g := s.gateFor(schemas.OpenAI, override); g == nil || g.tokenLimit != 5 {
			t.Fatalf("openai with a section = %+v, want 5", g)
		}
	})
}

func TestCapacityGatePerProviderLimits(t *testing.T) {
	clk := newFakeClock()
	p := &CooldownPlugin{State: NewCooldownState(5 * time.Minute)}
	p.State.now = clk.Now

	// A is deliberately tiny (5 tokens / 10s), B is generous (500 / 60s).
	polA := &schemas.CooldownPolicy{Capacity: &schemas.CooldownCapacityConfig{
		Enabled: true, TokenLimit: 5, WindowSeconds: 10, Headroom: 1,
	}}
	polB := &schemas.CooldownPolicy{Capacity: &schemas.CooldownCapacityConfig{
		Enabled: true, TokenLimit: 500, WindowSeconds: 60, Headroom: 1,
	}}

	spend(t, p, polA, schemas.OpenAI, "gpt-4o", 5)
	spend(t, p, polB, schemas.Anthropic, "claude-sonnet-4", 5)

	// A is spent → blocked, with a retry hint measured in its own 10s window.
	sc := checkCapacity(p, polA, schemas.OpenAI, "gpt-4o")
	if sc == nil {
		t.Fatal("provider A must be blocked once its own token_limit is reached")
	}
	if got := sc.Error.ExtraFields.RetryAfterSeconds; got != 10 {
		t.Fatalf("A retry-after = %ds, want its 10s window", got)
	}
	if !contains(sc.Error.Error.Message, "10s budget") {
		t.Fatalf("A message = %q, want the 10s window in it", sc.Error.Error.Message)
	}

	// B has only spent 5 of 500 → still allowed, so A's gate does not leak.
	if sc := checkCapacity(p, polB, schemas.Anthropic, "claude-sonnet-4"); sc != nil {
		t.Fatalf("provider B must not be gated by A's budget, got %q", sc.Error.Error.Message)
	}

	// Spend B up: it is blocked by its own 500/60s budget, not A's.
	for i := 0; i < 5; i++ {
		spend(t, p, polB, schemas.Anthropic, "claude-sonnet-4", 99)
	}
	sc = checkCapacity(p, polB, schemas.Anthropic, "claude-sonnet-4")
	if sc == nil {
		t.Fatal("provider B must be blocked once its own token_limit is reached")
	}
	if got := sc.Error.ExtraFields.RetryAfterSeconds; got != 60 {
		t.Fatalf("B retry-after = %ds, want its 60s window", got)
	}
	if !contains(sc.Error.Error.Message, "60s budget") {
		t.Fatalf("B message = %q, want the 60s window in it", sc.Error.Error.Message)
	}
}

func TestCapacityGateRuntimeReconfigurePerProvider(t *testing.T) {
	clk := newFakeClock()
	p := &CooldownPlugin{State: NewCooldownState(5 * time.Minute)}
	p.State.now = clk.Now

	polA := &schemas.CooldownPolicy{Capacity: &schemas.CooldownCapacityConfig{
		Enabled: true, TokenLimit: 100, WindowSeconds: 60, Headroom: 1,
	}}
	polB := &schemas.CooldownPolicy{Capacity: &schemas.CooldownCapacityConfig{
		Enabled: true, TokenLimit: 1000, WindowSeconds: 60, Headroom: 1,
	}}

	spend(t, p, polA, schemas.OpenAI, "gpt-4o", 100)
	spend(t, p, polB, schemas.Anthropic, "claude-sonnet-4", 300)

	gateA := p.State.gateFor(schemas.OpenAI, polA)
	gateB := p.State.gateFor(schemas.Anthropic, polB)
	if gateA == nil || gateB == nil {
		t.Fatal("both providers must have a gate")
	}
	if sc := checkCapacity(p, polA, schemas.OpenAI, "gpt-4o"); sc == nil {
		t.Fatal("A must be blocked at its 100-token limit")
	}

	// Raise A's limit at runtime through the policy the next request carries.
	polA2 := &schemas.CooldownPolicy{Capacity: &schemas.CooldownCapacityConfig{
		Enabled: true, TokenLimit: 1000, WindowSeconds: 60, Headroom: 1,
	}}
	newGateA := p.State.gateFor(schemas.OpenAI, polA2)
	if newGateA == gateA {
		t.Fatal("a changed capacity config must rebuild the provider's gate")
	}
	if sc := checkCapacity(p, polA2, schemas.OpenAI, "gpt-4o"); sc != nil {
		t.Fatalf("A must be allowed after raising its limit, got %q", sc.Error.Error.Message)
	}

	// B is untouched: same gate object, same accounted usage.
	if got := p.State.gateFor(schemas.Anthropic, polB); got != gateB {
		t.Fatal("A's reconfiguration must not rebuild B's gate")
	}
	if tokens, _ := gateB.usage(schemas.Anthropic, "claude-sonnet-4"); tokens != 300 {
		t.Fatalf("B usage = %d, want 300 (must survive A's reconfiguration)", tokens)
	}
}

func TestEscalationPerProviderMultiplier(t *testing.T) {
	clk := newFakeClock()
	s := NewCooldownState(30 * time.Second)
	s.now = clk.Now

	polA := &schemas.CooldownPolicy{Escalation: &schemas.CooldownEscalationConfig{Enabled: true, MaxMultiplier: 4}}
	polB := &schemas.CooldownPolicy{Escalation: &schemas.CooldownEscalationConfig{Enabled: true, MaxMultiplier: 1}}

	// Same key and model, different providers: the counters must not mix.
	wantA := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}
	for i, want := range wantA {
		s.markWithPolicy(polA, schemas.OpenAI, "shared", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
		if got := recordedTTL(t, s, schemas.OpenAI, "shared", "gpt-4o", schemas.CooldownScopeModel); got != want {
			t.Fatalf("A mark %d: TTL = %v, want %v", i+1, got, want)
		}
		clk.Advance(time.Second)
	}

	// B configures 1x, so its own marks never grow even though A escalated.
	for i := 0; i < 3; i++ {
		s.markWithPolicy(polB, schemas.Anthropic, "shared", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
		if got := recordedTTL(t, s, schemas.Anthropic, "shared", "gpt-4o", schemas.CooldownScopeModel); got != 30*time.Second {
			t.Fatalf("B mark %d: TTL = %v, want the un-escalated 30s", i+1, got)
		}
		clk.Advance(time.Second)
	}

	// A provider without any escalation section keeps the plain TTL and does
	// not create counters, even though other providers escalated.
	s.markWithPolicy(nil, schemas.Gemini, "shared", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	if got := recordedTTL(t, s, schemas.Gemini, "shared", "gpt-4o", schemas.CooldownScopeModel); got != 30*time.Second {
		t.Fatalf("Gemini mark: TTL = %v, want the plain 30s", got)
	}
	s.mu.RLock()
	_, hasGemini := s.escalation.entries[escalationKey{provider: schemas.Gemini, keyID: "shared", model: "gpt-4o", scope: schemas.CooldownScopeModel}]
	s.mu.RUnlock()
	if hasGemini {
		t.Fatal("a provider with escalation off must not get a counter entry")
	}
}

func TestEscalationStateNotAllocatedWhenEveryProviderIsOff(t *testing.T) {
	clk := newFakeClock()
	s := NewCooldownState(time.Minute)
	s.now = clk.Now

	// No policy, then an explicit disabled section: neither may allocate.
	s.markWithPolicy(nil, schemas.OpenAI, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)
	s.markWithPolicy(&schemas.CooldownPolicy{
		Escalation: &schemas.CooldownEscalationConfig{Enabled: false, MaxMultiplier: 8},
	}, schemas.Anthropic, "key-1", "gpt-4o", 0, CooldownKindRateLimit, schemas.CooldownScopeModel)

	s.mu.RLock()
	es := s.escalation
	s.mu.RUnlock()
	if es != nil {
		t.Fatalf("escalation state must stay nil when no provider enables it, got %+v", es)
	}
	if got := recordedTTL(t, s, schemas.OpenAI, "key-1", "gpt-4o", schemas.CooldownScopeModel); got != time.Minute {
		t.Fatalf("TTL = %v, want the unchanged 1m", got)
	}
}

func TestLearningPerProviderWindow(t *testing.T) {
	clk := newFakeClock()
	p := &CooldownPlugin{State: NewCooldownState(5 * time.Minute)}
	p.State.now = clk.Now

	// The stamped policy must still carry a rate-limit rule: the marker uses
	// the whole policy (not just the learning section) to classify the error.
	polA := &schemas.CooldownPolicy{
		RateLimit: schemas.DefaultCooldownPolicy(schemas.OpenAI).RateLimit,
		Learning: &schemas.CooldownLearningConfig{
			Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 1,
		},
	}
	polB := &schemas.CooldownPolicy{
		RateLimit: schemas.DefaultCooldownPolicy(schemas.Anthropic).RateLimit,
		Learning: &schemas.CooldownLearningConfig{
			Enabled: true, WindowSeconds: 3600, MaxSamples: 50, MinSamples: 1,
		},
	}

	spend(t, p, polA, schemas.OpenAI, "gpt-4o", 100)
	spend(t, p, polB, schemas.Anthropic, "claude-sonnet-4", 100)
	clk.Advance(120 * time.Second)
	// A's 60s window has slid past the first spend; B's 3600s window has not.
	spend(t, p, polA, schemas.OpenAI, "gpt-4o", 100)
	spend(t, p, polB, schemas.Anthropic, "claude-sonnet-4", 200)

	marker := p.State.AsMarker(nil)
	marker(policyCtx(polA), schemas.OpenAI, "key-a", "key-a", "gpt-4o", learnRateLimitErr())
	marker(policyCtx(polB), schemas.Anthropic, "key-b", "key-b", "claude-sonnet-4", learnRateLimitErr())

	entryA := learnLimitsFor(t, p, schemas.OpenAI, "gpt-4o")
	if entryA.WindowSeconds != 60 {
		t.Fatalf("A window = %d, want 60", entryA.WindowSeconds)
	}
	if entryA.LowerBound != 100 {
		t.Fatalf("A lower bound = %d, want 100 (the pre-120s spend must be dropped)", entryA.LowerBound)
	}
	entryB := learnLimitsFor(t, p, schemas.Anthropic, "claude-sonnet-4")
	if entryB.WindowSeconds != 3600 {
		t.Fatalf("B window = %d, want 3600", entryB.WindowSeconds)
	}
	if entryB.LowerBound != 300 {
		t.Fatalf("B lower bound = %d, want 300 (both spends are inside 3600s)", entryB.LowerBound)
	}

	// The default learner is off, so the handler-level convenience window is 0
	// while each per-provider entry still reports its own.
	if got := p.LearningWindowSeconds(); got != 0 {
		t.Fatalf("LearningWindowSeconds() = %d, want 0 when learning is off by default", got)
	}
	if got := len(p.LearnedLimits()); got != 2 {
		t.Fatalf("LearnedLimits() returned %d entries, want 2", got)
	}
}
