package providercooldown

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pin-gou/pg-gateway/core/schemas"
)

const (
	// PluginName is the identifier Bifrost uses to match this plugin in
	// config.json's plugins[] block (e.g. { "name": "provider-cooldown" }).
	PluginName = "provider-cooldown"

	// DefaultCooldownTTL is the default per-(provider, key_id) cooldown duration
	// applied when a quota-exhausted error is observed. The plugin does not have
	// access to HTTP response headers (e.g. Retry-After) at the post-hook layer,
	// so a single default is the simplest correct default.
	DefaultCooldownTTL = 5 * time.Minute

	pluginName = PluginName
)

// quotaExhaustedSubstrings are the lower-case substrings that, when found in
// the rendered error message, indicate a quota / billing exhaustion rather
// than a transient rate-limit hiccup. Transient 429s that don't match any
// of these patterns are intentionally NOT marked — they self-heal on the
// next retry, and over-cooling on them would cause avoidable fallback churn.
//
// "payment required" is intentionally NOT in this list. That string is the
// canonical HTTP 402 reason phrase, which is a permanent billing failure
// handled by bifrost's deadKeyIDs — see IsQuotaExhausted for the full
// rationale.
var quotaExhaustedSubstrings = []string{
	"insufficient_quota",
	"quota exceeded",
	"quota_exceeded",
	"billing",
	"usage limit",
}

// CooldownKind classifies why a (provider, key) was marked for cooldown.
// Used to attribute lifetime mark / suppressed counters to the underlying
// trigger (a transient rate-limit hit vs. a quota exhaustion) so the UI can
// surface "速率限制被标记 7 次 / 抑制 5 次" vs. "配额耗尽被标记 5 次 / 抑制 3 次"
// rather than a single rolled-up total.
//
// Empty ("") means "unclassified" — used by the legacy quota_patterns path
// that does not consult the per-provider CooldownPolicy; those marks still
// bump the lifetime markCount/suppressedCount counters (for backward
// compatibility with the original single-counter API) but are NOT attributed
// to any specific kind.
type CooldownKind string

const (
	CooldownKindRateLimit CooldownKind = "rate_limit"
	CooldownKindQuota     CooldownKind = "quota"
)

// kindIndex maps a CooldownKind to a fixed array index used by the atomic
// counter pair. Unknown / empty kinds are attributed to the legacy "unclassified"
// slot (index 2), which the by_kind JSON output omits — those marks show up
// only in the top-level mark_count / suppressed_count fields.
func kindIndex(k CooldownKind) int {
	switch k {
	case CooldownKindRateLimit:
		return 0
	case CooldownKindQuota:
		return 1
	default:
		return 2
	}
}

const (
	kindIdxUnclassified = 2
	kindIndexCount      = 3
)

// CooldownRecord is the per-entry state stored in CooldownState.cooldowns.
// It pairs the wall-clock expiry with the kind that triggered the mark so
// AsFilter can attribute each suppression to the right kind when the filter
// vetoes the key.
type CooldownRecord struct {
	expiresAt time.Time
	kind      CooldownKind
}

// KindCounters holds lifetime mark + suppressed counters for one (provider, kind)
// bucket. Exposed via Stats() so the monitoring UI can show per-provider
// breakdown (e.g. "OpenAI · 速率限制标记 4 / 抑制 2, 配额标记 1 / 抑制 0").
//
// Safe for concurrent reads/writes via sync/atomic when used standalone; here
// they live inside CooldownState's per-provider map which is itself guarded by
// c.mu, so direct field access is sufficient.
type KindCounters struct {
	MarkCount       uint64 `json:"mark_count"`
	SuppressedCount uint64 `json:"suppressed_count"`
}

// ProviderKindCounters aggregates per-kind counters for a single provider.
// The two kinds are independent (a provider can have rate-limit hits but no
// quota hits, and vice versa).
type ProviderKindCounters struct {
	RateLimit KindCounters `json:"rate_limit"`
	Quota     KindCounters `json:"quota"`
}

// ByKindCounters is the global by-kind rollup surfaced under stats.by_kind.
// "Unclassified" lifetime marks (from the legacy quota_patterns path) do NOT
// appear here — they are reflected only in the top-level mark_count /
// suppressed_count fields to keep the per-kind breakdown faithful to the
// CooldownPolicy-driven classification.
type ByKindCounters struct {
	RateLimit KindCounters `json:"rate_limit"`
	Quota     KindCounters `json:"quota"`
}

