package providercooldown

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// newCapacityTestState builds a state with the fake clock installed and the
// capacity gate enabled from the given (raw) config. The config is normalized
// the same way ParseConfig would normalize it.
func newCapacityTestState(clk *fakeClock, cfg *CapacityConfig) *CooldownState {
	s := NewCooldownState(5 * time.Minute)
	s.now = clk.Now
	normalized := cfg.Normalized()
	if normalized.Active() {
		s.enableCapacity(normalized)
	}
	return s
}

// newCapacityTestPlugin builds a plugin whose state carries the capacity
// gate, with the fake clock installed. It skips NewPlugin's background GC
// goroutine: the gate needs no pruning (buckets expire lazily on access), and
// avoiding the goroutine keeps the state swap below race-free.
func newCapacityTestPlugin(t *testing.T, clk *fakeClock, cfg *CapacityConfig) *CooldownPlugin {
	t.Helper()
	return &CooldownPlugin{State: newCapacityTestState(clk, cfg)}
}

// capacityReq builds a chat request for (provider, model).
func capacityReq(provider schemas.ModelProvider, model string) *schemas.BifrostRequest {
	return &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: provider,
			Model:    model,
		},
	}
}

// capacityCtx builds a request context with a provider key snapshot, so the
// cooldown hook's key-pool path is satisfied.
func capacityCtx(provider schemas.ModelProvider, keys ...schemas.Key) *schemas.BifrostContext {
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyProviderKeys, map[schemas.ModelProvider][]schemas.Key{
		provider: keys,
	})
	return ctx
}

// capacityResp builds a successful chat response carrying the given usage and
// routing info, mimicking what core hands to PostLLMHook.
func capacityResp(provider schemas.ModelProvider, model string, totalTokens int) *schemas.BifrostResponse {
	return &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			Usage: &schemas.BifrostLLMUsage{TotalTokens: totalTokens},
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType: schemas.ChatCompletionRequest,
				RoutingInfo: schemas.RoutingInfo{Provider: provider, Model: model},
			},
		},
	}
}

// capacityErr builds a terminal error carrying billed usage, mimicking what
// core hands to PostLLMHook on a failed-but-billed attempt.
func capacityErr(provider schemas.ModelProvider, model string, billedTokens int) *schemas.BifrostError {
	return &schemas.BifrostError{
		StatusCode: intPtr(429),
		Type:       strPtr("rate_limit_error"),
		Error: &schemas.ErrorField{
			Message: "upstream returned 429",
			Type:    strPtr("rate_limit_error"),
		},
		ExtraFields: schemas.BifrostErrorExtraFields{
			RequestType: schemas.ChatCompletionRequest,
			RoutingInfo: schemas.RoutingInfo{Provider: provider, Model: model},
			BilledUsage: &schemas.BifrostLLMUsage{TotalTokens: billedTokens},
		},
	}
}

// recordUsage drives the plugin's PostLLMHook so the gate sees a real
// terminal request.
func recordUsage(t *testing.T, p *CooldownPlugin, provider schemas.ModelProvider, model string, tokens int) {
	t.Helper()
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	gotResp, gotErr, err := p.PostLLMHook(ctx, capacityResp(provider, model, tokens), nil)
	if err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}
	if gotErr != nil {
		t.Fatalf("PostLLMHook fabricated an error: %v", gotErr)
	}
	if gotResp == nil {
		t.Fatal("PostLLMHook dropped the response")
	}
}

// ---------------------------------------------------------------------------
// 1. disabled => no gate, no state
// ---------------------------------------------------------------------------

func TestCapacityDisabledCreatesNoState(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:    false,
		TokenLimit: 1000,
		Headroom:   0.9,
	})

	if p.State.capacityGate() != nil {
		t.Fatal("a disabled capacity config must not install a gate")
	}

	// Blast the gate's would-be state through the hooks: nothing must be
	// tracked and nothing must be short-circuited.
	for i := 0; i < 20; i++ {
		recordUsage(t, p, schemas.OpenAI, "gpt-4o", 1000)
	}
	if gate := p.State.capacityGate(); gate != nil {
		if gate.trackedKeys() != 0 {
			t.Fatalf("disabled capacity must not track keys, got %d", gate.trackedKeys())
		}
	}

	ctx := capacityCtx(schemas.OpenAI, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(schemas.OpenAI, "gpt-4o"))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc != nil {
		t.Fatalf("disabled capacity must never short-circuit, got %+v", sc)
	}
}

