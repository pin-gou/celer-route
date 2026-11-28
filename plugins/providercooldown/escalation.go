package providercooldown

import (
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Adaptive cooldown escalation.
//
// Two independent signals lengthen a cooldown, both expressed purely as a
// bigger TTL (no sleeps, no blocking, and nothing that changes the bytes sent
// upstream):
//
//  1. Per-key escalation: the same (provider, key, model) failing repeatedly
//     inside escalationWindow doubles its TTL each time, up to the configured
//     max_multiplier. A successful request clears the count.
//  2. Storm escalation: when at least escalationStormMinKeys distinct keys fail
//     for the same (provider, model) inside escalationStormWindow, the failure
//     is probably a shared upstream condition rather than a bad key — rotating
//     to yet another key rarely helps, so *new* marks start at
//     escalationStormMultiplier instead of 1x. The signal is entirely derived
//     from the rolling window, so it decays on its own once keys stop failing.
//
// The storm signal has two refinements (both active only when the provider's
// chronic section is enabled — see stormSignalPolicy):
//
//  3. Chronically unhealthy keys do not count towards the distinct-key
//     threshold. A fleet where four credentials are chronically broken and two
//     are healthy would otherwise keep the storm signal permanently on, and
//     every healthy key's plain 429 would be inflated to 2x.
//  4. The signal must be FRESH: the failing-key set has to have grown
//     recently. A genuine group outage pushes the count over the threshold in
//     a moment, while a stable split of chronically bad keys keeps the count
//     constant and lets the signal age out.
//
// Everything here is guarded by CooldownState.mu (the same mutex that already
// protects the cooldowns map); no extra lock is introduced. The whole subsystem
// only exists when escalation is explicitly enabled — disabled, c.escalation is
// nil and every entry point returns immediately.

const (
	// escalationWindow is how long a single key's repeat-failure count stays
	// live. A mark that arrives more than this after the key's previous mark
	// restarts the count at 1 (i.e. multiplier back to 1x), so a key that
	// recovered for a while is not punished for ancient history.
	escalationWindow = 300 * time.Second

	// escalationStormWindow is the rolling window for the "many keys failing
	// at once" signal. A key's 429 stays in the window this long.
	escalationStormWindow = 60 * time.Second

	// escalationStormMinKeys is the number of distinct keys that must have hit
	// the same (provider, model) inside escalationStormWindow for the storm
	// signal to fire.
	escalationStormMinKeys = 3

	// escalationStormMultiplier is the starting multiplier for new marks while
	// the storm signal is active.
	escalationStormMultiplier = 2

	// escalationStormFreshnessWindow is how long the storm signal stays
	// "fresh" after the last growth of the failing-key set.
	//
	// A real group outage makes three keys fail within a moment, which grows
	// the set and refreshes the freshness timestamp — the signal fires. A
	// stable split (the same chronically bad keys failing over and over)
	// never grows the set, so the timestamp ages out and the signal stops
	// inflating new marks. 30s is half the storm window: long enough to cover
	// a burst of failures, short enough that a stale split decays before the
	// next window.
	escalationStormFreshnessWindow = 30 * time.Second

	// escalationMaxTTL is the hard ceiling on an escalated TTL, so an abnormal
	// burst can never park a key for hours or days. It only caps escalated
	// TTLs — a longer TTL configured by the operator is left untouched.
	escalationMaxTTL = time.Hour
)

const (
	// defaultEscalationMaxMultiplier applies when max_multiplier is unset
	// (nil / zero / negative).
	defaultEscalationMaxMultiplier = schemas.DefaultEscalationMaxMultiplier

	// maxAllowedEscalationMultiplier is the upper bound max_multiplier is
	// clamped to: 2^4 = 16x is already far beyond any sane cooldown.
	maxAllowedEscalationMultiplier = schemas.MaxEscalationMultiplier
)

// escalationKey identifies one per-key escalation counter. Provider, keyID and
// model must match the cooldown entry being marked; scope is included so a
// key-granularity mark and a model-granularity mark keep independent counts
// (they are independent cooldown entries with independent lifetimes).
type escalationKey struct {
	provider schemas.ModelProvider
	keyID    string
	model    string
	scope    schemas.CooldownPolicyScope
}

// escalationEntry is the per-key counter: how many marks landed inside the
// current window, and when the most recent one happened.
type escalationEntry struct {
	count  int
	lastAt time.Time
}

// stormBucketKey scopes the storm window to one (provider, model) pair.
type stormBucketKey struct {
	provider schemas.ModelProvider
	model    string
}

// stormBucket is the rolling window of failing keys for one (provider, model)
// pair.
//
// members maps a keyID to the timestamp of its last failure. grewAt is the
// moment the member set last GREW (a new key joined the failing set): it is
// what keeps the storm signal "fresh". A stable split of chronically bad keys
// keeps failing without ever growing the set, so grewAt ages out and the
// storm stops inflating new marks.
type stormBucket struct {
	members map[string]time.Time
	grewAt  time.Time
}

// stormSignalPolicy decides how the storm signal is evaluated for one mark.
//
// persistence is true when the provider's chronic section is active. Only then
// does the storm signal use the persistent-failure semantics: chronically
// unhealthy keys are excluded from the distinct-key count, and the signal
// only fires while it is fresh. With chronic off no key is chronically
// unhealthy, so the signal keeps its historical meaning exactly (count the
// distinct keys that failed inside the window) — a deployment that never
// enables chronic therefore behaves byte-for-byte like before.
//
// The struct is passed BY VALUE on purpose. It is a plain value (a bool, a
// pointer and a small spec), so threading it through the mark path allocates
// nothing; a closure would allocate on every mark.
type stormSignalPolicy struct {
	persistence bool
	state       *CooldownState
	spec        chronicSpec
}

// isChronic reports whether the key is currently in chronic long-backoff mode
// for (provider, model).
//
// It reuses the chronic predicate verbatim (isChronicLocked), so the storm
// signal and the chronic backoff always agree on what "chronic" means — there
// is exactly one source of truth (the health table's chronic latch).
func (p stormSignalPolicy) isChronic(provider schemas.ModelProvider, keyID string, model string, now time.Time) bool {
	if !p.persistence || p.state == nil {
		return false
	}
	return p.state.isChronicLocked(provider, keyID, model, p.spec, now)
}

// escalationState is the runtime state for adaptive escalation. Every field
// is guarded by CooldownState.mu; methods named *Locked require the caller to
// hold that write lock.
type escalationState struct {
	maxMultiplier int

	// entries is keyed by escalationKey and bounded by the number of distinct
	// (provider, key, model, scope) tuples that failed within the last
	// escalationWindow. A stale entry restarts at 1 in place on the key's next
	// mark, and the state GC ticker deletes entries that stay idle past the
	// window.
	entries map[escalationKey]*escalationEntry

	// storm maps (provider, model) → the rolling window of failing keys. Each
	// bucket holds at most the distinct keys that failed in the last
	// escalationStormWindow; empty buckets are dropped so the outer map stays
	// bounded by the number of (provider, model) pairs with recent failures.
	storm map[stormBucketKey]*stormBucket
}

func newEscalationState(maxMultiplier int) *escalationState {
	return &escalationState{
		maxMultiplier: schemas.NormalizeEscalationMaxMultiplier(maxMultiplier),
		entries:       make(map[escalationKey]*escalationEntry),
		storm:         make(map[stormBucketKey]*stormBucket),
	}
}

// enableEscalation registers the plugin-level escalation section as the
// DEFAULT for providers without their own section and builds the state
// eagerly (the historical behaviour, so `state.escalation` is non-nil right
// after AsState when the plugin enables it). The whole section is registered —
// max_multiplier and the optional chronic sub-section — so a provider without
// its own escalation section inherits both. Build the state and call this
// before it is shared across goroutines (Config.AsState does exactly that);
// the write is lock-protected, but writers racing with live readers would
// still see a partial switch.
func (c *CooldownState) enableEscalation(cfg *schemas.CooldownEscalationConfig) {
	if c == nil || cfg == nil || !cfg.Enabled {
		return
	}
	norm := cfg.Normalized()
	c.defaults.escalation = norm
	es := newEscalationState(norm.MaxMultiplier)
	c.mu.Lock()
	c.escalation = es
	c.mu.Unlock()
}

// ensureEscalationLocked returns the escalation state, creating it on first
// use. It is only reached for marks whose provider resolved an enabled
// escalation config, so a deployment where no provider enables escalation
// never allocates the entries/storm tables.
//
// The caller MUST hold c.mu (write).
func (c *CooldownState) ensureEscalationLocked() *escalationState {
	if c.escalation != nil {
		return c.escalation
	}
	maxMultiplier := 0
	if c.defaults.escalation != nil {
		maxMultiplier = c.defaults.escalation.MaxMultiplier
	}
	c.escalation = newEscalationState(maxMultiplier)
	return c.escalation
}

// applyLocked returns the TTL to record for this mark, after multiplying the
// already-resolved TTL (retry_after_mode included) by the escalation factor
// and applying the 1h ceiling. maxMultiplier is the effective per-provider cap
// (the provider's own escalation section, or the plugin default). The caller
// MUST hold c.mu (write).
func (e *escalationState) applyLocked(provider schemas.ModelProvider, keyID string, model string, scope schemas.CooldownPolicyScope, ttl time.Duration, maxMultiplier int, now time.Time, storm stormSignalPolicy) time.Duration {
	mult := e.multiplierLocked(provider, keyID, model, scope, maxMultiplier, now, storm)
	if mult <= 1 {
		return ttl
	}
	base := ttl
	ttl *= time.Duration(mult)
	if ttl > escalationMaxTTL {
		ttl = escalationMaxTTL
	}
	// Escalation only ever lengthens a cooldown. A base TTL already above the
	// ceiling (an operator may configure up to 24h per rule) must not be
	// shortened by the ceiling.
	if ttl < base {
		ttl = base
	}
	return ttl
}

// multiplierLocked advances the per-key counter and returns the multiplier to
// apply to this mark. The storm signal is evaluated *before* the current mark
// is recorded, so the key that reaches the threshold is not escalated by its
// own hit — only marks that arrive after the threshold are.
//
// maxMultiplier is the per-provider cap; the storm multiplier is clamped by it
// too, so a provider that configures 1x never escalates even during a storm.
//
// storm carries how the storm signal is judged for this provider (see
// stormSignalPolicy). It is passed by value: no allocation on the hot path.
func (e *escalationState) multiplierLocked(provider schemas.ModelProvider, keyID string, model string, scope schemas.CooldownPolicyScope, maxMultiplier int, now time.Time, storm stormSignalPolicy) int {
	if maxMultiplier < 1 {
		maxMultiplier = 1
	}
	k := escalationKey{provider: provider, keyID: keyID, model: model, scope: scope}
	entry := e.entries[k]
	if entry == nil || now.Sub(entry.lastAt) > escalationWindow {
		entry = &escalationEntry{}
		e.entries[k] = entry
	}
	entry.count++
	entry.lastAt = now

	isStorm := e.stormActiveLocked(provider, model, now, storm)
	e.recordStormLocked(provider, model, keyID, now)

	mult := 1
	if isStorm {
		mult = escalationStormMultiplier
	}
	for i := 1; i < entry.count; i++ {
		if mult >= maxMultiplier {
			break
		}
		mult *= 2
	}
	if mult > maxMultiplier {
		mult = maxMultiplier
	}
	return mult
}

// stormActiveLocked reports whether the storm signal is active for this
// (provider, model). The bucket is pruned first, so the answer decays
// automatically as old hits age out — no separate reset is needed.
//
// Two policies:
//
//   - Historical (policy.persistence == false, i.e. the provider's chronic
//     section is off): at least escalationStormMinKeys distinct keys hit this
//     (provider, model) inside the last escalationStormWindow. This is the
//     original rule, unchanged.
//   - Persistent-failure (chronic active): the same threshold, but
//     chronically unhealthy keys are excluded from the count (a stable split of
//     bad credentials is not a shared outage), and the signal must still be
//     FRESH — the failing-key set has to have grown within
//     escalationStormFreshnessWindow.
func (e *escalationState) stormActiveLocked(provider schemas.ModelProvider, model string, now time.Time, policy stormSignalPolicy) bool {
	bk := stormBucketKey{provider: provider, model: model}
	bucket := e.storm[bk]
	if bucket == nil {
		return false
	}
	e.pruneStormBucketLocked(bk, bucket, now)
	if len(bucket.members) < escalationStormMinKeys {
		return false
	}
	if !policy.persistence {
		return true
	}
	// Persistent-failure semantics: count only the keys that are NOT
	// chronically unhealthy. The predicate is the very same one the chronic
	// backoff uses, so the two views can never disagree about a key.
	distinct := 0
	for keyID := range bucket.members {
		if policy.isChronic(provider, keyID, model, now) {
			continue
		}
		distinct++
		if distinct >= escalationStormMinKeys {
			break
		}
	}
	if distinct < escalationStormMinKeys {
		return false
	}
	// Freshness: the failing-key set must have grown recently. A group outage
	// pushes the count over the threshold within moments (which refreshes
	// grewAt), a stable split never grows it and lets the signal age out.
	return now.Sub(bucket.grewAt) <= escalationStormFreshnessWindow
}

// recordStormLocked remembers that keyID just failed for (provider, model).
// A key that was not a member yet grows the failing set, which refreshes the
// bucket's freshness timestamp.
func (e *escalationState) recordStormLocked(provider schemas.ModelProvider, model string, keyID string, now time.Time) {
	bk := stormBucketKey{provider: provider, model: model}
	bucket := e.storm[bk]
	if bucket == nil {
		bucket = &stormBucket{
			members: make(map[string]time.Time, escalationStormMinKeys),
			grewAt:  now,
		}
		e.storm[bk] = bucket
	}
	if _, existed := bucket.members[keyID]; !existed {
		// The failing-key set grew: the storm signal is fresh again.
		bucket.grewAt = now
	}
	bucket.members[keyID] = now
}

// pruneStormBucketLocked drops keys whose last failure is older than
// escalationStormWindow. An emptied bucket is removed from the outer map so
// the (provider, model) cardinality cannot grow without bound either — the
// freshness timestamp goes with it.
func (e *escalationState) pruneStormBucketLocked(bk stormBucketKey, bucket *stormBucket, now time.Time) {
	for keyID, at := range bucket.members {
		if now.Sub(at) > escalationStormWindow {
			delete(bucket.members, keyID)
		}
	}
	if len(bucket.members) == 0 {
		delete(e.storm, bk)
	}
}

// cleanupLocked prunes every expired escalation counter and storm bucket. The
// state GC ticker calls it once per minute; marks also prune their own storm
// bucket inline, so the maps stay bounded even under a burst of distinct keys.
func (e *escalationState) cleanupLocked(now time.Time) {
	for k, entry := range e.entries {
		if now.Sub(entry.lastAt) > escalationWindow {
			delete(e.entries, k)
		}
	}
	for bk, bucket := range e.storm {
		e.pruneStormBucketLocked(bk, bucket, now)
	}
}

// clearEscalationLocked drops the escalation counters for a (provider, keyID)
// pair, optionally narrowed to one model when model is non-empty. It also
// removes the key from the storm window: a successful call proves the key is
// not currently throttled, so it should stop counting towards the "many keys
// failing" signal.
//
// The caller MUST hold c.mu (write).
func (c *CooldownState) clearEscalationLocked(provider schemas.ModelProvider, keyID string, model string) {
	es := c.escalation
	if es == nil {
		return
	}
	for k := range es.entries {
		if k.provider != provider || k.keyID != keyID {
			continue
		}
		if model != "" && k.model != model {
			continue
		}
		delete(es.entries, k)
	}
	for bk, bucket := range es.storm {
		if bk.provider != provider {
			continue
		}
		if model != "" && bk.model != model {
			continue
		}
		delete(bucket.members, keyID)
		if len(bucket.members) == 0 {
			delete(es.storm, bk)
		}
	}
}

// clearEscalationOnSuccess is the PostLLMHook success-path entry point. It
// resolves the provider and key from the context (RoutingInfo + AttemptTrail,
// the same sources lastAttemptProviderAndKey uses on the failure path) and
// clears every escalation counter for that key — across models and scopes — so
// a recovered credential is not punished for earlier failures.
//
// Chronic hysteresis: when escalation.chronic is enabled for this provider AND
// the key is still in chronic long-backoff mode, the counters are kept. A key
// with a ~15% success rate would otherwise reset its escalation on every lucky
// probe, which is precisely the "single success re-arms the short TTL" bug the
// chronic section exists to remove. PostLLMHook calls observeSuccess first, so
// by the time this runs the current success is already counted; once
// recovery_successes consecutive successes have lifted the chronic latch, the
// next success falls through to the normal clear. With chronic disabled the
// function is byte-for-byte the previous behaviour (one latch check on a nil
// spec).
//
// It returns immediately when escalation is disabled, before touching the
// context — that is what keeps the default configuration zero-overhead.
func (c *CooldownState) clearEscalationOnSuccess(policy *schemas.CooldownPolicy, ctx *schemas.BifrostContext) {
	if c == nil || c.escalation == nil || ctx == nil {
		return
	}
	ri, ok := ctx.Value(schemas.BifrostContextKeyRoutingInfo).(schemas.RoutingInfo)
	if !ok || ri.Provider == "" {
		return
	}
	provider := ri.Provider
	keyID := ""
	if trail, ok := ctx.Value(schemas.BifrostContextKeyAttemptTrail).([]schemas.KeyAttemptRecord); ok {
		// AttemptTrail is append-only and ordered; the last record is the
		// attempt that produced this response. Empty keyID is only legal for
		// keyless providers, mirroring lastAttemptProviderAndKey.
		for i := len(trail) - 1; i >= 0; i-- {
			rec := trail[i]
			if rec.KeyID == "" && !schemas.IsKeylessProvider(provider) {
				continue
			}
			keyID = rec.KeyID
			break
		}
	}
	if keyID == "" && !schemas.IsKeylessProvider(provider) {
		return
	}
	chronic, chronicActive := c.defaults.chronicSpec(policy)
	c.mu.Lock()
	if chronicActive && c.isChronicLocked(provider, keyID, "", chronic, c.nowTime()) {
		// Still chronic: keep the counters so the next probe is escalated the
		// same way it was before. The latch (and therefore this branch) is
		// released by the success observation once recovery_successes is hit.
		c.mu.Unlock()
		return
	}
	// Clear every model/scope counter for this key: a success proves the
	// credential itself is healthy right now, so keeping a stale count from a
	// different model (or from the legacy model-less Mark path) would only
	// over-escalate the next failure.
	c.clearEscalationLocked(provider, keyID, "")
	c.mu.Unlock()
}