// CooldownState holds the per-(provider, key_id) cooldown clock.
//
// Safe for concurrent use. The map is keyed by "<provider>::<keyID>"; values
// are CooldownRecord{expiresAt, kind}. Reads are O(1) with a RWMutex's RLock;
// the GC goroutine prunes expired entries on a ticker to keep the map from
// growing unboundedly under churn.
//
// Lifetime counters are split into three layers:
//
//   - markCount / suppressedCount (atomic): the lifetime total across all
//     kinds, including unclassified. Surfaced as the top-level
//     mark_count / suppressed_count JSON fields; preserved for backward
//     compatibility with the original single-counter API.
//
//   - markByKind / suppressedByKind (atomic, indexed by kindIndex): the
//     same counters broken down by CooldownKind. Surfaced under
//     stats.by_kind.{rate_limit,quota}. Unclassified marks do NOT bump
//     these — only policy-driven classifications do.
//
//   - perProvider (guarded by mu, RWMutex): the same counters broken down
//     by (provider, kind). Surfaced under stats.per_provider.{provider}.
//     {rate_limit,quota}. Used by the per-provider UI section.
type CooldownState struct {
	mu        sync.RWMutex
	cooldowns map[string]CooldownRecord
	ttl       time.Duration

	// ttlOverrides lets callers tune TTL per provider. Useful when a provider
	// resets quota on a known cadence (e.g. daily) and the default 10 minutes
	// would cause avoidable fallback churn after the reset.
	ttlOverrides map[schemas.ModelProvider]time.Duration

	// Lifetime totals (atomic; cover all kinds including unclassified).
	markCount       atomic.Uint64
	suppressedCount atomic.Uint64

	// Lifetime by-kind totals (atomic). Indexed by kindIndex().
	markByKind       [kindIndexCount]atomic.Uint64
	suppressedByKind [kindIndexCount]atomic.Uint64

	// perProvider is keyed by ModelProvider; only providers that have
	// experienced at least one classified mark/suppressed event appear.
	// Guarded by c.mu (RWMutex) for reads/writes.
	perProvider map[schemas.ModelProvider]*ProviderKindCounters
}

// NewCooldownState returns a CooldownState with the given default TTL.
// A non-positive TTL falls back to DefaultCooldownTTL.
func NewCooldownState(ttl time.Duration) *CooldownState {
	if ttl <= 0 {
		ttl = DefaultCooldownTTL
	}
	return &CooldownState{
		cooldowns:    make(map[string]CooldownRecord),
		ttl:          ttl,
		ttlOverrides: make(map[schemas.ModelProvider]time.Duration),
		perProvider:  make(map[schemas.ModelProvider]*ProviderKindCounters),
	}
}

// SetTTLOverride configures a per-provider TTL. Callers can also mutate the
// map after construction; concurrent access is safe.
func (c *CooldownState) SetTTLOverride(provider schemas.ModelProvider, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.ttlOverrides[provider] = ttl
	c.mu.Unlock()
}

func (c *CooldownState) key(provider schemas.ModelProvider, keyID string) string {
	return string(provider) + "::" + keyID
}

// effectiveTTL looks up an override; the caller MUST hold c.mu (read or
// write) for the duration of this call so the override map and the cooldown
// map are observed atomically.
func (c *CooldownState) effectiveTTLLocked(provider schemas.ModelProvider) time.Duration {
	if t, ok := c.ttlOverrides[provider]; ok {
		return t
	}
	return c.ttl
}

// IsCoolingDown reports whether the given (provider, keyID) is currently
// suppressed. Expired entries are pruned lazily on access.
func (c *CooldownState) IsCoolingDown(provider schemas.ModelProvider, keyID string) bool {
	_, _, ok := c.lookupCooldown(provider, keyID)
	return ok
}

// lookupCooldown is the internal accessor used by both IsCoolingDown (for the
// boolean answer) and AsFilter (which also needs the CooldownKind so it can
// attribute suppressions to the right by_kind / per_provider bucket).
// Returns (expiresAt, kind, true) when the entry exists AND is not expired.
// Expired entries are pruned lazily. Callers MUST treat the bool as
// authoritative — the time/kind values are only meaningful when true.
func (c *CooldownState) lookupCooldown(provider schemas.ModelProvider, keyID string) (time.Time, CooldownKind, bool) {
	if keyID == "" {
		return time.Time{}, "", false
	}
	k := c.key(provider, keyID)
	now := time.Now()
	c.mu.RLock()
	rec, ok := c.cooldowns[k]
	c.mu.RUnlock()
	if !ok {
		return time.Time{}, "", false
	}
	if !now.Before(rec.expiresAt) {
		// lazy prune
		c.mu.Lock()
		if cur, ok := c.cooldowns[k]; ok && !time.Now().Before(cur.expiresAt) {
			delete(c.cooldowns, k)
		}
		c.mu.Unlock()
		return time.Time{}, "", false
	}
	return rec.expiresAt, rec.kind, true
}

// Mark records a cooldown for the given (provider, keyID) with no classified
// kind. A no-op when keyID is empty (avoids accidentally cooling "the whole
// provider" when the request hit a non-key-bound error).
//
// Unclassified marks still bump the lifetime markCount counter for backward
// compatibility with the original single-counter API, but do NOT contribute
// to by_kind or per_provider stats. Callers that have a classified reason
// (policy hit) should use MarkWithTTL(..., kind) instead.
func (c *CooldownState) Mark(provider schemas.ModelProvider, keyID string) {
	c.MarkWithTTL(provider, keyID, 0, "")
}

// MarkWithTTL records a cooldown for (provider, keyID) using ttl as the
// cooldown duration, attributing the mark to the given kind (rate_limit or
// quota). A ttl <= 0 falls back to the provider's effective TTL (the configured
// default or per-provider override), preserving the behaviour of Mark.
// keyID empty is a no-op so callers don't need to guard.
//
// kind="rate_limit" / "quota" also bumps by_kind and per_provider counters.
// kind="" only bumps the legacy markCount counter.
func (c *CooldownState) MarkWithTTL(provider schemas.ModelProvider, keyID string, ttl time.Duration, kind CooldownKind) {
	if keyID == "" {
		return
	}
	c.mu.Lock()
	effective := ttl
	if effective <= 0 {
		effective = c.effectiveTTLLocked(provider)
	}
	c.cooldowns[c.key(provider, keyID)] = CooldownRecord{
		expiresAt: time.Now().Add(effective),
		kind:      kind,
	}
	idx := kindIndex(kind)
	c.markByKind[idx].Add(1)
	if kind == CooldownKindRateLimit {
		c.perProviderMarkLocked(provider).RateLimit.MarkCount++
	} else if kind == CooldownKindQuota {
		c.perProviderMarkLocked(provider).Quota.MarkCount++
	}
	c.mu.Unlock()
	c.markCount.Add(1)
}