func TestCapacityEnabledWithoutLimitsIsInert(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:      true,
		TokenLimit:   0,
		RequestLimit: 0,
	})

	if p.State.capacityGate() != nil {
		t.Fatal("a capacity config with no limits must not install a gate")
	}
	for i := 0; i < 10; i++ {
		recordUsage(t, p, schemas.OpenAI, "gpt-4o", 100_000)
	}
	ctx := capacityCtx(schemas.OpenAI, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(schemas.OpenAI, "gpt-4o"))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc != nil {
		t.Fatalf("a limit-less capacity config must never short-circuit, got %+v", sc)
	}
}

// ---------------------------------------------------------------------------
// 2. token budget
// ---------------------------------------------------------------------------

func TestCapacityTokenBudgetShortCircuits(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
		Action:        CapacityActionFallback,
	})

	provider := schemas.OpenAI
	model := "gpt-4o"

	// Just below the threshold: still allowed.
	recordUsage(t, p, provider, model, 899)
	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc != nil {
		t.Fatalf("must not short-circuit below the threshold, got %+v", sc)
	}

	// Crossing the threshold blocks the NEXT request.
	recordUsage(t, p, provider, model, 1)
	ctx2 := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc2, err := p.PreProviderHook(ctx2, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc2 == nil {
		t.Fatal("expected the capacity gate to short-circuit once the budget is spent")
	}
	if !sc2.Silent {
		t.Fatal("the capacity short-circuit must be silent so logging skips the fake row")
	}
	if sc2.Error == nil {
		t.Fatal("the capacity short-circuit must carry a BifrostError")
	}
	if sc2.Error.StatusCode == nil || *sc2.Error.StatusCode != 429 {
		t.Fatalf("expected HTTP 429, got %v", sc2.Error.StatusCode)
	}
	if sc2.Error.Type == nil || *sc2.Error.Type != capacityGateErrorType {
		t.Fatalf("expected type %q, got %v", capacityGateErrorType, sc2.Error.Type)
	}
	if sc2.Error.Error == nil || sc2.Error.Error.Type == nil || *sc2.Error.Error.Type != capacityGateErrorType {
		t.Fatalf("expected Error.Type %q", capacityGateErrorType)
	}
	if sc2.Error.AllowFallbacks == nil || !*sc2.Error.AllowFallbacks {
		t.Fatalf("action=fallback must set AllowFallbacks=true, got %v", sc2.Error.AllowFallbacks)
	}
	if sc2.Error.ExtraFields.RetryAfterSeconds <= 0 {
		t.Fatalf("expected a positive RetryAfterSeconds, got %d", sc2.Error.ExtraFields.RetryAfterSeconds)
	}
	msg := sc2.Error.Error.Message
	if msg == "" {
		t.Fatal("the capacity short-circuit must carry a message")
	}
	for _, want := range []string{"capacity gate", "openai", "gpt-4o", "retry in"} {
		if !contains(msg, want) {
			t.Fatalf("message %q must mention %q", msg, want)
		}
	}
}

func TestCapacityRejectActionBlocksFallbacks(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
		Action:        CapacityActionReject,
	})

	provider := schemas.OpenAI
	recordUsage(t, p, provider, "gpt-4o", 1000)

	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, "gpt-4o"))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc == nil {
		t.Fatal("expected the capacity gate to short-circuit")
	}
	if sc.Error.AllowFallbacks == nil || *sc.Error.AllowFallbacks {
		t.Fatalf("action=reject must set AllowFallbacks=false, got %v", sc.Error.AllowFallbacks)
	}
}

func TestCapacityBilledUsageIsAccounted(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	gotResp, gotErr, err := p.PostLLMHook(ctx, nil, capacityErr(provider, "gpt-4o", 1000))
	if err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}
	if gotErr == nil {
		t.Fatal("PostLLMHook must return the error unchanged")
	}
	if gotResp != nil {
		t.Fatal("PostLLMHook must not fabricate a response on the error path")
	}

	gate := p.State.capacityGate()
	tokens, requests := gate.usage(provider, "gpt-4o")
	if tokens != 1000 {
		t.Fatalf("billed usage must be accounted, got %d tokens", tokens)
	}
	if requests != 1 {
		t.Fatalf("a failed request must still count as one request, got %d", requests)
	}
}

