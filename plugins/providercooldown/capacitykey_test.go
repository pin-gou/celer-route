package providercooldown

import (
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Per-key capacity gate (scope=key) tests.
//
// The provider / provider_model scopes share one budget across the whole
// provider, so a single exhausted credential can block every other one. The
// "key" scope keeps an independent window per (provider, key, model) triple:
//
//   - record() accounts each terminal attempt against the credential that
//     served it;
//   - the key pool filter (AsFilter) removes only the keys whose own window is
//     over its threshold, so the request rotates to a key that still has
//     budget;
//   - PreProviderHook short-circuits only when NO key in the pool still has
//     budget, honouring capacity.action as before.
//
// Usage that cannot be attributed to a credential falls back to the
// provider_model bucket, and the same derivation is used by record and check.

// keyScopePolicy builds a per-provider policy with the given capacity section
// and no other capability, so the test's numbers are the only source of truth.
func keyScopePolicy(cfg *schemas.CooldownCapacityConfig) *schemas.CooldownPolicy {
	return &schemas.CooldownPolicy{Capacity: cfg}
}

// spendOnKey posts one successful terminal request through PostLLMHook with an
// attempt trail naming keyID, so a key-scoped gate accounts the usage against
// that credential. An empty keyID models an attempt that cannot be attributed.
func spendOnKey(t *testing.T, p *CooldownPlugin, policy *schemas.CooldownPolicy, provider schemas.ModelProvider, model, keyID string, tokens int) {
	t.Helper()
	ctx := withPolicy(newPerKeyTrailCtx(keyID), policy)
	if _, _, err := p.PostLLMHook(ctx, capacityResp(provider, model, tokens), nil); err != nil {
		t.Fatalf("PostLLMHook returned an error: %v", err)
	}
}

// filterKeys runs the plugin state's key pool filter for (provider, model)
// under the given policy and returns the surviving key ids.
func filterKeys(t *testing.T, p *CooldownPlugin, policy *schemas.CooldownPolicy, provider schemas.ModelProvider, model string, keys ...schemas.Key) []string {
	t.Helper()
	ctx := policyCtx(policy)
	out, err := p.State.AsFilter(nil)(ctx, provider, model, keys)
	if err != nil {
		t.Fatalf("AsFilter: %v", err)
	}
	ids := make([]string, 0, len(out))
	for _, k := range out {
		ids = append(ids, k.ID)
	}
	return ids
}

// checkCapacityWithKeys runs PreProviderHook with an explicit key pool.
func checkCapacityWithKeys(p *CooldownPlugin, policy *schemas.CooldownPolicy, provider schemas.ModelProvider, model string, keys ...schemas.Key) *schemas.LLMPluginShortCircuit {
	ctx := withPolicy(capacityCtx(provider, keys...), policy)
	_, sc, _ := p.PreProviderHook(ctx, capacityReq(provider, model))
	return sc
}

// TestCapacityScopeKeyIsolatesPerKey is the acceptance test: one credential
// over its own budget is rotated past while every other credential keeps
// serving traffic, and only a pool with NO budget left is short-circuited.
func TestCapacityScopeKeyIsolatesPerKey(t *testing.T) {
	clk := newFakeClock()
	s := NewCooldownState(5 * time.Minute)
	s.now = clk.Now
	p := &CooldownPlugin{State: s}
	policy := keyScopePolicy(&schemas.CooldownCapacityConfig{
		Enabled:       true,
		TokenLimit:    1000, // threshold = 900 with the default headroom
		WindowSeconds: 60,
		Scope:         schemas.CapacityScopeKey,
	})

	// key-1 spends past its threshold; key-2 spends a little.
	spendOnKey(t, p, policy, schemas.OpenAI, "gpt-4o", "key-1", 950)
	spendOnKey(t, p, policy, schemas.OpenAI, "gpt-4o", "key-2", 10)

	// The filter drops only the exhausted credential.
	if got := filterKeys(t, p, policy, schemas.OpenAI, "gpt-4o",
		schemas.Key{ID: "key-1"}, schemas.Key{ID: "key-2"}); len(got) != 1 || got[0] != "key-2" {
		t.Fatalf("filtered keys = %v, want only key-2 (key-1 is over its own budget)", got)
	}

	// A pool that still contains a key with budget is allowed.
	if sc := checkCapacityWithKeys(p, policy, schemas.OpenAI, "gpt-4o",
		schemas.Key{ID: "key-1"}, schemas.Key{ID: "key-2"}); sc != nil {
		t.Fatalf("pool with a key that still has budget was rejected: %q", sc.Error.Error.Message)
	}

	// A pool made of the exhausted key alone is refused with the gate's own
	// 429 — not with a generic failure.
	sc := checkCapacityWithKeys(p, policy, schemas.OpenAI, "gpt-4o", schemas.Key{ID: "key-1"})
	if sc == nil {
		t.Fatal("a pool with no remaining budget must be short-circuited")
	}
	if sc.Error.StatusCode == nil || *sc.Error.StatusCode != 429 {
		t.Fatalf("status = %v, want 429", sc.Error.StatusCode)
	}
	if sc.Error.Type == nil || *sc.Error.Type != capacityGateErrorType {
		t.Fatalf("type = %v, want %q", sc.Error.Type, capacityGateErrorType)
	}
	if !isCapacityGateError(sc.Error) {
		t.Fatalf("error %q is not recognised as a capacity-gate error", sc.Error.Error.Message)
	}
}

// TestCapacityScopeKeyRejectKeepsActionSemantics pins that action=reject still
// means "do not enter the fallback chain" under the per-key scope.
func TestCapacityScopeKeyRejectKeepsActionSemantics(t *testing.T) {
	clk := newFakeClock()
	for _, tt := range []struct {
		action       string
		wantFallback bool
	}{
		{schemas.CapacityActionFallback, true},
		{schemas.CapacityActionReject, false},
	} {
		t.Run(tt.action, func(t *testing.T) {
			s := NewCooldownState(5 * time.Minute)
			s.now = clk.Now
			p := &CooldownPlugin{State: s}
			policy := keyScopePolicy(&schemas.CooldownCapacityConfig{
				Enabled:       true,
				TokenLimit:    100,
				WindowSeconds: 60,
				Scope:         schemas.CapacityScopeKey,
				Action:        tt.action,
			})
			spendOnKey(t, p, policy, schemas.OpenAI, "gpt-4o", "only", 95)

			sc := checkCapacityWithKeys(p, policy, schemas.OpenAI, "gpt-4o", schemas.Key{ID: "only"})
			if sc == nil {
				t.Fatal("expected a short-circuit")
			}
			if sc.Error.AllowFallbacks == nil {
				t.Fatal("AllowFallbacks must be set explicitly")
			}
			if got := *sc.Error.AllowFallbacks; got != tt.wantFallback {
				t.Fatalf("AllowFallbacks = %t, want %t for action=%s", got, tt.wantFallback, tt.action)
			}
		})
	}
}

// TestCapacityScopeKeyMissingKeyIDFallsBack pins the safe fallback: usage with
// no attributable credential is accounted in the provider_model bucket, and
// check() consults that very bucket when it has no key either — so record and
// check always agree on where a given attempt lands.
func TestCapacityScopeKeyMissingKeyIDFallsBack(t *testing.T) {
	clk := newFakeClock()
	s := NewCooldownState(5 * time.Minute)
	s.now = clk.Now
	p := &CooldownPlugin{State: s}
	policy := keyScopePolicy(&schemas.CooldownCapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		WindowSeconds: 60,
		Scope:         schemas.CapacityScopeKey,
	})

	// No attempt trail → no credential to attribute the usage to.
	spend(t, p, policy, schemas.OpenAI, "gpt-4o", 950)

	gate := s.gateFor(schemas.OpenAI, policy)
	if gate == nil {
		t.Fatal("the gate must be installed")
	}
	// Recorded in the fallback bucket ...
	if tokens, _ := gate.usage(schemas.OpenAI, "", "gpt-4o"); tokens != 950 {
		t.Fatalf("fallback bucket tokens = %d, want 950", tokens)
	}
	// ... which is exactly the bucket an unattributed check reads.
	if v := gate.check(schemas.OpenAI, "", "gpt-4o"); v == nil {
		t.Fatal("check with no key must read the fallback bucket and find it over budget")
	}
	// A real credential's own bucket is untouched: the fallback does not
	// contaminate per-key accounting.
	if v := gate.check(schemas.OpenAI, "key-1", "gpt-4o"); v != nil {
		t.Fatalf("key-1 must not inherit the unattributed usage, got %#v", v)
	}

	// A keyless provider's only (sentinel) key has no id, so its pool check
	// reads the fallback bucket and refuses the request.
	if sc := checkCapacityWithKeys(p, policy, schemas.OpenAI, "gpt-4o", schemas.Key{}); sc == nil {
		t.Fatal("a keyless pool must be gated through the fallback bucket")
	}
}