// perProviderMarkLocked returns the per-provider counter struct for the
// given provider, creating it on demand. The caller MUST hold c.mu (write).
func (c *CooldownState) perProviderMarkLocked(provider schemas.ModelProvider) *ProviderKindCounters {
	pc, ok := c.perProvider[provider]
	if !ok {
		pc = &ProviderKindCounters{}
		c.perProvider[provider] = pc
	}
	return pc
}

// EffectiveTTL returns the TTL that applies to the given provider, respecting
// any per-provider override. Safe for concurrent use.
func (c *CooldownState) EffectiveTTL(provider schemas.ModelProvider) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.effectiveTTLLocked(provider)
}

// Size returns the current number of cooldown entries (including those that
// may have expired but haven't been pruned yet). Primarily for tests and
// debug logging.
func (c *CooldownState) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cooldowns)
}

// CooldownEntry is a read-only view of one cooldown entry, exposed via the
// state-monitoring API. ExpiresAt is the wall-clock time at which the
// cooldown will no longer suppress the key.
//
// Kind carries the CooldownKind that triggered the mark (empty when the
// mark was unclassified, e.g. via the legacy quota_patterns path). Surfaced
// via the API so the UI can render "速率限制" vs "配额耗尽" badges on each row.
type CooldownEntry struct {
	Provider  schemas.ModelProvider `json:"provider"`
	KeyID     string                `json:"key_id"`
	ExpiresAt time.Time             `json:"expires_at"`
	Remaining time.Duration         `json:"remaining"`
	Kind      CooldownKind          `json:"kind,omitempty"`
}

// Snapshot returns a copy of every active (non-expired) cooldown entry,
// suitable for surfacing through a management API. Expired entries are
// pruned lazily as they are encountered; callers wanting an exact snapshot
// may want to call this twice and trust the second result.
func (c *CooldownState) Snapshot() []CooldownEntry {
	now := time.Now()
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]CooldownEntry, 0, len(c.cooldowns))
	for k, rec := range c.cooldowns {
		if !now.Before(rec.expiresAt) {
			continue
		}
		// k is "provider::keyID". Find the first "::" (provider names
		// never contain it; keyIDs come from provider configs).
		i := strings.Index(k, "::")
		if i < 0 {
			continue
		}
		out = append(out, CooldownEntry{
			Provider:  schemas.ModelProvider(k[:i]),
			KeyID:     k[i+2:],
			ExpiresAt: rec.expiresAt,
			Remaining: rec.expiresAt.Sub(now),
			Kind:      rec.kind,
		})
	}
	return out
}

// ClearKey removes any cooldown on the given (provider, keyID). Returns true
// if an entry was removed. Calling on an unknown key or empty keyID is a
// no-op (returns false). Useful when an operator manually un-cools a key
// after fixing the underlying issue (e.g. topping up quota).
func (c *CooldownState) ClearKey(provider schemas.ModelProvider, keyID string) bool {
	if keyID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	k := c.key(provider, keyID)
	if _, ok := c.cooldowns[k]; !ok {
		return false
	}
	delete(c.cooldowns, k)
	return true
}

// CooldownStats is the read-only lifetime summary surfaced via Stats() and
// the GET /api/plugins/provider-cooldown/stats endpoint.
//
//   - MarkCount / SuppressedCount are monotonic counters across the state
//     lifetime covering ALL kinds (including unclassified). Preserved for
//     backward compatibility with the original single-counter API.
//   - CurrentActiveCount is a point-in-time snapshot of how many
//     (provider, key) pairs are mid-cooldown right now.
//   - ByKind splits mark / suppressed counters by CooldownKind (rate_limit
//     vs quota). Unclassified marks do NOT contribute — they only show up
//     in the top-level fields.
//   - PerProvider is the same kind-bucketed counters broken down per
//     provider. Only providers that have experienced at least one
//     classified event appear.
type CooldownStats struct {
	MarkCount          uint64                                         `json:"mark_count"`
	SuppressedCount    uint64                                         `json:"suppressed_count"`
	CurrentActiveCount int                                            `json:"current_active_count"`
	ByKind             ByKindCounters                                 `json:"by_kind"`
	PerProvider        map[schemas.ModelProvider]ProviderKindCounters `json:"per_provider"`
}

// Stats returns the lifetime counters and the current number of active
// cooldowns. Safe for concurrent use.
//
// Returns a fully-populated CooldownStats including the new ByKind and
// PerProvider breakdowns; older callers that read only the top-level
// MarkCount / SuppressedCount / CurrentActiveCount are unaffected by the
// new fields.
func (c *CooldownState) Stats() CooldownStats {
	stats := CooldownStats{
		MarkCount:       c.markCount.Load(),
		SuppressedCount: c.suppressedCount.Load(),
		ByKind: ByKindCounters{
			RateLimit: KindCounters{
				MarkCount:       c.markByKind[kindIndex(CooldownKindRateLimit)].Load(),
				SuppressedCount: c.suppressedByKind[kindIndex(CooldownKindRateLimit)].Load(),
			},
			Quota: KindCounters{
				MarkCount:       c.markByKind[kindIndex(CooldownKindQuota)].Load(),
				SuppressedCount: c.suppressedByKind[kindIndex(CooldownKindQuota)].Load(),
			},
		},
	}
	c.mu.RLock()
	stats.CurrentActiveCount = len(c.cooldowns)
	stats.PerProvider = make(map[schemas.ModelProvider]ProviderKindCounters, len(c.perProvider))
	for p, pc := range c.perProvider {
		stats.PerProvider[p] = *pc
	}
	c.mu.RUnlock()
	return stats
}

