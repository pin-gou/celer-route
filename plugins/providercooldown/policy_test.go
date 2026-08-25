package providercooldown

import (
	"testing"
	"time"

	"github.com/pin-gou/pg-gateway/core/schemas"
)

// makeCtx stamps a CooldownPolicy onto a fresh context so classify sees it.
func makeCtx(t *testing.T, policy *schemas.CooldownPolicy) *schemas.BifrostContext {
	t.Helper()
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	if policy != nil {
		ctx.SetValue(schemas.BifrostContextKeyCooldownPolicy, policy)
	}
	return ctx
}

func newErr(status int, typ, code, message string) *schemas.BifrostError {
	return newErrOn(status, typ, code, message, "")
}

func newErrOn(status int, typ, code, message, provider string) *schemas.BifrostError {
	e := &schemas.BifrostError{
		IsBifrostError: true,
	}
	if status > 0 {
		e.StatusCode = intPtr(status)
	}
	if typ != "" || code != "" || message != "" {
		e.Error = &schemas.ErrorField{}
		if typ != "" {
			e.Error.Type = strPtr(typ)
		}
		if code != "" {
			e.Error.Code = strPtr(code)
		}
		if message != "" {
			e.Error.Message = message
		}
	}
	if provider != "" {
		e.ExtraFields.RoutingInfo.Provider = schemas.ModelProvider(provider)
	}
	return e
}

// withTrail stamps the CooldownPolicy plus an AttemptTrail with one key
// record onto a fresh context. lastAttemptProviderAndKey needs the trail to
// resolve a keyID; without it classify returns ok=false regardless of the
// policy match.
func withTrail(t *testing.T, policy *schemas.CooldownPolicy) *schemas.BifrostContext {
	t.Helper()
	ctx := makeCtx(t, policy)
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{KeyID: "k1", KeyName: "key-1"},
	})
	return ctx
}

func TestClassify_QuotaRule_FiresOnSensenovaWorkspaceQuota(t *testing.T) {
	plugin := NewPlugin(nil)
	ctx := withTrail(t, schemas.DefaultCooldownPolicy(schemas.Sensenova))
	// 429 with "Workspace allocated quota exceeded" + code=insufficient_quota
	// is the exact pattern observed on staging: sensenova returns 429 +
	// invalid_request_error + insufficient_quota. RoutingInfo.Provider is
	// stamped by core so lastAttemptProviderAndKey can resolve the attempt's
	// provider even when no key rotates.
	err := newErrOn(429, "invalid_request_error", "insufficient_quota",
		"Workspace allocated quota exceeded, please increase your quota limit.",
		string(schemas.Sensenova))

	ttl, keyID, _, _, ok := plugin.classify(ctx, err)
	if !ok {
		t.Fatal("expected classify to return ok=true for sensenova quota error")
	}
	if keyID == "" {
		t.Fatal("expected non-empty keyID when AttemptTrail has a key")
	}
	want := time.Duration(schemas.SensenovaQuotaTTLSeconds) * time.Second
	if ttl != want {
		t.Fatalf("TTL=%v, want %v", ttl, want)
	}
}

func TestClassify_RateLimitRule_FiresOnBare429(t *testing.T) {
	plugin := NewPlugin(nil)
	ctx := withTrail(t, schemas.DefaultCooldownPolicy(schemas.Sensenova))
	// Bare 429 + rate_limit_error — no quota substring.
	err := newErrOn(429, "rate_limit_error", "", "HTTP error 429: ", string(schemas.Sensenova))

	ttl, _, _, _, ok := plugin.classify(ctx, err)
	if !ok {
		t.Fatal("expected rate_limit policy to fire on bare 429 + rate_limit_error type")
	}
	want := time.Duration(schemas.SensenovaRateLimitTTLSeconds) * time.Second
	if ttl != want {
		t.Fatalf("TTL=%v, want %v", ttl, want)
	}
}

func TestClassify_QuotaCheckedBeforeRateLimit(t *testing.T) {
	plugin := NewPlugin(nil)
	policy := &schemas.CooldownPolicy{
		Quota: &schemas.CooldownPolicyRule{
			MatchMode: "any",
			TTLSeconds: 1000,
			Match: []schemas.CooldownPolicyMatch{
				{StatusCode: intPtr(429)},
			},
		},
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode: "any",
			TTLSeconds: 10,
			Match:     []schemas.CooldownPolicyMatch{{StatusCode: intPtr(429)}},
		},
	}
	ctx := withTrail(t, policy)
	err := newErr(429, "", "", "")

	ttl, _, _, _, ok := plugin.classify(ctx, err)
	if !ok {
		t.Fatal("expected classify to fire")
	}
	if ttl != 1000*time.Second {
		t.Fatalf("expected quota TTL (1000s) to win over rate_limit TTL (10s), got %v", ttl)
	}
}

