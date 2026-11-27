package schemas

import (
	"fmt"
	"strings"
)

// Bounds and defaults applied to the optional cooldown capability sections.
// They live here (next to the types) so the plugin, the HTTP handlers and the
// per-provider CooldownPolicy all normalize their inputs identically. The
// values are part of the wire contract: changing one changes what an operator
// gets from an "unset" field.
const (
	// DefaultCapacityWindowSeconds is the rolling window used when
	// capacity.window_seconds is unset (zero or negative).
	DefaultCapacityWindowSeconds = 60
	// MinCapacityWindowSeconds / MaxCapacityWindowSeconds bound the rolling
	// window.
	MinCapacityWindowSeconds = 10
	MaxCapacityWindowSeconds = 3600

	// DefaultCapacityHeadroom is the fraction of a limit that may be used
	// before the capacity gate engages when headroom is unset.
	DefaultCapacityHeadroom = 0.9
	// MinCapacityHeadroom / MaxCapacityHeadroom bound the headroom fraction.
	MinCapacityHeadroom = 0.1
	MaxCapacityHeadroom = 1.0

	// CapacityScopeProvider shares one budget across every model of a
	// provider; CapacityScopeProviderModel keeps an independent budget per
	// (provider, model) pair.
	CapacityScopeProvider      = "provider"
	CapacityScopeProviderModel = "provider_model"

	// CapacityActionFallback allows the configured fallback chain to take
	// over once the gate engages; CapacityActionReject refuses the request.
	CapacityActionFallback = "fallback"
	CapacityActionReject   = "reject"

	// DefaultEscalationMaxMultiplier applies when escalation.max_multiplier
	// is unset (zero or negative).
	DefaultEscalationMaxMultiplier = 4
	// MaxEscalationMultiplier is the upper bound max_multiplier is clamped
	// to: 2^4 = 16x is already far beyond any sane cooldown.
	MaxEscalationMultiplier = 16

	// DefaultLearningWindowSeconds is the rolling window the learner uses to
	// measure "tokens already consumed". It matches the order of magnitude of
	// a provider's TPM window (60s) and the capacity gate's default window.
	DefaultLearningWindowSeconds = 60
	MinLearningWindowSeconds     = 10
	MaxLearningWindowSeconds     = 3600

	// DefaultLearningMaxSamples is how many 429 samples are kept per
	// (provider, model) pair. Older samples are dropped first.
	DefaultLearningMaxSamples = 50
	MinLearningMaxSamples     = 5
	MaxLearningMaxSamples     = 500

	// DefaultLearningMinSamples is the number of samples required before an
	// estimate is published. Below it the entry reports "insufficient".
	DefaultLearningMinSamples = 5
	MinLearningMinSamples     = 1
	MaxLearningMinSamples     = 100

	// DefaultHealthHalfLifeSeconds is the EWMA half-life used by the optional
	// health-aware key weighting. 120s is the order of magnitude of a
	// provider's rate-limit recovery window, so a key that recovers is
	// re-weighted quickly.
	DefaultHealthHalfLifeSeconds = 120
	MinHealthHalfLifeSeconds     = 10
	MaxHealthHalfLifeSeconds     = 3600

	// DefaultHealthMinSamples is how many observations a (provider, key,
	// model) triple needs before its health may scale the key's weight. Below
	// it the key keeps its configured weight exactly.
	DefaultHealthMinSamples = 5
	MinHealthMinSamples     = 1
	MaxHealthMinSamples     = 1000

	// DefaultHealthMinWeightFactor is the floor applied to the health factor.
	// It MUST stay above zero: a key that is never selected can never
	// demonstrate recovery, so zero weight would starve the pool instead of
	// merely de-prioritising a key.
	DefaultHealthMinWeightFactor = 0.05
	MinHealthMinWeightFactor     = 0.01
	MaxHealthMinWeightFactor     = 1.0
)

