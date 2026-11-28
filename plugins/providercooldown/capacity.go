package providercooldown

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Defaults / bounds for CooldownCapacityConfig. The values and the
// normalization rules are owned by core/schemas; these aliases keep the
// historical package-level names available while leaving exactly one source
// of truth for the numbers.
const (
	defaultCapacityWindowSeconds = schemas.DefaultCapacityWindowSeconds
	minCapacityWindowSeconds     = schemas.MinCapacityWindowSeconds
	maxCapacityWindowSeconds     = schemas.MaxCapacityWindowSeconds

	defaultCapacityHeadroom = schemas.DefaultCapacityHeadroom
	minCapacityHeadroom     = schemas.MinCapacityHeadroom
	maxCapacityHeadroom     = schemas.MaxCapacityHeadroom

	// CapacityScopeProvider shares one budget across every model of a
	// provider; CapacityScopeProviderModel keeps an independent budget per
	// (provider, model) pair; CapacityScopeKey keeps an independent budget
	// per (provider, key, model) triple.
	CapacityScopeProvider      = schemas.CapacityScopeProvider
	CapacityScopeProviderModel = schemas.CapacityScopeProviderModel
	CapacityScopeKey           = schemas.CapacityScopeKey

	// CapacityActionFallback allows the configured fallback chain to take
	// over once the gate engages; CapacityActionReject refuses the request.
	CapacityActionFallback = schemas.CapacityActionFallback
	CapacityActionReject   = schemas.CapacityActionReject

	// capacityGateErrorType is the error type the gate stamps on its synthetic
	// 429. It is deliberately distinct from "no_eligible_keys": the client
	// must be able to tell "our own budget gate" apart from "every key is
	// cooling down".
	capacityGateErrorType = "rate_limit_exceeded"

	// capacityGateMessagePrefix prefixes the gate's client-visible message.
	// isCapacityGateError uses it to recognise the synthetic error without
	// adding a wire-visible field.
	capacityGateMessagePrefix = "capacity gate: "

	// The configuration layer a resolved budget came from. Reported by the
	// usage snapshot so an operator can tell a shared default apart from a
	// per-key or per-(key, model) override.
	capacityLimitSourceBase     = "base"
	capacityLimitSourceKey      = "key"
	capacityLimitSourceKeyModel = "key_model"
)

// capacityBucket is one second of accounted usage. epoch is the Unix second
// the bucket describes; 0 (or any epoch older than the live window) marks it
// as empty. Buckets are recycled, never reallocated.
type capacityBucket struct {
	epoch    int64
	tokens   int64
	requests int64
}

// capacityWindow is the fixed-length ring of per-second buckets for one
// accounting key. len(buckets) == window_seconds; the bucket for second e
// lives at index e % window_seconds.
type capacityWindow struct {
	buckets []capacityBucket
}

// capacityGate is the retrospective budget gate. It sums real usage over a
// rolling window and refuses *subsequent* requests once the window's usage
// crosses limit*headroom.
//
// Concurrency: the gate has its own mutex rather than sharing
// CooldownState's RWMutex. The cooldown map's lock is held briefly on the
// mark/filter path, but the gate is touched on every terminal request
// (write) and every attempt (read), including per-stream-chunk PostLLMHook
// calls. Keeping the two independent avoids coupling that write-heavy traffic
// to key-pool filtering (AsFilter) latency, and avoids turning the cooldown
// RWMutex into a write-lock bottleneck under load. The gate is nil unless
// capacity.enabled=true and at least one limit is set, so a disabled config
// pays exactly one nil-pointer check per request.
//
// Observer mode: a gate built without any threshold (tokenThresh == 0 and
// requestThresh == 0) can never gate anything — check() only fires when a
// threshold is configured and reached — so the same window accounting can be
// reused purely to MEASURE real usage. The limit learner (see learn.go)
// builds such an observer to learn "how many tokens did we already spend in
// the window when the provider refused", without requiring the capacity gate
// to be enabled.
type capacityGate struct {
	// Immutable after construction.
	tokenLimit    int64
	requestLimit  int64
	tokenThresh   int64 // token usage at which the gate engages, 0 when unlimited
	requestThresh int64 // request count at which the gate engages, 0 when unlimited
	windowSeconds int64
	scope         string
	action        string
	headroom      float64

	// keyLimits overrides the provider-level thresholds for individual
	// credentials, keyed by the key's NAME (the stable, user-authored
	// identifier written in config.json). Empty unless Scope is "key".
	keyLimits map[string]capacityKeyLimits

	// keyModelLimits narrows a credential's budget to individual models:
	// key name → model → thresholds. It is the most specific layer, so it wins
	// over keyLimits (which wins over the provider-level scalars). Empty unless
	// Scope is "key" and at least one key declares per-model overrides.
	keyModelLimits map[string]map[string]capacityKeyLimits

	// keyNames maps a credential ID — what the accounting keys and the usage
	// snapshot carry — to its name, learned from the key pool on the check path
	// so the snapshot can report the override and a readable label. Guarded by
	// mu; a rename self-heals on the next check.
	keyNames map[string]string

	// now is the clock, shared with the owning CooldownState so tests inject
	// one fake clock for both features. It is a method value, so a clock
	// installed after the gate was built is still honoured.
	now func() time.Time

	mu   sync.Mutex
	keys map[string]*capacityWindow

	// lastSweepSecond is the Unix second of the last opportunistic sweep of
	// expired keys. Sweeping at most once per window keeps the key map
	// bounded by the number of provider/model pairs with live traffic
	// without needing a background goroutine.
	lastSweepSecond int64
}

