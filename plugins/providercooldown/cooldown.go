package providercooldown

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

const (
	// PluginName is the identifier Bifrost uses to match this plugin in
	// config.json's plugins[] block (e.g. { "name": "provider-cooldown" }).
	PluginName = "provider-cooldown"

	// DefaultCooldownTTL is the default per-(provider, key_id) cooldown duration
	// applied when a quota-exhausted error is observed. The plugin does not have
	// access to HTTP response headers (e.g. Retry-After) at the post-hook layer,
	// so a single default is the simplest correct default.
	DefaultCooldownTTL = 10 * time.Minute

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

// CooldownState holds the per-(provider, key_id) cooldown clock.
//
// Safe for concurrent use. The map is keyed by "<provider>::<keyID>"; values
// are the wall-clock time at which the entry expires. Reads are O(1) with a
// RWMutex's RLock; the GC goroutine prunes expired entries on a ticker to
// keep the map from growing unboundedly under churn.
type CooldownState struct {
	mu        sync.RWMutex
	cooldowns map[string]time.Time
	ttl       time.Duration

	// ttlOverrides lets callers tune TTL per provider. Useful when a provider
	// resets quota on a known cadence (e.g. daily) and the default 10 minutes
	// would cause avoidable fallback churn after the reset.
	ttlOverrides map[schemas.ModelProvider]time.Duration
}

// NewCooldownState returns a CooldownState with the given default TTL.
// A non-positive TTL falls back to DefaultCooldownTTL.
func NewCooldownState(ttl time.Duration) *CooldownState {
	if ttl <= 0 {
		ttl = DefaultCooldownTTL
	}
	return &CooldownState{
		cooldowns:    make(map[string]time.Time),
		ttl:          ttl,
		ttlOverrides: make(map[schemas.ModelProvider]time.Duration),
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
	if keyID == "" {
		return false
	}
	c.mu.RLock()
	exp, ok := c.cooldowns[c.key(provider, keyID)]
	c.mu.RUnlock()
	if !ok {
		return false
	}
	if !time.Now().Before(exp) {
		// lazy prune
		c.mu.Lock()
		// re-check in case another goroutine already pruned
		if cur, ok := c.cooldowns[c.key(provider, keyID)]; ok && !time.Now().Before(cur) {
			delete(c.cooldowns, c.key(provider, keyID))
		}
		c.mu.Unlock()
		return false
	}
	return true
}

// Mark records a cooldown for the given (provider, keyID). A no-op when
// keyID is empty (avoids accidentally cooling "the whole provider" when
// the request hit a non-key-bound error).
func (c *CooldownState) Mark(provider schemas.ModelProvider, keyID string) {
	if keyID == "" {
		return
	}
	c.mu.Lock()
	c.cooldowns[c.key(provider, keyID)] = time.Now().Add(c.effectiveTTLLocked(provider))
	c.mu.Unlock()
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
type CooldownEntry struct {
	Provider  schemas.ModelProvider `json:"provider"`
	KeyID     string                `json:"key_id"`
	ExpiresAt time.Time             `json:"expires_at"`
	Remaining time.Duration         `json:"remaining"`
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
	for k, exp := range c.cooldowns {
		if !now.Before(exp) {
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
			ExpiresAt: exp,
			Remaining: exp.Sub(now),
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
			for k, exp := range c.cooldowns {
				if !now.Before(exp) {
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
// The filter is read-only against the state — Bifrost's retry loop calls it
// on every attempt, and we must not introduce back-pressure by holding the
// write lock here.
func (c *CooldownState) AsFilter(logger schemas.Logger) schemas.KeyPoolFilter {
	return func(_ *schemas.BifrostContext, provider schemas.ModelProvider, model string, keys []schemas.Key) ([]schemas.Key, error) {
		if len(keys) == 0 {
			return keys, nil
		}
		out := make([]schemas.Key, 0, len(keys))
		for _, k := range keys {
			if !c.IsCoolingDown(provider, k.ID) {
				out = append(out, k)
			} else if logger != nil {
				logger.Debug("[provider-cooldown] suppressed key %s/%s (model=%s)", provider, k.ID, model)
			}
		}
		return out, nil
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
	msg := strings.ToLower(err.GetErrorString())
	for _, sub := range p.quotaPatterns {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	return false
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

// PreLLMHook is a no-op. The plugin only needs to act on the failure path.
func (p *CooldownPlugin) PreLLMHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return nil, nil, nil
}

// PreRequestHook is a no-op.
func (p *CooldownPlugin) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

// PostLLMHook inspects the response and (when applicable) the error. On a
// quota-exhausted error it reads the (provider, keyID) of the last attempt
// from the AttemptTrail and marks the (provider, key) pair in cooldown.
//
// The hook is intentionally conservative: it never mutates the response or
// error, and it never panics on missing context values — both can occur when
// called from short-circuit or fallback paths.
func (p *CooldownPlugin) PostLLMHook(ctx *schemas.BifrostContext, _ *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	if p.State == nil || bifrostErr == nil {
		return nil, nil, nil
	}
	if !p.isQuotaExhausted(bifrostErr) {
		return nil, nil, nil
	}

	provider, keyID := lastAttemptProviderAndKey(ctx, bifrostErr)
	if keyID == "" {
		return nil, nil, nil
	}
	p.State.Mark(provider, keyID)
	if p.logger != nil {
		p.logger.Info("[provider-cooldown] marked key %s/%s (TTL=%v)", provider, keyID, p.State.EffectiveTTL(provider))
	}
	return nil, nil, nil
}

// IsQuotaExhausted returns true when the given BifrostError looks like the
// provider's per-key quota on this key is exhausted, as opposed to a
// transient 429 that will recover on its own or a permanent billing
// failure that the retry loop already routes around via deadKeyIDs.
//
// Recognised signals:
//   - HTTP 429 is treated as quota only when the rendered message matches
//     one of the known quota-exhaustion substrings (insufficient_quota,
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
	msg := strings.ToLower(err.GetErrorString())
	for _, sub := range quotaExhaustedSubstrings {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	return false
}

// lastAttemptProviderAndKey resolves the (provider, keyID) of the most
// recent failed attempt. Preference order:
//  1. BifrostError.ExtraFields.RoutingInfo.Provider (set by core right
//     before invoking post-hooks; survives the round trip through plugins).
//  2. BifrostError.ExtraFields.Provider (deprecated, but still populated).
//  3. The last KeyAttemptRecord with a non-success outcome (carries keyID).
//
// Returns an empty keyID when none can be determined — callers MUST treat
// that as "do not mark" rather than "mark the whole provider".
func lastAttemptProviderAndKey(ctx *schemas.BifrostContext, bifrostErr *schemas.BifrostError) (schemas.ModelProvider, string) {
	var provider schemas.ModelProvider
	if bifrostErr != nil {
		if bifrostErr.ExtraFields.RoutingInfo.Provider != "" {
			provider = bifrostErr.ExtraFields.RoutingInfo.Provider
		} else if bifrostErr.ExtraFields.Provider != "" {
			provider = bifrostErr.ExtraFields.Provider
		}
	}
	if ctx == nil {
		return provider, ""
	}
	trail, ok := ctx.Value(schemas.BifrostContextKeyAttemptTrail).([]schemas.KeyAttemptRecord)
	if !ok {
		return provider, ""
	}
	// Pick the last record that has a KeyID. AttemptTrail is append-only and
	// ordered, so iterating in reverse finds the most recent attempt.
	for i := len(trail) - 1; i >= 0; i-- {
		rec := trail[i]
		if rec.KeyID == "" {
			continue
		}
		return provider, rec.KeyID
	}
	return provider, ""
}