func TestClassify_AllModeRequiresEveryMatch(t *testing.T) {
	plugin := NewPlugin(nil)
	policy := &schemas.CooldownPolicy{
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode: "all",
			TTLSeconds: 30,
			Match: []schemas.CooldownPolicyMatch{
				{StatusCode: intPtr(429)},
				{Code: []string{"insufficient_quota"}},
			},
		},
	}
	ctx := withTrail(t, policy)

	// status_code 429 hits but code missing → must NOT fire under "all".
	err := newErr(429, "rate_limit_error", "", "anything")
	if _, _, _, _, ok := plugin.classify(ctx, err); ok {
		t.Fatal("\"all\" mode should not fire when only one of two matches succeeds")
	}

	// both predicates satisfied → fires.
	err = newErr(429, "", "insufficient_quota", "")
	if _, _, _, _, ok := plugin.classify(ctx, err); !ok {
		t.Fatal("\"all\" mode should fire when every match succeeds")
	}
}

func TestClassify_MessageContainsIsCaseInsensitive(t *testing.T) {
	plugin := NewPlugin(nil)
	policy := &schemas.CooldownPolicy{
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode: "any",
			TTLSeconds: 30,
			Match: []schemas.CooldownPolicyMatch{
				{MessageContains: []string{"WORKSPACE ALLOCATED QUOTA"}},
			},
		},
	}
	ctx := withTrail(t, policy)
	err := newErr(429, "", "", "workspace allocated quota exceeded")

	_, _, _, _, ok := plugin.classify(ctx, err)
	if !ok {
		t.Fatal("expected classify to fire on lower-case message containing upper-case pattern")
	}
}

func TestClassify_FallsBackToDefaultPolicyWhenStampMissing(t *testing.T) {
	plugin := NewPlugin(nil)
	// ctx without BifrostContextKeyCooldownPolicy stamp — must fall back to
	// DefaultCooldownPolicy(provider) using the attempt's provider name.
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	// Provider name comes from lastAttemptProviderAndKey → not the ctx. We
	// attach an AttemptTrail with no key to verify "no key → no mark"
	// still works (it shouldn't even fall through to classify).
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{KeyID: "k1", KeyName: "key-1"},
	})

	err := newErr(429, "rate_limit_error", "", "")
	ttl, _, _, _, ok := plugin.classify(ctx, err)
	if !ok {
		t.Fatal("expected classify to fire using default policy fallback")
	}
	// Default TTL for Sensenova rate limit is schemas.SensenovaRateLimitTTLSeconds.
	// Provider is "" here (no provider on AttemptTrail) → DefaultCooldownPolicy("")
	// returns the generic default branch.
	if ttl != time.Duration(schemas.DefaultCooldownTTLSeconds)*time.Second {
		t.Fatalf("expected generic default TTL %v, got %v",
			time.Duration(schemas.DefaultCooldownTTLSeconds)*time.Second, ttl)
	}
}

func TestClassify_NoMatchReturnsFalse(t *testing.T) {
	plugin := NewPlugin(nil)
	policy := &schemas.CooldownPolicy{
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode: "any",
			TTLSeconds: 30,
			Match: []schemas.CooldownPolicyMatch{
				{MessageContains: []string{"never matches"}},
			},
		},
	}
	ctx := makeCtx(t, policy)
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{KeyID: "k1", KeyName: "key-1"},
	})
	err := newErr(500, "", "", "some server error")

	if _, _, _, _, ok := plugin.classify(ctx, err); ok {
		t.Fatal("non-4xx error must not match a 4xx-only policy")
	}
}

func TestDefaultCooldownPolicy_SensenovaHasBothRules(t *testing.T) {
	p := schemas.DefaultCooldownPolicy(schemas.Sensenova)
	if p == nil {
		t.Fatal("expected non-nil policy for Sensenova")
	}
	if p.RateLimit == nil || p.RateLimit.TTLSeconds != schemas.SensenovaRateLimitTTLSeconds {
		t.Fatalf("expected RateLimit TTL=%d, got %+v", schemas.SensenovaRateLimitTTLSeconds, p.RateLimit)
	}
	if p.Quota == nil || p.Quota.TTLSeconds != schemas.SensenovaQuotaTTLSeconds {
		t.Fatalf("expected Quota TTL=%d, got %+v", schemas.SensenovaQuotaTTLSeconds, p.Quota)
	}
}