// capacityKeyLimits is one credential's resolved budget: the raw limits (for
// reporting and for change detection) plus the thresholds the gate compares
// against, both already normalized with the provider's headroom.
type capacityKeyLimits struct {
	tokenLimit    int64
	requestLimit  int64
	tokenThresh   int64 // 0 when unlimited
	requestThresh int64 // 0 when unlimited
}

// capacityVerdict is returned by check when the gate refuses a request.
type capacityVerdict struct {
	usedTokens        int64
	usedRequests      int64
	retryAfterSeconds int64
	// tokenLimit / requestLimit are the effective budgets that produced this
	// verdict (the provider-level scalars, or a credential's override). They
	// are only used for logging and diagnostics.
	tokenLimit   int64
	requestLimit int64
}

// newCapacityGateFromSpec builds a gate from an already-normalized spec. The
// only constructor: callers go through capacitySpecOf so the same
// normalization is applied everywhere.
//
// keyLimits is the raw per-credential override map from the config (nil in the
// common case). Each entry is resolved against the provider-level scalars here,
// once, so the request path only does a map lookup.
func newCapacityGateFromSpec(spec capacitySpec, keyLimits map[string]schemas.CooldownCapacityKeyLimit) *capacityGate {
	g := &capacityGate{
		tokenLimit:    spec.tokenLimit,
		requestLimit:  spec.requestLimit,
		windowSeconds: spec.windowSeconds,
		scope:         spec.scope,
		action:        spec.action,
		headroom:      spec.headroom,
		keys:          make(map[string]*capacityWindow),
	}
	if g.tokenLimit > 0 {
		g.tokenThresh = capacityThreshold(g.tokenLimit, g.headroom)
	}
	if g.requestLimit > 0 {
		g.requestThresh = capacityThreshold(g.requestLimit, g.headroom)
	}
	if len(keyLimits) > 0 && g.scope == CapacityScopeKey {
		g.keyLimits = make(map[string]capacityKeyLimits, len(keyLimits))
		for name, raw := range keyLimits {
			if name == "" {
				continue
			}
			keyLim := newCapacityKeyLimits(raw.TokenLimit, raw.RequestLimit, g.tokenLimit, g.requestLimit, g.headroom)
			g.keyLimits[name] = keyLim
			if len(raw.Models) == 0 {
				continue
			}
			models := make(map[string]capacityKeyLimits, len(raw.Models))
			for model, modelRaw := range raw.Models {
				if model == "" {
					continue
				}
				// Inherit from the credential layer, not the provider layer:
				// a per-model override only names what differs from its key.
				models[model] = newCapacityKeyLimits(modelRaw.TokenLimit, modelRaw.RequestLimit, keyLim.tokenLimit, keyLim.requestLimit, g.headroom)
			}
			if len(models) > 0 {
				if g.keyModelLimits == nil {
					g.keyModelLimits = make(map[string]map[string]capacityKeyLimits, len(keyLimits))
				}
				g.keyModelLimits[name] = models
			}
		}
	}
	return g
}

// newCapacityKeyLimits resolves one override layer on top of the values of the
// layer above it and precomputes its thresholds. Nil pointers inherit; an
// explicit 0 means "unlimited at this layer".
func newCapacityKeyLimits(tokenLimit, requestLimit *int64, baseToken, baseRequest int64, headroom float64) capacityKeyLimits {
	lim := capacityKeyLimits{tokenLimit: baseToken, requestLimit: baseRequest}
	if tokenLimit != nil {
		lim.tokenLimit = *tokenLimit
	}
	if requestLimit != nil {
		lim.requestLimit = *requestLimit
	}
	if lim.tokenLimit < 0 {
		lim.tokenLimit = 0
	}
	if lim.requestLimit < 0 {
		lim.requestLimit = 0
	}
	if lim.tokenLimit > 0 {
		lim.tokenThresh = capacityThreshold(lim.tokenLimit, headroom)
	}
	if lim.requestLimit > 0 {
		lim.requestThresh = capacityThreshold(lim.requestLimit, headroom)
	}
	return lim
}

// baseLimits is the provider-level budget, used for every credential without an
// override and for the scopes that have no credential in their accounting key.
func (g *capacityGate) baseLimits() capacityKeyLimits {
	return capacityKeyLimits{
		tokenLimit:    g.tokenLimit,
		requestLimit:  g.requestLimit,
		tokenThresh:   g.tokenThresh,
		requestThresh: g.requestThresh,
	}
}

// limitsFor resolves the budget that applies to one credential on one model.
// The most specific layer wins:
//
//	key × model → key → provider
//
// The second return value names the layer that produced the budget, so the
// usage snapshot can explain why a row's threshold is not the shared default.
func (g *capacityGate) limitsFor(keyName string, model string) (capacityKeyLimits, string) {
	if keyName != "" && len(g.keyModelLimits) > 0 {
		if models, ok := g.keyModelLimits[keyName]; ok && model != "" {
			if lim, ok := models[model]; ok {
				return lim, capacityLimitSourceKeyModel
			}
		}
	}
	if keyName != "" && len(g.keyLimits) > 0 {
		if lim, ok := g.keyLimits[keyName]; ok {
			return lim, capacityLimitSourceKey
		}
	}
	return g.baseLimits(), capacityLimitSourceBase
}

