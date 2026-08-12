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
var quotaExhaustedSubstrings = []string{
	"insufficient_quota",
	"quota exceeded",
	"quota_exceeded",
	"billing",
	"payment required",
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

// Size returns the current number of cooldown entries (including those that
// may have expired but haven't been pruned yet). Primarily for tests and
// debug logging.
func (c *CooldownState) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cooldowns)
}

// RunGC prunes expired entries on a fixed cadence. Blocks until stop is
// closed (or receives a value). Safe to invoke from a goroutine without
// further synchronization.
//
// Typical usage:
//
//	stop := make(chan struct{})
//	go cooldown.RunGC(stop)
//	defer close(stop)
func (c *CooldownState) RunGC(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now()
			c.mu.Lock()
			for k, exp := range c.cooldowns {
				if !now.Before(exp) {
					delete(c.cooldowns, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

// AsFilter returns a schemas.KeyPoolFilter that suppresses any key currently
// in cooldown. Wire it into BifrostConfig.KeyPoolFilter at startup.
//
// The filter is read-only against the state — Bifrost's retry loop calls it
// on every attempt, and we must not introduce back-pressure by holding the
// write lock here.
func (c *CooldownState) AsFilter() schemas.KeyPoolFilter {
	return func(_ *schemas.BifrostContext, provider schemas.ModelProvider, _ string, keys []schemas.Key) ([]schemas.Key, error) {
		if len(keys) == 0 {
			return keys, nil
		}
		out := make([]schemas.Key, 0, len(keys))
		for _, k := range keys {
			if !c.IsCoolingDown(provider, k.ID) {
				out = append(out, k)
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
type CooldownPlugin struct {
	State *CooldownState
}

// NewPlugin returns a CooldownPlugin with a fresh state using DefaultCooldownTTL.
// For a custom TTL or to share state across plugins/filters, build the state
// via NewCooldownState and embed it directly.
func NewPlugin() *CooldownPlugin {
	return &CooldownPlugin{State: NewCooldownState(DefaultCooldownTTL)}
}

// Init applies the plugin's configuration. It is invoked by Bifrost with
// the raw `config` map from config.json's plugins[].config block before
// any request is served. The raw map may be nil — that is treated as an
// empty config and the plugin keeps its default state.
//
// Init is idempotent: calling it more than once replaces the state with
// one built from the new config. Callers that need to preserve a running
// state across reconfigs should call SetTTLOverride directly instead.
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
	p.State = cfg.AsState()
	return nil
}

// GetName implements schemas.BasePlugin.
func (p *CooldownPlugin) GetName() string { return pluginName }

// Cleanup implements schemas.BasePlugin. The plugin owns no external resources
// beyond the in-memory map; the GC goroutine (if started via State.RunGC)
// should be stopped by cancelling its context.
func (p *CooldownPlugin) Cleanup() error { return nil }

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
	if !IsQuotaExhausted(bifrostErr) {
		return nil, nil, nil
	}

	provider, keyID := lastAttemptProviderAndKey(ctx, bifrostErr)
	if keyID == "" {
		return nil, nil, nil
	}
	p.State.Mark(provider, keyID)
	return nil, nil, nil
}

// IsQuotaExhausted returns true when the given BifrostError looks like the
// provider's quota / billing on this key is exhausted, as opposed to a
// transient 429 that will recover on its own.
//
// Recognised signals:
//   - HTTP 402 (Payment Required) is always treated as billing/quota.
//   - HTTP 429 is treated as quota only when the rendered message matches
//     one of the known quota-exhaustion substrings (insufficient_quota,
//     quota exceeded, billing, payment required, usage limit). Generic
//     "rate limit" / "too many requests" messages are intentionally NOT
//     treated as quota — they self-heal and over-cooling them would
//     cause unnecessary fallback churn.
func IsQuotaExhausted(err *schemas.BifrostError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode != nil {
		switch *err.StatusCode {
		case 402:
			return true
		case 429:
			// fall through to message check
		default:
			if *err.StatusCode < 400 || *err.StatusCode >= 500 {
				return false
			}
		}
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