// TestCapacityScopeKeyUsageSnapshotCarriesKeyID pins the observability side:
// GET /capacity-usage reuses the existing endpoint and reports the credential
// each key-scoped row belongs to.
func TestCapacityScopeKeyUsageSnapshotCarriesKeyID(t *testing.T) {
	clk := newFakeClock()
	s := NewCooldownState(5 * time.Minute)
	s.now = clk.Now
	p := &CooldownPlugin{State: s}
	policy := keyScopePolicy(&schemas.CooldownCapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		WindowSeconds: 60,
		Scope:         schemas.CapacityScopeKey,
	})

	spendOnKey(t, p, policy, schemas.OpenAI, "gpt-4o", "key-1", 100)
	spendOnKey(t, p, policy, schemas.OpenAI, "gpt-4o", "key-2", 200)

	entries := p.CapacityUsage()
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want one per credential", entries)
	}
	byKey := map[string]ProviderCapacityUsage{}
	for _, e := range entries {
		if e.Scope != schemas.CapacityScopeKey {
			t.Fatalf("entry scope = %q, want %q", e.Scope, schemas.CapacityScopeKey)
		}
		if e.Model != "gpt-4o" {
			t.Fatalf("entry model = %q, want gpt-4o", e.Model)
		}
		byKey[e.KeyID] = e
	}
	if got := byKey["key-1"].UsedTokens; got != 100 {
		t.Fatalf("key-1 used tokens = %d, want 100", got)
	}
	if got := byKey["key-2"].UsedTokens; got != 200 {
		t.Fatalf("key-2 used tokens = %d, want 200", got)
	}
}

