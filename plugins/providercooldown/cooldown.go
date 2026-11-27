package providercooldown

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

const (
	// PluginName is the identifier Bifrost uses to match this plugin in
	// config.json's plugins[] block (e.g. { "name": "provider-cooldown" }).
	PluginName = "provider-cooldown"

	// DefaultCooldownTTL is the default per-(provider, key_id) cooldown duration
	// applied when a quota-exhausted error is observed. The per-provider
	// CooldownPolicy supersedes it; this constant is only the legacy fallback
	// for configs that still use quota_patterns. Provider retry hints
	// (Retry-After / x-ratelimit-reset-*) are honoured only when a policy rule
	// opts in via retry_after_mode (see effectiveCooldownTTL).
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

	// escalation, when non-nil, enables adaptive TTL escalation (see
	// escalation.go). It is created eagerly only when the plugin-level
	// default enables it; a state where only some providers enable escalation
	// through their own CooldownPolicy creates it lazily on the first mark
	// that needs it. Nil plus an inert defaults.escalation means the whole
	// subsystem stays inert: no extra state is read, no TTL is scaled, and the
	// Mark path is unchanged.
	escalation *escalationState

	// health, when non-nil, holds the per-(provider, key, model) EWMA success
	// rates used by health-aware key weighting (see health.go). It is created
	// lazily by the first observation of a provider whose effective health
	// section is enabled, so a deployment with the feature off keeps it nil
	// and every entry point short-circuits. Guarded by c.mu.
	health *healthState

	// defaults holds the plugin-level (default) capacity / escalation /
	// learning / health sections. A provider's own CooldownPolicy section
	// replaces the matching default wholesale; nil or a section that is not
	// configured falls back to these defaults.
	defaults capabilityDefaults

	// capacity, when non-nil, is the DEFAULT capacity gate built from
	// defaults.capacity (see capacity.go). Providers whose effective capacity
	// spec differs get their own gate from capacityRegistry. Stored in an
	// atomic pointer so a reconfiguration can swap it without a lock on the
	// read path.
	capacity              atomic.Pointer[capacityGate]
	defaultCapacitySpec   capacitySpec
	defaultCapacityActive bool
	capacityRegistry      sync.Map // schemas.ModelProvider -> *capacityHolder

	// learning, when non-nil, is the DEFAULT limit learner built from
	// defaults.learning (see learn.go). Providers whose effective learning
	// spec differs get their own learner from learningRegistry.
	learning              atomic.Pointer[limitLearner]
	defaultLearningSpec   learningSpec
	defaultLearningActive bool
	learningRegistry      sync.Map // schemas.ModelProvider -> *learnerHolder

	// now is the time source used by the state. Nil means time.Now. Tests
	// inject a fake clock here; production never mutates it. It must not be
	// swapped while the state is in concurrent use.
	now func() time.Time

	// randFloat supplies the [0, 1) draw used to jitter the chronic long
	// backoff. Nil means "use the state-owned rng below". Tests inject a fixed
	// value to pin exact TTLs. Both fields are only touched by the mark path,
	// which holds c.mu (write), so they need no separate lock and are never
	// read from a concurrent reader.
	randFloat func() float64
	rng       *rand.Rand

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

	// perProviderModel is keyed by "provider::model"; only (provider, model)
	// pairs that have experienced at least one model-granularity (scope=model)
	// classified mark/suppressed event appear. Guarded by c.mu (RWMutex).
	perProviderModel map[string]*ProviderKindCounters

	// perProviderScopeKey splits the same per-provider counters by mark
	// scope, so the UI can surface "key-scope 标记 / 抑制" alongside the
	// aggregated total in perProvider. Without this split, the
	// per-provider total includes both scope=key and scope=model hits and
	// there is no way to attribute the difference between the total and
	// the per-model breakdown — a real source of confusion in mixed-scope
	// policies. Only providers that have experienced at least one
	// classified event under scope=key appear. Guarded by c.mu (RWMutex).
	perProviderScopeKey map[schemas.ModelProvider]*ProviderKindCounters

	// perProviderScopeModel is the model-scope mirror of
	// perProviderScopeKey: same shape, same atomic accounting rules, but
	// only fires when a mark is recorded with scope=model and a non-empty
	// model. The sum of perProviderScopeKey[provider] and
	// perProviderScopeModel[provider] equals perProvider[provider] for
	// every provider (modulo unclassified legacy marks, which contribute
	// to neither sub-bucket). Guarded by c.mu (RWMutex).
	perProviderScopeModel map[schemas.ModelProvider]*ProviderKindCounters

	// perKey holds the per-(provider, key_id) observability counters (mark /
	// suppressed / success plus their last-seen timestamps). It is a pure
	// observation layer: nothing reads it to make a routing decision, it only
	// reports what the existing mark / suppress / success paths already did.
	// Guarded by c.mu (RWMutex); pruned by the same GC tick that prunes
	// the escalation tables (see cleanupPerKeyStatsLocked).
	perKey map[perKeyID]*perKeyEntry
}