// runGCLocked is the inner GC loop, factored out so tests can drive it with a
// short interval and the plugin's auto-started GC can use the default. The
// caller must hold no locks — runGC acquires c.mu itself.
func (c *CooldownState) runGCLocked(logger schemas.Logger, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now()
			c.mu.Lock()
			pruned := 0
			remaining := 0
			for k, rec := range c.cooldowns {
				if !now.Before(rec.expiresAt) {
					delete(c.cooldowns, k)
					pruned++
				}
			}
			remaining = len(c.cooldowns)
			c.mu.Unlock()
			if pruned > 0 && logger != nil {
				logger.Debug("[provider-cooldown] GC pruned %d expired entries, %d remaining", pruned, remaining)
			}
		}
	}
}

// RunGC prunes expired entries on a fixed cadence (default 1 minute). Blocks
// until stop is closed. Safe to invoke from a goroutine without further
// synchronization. New callers should usually rely on the plugin's
// auto-started GC instead — see CooldownPlugin.Cleanup.
//
// Typical usage:
//
//	stop := make(chan struct{})
//	go cooldown.RunGC(stop)
//	defer close(stop)
func (c *CooldownState) RunGC(logger schemas.Logger, stop <-chan struct{}) {
	c.runGCLocked(logger, time.Minute, stop)
}

// AsFilter returns a schemas.KeyPoolFilter that suppresses any key currently
// in cooldown. Wire it into BifrostConfig.KeyPoolFilter at startup. The given
// logger records each suppression decision (may be nil).
//
// Each suppression is attributed to the kind that originally marked the key
// (looked up from the cooldown record), so by_kind / per_provider counters
// reflect the actual reason the request was vetoed — not the most recent
// mark, which can differ if the same key was quota-marked then re-marked
// for rate_limit.
//
// The filter is read-only against the state on the happy path — Bifrost's
// retry loop calls it on every attempt, and we must not introduce
// back-pressure by holding the write lock here. The one write it performs
// (incrementing the suppressed counters per key) is a quick
// perProvider[provider][kind].Suppressed++ under a short write lock; in
// practice this is uncontended because filter calls are not concurrent
// with each other for the same provider.
func (c *CooldownState) AsFilter(logger schemas.Logger) schemas.KeyPoolFilter {
	return func(_ *schemas.BifrostContext, provider schemas.ModelProvider, model string, keys []schemas.Key) ([]schemas.Key, error) {
		if len(keys) == 0 {
			return keys, nil
		}
		out := make([]schemas.Key, 0, len(keys))
		for _, k := range keys {
			_, kind, suppressed := c.lookupCooldown(provider, k.ID)
			if !suppressed {
				out = append(out, k)
				continue
			}
			// Attribute this suppression to the kind that triggered the mark.
			c.suppressedCount.Add(1)
			idx := kindIndex(kind)
			// Unclassified suppressions still bump the legacy total but
			// skip by_kind / per_provider attribution — keeping the
			// per-kind breakdown faithful to CooldownPolicy-driven marks.
			if kind != "" {
				c.suppressedByKind[idx].Add(1)
				c.mu.Lock()
				switch kind {
				case CooldownKindRateLimit:
					c.perProviderMarkLocked(provider).RateLimit.SuppressedCount++
				case CooldownKindQuota:
					c.perProviderMarkLocked(provider).Quota.SuppressedCount++
				}
				c.mu.Unlock()
			}
			if logger != nil {
				logger.Info("[provider-cooldown] suppressed key %s/%s (name=%s, model=%s, kind=%s)", provider, k.ID, k.Name, model, kind)
			}
		}
		return out, nil
	}
}

// AsMarker returns a schemas.PerKeyFailureMarker that bifrost's retry loop
// calls every time it observes a per-key failure it has excluded from the
// current request via usedKeyIDs (the transient branch: 429 / quota-style
// errors; permanent 401/402/403 failures go through deadKeyIDs and are
// intentionally not signalled here because they are already isolated for
// the lifetime of the request).
//
// Wire it into bifrost.SetPerKeyFailureMarker at startup. The given logger
// records each mark and may be nil.
//
// Why a marker is needed even though PostLLMHook already marks on quota
// errors: PostLLMHook fires once on the terminal outcome. When the retry
// loop rotates past a 429 on key A and succeeds on key B, PostLLMHook
// receives (response, nil) and short-circuits — key A never gets marked,
// so the next incoming request reaches for A again, hits 429, rotates
// again, and the whole pool thrashes until quota genuinely recovers.
// The marker closes that gap by letting us mark A the moment its failure
// is observed, not the moment the request as a whole settles.
//
// Classification reuses classifyForMarker() — the same rule lookup
// PostLLMHook.classify uses — so the per-provider CooldownPolicy stamp
// on ctx (or DefaultCooldownPolicy when the stamp is absent) is the single
// source of truth for what counts as rate_limit / quota. The marker
// threads the (provider, keyID, keyName) tuple through directly instead of
// re-deriving it from ctx via lastAttemptProviderAndKey — the retry loop
// already knows which key it just failed with, and skipping the ctx scan
// keeps the marker hot-path allocation-free.
//
// The logger can be nil — a no-op implementation is acceptable.
func (c *CooldownState) AsMarker(logger schemas.Logger) schemas.PerKeyFailureMarker {
	return func(ctx *schemas.BifrostContext, provider schemas.ModelProvider, keyID string, keyName string, model string, bifrostErr *schemas.BifrostError) {
		if c == nil || bifrostErr == nil || keyID == "" {
			return
		}
		// Reuse classify() — single source of truth for rate_limit vs quota
		// matching against the per-provider CooldownPolicy. classify reads
		// the policy from ctx[BifrostContextKeyCooldownPolicy] and falls
		// back to DefaultCooldownPolicy(provider) when the stamp is absent.
		ttl, _, _, kind, ok := classifyForMarker(ctx, provider, keyID, keyName, bifrostErr)
		if !ok {
			return
		}
		c.MarkWithTTL(provider, keyID, ttl, kind)
		if logger != nil {
			logger.Info("[provider-cooldown] marked key %s/%s (name=%s, TTL=%v, kind=%s) from per-key-failure marker", provider, keyID, keyName, ttl, kind)
		}
	}
}