func TestCapacityWithoutUsageStillCountsRequest(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		RequestLimit:  10,
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	// A response without any usage block: tokens unknown, request known.
	resp := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType: schemas.ChatCompletionRequest,
				RoutingInfo: schemas.RoutingInfo{Provider: provider, Model: "gpt-4o"},
			},
		},
	}
	if _, _, err := p.PostLLMHook(ctx, resp, nil); err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}

	gate := p.State.capacityGate()
	tokens, requests := gate.usage(provider, "gpt-4o")
	if tokens != 0 {
		t.Fatalf("unknown usage must be recorded as 0 tokens, got %d", tokens)
	}
	if requests != 1 {
		t.Fatalf("expected the request to be counted, got %d", requests)
	}
}

// ---------------------------------------------------------------------------
// 3. request budget
// ---------------------------------------------------------------------------

func TestCapacityRequestBudgetShortCircuits(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    0, // unlimited
		RequestLimit:  10,
		Headroom:      1.0, // threshold = 10 requests
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	model := "gpt-4o"
	for i := 0; i < 9; i++ {
		recordUsage(t, p, provider, model, 0)
	}
	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc != nil {
		t.Fatalf("must not short-circuit below the request threshold, got %+v", sc)
	}

	recordUsage(t, p, provider, model, 0)
	ctx2 := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc2, err := p.PreProviderHook(ctx2, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc2 == nil {
		t.Fatal("expected the request budget to short-circuit")
	}
	if sc2.Error.ExtraFields.RetryAfterSeconds <= 0 {
		t.Fatalf("expected a positive RetryAfterSeconds, got %d", sc2.Error.ExtraFields.RetryAfterSeconds)
	}
}

// ---------------------------------------------------------------------------
// 4. headroom boundary
// ---------------------------------------------------------------------------

func TestCapacityHeadroomBoundary(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	model := "gpt-4o"
	recordUsage(t, p, provider, model, 899)

	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc != nil {
		t.Fatalf("899 tokens (limit*headroom=900) must not short-circuit, got %+v", sc)
	}

	recordUsage(t, p, provider, model, 1)
	ctx2 := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc2, err := p.PreProviderHook(ctx2, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc2 == nil {
		t.Fatal("900 tokens (limit*headroom=900) must short-circuit")
	}
}

// ---------------------------------------------------------------------------
// 5. window sliding
// ---------------------------------------------------------------------------

func TestCapacityWindowSlidesAndClearsOldBuckets(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	model := "gpt-4o"
	recordUsage(t, p, provider, model, 1000)

	gate := p.State.capacityGate()
	if tokens, _ := gate.usage(provider, model); tokens != 1000 {
		t.Fatalf("expected 1000 accounted tokens, got %d", tokens)
	}

	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc == nil {
		t.Fatal("expected the gate to be engaged before the window slides")
	}

	// Half the window: the original bucket is still live, so the gate stays.
	clk.Advance(30 * time.Second)
	ctx2 := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc2, err := p.PreProviderHook(ctx2, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc2 == nil {
		t.Fatal("expected the gate to stay engaged while the window still holds the usage")
	}

	// The whole window has slid away: the gate re-opens and the stale bucket
	// is gone.
	clk.Advance(31 * time.Second)
	ctx3 := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc3, err := p.PreProviderHook(ctx3, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc3 != nil {
		t.Fatalf("expected the gate to re-open after the window slid, got %+v", sc3)
	}
	tokens, requests := gate.usage(provider, model)
	if tokens != 0 || requests != 0 {
		t.Fatalf("expected the window to be empty after sliding, got %d tokens / %d requests", tokens, requests)
	}
	if gate.liveBuckets(provider, model) != 0 {
		t.Fatalf("expected stale buckets to be cleared, got %d", gate.liveBuckets(provider, model))
	}
}

func TestCapacityRetryAfterReflectsWindowRelease(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	model := "gpt-4o"
	recordUsage(t, p, provider, model, 1000)

	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc == nil {
		t.Fatal("expected the gate to be engaged")
	}
	// The usage sits in the current second, so it only leaves the window one
	// full window later.
	retryAfter := sc.Error.ExtraFields.RetryAfterSeconds
	if retryAfter != 60 {
		t.Fatalf("expected a 60s retry hint, got %d", retryAfter)
	}
}

// ---------------------------------------------------------------------------
// 6. scope
// ---------------------------------------------------------------------------

func TestCapacityScopeProviderSharesAcrossModels(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
		Scope:         CapacityScopeProvider,
	})

	provider := schemas.OpenAI
	recordUsage(t, p, provider, "gpt-4o", 1000)

	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, "gpt-4o-mini"))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc == nil {
		t.Fatal("scope=provider must share one budget across the provider's models")
	}
}

