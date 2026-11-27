package providercooldown

import (
	"fmt"
	"strings"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
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

	// Escalation optionally makes cooldown TTLs grow when the same key keeps
	// failing (and when many keys fail at once). Nil or Enabled=false keeps
	// the pre-escalation behaviour byte-for-byte.
	//
	// This is the plugin-level DEFAULT: a provider whose own
	// schemas.CooldownPolicy carries an escalation section uses that instead
	// (whole-section replacement, no field-level merge). Keeping the default
	// in the plugin config means the provider cooldown policy form — which
	// rebuilds the whole cooldown_policy object from the fields it knows about
	// — cannot silently drop it on an unrelated save.
	Escalation *EscalationConfig `json:"escalation,omitempty"`

	// Deprecated: legacy fleet default. A single plugin-level capacity section
	// is inherited by every provider without its own cooldown_policy.capacity,
	// which is why capacity is now configured per provider (and why the plugin
	// UI no longer edits this key). The field is kept so existing config.json
	// deployments keep gating exactly as before; it is only read, never
	// rewritten by the UI.
	//
	// Capacity optionally enables a retrospective budget gate: real token /
	// request usage is accounted after each terminal request, and once a
	// rolling window's usage crosses limit*headroom, subsequent requests are
	// short-circuited until enough of the window slides away.
	//
	// Nil or Enabled=false (the default) keeps the feature completely inert:
	// no state is allocated, no lock is taken, and no request is gated. This
	// is the plugin-level default; a provider that configures its own capacity
	// section in schemas.CooldownPolicy uses that gate instead (and can switch
	// it off with enabled=false).
	Capacity *CapacityConfig `json:"capacity,omitempty"`

	// Learning optionally enables the read-only limit learner: the plugin
	// watches provider rate-limit rejections and derives an estimate of the
	// provider's effective per-window token limit, which is exposed through a
	// read-only endpoint.
	//
	// Nil or Enabled=false (the default) keeps the feature completely inert:
	// no state is allocated, no sample is collected, and nothing is ever
	// written back to the configuration. The learner only produces a report.
	// This is the plugin-level default; providers can override the window
	// length in their own schemas.CooldownPolicy learning section.
	Learning *LearningConfig `json:"learning,omitempty"`

	// Health optionally enables health-aware key weighting: each key's
	// observed success rate is folded into an EWMA and the key-pool filter
	// scales that key's Weight by it, so chronically failing credentials
	// receive a smaller (never zero) share of the traffic.
	//
	// Nil or Enabled=false (the default) keeps the feature completely inert:
	// no state is allocated, no lock is taken, and every key keeps its
	// configured weight. This is the plugin-level DEFAULT: a provider whose
	// own schemas.CooldownPolicy carries a health section uses that instead
	// (whole-section replacement, no field-level merge), and a section with
	// only enabled=false explicitly turns the feature off for that provider.
	Health *HealthConfig `json:"health,omitempty"`
}

