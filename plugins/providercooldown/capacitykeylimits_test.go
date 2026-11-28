package providercooldown

import (
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

func int64Ptr(v int64) *int64 { return &v }

// keyLimitGate builds a key-scoped gate with the given provider-level limits and
// per-credential overrides.
func keyLimitGate(t *testing.T, tokenLimit int64, overrides map[string]schemas.CooldownCapacityKeyLimit) *capacityGate {
	t.Helper()
	cfg := &schemas.CooldownCapacityConfig{
		Enabled:    true,
		TokenLimit: tokenLimit,
		Scope:      schemas.CapacityScopeKey,
		KeyLimits:  overrides,
	}
	spec, ok := capacitySpecOf(cfg)
	if !ok {
		t.Fatal("capacitySpecOf reported the gate as inert")
	}
	return newCapacityGateFromSpec(spec, cfg.KeyLimits)
}

// TestCapacityKeyLimitsOverridePerCredential pins P3: with scope=key, a single
// provider-level number applies to every credential, and key_limits lets one
// credential deviate without giving it a fake provider-wide default.
func TestCapacityKeyLimitsOverridePerCredential(t *testing.T) {
	g := keyLimitGate(t, 1000, map[string]schemas.CooldownCapacityKeyLimit{
		"small": {TokenLimit: int64Ptr(100)},
	})

	g.record(schemas.OpenAI, "id-small", "gpt-4o", 100)
	g.record(schemas.OpenAI, "id-big", "gpt-4o", 100)

	// small: 100 tokens against 100*0.9 = 90 → over budget.
	if v := g.checkKey(schemas.OpenAI, "id-small", "small", "gpt-4o"); v == nil {
		t.Fatal("the overridden credential must be gated at its own threshold")
	} else if v.tokenLimit != 100 {
		t.Fatalf("verdict token_limit = %d, want the override 100", v.tokenLimit)
	}
	// big: same 100 tokens against 1000*0.9 = 900 → fine.
	if v := g.checkKey(schemas.OpenAI, "id-big", "big", "gpt-4o"); v != nil {
		t.Fatalf("the un-overridden credential must keep the provider-level threshold, got %+v", v)
	}
	// Without a name there is nothing to look the override up by, so the check
	// falls back to the provider-level budget.
	if v := g.check(schemas.OpenAI, "id-small", "gpt-4o"); v != nil {
		t.Fatalf("check() without a key name must use the provider-level budget, got %+v", v)
	}
}

// TestCapacityKeyLimitsExplicitZeroMeansUnlimited pins the pointer semantics:
// an explicit 0 disables the budget for that credential, while a nil pointer
// inherits the provider-level value.
func TestCapacityKeyLimitsExplicitZeroMeansUnlimited(t *testing.T) {
	g := keyLimitGate(t, 100, map[string]schemas.CooldownCapacityKeyLimit{
		"unlimited": {TokenLimit: int64Ptr(0)},
		"inherit":   {},
	})

	g.record(schemas.OpenAI, "id-a", "gpt-4o", 1000)
	g.record(schemas.OpenAI, "id-b", "gpt-4o", 1000)

	if v := g.checkKey(schemas.OpenAI, "id-a", "unlimited", "gpt-4o"); v != nil {
		t.Fatalf("an explicit 0 must mean unlimited, got %+v", v)
	}
	if v := g.checkKey(schemas.OpenAI, "id-b", "inherit", "gpt-4o"); v == nil {
		t.Fatal("an entry without token_limit must inherit the provider-level budget")
	}
}

// TestCapacityKeyLimitsOnlyConfigActivatesGate pins that per-key budgets may be
// the only budget configured: no provider-wide scalar is required.
func TestCapacityKeyLimitsOnlyConfigActivatesGate(t *testing.T) {
	cfg := &schemas.CooldownCapacityConfig{
		Enabled: true,
		Scope:   schemas.CapacityScopeKey,
		KeyLimits: map[string]schemas.CooldownCapacityKeyLimit{
			"k1": {RequestLimit: int64Ptr(5)},
		},
	}
	if !cfg.Active() {
		t.Fatal("a key_limits-only section must activate the gate")
	}
	spec, ok := capacitySpecOf(cfg)
	if !ok {
		t.Fatal("capacitySpecOf must accept a key_limits-only section")
	}
	g := newCapacityGateFromSpec(spec, cfg.KeyLimits)

	for i := 0; i < 5; i++ {
		g.record(schemas.OpenAI, "id-k1", "gpt-4o", 0)
	}
	// 5 requests against 5*0.9 = ceil(4.5) = 5 → engaged.
	if v := g.checkKey(schemas.OpenAI, "id-k1", "k1", "gpt-4o"); v == nil {
		t.Fatal("the per-key request budget must engage")
	}
	// Another credential has no budget at all (no provider-level default).
	if v := g.checkKey(schemas.OpenAI, "id-k2", "k2", "gpt-4o"); v != nil {
		t.Fatalf("an unconfigured credential has no budget and must not be gated, got %+v", v)
	}
}

// TestCapacityKeyLimitsSnapshotReportsEffectiveThresholds pins the observability
// side: the usage row must show the credential's name and the thresholds the
// gate actually compared against, not the provider-level scalars.
func TestCapacityKeyLimitsSnapshotReportsEffectiveThresholds(t *testing.T) {
	g := keyLimitGate(t, 1000, map[string]schemas.CooldownCapacityKeyLimit{
		"small": {TokenLimit: int64Ptr(100)},
	})

	g.record(schemas.OpenAI, "id-small", "gpt-4o", 95)
	// A key-pool check is what teaches the gate the ID→name mapping.
	g.checkKey(schemas.OpenAI, "id-small", "small", "gpt-4o")

	entries := g.capacityUsage(capacityUsageSourceProvider)
	if len(entries) != 1 {
		t.Fatalf("expected one usage row, got %d: %+v", len(entries), entries)
	}
	row := entries[0]
	if row.KeyName != "small" {
		t.Fatalf("key_name = %q, want %q", row.KeyName, "small")
	}
	if row.TokenLimit != 100 {
		t.Fatalf("token_limit = %d, want the override 100", row.TokenLimit)
	}
	if row.ThresholdTokens != 90 {
		t.Fatalf("threshold_tokens = %d, want 100*0.9 = 90", row.ThresholdTokens)
	}
	if row.RetryAfterSeconds == 0 {
		t.Fatal("95 tokens is over the 90 threshold, so retry_after_seconds must be set")
	}
}

// TestCapacityKeyLimitsRebuildDetection pins that a key_limits change is
// detected (the gate is rebuilt) while an identical map is not.
func TestCapacityKeyLimitsRebuildDetection(t *testing.T) {
	cfg := &schemas.CooldownCapacityConfig{
		Enabled:    true,
		TokenLimit: 1000,
		Scope:      schemas.CapacityScopeKey,
		KeyLimits: map[string]schemas.CooldownCapacityKeyLimit{
			"small": {TokenLimit: int64Ptr(100)},
		},
	}
	spec, ok := capacitySpecOf(cfg)
	if !ok {
		t.Fatal("capacitySpecOf reported the gate as inert")
	}
	g := newCapacityGateFromSpec(spec, cfg.KeyLimits)

	same := map[string]schemas.CooldownCapacityKeyLimit{"small": {TokenLimit: int64Ptr(100)}}
	if !g.matchesCapacitySpec(spec, same) {
		t.Fatal("an identical key_limits map must not force a rebuild")
	}

	changed := map[string]schemas.CooldownCapacityKeyLimit{"small": {TokenLimit: int64Ptr(200)}}
	if g.matchesCapacitySpec(spec, changed) {
		t.Fatal("a changed key_limits map must force a rebuild")
	}

	added := map[string]schemas.CooldownCapacityKeyLimit{
		"small": {TokenLimit: int64Ptr(100)},
		"big":   {TokenLimit: int64Ptr(900)},
	}
	if g.matchesCapacitySpec(spec, added) {
		t.Fatal("an added key_limits entry must force a rebuild")
	}

	if g.matchesCapacitySpec(spec, nil) {
		t.Fatal("nil key_limits must not match a gate that has overrides")
	}
}

// TestCapacityKeyLimitsIgnoredOutsideKeyScope pins that the overrides are inert
// under the provider / provider_model scopes, matching Validate's refusal to
// store them there.
func TestCapacityKeyLimitsIgnoredOutsideKeyScope(t *testing.T) {
	cfg := &schemas.CooldownCapacityConfig{
		Enabled:    true,
		TokenLimit: 1000,
		Scope:      schemas.CapacityScopeProvider,
		KeyLimits: map[string]schemas.CooldownCapacityKeyLimit{
			"small": {TokenLimit: int64Ptr(1)},
		},
	}
	spec, ok := capacitySpecOf(cfg)
	if !ok {
		t.Fatal("capacitySpecOf reported the gate as inert")
	}
	g := newCapacityGateFromSpec(spec, cfg.KeyLimits)
	if len(g.keyLimits) != 0 {
		t.Fatalf("overrides must not be built outside the key scope, got %+v", g.keyLimits)
	}
}

// TestCapacityKeyModelLimitsOverridePerModel pins the third resolution layer:
// a credential may carry a different budget per model, and the narrowest layer
// wins over the credential-level value.
func TestCapacityKeyModelLimitsOverridePerModel(t *testing.T) {
	g := keyLimitGate(t, 1000, map[string]schemas.CooldownCapacityKeyLimit{
		"k": {
			TokenLimit: int64Ptr(1000),
			Models: map[string]schemas.CooldownCapacityModelLimit{
				"gpt-4o": {TokenLimit: int64Ptr(100)},
			},
		},
	})

	g.record(schemas.OpenAI, "id-k", "gpt-4o", 100)
	g.record(schemas.OpenAI, "id-k", "gpt-4o-mini", 100)

	// gpt-4o: 100 tokens against 100*0.9 = 90 → over budget.
	v := g.checkKey(schemas.OpenAI, "id-k", "k", "gpt-4o")
	if v == nil {
		t.Fatal("the per-model override must gate gpt-4o at its own threshold")
	}
	if v.tokenLimit != 100 {
		t.Fatalf("verdict token_limit = %d, want the model override 100", v.tokenLimit)
	}
	// gpt-4o-mini has no model override, so the credential's 1000 applies.
	if v := g.checkKey(schemas.OpenAI, "id-k", "k", "gpt-4o-mini"); v != nil {
		t.Fatalf("a model without an override must inherit the key budget, got %+v", v)
	}
}

// TestCapacityKeyModelLimitsInheritPerField pins that a model entry only names
// what differs from its credential: an omitted dimension still inherits.
func TestCapacityKeyModelLimitsInheritPerField(t *testing.T) {
	g := keyLimitGate(t, 1000, map[string]schemas.CooldownCapacityKeyLimit{
		"k": {
			TokenLimit:   int64Ptr(1000),
			RequestLimit: int64Ptr(10),
			Models: map[string]schemas.CooldownCapacityModelLimit{
				"gpt-4o": {RequestLimit: int64Ptr(2)},
			},
		},
	})

	for i := 0; i < 2; i++ {
		g.record(schemas.OpenAI, "id-k", "gpt-4o", 100)
	}
	v := g.checkKey(schemas.OpenAI, "id-k", "k", "gpt-4o")
	if v == nil {
		t.Fatal("the model's request override (2*0.9 → 2) must engage at 2 requests")
	}
	if v.tokenLimit != 1000 {
		t.Fatalf("token_limit = %d, want the inherited key value 1000", v.tokenLimit)
	}
	if v.requestLimit != 2 {
		t.Fatalf("request_limit = %d, want the model override 2", v.requestLimit)
	}
}

// TestCapacityKeyModelLimitsSnapshotReportsSource pins the observability side:
// the row must say which layer produced its thresholds.
func TestCapacityKeyModelLimitsSnapshotReportsSource(t *testing.T) {
	g := keyLimitGate(t, 1000, map[string]schemas.CooldownCapacityKeyLimit{
		"k": {
			TokenLimit: int64Ptr(1000),
			Models: map[string]schemas.CooldownCapacityModelLimit{
				"gpt-4o": {TokenLimit: int64Ptr(100)},
			},
		},
	})

	g.record(schemas.OpenAI, "id-k", "gpt-4o", 95)
	g.record(schemas.OpenAI, "id-k", "gpt-4o-mini", 95)
	g.checkKey(schemas.OpenAI, "id-k", "k", "gpt-4o")

	byModel := map[string]ProviderCapacityUsage{}
	for _, row := range g.capacityUsage(capacityUsageSourceProvider) {
		byModel[row.Model] = row
	}
	if got := byModel["gpt-4o"].LimitSource; got != capacityLimitSourceKeyModel {
		t.Fatalf("gpt-4o limit_source = %q, want %q", got, capacityLimitSourceKeyModel)
	}
	if got := byModel["gpt-4o"].TokenLimit; got != 100 {
		t.Fatalf("gpt-4o token_limit = %d, want the model override 100", got)
	}
	if got := byModel["gpt-4o-mini"].LimitSource; got != capacityLimitSourceKey {
		t.Fatalf("gpt-4o-mini limit_source = %q, want %q", got, capacityLimitSourceKey)
	}
}

// TestCapacityKeyModelLimitsOnlyConfigActivatesGate pins that a nested override
// alone may activate the gate: no provider-wide scalar is required.
func TestCapacityKeyModelLimitsOnlyConfigActivatesGate(t *testing.T) {
	cfg := &schemas.CooldownCapacityConfig{
		Enabled: true,
		Scope:   schemas.CapacityScopeKey,
		KeyLimits: map[string]schemas.CooldownCapacityKeyLimit{
			"k": {Models: map[string]schemas.CooldownCapacityModelLimit{
				"gpt-4o": {RequestLimit: int64Ptr(2)},
			}},
		},
	}
	if !cfg.Active() {
		t.Fatal("a models-only key_limits section must activate the gate")
	}
	spec, ok := capacitySpecOf(cfg)
	if !ok {
		t.Fatal("capacitySpecOf must accept a models-only key_limits section")
	}
	g := newCapacityGateFromSpec(spec, cfg.KeyLimits)

	for i := 0; i < 2; i++ {
		g.record(schemas.OpenAI, "id-k", "gpt-4o", 0)
	}
	if v := g.checkKey(schemas.OpenAI, "id-k", "k", "gpt-4o"); v == nil {
		t.Fatal("the nested request budget must engage")
	}
	// Another model has no budget at all under this key.
	if v := g.checkKey(schemas.OpenAI, "id-k", "k", "gpt-4o-mini"); v != nil {
		t.Fatalf("a model without a nested budget must not be gated, got %+v", v)
	}
}

// TestCapacityKeyModelLimitsRebuildDetection pins that changing a nested
// override forces a gate rebuild, while an identical map does not.
func TestCapacityKeyModelLimitsRebuildDetection(t *testing.T) {
	cfg := &schemas.CooldownCapacityConfig{
		Enabled:    true,
		TokenLimit: 1000,
		Scope:      schemas.CapacityScopeKey,
		KeyLimits: map[string]schemas.CooldownCapacityKeyLimit{
			"k": {Models: map[string]schemas.CooldownCapacityModelLimit{
				"gpt-4o": {TokenLimit: int64Ptr(100)},
			}},
		},
	}
	spec, ok := capacitySpecOf(cfg)
	if !ok {
		t.Fatal("capacitySpecOf reported the gate as inert")
	}
	g := newCapacityGateFromSpec(spec, cfg.KeyLimits)

	same := map[string]schemas.CooldownCapacityKeyLimit{
		"k": {Models: map[string]schemas.CooldownCapacityModelLimit{
			"gpt-4o": {TokenLimit: int64Ptr(100)},
		}},
	}
	if !g.matchesCapacitySpec(spec, same) {
		t.Fatal("an identical nested override must not force a rebuild")
	}

	changed := map[string]schemas.CooldownCapacityKeyLimit{
		"k": {Models: map[string]schemas.CooldownCapacityModelLimit{
			"gpt-4o": {TokenLimit: int64Ptr(200)},
		}},
	}
	if g.matchesCapacitySpec(spec, changed) {
		t.Fatal("a changed nested override must force a rebuild")
	}

	addedModel := map[string]schemas.CooldownCapacityKeyLimit{
		"k": {Models: map[string]schemas.CooldownCapacityModelLimit{
			"gpt-4o":      {TokenLimit: int64Ptr(100)},
			"gpt-4o-mini": {TokenLimit: int64Ptr(50)},
		}},
	}
	if g.matchesCapacitySpec(spec, addedModel) {
		t.Fatal("an added model override must force a rebuild")
	}
}
