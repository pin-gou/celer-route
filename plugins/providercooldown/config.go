package providercooldown

import (
	"fmt"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
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
	return c, nil
}

// AsState builds a CooldownState with the parsed config applied. The state
// is independent of the receiver and is safe to mutate further (e.g. via
// SetTTLOverride) without affecting the source Config.
func (c *Config) AsState() *CooldownState {
	ttl := DefaultCooldownTTL
	if c.DefaultTTLSeconds > 0 {
		ttl = time.Duration(c.DefaultTTLSeconds) * time.Second
	}
	s := NewCooldownState(ttl)
	for prov, secs := range c.TTLOverrides {
		if secs <= 0 {
			continue
		}
		s.SetTTLOverride(schemas.ModelProvider(prov), time.Duration(secs)*time.Second)
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