// The capability config types are owned by core/schemas so the plugin config
// and the per-provider CooldownPolicy (schemas.CooldownPolicy) stay
// structurally identical. These aliases keep the historical package-level
// names working at every existing call site.
type (
	CapacityConfig   = schemas.CooldownCapacityConfig
	EscalationConfig = schemas.CooldownEscalationConfig
	LearningConfig   = schemas.CooldownLearningConfig
	HealthConfig     = schemas.CooldownHealthConfig
)

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
	if v, ok := raw["escalation"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("escalation: expected object, got %T", v)
		}
		ec := &EscalationConfig{}
		for k, vv := range m {
			switch k {
			case "enabled":
				b, ok := vv.(bool)
				if !ok {
					return nil, fmt.Errorf("escalation.enabled: expected boolean, got %T", vv)
				}
				ec.Enabled = b
			case "max_multiplier":
				n, err := toInt(vv)
				if err != nil {
					return nil, fmt.Errorf("escalation.max_multiplier: %w", err)
				}
				ec.MaxMultiplier = n
			default:
				return nil, fmt.Errorf("escalation: unknown key %q", k)
			}
		}
		c.Escalation = ec.Normalized()
	}
	if v, ok := raw["capacity"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("capacity: expected object, got %T", v)
		}
		cc := &CapacityConfig{}
		for k, vv := range m {
			switch k {
			case "enabled":
				b, ok := vv.(bool)
				if !ok {
					return nil, fmt.Errorf("capacity.enabled: expected boolean, got %T", vv)
				}
				cc.Enabled = b
			case "token_limit":
				n, err := toInt64(vv)
				if err != nil {
					return nil, fmt.Errorf("capacity.token_limit: %w", err)
				}
				if n < 0 {
					return nil, fmt.Errorf("capacity.token_limit: must not be negative, got %d", n)
				}
				cc.TokenLimit = n
			case "request_limit":
				n, err := toInt64(vv)
				if err != nil {
					return nil, fmt.Errorf("capacity.request_limit: %w", err)
				}
				if n < 0 {
					return nil, fmt.Errorf("capacity.request_limit: must not be negative, got %d", n)
				}
				cc.RequestLimit = n
			case "window_seconds":
				n, err := toInt(vv)
				if err != nil {
					return nil, fmt.Errorf("capacity.window_seconds: %w", err)
				}
				cc.WindowSeconds = n
			case "headroom":
				f, err := toFloat64(vv)
				if err != nil {
					return nil, fmt.Errorf("capacity.headroom: %w", err)
				}
				cc.Headroom = f
			case "scope":
				s, ok := vv.(string)
				if !ok {
					return nil, fmt.Errorf("capacity.scope: expected string, got %T", vv)
				}
				switch strings.TrimSpace(strings.ToLower(s)) {
				case CapacityScopeProvider, CapacityScopeProviderModel:
					cc.Scope = strings.TrimSpace(strings.ToLower(s))
				default:
					return nil, fmt.Errorf("capacity.scope: unknown value %q (expected %q or %q)", s, CapacityScopeProvider, CapacityScopeProviderModel)
				}
			case "action":
				s, ok := vv.(string)
				if !ok {
					return nil, fmt.Errorf("capacity.action: expected string, got %T", vv)
				}
				switch strings.TrimSpace(strings.ToLower(s)) {
				case CapacityActionFallback, CapacityActionReject:
					cc.Action = strings.TrimSpace(strings.ToLower(s))
				default:
					return nil, fmt.Errorf("capacity.action: unknown value %q (expected %q or %q)", s, CapacityActionFallback, CapacityActionReject)
				}
			default:
				return nil, fmt.Errorf("capacity: unknown key %q", k)
			}
		}
		c.Capacity = cc.Normalized()
	}
	if v, ok := raw["learning"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("learning: expected object, got %T", v)
		}
		lc := &LearningConfig{}
		for k, vv := range m {
			switch k {
			case "enabled":
				b, ok := vv.(bool)
				if !ok {
					return nil, fmt.Errorf("learning.enabled: expected boolean, got %T", vv)
				}
				lc.Enabled = b
			case "window_seconds":
				n, err := toInt(vv)
				if err != nil {
					return nil, fmt.Errorf("learning.window_seconds: %w", err)
				}
				lc.WindowSeconds = n
			case "max_samples":
				n, err := toInt(vv)
				if err != nil {
					return nil, fmt.Errorf("learning.max_samples: %w", err)
				}
				lc.MaxSamples = n
			case "min_samples":
				n, err := toInt(vv)
				if err != nil {
					return nil, fmt.Errorf("learning.min_samples: %w", err)
				}
				lc.MinSamples = n
			default:
				return nil, fmt.Errorf("learning: unknown key %q", k)
			}
		}
		c.Learning = lc.Normalized()
	}
	if v, ok := raw["health"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("health: expected object, got %T", v)
		}
		hc := &HealthConfig{}
		for k, vv := range m {
			switch k {
			case "enabled":
				b, ok := vv.(bool)
				if !ok {
					return nil, fmt.Errorf("health.enabled: expected boolean, got %T", vv)
				}
				hc.Enabled = b
			case "half_life_seconds":
				n, err := toInt(vv)
				if err != nil {
					return nil, fmt.Errorf("health.half_life_seconds: %w", err)
				}
				hc.HalfLifeSeconds = n
			case "min_samples":
				n, err := toInt(vv)
				if err != nil {
					return nil, fmt.Errorf("health.min_samples: %w", err)
				}
				hc.MinSamples = n
			case "min_weight_factor":
				f, err := toFloat64(vv)
				if err != nil {
					return nil, fmt.Errorf("health.min_weight_factor: %w", err)
				}
				hc.MinWeightFactor = f
			default:
				return nil, fmt.Errorf("health: unknown key %q", k)
			}
		}
		c.Health = hc.Normalized()
	}
	return c, nil
}

// AsState builds a CooldownState with the parsed config applied. The state
// is independent of the receiver and is safe to mutate further (e.g. via
// SetTTLOverride) without affecting the source Config.
//
// The capability sections are registered as the state's DEFAULTS: each
// provider that configures its own capacity / escalation / learning / health
// section in schemas.CooldownPolicy overrides them at request time, and
// providers without one keep inheriting these. No per-provider state is
// created here — the plugin does not know the provider list.
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
	if esc := c.Escalation.Normalized(); esc != nil && esc.Enabled {
		s.enableEscalation(esc.MaxMultiplier)
	}
	if capCfg := c.Capacity.Normalized(); capCfg.Active() {
		s.enableCapacity(capCfg)
	}
	if learnCfg := c.Learning.Normalized(); learnCfg.Active() {
		s.enableLearning(learnCfg)
	}
	if healthCfg := c.Health.Normalized(); healthCfg.Active() {
		s.enableHealth(healthCfg)
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

func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case float64:
		return int64(x), nil
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	case int32:
		return int64(x), nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", v)
	}
}

func toFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case int32:
		return float64(x), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}