// matchesCapacitySpec reports whether the gate was built from the given
// normalized spec and per-key overrides. Field-by-field comparison: no reflect,
// no allocation.
func (g *capacityGate) matchesCapacitySpec(spec capacitySpec, keyLimits map[string]schemas.CooldownCapacityKeyLimit) bool {
	if g == nil {
		return false
	}
	if g.tokenLimit != spec.tokenLimit ||
		g.requestLimit != spec.requestLimit ||
		g.windowSeconds != spec.windowSeconds ||
		g.headroom != spec.headroom ||
		g.scope != spec.scope ||
		g.action != spec.action {
		return false
	}
	return g.keyLimitsMatch(keyLimits)
}

// keyLimitsMatch reports whether the gate's resolved overrides reflect the raw
// config map. It compares the raw values against the stored resolved ones, so
// no map is allocated on the request path; the common nil/nil case exits after
// one length check.
func (g *capacityGate) keyLimitsMatch(raw map[string]schemas.CooldownCapacityKeyLimit) bool {
	if len(g.keyLimits) != len(raw) {
		return false
	}
	for name, entry := range raw {
		cur, ok := g.keyLimits[name]
		if !ok {
			return false
		}
		want := newCapacityKeyLimits(entry.TokenLimit, entry.RequestLimit, g.tokenLimit, g.requestLimit, g.headroom)
		if cur != want {
			return false
		}
		curModels := g.keyModelLimits[name]
		if len(curModels) != len(entry.Models) {
			return false
		}
		for model, modelEntry := range entry.Models {
			curModel, ok := curModels[model]
			if !ok {
				return false
			}
			// Inherit from the credential layer, exactly as construction does.
			wantModel := newCapacityKeyLimits(modelEntry.TokenLimit, modelEntry.RequestLimit, cur.tokenLimit, cur.requestLimit, g.headroom)
			if curModel != wantModel {
				return false
			}
		}
	}
	return true
}

// capacityThreshold converts limit*headroom into the integer usage count at
// which the gate engages.
//
// The epsilon keeps the boundary exact for headrooms whose float product
// lands a hair above the intended integer (0.9*1000 evaluates to
// 900.0000000000001 in float64, which would otherwise push the gate to 901)
// while still rounding a genuinely fractional budget up (0.9*7 = 6.3 -> 7,
// since usage is always an integer count).
func capacityThreshold(limit int64, headroom float64) int64 {
	t := int64(math.Ceil(float64(limit)*headroom - 1e-9))
	if t < 1 {
		t = 1
	}
	return t
}

// windowKey returns the accounting key for (provider, key, model) under the
// configured scope.
//
// The SAME function builds the key for both record() and check(), which is
// what guarantees the two never disagree ("recorded in bucket A, looked up in
// bucket B" would let a key bypass its own budget).
//
//   - "provider"       → "provider" (one budget for the whole provider)
//   - "provider_model" → "provider|model"
//   - "key"            → "provider|keyID|model"
//
// A "key" scope without a keyID (a keyless provider, or an attempt that
// cannot be attributed to a credential) falls back to the provider_model key:
// the usage is still accounted, and it is accounted where it can also be
// checked.
func (g *capacityGate) windowKey(provider schemas.ModelProvider, keyID string, model string) string {
	switch g.scope {
	case CapacityScopeKey:
		if keyID == "" {
			return string(provider) + "|" + model
		}
		return string(provider) + "|" + keyID + "|" + model
	case CapacityScopeProviderModel:
		return string(provider) + "|" + model
	default:
		return string(provider)
	}
}

// nowSecond returns the current Unix second from the shared clock.
func (g *capacityGate) nowSecond() int64 {
	if g.now != nil {
		return g.now().Unix()
	}
	return time.Now().Unix()
}

// check reports whether the current window has already consumed its budget.
// A nil return means "allow". Reads lazily expire stale buckets and drop the
// key entry entirely once nothing is live, so a window that has fully slid
// away costs no memory.
//
// keyID selects the per-key budget under the "key" scope; it is ignored by
// the other scopes (their key does not contain a credential). The provider-level
// budget is used, which is the only budget when no per-key override is
// configured — see checkKey when the credential's name is known.
func (g *capacityGate) check(provider schemas.ModelProvider, keyID string, model string) *capacityVerdict {
	if g == nil {
		return nil
	}
	return g.checkWith(provider, keyID, "", model, g.baseLimits())
}

// checkKey is check with the credential's identity, so a per-key override
// (capacity.key_limits) applies. The name is also remembered against the ID, so
// the usage snapshot can label the row and report the same thresholds the gate
// actually used.
func (g *capacityGate) checkKey(provider schemas.ModelProvider, keyID string, keyName string, model string) *capacityVerdict {
	if g == nil {
		return nil
	}
	lim, _ := g.limitsFor(keyName, model)
	return g.checkWith(provider, keyID, keyName, model, lim)
}