func TestCapacityScopeProviderModelIsolatesModels(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
		Scope:         CapacityScopeProviderModel,
	})

	provider := schemas.OpenAI
	recordUsage(t, p, provider, "gpt-4o", 1000)

	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, "gpt-4o-mini"))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc != nil {
		t.Fatalf("scope=provider_model must not charge another model's budget, got %+v", sc)
	}

	ctx2 := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc2, err := p.PreProviderHook(ctx2, capacityReq(provider, "gpt-4o"))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc2 == nil {
		t.Fatal("scope=provider_model must charge the model that actually spent the budget")
	}
}

// ---------------------------------------------------------------------------
// 7. the gate must not count its own rejections
// ---------------------------------------------------------------------------

func TestCapacityGateDoesNotCountRejectedRequests(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	model := "gpt-4o"
	recordUsage(t, p, provider, model, 1000)

	gate := p.State.capacityGate()
	before, _ := gate.usage(provider, model)

	// A rejected request never reaches the provider, so the framework marks
	// the attempt silent. It must not extend the window that rejected it.
	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc == nil {
		t.Fatal("expected the gate to be engaged")
	}
	ctx.SetValue(schemas.BifrostContextKeySilentLog, true)
	if _, _, err := p.PostLLMHook(ctx, nil, sc.Error); err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}

	after, _ := gate.usage(provider, model)
	if after != before {
		t.Fatalf("a rejected request must not be accounted: %d -> %d tokens", before, after)
	}
}

// ---------------------------------------------------------------------------
// streaming accounting
// ---------------------------------------------------------------------------

func TestCapacityStreamingAccountsOnlyFinalChunk(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		RequestLimit:  100,
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	model := "gpt-4o"
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	streamResp := func(tokens int) *schemas.BifrostResponse {
		return &schemas.BifrostResponse{
			ChatResponse: &schemas.BifrostChatResponse{
				Usage: &schemas.BifrostLLMUsage{TotalTokens: tokens},
				ExtraFields: schemas.BifrostResponseExtraFields{
					RequestType: schemas.ChatCompletionStreamRequest,
					RoutingInfo: schemas.RoutingInfo{Provider: provider, Model: model},
				},
			},
		}
	}

	// Three non-final chunks must not be accounted.
	for i := 0; i < 3; i++ {
		if _, _, err := p.PostLLMHook(ctx, streamResp(0), nil); err != nil {
			t.Fatalf("PostLLMHook returned an error: %v", err)
		}
	}
	gate := p.State.capacityGate()
	if _, requests := gate.usage(provider, model); requests != 0 {
		t.Fatalf("non-final chunks must not be accounted, got %d requests", requests)
	}

	// The terminal chunk (StreamEndIndicator=true) is accounted exactly once,
	// with the usage it carries.
	ctx.SetValue(schemas.BifrostContextKeyStreamEndIndicator, true)
	if _, _, err := p.PostLLMHook(ctx, streamResp(250), nil); err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}
	tokens, requests := gate.usage(provider, model)
	if requests != 1 {
		t.Fatalf("expected exactly one accounted streaming request, got %d", requests)
	}
	if tokens != 250 {
		t.Fatalf("expected the final chunk's usage to be accounted, got %d tokens", tokens)
	}
}

func TestCapacityStreamingErrorCountsOnce(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		RequestLimit:  100,
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	model := "gpt-4o"
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	berr := capacityErr(provider, model, 400)
	berr.ExtraFields.RequestType = schemas.ChatCompletionStreamRequest

	// Not terminal yet: must not be accounted.
	if _, _, err := p.PostLLMHook(ctx, nil, berr); err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}
	gate := p.State.capacityGate()
	if _, requests := gate.usage(provider, model); requests != 0 {
		t.Fatalf("a non-terminal stream error must not be accounted, got %d requests", requests)
	}

	// Terminal error: accounted once, with the billed usage it carries.
	ctx.SetValue(schemas.BifrostContextKeyStreamEndIndicator, true)
	if _, _, err := p.PostLLMHook(ctx, nil, berr); err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}
	tokens, requests := gate.usage(provider, model)
	if requests != 1 {
		t.Fatalf("expected exactly one accounted streaming failure, got %d", requests)
	}
	if tokens != 400 {
		t.Fatalf("expected the billed usage to be accounted, got %d tokens", tokens)
	}
}

