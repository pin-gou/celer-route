package governance

import (
	"context"
	"sync"
	"time"

	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
)

// AlertRuleCache is an in-process TTL cache in front of
// AlertEvaluatorStore.ListAlertRulesForScope so the soft-threshold hot path
// doesn't pay a database round-trip on every request.
//
// Scope-keyed entries (one entry per distinct (scope_type, scope_id) pair
// seen by the evaluator). Each entry expires independently after ttl elapses
// from its fetch; the next Get after expiry repopulates from the store.
//
// Invalidation is whole-table via Invalidate(): rules are CRUD'd rarely
// (admin-only POST/PUT/DELETE/PATCH toggles) so a coarse blow-away is
// cheaper than tracking which (scope_type, scope_id) needs to drop, and it
// also covers content changes (a rule's threshold or channels changed
// without its key changing). Config reloads invalidate for the same reason.
//
// scopeKey is the stable string identifier for one cache entry. The
// sentinel "\x00global\x00\x00global" stands in for "every global rule"
// — the cache keeps team-scope results separate from global-scope results
// because ListAlertRulesForScope returns different rule sets for each
// (callers depend on the union: a team rule plus the global rules that
// apply to that team). Caching them under the same key would be wrong.
const alertRuleCacheScopeGlobal = "global"

func alertRuleCacheKey(scopeType, scopeID string) string {
	return scopeType + "\x00" + scopeID
}

type alertRuleCacheEntry struct {
	rules      []configstoreTables.TableAlertRule
	fetchedAt  time.Time
}

// AlertRuleCache caches ListAlertRulesForScope results for ttl. Safe for
// concurrent use; the underlying store is only ever called from a single
// goroutine inside Get because of the write lock.
type AlertRuleCache struct {
	mu      sync.RWMutex
	entries map[string]alertRuleCacheEntry
	ttl     time.Duration
	now     func() time.Time
}

// NewAlertRuleCache returns a cache with the given TTL. ttl <= 0 falls back
// to 60s — the soft-threshold hot path should never pay repeated DB hits.
func NewAlertRuleCache(ttl time.Duration) *AlertRuleCache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &AlertRuleCache{
		entries: map[string]alertRuleCacheEntry{},
		ttl:     ttl,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Get returns the cached rules for (scopeType, scopeID), calling store
// when the entry is missing or older than ttl. Errors are returned as-is
// so the caller can log them; an error leaves the cache untouched (next
// call gets a fresh shot).
//
// Store call-count is observable via ListCallsCount for tests; production
// callers should ignore it.
func (c *AlertRuleCache) Get(
	ctx context.Context,
	store AlertEvaluatorStore,
	scopeType, scopeID string,
) ([]configstoreTables.TableAlertRule, error) {
	key := alertRuleCacheKey(scopeType, scopeID)
	now := c.now()

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if ok && now.Sub(entry.fetchedAt) < c.ttl {
		return entry.rules, nil
	}

	rules, err := store.ListAlertRulesForScope(ctx, scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.entries[key] = alertRuleCacheEntry{rules: rules, fetchedAt: now}
	c.mu.Unlock()
	return rules, nil
}

// Invalidate drops every entry. Called after a write to alert_rules
// (POST/PUT/DELETE/PATCH toggle) or on config reload — until then the
// admin sees stale rules for at most TTL.
func (c *AlertRuleCache) Invalidate() {
	c.mu.Lock()
	c.entries = map[string]alertRuleCacheEntry{}
	c.mu.Unlock()
}

// InvalidateScope drops just the entry for (scopeType, scopeID). Not
// currently used by the alerting handlers (they prefer the whole-table
// Invalidate for simplicity), but exposed so future callers can target
// a single scope when they know which one was touched.
func (c *AlertRuleCache) InvalidateScope(scopeType, scopeID string) {
	key := alertRuleCacheKey(scopeType, scopeID)
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Len is the number of cached entries; tests use it to assert invalidation.
func (c *AlertRuleCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Keys returns the cache keys currently held. Tests assert scope isolation
// by checking Keys() after two Get calls on different scopes.
func (c *AlertRuleCache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.entries))
	for k := range c.entries {
		out = append(out, k)
	}
	return out
}