func (g *capacityGate) checkWith(provider schemas.ModelProvider, keyID string, keyName string, model string, lim capacityKeyLimits) *capacityVerdict {
	if g.scope != CapacityScopeProvider && model == "" {
		// Nothing to attribute; never gate on an unattributable pair.
		return nil
	}
	key := g.windowKey(provider, keyID, model)
	now := g.nowSecond()

	g.mu.Lock()
	defer g.mu.Unlock()

	if keyName != "" && keyID != "" {
		// Remember the display name for the snapshot. Writing only on a change
		// keeps the steady state to one map read.
		if cur, ok := g.keyNames[keyID]; !ok || cur != keyName {
			if g.keyNames == nil {
				g.keyNames = make(map[string]string, 4)
			}
			g.keyNames[keyID] = keyName
		}
	}

	w := g.keys[key]
	if w == nil {
		return nil
	}
	tokens, requests := w.sweepLocked(g.windowSeconds, now)
	if tokens == 0 && requests == 0 {
		delete(g.keys, key)
		return nil
	}
	tokenHit := lim.tokenThresh > 0 && tokens >= lim.tokenThresh
	requestHit := lim.requestThresh > 0 && requests >= lim.requestThresh
	if !tokenHit && !requestHit {
		return nil
	}
	return &capacityVerdict{
		usedTokens:        tokens,
		usedRequests:      requests,
		retryAfterSeconds: g.retryAfterLocked(w, now, tokens, requests, lim.tokenThresh, lim.requestThresh),
		tokenLimit:        lim.tokenLimit,
		requestLimit:      lim.requestLimit,
	}
}

// record adds one terminal request's real usage to the window. tokens may be
// 0 (usage unavailable or a failure that was not billed) — the request count
// always advances, since it measures traffic we actually sent upstream.
//
// keyID is the credential that served the attempt; it selects the per-key
// budget under the "key" scope and is ignored by the other scopes.
func (g *capacityGate) record(provider schemas.ModelProvider, keyID string, model string, tokens int64) {
	if g == nil || provider == "" || model == "" {
		return
	}
	if tokens < 0 {
		tokens = 0
	}
	key := g.windowKey(provider, keyID, model)
	now := g.nowSecond()

	g.mu.Lock()
	w := g.keys[key]
	if w == nil {
		w = &capacityWindow{buckets: make([]capacityBucket, g.windowSeconds)}
		g.keys[key] = w
	}
	w.addLocked(g.windowSeconds, now, tokens)
	g.sweepKeysLocked(now)
	g.mu.Unlock()
}

// sweepKeysLocked drops accounting keys whose window has fully expired. It
// runs at most once per window so the key map stays bounded by the number of
// provider/model pairs with live traffic, without a background goroutine.
func (g *capacityGate) sweepKeysLocked(now int64) {
	if g.lastSweepSecond != 0 && now-g.lastSweepSecond < g.windowSeconds {
		return
	}
	g.lastSweepSecond = now
	for key, w := range g.keys {
		tokens, requests := w.sweepLocked(g.windowSeconds, now)
		if tokens == 0 && requests == 0 {
			delete(g.keys, key)
		}
	}
}

// retryAfterLocked estimates how many seconds must pass before both limits
// are back below their thresholds. Each dimension is evaluated independently
// by dropping its oldest live buckets (which is exactly what window sliding
// does) and taking the release time of the bucket that brings usage back
// under the threshold; the larger of the two wins, because both must hold.
// Falls back to a full window when nothing usable can be computed.
func (g *capacityGate) retryAfterLocked(w *capacityWindow, now, tokens, requests, tokenThresh, requestThresh int64) int64 {
	wait := int64(0)
	if tokenThresh > 0 && tokens >= tokenThresh {
		if d := g.secondsUntilBelow(w, now, tokens, tokenThresh, true); d > wait {
			wait = d
		}
	}
	if requestThresh > 0 && requests >= requestThresh {
		if d := g.secondsUntilBelow(w, now, requests, requestThresh, false); d > wait {
			wait = d
		}
	}
	if wait < 1 {
		wait = g.windowSeconds
	}
	return wait
}

// secondsUntilBelow walks live buckets from oldest to newest (the order the
// window releases them) and returns the seconds until usage drops below
// threshold. 0 means "no further release needed".
//
// A bucket for second e leaves the window at second e+window_seconds, so
// once the bucket that brings usage under the threshold is identified, the
// wait is exactly e+window_seconds-now.
func (g *capacityGate) secondsUntilBelow(w *capacityWindow, now, used, threshold int64, tokens bool) int64 {
	if w == nil {
		return g.windowSeconds
	}
	remaining := used
	for epoch := now - g.windowSeconds + 1; epoch <= now; epoch++ {
		b := &w.buckets[capacitySlot(epoch, g.windowSeconds)]
		if b.epoch != epoch {
			// Never written (or already recycled) — contributes nothing.
			continue
		}
		if tokens {
			remaining -= b.tokens
		} else {
			remaining -= b.requests
		}
		if remaining < threshold {
			release := epoch + g.windowSeconds - now
			if release < 1 {
				return 1
			}
			return release
		}
	}
	return g.windowSeconds
}

// sweepLocked zeroes every expired bucket and returns the live totals. A
// bucket with epoch e is live while e > now-windowSeconds (its release time
// is e+windowSeconds), so the live range is [now-window+1, now].
func (w *capacityWindow) sweepLocked(windowSeconds, now int64) (tokens, requests int64) {
	oldestLive := now - windowSeconds + 1
	for i := range w.buckets {
		b := &w.buckets[i]
		if b.epoch < oldestLive {
			// Includes epoch 0 (never written) and any bucket that has aged
			// out of the window. Reset in place; the ring slot is reused.
			b.epoch = 0
			b.tokens = 0
			b.requests = 0
			continue
		}
		tokens += b.tokens
		requests += b.requests
	}
	return tokens, requests
}

