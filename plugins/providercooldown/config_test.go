package providercooldown

import (
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
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
	s := c.AsState()

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
	s := c.AsState()
	if s.ttl != DefaultCooldownTTL {
		t.Fatalf("zero TTL must fall back to DefaultCooldownTTL, got %v", s.ttl)
	}
}

func TestConfigAsStateIgnoresNonPositiveOverrides(t *testing.T) {
	c := &Config{
		DefaultTTLSeconds: 60,
		TTLOverrides: map[string]int{
			"openai":    -5,  // ignored
			"anthropic": 0,   // ignored
		},
	}
	s := c.AsState()
	if _, ok := s.ttlOverrides[schemas.OpenAI]; ok {
		t.Fatal("non-positive openai override should be ignored")
	}
	if _, ok := s.ttlOverrides[schemas.Anthropic]; ok {
		t.Fatal("zero anthropic override should be ignored")
	}
}