// NewCooldownState returns a CooldownState with the given default TTL.
// A non-positive TTL falls back to DefaultCooldownTTL.
func NewCooldownState(ttl time.Duration) *CooldownState {
	if ttl <= 0 {
		ttl = DefaultCooldownTTL
	}
	return &CooldownState{
		cooldowns:             make(map[string]CooldownRecord),
		ttl:                   ttl,
		ttlOverrides:          make(map[schemas.ModelProvider]time.Duration),
		perProvider:           make(map[schemas.ModelProvider]*ProviderKindCounters),
		perProviderModel:      make(map[string]*ProviderKindCounters),
		perProviderScopeKey:   make(map[schemas.ModelProvider]*ProviderKindCounters),
		perProviderScopeModel: make(map[schemas.ModelProvider]*ProviderKindCounters),
		perKey:                make(map[perKeyID]*perKeyEntry),
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

// nowTime returns the state's current time, defaulting to time.Now. Reading
// the now field is safe as long as it is never swapped while the state is in
// concurrent use (tests install a fake clock before that point).
func (c *CooldownState) nowTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// key builds the storage key for a key-granularity cooldown: a trailing "::"
// and empty model distinguish it from a model-granularity entry when Snapshot
// parses the map (last "::" split → model; first "::" in the head → provider
// / keyID). The state is process-local and wiped on restart, so the format
// carries no migration burden.
func (c *CooldownState) key(provider schemas.ModelProvider, keyID string) string {
	return string(provider) + "::" + keyID + "::"
}

// keyModel builds the storage key for a model-granularity cooldown. Keyless
// providers (schemas.IsKeylessProvider) have no keyID — their model-scoped
// key collapses to "<provider>::<model>" per convention, keeping the empty
// keyID sentinel unambiguous from the key-granularity "<provider>::".
func (c *CooldownState) keyModel(provider schemas.ModelProvider, keyID string, model string) string {
	if schemas.IsKeylessProvider(provider) {
		return string(provider) + "::" + model
	}
	return string(provider) + "::" + keyID + "::" + model
}

// markKey resolves the storage key for a (provider, keyID, model) mark under
// the given scope.
func (c *CooldownState) markKey(provider schemas.ModelProvider, keyID string, model string, scope schemas.CooldownPolicyScope) string {
	if scope.OrDefault() == schemas.CooldownScopeModel {
		return c.keyModel(provider, keyID, model)
	}
	return c.key(provider, keyID)
}

// parseCooldownKey reverses markKey for Snapshot: splits "<provider>::<keyID>
// ::<model>" on the LAST "::" to separate the model, then the FIRST "::" of
// the head to separate provider from keyID. Keyless providers (whose keyID is
// empty) produce a head with no separator — mapped to keyID="".
func parseCooldownKey(k string) (provider string, keyID string, model string) {
	last := strings.LastIndex(k, "::")
	if last < 0 {
		return "", "", ""
	}
	head, model := k[:last], k[last+2:]
	first := strings.Index(head, "::")
	if first < 0 {
		// no provider::keyID separator → keyless provider entry
		return head, "", model
	}
	return head[:first], head[first+2:], model
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
// suppressed at key granularity. Expired entries are pruned lazily on access.
// For model-granularity queries use IsCoolingDownForModel.
func (c *CooldownState) IsCoolingDown(provider schemas.ModelProvider, keyID string) bool {
	_, _, ok := c.lookupCooldown(provider, keyID, "", schemas.CooldownScopeKey)
	return ok
}

// IsCoolingDownForModel reports whether the given (provider, keyID, model)
// triple is currently suppressed at model granularity. Expired entries are
// pruned lazily on access.
func (c *CooldownState) IsCoolingDownForModel(provider schemas.ModelProvider, keyID string, model string) bool {
	_, _, ok := c.lookupCooldown(provider, keyID, model, schemas.CooldownScopeModel)
	return ok
}

// lookupCooldown is the internal accessor used by both IsCoolingDown (for the
// boolean answer) and AsFilter (which also needs the CooldownKind so it can
// attribute each suppression to the right by_kind / per_provider bucket).
// Returns (expiresAt, kind, true) when the entry exists AND is not expired.
// Expired entries are pruned lazily on access. Callers MUST treat the bool as
// authoritative — the time/kind values are only meaningful when true.
//
// scope controls which storage key format to query: CooldownScopeKey checks
// the "<provider>::<keyID>::" entry; CooldownScopeModel checks the
// "<provider>::<keyID>::<model>" entry.
//
// For keyless providers (see schemas.IsKeylessProvider, e.g. bare `opencode`)
// the empty keyID is a legal sentinel: bifrost's GetKeysForProvider returns a
// single []schemas.Key{{}} for keyless providers and the retry loop routes
// every attempt through that single sentinel key. Empty keyID on a non-keyless
// provider is still treated as a no-op so a caller bug cannot silently mark
// "the whole provider" by accident.
//
// A model-granularity query with an empty model returns false immediately
// (the filter never consults the model granularity when the runtime model is
// empty, so the lookup is trivially "not cooling").
func (c *CooldownState) lookupCooldown(provider schemas.ModelProvider, keyID string, model string, scope schemas.CooldownPolicyScope) (time.Time, CooldownKind, bool) {
	if keyID == "" && !schemas.IsKeylessProvider(provider) {
		return time.Time{}, "", false
	}
	if scope.OrDefault() == schemas.CooldownScopeModel && model == "" {
		return time.Time{}, "", false
	}
	k := c.markKey(provider, keyID, model, scope)
	now := c.nowTime()
	c.mu.RLock()
	rec, ok := c.cooldowns[k]
	c.mu.RUnlock()
	if !ok {
		return time.Time{}, "", false
	}
	if !now.Before(rec.expiresAt) {
		// lazy prune
		c.mu.Lock()
		if cur, ok := c.cooldowns[k]; ok && !c.nowTime().Before(cur.expiresAt) {
			delete(c.cooldowns, k)
		}
		c.mu.Unlock()
		return time.Time{}, "", false
	}
	return rec.expiresAt, rec.kind, true
}

// lookupSuppressed checks both the key-granularity and (when the runtime
// model is non-empty) the model-granularity entries for a key. A key is
// suppressed if EITHER form has an active cooldown:
//
//   - scope=key marks match every request for that key across all models.
//   - scope=model marks match only requests whose model equals the marked one.
//
// The key-granularity check runs first so its kind wins when both forms are
// present (a key-level quota mark is broader than a model-level one and takes
// precedence in the stats attribution).
func (c *CooldownState) lookupSuppressed(provider schemas.ModelProvider, keyID string, model string) (time.Time, CooldownKind, schemas.CooldownPolicyScope, bool) {
	if t, k, ok := c.lookupCooldown(provider, keyID, "", schemas.CooldownScopeKey); ok {
		return t, k, schemas.CooldownScopeKey, true
	}
	if model == "" {
		return time.Time{}, "", schemas.CooldownScopeKey, false
	}
	t, k, ok := c.lookupCooldown(provider, keyID, model, schemas.CooldownScopeModel)
	if ok {
		return t, k, schemas.CooldownScopeModel, true
	}
	return time.Time{}, "", schemas.CooldownScopeKey, false
}

// ShortestRemainingCooldown returns the shortest remaining cooldown across the
// given keys for (provider, model) — the earliest moment any of them could
// become eligible again. Returns 0 when none of the keys is currently
// suppressed. Used to derive the Retry-After hint on the synthetic 429
// "no_eligible_keys" response so clients know when to retry.
func (c *CooldownState) ShortestRemainingCooldown(provider schemas.ModelProvider, model string, keys []schemas.Key) time.Duration {
	now := c.nowTime()
	var shortest time.Duration
	found := false
	for _, k := range keys {
		expiresAt, _, _, suppressed := c.lookupSuppressed(provider, k.ID, model)
		if !suppressed {
			continue
		}
		remaining := expiresAt.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		if !found || remaining < shortest {
			shortest = remaining
			found = true
		}
	}
	if !found {
		return 0
	}
	return shortest
}

// retryAfterSecondsHint rounds a remaining cooldown up to a whole number of
// seconds (minimum 1) for the Retry-After hint / ctx stamp.
func retryAfterSecondsHint(remaining time.Duration) int64 {
	if remaining <= 0 {
		return 0
	}
	secs := int64(remaining / time.Second)
	if remaining%time.Second != 0 {
		secs++
	}
	if secs < 1 {
		secs = 1
	}
	return secs
}

// Mark records a cooldown for the given (provider, keyID) with no classified
// kind at key granularity. A no-op when keyID is empty AND the provider is not
// a known keyless provider — for keyless providers (e.g. bare `opencode`), the
// empty keyID is a legal sentinel representing the whole provider, so we mark
// at the provider level there.
//
// Unclassified marks still bump the lifetime markCount counter for backward
// compatibility with the original single-counter API, but do NOT contribute
// to by_kind or per_provider stats. Callers that have a classified reason
// (policy hit) should use MarkWithTTL(..., kind, scope) instead.
func (c *CooldownState) Mark(provider schemas.ModelProvider, keyID string) {
	c.MarkWithTTL(provider, keyID, "", 0, "", schemas.CooldownScopeKey)
}

// MarkWithTTL records a cooldown for (provider, keyID, model) using ttl as the
// cooldown duration, attributing the mark to the given kind (rate_limit or
// quota) and scope ("key" or "model"). A ttl <= 0 falls back to the provider's
// effective TTL (the configured default or per-provider override), preserving
// the behaviour of Mark.
//
// This is the policy-less entry point: escalation uses the plugin default. The
// plugin's own paths (the per-key marker and PostLLMHook) call markWithPolicy
// with the provider's stamped CooldownPolicy instead, so a provider can carry
// its own escalation section.
//
// For keyless providers (schemas.IsKeylessProvider, e.g. bare `opencode`),
// an empty keyID is the legal sentinel key returned by bifrost's
// GetKeysForProvider; the mark is then recorded against the whole provider
// (the "<provider>::" map entry). On any other provider, keyID empty remains
// a no-op so a caller bug cannot accidentally cool "the whole provider".
//
// kind="rate_limit" / "quota" also bumps by_kind and per_provider counters.
// kind="" only bumps the legacy markCount counter.
//
// A model-granularity mark (scope="model") requires a non-empty model to key
// on — without one an entry would be recorded that the filter can never match
// (the filter only consults the model granularity when the runtime model is
// non-empty), so the mark is skipped rather than recorded as a dead entry.
func (c *CooldownState) MarkWithTTL(provider schemas.ModelProvider, keyID string, model string, ttl time.Duration, kind CooldownKind, scope schemas.CooldownPolicyScope) {
	c.markWithPolicy(nil, provider, keyID, model, ttl, kind, scope)
}

// markWithPolicy is MarkWithTTL with an explicit per-provider policy. A nil
// policy means "use the plugin defaults".
func (c *CooldownState) markWithPolicy(policy *schemas.CooldownPolicy, provider schemas.ModelProvider, keyID string, model string, ttl time.Duration, kind CooldownKind, scope schemas.CooldownPolicyScope) {
	if c == nil {
		return
	}
	if keyID == "" && !schemas.IsKeylessProvider(provider) {
		return
	}
	if scope.OrDefault() == schemas.CooldownScopeModel && model == "" {
		return
	}
	// Resolve the effective escalation section before taking the lock. The
	// registry/defaults are immutable after construction, so this is a couple
	// of pointer reads; when escalation is off for this provider the mark path
	// stays byte-for-byte the pre-escalation behaviour.
	maxMultiplier, escalate := c.defaults.escalationMaxMultiplier(policy)
	// Chronic long-backoff is a SUB-section of escalation: chronicActive is
	// false unless the effective escalation section is enabled AND its chronic
	// block is enabled. Resolving it here keeps the disabled case at two
	// pointer reads with no lock and no clock read.
	chronic, chronicActive := c.defaults.chronicSpec(policy)

	c.mu.Lock()
	effective := ttl
	if effective <= 0 {
		effective = c.effectiveTTLLocked(provider)
	}
	now := c.nowTime()
	// Escalation is applied after the base TTL (including any retry_after_mode
	// resolution) has been computed, so the two features compose: the mode
	// decides the base, then the multiplier stretches it, then the 1h ceiling
	// bounds it. With escalation disabled for this provider this branch is
	// skipped entirely.
	if escalate {
		es := c.ensureEscalationLocked()
		// The storm signal is judged with the persistent-failure semantics
		// only when the chronic section is active for this provider: with
		// chronic off no key is chronically unhealthy, so the signal keeps its
		// historical meaning (and the deployment stays byte-for-byte
		// identical to the pre-B4 behaviour). The policy is a plain value, so
		// passing it down the mark path allocates nothing.
		storm := stormSignalPolicy{persistence: chronicActive, state: c, spec: chronic}
		effective = es.applyLocked(provider, keyID, model, scope, effective, maxMultiplier, now, storm)
	}
	// Chronic backoff runs AFTER escalation on purpose: applyLocked's counter
	// advance and storm-window recording are the side effects that keep the
	// escalation state self-consistent, so a chronic key keeps counting its
	// failures. When the chronic latch is released (after recovery_successes
	// consecutive successes) the success path clears those counters, so the next
	// failure starts a fresh, non-stale count. The TTL returned by applyLocked
	// is only a floor here: the chronic value replaces it when it is longer.
	if chronicActive {
		effective = c.applyChronicTTLLocked(provider, keyID, model, chronic, effective, now)
	}
	c.cooldowns[c.markKey(provider, keyID, model, scope)] = CooldownRecord{
		expiresAt: now.Add(effective),
		kind:      kind,
	}
	// Per-key observation: record the mark against the key it belongs to so
	// the stats endpoint can attribute cooldowns per credential instead of
	// only per provider. Purely additive — it changes no decision here.
	c.recordPerKeyMarkLocked(provider, keyID, now)
	idx := kindIndex(kind)
	c.markByKind[idx].Add(1)
	if kind == CooldownKindRateLimit {
		c.perProviderMarkLocked(provider).RateLimit.MarkCount++
	} else if kind == CooldownKindQuota {
		c.perProviderMarkLocked(provider).Quota.MarkCount++
	}
	if scope.OrDefault() == schemas.CooldownScopeModel && model != "" && kind != "" {
		if kind == CooldownKindRateLimit {
			c.perProviderModelMarkLocked(provider, model).RateLimit.MarkCount++
		} else if kind == CooldownKindQuota {
			c.perProviderModelMarkLocked(provider, model).Quota.MarkCount++
		}
		if kind == CooldownKindRateLimit {
			c.perProviderScopeMarkLocked(provider, schemas.CooldownScopeModel).RateLimit.MarkCount++
		} else if kind == CooldownKindQuota {
			c.perProviderScopeMarkLocked(provider, schemas.CooldownScopeModel).Quota.MarkCount++
		}
	} else if kind != "" {
		if kind == CooldownKindRateLimit {
			c.perProviderScopeMarkLocked(provider, schemas.CooldownScopeKey).RateLimit.MarkCount++
		} else if kind == CooldownKindQuota {
			c.perProviderScopeMarkLocked(provider, schemas.CooldownScopeKey).Quota.MarkCount++
		}
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

// perProviderModelMarkLocked returns the per-(provider, model) counter struct
// for the given (provider, model), creating it on demand. The caller MUST
// hold c.mu (write). The model must be non-empty (caller is responsible for
// checking — this is guaranteed by MarkWithTTL's scope=model guard).
func (c *CooldownState) perProviderModelMarkLocked(provider schemas.ModelProvider, model string) *ProviderKindCounters {
	key := string(provider) + "::" + model
	pc, ok := c.perProviderModel[key]
	if !ok {
		pc = &ProviderKindCounters{}
		c.perProviderModel[key] = pc
	}
	return pc
}

// perProviderScopeMarkLocked returns the per-(provider, scope) counter struct
// for the given (provider, scope), creating it on demand. The caller MUST
// hold c.mu (write). This split lets the UI attribute per-provider totals
// to "key-scope hits" vs "model-scope hits" without losing information —
// perProviderScopeKey + perProviderScopeModel == perProvider for every
// provider (modulo unclassified legacy marks, which contribute to neither
// sub-bucket because they have no kind).
func (c *CooldownState) perProviderScopeMarkLocked(provider schemas.ModelProvider, scope schemas.CooldownPolicyScope) *ProviderKindCounters {
	var bucket map[schemas.ModelProvider]*ProviderKindCounters
	switch scope.OrDefault() {
	case schemas.CooldownScopeModel:
		bucket = c.perProviderScopeModel
	default:
		bucket = c.perProviderScopeKey
	}
	pc, ok := bucket[provider]
	if !ok {
		pc = &ProviderKindCounters{}
		bucket[provider] = pc
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
	Model     string                `json:"model,omitempty"`
	ExpiresAt time.Time             `json:"expires_at"`
	Remaining time.Duration         `json:"remaining"`
	Kind      CooldownKind          `json:"kind,omitempty"`
}

// Snapshot returns a copy of every active (non-expired) cooldown entry,
// suitable for surfacing through a management API. Expired entries are
// pruned lazily as they are encountered; callers wanting an exact snapshot
// may want to call this twice and trust the second result.
func (c *CooldownState) Snapshot() []CooldownEntry {
	now := c.nowTime()
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]CooldownEntry, 0, len(c.cooldowns))
	for k, rec := range c.cooldowns {
		if !now.Before(rec.expiresAt) {
			continue
		}
		prov, keyID, model := parseCooldownKey(k)
		if prov == "" {
			continue
		}
		out = append(out, CooldownEntry{
			Provider:  schemas.ModelProvider(prov),
			KeyID:     keyID,
			Model:     model,
			ExpiresAt: rec.expiresAt,
			Remaining: rec.expiresAt.Sub(now),
			Kind:      rec.kind,
		})
	}
	return out
}

// ClearKey removes any cooldown on the given (provider, keyID), or — when
// model is non-empty — on the (provider, keyID, model) triple. Returns true
// if an entry was removed. Calling on an unknown key or empty keyID is a
// no-op (returns false). Useful when an operator manually un-cools a key
// after fixing the underlying issue (e.g. topping up quota).
//
// The empty keyID is rejected even for keyless providers because there is no
// meaningful UI/operator path that targets the synthetic "<provider>::"
// sentinel — if an operator wants to lift a provider-level cooldown they can
// wait for it to expire. Mark/lookup on the same sentinel are intentionally
// permitted (see MarkWithTTL) so the cooldown can actually take hold.
func (c *CooldownState) ClearKey(provider schemas.ModelProvider, keyID string, model string) bool {
	if keyID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	k := c.key(provider, keyID)
	if model != "" {
		k = c.keyModel(provider, keyID, model)
	}
	if _, ok := c.cooldowns[k]; !ok {
		return false
	}
	delete(c.cooldowns, k)
	// A manual unfreeze means the operator considers the key healthy again, so
	// its escalation counter (and storm-window membership) is reset too —
	// otherwise the next single failure would immediately jump to 2x/4x.
	if c.escalation != nil {
		clearModel := ""
		if model != "" {
			clearModel = model
		}
		c.clearEscalationLocked(provider, keyID, clearModel)
	}
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
//   - PerProviderModel is the same counters broken down per (provider, model)
//     for model-granularity (scope=model) events. Keyed by "provider::model".
//     Only (provider, model) pairs that have experienced at least one
//     model-granularity event appear.
//   - PerProviderScopeKey / PerProviderScopeModel split the same per-provider
//     totals by mark scope so the UI can attribute the difference between
//     the aggregated per-provider total and the per-model breakdown (e.g.
//     "rate_limit 总 932, 但按模型细分 deepseek-v4-flash 是 0, 那剩下的 932
//     全来自 key-scope 命中"). For every provider the two sub-buckets sum
//     to PerProvider (modulo unclassified legacy marks, which contribute to
//     neither sub-bucket because they have no kind).
//   - PerKey is the same observation broken down per (provider, key_id),
//     keyed by "<provider>::<keyID>". It carries the mark / suppressed /
//     success counters plus the remaining time on the key's longest active
//     cooldown, so an operator can tell "which key is paying for it"
//     instead of only "which provider".
//   - ChronicKeyCount is the number of distinct (provider, key_id) pairs with
//     at least one health row currently latched in chronic long-backoff. It is
//     zero unless escalation.chronic is enabled for some provider.
type CooldownStats struct {
	MarkCount             uint64                                         `json:"mark_count"`
	SuppressedCount       uint64                                         `json:"suppressed_count"`
	CurrentActiveCount    int                                            `json:"current_active_count"`
	ChronicKeyCount       int                                            `json:"chronic_key_count"`
	ByKind                ByKindCounters                                 `json:"by_kind"`
	PerProvider           map[schemas.ModelProvider]ProviderKindCounters `json:"per_provider"`
	PerProviderModel      map[string]ProviderKindCounters                `json:"per_provider_model,omitempty"`
	PerProviderScopeKey   map[schemas.ModelProvider]ProviderKindCounters `json:"per_provider_scope_key,omitempty"`
	PerProviderScopeModel map[schemas.ModelProvider]ProviderKindCounters `json:"per_provider_scope_model,omitempty"`
	PerKey                map[string]PerKeyStats                         `json:"per_key,omitempty"`
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
	// The per-key rows carry the remaining time of the key's longest active
	// cooldown. That value is taken from Snapshot() — the same source the
	// GET /state endpoint serves — so the two views can never disagree.
	// Snapshot() takes its own read lock, so it has to run BEFORE this
	// method takes c.mu: sync.RWMutex is not reentrant.
	remaining := perKeyRemainingByKey(c.Snapshot())
	c.mu.RLock()
	stats.CurrentActiveCount = len(c.cooldowns)
	stats.PerProvider = make(map[schemas.ModelProvider]ProviderKindCounters, len(c.perProvider))
	for p, pc := range c.perProvider {
		stats.PerProvider[p] = *pc
	}
	stats.PerProviderModel = make(map[string]ProviderKindCounters, len(c.perProviderModel))
	for mk, mc := range c.perProviderModel {
		stats.PerProviderModel[mk] = *mc
	}
	stats.PerProviderScopeKey = make(map[schemas.ModelProvider]ProviderKindCounters, len(c.perProviderScopeKey))
	for p, pc := range c.perProviderScopeKey {
		stats.PerProviderScopeKey[p] = *pc
	}
	stats.PerProviderScopeModel = make(map[schemas.ModelProvider]ProviderKindCounters, len(c.perProviderScopeModel))
	for p, pc := range c.perProviderScopeModel {
		stats.PerProviderScopeModel[p] = *pc
	}
	stats.PerKey = c.perKeyStatsSnapshot(remaining)
	stats.ChronicKeyCount = c.chronicKeyCountLocked()
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
			now := c.nowTime()
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
			if c.escalation != nil {
				c.escalation.cleanupLocked(now)
			}
			// The per-key observability rows are pruned on the same tick as
			// the escalation tables, so every in-memory table of this state
			// shares one bounded cadence.
			c.cleanupPerKeyStatsLocked(now)
			c.cleanupHealthLocked(now)
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
// with each other for the same provider. The per-key observation counter is
// bumped in the same critical section, so a suppression still costs exactly
// one short write lock.
//
// Optional health-aware weighting (see health.go): when the effective health
// section for the provider is enabled, each surviving key's Weight is scaled by
// its EWMA success rate (floored at min_weight_factor) on the returned copy.
// When disabled — the default — the returned keys carry their configured
// Weight unchanged.
func (c *CooldownState) AsFilter(logger schemas.Logger) schemas.KeyPoolFilter {
	return func(ctx *schemas.BifrostContext, provider schemas.ModelProvider, model string, keys []schemas.Key) ([]schemas.Key, error) {
		if len(keys) == 0 {
			return keys, nil
		}
		out := make([]schemas.Key, 0, len(keys))
		// The per-provider policy is resolved once per filter call; both the
		// health section and the capacity gate read it.
		policy := policyFromContext(ctx)
		// Health-aware weighting is resolved once per filter call. An inert
		// configuration (nil or disabled for this provider) leaves healthActive
		// false and the whole subsystem is skipped, keeping the disabled path
		// at the cost of two pointer reads.
		healthSpec, healthActive := healthSpecOf(c.defaults.resolveHealth(policy))
		// Per-key capacity gate (scope=key only). For every other scope — and
		// for the default, fully disabled deployment — this resolves to nil, so
		// the filter behaves exactly as before. Per-key suppression happens
		// HERE rather than in PreProviderHook because only the filter knows
		// which keys the worker is about to choose from: removing one key's
		// exhausted budget leaves every other key selectable.
		capacityGate := c.capacityGateForFilter(provider, policy)
		// now is resolved lazily: the happy path (nothing suppressed) must not
		// pay for a clock read, and one read covers every vetoed key in this
		// call.
		var now time.Time
		nowResolved := false
		for _, k := range keys {
			_, kind, matchedScope, suppressed := c.lookupSuppressed(provider, k.ID, model)
			if !suppressed {
				if capacityGate != nil && capacityGate.check(provider, k.ID, model) != nil {
					// This credential's own window is over budget. Rotate past
					// it instead of refusing the request: the only request that
					// has to be refused is the one no key can serve (handled in
					// PreProviderHook). No cooldown counter is touched — this is
					// a budget decision, not a cooldown suppression.
					if logger != nil {
						logger.Info("[provider-cooldown] capacity gate excluded key %s/%s (name=%s, model=%s, scope=key)", provider, k.ID, k.Name, model)
					}
					continue
				}
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
			}
			c.mu.Lock()
			if kind != "" {
				switch kind {
				case CooldownKindRateLimit:
					c.perProviderMarkLocked(provider).RateLimit.SuppressedCount++
				case CooldownKindQuota:
					c.perProviderMarkLocked(provider).Quota.SuppressedCount++
				}
				// When the suppression came from a model-granularity match,
				// also attribute it to the per-(provider, model) bucket.
				if matchedScope == schemas.CooldownScopeModel && model != "" {
					switch kind {
					case CooldownKindRateLimit:
						c.perProviderModelMarkLocked(provider, model).RateLimit.SuppressedCount++
					case CooldownKindQuota:
						c.perProviderModelMarkLocked(provider, model).Quota.SuppressedCount++
					}
					switch kind {
					case CooldownKindRateLimit:
						c.perProviderScopeMarkLocked(provider, schemas.CooldownScopeModel).RateLimit.SuppressedCount++
					case CooldownKindQuota:
						c.perProviderScopeMarkLocked(provider, schemas.CooldownScopeModel).Quota.SuppressedCount++
					}
				} else {
					switch kind {
					case CooldownKindRateLimit:
						c.perProviderScopeMarkLocked(provider, schemas.CooldownScopeKey).RateLimit.SuppressedCount++
					case CooldownKindQuota:
						c.perProviderScopeMarkLocked(provider, schemas.CooldownScopeKey).Quota.SuppressedCount++
					}
				}
			}
			// Per-key observation: the vetoed key itself gets the credit, so
			// the stats endpoint can show which credential the filter keeps
			// taking out of the rotation. Like the legacy suppressedCount
			// above, this flat counter also covers unclassified marks.
			if !nowResolved {
				now = c.nowTime()
				nowResolved = true
			}
			c.recordPerKeySuppressedLocked(provider, k.ID, now)
			c.mu.Unlock()
			if logger != nil {
				logger.Info("[provider-cooldown] suppressed key %s/%s (name=%s, model=%s, kind=%s)", provider, k.ID, k.Name, model, kind)
			}
		}
		// When every key was vetoed, stamp the shortest remaining cooldown on
		// the context so core's executeRequestWithRetries can attach the
		// Retry-After hint to the synthetic 429 no_eligible_keys response.
		// Guard ctx: the filter may be invoked with a nil context by callers
		// that only need the key filtering behaviour (e.g. tests).
		if len(out) == 0 && ctx != nil {
			if rem := c.ShortestRemainingCooldown(provider, model, keys); rem > 0 {
				ctx.SetValue(schemas.BifrostContextKeyRetryAfterSeconds, retryAfterSecondsHint(rem))
			}
		}
		// Health-aware weighting: scale every live key's Weight by its EWMA
		// success rate so a chronically failing credential receives a smaller
		// (never zero) share of the traffic. Suppressed keys are already out of
		// `out`, so they are not weighted. This only edits the copies `out`
		// already holds — the caller's slice is never mutated.
		if healthActive {
			c.applyHealthWeights(provider, model, out, healthSpec)
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
		// Empty keyID is the legal sentinel for keyless providers (see
		// MarkWithTTL); for those we still want to mark the provider as a
		// whole. For non-keyless providers an empty keyID means the retry
		// loop could not identify the failing key — skip rather than guess.
		if c == nil || bifrostErr == nil {
			return
		}
		if keyID == "" && !schemas.IsKeylessProvider(provider) {
			return
		}
		// Reuse classify() — single source of truth for rate_limit vs quota
		// matching against the per-provider CooldownPolicy. classify reads
		// the policy from ctx[BifrostContextKeyCooldownPolicy] and falls
		// back to DefaultCooldownPolicy(provider) when the stamp is absent.
		ttl, _, _, kind, scope, ok := classifyForMarker(ctx, provider, keyID, keyName, bifrostErr)
		if !ok {
			return
		}
		// Resolve the policy once and use it for both the mark (so the
		// provider's own escalation section applies) and the learning sample
		// (so the provider's own learning window applies).
		policy := policyFromContext(ctx)
		c.markWithPolicy(policy, provider, keyID, model, ttl, kind, scope)
		// Health signal (optional, off by default): a policy-matched rejection
		// is evidence that this credential is unhealthy at selection time.
		// Only rate_limit / quota matches (the ok==true branch above) are
		// recorded — an unrelated error would say nothing about a provider's
		// throughput budget and must not depress the key's weight.
		c.observe(policy, provider, keyID, model, false)
		// Optional read-only learning (off by default): a rate-limit
		// rejection brackets the provider's effective per-window token
		// limit. Quota-classified rejections are deliberately NOT sampled —
		// they are billing/entitlement failures, not a window limit, so
		// they say nothing about the provider's TPM.
		if kind == CooldownKindRateLimit {
			c.observeRateLimit(ctx, policy, provider, model, keyID, bifrostErr)
		}
		if logger != nil {
			logger.Info("[provider-cooldown] marked key %s/%s (name=%s, model=%s, TTL=%v, kind=%s, scope=%s) from per-key-failure marker", provider, keyID, keyName, model, ttl, kind, scope)
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
		escalationEnabled := cfg.Escalation != nil && cfg.Escalation.Enabled
		chronicEnabled := escalationEnabled && cfg.Escalation.Chronic != nil && cfg.Escalation.Chronic.Enabled
		capacityEnabled := cfg.Capacity.Active()
		learningEnabled := cfg.Learning.Active()
		healthEnabled := cfg.Health.Active()
		p.logger.Info("[provider-cooldown] initialized (default_ttl=%v, %d provider override(s), %d custom quota pattern(s), escalation=%t, chronic=%t, capacity=%t, learning=%t, health=%t; providers without their own cooldown_policy section inherit these defaults)",
			p.State.ttl, len(p.State.ttlOverrides), len(p.quotaPatterns), escalationEnabled, chronicEnabled, capacityEnabled, learningEnabled, healthEnabled)
		// The plugin-level capacity section is a deprecated fleet-wide default:
		// one global limit inherited by every provider without an override. Warn
		// once at startup (only when the section is actually present) so
		// operators still relying on it know to move to per-provider config.
		if cfg.Capacity != nil {
			p.logger.Warn("[provider-cooldown] plugin-level capacity config is a deprecated fleet default; configure capacity per provider (provider.cooldown_policy.capacity). It still applies to providers without an override.")
		}
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

// ClearKey removes any cooldown on (provider, keyID) — or (provider, keyID,
// model) when model is non-empty — from the current State. Returns false when
// the plugin has no state or no entry matched.
func (p *CooldownPlugin) ClearKey(provider schemas.ModelProvider, keyID string, model string) bool {
	if p.State == nil {
		return false
	}
	return p.State.ClearKey(provider, keyID, model)
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
// state and short-circuits the attempt with a synthetic 429 "no_eligible_keys"
// BifrostError (carrying a Retry-After hint for the soonest cooldown lapse)
// when every key is in cooldown. The framework stamps
// ctx[BifrostContextKeyProviderKeys] with the per-provider key snapshot before
// calling this hook, so we never have to round-trip through bifrost.account.
//
// Returning a Silent short-circuit asks presentation plugins (logging) to skip
// their log writes for this attempt — the underlying BifrostError still
// propagates to the caller so the fallback chain can proceed (and the cooldown
// timeline updates cleanly). Without this hook, the request would otherwise
// reach the worker queue, have every key vetoed by AsFilter, surface a
// "no_eligible_keys" 429 there, and force a "spurious" status=cancelled log
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

	// Retrospective capacity gate (optional, off by default). Accounting uses
	// real usage only, so this check never estimates tokens or reads the
	// request body. The gate is resolved per provider: a provider's own
	// CooldownPolicy capacity section wins over the plugin default, and the
	// default-off case costs one pointer walk and one nil check.
	//
	// The provider / provider_model scopes decide on (provider[, model])
	// alone, so they are evaluated here, before the key pool is consulted. The
	// per-key scope needs the pool and is evaluated further down, once the
	// keys are known.
	policy := policyFromContext(ctx)
	gate := p.State.gateFor(provider, policy)
	if gate != nil && gate.scope != CapacityScopeKey {
		if sc := p.capacityShortCircuit(ctx, gate, provider, model); sc != nil {
			return req, sc, nil
		}
	}

	providerKeysAny := ctx.Value(schemas.BifrostContextKeyProviderKeys)
	providerKeys, _ := providerKeysAny.(map[schemas.ModelProvider][]schemas.Key)
	keys, ok := providerKeys[provider]
	if !ok || len(keys) == 0 {
		// No keys configured for this provider. For keyless providers
		// (schemas.IsKeylessProvider, e.g. bare `opencode` / OpenCode Free)
		// the framework's stampProviderKeysOnContext (core/bifrost.go:518)
		// skips the entry entirely because GetKeysForProvider returns an
		// empty slice when no keys are configured — but the legal sentinel
		// key for keyless providers is the empty Key struct, the same
		// sentinel getAllSupportedKeys (core/bifrost.go:8816-8824)
		// synthesizes for ListModels. Without this backfill, AsFilter
		// never runs for keyless providers and a mark recorded by the
		// PerKeyFailureMarker / PostLLMHook path (against "<provider>::")
		// would never suppress — the UI would show "配额耗尽 标记"
		// climbing while "配额耗尽 抑制" stayed at 0.
		//
		// Synthesizing a sentinel ONLY for keyless providers is safe:
		// AsFilter's lookupCooldown treats empty keyID as the legal
		// sentinel for keyless providers, and a non-keyless provider with
		// no keys still falls through to the normal pipeline (so the
		// operator-facing "no keys configured" diagnostics stay visible).
		if schemas.IsKeylessProvider(provider) {
			keys = []schemas.Key{{}}
		} else {
			return req, nil, nil
		}
	}

	// Per-key capacity gate (scope=key only). The provider/model scopes were
	// already decided above; this one needs the credential list. It refuses
	// the request only when NO key still has budget — when at least one does,
	// AsFilter below keeps the exhausted keys out of the selection, so a
	// single over-budget credential never blocks the others.
	if gate != nil && gate.scope == CapacityScopeKey {
		if sc := p.capacityShortCircuitPerKey(ctx, gate, provider, model, keys); sc != nil {
			return req, sc, nil
		}
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

	// All keys are in cooldown — synthesize the same 429 that the worker would
	// have produced via AsFilter, but surface it here so logging sees the
	// SilentLog flag and skips the spurious "cancelled" row. The Retry-After
	// hint tells the caller when the soonest of these cooldowns lapses.
	statusCode := 429
	errType := "no_eligible_keys"
	var retryAfter int64
	if rem := p.State.ShortestRemainingCooldown(provider, model, keys); rem > 0 {
		retryAfter = retryAfterSecondsHint(rem)
		ctx.SetValue(schemas.BifrostContextKeyRetryAfterSeconds, retryAfter)
	}
	// Shared message format with core's worker path (core/bifrost.go
	// executeRequestWithRetries) so the wire text is identical no matter
	// which raise site fired — and includes the explicit retry hint opencode
	// matches on.
	message := schemas.NoEligibleKeysMessage(provider, retryAfter)
	if p.logger != nil {
		p.logger.Info("[provider-cooldown] short-circuit on %s/%s — all %d key(s) in cooldown (retry-after %ds)", provider, model, len(keys), retryAfter)
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
			ExtraFields: schemas.BifrostErrorExtraFields{
				RetryAfterSeconds: retryAfter,
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
func (p *CooldownPlugin) classify(ctx *schemas.BifrostContext, bifrostErr *schemas.BifrostError) (ttl time.Duration, keyID string, keyName string, model string, kind CooldownKind, scope schemas.CooldownPolicyScope, ok bool) {
	if p.State == nil || bifrostErr == nil {
		return 0, "", "", "", "", schemas.CooldownScopeKey, false
	}
	provider, keyID, keyName, model := lastAttemptProviderAndKey(ctx, bifrostErr)
	// lastAttemptProviderAndKey returns an empty keyID for non-keyless
	// providers when no trail record carried a key; for keyless providers
	// the empty sentinel is valid and we fall through to classifyForMarker
	// so the policy lookup still runs.
	if keyID == "" && !schemas.IsKeylessProvider(provider) {
		return 0, "", "", "", "", schemas.CooldownScopeKey, false
	}
	ttl, keyID, keyName, kind, scope, ok = classifyForMarker(ctx, provider, keyID, keyName, bifrostErr)
	return
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
func classifyForMarker(ctx *schemas.BifrostContext, provider schemas.ModelProvider, keyID string, keyName string, bifrostErr *schemas.BifrostError) (ttl time.Duration, rKeyID string, rKeyName string, kind CooldownKind, scope schemas.CooldownPolicyScope, ok bool) {
	if bifrostErr == nil {
		return 0, "", "", "", schemas.CooldownScopeKey, false
	}
	// Empty keyID is the legal sentinel for keyless providers (see
	// MarkWithTTL); for those we still want to classify and mark. For
	// non-keyless providers an empty keyID means the caller could not
	// identify the failing key — skip rather than guess.
	if keyID == "" && !schemas.IsKeylessProvider(provider) {
		return 0, "", "", "", schemas.CooldownScopeKey, false
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
		return effectiveQuotaCooldownTTL(policy.Quota, bifrostErr), keyID, keyName, CooldownKindQuota, policy.Quota.ScopeOrDefault(), true
	}
	if policy.RateLimit != nil && policy.RateLimit.MatchesRule(bifrostErr) {
		return effectiveCooldownTTL(policy.RateLimit, bifrostErr), keyID, keyName, CooldownKindRateLimit, policy.RateLimit.ScopeOrDefault(), true
	}
	return 0, keyID, keyName, "", schemas.CooldownScopeKey, false
}

// effectiveCooldownTTL resolves the cooldown duration a fired rule applies,
// optionally honouring the provider's own retry hint captured on the error
// path in bifrostErr.ExtraFields.ProviderResponseHeaders.
//
// Two modes of operation, selected by whether the rule configures a
// dimension-specific TTL:
//
// With RPMTTLSeconds and TPMTTLSeconds both unset (0), this is the pre-B3
// implementation byte-for-byte: RetryAfterModeOrDefault() == "off" (the
// default) returns TTLSeconds verbatim without reading any header, "cap" is
// min(TTLSeconds, hint) and "exact" is the hint (falling back to TTLSeconds).
//
// With either dimension TTL set, the rate-limit dimension derived from the
// error body picks the base: rpm -> RPMTTLSeconds, tpm -> TPMTTLSeconds,
// unknown (or an unset dimension value) -> TTLSeconds. The retry_after_mode is
// then applied relative to that base:
//
//   - "off": a usable provider hint is a LOWER BOUND, effective = max(base,
//     hint). This is a deliberate change from the legacy "ignore hints" rule,
//     because the dimension TTL is a floor derived from how the budget
//     recovers; a provider asking us to wait longer must still be honoured.
//   - "cap": effective = min(base, hint); no usable hint -> base.
//   - "exact": effective = hint; no usable hint -> base.
//
// The hint is always bounded by maxProviderRetryAfterHint inside
// providerRetryAfterHint, so neither branch can park a key for days.
func effectiveCooldownTTL(rule *schemas.CooldownPolicyRule, bifrostErr *schemas.BifrostError) time.Duration {
	return effectiveCooldownTTLInternal(rule, bifrostErr, true)
}

// effectiveQuotaCooldownTTL is the quota-rule variant. rpm_ttl_seconds /
// tpm_ttl_seconds are rate_limit-only (Validate rejects them on a quota rule),
// so the quota path ignores them even if a policy reached the runtime without
// validation. The legacy mode semantics apply unchanged.
func effectiveQuotaCooldownTTL(rule *schemas.CooldownPolicyRule, bifrostErr *schemas.BifrostError) time.Duration {
	return effectiveCooldownTTLInternal(rule, bifrostErr, false)
}

func effectiveCooldownTTLInternal(rule *schemas.CooldownPolicyRule, bifrostErr *schemas.BifrostError, dimensionAware bool) time.Duration {
	if rule == nil {
		return 0
	}
	ttl := time.Duration(rule.TTLSeconds) * time.Second
	rpmTTL := 0
	tpmTTL := 0
	if dimensionAware {
		rpmTTL = rule.RPMTTLSeconds
		tpmTTL = rule.TPMTTLSeconds
	}
	if rpmTTL <= 0 && tpmTTL <= 0 {
		// No dimension override: preserve the historical behaviour exactly,
		// including not reading headers at all in "off" mode.
		mode := rule.RetryAfterModeOrDefault()
		if mode != "cap" && mode != "exact" {
			return ttl
		}
		if bifrostErr == nil {
			return ttl
		}
		hint := providerRetryAfterHint(bifrostErr.ExtraFields.ProviderResponseHeaders)
		if hint <= 0 {
			return ttl
		}
		if mode == "cap" && hint > ttl {
			return ttl
		}
		return hint
	}

	// Dimension-aware base selection. An unset dimension value falls back to
	// ttl_seconds; a dimension whose own TTL is 0 also falls back (per-field
	// opt-in).
	switch limiterDimensionOf(bifrostErr) {
	case dimensionRPM:
		if rpmTTL > 0 {
			ttl = time.Duration(rpmTTL) * time.Second
		}
	case dimensionTPM:
		if tpmTTL > 0 {
			ttl = time.Duration(tpmTTL) * time.Second
		}
	}

	mode := rule.RetryAfterModeOrDefault()
	if bifrostErr == nil {
		return ttl
	}
	hint := providerRetryAfterHint(bifrostErr.ExtraFields.ProviderResponseHeaders)
	if hint <= 0 {
		return ttl
	}
	switch mode {
	case "cap":
		if hint < ttl {
			return hint
		}
		return ttl
	case "exact":
		return hint
	default:
		// "off" with a dimension TTL configured: the hint is a lower bound.
		if hint > ttl {
			return hint
		}
		return ttl
	}
}

func (p *CooldownPlugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	if p.State == nil {
		return resp, bifrostErr, nil
	}
	// Snapshot the state once: Init replaces it on a config reload, and this
	// hook touches the state several times.
	state := p.State
	// The per-provider policy is stamped on the context by core before every
	// attempt. Both capabilities below resolve their effective section from it
	// (own section, else plugin default), so two providers can run different
	// windows / thresholds inside the same state.
	policy := policyFromContext(ctx)
	// Optional read-only learning (off by default). The sample is taken
	// BEFORE the terminal attempt's own usage is accounted, so the measured
	// window consumption is the one that preceded the rejection. Learning
	// disabled for this provider costs a couple of pointer reads.
	if bifrostErr != nil {
		state.observeLearnedRateLimit(ctx, policy, bifrostErr)
	}
	// Capacity + observer accounting runs for every terminal outcome — success
	// included — before the cooldown-only early return below, which
	// deliberately records no cooldown state on success. When neither
	// capability is on for this provider this returns before inspecting the
	// response at all.
	p.recordCapacity(ctx, policy, resp, bifrostErr)
	if bifrostErr == nil {
		// Success: the health observation runs FIRST so the chronic latch and
		// the consecutive-success counter are up to date when the escalation
		// clear below decides whether this success was enough to end a chronic
		// backoff (recovery_successes hysteresis). Both steps are independent of
		// each other otherwise.
		state.observeSuccess(policy, ctx, resp)
		// Then clear this key's escalation counter so a recovered key is not
		// punished for earlier 429s. This deliberately records NO cooldown state
		// — success must stay a no-op for the cooldown map itself. When
		// escalation is disabled the helper returns before reading ctx.
		state.clearEscalationOnSuccess(policy, ctx)
		// Per-key observation: a success is the third leg of the per-key
		// picture (marks / suppressions / successes). It is counted for the
		// key that served the request and changes no cooldown state.
		state.recordPerKeySuccess(ctx, resp)
		return resp, bifrostErr, nil
	}

	// A capacity-gate rejection never reached a provider, so it must never
	// mark a cooldown either — an operator's custom quota pattern could
	// otherwise match the synthetic message and cool a key for a request we
	// refused ourselves.
	if isCapacityGateError(bifrostErr) {
		return resp, bifrostErr, nil
	}

	ttl, keyID, keyName, model, kind, scope, ok := p.classify(ctx, bifrostErr)
	if !ok {
		// Fallback for builds / providers without a per-provider policy stamp:
		// preserve the original quota_patterns behaviour so legacy configs
		// keep working until they migrate to provider.cooldown_policy. The
		// legacy path marks with empty kind (unclassified) so the lifetime
		// markCount still increments for backward compatibility, but the
		// per-kind breakdown stays faithful to CooldownPolicy-driven marks.
		if p.isQuotaExhausted(bifrostErr) {
			provider, fallbackKeyID, fallbackKeyName, _ := lastAttemptProviderAndKey(ctx, bifrostErr)
			// lastAttemptProviderAndKey already widens the "skip empty keyID"
			// rule for keyless providers, so an empty fallbackKeyID only
			// happens when the provider is non-keyless and the trail did not
			// record a usable key.
			if fallbackKeyID == "" && !schemas.IsKeylessProvider(provider) {
				return resp, bifrostErr, nil
			}
			state.markWithPolicy(policy, provider, fallbackKeyID, "", 0, "", schemas.CooldownScopeKey)
			if p.logger != nil {
				p.logger.Info("[provider-cooldown] marked key %s/%s (name=%s, TTL=%v, legacy quota_patterns match)", provider, fallbackKeyID, fallbackKeyName, state.EffectiveTTL(provider))
			}
		}
		return resp, bifrostErr, nil
	}

	provider, _, _, _ := lastAttemptProviderAndKey(ctx, bifrostErr)
	state.markWithPolicy(policy, provider, keyID, model, ttl, kind, scope)
	if p.logger != nil {
		p.logger.Info("[provider-cooldown] marked key %s/%s (name=%s, model=%s, TTL=%v, kind=%s, scope=%s)", provider, keyID, keyName, model, ttl, kind, scope)
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
// recent failed attempt, plus the model the failed attempt was routed on.
// Preference order:
//  1. BifrostError.ExtraFields.RoutingInfo.Provider (set by core right
//     before invoking post-hooks; survives the round trip through plugins).
//  2. BifrostError.ExtraFields.Provider (deprecated, but still populated).
//  3. The last KeyAttemptRecord with a non-success outcome (carries keyID).
//
// Returns an empty keyID when none can be determined — callers MUST treat
// that as "do not mark" rather than "mark the whole provider" UNLESS the
// resolved provider is a known keyless provider (schemas.IsKeylessProvider),
// in which case the empty keyID is the legal sentinel key for that provider
// and is returned as-is so PostLLMHook can mark at the provider level. The
// keyName is only populated when resolved from a KeyAttemptRecord; otherwise
// it is empty.
//
// The model is read from RoutingInfo.Model (the model passed to the failed
// attempt's key), falling back to the deprecated OriginalModelRequested when
// RoutingInfo is empty. This matches the `model` argument the retry loop
// hands the PerKeyFailureMarker, so PostLLMHook and the marker agree on the
// model used to scope model-granularity marks.
func lastAttemptProviderAndKey(ctx *schemas.BifrostContext, bifrostErr *schemas.BifrostError) (schemas.ModelProvider, string, string, string) {
	var provider schemas.ModelProvider
	model := ""
	if bifrostErr != nil {
		if bifrostErr.ExtraFields.RoutingInfo.Provider != "" {
			provider = bifrostErr.ExtraFields.RoutingInfo.Provider
		} else if bifrostErr.ExtraFields.Provider != "" {
			provider = bifrostErr.ExtraFields.Provider
		}
		if bifrostErr.ExtraFields.RoutingInfo.Model != "" {
			model = bifrostErr.ExtraFields.RoutingInfo.Model
		} else {
			model = bifrostErr.ExtraFields.OriginalModelRequested
		}
	}
	if ctx == nil {
		return provider, "", "", model
	}
	trail, ok := ctx.Value(schemas.BifrostContextKeyAttemptTrail).([]schemas.KeyAttemptRecord)
	if !ok {
		return provider, "", "", model
	}
	// Pick the last record. AttemptTrail is append-only and ordered, so
	// iterating in reverse finds the most recent attempt. Empty keyID on the
	// trail is acceptable for keyless providers (their single sentinel key
	// has KeyID=""); for other providers an empty keyID means "no key info
	// captured" so we keep scanning further back.
	for i := len(trail) - 1; i >= 0; i-- {
		rec := trail[i]
		if rec.KeyID == "" && !schemas.IsKeylessProvider(provider) {
			continue
		}
		return provider, rec.KeyID, rec.KeyName, model
	}
	return provider, "", "", model
}