// addLocked accounts one request (and tokens) into the bucket for second now,
// recycling the slot when it belongs to an earlier second.
func (w *capacityWindow) addLocked(windowSeconds, now, tokens int64) {
	b := &w.buckets[capacitySlot(now, windowSeconds)]
	if b.epoch != now {
		b.epoch = now
		b.tokens = 0
		b.requests = 0
	}
	b.tokens += tokens
	b.requests++
}

// capacitySlot maps a Unix second to its ring index. Go's % keeps the sign of
// the dividend, so a pre-1970 clock would otherwise index negatively.
func capacitySlot(epoch, windowSeconds int64) int64 {
	idx := epoch % windowSeconds
	if idx < 0 {
		idx += windowSeconds
	}
	return idx
}

// --- observability helpers (used by tests and diagnostics) ---

// trackedKeys reports how many accounting keys currently hold state. A
// disabled gate is nil, so this is only reachable on an active gate.
func (g *capacityGate) trackedKeys() int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.keys)
}

// liveBuckets reports how many buckets for (provider, key, model) still carry
// a live epoch. Used by tests to assert stale buckets were zeroed.
func (g *capacityGate) liveBuckets(provider schemas.ModelProvider, keyID string, model string) int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	w := g.keys[g.windowKey(provider, keyID, model)]
	if w == nil {
		return 0
	}
	n := 0
	for i := range w.buckets {
		if w.buckets[i].epoch != 0 {
			n++
		}
	}
	return n
}

