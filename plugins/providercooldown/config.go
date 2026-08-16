package providercooldown

import (
	"fmt"
	"strings"
	"time"

	"github.com/pin-gou/pg-gateway/core/schemas"
)

// Config is the JSON-serializable form parsed from config.json's
// plugins[].config block. Field tags mirror the public JSON keys so the
// struct can be unmarshaled directly when callers want to skip the
// raw-map form.
//
// All time values are stored in seconds (ints) to match the on-disk config
// convention; zero or negative values fall back to the package default.
type Config struct {
	// DefaultTTLSeconds is applied to any provider without an override.
	// <= 0 falls back to DefaultCooldownTTL.
	DefaultTTLSeconds int `json:"default_ttl_seconds"`

	// TTLOverrides maps a provider name to its provider-specific TTL in
	// seconds. Entries <= 0 are ignored at apply time.
	TTLOverrides map[string]int `json:"ttl_overrides"`

	// QuotaPatterns extends the built-in quotaExhaustedSubstrings with
	// additional lower-case substrings that should be treated as quota
	// exhaustion. Useful when a provider returns quota errors with
	// non-standard phrasing. Empty / nil falls back to the built-in list.
	// Patterns are matched against the lower-cased error message via
	// strings.Contains (same semantics as the built-in list).
	QuotaPatterns []string `json:"quota_patterns"`
}

// ParseConfig decodes the raw config map (as received from the plugin
// loader). Unknown keys are surfaced as errors so typos don't silently
// no-op. A nil raw map is treated as an empty config (all defaults).
func ParseConfig(raw map[string]any) (*Config, error) {
	c := &Config{}
	if raw == nil {
		return c, nil
	}
	if v, ok := raw["default_ttl_seconds"]; ok {
		n, err := toInt(v)
		if err != nil {
			return nil, fmt.Errorf("default_ttl_seconds: %w", err)
		}
		c.DefaultTTLSeconds = n
	}
	if v, ok := raw["ttl_overrides"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("ttl_overrides: expected object, got %T", v)
		}
		c.TTLOverrides = make(map[string]int, len(m))
		for k, vv := range m {
			n, err := toInt(vv)
			if err != nil {
				return nil, fmt.Errorf("ttl_overrides[%s]: %w", k, err)
			}
			c.TTLOverrides[k] = n
		}
	}
	if v, ok := raw["quota_patterns"]; ok {
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("quota_patterns: expected array, got %T", v)
		}
		c.QuotaPatterns = make([]string, 0, len(list))
		for i, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("quota_patterns[%d]: expected string, got %T", i, item)
			}
			if s = strings.TrimSpace(strings.ToLower(s)); s != "" {
				c.QuotaPatterns = append(c.QuotaPatterns, s)
			}
		}
	}
	return c, nil
}

// AsState builds a CooldownState with the parsed config applied. The state
// is independent of the receiver and is safe to mutate further (e.g. via
// SetTTLOverride) without affecting the source Config.
//
// If a logger is provided, AsState logs a warning for each ttl_overrides
// key whose name does not match any known Bifrost provider. A typo
// (`"open-ai"` instead of `"openai"`) would otherwise be silently ignored
// and the admin would think their override took effect when it did not.
func (c *Config) AsState(logger schemas.Logger) *CooldownState {
	ttl := DefaultCooldownTTL
	if c.DefaultTTLSeconds > 0 {
		ttl = time.Duration(c.DefaultTTLSeconds) * time.Second
	}
	s := NewCooldownState(ttl)
	for prov, secs := range c.TTLOverrides {
		if secs <= 0 {
			continue
		}
		mp := schemas.ModelProvider(prov)
		if logger != nil && !schemas.IsKnownProvider(prov) {
			logger.Warn(
				"[provider-cooldown] ttl_overrides key %q does not match any known provider; override will be silently ignored",
				prov,
			)
		}
		s.SetTTLOverride(mp, time.Duration(secs)*time.Second)
	}
	return s
}

func toInt(v any) (int, error) {
	switch x := v.(type) {
	case float64:
		return int(x), nil
	case int:
		return x, nil
	case int64:
		return int(x), nil
	case int32:
		return int(x), nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", v)
	}
}