// CooldownPlugin implements schemas.LLMPlugin. Its PostLLMHook inspects the
// terminal BifrostError and, when it looks like a quota exhaustion, marks
// the (provider, keyID) of the last failed attempt as cooling down for the
// state's TTL. Subsequent requests will skip that key during weighted-random
// selection, falling back to the configured Fallbacks chain (or another
// non-cooled key in the same provider's pool) instead of burning a 429.
//
// The logger is used to record cooldown events and filter decisions. Pass a
// nil logger (or a no-op implementation) to suppress logging.
//
// The plugin owns a background GC goroutine that prunes expired entries on
// a 1-minute cadence. NewPlugin and Init start it; Cleanup stops it.
type CooldownPlugin struct {
	State  *CooldownState
	logger schemas.Logger

	// quotaPatterns extends the built-in quotaExhaustedSubstrings. Empty
	// means "use only the built-in list". Populated from config.json's
	// quota_patterns by Init; nil for plugins constructed directly.
	quotaPatterns []string

	gcStop chan struct{}
	gcDone chan struct{}
}

// NewPlugin returns a CooldownPlugin with a fresh state using DefaultCooldownTTL
// and the given logger. The background GC goroutine is started automatically;
// call Cleanup to stop it. For a custom initial TTL use NewPluginWithTTL; for
// a shared or externally-managed state, build the state via NewCooldownState
// and embed it directly. Pass a nil logger to suppress all logging.
func NewPlugin(logger schemas.Logger) *CooldownPlugin {
	return NewPluginWithTTL(logger, DefaultCooldownTTL)
}

// NewPluginWithTTL is like NewPlugin but uses the given initial TTL instead
// of DefaultCooldownTTL. A non-positive TTL falls back to DefaultCooldownTTL
// (matching NewCooldownState). Any subsequent Init call replaces the state
// with one built from config.json, so this TTL only governs the period
// before Init has been invoked.
func NewPluginWithTTL(logger schemas.Logger, ttl time.Duration) *CooldownPlugin {
	p := &CooldownPlugin{
		State:  NewCooldownState(ttl),
		logger: logger,
	}
	p.startGC()
	return p
}

// Init applies the plugin's configuration. It is invoked by Bifrost with
// the raw `config` map from config.json's plugins[].config block before
// any request is served. The raw map may be nil — that is treated as an
// empty config and the plugin keeps its default state.
//
// Init is idempotent: calling it more than once replaces the state with
// one built from the new config and restarts the GC goroutine against the
// new state. Callers that need to preserve a running state across
// reconfigs should call SetTTLOverride directly instead.
func (p *CooldownPlugin) Init(rawConfig any) error {
	var raw map[string]any
	if rawConfig != nil {
		m, ok := rawConfig.(map[string]any)
		if !ok {
			return fmt.Errorf("provider-cooldown: expected map[string]any, got %T", rawConfig)
		}
		raw = m
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		return fmt.Errorf("provider-cooldown: %w", err)
	}
	p.stopGC()
	p.State = cfg.AsState(p.logger)
	p.quotaPatterns = cfg.QuotaPatterns
	p.startGC()
	if p.logger != nil {
		p.logger.Info("[provider-cooldown] initialized (default_ttl=%v, %d provider override(s), %d custom quota pattern(s))",
			p.State.ttl, len(p.State.ttlOverrides), len(p.quotaPatterns))
	}
	return nil
}

// isQuotaExhausted reports whether the error looks like quota exhaustion,
// honoring any quota_patterns configured via config.json. When no custom
// patterns are configured, this is equivalent to the package-level
// IsQuotaExhausted. Custom patterns EXTEND the built-in list (they do not
// replace it), but both are subject to the same status-code gate — a 402
// never triggers cooldown even if a custom pattern matches its message.
// Patterns are matched against the error's message, code, and type fields.
func (p *CooldownPlugin) isQuotaExhausted(err *schemas.BifrostError) bool {
	if err == nil {
		return false
	}
	if IsQuotaExhausted(err) {
		return true
	}
	if len(p.quotaPatterns) == 0 {
		return false
	}
	// Built-in patterns did not match. Apply the same status-code gate, then
	// scan with the custom patterns. This keeps 402/5xx excluded even when a
	// custom pattern happens to match their message text.
	if !statusCodeAllowsQuota(err) {
		return false
	}
	return matchesAnyErrorField(err, p.quotaPatterns)
}