// usage returns the live window totals for (provider, key, model) without
// gating. Used by tests.
func (g *capacityGate) usage(provider schemas.ModelProvider, keyID string, model string) (tokens, requests int64) {
	if g == nil {
		return 0, 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	w := g.keys[g.windowKey(provider, keyID, model)]
	if w == nil {
		return 0, 0
	}
	return w.sweepLocked(g.windowSeconds, g.nowSecond())
}

// --- CooldownState / plugin wiring ---

// capacityHolder owns one provider's gate. The atomic pointer makes the
// steady-state read lock-free; the mutex only serializes a rebuild when that
// provider's effective capacity config changes at runtime.
type capacityHolder struct {
	mu   sync.Mutex
	gate atomic.Pointer[capacityGate]
}

// enableCapacity registers the plugin-level capacity section as the DEFAULT
// for every provider that does not configure its own. Like enableEscalation,
// this is normally called while the state is being built (AsState), but the
// atomic store makes a mid-flight reconfiguration race-free too.
func (c *CooldownState) enableCapacity(cfg *CapacityConfig) {
	if c == nil || cfg == nil || !cfg.Active() {
		return
	}
	spec, ok := capacitySpecOf(cfg)
	if !ok {
		return
	}
	g := newCapacityGateFromSpec(spec, cfg.KeyLimits)
	g.now = c.nowTime
	c.defaultCapacitySpec = spec
	c.defaultCapacityActive = true
	c.defaults.capacity = cfg.Normalized()
	c.capacity.Store(g)
}

// capacityGate returns the plugin-DEFAULT gate installed by enableCapacity, or
// nil when the feature is off by default. Per-provider gates (which also cover
// providers that enabled the feature through their own CooldownPolicy) are
// obtained through gateFor.
func (c *CooldownState) capacityGate() *capacityGate {
	if c == nil {
		return nil
	}
	return c.capacity.Load()
}

// gateFor returns the effective capacity gate for one provider under the given
// per-provider policy.
//
// Resolution is done first (provider section when present, plugin default
// otherwise), so PreProviderHook and PostLLMHook always agree on the config.
// The steady-state path is a spec comparison plus a sync.Map read plus an
// atomic load; a gate is only built when the provider's effective spec differs
// from the plugin default and from the gate already registered for it. A
// rebuild intentionally starts from a fresh window accounting state.
func (c *CooldownState) gateFor(provider schemas.ModelProvider, policy *schemas.CooldownPolicy) *capacityGate {
	if c == nil {
		return nil
	}
	cfg := c.defaults.resolveCapacity(policy)
	spec, ok := capacitySpecOf(cfg)
	if !ok {
		return nil
	}
	var keyLimits map[string]schemas.CooldownCapacityKeyLimit
	if cfg != nil {
		keyLimits = cfg.KeyLimits
	}
	// Same effective spec as the plugin default → share its gate. This is the
	// common case (a provider with no capacity section) and costs one struct
	// comparison plus the per-key override check, no allocation. The gate's
	// accounting keys already include the provider, so sharing is safe across
	// providers.
	if c.defaultCapacityActive && spec == c.defaultCapacitySpec {
		if g := c.capacity.Load(); g != nil && g.keyLimitsMatch(keyLimits) {
			return g
		}
	}

	holderAny, _ := c.capacityRegistry.Load(provider)
	holder, _ := holderAny.(*capacityHolder)
	if holder == nil {
		fresh := &capacityHolder{}
		holderAny, _ = c.capacityRegistry.LoadOrStore(provider, fresh)
		holder, _ = holderAny.(*capacityHolder)
	}
	if g := holder.gate.Load(); g != nil && g.matchesCapacitySpec(spec, keyLimits) {
		return g
	}

	holder.mu.Lock()
	defer holder.mu.Unlock()
	// Re-check under the lock: a concurrent request may have rebuilt it.
	if g := holder.gate.Load(); g != nil && g.matchesCapacitySpec(spec, keyLimits) {
		return g
	}
	g := newCapacityGateFromSpec(spec, keyLimits)
	g.now = c.nowTime
	holder.gate.Store(g)
	return g
}

// capacityGateForFilter resolves the gate the key pool filter must consult,
// or nil when there is nothing for it to do.
//
// It deliberately returns nil for every scope except "key": provider and
// provider_model marks are decided in PreProviderHook, before key selection,
// and their budget does not depend on which credential serves the request. By
// short-circuiting on the spec (which is a handful of comparisons, no
// allocation) the filter keeps the default — capacity disabled entirely —
// path at a couple of pointer reads.
func (c *CooldownState) capacityGateForFilter(provider schemas.ModelProvider, policy *schemas.CooldownPolicy) *capacityGate {
	if c == nil {
		return nil
	}
	spec, ok := capacitySpecOf(c.defaults.resolveCapacity(policy))
	if !ok || spec.scope != CapacityScopeKey {
		return nil
	}
	g := c.gateFor(provider, policy)
	if g == nil || g.scope != CapacityScopeKey {
		return nil
	}
	return g
}

// capacityShortCircuit evaluates the provider/model gate and, when the
// window's budget is spent, builds the synthetic 429 that short-circuits the
// attempt. Returns nil when the request may proceed.
//
// This is the provider / provider_model path: the accounting key does not
// contain a credential, so the decision can be made before key selection. The
// per-key scope is handled by capacityShortCircuitPerKey and the key pool
// filter (see AsFilter), which together keep "one over-budget key" from
// blocking the others.
func (p *CooldownPlugin) capacityShortCircuit(ctx *schemas.BifrostContext, gate *capacityGate, provider schemas.ModelProvider, model string) *schemas.LLMPluginShortCircuit {
	return p.capacityShortCircuitFor(ctx, gate, provider, "", model, gate.check(provider, "", model))
}

// capacityShortCircuitPerKey evaluates a "key"-scoped gate against the whole
// key pool and short-circuits only when NO key still has budget.
//
// Why the pool and not a single key: PreProviderHook runs before key
// selection, so it cannot know which credential the worker will pick. Asking
// "does the pool still have room?" is the only question with an answer at this
// point. Per-key precision is provided by AsFilter, which removes the
// exhausted keys from the candidate set so the worker rotates to one that
// still has budget — that is what makes "one key over its budget blocked,
// every other key unaffected" true.
//
// When at least one key still has budget this returns nil and the request
// proceeds; the filter then keeps the exhausted keys out of the selection.
func (p *CooldownPlugin) capacityShortCircuitPerKey(ctx *schemas.BifrostContext, gate *capacityGate, provider schemas.ModelProvider, model string, keys []schemas.Key) *schemas.LLMPluginShortCircuit {
	if gate == nil || len(keys) == 0 {
		return nil
	}
	// Pick the worst verdict (the longest retry-after) so the client is told
	// when the pool as a whole becomes usable again rather than when the
	// least-loaded key does.
	var worst *capacityVerdict
	worstKeyID := ""
	for _, k := range keys {
		v := gate.checkKey(provider, k.ID, k.Name, model)
		if v == nil {
			// This key still has budget: the request can be served.
			return nil
		}
		if worst == nil || v.retryAfterSeconds > worst.retryAfterSeconds {
			worst = v
			worstKeyID = k.ID
		}
	}
	return p.capacityShortCircuitFor(ctx, gate, provider, worstKeyID, model, worst)
}

// capacityShortCircuitFor builds the synthetic 429 for one verdict. keyID is
// empty (and omitted from the message) for the provider / provider_model
// scopes; it names the credential for the "key" scope so the operator can tell
// which budget exhausted the pool.
func (p *CooldownPlugin) capacityShortCircuitFor(ctx *schemas.BifrostContext, gate *capacityGate, provider schemas.ModelProvider, keyID string, model string, verdict *capacityVerdict) *schemas.LLMPluginShortCircuit {
	if verdict == nil {
		return nil
	}
	retryAfter := verdict.retryAfterSeconds
	statusCode := 429
	errType := capacityGateErrorType
	target := string(provider) + "/" + model
	if keyID != "" {
		target += " (key " + keyID + ")"
	}
	message := fmt.Sprintf(
		"%s%s exceeded its %ds budget; retry in %ds",
		capacityGateMessagePrefix, target, gate.windowSeconds, retryAfter,
	)
	// "fallback" lets the configured fallback chain pick the request up;
	// "reject" returns this error immediately. nil==true by default, so both
	// branches set the field explicitly.
	allowFallbacks := gate.action != CapacityActionReject
	if ctx != nil && retryAfter > 0 {
		ctx.SetValue(schemas.BifrostContextKeyRetryAfterSeconds, retryAfter)
	}
	if p.logger != nil {
		p.logger.Info(
			"[provider-cooldown] capacity gate short-circuit on %s/%s (key=%q) — used %d token(s) / %d request(s) in %ds window (token_limit=%d, request_limit=%d, headroom=%.2f, scope=%s, action=%s, retry-after %ds)",
			provider, model, keyID, verdict.usedTokens, verdict.usedRequests, gate.windowSeconds,
			verdict.tokenLimit, verdict.requestLimit, gate.headroom, gate.scope, gate.action, retryAfter,
		)
	}
	return &schemas.LLMPluginShortCircuit{
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
			AllowFallbacks: &allowFallbacks,
		},
		Silent: true,
	}
}

