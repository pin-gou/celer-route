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

	// storm maps (provider, model) → keyID → last-429 timestamp. Each bucket
	// holds at most the distinct keys that failed in the last
	// escalationStormWindow; empty buckets are dropped so the outer map stays
	// bounded by the number of (provider, model) pairs with recent failures.
	storm map[stormBucketKey]map[string]time.Time
}

func newEscalationState(maxMultiplier int) *escalationState {
	return &escalationState{
		maxMultiplier: schemas.NormalizeEscalationMaxMultiplier(maxMultiplier),
		entries:       make(map[escalationKey]*escalationEntry),
		storm:         make(map[stormBucketKey]map[string]time.Time),
	}
}

// enableEscalation turns on adaptive escalation for the state. Build the state
// and call this before it is shared across goroutines (Config.AsState does
// exactly that); the write is lock-protected, but writers racing with live
// readers would still see a partial switch.
func (c *CooldownState) enableEscalation(maxMultiplier int) {
	if c == nil {
		return
	}
	es := newEscalationState(maxMultiplier)
	c.mu.Lock()
	c.escalation = es
	c.mu.Unlock()
}

// applyLocked returns the TTL to record for this mark, after multiplying the
// already-resolved TTL (retry_after_mode included) by the escalation factor
// and applying the 1h ceiling. The caller MUST hold c.mu (write).
func (e *escalationState) applyLocked(provider schemas.ModelProvider, keyID string, model string, scope schemas.CooldownPolicyScope, ttl time.Duration, now time.Time) time.Duration {
	mult := e.multiplierLocked(provider, keyID, model, scope, now)
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
func (e *escalationState) multiplierLocked(provider schemas.ModelProvider, keyID string, model string, scope schemas.CooldownPolicyScope, now time.Time) int {
	k := escalationKey{provider: provider, keyID: keyID, model: model, scope: scope}
	entry := e.entries[k]
	if entry == nil || now.Sub(entry.lastAt) > escalationWindow {
		entry = &escalationEntry{}
		e.entries[k] = entry
	}
	entry.count++
	entry.lastAt = now

	storm := e.stormActiveLocked(provider, model, now)
	e.recordStormLocked(provider, model, keyID, now)

	mult := 1
	if storm {
		mult = escalationStormMultiplier
	}
	for i := 1; i < entry.count; i++ {
		if mult >= e.maxMultiplier {
			break
		}
		mult *= 2
	}
	if mult > e.maxMultiplier {
		mult = e.maxMultiplier
	}
	return mult
}

// stormActiveLocked reports whether at least escalationStormMinKeys distinct
// keys hit this (provider, model) inside the last escalationStormWindow. The
// bucket is pruned first, so the answer decays automatically as old hits age
// out — no separate reset is needed.
func (e *escalationState) stormActiveLocked(provider schemas.ModelProvider, model string, now time.Time) bool {
	bk := stormBucketKey{provider: provider, model: model}
	bucket := e.storm[bk]
	if bucket == nil {
		return false
	}
	e.pruneStormBucketLocked(bk, bucket, now)
	return len(bucket) >= escalationStormMinKeys
}

// recordStormLocked remembers that keyID just failed for (provider, model).
func (e *escalationState) recordStormLocked(provider schemas.ModelProvider, model string, keyID string, now time.Time) {
	bk := stormBucketKey{provider: provider, model: model}
	bucket := e.storm[bk]
	if bucket == nil {
		bucket = make(map[string]time.Time, escalationStormMinKeys)
		e.storm[bk] = bucket
	}
	bucket[keyID] = now
}

// pruneStormBucketLocked drops keys whose last failure is older than
// escalationStormWindow. An emptied bucket is removed from the outer map so
// the (provider, model) cardinality cannot grow without bound either.
func (e *escalationState) pruneStormBucketLocked(bk stormBucketKey, bucket map[string]time.Time, now time.Time) {
	for keyID, at := range bucket {
		if now.Sub(at) > escalationStormWindow {
			delete(bucket, keyID)
		}
	}
	if len(bucket) == 0 {
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
		delete(bucket, keyID)
		if len(bucket) == 0 {
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
// It returns immediately when escalation is disabled, before touching the
// context — that is what keeps the default configuration zero-overhead.
func (c *CooldownState) clearEscalationOnSuccess(ctx *schemas.BifrostContext) {
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
	// Clear every model/scope counter for this key: a success proves the
	// credential itself is healthy right now, so keeping a stale count from a
	// different model (or from the legacy model-less Mark path) would only
	// over-escalate the next failure.
	c.mu.Lock()
	c.clearEscalationLocked(provider, keyID, "")
	c.mu.Unlock()
}
