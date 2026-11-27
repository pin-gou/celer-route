package providercooldown

import (
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Per-key observability counters.
//
// The aggregate counters that CooldownStats already exposes (mark_count /
// suppressed_count / by_kind / per_provider) answer "how much is cooling
// down overall" but not "which key is paying for it". For a fleet where a
// handful of upstream credentials are chronically rate-limited while the rest
// are healthy, the per-provider rollup hides exactly the signal the operator
// needs: that one key absorbs 90% of the marks.
//
// These counters are purely additive: they observe the existing mark /
// suppress / success paths without changing any decision they make. They
// exist so later optimisation batches have a per-key baseline to compare
// against (and a regression truth to diff against).

// perKeyStatsRetention is how long a per-key counter row survives without any
// activity before the state GC drops it.
//
// The map is keyed by (provider, keyID) — NOT by (provider, keyID, model) —
// so its cardinality is bounded by the number of distinct upstream credentials
// the gateway has ever talked to, which is small and essentially static.
// The retention window exists only for the churn case (rotated / retired
// credentials): a row that has been idle for a full day cannot take part in
// the daily baseline windows the optimisation batches compare against, so
// dropping it costs nothing and keeps the map from growing without bound.
//
// 24h is deliberately longer than the GC cadence (1 minute) and longer than
// any cooldown TTL (max 1h) so a row is never pruned while the key it
// describes is still relevant.
const perKeyStatsRetention = 24 * time.Hour

// PerKeyStats is the read-only per-(provider, key) rollup exposed under
// stats.per_key.
//
// The counters mirror the aggregate ones one level down:
//
//   - MarkCount is the number of cooldowns recorded for this key.
//   - SuppressedCount is the number of times this key was vetoed by the key
//     pool filter. Unlike the by_kind / per_provider breakdowns this flat
//     counter also covers unclassified (legacy quota_patterns) marks, so it
//     sums to the aggregate suppressed_count.
//   - SuccessCount is the number of requests that completed successfully on
//     this key.
//
// LastMarkAt / LastSuccessAt are nil when the event never happened.
// CooldownRemaining is the time left on the key's longest still-active
// cooldown (0 when the key is not cooling down); it is derived from the same
// snapshot the GET /state endpoint serves, so the two views can never
// disagree.
type PerKeyStats struct {
	MarkCount         uint64        `json:"mark_count"`
	SuppressedCount   uint64        `json:"suppressed_count"`
	SuccessCount      uint64        `json:"success_count"`
	LastMarkAt        *time.Time    `json:"last_mark_at,omitempty"`
	LastSuccessAt     *time.Time    `json:"last_success_at,omitempty"`
	CooldownRemaining time.Duration `json:"cooldown_remaining,omitempty"`
}

// perKeyID is the identity of a per-key counter row. It is a struct rather
// than a formatted "<provider>::<keyID>" string so the hot paths (mark /
// suppress) never allocate for a map lookup — the string form is rendered
// only when the stats snapshot is built.
type perKeyID struct {
	provider schemas.ModelProvider
	keyID    string
}

// perKeyEntry is the mutable, lock-guarded counterpart of PerKeyStats.
// Every field is guarded by CooldownState.mu.
type perKeyEntry struct {
	markCount       uint64
	suppressedCount uint64
	successCount    uint64
	lastMarkAt      time.Time
	lastSuccessAt   time.Time
	// lastActivityAt is the newest of the three events above; it drives the
	// GC retention check.
	lastActivityAt time.Time
}

// perKeyStatsLabel renders the wire label of a per-key row:
// "<provider>::<keyID>". The format mirrors the cooldown map's key prefix so
// the two can be correlated by eye; for keyless providers (whose sentinel
// keyID is empty) the label collapses to "<provider>::", exactly like
// CooldownState.key does.
func perKeyStatsLabel(provider schemas.ModelProvider, keyID string) string {
	return string(provider) + "::" + keyID
}

// perKeyEntryLocked returns the counter row for the given (provider, keyID),
// creating it on demand. The caller MUST hold c.mu (write).
func (c *CooldownState) perKeyEntryLocked(provider schemas.ModelProvider, keyID string, now time.Time) *perKeyEntry {
	if c.perKey == nil {
		c.perKey = make(map[perKeyID]*perKeyEntry)
	}
	id := perKeyID{provider: provider, keyID: keyID}
	e, ok := c.perKey[id]
	if !ok {
		e = &perKeyEntry{}
		c.perKey[id] = e
	}
	e.lastActivityAt = now
	return e
}

// recordPerKeyMarkLocked bumps the per-key mark counter. The caller MUST
// hold c.mu (write).
func (c *CooldownState) recordPerKeyMarkLocked(provider schemas.ModelProvider, keyID string, now time.Time) {
	e := c.perKeyEntryLocked(provider, keyID, now)
	e.markCount++
	e.lastMarkAt = now
}

// recordPerKeySuppressedLocked bumps the per-key suppressed counter. The
// caller MUST hold c.mu (write).
func (c *CooldownState) recordPerKeySuppressedLocked(provider schemas.ModelProvider, keyID string, now time.Time) {
	e := c.perKeyEntryLocked(provider, keyID, now)
	e.suppressedCount++
}

// recordPerKeySuccessLocked bumps the per-key success counter. The caller MUST
// hold c.mu (write).
func (c *CooldownState) recordPerKeySuccessLocked(provider schemas.ModelProvider, keyID string, now time.Time) {
	e := c.perKeyEntryLocked(provider, keyID, now)
	e.successCount++
	e.lastSuccessAt = now
}

// recordPerKeySuccess bumps the per-key success counter for the key that
// served a successful request.
//
// It is a no-op when the (provider, key) pair cannot be resolved — a request
// that never reached a provider (or one whose attempt trail carries no key)
// must not be attributed to an arbitrary key.
func (c *CooldownState) recordPerKeySuccess(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse) {
	if c == nil {
		return
	}
	provider, keyID, ok := successProviderAndKey(ctx, resp)
	if !ok {
		return
	}
	c.mu.Lock()
	c.recordPerKeySuccessLocked(provider, keyID, c.nowTime())
	c.mu.Unlock()
}

// successProviderAndKey resolves the (provider, keyID) pair that served a
// successful request.
//
// The provider comes from the response's RoutingInfo (populated for every
// response type via GetExtraFields) and falls back to the context snapshot
// that core stamps for streaming attempts. The key comes from the attempt
// trail — the last record is the attempt that produced this response.
//
// The empty-keyID rule mirrors the rest of the plugin: it is only a legal
// identity for keyless providers, whose single sentinel key has no ID. For
// any other provider an unresolvable key means "do not attribute", never
// "attribute to the whole provider".
func successProviderAndKey(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse) (schemas.ModelProvider, string, bool) {
	var provider schemas.ModelProvider
	if resp != nil {
		if ef := resp.GetExtraFields(); ef != nil {
			provider = ef.RoutingInfo.Provider
		}
	}
	if provider == "" && ctx != nil {
		if ri, ok := ctx.Value(schemas.BifrostContextKeyRoutingInfo).(schemas.RoutingInfo); ok {
			provider = ri.Provider
		}
	}
	if provider == "" {
		return "", "", false
	}
	keyID := ""
	if ctx != nil {
		if trail, ok := ctx.Value(schemas.BifrostContextKeyAttemptTrail).([]schemas.KeyAttemptRecord); ok {
			// AttemptTrail is append-only and ordered; the last record is the
			// attempt that produced this response.
			for i := len(trail) - 1; i >= 0; i-- {
				rec := trail[i]
				if rec.KeyID == "" && !schemas.IsKeylessProvider(provider) {
					continue
				}
				keyID = rec.KeyID
				break
			}
		}
	}
	if keyID == "" && !schemas.IsKeylessProvider(provider) {
		return "", "", false
	}
	return provider, keyID, true
}

// perKeyStatsSnapshot copies the per-key counters into the exported read-only
// view, rendering each struct key into its "<provider>::<keyID>" wire label
// and attaching the remaining time of the key's longest active cooldown
// (looked up by identity in remaining, which Stats derives from Snapshot()).
// The caller MUST hold c.mu (read).
func (c *CooldownState) perKeyStatsSnapshot(remaining map[perKeyID]time.Duration) map[string]PerKeyStats {
	out := make(map[string]PerKeyStats, len(c.perKey))
	for id, e := range c.perKey {
		row := PerKeyStats{
			MarkCount:         e.markCount,
			SuppressedCount:   e.suppressedCount,
			SuccessCount:      e.successCount,
			CooldownRemaining: remaining[id],
		}
		if !e.lastMarkAt.IsZero() {
			t := e.lastMarkAt
			row.LastMarkAt = &t
		}
		if !e.lastSuccessAt.IsZero() {
			t := e.lastSuccessAt
			row.LastSuccessAt = &t
		}
		out[perKeyStatsLabel(id.provider, id.keyID)] = row
	}
	return out
}

// cleanupPerKeyStatsLocked drops the rows that have seen no activity for
// perKeyStatsRetention. The caller MUST hold c.mu (write).
//
// It runs on the same GC tick as the escalation cleanup so both tables stay
// bounded by the same cadence.
func (c *CooldownState) cleanupPerKeyStatsLocked(now time.Time) {
	for id, e := range c.perKey {
		if now.Sub(e.lastActivityAt) > perKeyStatsRetention {
			delete(c.perKey, id)
		}
	}
}

// perKeyRemainingByKey reduces a cooldown snapshot to the longest still
// active cooldown per (provider, keyID).
//
// A key can carry both a key-granularity and a model-granularity entry; the
// key only becomes usable again once BOTH have lapsed, so the longest
// remaining duration is the one that matters.
func perKeyRemainingByKey(entries []CooldownEntry) map[perKeyID]time.Duration {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[perKeyID]time.Duration, len(entries))
	for _, e := range entries {
		id := perKeyID{provider: e.Provider, keyID: e.KeyID}
		if cur, ok := out[id]; !ok || e.Remaining > cur {
			out[id] = e.Remaining
		}
	}
	return out
}