// recordCapacity applies one terminal request's real usage to the provider's
// effective capacity gate and to the provider's learner observer window.
//
// It runs for both success and failure because it measures traffic, not
// outcomes: the request count advances on every terminal outcome, and tokens
// are added only when the provider reported them (response usage on success,
// ExtraFields.BilledUsage on a failed-but-billed attempt). When usage cannot
// be determined the request is still counted, tokens are 0 — the documented
// degradation, since guessing would defeat the whole point of a gate that
// only trusts real numbers.
//
// Two guards keep rejected requests out of the budget:
//
//   - Attempts that never reached the provider are skipped. The framework
//     marks those with BifrostContextKeySilentLog (our own short-circuit,
//     the "no eligible keys" short-circuit, and the worker's synthetic
//     no_eligible_keys error), and plugin-produced responses (e.g. a cache
//     hit) carry no RoutingInfo.Provider.
//   - Streaming calls PostLLMHook once per chunk, so only the terminal chunk
//     is accounted.
func (p *CooldownPlugin) recordCapacity(ctx *schemas.BifrostContext, policy *schemas.CooldownPolicy, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) {
	if p == nil || p.State == nil {
		return
	}
	state := p.State
	// Cheap early-out before touching the response: when neither capability is
	// on for this provider (the default), a terminal request pays only the
	// config resolution.
	if !state.capacityOrLearningActive(policy) {
		return
	}
	provider, model, keyID, tokens, ok := terminalUsage(ctx, resp, bifrostErr)
	if !ok {
		return
	}
	if gate := state.gateFor(provider, policy); gate != nil {
		gate.record(provider, keyID, model, tokens)
	}
	// The learner's observer window accounts the same real usage, but
	// read-only: it never gates anything. It stays provider/model granular
	// because the estimate it produces is a provider/model limit.
	//
	// The per-credential ring records the same usage a second time, so a
	// rejection can be measured against the key that hit it ("key_model"
	// estimates). That ring is key-scoped; unattributable usage (keyless
	// provider / no attempt trail) falls back to the provider/model bucket,
	// exactly like the gate's own "key" scope does.
	if l := state.learnerFor(provider, policy); l != nil {
		l.observer.record(provider, "", model, tokens)
		l.keyObserver.record(provider, keyID, model, tokens)
	}
}

