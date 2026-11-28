package providercooldown

import (
	"math/rand"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Chronic backoff for chronically unhealthy keys.
//
// Escalation alone caps out at base_ttl * max_multiplier: for a 30s rate-limit
// rule with the default 4x that is 120s, which is far too short for an upstream
// account whose quota is consumed outside this gateway. The pool then burns one
// request every two minutes to re-learn the same 429, and because a single
// success clears every counter, a key with a ~15% success rate never stays
// escalated: the lucky probe resets the clock and the probe tax is permanent.
//
// Chronic detection closes both holes with data the health table already keeps:
//
//   - a (provider, key, model) row with enough samples and an EWMA success rate
//     below the threshold is chronically unhealthy, and its marks use
//     chronic_max_ttl_seconds (with jitter) instead of the escalation result;
//   - the row latches, and is only released after recovery_successes
//     consecutive successes (see updateChronicLocked), so recovery is
//     hysteresis-based rather than a single lucky probe.
//
// Half-open probing needs no scheduler: once the long TTL expires the key is
// selectable again (never at zero weight while health weighting is on), so the
// next natural rotation is the probe. A failed probe re-latches the row and the
// next backoff is long again; a successful one advances the recovery counter.
//
// Everything here runs behind CooldownState.mu. The mark path already holds the
// write lock when it resolves a TTL, so the reads below take no extra lock.

// isChronicLocked reports whether (provider, keyID, model) is currently in
// chronic long-backoff mode.
//
// The authoritative state is the row's chronic latch (set when the EWMA /
// sample predicate first fires, released only after recovery_successes
// consecutive successes). The statistical predicate is also evaluated lazily
// here for a row that has not latched yet — that covers a provider whose
// chronic section was enabled after the health rows had already been observed
// (the state is not rebuilt on a policy change) — as long as the row is not
// inside its post-recovery window (consecutive successes at/above the recovery
// threshold). That guard keeps a just-released row from being re-flagged by an
// EWMA that has not caught up yet.
//
// model may be empty: key-scope marks (and the legacy quota_patterns path) have
// no model, so judging them on "no row exists for the empty model" would keep
// every key-scope mark out of chronic mode forever — exactly the path the
// production deployment exercises. Instead, an empty model aggregates every
// model row of that key: any latched row makes the key chronic (a single bad
// model must not hide behind healthy siblings), otherwise the average EWMA
// weighted by sample count is compared with the threshold over the summed
// samples.
//
// now is used only to ignore rows whose evidence has aged past the GC
// retention: a row that old is about to be delivered to the GC, and its EWMA
// describes a different epoch.
//
// The caller MUST hold c.mu (read or write).
func (c *CooldownState) isChronicLocked(provider schemas.ModelProvider, keyID string, model string, spec chronicSpec, now time.Time) bool {
	if c == nil || c.health == nil {
		return false
	}
	if model != "" {
		e := c.health.entries[healthKey{provider: provider, keyID: keyID, model: model}]
		if e == nil || now.Sub(e.lastAt) > healthStateRetention {
			return false
		}
		return chronicRowMatched(e, spec)
	}
	for id, e := range c.health.entries {
		if id.provider != provider || id.keyID != keyID {
			continue
		}
		if now.Sub(e.lastAt) > healthStateRetention {
			continue
		}
		if e.chronic {
			return true
		}
	}
	var weighted float64
	samples := 0
	// eligible tracks whether at least one row is outside its post-recovery
	// window. Without it, a key whose rows were all just released would be
	// re-flagged by an EWMA that has not caught up yet.
	eligible := false
	for id, e := range c.health.entries {
		if id.provider != provider || id.keyID != keyID {
			continue
		}
		if now.Sub(e.lastAt) > healthStateRetention {
			continue
		}
		weighted += e.ewma * float64(e.samples)
		samples += e.samples
		if e.consecutiveSuccesses < spec.recoverySuccesses {
			eligible = true
		}
	}
	if samples == 0 || !eligible {
		return false
	}
	return samples >= spec.minSamples && weighted/float64(samples) < spec.successRateThreshold
}

// chronicRowMatched is the single-row form of the chronic predicate: latched,
// or fresh low-EWMA evidence on a row that is not inside its post-recovery
// window.
func chronicRowMatched(e *healthEntry, spec chronicSpec) bool {
	if e == nil {
		return false
	}
	if e.chronic {
		return true
	}
	if e.consecutiveSuccesses >= spec.recoverySuccesses {
		return false
	}
	return e.samples >= spec.minSamples && e.ewma < spec.successRateThreshold
}

// applyChronicTTLLocked returns the TTL a mark should record, replacing the
// base/escalated value with a jittered chronic_max_ttl when the row is chronic.
//
// Long backoff only ever lengthens a cooldown: the result is floored at
// effective (which is the base TTL after retry_after_mode and escalation), so a
// provider configured with a longer fixed TTL is never shortened. The jitter is
// uniform in [1-jitterRatio, 1+jitterRatio) and exists to de-synchronise keys
// that started failing together (a shared upstream outage otherwise makes the
// whole pool retry in lockstep).
//
// The caller MUST hold c.mu (write).
func (c *CooldownState) applyChronicTTLLocked(provider schemas.ModelProvider, keyID string, model string, spec chronicSpec, effective time.Duration, now time.Time) time.Duration {
	if !c.isChronicLocked(provider, keyID, model, spec, now) {
		return effective
	}
	chronic := time.Duration(spec.maxTTLSeconds) * time.Second
	if spec.jitterRatio > 0 {
		// randFloatLocked returns [0, 1); map it to [-1, 1).
		r := c.randFloatLocked()*2 - 1
		chronic = time.Duration(float64(chronic) * (1 + spec.jitterRatio*r))
	}
	if chronic < effective {
		chronic = effective
	}
	return chronic
}

// randFloatLocked returns a float in [0, 1) for TTL jitter. Tests inject
// randFloat; production uses a state-owned source so the hot path never touches
// math/rand's global generator (and so the sequence is independent per state).
// The source is created lazily on the first chronic mark (never in the default
// deployment) and only ever read under c.mu, which the mark path already holds.
//
// The caller MUST hold c.mu (write).
func (c *CooldownState) randFloatLocked() float64 {
	if c.randFloat != nil {
		return c.randFloat()
	}
	if c.rng == nil {
		c.rng = rand.New(rand.NewSource(c.nowTime().UnixNano()))
	}
	return c.rng.Float64()
}

// chronicKeyCountLocked counts the distinct (provider, keyID) pairs that have
// at least one latched chronic row, for the stats endpoint. It allocates one
// small set per call, which is fine: it only runs for an operator-initiated
// stats read, never on the request path.
//
// The caller MUST hold c.mu (read).
func (c *CooldownState) chronicKeyCountLocked() int {
	if c.health == nil {
		return 0
	}
	seen := make(map[healthKey]struct{})
	for id, e := range c.health.entries {
		if !e.chronic {
			continue
		}
		seen[healthKey{provider: id.provider, keyID: id.keyID}] = struct{}{}
	}
	return len(seen)
}