func TestCapacityIgnoresResponsesWithoutRoutingInfo(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		RequestLimit:  100,
		WindowSeconds: 60,
	})

	// A plugin-synthesized response (e.g. a semantic-cache hit) carries no
	// RoutingInfo.Provider and must not consume the provider's budget.
	resp := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			Usage: &schemas.BifrostLLMUsage{TotalTokens: 5000},
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType: schemas.ChatCompletionRequest,
			},
		},
	}
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	if _, _, err := p.PostLLMHook(ctx, resp, nil); err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}

	gate := p.State.capacityGate()
	if gate.trackedKeys() != 0 {
		t.Fatalf("a response without routing info must not create accounting state, got %d keys", gate.trackedKeys())
	}
}

func TestCapacityGateErrorNeverMarksCooldown(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		Headroom:      0.9,
		WindowSeconds: 60,
	})
	// A custom quota pattern that WOULD match the gate's message.
	p.quotaPatterns = []string{"exceeded"}

	provider := schemas.OpenAI
	model := "gpt-4o"
	recordUsage(t, p, provider, model, 1000)

	ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
	_, sc, err := p.PreProviderHook(ctx, capacityReq(provider, model))
	if err != nil {
		t.Fatalf("PreProviderHook returned an error: %v", err)
	}
	if sc == nil {
		t.Fatal("expected the gate to be engaged")
	}
	if _, _, err := p.PostLLMHook(ctx, nil, sc.Error); err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}
	if p.State.IsCoolingDown(provider, "key-1") {
		t.Fatal("the capacity gate's own rejection must never mark a cooldown")
	}
}

// ---------------------------------------------------------------------------
// 8. concurrency
// ---------------------------------------------------------------------------

func TestCapacityConcurrentRecordAndCheck(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1_000_000,
		RequestLimit:  1_000_000,
		WindowSeconds: 60,
	})

	provider := schemas.OpenAI
	model := "gpt-4o"

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ctx := schemas.NewBifrostContext(nil, time.Time{})
			_, _, _ = p.PostLLMHook(ctx, capacityResp(provider, model, 10), nil)
		}()
		go func() {
			defer wg.Done()
			ctx := capacityCtx(provider, schemas.Key{ID: "key-1"})
			_, _, _ = p.PreProviderHook(ctx, capacityReq(provider, model))
		}()
	}
	wg.Wait()

	gate := p.State.capacityGate()
	tokens, requests := gate.usage(provider, model)
	if requests != n {
		t.Fatalf("expected %d accounted requests, got %d", n, requests)
	}
	if tokens != int64(n*10) {
		t.Fatalf("expected %d accounted tokens, got %d", n*10, tokens)
	}
}

// ---------------------------------------------------------------------------
// 9. ParseConfig
// ---------------------------------------------------------------------------