// terminalUsage extracts the (provider, model, keyID, tokens) of a terminal
// request from either the response or the error. ok=false means the outcome
// must not be accounted at all (non-terminal stream chunk, silent/short-
// circuited attempt, plugin-produced response, or unattributable routing
// info).
//
// keyID is the credential the attempt ran on, resolved from the attempt trail
// with the same rule the rest of the plugin uses (the last record with a
// usable key; the empty sentinel is legal only for keyless providers). It is
// what the "key"-scoped capacity gate accounts against; an empty value is the
// documented safe fallback that routes the usage to the provider/model bucket.
func terminalUsage(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (provider schemas.ModelProvider, model string, keyID string, tokens int64, ok bool) {
	if ctx == nil {
		return "", "", "", 0, false
	}
	// The framework sets this flag for attempts that were rejected before
	// reaching a provider. Counting them would let rejections extend the
	// very window that rejected them.
	if silent, _ := ctx.Value(schemas.BifrostContextKeySilentLog).(bool); silent {
		return "", "", "", 0, false
	}

	var requestType schemas.RequestType
	if bifrostErr != nil {
		requestType = bifrostErr.ExtraFields.RequestType
		routing := bifrostErr.ExtraFields.RoutingInfo
		provider = routing.Provider
		model = routing.Model
		tokens = usageTotal(bifrostErr.ExtraFields.BilledUsage)
	} else if resp != nil {
		extra := resp.GetExtraFields()
		if extra == nil {
			return "", "", "", 0, false
		}
		requestType = extra.RequestType
		routing := extra.RoutingInfo
		provider = routing.Provider
		model = routing.Model
		tokens = responseTokenUsage(resp)
	} else {
		return "", "", "", 0, false
	}

	// A response/error without routing info did not come from a provider
	// attempt (e.g. a semantic-cache hit produced by a plugin).
	if provider == "" || model == "" {
		return "", "", "", 0, false
	}
	keyID = attemptKeyID(ctx, provider)

	// Streaming calls PostLLMHook once per chunk; account only the terminal
	// one. Providers set BifrostContextKeyStreamEndIndicator on every
	// terminal path — a normal final chunk, a truncated stream, a cancellation
	// and an upstream read error all set it before running post hooks — so
	// this both avoids counting every chunk and keeps one stream from being
	// billed twice. A stream that dies without ever setting the marker is not
	// accounted (tokens 0, request 0): under-counting is preferred to
	// double-counting a stream's tokens.
	if isStreamingRequest(requestType) && !isFinalChunk(ctx) {
		return "", "", "", 0, false
	}
	return provider, model, keyID, tokens, true
}

// attemptKeyID returns the keyID of the attempt that produced the terminal
// outcome, resolved from the append-only attempt trail. It mirrors the trail
// scan in lastAttemptProviderAndKey / successProviderAndKey (empty keyID is
// legal only for keyless providers), but returns "" rather than failing when
// no usable key is recorded — the capacity gate treats that as the documented
// provider_model fallback.
func attemptKeyID(ctx *schemas.BifrostContext, provider schemas.ModelProvider) string {
	if ctx == nil {
		return ""
	}
	trail, ok := ctx.Value(schemas.BifrostContextKeyAttemptTrail).([]schemas.KeyAttemptRecord)
	if !ok {
		return ""
	}
	for i := len(trail) - 1; i >= 0; i-- {
		rec := trail[i]
		if rec.KeyID == "" && !schemas.IsKeylessProvider(provider) {
			continue
		}
		return rec.KeyID
	}
	return ""
}

// isCapacityGateError recognises the synthetic error built by
// capacityShortCircuit. Keeping the cooldown marker away from it matters: the
// rejection never reached a provider, and an operator's custom quota pattern
// (e.g. a bare "exceeded") could otherwise match the message and cool a key
// for a request we refused ourselves.
func isCapacityGateError(err *schemas.BifrostError) bool {
	if err == nil || err.Error == nil {
		return false
	}
	if err.Type == nil || *err.Type != capacityGateErrorType {
		return false
	}
	return strings.HasPrefix(err.Error.Message, capacityGateMessagePrefix)
}

// isStreamingRequest reports whether the given request type is a streaming
// one, mirroring bifrost.IsStreamRequestType. The realtime request type is
// excluded: a realtime turn is one completed exchange, not a chunk stream.
func isStreamingRequest(t schemas.RequestType) bool {
	switch t {
	case schemas.TextCompletionStreamRequest,
		schemas.ChatCompletionStreamRequest,
		schemas.ResponsesStreamRequest,
		schemas.ResponsesRetrieveStreamRequest,
		schemas.SpeechStreamRequest,
		schemas.TranscriptionStreamRequest,
		schemas.ImageGenerationStreamRequest,
		schemas.ImageEditStreamRequest,
		schemas.PassthroughStreamRequest,
		schemas.WebSocketResponsesRequest:
		return true
	default:
		return false
	}
}

// isFinalChunk mirrors bifrost.IsFinalChunk without importing the whole core
// package (the plugin only needs schemas).
func isFinalChunk(ctx *schemas.BifrostContext) bool {
	if ctx == nil {
		return false
	}
	v, ok := ctx.Value(schemas.BifrostContextKeyStreamEndIndicator).(bool)
	return ok && v
}

// usageTotal returns the total token count of a usage block, falling back to
// prompt+completion when the provider omitted the total.
func usageTotal(u *schemas.BifrostLLMUsage) int64 {
	if u == nil {
		return 0
	}
	if u.TotalTokens > 0 {
		return int64(u.TotalTokens)
	}
	return int64(u.PromptTokens) + int64(u.CompletionTokens)
}

// responseTokenUsage extracts the real token usage from any supported
// response shape, mirroring the logging plugin's extraction order. Returns 0
// when the shape carries no usage; callers treat that as "unknown" and still
// count the request.
func responseTokenUsage(resp *schemas.BifrostResponse) int64 {
	if resp == nil {
		return 0
	}
	switch {
	case resp.TextCompletionResponse != nil:
		return usageTotal(resp.TextCompletionResponse.Usage)
	case resp.ChatResponse != nil:
		return usageTotal(resp.ChatResponse.Usage)
	case resp.ResponsesResponse != nil && resp.ResponsesResponse.Usage != nil:
		return usageTotal(resp.ResponsesResponse.Usage.ToBifrostLLMUsage())
	case resp.ResponsesStreamResponse != nil && resp.ResponsesStreamResponse.Response != nil:
		return usageTotal(resp.ResponsesStreamResponse.Response.Usage.ToBifrostLLMUsage())
	case resp.CompactionResponse != nil && resp.CompactionResponse.Usage != nil:
		return usageTotal(resp.CompactionResponse.Usage.ToBifrostLLMUsage())
	case resp.EmbeddingResponse != nil:
		return usageTotal(resp.EmbeddingResponse.Usage)
	case resp.RerankResponse != nil:
		return usageTotal(resp.RerankResponse.Usage)
	case resp.TranscriptionResponse != nil && resp.TranscriptionResponse.Usage != nil:
		u := resp.TranscriptionResponse.Usage
		if u.TotalTokens != nil && *u.TotalTokens > 0 {
			return int64(*u.TotalTokens)
		}
		var total int64
		if u.InputTokens != nil {
			total += int64(*u.InputTokens)
		}
		if u.OutputTokens != nil {
			total += int64(*u.OutputTokens)
		}
		return total
	case resp.ImageGenerationResponse != nil && resp.ImageGenerationResponse.Usage != nil:
		u := resp.ImageGenerationResponse.Usage
		if u.TotalTokens > 0 {
			return int64(u.TotalTokens)
		}
		return int64(u.InputTokens) + int64(u.OutputTokens)
	case resp.PassthroughResponse != nil && resp.PassthroughResponse.PassthroughUsage != nil:
		return usageTotal(resp.PassthroughResponse.PassthroughUsage.LLMUsage)
	}
	return 0
}
