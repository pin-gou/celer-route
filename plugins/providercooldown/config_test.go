package providercooldown

import (
	"strings"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestParseConfigNil(t *testing.T) {
	c, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("nil config should parse, got %v", err)
	}
	if c.DefaultTTLSeconds != 0 || c.TTLOverrides != nil {
		t.Fatalf("expected zero-value Config, got %+v", c)
	}
}

func TestParseConfigEmpty(t *testing.T) {
	c, err := ParseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("empty config should parse, got %v", err)
	}
	if c.DefaultTTLSeconds != 0 {
		t.Fatalf("DefaultTTLSeconds = %d, want 0", c.DefaultTTLSeconds)
	}
}

func TestParseConfigDefaultTTL(t *testing.T) {
	c, err := ParseConfig(map[string]any{
		"default_ttl_seconds": float64(120),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.DefaultTTLSeconds != 120 {
		t.Fatalf("DefaultTTLSeconds = %d, want 120", c.DefaultTTLSeconds)
	}
}

func TestParseConfigOverrides(t *testing.T) {
	c, err := ParseConfig(map[string]any{
		"default_ttl_seconds": float64(600),
		"ttl_overrides": map[string]any{
			"openai":    float64(30),
			"anthropic": float64(1200),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := c.TTLOverrides["openai"]; got != 30 {
		t.Fatalf("openai override = %d, want 30", got)
	}
	if got := c.TTLOverrides["anthropic"]; got != 1200 {
		t.Fatalf("anthropic override = %d, want 1200", got)
	}
}

func TestParseConfigRejectsNonIntTTL(t *testing.T) {
	_, err := ParseConfig(map[string]any{
		"default_ttl_seconds": "forever",
	})
	if err == nil {
		t.Fatal("expected error on non-int default_ttl_seconds")
	}
}

func TestParseConfigRejectsNonIntOverride(t *testing.T) {
	_, err := ParseConfig(map[string]any{
		"ttl_overrides": map[string]any{
			"openai": "thirty",
		},
	})
	if err == nil {
		t.Fatal("expected error on non-int override")
	}
}

func TestParseConfigRejectsNonObjectOverrides(t *testing.T) {
	_, err := ParseConfig(map[string]any{
		"ttl_overrides": []any{float64(1), float64(2)},
	})
	if err == nil {
		t.Fatal("expected error on non-object ttl_overrides")
	}
}

func TestConfigAsStateAppliesValues(t *testing.T) {
	c := &Config{
		DefaultTTLSeconds: 120,
		TTLOverrides: map[string]int{
			"openai": 30,
		},
	}
	s := c.AsState(nil)

	if s.ttl != 120*time.Second {
		t.Fatalf("default TTL = %v, want 120s", s.ttl)
	}
	if got := s.effectiveTTLLocked(schemas.OpenAI); got != 30*time.Second {
		t.Fatalf("openai override = %v, want 30s", got)
	}
	if got := s.effectiveTTLLocked(schemas.Anthropic); got != 120*time.Second {
		t.Fatalf("anthropic default = %v, want 120s", got)
	}
}

func TestConfigAsStateFallsBackOnZero(t *testing.T) {
	c := &Config{DefaultTTLSeconds: 0} // zero / negative -> DefaultCooldownTTL
	s := c.AsState(nil)
	if s.ttl != DefaultCooldownTTL {
		t.Fatalf("zero TTL must fall back to DefaultCooldownTTL, got %v", s.ttl)
	}
}

func TestConfigAsStateIgnoresNonPositiveOverrides(t *testing.T) {
	c := &Config{
		DefaultTTLSeconds: 60,
		TTLOverrides: map[string]int{
			"openai":    -5, // ignored
			"anthropic": 0,  // ignored
		},
	}
	s := c.AsState(nil)
	if _, ok := s.ttlOverrides[schemas.OpenAI]; ok {
		t.Fatal("non-positive openai override should be ignored")
	}
	if _, ok := s.ttlOverrides[schemas.Anthropic]; ok {
		t.Fatal("zero anthropic override should be ignored")
	}
}

func TestConfigAsStateWarnsOnUnknownProvider(t *testing.T) {
	log := &testLogger{}
	c := &Config{
		DefaultTTLSeconds: 60,
		TTLOverrides: map[string]int{
			"openai":           30,  // known — no warning expected
			"open-ai":          45,  // typo — warning expected
			"anthropic":        90,  // known — no warning expected
			"made-up-provider": 120, // unknown — warning expected
		},
	}
	s := c.AsState(log)

	// Known providers should still get applied.
	if got := s.effectiveTTLLocked(schemas.OpenAI); got != 30*time.Second {
		t.Fatalf("openai override = %v, want 30s", got)
	}
	if got := s.effectiveTTLLocked(schemas.Anthropic); got != 90*time.Second {
		t.Fatalf("anthropic override = %v, want 90s", got)
	}

	// Two warnings expected (for "open-ai" and "made-up-provider").
	if !log.contains(`"open-ai"`) {
		t.Fatalf("expected warning for typo'd provider name, got messages: %v", log.msgs)
	}
	if !log.contains(`"made-up-provider"`) {
		t.Fatalf("expected warning for unknown provider, got messages: %v", log.msgs)
	}
	// Known providers should NOT appear in warnings.
	for _, m := range log.msgs {
		if strings.Contains(m, `"openai"`) || strings.Contains(m, `"anthropic"`) {
			t.Fatalf("known providers should not appear in warnings, got: %v", log.msgs)
		}
	}
}

func TestConfigAsStateNilLoggerDoesNotPanic(t *testing.T) {
	c := &Config{
		TTLOverrides: map[string]int{
			"some-unknown-provider": 60,
		},
	}
	// nil logger must not panic on unknown provider names.
	s := c.AsState(nil)
	if s == nil {
		t.Fatal("AsState should still return a state with nil logger")
	}
}

func TestParseConfigQuotaPatterns(t *testing.T) {
	c, err := ParseConfig(map[string]any{
		"quota_patterns": []any{"custom_quota", "  WHITESPACE AND CAPS  ", ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Whitespace trimmed, lowercased, empty strings dropped.
	if len(c.QuotaPatterns) != 2 {
		t.Fatalf("expected 2 patterns after trimming/dropping, got %d: %v", len(c.QuotaPatterns), c.QuotaPatterns)
	}
	if c.QuotaPatterns[0] != "custom_quota" {
		t.Fatalf("first pattern = %q, want custom_quota", c.QuotaPatterns[0])
	}
	if c.QuotaPatterns[1] != "whitespace and caps" {
		t.Fatalf("second pattern = %q, want trimmed+lowercased", c.QuotaPatterns[1])
	}
}

func TestParseConfigQuotaPatternsRejectsNonArray(t *testing.T) {
	_, err := ParseConfig(map[string]any{
		"quota_patterns": "not-an-array",
	})
	if err == nil {
		t.Fatal("expected error on non-array quota_patterns")
	}
}

func TestParseConfigQuotaPatternsRejectsNonString(t *testing.T) {
	_, err := ParseConfig(map[string]any{
		"quota_patterns": []any{float64(42)},
	})
	if err == nil {
		t.Fatal("expected error on non-string pattern element")
	}
}

func TestParseConfigQuotaPatternsNilIsOK(t *testing.T) {
	c, err := ParseConfig(map[string]any{
		"default_ttl_seconds": float64(60),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.QuotaPatterns) != 0 {
		t.Fatalf("absent quota_patterns should parse to empty, got %v", c.QuotaPatterns)
	}
}

func TestConfig_DefaultOnInitialization(t *testing.T) {
	// Verify that NewPlugin(logger).Init(nil) succeeds with default
	// configuration and produces a fully usable plugin (covering the
	// default-enable path where no config.json block is present).
	log := &testLogger{}
	plugin := NewPlugin(log)
	defer plugin.Cleanup()

	if err := plugin.Init(nil); err != nil {
		t.Fatalf("Init(nil) should succeed with default config, got %v", err)
	}
	if plugin.State == nil {
		t.Fatal("Init(nil) must populate State")
	}
	// Default TTL must be DefaultCooldownTTL (5 minutes) when no config
	// overrides are provided.
	if got := plugin.State.EffectiveTTL(schemas.OpenAI); got != DefaultCooldownTTL {
		t.Fatalf("default TTL after Init(nil) = %v, want %v", got, DefaultCooldownTTL)
	}
	// The plugin must be fully usable: mark a key and verify cooldown.
	plugin.State.Mark(schemas.OpenAI, "key-1")
	if !plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("key should be in cooldown after Mark")
	}
	// Verify the init log line was emitted.
	if !log.contains("initialized") {
		t.Fatalf("expected init log message, got %v", log.msgs)
	}
	// No plugin-level capacity section → no deprecation warning.
	if log.warnCount() != 0 {
		t.Fatalf("Init(nil) must not warn, got %v", log.msgs)
	}
}

func TestConfig_LegacyCapacityWarnsOnce(t *testing.T) {
	// A config.json that still carries the plugin-level capacity section gets
	// exactly one deprecation warning at init — the section is still honoured,
	// but operators are told to move to per-provider configuration.
	log := &testLogger{}
	plugin := NewPlugin(log)
	defer plugin.Cleanup()

	if err := plugin.Init(map[string]any{
		"capacity": map[string]any{
			"enabled":     true,
			"token_limit": float64(1000),
		},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !log.contains("plugin-level capacity config is a deprecated fleet default") {
		t.Fatalf("expected a deprecation warning for config.capacity, got %v", log.msgs)
	}
	if got := log.warnCount(); got != 1 {
		t.Fatalf("deprecation warning emitted %d times, want exactly 1: %v", got, log.msgs)
	}
}

func TestPluginCustomQuotaPatternMatches(t *testing.T) {
	log := &testLogger{}
	plugin := NewPlugin(log)
	defer plugin.Cleanup()

	// Configure a custom pattern that is NOT in the built-in list.
	if err := plugin.Init(map[string]any{
		"quota_patterns": []any{"myprovider quota exhausted"},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	ctx := newTrailCtx("key-1")
	err := &schemas.BifrostError{
		StatusCode: intPtr(429),
		Error:      &schemas.ErrorField{Message: "MyProvider Quota Exhausted: please upgrade"},
		ExtraFields: schemas.BifrostErrorExtraFields{
			RoutingInfo: schemas.RoutingInfo{Provider: schemas.OpenAI},
		},
	}
	plugin.PostLLMHook(ctx, nil, err)

	if !plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("custom quota pattern should have triggered cooldown")
	}
	if !log.contains("marked key openai/key-1 (name=k-key-1, TTL=") {
		t.Fatalf("expected Mark log, got messages: %v", log.msgs)
	}
}

func TestPluginCustomQuotaPatternDoesNotMaskBuiltin(t *testing.T) {
	plugin := NewPlugin(nil)
	defer plugin.Cleanup()

	// Even with a custom pattern configured, the built-in patterns must
	// still work (custom patterns EXTEND, not replace).
	if err := plugin.Init(map[string]any{
		"quota_patterns": []any{"irrelevant pattern"},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	ctx := newTrailCtx("key-1")
	// Uses the built-in "insufficient_quota" substring.
	err := &schemas.BifrostError{
		StatusCode: intPtr(429),
		Error:      &schemas.ErrorField{Message: "insufficient_quota"},
		ExtraFields: schemas.BifrostErrorExtraFields{
			RoutingInfo: schemas.RoutingInfo{Provider: schemas.OpenAI},
		},
	}
	plugin.PostLLMHook(ctx, nil, err)

	if !plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("built-in patterns must still work when custom patterns are configured")
	}
}

func TestPluginCustomQuotaPatternStillRespects402(t *testing.T) {
	plugin := NewPlugin(nil)
	defer plugin.Cleanup()

	// Even if a custom pattern matches a 402 message, 402 must still NOT
	// trigger cooldown (permanent billing failure is handled by deadKeyIDs).
	if err := plugin.Init(map[string]any{
		"quota_patterns": []any{"payment required"},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	ctx := newTrailCtx("key-1")
	err := &schemas.BifrostError{
		StatusCode: intPtr(402),
		Error:      &schemas.ErrorField{Message: "payment required"},
		ExtraFields: schemas.BifrostErrorExtraFields{
			RoutingInfo: schemas.RoutingInfo{Provider: schemas.OpenAI},
		},
	}
	plugin.PostLLMHook(ctx, nil, err)

	if plugin.State.IsCoolingDown(schemas.OpenAI, "key-1") {
		t.Fatal("402 must never trigger cooldown, even with a matching custom pattern")
	}
}

func TestPluginIsQuotaExhaustedMethodNoCustomPatterns(t *testing.T) {
	// A plugin constructed directly (no Init) has no custom patterns, so
	// isQuotaExhausted must behave exactly like the package-level
	// IsQuotaExhausted.
	plugin := NewPlugin(nil)
	defer plugin.Cleanup()

	quotaErr := newQuotaError(schemas.OpenAI)
	if !plugin.isQuotaExhausted(quotaErr) {
		t.Fatal("isQuotaExhausted should match built-in quota error")
	}
	transient := &schemas.BifrostError{
		StatusCode: intPtr(429),
		Error:      &schemas.ErrorField{Message: "too many requests"},
	}
	if plugin.isQuotaExhausted(transient) {
		t.Fatal("isQuotaExhausted should not match generic rate limit")
	}
}

// TestParseConfigCapacityKeyLimits pins the plugin-level parsing of per-key
// overrides, including the scope requirement (the same rule Validate and the
// provider write path enforce).
func TestParseConfigCapacityKeyLimits(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{
		"default_ttl_seconds": float64(300),
		"ttl_overrides":       map[string]any{},
		"quota_patterns":      []any{},
		"capacity": map[string]any{
			"enabled":     true,
			"scope":       "key",
			"token_limit": float64(1000),
			"key_limits": map[string]any{
				"key-a": map[string]any{"token_limit": float64(10)},
				"key-b": map[string]any{"request_limit": float64(0)},
			},
		},
	})
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	if cfg.Capacity == nil || len(cfg.Capacity.KeyLimits) != 2 {
		t.Fatalf("expected two per-key overrides, got %+v", cfg.Capacity)
	}
	a := cfg.Capacity.KeyLimits["key-a"]
	if a.TokenLimit == nil || *a.TokenLimit != 10 || a.RequestLimit != nil {
		t.Fatalf("key-a override = %+v, want token_limit=10 and no request limit", a)
	}
	b := cfg.Capacity.KeyLimits["key-b"]
	if b.RequestLimit == nil || *b.RequestLimit != 0 || b.TokenLimit != nil {
		t.Fatalf("key-b override = %+v, want an explicit request_limit=0", b)
	}

	// key_limits is meaningless outside the key scope and must be rejected
	// rather than silently ignored.
	_, err = ParseConfig(map[string]any{
		"default_ttl_seconds": float64(300),
		"ttl_overrides":       map[string]any{},
		"quota_patterns":      []any{},
		"capacity": map[string]any{
			"enabled":    true,
			"scope":      "provider_model",
			"key_limits": map[string]any{"key-a": map[string]any{"token_limit": float64(10)}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "capacity.key_limits requires capacity.scope") {
		t.Fatalf("expected a scope error, got %v", err)
	}

	// An entry that sets nothing is a typo, not a no-op.
	_, err = ParseConfig(map[string]any{
		"default_ttl_seconds": float64(300),
		"ttl_overrides":       map[string]any{},
		"quota_patterns":      []any{},
		"capacity": map[string]any{
			"enabled":    true,
			"scope":      "key",
			"key_limits": map[string]any{"key-a": map[string]any{}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "must set token_limit or request_limit") {
		t.Fatalf("expected an empty-entry error, got %v", err)
	}
}

// TestParseConfigCapacityKeyModelLimits pins the nested per-model layer: it is
// parsed under a per-key entry, inherits that entry, and rejects empty/invalid
// entries instead of silently dropping them.
func TestParseConfigCapacityKeyModelLimits(t *testing.T) {
	base := map[string]any{
		"default_ttl_seconds": float64(300),
		"ttl_overrides":       map[string]any{},
		"quota_patterns":      []any{},
	}
	withCapacity := func(capacity map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range base {
			out[k] = v
		}
		out["capacity"] = capacity
		return out
	}

	cfg, err := ParseConfig(withCapacity(map[string]any{
		"enabled":     true,
		"scope":       "key",
		"token_limit": float64(1000),
		"key_limits": map[string]any{
			"key-a": map[string]any{
				"token_limit": float64(500),
				"models": map[string]any{
					"gpt-4o":      map[string]any{"token_limit": float64(100)},
					"gpt-4o-mini": map[string]any{"request_limit": float64(0)},
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	entry := cfg.Capacity.KeyLimits["key-a"]
	if entry.TokenLimit == nil || *entry.TokenLimit != 500 {
		t.Fatalf("key-level token_limit = %v, want 500", entry.TokenLimit)
	}
	if len(entry.Models) != 2 {
		t.Fatalf("expected two model overrides, got %+v", entry.Models)
	}
	if m := entry.Models["gpt-4o"]; m.TokenLimit == nil || *m.TokenLimit != 100 || m.RequestLimit != nil {
		t.Fatalf("gpt-4o override = %+v, want token_limit=100 only", m)
	}
	if m := entry.Models["gpt-4o-mini"]; m.RequestLimit == nil || *m.RequestLimit != 0 || m.TokenLimit != nil {
		t.Fatalf("gpt-4o-mini override = %+v, want an explicit request_limit=0", m)
	}

	// A models-only credential is valid: no credential-level scalar required.
	if _, err := ParseConfig(withCapacity(map[string]any{
		"enabled": true,
		"scope":   "key",
		"key_limits": map[string]any{
			"key-a": map[string]any{
				"models": map[string]any{"gpt-4o": map[string]any{"request_limit": float64(5)}},
			},
		},
	})); err != nil {
		t.Fatalf("a models-only key_limits section must parse, got %v", err)
	}

	invalid := []struct {
		name     string
		capacity map[string]any
		want     string
	}{
		{
			"empty model name",
			map[string]any{"enabled": true, "scope": "key", "key_limits": map[string]any{
				"key-a": map[string]any{"models": map[string]any{" ": map[string]any{"token_limit": float64(1)}}},
			}},
			"empty model name",
		},
		{
			"model entry with no limit",
			map[string]any{"enabled": true, "scope": "key", "key_limits": map[string]any{
				"key-a": map[string]any{"models": map[string]any{"gpt-4o": map[string]any{}}},
			}},
			"must set token_limit or request_limit",
		},
		{
			"negative model token limit",
			map[string]any{"enabled": true, "scope": "key", "key_limits": map[string]any{
				"key-a": map[string]any{"models": map[string]any{"gpt-4o": map[string]any{"token_limit": float64(-1)}}},
			}},
			"must not be negative",
		},
	}
	for _, tt := range invalid {
		_, err := ParseConfig(withCapacity(tt.capacity))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("%s: err = %v, want it to contain %q", tt.name, err, tt.want)
		}
	}
}