// CooldownCapacityConfig configures the optional retrospective capacity gate.
//
// The gate never estimates tokens and never reads the request body: it only
// sums the real usage the provider reported over a rolling window, then
// refuses *subsequent* requests once the window's usage crosses
// limit*headroom. The request that crosses the line still goes out — the same
// trade-off the passive cooldown already makes — in exchange for zero
// estimation error and explainable thresholds.
//
// It is embedded in both the plugin config and (optionally) a provider's
// CooldownPolicy; a nil pointer means "not configured".
type CooldownCapacityConfig struct {
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
// clamped the same way the plugin's ParseConfig does, so callers that build
// the struct directly still get deterministic behaviour.
func (c *CooldownCapacityConfig) Normalized() *CooldownCapacityConfig {
	if c == nil {
		return nil
	}
	out := *c
	out.WindowSeconds = NormalizeCapacityWindowSeconds(out.WindowSeconds)
	out.Headroom = NormalizeCapacityHeadroom(out.Headroom)
	out.Scope = NormalizeCapacityScope(out.Scope)
	out.Action = NormalizeCapacityAction(out.Action)
	return &out
}

// Active reports whether the gate should do any work at all. A gate with no
// limit configured is a no-op even when Enabled is true.
func (c *CooldownCapacityConfig) Active() bool {
	return c != nil && c.Enabled && (c.TokenLimit > 0 || c.RequestLimit > 0)
}

// Validate performs strict validation for write paths. Unlike Normalized it
// never silently repairs a value: an out-of-range field is reported so the
// caller can surface the mistake. Zero means "unset" for every bounded field
// and is always accepted; the write path stores the raw value and Normalized
// fills the default when it is applied.
func (c *CooldownCapacityConfig) Validate() error {
	if c == nil {
		return nil
	}
	if c.WindowSeconds != 0 && (c.WindowSeconds < MinCapacityWindowSeconds || c.WindowSeconds > MaxCapacityWindowSeconds) {
		return fmt.Errorf(
			"capacity.window_seconds must be between %d and %d (or 0 for the default %d), got %d",
			MinCapacityWindowSeconds, MaxCapacityWindowSeconds, DefaultCapacityWindowSeconds, c.WindowSeconds,
		)
	}
	if c.Headroom != 0 && (c.Headroom < MinCapacityHeadroom || c.Headroom > MaxCapacityHeadroom) {
		return fmt.Errorf(
			"capacity.headroom must be between %v and %v (or 0 for the default %v), got %v",
			MinCapacityHeadroom, MaxCapacityHeadroom, DefaultCapacityHeadroom, c.Headroom,
		)
	}
	if c.TokenLimit < 0 {
		return fmt.Errorf("capacity.token_limit must not be negative, got %d", c.TokenLimit)
	}
	if c.RequestLimit < 0 {
		return fmt.Errorf("capacity.request_limit must not be negative, got %d", c.RequestLimit)
	}
	switch strings.TrimSpace(strings.ToLower(c.Scope)) {
	case "", CapacityScopeProvider, CapacityScopeProviderModel:
	default:
		return fmt.Errorf(
			"capacity.scope %q is invalid; must be %q or %q (or empty for the default)",
			c.Scope, CapacityScopeProvider, CapacityScopeProviderModel,
		)
	}
	switch strings.TrimSpace(strings.ToLower(c.Action)) {
	case "", CapacityActionFallback, CapacityActionReject:
	default:
		return fmt.Errorf(
			"capacity.action %q is invalid; must be %q or %q (or empty for the default)",
			c.Action, CapacityActionFallback, CapacityActionReject,
		)
	}
	return nil
}

// NormalizeCapacityWindowSeconds applies the documented default (60) and
// bounds (10..3600). Zero / negative means "unset" and takes the default;
// positive values outside the bounds are clamped.
func NormalizeCapacityWindowSeconds(n int) int {
	switch {
	case n <= 0:
		return DefaultCapacityWindowSeconds
	case n < MinCapacityWindowSeconds:
		return MinCapacityWindowSeconds
	case n > MaxCapacityWindowSeconds:
		return MaxCapacityWindowSeconds
	default:
		return n
	}
}

// NormalizeCapacityHeadroom applies the documented default (0.9) and bounds
// (0.1..1.0). Zero / negative means "unset" and takes the default; positive
// values outside the bounds are clamped.
func NormalizeCapacityHeadroom(h float64) float64 {
	switch {
	case h <= 0:
		return DefaultCapacityHeadroom
	case h < MinCapacityHeadroom:
		return MinCapacityHeadroom
	case h > MaxCapacityHeadroom:
		return MaxCapacityHeadroom
	default:
		return h
	}
}

// NormalizeCapacityScope resolves an unrecognized or empty scope to the
// default "provider".
func NormalizeCapacityScope(s string) string {
	if strings.TrimSpace(strings.ToLower(s)) == CapacityScopeProviderModel {
		return CapacityScopeProviderModel
	}
	return CapacityScopeProvider
}

// NormalizeCapacityAction resolves an unrecognized or empty action to the
// default "fallback".
func NormalizeCapacityAction(a string) string {
	if strings.TrimSpace(strings.ToLower(a)) == CapacityActionReject {
		return CapacityActionReject
	}
	return CapacityActionFallback
}

// CooldownEscalationConfig controls the optional adaptive TTL upgrade.
//
// The behaviour is expressed purely as a longer TTL — no sleeps, no blocking,
// and no change to the bytes sent upstream. A nil pointer means "not
// configured".
type CooldownEscalationConfig struct {
	// Enabled turns the feature on. Default false keeps current behaviour.
	Enabled bool `json:"enabled"`

	// MaxMultiplier caps how much a single key's TTL can grow (1x, 2x, 4x …).
	// Values <= 0 fall back to DefaultEscalationMaxMultiplier; values above
	// MaxEscalationMultiplier are clamped to it.
	MaxMultiplier int `json:"max_multiplier"`
}

// Normalized returns a copy of the config with MaxMultiplier applied the same
// way the plugin's ParseConfig / AsState apply it, so callers that build the
// struct directly still get deterministic behaviour.
func (e *CooldownEscalationConfig) Normalized() *CooldownEscalationConfig {
	if e == nil {
		return nil
	}
	out := *e
	out.MaxMultiplier = NormalizeEscalationMaxMultiplier(out.MaxMultiplier)
	return &out
}

// Active reports whether escalation should do any work at all.
func (e *CooldownEscalationConfig) Active() bool {
	return e != nil && e.Enabled
}

// Validate performs strict validation for write paths. Zero means "unset" and
// is accepted; Normalized applies the default.
func (e *CooldownEscalationConfig) Validate() error {
	if e == nil {
		return nil
	}
	if e.MaxMultiplier != 0 && (e.MaxMultiplier < 1 || e.MaxMultiplier > MaxEscalationMultiplier) {
		return fmt.Errorf(
			"escalation.max_multiplier must be between 1 and %d (or 0 for the default %d), got %d",
			MaxEscalationMultiplier, DefaultEscalationMaxMultiplier, e.MaxMultiplier,
		)
	}
	return nil
}

// NormalizeEscalationMaxMultiplier applies the documented default (4) and
// upper bound (16). Negative / zero means "unset" and takes the default;
// everything above the bound is clamped.
func NormalizeEscalationMaxMultiplier(n int) int {
	switch {
	case n <= 0:
		return DefaultEscalationMaxMultiplier
	case n > MaxEscalationMultiplier:
		return MaxEscalationMultiplier
	default:
		return n
	}
}

// CooldownLearningConfig configures the optional read-only limit learner.
//
// The learner is completely inert unless Enabled is true: no state is
// allocated, no lock is taken on the request path, and no sample is
// collected. It never writes configuration — the only thing it produces is a
// report. A nil pointer means "not configured".
type CooldownLearningConfig struct {
	// Enabled turns the learner on. Default false keeps current behaviour.
	Enabled bool `json:"enabled"`

	// WindowSeconds is the rolling window used to measure the real token
	// consumption that preceded a rejection. <= 0 falls back to 60; values
	// are clamped into 10..3600.
	WindowSeconds int `json:"window_seconds"`

	// MaxSamples is how many samples are kept per (provider, model) pair.
	// The oldest sample is dropped when the cap is exceeded. <= 0 falls back
	// to 50; values are clamped into 5..500.
	MaxSamples int `json:"max_samples"`

	// MinSamples is how many samples are required before an estimate is
	// published. Below it the entry reports "insufficient". <= 0 falls back
	// to 5; values are clamped into 1..100.
	MinSamples int `json:"min_samples"`
}

// Normalized returns a copy of the config with defaults applied and values
// clamped the same way the plugin's ParseConfig does, so callers that build
// the struct directly still get deterministic behaviour.
func (c *CooldownLearningConfig) Normalized() *CooldownLearningConfig {
	if c == nil {
		return nil
	}
	out := *c
	out.WindowSeconds = NormalizeLearningWindowSeconds(out.WindowSeconds)
	out.MaxSamples = NormalizeLearningMaxSamples(out.MaxSamples)
	out.MinSamples = NormalizeLearningMinSamples(out.MinSamples)
	// A min_samples above max_samples could never be reached (the sample
	// buffer is capped), which would silently disable the learner forever.
	// Pull it down to the cap instead.
	if out.MinSamples > out.MaxSamples {
		out.MinSamples = out.MaxSamples
	}
	return &out
}

// Active reports whether the learner should do any work at all.
func (c *CooldownLearningConfig) Active() bool {
	return c != nil && c.Enabled
}

// Validate performs strict validation for write paths. Zero means "unset" for
// every bounded field and is accepted; Normalized applies the default.
func (c *CooldownLearningConfig) Validate() error {
	if c == nil {
		return nil
	}
	if c.WindowSeconds != 0 && (c.WindowSeconds < MinLearningWindowSeconds || c.WindowSeconds > MaxLearningWindowSeconds) {
		return fmt.Errorf(
			"learning.window_seconds must be between %d and %d (or 0 for the default %d), got %d",
			MinLearningWindowSeconds, MaxLearningWindowSeconds, DefaultLearningWindowSeconds, c.WindowSeconds,
		)
	}
	if c.MaxSamples != 0 && (c.MaxSamples < MinLearningMaxSamples || c.MaxSamples > MaxLearningMaxSamples) {
		return fmt.Errorf(
			"learning.max_samples must be between %d and %d (or 0 for the default %d), got %d",
			MinLearningMaxSamples, MaxLearningMaxSamples, DefaultLearningMaxSamples, c.MaxSamples,
		)
	}
	if c.MinSamples != 0 && (c.MinSamples < MinLearningMinSamples || c.MinSamples > MaxLearningMinSamples) {
		return fmt.Errorf(
			"learning.min_samples must be between %d and %d (or 0 for the default %d), got %d",
			MinLearningMinSamples, MaxLearningMinSamples, DefaultLearningMinSamples, c.MinSamples,
		)
	}
	// Compare the effective values so a min_samples that is valid on its own
	// but above the defaulted max_samples is still rejected instead of being
	// silently pulled down by Normalized.
	maxSamples := c.MaxSamples
	if maxSamples == 0 {
		maxSamples = DefaultLearningMaxSamples
	}
	minSamples := c.MinSamples
	if minSamples == 0 {
		minSamples = DefaultLearningMinSamples
	}
	if minSamples > maxSamples {
		return fmt.Errorf(
			"learning.min_samples must be <= learning.max_samples, got min_samples=%d and max_samples=%d",
			minSamples, maxSamples,
		)
	}
	return nil
}

// NormalizeLearningWindowSeconds applies the documented default (60) and
// bounds (10..3600). Zero / negative means "unset" and takes the default;
// positive values outside the bounds are clamped.
func NormalizeLearningWindowSeconds(n int) int {
	switch {
	case n <= 0:
		return DefaultLearningWindowSeconds
	case n < MinLearningWindowSeconds:
		return MinLearningWindowSeconds
	case n > MaxLearningWindowSeconds:
		return MaxLearningWindowSeconds
	default:
		return n
	}
}

// NormalizeLearningMaxSamples applies the documented default (50) and bounds
// (5..500). Zero / negative means "unset" and takes the default; positive
// values outside the bounds are clamped.
func NormalizeLearningMaxSamples(n int) int {
	switch {
	case n <= 0:
		return DefaultLearningMaxSamples
	case n < MinLearningMaxSamples:
		return MinLearningMaxSamples
	case n > MaxLearningMaxSamples:
		return MaxLearningMaxSamples
	default:
		return n
	}
}

// NormalizeLearningMinSamples applies the documented default (5) and bounds
// (1..100). Zero / negative means "unset" and takes the default; positive
// values outside the bounds are clamped.
func NormalizeLearningMinSamples(n int) int {
	switch {
	case n <= 0:
		return DefaultLearningMinSamples
	case n < MinLearningMinSamples:
		return MinLearningMinSamples
	case n > MaxLearningMinSamples:
		return MaxLearningMinSamples
	default:
		return n
	}
}

// CooldownHealthConfig configures the optional health-aware key weighting.
//
// When enabled, every key's observed success/failure stream is folded into an
// EWMA success rate, and the key-pool filter scales that key's configured
// Weight by it before weighted-random selection. The effect is purely a
// selection probability: no request is ever refused, and no cooldown entry is
// created or shortened. A nil pointer means "not configured".
//
// Disabled is the default and keeps the selection weights byte-for-byte
// identical to the pre-health behaviour.
type CooldownHealthConfig struct {
	// Enabled turns health-aware weighting on. Default false keeps current
	// behaviour.
	Enabled bool `json:"enabled"`

	// HalfLifeSeconds is the EWMA half-life: an observation one half-life old
	// carries half the weight of a fresh one. <= 0 falls back to 120; values
	// are clamped into 10..3600.
	HalfLifeSeconds int `json:"half_life_seconds,omitempty"`

	// MinSamples is how many observations a (provider, key, model) triple
	// needs before its health may scale the weight. <= 0 falls back to 5;
	// values are clamped into 1..1000.
	MinSamples int `json:"min_samples,omitempty"`

	// MinWeightFactor is the lower bound of the scaling factor, so a
	// chronically unhealthy key keeps a small, non-zero share of the traffic
	// and can demonstrate recovery. <= 0 falls back to 0.05; values are
	// clamped into 0.01..1.0. It is never allowed to reach zero.
	MinWeightFactor float64 `json:"min_weight_factor,omitempty"`
}

// Normalized returns a copy of the config with defaults applied and values
// clamped the same way the plugin's ParseConfig does, so callers that build
// the struct directly still get deterministic behaviour.
func (c *CooldownHealthConfig) Normalized() *CooldownHealthConfig {
	if c == nil {
		return nil
	}
	out := *c
	out.HalfLifeSeconds = NormalizeHealthHalfLifeSeconds(out.HalfLifeSeconds)
	out.MinSamples = NormalizeHealthMinSamples(out.MinSamples)
	out.MinWeightFactor = NormalizeHealthMinWeightFactor(out.MinWeightFactor)
	return &out
}

// Active reports whether health-aware weighting should do any work at all.
func (c *CooldownHealthConfig) Active() bool {
	return c != nil && c.Enabled
}

// Validate performs strict validation for write paths. Zero means "unset" for
// every bounded field and is accepted; Normalized applies the default.
func (c *CooldownHealthConfig) Validate() error {
	if c == nil {
		return nil
	}
	if c.HalfLifeSeconds != 0 && (c.HalfLifeSeconds < MinHealthHalfLifeSeconds || c.HalfLifeSeconds > MaxHealthHalfLifeSeconds) {
		return fmt.Errorf(
			"health.half_life_seconds must be between %d and %d (or 0 for the default %d), got %d",
			MinHealthHalfLifeSeconds, MaxHealthHalfLifeSeconds, DefaultHealthHalfLifeSeconds, c.HalfLifeSeconds,
		)
	}
	if c.MinSamples != 0 && (c.MinSamples < MinHealthMinSamples || c.MinSamples > MaxHealthMinSamples) {
		return fmt.Errorf(
			"health.min_samples must be between %d and %d (or 0 for the default %d), got %d",
			MinHealthMinSamples, MaxHealthMinSamples, DefaultHealthMinSamples, c.MinSamples,
		)
	}
	if c.MinWeightFactor != 0 && (c.MinWeightFactor < MinHealthMinWeightFactor || c.MinWeightFactor > MaxHealthMinWeightFactor) {
		return fmt.Errorf(
			"health.min_weight_factor must be between %v and %v (or 0 for the default %v), got %v; "+
				"a factor of zero would starve the key pool, so it is rejected",
			MinHealthMinWeightFactor, MaxHealthMinWeightFactor, DefaultHealthMinWeightFactor, c.MinWeightFactor,
		)
	}
	return nil
}

// NormalizeHealthHalfLifeSeconds applies the documented default (120) and
// bounds (10..3600). Zero / negative means "unset" and takes the default;
// positive values outside the bounds are clamped.
func NormalizeHealthHalfLifeSeconds(n int) int {
	switch {
	case n <= 0:
		return DefaultHealthHalfLifeSeconds
	case n < MinHealthHalfLifeSeconds:
		return MinHealthHalfLifeSeconds
	case n > MaxHealthHalfLifeSeconds:
		return MaxHealthHalfLifeSeconds
	default:
		return n
	}
}

// NormalizeHealthMinSamples applies the documented default (5) and bounds
// (1..1000). Zero / negative means "unset" and takes the default; positive
// values outside the bounds are clamped.
func NormalizeHealthMinSamples(n int) int {
	switch {
	case n <= 0:
		return DefaultHealthMinSamples
	case n < MinHealthMinSamples:
		return MinHealthMinSamples
	case n > MaxHealthMinSamples:
		return MaxHealthMinSamples
	default:
		return n
	}
}

// NormalizeHealthMinWeightFactor applies the documented default (0.05) and
// bounds (0.01..1.0). Zero / negative means "unset" and takes the default;
// positive values below the floor are raised to it so the factor can never
// reach zero and starve the key pool.
func NormalizeHealthMinWeightFactor(f float64) float64 {
	switch {
	case f <= 0:
		return DefaultHealthMinWeightFactor
	case f < MinHealthMinWeightFactor:
		return MinHealthMinWeightFactor
	case f > MaxHealthMinWeightFactor:
		return MaxHealthMinWeightFactor
	default:
		return f
	}
}
