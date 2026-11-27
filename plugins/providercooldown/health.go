package providercooldown

import (
	"math"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Health-aware key weighting.
//
// A handful of upstream credentials in a shared pool can be chronically
// rate-limited while the rest are healthy. Passive cooldown already removes a
// key for the length of its TTL, but between cooldowns weighted-random still
// gives the bad key its full configured weight, so every rotation burns a
// request on a key that is known to be failing. This subsystem folds each key's
// observed success/failure stream into an EWMA success rate and has the key
// pool filter scale that key's Weight by it, turning "concentrate on healthy
// keys" into an explicit, tunable policy instead of an emergent side effect of
// 429-driven cooldown churn.
//
// Design constraints (see docs/provider-cooldown-optimization):
//
//   - Selection only: nothing here marks a cooldown, refuses a request or
//     changes the bytes sent upstream. A de-prioritised key still gets a small
//     share of the traffic (min_weight_factor > 0), which is its recovery
//     probe.
//   - Disabled is free: the map is nil and every entry point returns after one
//     config resolution, so the default deployment pays no lock and allocates
//     nothing.
//   - Bounded memory: entries are pruned on the same GC tick as the cooldown /
//     per-key tables.
//
// Everything here is guarded by CooldownState.mu — the same mutex that protects
// the cooldowns map — so no additional lock is introduced and observations can
// never interleave with a filter read.

// healthStateRetention is how long a (provider, key, model) health row
// survives without any observation before the GC drops it. It matches the
// per-key counter retention (24h), which is far longer than any cooldown TTL
// (max 1h) and than the maximum EWMA half-life (1h), so a live key's health is
// never pruned while it still matters.
const healthStateRetention = 24 * time.Hour

// minSelectableKeyWeight is the smallest weight WeightedRandom can actually
// select: it converts each Weight to int(Weight*100). Scaling a key below this
// unit would make it unselectable and defeat the min_weight_factor guarantee,
// so a scaled weight that would quantize to zero is floored at one unit
// instead. Keys configured below the unit are left untouched (they were
// already unselectable before health weighting).
const minSelectableKeyWeight = 0.01

// healthKey identifies one health row. It is a struct (not a formatted string)
// so the hot paths never allocate for a map lookup.
type healthKey struct {
	provider schemas.ModelProvider
	keyID    string
	model    string
}

// healthEntry is the mutable, lock-guarded health row.
type healthEntry struct {
	// ewma is the exponential moving average of the success indicator
	// (1 = success, 0 = failure), in [0, 1].
	ewma float64
	// samples counts observations, including the initial one.
	samples int
	// lastAt is when the most recent observation happened; it drives both the
	// EWMA half-life decay and the GC retention check.
	lastAt time.Time
}

// healthState is the lazily allocated health table. Nil means "never
// observed", which is indistinguishable from "disabled" on the read path.
type healthState struct {
	entries map[healthKey]*healthEntry
}

func newHealthState() *healthState {
	return &healthState{entries: make(map[healthKey]*healthEntry)}
}

// enableHealth registers the plugin-level health section as the DEFAULT for
// every provider without its own section. Like enableCapacity it is normally
// called while the state is being built (Config.AsState); the table itself is
// created lazily by the first observation so an enabled-but-unused deployment
// allocates nothing.
func (c *CooldownState) enableHealth(cfg *HealthConfig) {
	if c == nil || cfg == nil || !cfg.Active() {
		return
	}
	c.defaults.health = cfg.Normalized()
}

// ensureHealthLocked returns the health table, creating it on first use. The
// caller MUST hold c.mu (write).
func (c *CooldownState) ensureHealthLocked() *healthState {
	if c.health == nil {
		c.health = newHealthState()
	}
	return c.health
}

// observe folds one terminal outcome for (provider, keyID, model) into that
// row's EWMA. It is a no-op when health weighting is off for the provider, so
// the default deployment never takes the lock here.
//
// Failures are recorded from the per-key failure marker only. PostLLMHook also
// marks a terminal 429, but the marker already sees that same rejection, so
// recording in both places would count one failure twice.
//
// The decay is time-aware: a row observed dt ago is first aged by
// alpha = 1 - 2^(-dt/halfLife), then moved toward the target. A key that failed
// hard and then sat idle for several half-lives therefore snaps back to healthy
// on its very next success — exactly the "recovered upstream" case — instead of
// being penalised for stale history.
func (c *CooldownState) observe(policy *schemas.CooldownPolicy, provider schemas.ModelProvider, keyID string, model string, success bool) {
	if c == nil {
		return
	}
	// Empty keyID is only a legal identity for keyless providers; mirror the
	// mark path's rule so a caller bug cannot attribute health to the whole
	// provider.
	if keyID == "" && !schemas.IsKeylessProvider(provider) {
		return
	}
	spec, ok := healthSpecOf(c.defaults.resolveHealth(policy))
	if !ok {
		return
	}
	target := 0.0
	if success {
		target = 1.0
	}
	now := c.nowTime()

	c.mu.Lock()
	defer c.mu.Unlock()
	hs := c.ensureHealthLocked()
	id := healthKey{provider: provider, keyID: keyID, model: model}
	e, found := hs.entries[id]
	if !found {
		// The first observation seeds the EWMA at the target: there is no
		// history to average against yet.
		hs.entries[id] = &healthEntry{ewma: target, samples: 1, lastAt: now}
		return
	}
	alpha := 1.0
	if !e.lastAt.IsZero() {
		if dt := now.Sub(e.lastAt); dt > 0 {
			alpha = 1 - math.Pow(2, -dt.Seconds()/spec.halfLifeSeconds)
		} else {
			// Same instant (or a clock that went backwards): no time has
			// passed, so keep the previous value and just count the sample.
			alpha = 0
		}
	}
	if alpha < 0 {
		alpha = 0
	} else if alpha > 1 {
		alpha = 1
	}
	e.ewma += alpha * (target - e.ewma)
	e.samples++
	e.lastAt = now
}

// observeSuccess is the PostLLMHook success-path entry point: it records a
// success for the key that served the request. Provider and key are resolved
// the same way the per-key counters resolve them (RoutingInfo + attempt trail),
// so the two views always agree on which credential is being described.
//
// policy is the section the caller already resolved from the context (nil when
// there is none) — passing it in avoids a second context lookup on the success
// path, which runs for every terminal request.
func (c *CooldownState) observeSuccess(policy *schemas.CooldownPolicy, ctx *schemas.BifrostContext, resp *schemas.BifrostResponse) {
	if c == nil {
		return
	}
	// Resolve the effective section BEFORE scanning the context/response: with
	// health off for this provider (the default) the whole success path costs
	// two pointer reads, exactly like the disabled capacity / learning paths.
	if _, ok := healthSpecOf(c.defaults.resolveHealth(policy)); !ok {
		return
	}
	provider, keyID, ok := successProviderAndKey(ctx, resp)
	if !ok {
		return
	}
	model := ""
	if resp != nil {
		if ef := resp.GetExtraFields(); ef != nil {
			model = ef.RoutingInfo.Model
		}
	}
	if model == "" && ctx != nil {
		if ri, ok := ctx.Value(schemas.BifrostContextKeyRoutingInfo).(schemas.RoutingInfo); ok {
			model = ri.Model
		}
	}
	c.observe(policy, provider, keyID, model, true)
}

// applyHealthWeights scales each live key's Weight by its health factor. The
// keys slice is the filter's own output (already a copy of the caller's), so
// mutating the Weight field is safe and never touches the caller's slice.
//
// The caller MUST resolve spec once per filter call (healthSpecOf) so an
// inert configuration skips this call entirely and the hot path stays
// allocation-free and lock-free.
func (c *CooldownState) applyHealthWeights(provider schemas.ModelProvider, model string, keys []schemas.Key, spec healthSpec) {
	if c == nil || len(keys) == 0 {
		return
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	hs := c.health
	if hs == nil {
		return
	}
	for i := range keys {
		id := healthKey{provider: provider, keyID: keys[i].ID, model: model}
		if id.keyID == "" && !schemas.IsKeylessProvider(provider) {
			continue
		}
		e := hs.entries[id]
		if e == nil || e.samples < spec.minSamples {
			// Not enough evidence: keep the configured weight byte-for-byte.
			continue
		}
		factor := e.ewma
		if factor < spec.minWeightFactor {
			factor = spec.minWeightFactor
		} else if factor > 1 {
			factor = 1
		}
		scaled := keys[i].Weight * factor
		// Preserve the exploration guarantee under WeightedRandom's 0.01
		// quantization: a positive configured weight must stay selectable.
		if keys[i].Weight >= minSelectableKeyWeight && scaled < minSelectableKeyWeight {
			scaled = minSelectableKeyWeight
		}
		keys[i].Weight = scaled
	}
}

// cleanupHealthLocked drops the health rows that have not been observed for
// healthStateRetention. The caller MUST hold c.mu (write).
//
// It runs on the same GC tick as the cooldown / escalation / per-key tables so
// every in-memory table of this state shares one bounded cadence.
func (c *CooldownState) cleanupHealthLocked(now time.Time) {
	if c.health == nil {
		return
	}
	for id, e := range c.health.entries {
		if now.Sub(e.lastAt) > healthStateRetention {
			delete(c.health.entries, id)
		}
	}
}

// healthRow is the internal read-only view of one health row, used by tests.
type healthRow struct {
	ewma    float64
	samples int
	lastAt  time.Time
}

// healthRowFor returns a copy of the health row for (provider, keyID, model),
// plus false when none exists. Used by tests; production reads go through
// applyHealthWeights.
func (c *CooldownState) healthRowFor(provider schemas.ModelProvider, keyID string, model string) (healthRow, bool) {
	if c == nil {
		return healthRow{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.health == nil {
		return healthRow{}, false
	}
	e, ok := c.health.entries[healthKey{provider: provider, keyID: keyID, model: model}]
	if !ok {
		return healthRow{}, false
	}
	return healthRow{ewma: e.ewma, samples: e.samples, lastAt: e.lastAt}, true
}

// healthSize returns the number of health rows (tests / diagnostics).
func (c *CooldownState) healthSize() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.health == nil {
		return 0
	}
	return len(c.health.entries)
}
