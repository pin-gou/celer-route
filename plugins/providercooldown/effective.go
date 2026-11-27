package providercooldown

import (
	"sort"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Per-provider capability resolution.
//
// The plugin-level capacity / escalation / learning sections act as the
// DEFAULTS for every provider that does not configure its own section. Core
// stamps the provider's schemas.CooldownPolicy on the request context
// (BifrostContextKeyCooldownPolicy, see bifrost.stampProviderCooldownPolicyOn
// Context) before every attempt, so the plugin only has to read it back here.
//
// Resolution is whole-section: a section present on the provider's policy
// replaces the plugin default entirely (no field-level merging), and a section
// that is present but Enabled=false explicitly turns the capability off for
// that provider.
//
// Allocation note: defaults are normalized once when the state is built, and
// an overriding section is exposed as-is. Callers that need normalized scalars
// (gateFor / learnerFor / markWithPolicy) use the non-allocating
// capacitySpecOf / learningSpecOf helpers, which apply exactly the same rules
// as schemas.CooldownCapacityConfig.Normalized / CooldownLearningConfig.
// Normalized. Calling Normalized() on the request path would allocate one
// config copy per request (two on the success path, since capacity and
// learning are both consulted), which the hot path cannot afford.
type capabilityDefaults struct {
	capacity   *schemas.CooldownCapacityConfig
	escalation *schemas.CooldownEscalationConfig
	learning   *schemas.CooldownLearningConfig
	health     *schemas.CooldownHealthConfig
}

// policyFromContext returns the per-provider CooldownPolicy core stamped on the
// context, or nil when there is none (older builds, direct plugin use, or a
// context that never went through an attempt). Nil-safe.
func policyFromContext(ctx *schemas.BifrostContext) *schemas.CooldownPolicy {
	if ctx == nil {
		return nil
	}
	policy, _ := ctx.Value(schemas.BifrostContextKeyCooldownPolicy).(*schemas.CooldownPolicy)
	return policy
}

// resolveCapacity returns the effective capacity section for a provider: the
// provider's own section when present (even when it only disables the gate),
// otherwise the plugin default. The result may be nil (capability unset) and
// is NOT normalized here — see the allocation note above.
func (d *capabilityDefaults) resolveCapacity(policy *schemas.CooldownPolicy) *schemas.CooldownCapacityConfig {
	if policy != nil && policy.Capacity != nil {
		return policy.Capacity
	}
	if d == nil {
		return nil
	}
	return d.capacity
}

// resolveEscalation returns the effective escalation section for a provider.
// Same whole-section replacement rule as resolveCapacity.
func (d *capabilityDefaults) resolveEscalation(policy *schemas.CooldownPolicy) *schemas.CooldownEscalationConfig {
	if policy != nil && policy.Escalation != nil {
		return policy.Escalation
	}
	if d == nil {
		return nil
	}
	return d.escalation
}

// resolveLearning returns the effective learning section for a provider.
// Same whole-section replacement rule as resolveCapacity.
func (d *capabilityDefaults) resolveLearning(policy *schemas.CooldownPolicy) *schemas.CooldownLearningConfig {
	if policy != nil && policy.Learning != nil {
		return policy.Learning
	}
	if d == nil {
		return nil
	}
	return d.learning
}

// resolveHealth returns the effective health section for a provider.
// Same whole-section replacement rule as resolveCapacity: a provider section
// present with only enabled=false disables health weighting for that provider
// even when the plugin default enables it.
func (d *capabilityDefaults) resolveHealth(policy *schemas.CooldownPolicy) *schemas.CooldownHealthConfig {
	if policy != nil && policy.Health != nil {
		return policy.Health
	}
	if d == nil {
		return nil
	}
	return d.health
}

// escalationMaxMultiplier resolves the effective escalation section and
// returns its normalized multiplier plus whether escalation is on for this
// provider. The second return value is false for nil/disabled configs, which
// is what keeps the mark path free of any escalation work by default.
func (d *capabilityDefaults) escalationMaxMultiplier(policy *schemas.CooldownPolicy) (int, bool) {
	cfg := d.resolveEscalation(policy)
	if cfg == nil || !cfg.Enabled {
		return 0, false
	}
	return schemas.NormalizeEscalationMaxMultiplier(cfg.MaxMultiplier), true
}

// capacitySpec is the comparable, already-normalized field set of a capacity
// section. Comparing these scalars is how gateFor detects a configuration
// change without reflect and without allocating.
type capacitySpec struct {
	tokenLimit    int64
	requestLimit  int64
	windowSeconds int64
	headroom      float64
	scope         string
	action        string
}

// capacitySpecOf normalizes cfg into a capacitySpec. The second return value
// is false when the capability is inert (nil, disabled, or no limit set), the
// same predicate as CooldownCapacityConfig.Active.
func capacitySpecOf(cfg *schemas.CooldownCapacityConfig) (capacitySpec, bool) {
	if cfg == nil || !cfg.Enabled || (cfg.TokenLimit <= 0 && cfg.RequestLimit <= 0) {
		return capacitySpec{}, false
	}
	return capacitySpec{
		tokenLimit:    cfg.TokenLimit,
		requestLimit:  cfg.RequestLimit,
		windowSeconds: int64(schemas.NormalizeCapacityWindowSeconds(cfg.WindowSeconds)),
		headroom:      schemas.NormalizeCapacityHeadroom(cfg.Headroom),
		scope:         schemas.NormalizeCapacityScope(cfg.Scope),
		action:        schemas.NormalizeCapacityAction(cfg.Action),
	}, true
}

// learningSpec is the comparable, already-normalized field set of a learning
// section.
type learningSpec struct {
	windowSeconds int64
	maxSamples    int
	minSamples    int
}

// learningSpecOf normalizes cfg into a learningSpec. The second return value
// is false when the learner is inert (nil or disabled), the same predicate as
// CooldownLearningConfig.Active.
func learningSpecOf(cfg *schemas.CooldownLearningConfig) (learningSpec, bool) {
	if cfg == nil || !cfg.Enabled {
		return learningSpec{}, false
	}
	maxSamples := schemas.NormalizeLearningMaxSamples(cfg.MaxSamples)
	minSamples := schemas.NormalizeLearningMinSamples(cfg.MinSamples)
	// A min_samples above max_samples could never be reached (the buffer is
	// capped), so Normalized pulls it down to the cap; mirror that here.
	if minSamples > maxSamples {
		minSamples = maxSamples
	}
	return learningSpec{
		windowSeconds: int64(schemas.NormalizeLearningWindowSeconds(cfg.WindowSeconds)),
		maxSamples:    maxSamples,
		minSamples:    minSamples,
	}, true
}

// capacityOrLearningActive reports whether either capability that consumes
// terminal usage is enabled for the given provider/policy. It exists so the
// post-hook can skip response inspection entirely when both are off (the
// default deployment).
func (c *CooldownState) capacityOrLearningActive(policy *schemas.CooldownPolicy) bool {
	if c == nil {
		return false
	}
	if _, ok := capacitySpecOf(c.defaults.resolveCapacity(policy)); ok {
		return true
	}
	_, ok := learningSpecOf(c.defaults.resolveLearning(policy))
	return ok
}

// healthSpec is the comparable, already-normalized field set of a health
// section. Like capacitySpec / learningSpec it exists so the hot paths can
// compare/apply the effective configuration without calling Normalized (which
// would allocate one config copy per request).
type healthSpec struct {
	halfLifeSeconds float64
	minSamples      int
	minWeightFactor float64
}

// healthSpecOf normalizes cfg into a healthSpec. The second return value is
// false when health weighting is inert (nil or disabled), the same predicate
// as CooldownHealthConfig.Active.
func healthSpecOf(cfg *schemas.CooldownHealthConfig) (healthSpec, bool) {
	if cfg == nil || !cfg.Enabled {
		return healthSpec{}, false
	}
	return healthSpec{
		halfLifeSeconds: float64(schemas.NormalizeHealthHalfLifeSeconds(cfg.HalfLifeSeconds)),
		minSamples:      schemas.NormalizeHealthMinSamples(cfg.MinSamples),
		minWeightFactor: schemas.NormalizeHealthMinWeightFactor(cfg.MinWeightFactor),
	}, true
}

// dedupeLearnedLimits keeps one entry per (provider, model) pair, preferring
// the LAST occurrence. Callers append the plugin-default learner first and the
// per-provider learners after it, so a provider whose section was added while
// the plugin was running reports its current window instead of the stale
// default-learner window.
func dedupeLearnedLimits(in []LearnedLimit) []LearnedLimit {
	if len(in) < 2 {
		return in
	}
	byPair := make(map[string]LearnedLimit, len(in))
	for _, l := range in {
		byPair[l.Provider+"|"+l.Model] = l
	}
	out := make([]LearnedLimit, 0, len(byPair))
	for _, l := range byPair {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out
}