func TestDefaultCooldownPolicy_AnthropicHasNoQuota(t *testing.T) {
	p := schemas.DefaultCooldownPolicy(schemas.Anthropic)
	if p == nil {
		t.Fatal("expected non-nil policy for Anthropic")
	}
	if p.RateLimit == nil {
		t.Fatal("Anthropic must have a rate_limit rule")
	}
	if p.Quota != nil {
		t.Fatal("Anthropic returns 402 for billing, not 429 — quota rule should be absent")
	}
}

func TestDefaultCooldownPolicy_UnknownProviderReturnsGeneric(t *testing.T) {
	p := schemas.DefaultCooldownPolicy(schemas.ModelProvider("not-a-real-provider"))
	if p == nil || p.RateLimit == nil || p.Quota == nil {
		t.Fatal("unknown provider must still get a generic rate_limit + quota rule")
	}
}

func TestPostLLMHook_MarksKeyWithPolicyTTL(t *testing.T) {
	plugin := NewPlugin(nil)
	ctx := makeCtx(t, schemas.DefaultCooldownPolicy(schemas.Sensenova))
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{KeyID: "k1", KeyName: "key-1"},
	})

	err := newErrOn(429, "rate_limit_error", "", "HTTP error 429: ", string(schemas.Sensenova))
	_, _, _ = plugin.PostLLMHook(ctx, nil, err)

	if !plugin.State.IsCoolingDown(schemas.Sensenova, "k1") {
		t.Fatal("expected sensenova/k1 to be in cooldown after rate_limit policy fired")
	}
}

func TestPostLLMHook_NoMatchDoesNotMark(t *testing.T) {
	plugin := NewPlugin(nil)
	ctx := makeCtx(t, schemas.DefaultCooldownPolicy(schemas.Sensenova))
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{
		{KeyID: "k1", KeyName: "key-1"},
	})

	err := newErr(500, "", "", "server error")
	_, _, _ = plugin.PostLLMHook(ctx, nil, err)

	if plugin.State.IsCoolingDown(schemas.Sensenova, "k1") {
		t.Fatal("non-4xx error must not trigger cooldown")
	}
}

func TestPostLLMHook_NoKeyIDDoesNotMark(t *testing.T) {
	plugin := NewPlugin(nil)
	ctx := makeCtx(t, schemas.DefaultCooldownPolicy(schemas.Sensenova))
	// No AttemptTrail set → lastAttemptProviderAndKey returns "" key.

	err := newErr(429, "rate_limit_error", "", "rate limit")
	_, _, _ = plugin.PostLLMHook(ctx, nil, err)

	if size := plugin.State.Size(); size != 0 {
		t.Fatalf("expected zero cooldown entries when keyID is unknown, got %d", size)
	}
}

func TestMarkWithTTL_OverridesEffectiveTTL(t *testing.T) {
	s := NewCooldownState(300 * time.Second)
	s.MarkWithTTL(schemas.OpenAI, "k", 60*time.Second, "")
	// Verify the key expires within ~60s, not 300s.
	rec := s.cooldowns[s.key(schemas.OpenAI, "k")]
	if rec.expiresAt.IsZero() {
		t.Fatal("expected cooldown entry to be recorded")
	}
	delta := time.Until(rec.expiresAt)
	if delta > 61*time.Second || delta < 59*time.Second {
		t.Fatalf("expected cooldown ~60s, got %v", delta)
	}
}

func TestMarkWithTTL_FallsBackToEffectiveTTLWhenZero(t *testing.T) {
	s := NewCooldownState(300 * time.Second)
	s.MarkWithTTL(schemas.OpenAI, "k", 0, "")
	rec := s.cooldowns[s.key(schemas.OpenAI, "k")]
	delta := time.Until(rec.expiresAt)
	if delta < 299*time.Second || delta > 301*time.Second {
		t.Fatalf("expected fallback TTL ~300s, got %v", delta)
	}
}

func TestMarkWithTTL_EmptyKeyIsNoOp(t *testing.T) {
	s := NewCooldownState(300 * time.Second)
	s.MarkWithTTL(schemas.OpenAI, "", 60*time.Second, "")
	if s.Size() != 0 {
		t.Fatalf("expected no entries when key is empty, got %d", s.Size())
	}
}