// TestCapacityScopeKeyDedupeKeepsEveryKey pins the snapshot identity: two
// credentials serving the same model must not collapse into one row.
func TestCapacityScopeKeyDedupeKeepsEveryKey(t *testing.T) {
	in := []ProviderCapacityUsage{
		{Provider: schemas.OpenAI, KeyID: "a", Model: "gpt-4o", Scope: schemas.CapacityScopeKey, Source: capacityUsageSourceProvider},
		{Provider: schemas.OpenAI, KeyID: "b", Model: "gpt-4o", Scope: schemas.CapacityScopeKey, Source: capacityUsageSourceProvider},
		{Provider: schemas.OpenAI, KeyID: "a", Model: "gpt-4o", Scope: schemas.CapacityScopeKey, Source: capacityUsageSourceDefault},
	}
	out := dedupeCapacityUsage(in)
	if len(out) != 2 {
		t.Fatalf("deduped entries = %#v, want one per key", out)
	}
	if out[0].KeyID != "a" || out[0].Source != capacityUsageSourceProvider {
		t.Fatalf("first entry = %#v, want key a from the provider gate", out[0])
	}
	if out[1].KeyID != "b" {
		t.Fatalf("second entry = %#v, want key b", out[1])
	}
}

// TestCapacityScopeProviderAndModelUnchanged is the regression guard: with the
// pre-existing scopes the gate keeps merging usage across credentials and the
// key pool filter must NOT drop anything.
func TestCapacityScopeProviderAndModelUnchanged(t *testing.T) {
	for _, scope := range []string{schemas.CapacityScopeProvider, schemas.CapacityScopeProviderModel} {
		t.Run(scope, func(t *testing.T) {
			clk := newFakeClock()
			s := NewCooldownState(5 * time.Minute)
			s.now = clk.Now
			p := &CooldownPlugin{State: s}
			policy := keyScopePolicy(&schemas.CooldownCapacityConfig{
				Enabled:       true,
				TokenLimit:    1000,
				WindowSeconds: 60,
				Scope:         scope,
			})

			// Usage from two credentials lands in ONE bucket.
			spendOnKey(t, p, policy, schemas.OpenAI, "gpt-4o", "key-1", 950)
			spendOnKey(t, p, policy, schemas.OpenAI, "gpt-4o", "key-2", 10)

			// The filter never applies a capacity exclusion outside the key
			// scope: both keys stay selectable.
			if got := filterKeys(t, p, policy, schemas.OpenAI, "gpt-4o",
				schemas.Key{ID: "key-1"}, schemas.Key{ID: "key-2"}); len(got) != 2 {
				t.Fatalf("filtered keys = %v, want both keys (scope=%s must not filter per key)", got, scope)
			}

			// The shared budget is over its threshold, so the request is
			// refused before key selection — the historical behaviour.
			if sc := checkCapacityWithKeys(p, policy, schemas.OpenAI, "gpt-4o",
				schemas.Key{ID: "key-1"}, schemas.Key{ID: "key-2"}); sc == nil {
				t.Fatalf("scope=%s must refuse once the shared window is over budget", scope)
			}
		})
	}
}

// TestParseConfigCapacityScopeKey pins the config surface: "key" is accepted,
// an unknown value is still an error, and normalization falls back to
// "provider" for anything unrecognised.
func TestParseConfigCapacityScopeKey(t *testing.T) {
	c, err := ParseConfig(map[string]any{
		"capacity": map[string]any{
			"enabled":        true,
			"token_limit":    float64(1000),
			"scope":          "key",
			"window_seconds": float64(60),
		},
	})
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if c.Capacity == nil || c.Capacity.Scope != CapacityScopeKey {
		t.Fatalf("Capacity = %+v, want scope %q", c.Capacity, CapacityScopeKey)
	}

	if _, err := ParseConfig(map[string]any{
		"capacity": map[string]any{"enabled": true, "scope": "tenant"},
	}); err == nil {
		t.Fatal("an unknown scope must still be rejected at parse time")
	}

	if got := schemas.NormalizeCapacityScope("tenant"); got != schemas.CapacityScopeProvider {
		t.Fatalf("NormalizeCapacityScope(tenant) = %q, want the provider fallback", got)
	}
}
