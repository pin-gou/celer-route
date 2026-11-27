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
	// This intentionally lives in the *plugin* config rather than in
	// schemas.CooldownPolicy: the provider cooldown policy form rebuilds the
	// whole cooldown_policy object from the fields it knows about, silently
	// dropping any sub-field it does not understand. The plugin config has no
	// such form (the UI only toggles the plugin on/off), so fields here
	// survive an unrelated save.
	Escalation *EscalationConfig `json:"escalation,omitempty"`

	// Capacity optionally enables a retrospective budget gate: real token /
	// request usage is accounted after each terminal request, and once a
	// rolling window's usage crosses limit*headroom, subsequent requests are
	// short-circuited until enough of the window slides away.
	//
	// Nil or Enabled=false (the default) keeps the feature completely inert:
	// no state is allocated, no lock is taken, and no request is gated.
	//
	// Like Escalation, this lives in the *plugin* config rather than in
	// schemas.CooldownPolicy: the provider-cooldown policy form rebuilds the
	// whole cooldown_policy object from the fields it knows about, silently
	// dropping unknown sub-fields. The plugin config has no such form, so
	// these fields survive an unrelated save.
	Capacity *CapacityConfig `json:"capacity,omitempty"`
}

// CapacityConfig configures the optional retrospective capacity gate.
//
// The gate never estimates tokens and never reads the request body: it only
// sums the real usage the provider reported (response usage, or
// BilledUsage on a failed-but-billed attempt) over a rolling window, then
// refuses *subsequent* requests once the window's usage crosses
// limit*headroom. The request that crosses the line still goes out — the same
// trade-off the passive cooldown already makes — in exchange for zero
// estimation error and explainable thresholds.
type CapacityConfig struct {
	// Enabled turns the gate on. Default false keeps current behaviour.
	Enabled bool `json:"enabled"`

	// TokenLimit is the per-window token budget. 0 means "no token limit".
	TokenLimit int64 `json:"token_limit"`

	// RequestLimit is the per-window request budget. 0 means "no request
	// limit". At least one of TokenLimit / RequestLimit must be non-zero for
	// the gate to do anything.
	RequestLimit int64 `json:"request_limit"`

	// WindowSeconds is the rolling window length. <= 0 falls back to 60;
	// values are clamped into 10..3600.
	WindowSeconds int `json:"window_seconds"`

	// Headroom is the fraction of the limit that may be used before the gate
	// engages (0.9 = block once 90% of the budget is spent). <= 0 falls back
	// to 0.9; values are clamped into 0.1..1.0.
	Headroom float64 `json:"headroom"`

	// Scope selects the accounting granularity: "provider" (default) shares
	// one budget across every model of a provider, while "provider_model"
	// keeps an independent budget per (provider, model) pair.
	Scope string `json:"scope"`

	// Action decides what happens once the gate engages: "fallback" (default)
	// allows the configured fallback chain to take over, "reject" refuses the
	// request outright.
	Action string `json:"action"`
}

// Normalized returns a copy of the config with defaults applied and values
// clamped the same way ParseConfig does, so callers that build the struct
// directly still get deterministic behaviour.
func (c *CapacityConfig) Normalized() *CapacityConfig {
	if c == nil {
		return nil
	}
	out := *c
	out.WindowSeconds = normalizeCapacityWindowSeconds(out.WindowSeconds)
	out.Headroom = normalizeCapacityHeadroom(out.Headroom)
	out.Scope = normalizeCapacityScope(out.Scope)
	out.Action = normalizeCapacityAction(out.Action)
	return &out
}

// Active reports whether the gate should do any work at all. A gate with no
// limit configured is a no-op even when Enabled is true.
func (c *CapacityConfig) Active() bool {
	return c != nil && c.Enabled && (c.TokenLimit > 0 || c.RequestLimit > 0)
}

func normalizeCapacityWindowSeconds(n int) int {
	switch {
	case n <= 0:
		return defaultCapacityWindowSeconds
	case n < minCapacityWindowSeconds:
		return minCapacityWindowSeconds
	case n > maxCapacityWindowSeconds:
		return maxCapacityWindowSeconds
	default:
		return n
	}
}

func normalizeCapacityHeadroom(h float64) float64 {
	switch {
	case h <= 0:
		return defaultCapacityHeadroom
	case h < minCapacityHeadroom:
		return minCapacityHeadroom
	case h > maxCapacityHeadroom:
		return maxCapacityHeadroom
	default:
		return h
	}
}

func normalizeCapacityScope(s string) string {
	if strings.TrimSpace(strings.ToLower(s)) == CapacityScopeProviderModel {
		return CapacityScopeProviderModel
	}
	return CapacityScopeProvider
}

func normalizeCapacityAction(a string) string {
	if strings.TrimSpace(strings.ToLower(a)) == CapacityActionReject {
		return CapacityActionReject
	}
	return CapacityActionFallback
}

// EscalationConfig controls the optional adaptive TTL upgrade.
//
// The behaviour is expressed purely as a longer TTL — no sleeps, no blocking,
// and no change to the bytes sent upstream.
type EscalationConfig struct {
	// Enabled turns the feature on. Default false keeps current behaviour.
	Enabled bool `json:"enabled"`

	// MaxMultiplier caps how much a single key's TTL can grow (1x, 2x, 4x …).
	// Values <= 0 fall back to defaultEscalationMaxMultiplier; values above
	// maxAllowedEscalationMultiplier are clamped to it.
	MaxMultiplier int `json:"max_multiplier"`
}

// Normalized returns a copy of the config with MaxMultiplier applied the same
// way ParseConfig / AsState apply it, so callers that build the struct
// directly still get deterministic behaviour.
func (e *EscalationConfig) Normalized() *EscalationConfig {
	if e == nil {
		return nil
	}
	out := *e
	out.MaxMultiplier = normalizeMaxMultiplier(out.MaxMultiplier)
	return &out
}

// normalizeMaxMultiplier applies the documented default (4) and upper bound
// (16). Negative / zero means "unset" and takes the default; everything above
// the bound is clamped.
func normalizeMaxMultiplier(n int) int {
	switch {
	case n <= 0:
		return defaultEscalationMaxMultiplier
	case n > maxAllowedEscalationMultiplier:
		return maxAllowedEscalationMultiplier
	default:
		return n
	}
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
		ec.MaxMultiplier = normalizeMaxMultiplier(ec.MaxMultiplier)
		c.Escalation = ec
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
	if esc := c.Escalation.Normalized(); esc != nil && esc.Enabled {
		s.enableEscalation(esc.MaxMultiplier)
	}
	if capCfg := c.Capacity.Normalized(); capCfg.Active() {
		s.enableCapacity(capCfg)
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