func TestParseConfigCapacityDefaults(t *testing.T) {
	c, err := ParseConfig(map[string]any{
		"capacity": map[string]any{
			"enabled":       true,
			"token_limit":   float64(5000),
			"request_limit": float64(100),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Capacity == nil {
		t.Fatal("expected the capacity block to be parsed")
	}
	if !c.Capacity.Enabled {
		t.Fatal("expected enabled=true")
	}
	if c.Capacity.TokenLimit != 5000 {
		t.Fatalf("TokenLimit = %d, want 5000", c.Capacity.TokenLimit)
	}
	if c.Capacity.RequestLimit != 100 {
		t.Fatalf("RequestLimit = %d, want 100", c.Capacity.RequestLimit)
	}
	if c.Capacity.WindowSeconds != defaultCapacityWindowSeconds {
		t.Fatalf("WindowSeconds = %d, want %d", c.Capacity.WindowSeconds, defaultCapacityWindowSeconds)
	}
	if c.Capacity.Headroom != defaultCapacityHeadroom {
		t.Fatalf("Headroom = %v, want %v", c.Capacity.Headroom, defaultCapacityHeadroom)
	}
	if c.Capacity.Scope != CapacityScopeProvider {
		t.Fatalf("Scope = %q, want %q", c.Capacity.Scope, CapacityScopeProvider)
	}
	if c.Capacity.Action != CapacityActionFallback {
		t.Fatalf("Action = %q, want %q", c.Capacity.Action, CapacityActionFallback)
	}
}

func TestParseConfigCapacityClamps(t *testing.T) {
	c, err := ParseConfig(map[string]any{
		"capacity": map[string]any{
			"enabled":        true,
			"token_limit":    float64(1000),
			"window_seconds": float64(5),
			"headroom":       float64(0),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Capacity.WindowSeconds != minCapacityWindowSeconds {
		t.Fatalf("WindowSeconds = %d, want %d", c.Capacity.WindowSeconds, minCapacityWindowSeconds)
	}
	if c.Capacity.Headroom != defaultCapacityHeadroom {
		t.Fatalf("Headroom = %v, want %v", c.Capacity.Headroom, defaultCapacityHeadroom)
	}

	c2, err := ParseConfig(map[string]any{
		"capacity": map[string]any{
			"enabled":        true,
			"token_limit":    float64(1000),
			"window_seconds": float64(9999),
			"headroom":       float64(1.5),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c2.Capacity.WindowSeconds != maxCapacityWindowSeconds {
		t.Fatalf("WindowSeconds = %d, want %d", c2.Capacity.WindowSeconds, maxCapacityWindowSeconds)
	}
	if c2.Capacity.Headroom != maxCapacityHeadroom {
		t.Fatalf("Headroom = %v, want %v", c2.Capacity.Headroom, maxCapacityHeadroom)
	}
}

func TestParseConfigCapacityRejectsUnknownScopeAndAction(t *testing.T) {
	if _, err := ParseConfig(map[string]any{
		"capacity": map[string]any{"scope": "cluster"},
	}); err == nil {
		t.Fatal("expected an error for an unknown capacity.scope")
	}
	if _, err := ParseConfig(map[string]any{
		"capacity": map[string]any{"action": "explode"},
	}); err == nil {
		t.Fatal("expected an error for an unknown capacity.action")
	}
}

func TestParseConfigCapacityRejectsUnknownKey(t *testing.T) {
	if _, err := ParseConfig(map[string]any{
		"capacity": map[string]any{"token_budget": float64(10)},
	}); err == nil {
		t.Fatal("expected an error for an unknown capacity key")
	}
}

func TestParseConfigCapacityRejectsBadTypes(t *testing.T) {
	if _, err := ParseConfig(map[string]any{
		"capacity": map[string]any{"enabled": "yes"},
	}); err == nil {
		t.Fatal("expected an error for a non-boolean capacity.enabled")
	}
	if _, err := ParseConfig(map[string]any{
		"capacity": map[string]any{"token_limit": "many"},
	}); err == nil {
		t.Fatal("expected an error for a non-integer capacity.token_limit")
	}
	if _, err := ParseConfig(map[string]any{
		"capacity": map[string]any{"token_limit": float64(-5)},
	}); err == nil {
		t.Fatal("expected an error for a negative capacity.token_limit")
	}
	if _, err := ParseConfig(map[string]any{
		"capacity": []any{"nope"},
	}); err == nil {
		t.Fatal("expected an error for a non-object capacity block")
	}
}

func TestConfigAsStateInstallsCapacityGate(t *testing.T) {
	c := &Config{
		Capacity: &CapacityConfig{
			Enabled:       true,
			TokenLimit:    1000,
			WindowSeconds: 30,
			Headroom:      0.5,
			Scope:         CapacityScopeProviderModel,
			Action:        CapacityActionReject,
		},
	}
	s := c.AsState(nil)
	gate := s.capacityGate()
	if gate == nil {
		t.Fatal("expected AsState to install the capacity gate")
	}
	if gate.windowSeconds != 30 {
		t.Fatalf("windowSeconds = %d, want 30", gate.windowSeconds)
	}
	if gate.scope != CapacityScopeProviderModel {
		t.Fatalf("scope = %q, want %q", gate.scope, CapacityScopeProviderModel)
	}
	if gate.action != CapacityActionReject {
		t.Fatalf("action = %q, want %q", gate.action, CapacityActionReject)
	}
	if gate.tokenThresh != 500 {
		t.Fatalf("tokenThresh = %d, want 500", gate.tokenThresh)
	}
}

func TestConfigAsStateSkipsDisabledCapacity(t *testing.T) {
	c := &Config{
		Capacity: &CapacityConfig{
			Enabled:    false,
			TokenLimit: 1000,
		},
	}
	s := c.AsState(nil)
	if s.capacityGate() != nil {
		t.Fatal("AsState must not install a gate for a disabled capacity config")
	}
}

// contains is a tiny substring helper (Go 1.26 has strings.Contains, but this
// keeps the test file dependency-free).
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

var _ = fmt.Sprintf