// startGC launches the background GC goroutine on State. Idempotent within
// the same plugin instance only if the previous stop channel has already
// been closed and drained — otherwise it no-ops to avoid goroutine leaks.
func (p *CooldownPlugin) startGC() {
	p.gcStop = make(chan struct{})
	p.gcDone = make(chan struct{})
	go func() {
		defer close(p.gcDone)
		p.State.runGCLocked(p.logger, time.Minute, p.gcStop)
	}()
}

// stopGC closes the GC's stop channel and waits for the goroutine to exit.
// Safe to call multiple times.
func (p *CooldownPlugin) stopGC() {
	if p.gcStop == nil {
		return
	}
	close(p.gcStop)
	<-p.gcDone
	p.gcStop = nil
	p.gcDone = nil
}

// GetName implements schemas.BasePlugin.
func (p *CooldownPlugin) GetName() string { return pluginName }

// Cleanup implements schemas.BasePlugin. Stops the background GC goroutine
// the plugin started in NewPlugin/Init. Idempotent — calling Cleanup more
// than once is a no-op.
func (p *CooldownPlugin) Cleanup() error {
	p.stopGC()
	return nil
}

// Snapshot returns a copy of every active cooldown entry, delegating to the
// current State. Returning nil when the plugin has no state yet lets callers
// distinguish "plugin loaded but never initialized" from "no cooldowns".
func (p *CooldownPlugin) Snapshot() []CooldownEntry {
	if p.State == nil {
		return nil
	}
	return p.State.Snapshot()
}

// ClearKey removes any cooldown on (provider, keyID) from the current State.
// Returns false when the plugin has no state or no entry matched.
func (p *CooldownPlugin) ClearKey(provider schemas.ModelProvider, keyID string) bool {
	if p.State == nil {
		return false
	}
	return p.State.ClearKey(provider, keyID)
}

// Stats returns lifetime counters and the current active cooldown count
// from the loaded State. Returns the zero value when the plugin has no
// state yet (e.g. called before Init).
func (p *CooldownPlugin) Stats() CooldownStats {
	if p.State == nil {
		return CooldownStats{}
	}
	return p.State.Stats()
}

// PreLLMHook is a no-op. The plugin only needs to act on the failure path.
//
// IMPORTANT: must return the request UNCHANGED. The pipeline reassigns its
// working request to this return value (RunLLMPreHooks), so returning nil
// would hand every subsequent plugin a nil request and collapse the whole
// chain into "bifrost request after plugin hooks cannot be nil".
func (p *CooldownPlugin) PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}

// PreRequestHook is a no-op.
func (p *CooldownPlugin) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

// PreProviderHook inspects the targeted provider's key pool against the cooldown
// state and short-circuits the attempt with a synthetic 503 "no_eligible_keys"
// BifrostError when every key is in cooldown. The framework stamps
// ctx[BifrostContextKeyProviderKeys] with the per-provider key snapshot before
// calling this hook, so we never have to round-trip through bifrost.account.
//
// Returning a Silent short-circuit asks presentation plugins (logging) to skip
// their log writes for this attempt — the underlying BifrostError still
// propagates to the caller so the fallback chain can proceed (and the cooldown
// timeline updates cleanly). Without this hook, the request would otherwise
// reach the worker queue, have every key vetoed by AsFilter, surface a
// "no_eligible_keys" 503 there, and force a "spurious" status=cancelled log
// row before the fallback attempt ran.
//
// PreProviderHook is also called for each fallback attempt, so cooldown re-
// checks the fallback provider's key pool before the fallback is enqueued.
// AsFilter inside the worker's keyProvider still runs for the retry /
// unfiltered-key paths so this hook is purely an early-out optimization, not
// the sole veto authority.
func (p *CooldownPlugin) PreProviderHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if p.State == nil || req == nil {
		return req, nil, nil
	}
	provider, model, _ := req.GetRequestFields()
	if provider == "" || model == "" {
		return req, nil, nil
	}

	providerKeysAny := ctx.Value(schemas.BifrostContextKeyProviderKeys)
	providerKeys, _ := providerKeysAny.(map[schemas.ModelProvider][]schemas.Key)
	keys, ok := providerKeys[provider]
	if !ok || len(keys) == 0 {
		// No keys configured for this provider — let the normal pipeline surface
		// the error so the caller sees the usual diagnostics.
		return req, nil, nil
	}

	filter := p.State.AsFilter(p.logger)
	available, err := filter(ctx, provider, model, keys)
	if err != nil {
		// Filter error shouldn't take down the request — let the worker's
		// KeyPoolFilter path handle it.
		if p.logger != nil {
			p.logger.Warn("[provider-cooldown] PreProviderHook filter failed for provider %s: %v", provider, err)
		}
		return req, nil, nil
	}
	if len(available) > 0 {
		return req, nil, nil
	}

	// All keys are in cooldown — synthesize the same 503 that the worker would
	// have produced via AsFilter, but surface it here so logging sees the
	// SilentLog flag and skips the spurious "cancelled" row.
	statusCode := 503
	errType := "no_eligible_keys"
	message := fmt.Sprintf("no eligible keys for provider %s (all in cooldown)", provider)
	if p.logger != nil {
		p.logger.Info("[provider-cooldown] short-circuit on %s/%s — all %d key(s) in cooldown", provider, model, len(keys))
	}
	return req, &schemas.LLMPluginShortCircuit{
		Error: &schemas.BifrostError{
			IsBifrostError: false,
			StatusCode:     &statusCode,
			Type:           &errType,
			Error: &schemas.ErrorField{
				Type:    &errType,
				Message: message,
			},
			// AllowFallbacks stays nil (= allow) so the configured fallback chain
			// can pick up the request, matching the worker's errAllKeysFiltered path.
		},
		Silent: true,
	}, nil
}

// PostLLMHook inspects the response and (when applicable) the error. On a
// quota-exhausted error it reads the (provider, keyID) of the last attempt
// from the AttemptTrail and marks the (provider, key) pair in cooldown.
//
// The hook is intentionally conservative: it never mutates the response or
// error, and it never panics on missing context values — both can occur when
// called from short-circuit or fallback paths.
//
// IMPORTANT: must return the response and error UNCHANGED. The pipeline
// reassigns its working resp/bifrostErr to these return values
// (RunPostLLMHooks), so returning nil,nil would wipe a valid response or
// error for every downstream consumer. A plugin only nils one of them when
// it deliberately recovers from an error (nil err + new resp) or invalidates
// a response (nil resp + new err) — neither applies here.
// classify runs the provider's CooldownPolicy against the BifrostError and
// returns the TTL to apply plus the key to mark. Quota is checked before
// rate_limit; a non-policy match (nil policy, both rules absent) returns ok=false.
//
// The (provider, keyID, keyName) is derived from ctx via lastAttemptProviderAndKey
// — PostLLMHook fires once on the terminal outcome and only has the AttemptTrail
// to consult, so it cannot accept these as arguments. The marker path takes a
// different shape (see classifyForMarker) because the retry loop already knows
// which key it just failed with.
//
// The policy is read from ctx[BifrostContextKeyCooldownPolicy]; when the
// stamp is missing or nil (e.g. older builds without the stamp), the
// function falls back to DefaultCooldownPolicy so every provider still gets
// a sensible rule set.
func (p *CooldownPlugin) classify(ctx *schemas.BifrostContext, bifrostErr *schemas.BifrostError) (ttl time.Duration, keyID string, keyName string, kind CooldownKind, ok bool) {
	if p.State == nil || bifrostErr == nil {
		return 0, "", "", "", false
	}
	provider, keyID, keyName := lastAttemptProviderAndKey(ctx, bifrostErr)
	if keyID == "" {
		return 0, "", "", "", false
	}
	return classifyForMarker(ctx, provider, keyID, keyName, bifrostErr)
}

// classifyForMarker is the marker-side counterpart of (*CooldownPlugin).classify:
// same CooldownPolicy lookup, same rule priority (quota > rate_limit), but it
// takes the (provider, keyID, keyName) tuple as arguments instead of deriving
// them from ctx via lastAttemptProviderAndKey. The retry loop already knows
// which key it just failed with — passing through keeps the marker path free
// of a ctx scan and avoids re-deriving the same value PostLLMHook will later
// derive independently.
//
// The function returns ok=false when the error is not interesting to the
// configured policy; the marker uses that to skip the Mark call entirely so
// we don't bump counters on policy-non-matching errors.
//
// Quota-first precedence mirrors (*CooldownPlugin).classify — a single error
// must never be attributed to both rate_limit and quota.
func classifyForMarker(ctx *schemas.BifrostContext, provider schemas.ModelProvider, keyID string, keyName string, bifrostErr *schemas.BifrostError) (ttl time.Duration, rKeyID string, rKeyName string, kind CooldownKind, ok bool) {
	if bifrostErr == nil || keyID == "" {
		return 0, "", "", "", false
	}
	var policy *schemas.CooldownPolicy
	if ctx != nil {
		if v, has := ctx.Value(schemas.BifrostContextKeyCooldownPolicy).(*schemas.CooldownPolicy); has && v != nil {
			policy = v
		}
	}
	if policy == nil {
		policy = schemas.DefaultCooldownPolicy(provider)
	}
	if policy.Quota != nil && policy.Quota.MatchesRule(bifrostErr) {
		return time.Duration(policy.Quota.TTLSeconds) * time.Second, keyID, keyName, CooldownKindQuota, true
	}
	if policy.RateLimit != nil && policy.RateLimit.MatchesRule(bifrostErr) {
		return time.Duration(policy.RateLimit.TTLSeconds) * time.Second, keyID, keyName, CooldownKindRateLimit, true
	}
	return 0, keyID, keyName, "", false
}

func (p *CooldownPlugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	if p.State == nil || bifrostErr == nil {
		return resp, bifrostErr, nil
	}

	ttl, keyID, keyName, kind, ok := p.classify(ctx, bifrostErr)
	if !ok {
		// Fallback for builds / providers without a per-provider policy stamp:
		// preserve the original quota_patterns behaviour so legacy configs
		// keep working until they migrate to provider.cooldown_policy. The
		// legacy path marks with empty kind (unclassified) so the lifetime
		// markCount still increments for backward compatibility, but the
		// per-kind breakdown stays faithful to CooldownPolicy-driven marks.
		if p.isQuotaExhausted(bifrostErr) {
			provider, fallbackKeyID, fallbackKeyName := lastAttemptProviderAndKey(ctx, bifrostErr)
			if fallbackKeyID == "" {
				return resp, bifrostErr, nil
			}
			p.State.Mark(provider, fallbackKeyID)
			if p.logger != nil {
				p.logger.Info("[provider-cooldown] marked key %s/%s (name=%s, TTL=%v, legacy quota_patterns match)", provider, fallbackKeyID, fallbackKeyName, p.State.EffectiveTTL(provider))
			}
		}
		return resp, bifrostErr, nil
	}

	provider, _, _ := lastAttemptProviderAndKey(ctx, bifrostErr)
	p.State.MarkWithTTL(provider, keyID, ttl, kind)
	if p.logger != nil {
		p.logger.Info("[provider-cooldown] marked key %s/%s (name=%s, TTL=%v, kind=%s)", provider, keyID, keyName, ttl, kind)
	}
	return resp, bifrostErr, nil
}

// IsQuotaExhausted returns true when the given BifrostError looks like the
// provider's per-key quota on this key is exhausted, as opposed to a
// transient 429 that will recover on its own or a permanent billing
// failure that the retry loop already routes around via deadKeyIDs.
//
// Recognised signals:
//   - HTTP 429 is treated as quota when the error's message, code, or type
//     matches one of the known quota-exhaustion substrings (insufficient_quota,
//     quota exceeded, billing, payment required, usage limit). Generic
//     "rate limit" / "too many requests" messages are intentionally NOT
//     treated as quota — they self-heal and over-cooling them would
//     cause unnecessary fallback churn.
//
// HTTP 402 (Payment Required) is intentionally NOT treated as quota:
// billing failures are permanent (a dead account does not recover after
// 10 minutes), so cooldown would let the same dead key get retried
// repeatedly. Bifrost core already routes 402 (along with 401/403) into
// the request-scoped deadKeyIDs set on the first failure, so the plugin
// has nothing to add.
// statusCodeAllowsQuota reports whether the error's status code is in a
// range where a message-based quota scan is even meaningful. Errors with
// no status code, 429, or other 4xx (except 402) pass the gate; 402 and
// non-4xx codes are rejected up front so message scanning never runs on
// them. Shared by IsQuotaExhausted and the plugin's custom-pattern scan so
// both apply the same gate.
func statusCodeAllowsQuota(err *schemas.BifrostError) bool {
	if err.StatusCode == nil {
		return true
	}
	switch *err.StatusCode {
	case 402:
		// Permanent billing failure — handled by bifrost's deadKeyIDs.
		// Don't cooldown: the key is dead, not transiently quota-exhausted.
		return false
	case 429:
		return true
	default:
		return *err.StatusCode >= 400 && *err.StatusCode < 500
	}
}

func IsQuotaExhausted(err *schemas.BifrostError) bool {
	if err == nil {
		return false
	}
	if !statusCodeAllowsQuota(err) {
		return false
	}
	return matchesAnyErrorField(err, quotaExhaustedSubstrings)
}

// matchesAnyErrorField checks whether any of the given patterns appear in the
// error's message, code, or type fields. All comparisons are case-insensitive
// using strings.Contains. Providers often signal quota exhaustion in the code
// or type field (e.g. "insufficient_quota") while the message is a
// human-readable sentence that doesn't contain the same keywords.
func matchesAnyErrorField(err *schemas.BifrostError, patterns []string) bool {
	msg := strings.ToLower(err.GetErrorString())
	for _, sub := range patterns {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	if err.Error != nil {
		if err.Error.Code != nil {
			code := strings.ToLower(*err.Error.Code)
			for _, sub := range patterns {
				if strings.Contains(code, sub) {
					return true
				}
			}
		}
		if err.Error.Type != nil {
			typ := strings.ToLower(*err.Error.Type)
			for _, sub := range patterns {
				if strings.Contains(typ, sub) {
					return true
				}
			}
		}
	}
	return false
}

// lastAttemptProviderAndKey resolves the (provider, keyID, keyName) of the most
// recent failed attempt. Preference order:
//  1. BifrostError.ExtraFields.RoutingInfo.Provider (set by core right
//     before invoking post-hooks; survives the round trip through plugins).
//  2. BifrostError.ExtraFields.Provider (deprecated, but still populated).
//  3. The last KeyAttemptRecord with a non-success outcome (carries keyID).
//
// Returns an empty keyID when none can be determined — callers MUST treat
// that as "do not mark" rather than "mark the whole provider". The keyName is
// only populated when resolved from a KeyAttemptRecord; otherwise it is empty.
func lastAttemptProviderAndKey(ctx *schemas.BifrostContext, bifrostErr *schemas.BifrostError) (schemas.ModelProvider, string, string) {
	var provider schemas.ModelProvider
	if bifrostErr != nil {
		if bifrostErr.ExtraFields.RoutingInfo.Provider != "" {
			provider = bifrostErr.ExtraFields.RoutingInfo.Provider
		} else if bifrostErr.ExtraFields.Provider != "" {
			provider = bifrostErr.ExtraFields.Provider
		}
	}
	if ctx == nil {
		return provider, "", ""
	}
	trail, ok := ctx.Value(schemas.BifrostContextKeyAttemptTrail).([]schemas.KeyAttemptRecord)
	if !ok {
		return provider, "", ""
	}
	// Pick the last record that has a KeyID. AttemptTrail is append-only and
	// ordered, so iterating in reverse finds the most recent attempt.
	for i := len(trail) - 1; i >= 0; i-- {
		rec := trail[i]
		if rec.KeyID == "" {
			continue
		}
		return provider, rec.KeyID, rec.KeyName
	}
	return provider, "", ""
}
