package providercooldown

import (
	"sort"
	"strings"

	"github.com/pin-gou/celer-route/core/schemas"
)

// capacityUsageSource* label where a snapshot entry's gate came from. A
// provider that configures its own capacity section in its CooldownPolicy gets
// "provider"; every provider that simply inherits the plugin-level default
// gets "default".
const (
	capacityUsageSourceDefault  = "default"
	capacityUsageSourceProvider = "provider"
)

// ProviderCapacityUsage is one live entry of the retrospective capacity gate,
// as reported by the monitoring endpoint.
//
// It is a read-only snapshot: the usage numbers are the real tokens / requests
// accounted in the current rolling window, the thresholds are limit*headroom
// (the value the gate actually compares against), and Headroom is the fraction
// the operator configured.
//
// Granularity follows Scope: "provider" shares one budget per provider (Model
// and KeyID empty), "provider_model" reports one row per (provider, model), and
// "key" reports one row per (provider, key, model) with KeyID set. Usage that
// cannot be attributed to a credential under the "key" scope is accounted — and
// therefore reported — against the provider_model bucket (KeyID empty).
//
// Source tells the operator whether the effective budget comes from the
// provider's own cooldown_policy.capacity section ("provider") or from the
// plugin-level default every unconfigured provider inherits ("default").
type ProviderCapacityUsage struct {
	Provider schemas.ModelProvider `json:"provider"`
	KeyID    string                `json:"key_id,omitempty"`
	// KeyName is the credential's display name, resolved from the key pool on
	// the check path. Empty for the provider / provider_model scopes and for a
	// credential the gate has not seen in a key-pool check yet.
	KeyName           string  `json:"key_name,omitempty"`
	Model             string  `json:"model,omitempty"`
	Scope             string  `json:"scope"`
	WindowSeconds     int64   `json:"window_seconds"`
	UsedTokens        int64   `json:"used_tokens"`
	UsedRequests      int64   `json:"used_requests"`
	TokenLimit        int64   `json:"token_limit"`
	RequestLimit      int64   `json:"request_limit"`
	Headroom          float64 `json:"headroom"`
	ThresholdTokens   int64   `json:"threshold_tokens"`
	ThresholdRequests int64   `json:"threshold_requests"`
	RetryAfterSeconds int64   `json:"retry_after_seconds"`
	Source            string  `json:"source"`
}

// CapacityUsage returns a point-in-time snapshot of the capacity gate's live
// accounting entries across the plugin-default gate and every per-provider
// override gate. Only keys that still hold usage inside their window are
// reported, so a provider whose window has fully slid away does not appear.
//
// The slice is always non-nil (empty when no state or no gate is enabled), so
// the HTTP endpoint can serialize "entries": [] instead of null.
//
// Cost: this is a diagnostic read. It takes each gate's own mutex for the
// duration of the snapshot and does not touch, reorder or add locking to the
// request path.
func (p *CooldownPlugin) CapacityUsage() []ProviderCapacityUsage {
	if p == nil || p.State == nil {
		return []ProviderCapacityUsage{}
	}
	return p.State.capacityUsage()
}

// capacityUsage aggregates the default and per-provider gates into one sorted
// slice. A (provider, model, scope) that appears in both a stale default gate
// and a provider gate (possible while a reconfiguration is in flight) is
// reported once, with the provider-specific gate winning.
func (c *CooldownState) capacityUsage() []ProviderCapacityUsage {
	entries := make([]ProviderCapacityUsage, 0)
	if c == nil {
		return entries
	}
	if g := c.capacity.Load(); g != nil {
		entries = append(entries, g.capacityUsage(capacityUsageSourceDefault)...)
	}
	c.capacityRegistry.Range(func(_, v any) bool {
		if holder, ok := v.(*capacityHolder); ok {
			if g := holder.gate.Load(); g != nil {
				entries = append(entries, g.capacityUsage(capacityUsageSourceProvider)...)
			}
		}
		return true
	})
	return dedupeCapacityUsage(entries)
}

// capacityUsage snapshots one gate's live keys. Expired keys encountered on the
// way are dropped, which is the same lazy cleanup check() performs.
func (g *capacityGate) capacityUsage(source string) []ProviderCapacityUsage {
	if g == nil {
		return nil
	}
	now := g.nowSecond()

	g.mu.Lock()
	defer g.mu.Unlock()

	out := make([]ProviderCapacityUsage, 0, len(g.keys))
	for key, w := range g.keys {
		tokens, requests := w.sweepLocked(g.windowSeconds, now)
		if tokens == 0 && requests == 0 {
			delete(g.keys, key)
			continue
		}
		provider, keyID, model := splitCapacityWindowKey(key, g.scope)
		// A per-key override changes the thresholds this row was measured
		// against, so report the effective ones (and a readable label).
		keyName := ""
		if keyID != "" {
			keyName = g.keyNames[keyID]
		}
		lim := g.limitsFor(keyName)
		entry := ProviderCapacityUsage{
			Provider:          provider,
			KeyID:             keyID,
			KeyName:           keyName,
			Model:             model,
			Scope:             g.scope,
			WindowSeconds:     g.windowSeconds,
			UsedTokens:        tokens,
			UsedRequests:      requests,
			TokenLimit:        lim.tokenLimit,
			RequestLimit:      lim.requestLimit,
			Headroom:          g.headroom,
			ThresholdTokens:   lim.tokenThresh,
			ThresholdRequests: lim.requestThresh,
			Source:            source,
		}
		// Mirror check(): RetryAfterSeconds is only meaningful once the window
		// is actually over a threshold, and is 0 otherwise.
		tokenHit := lim.tokenThresh > 0 && tokens >= lim.tokenThresh
		requestHit := lim.requestThresh > 0 && requests >= lim.requestThresh
		if tokenHit || requestHit {
			entry.RetryAfterSeconds = g.retryAfterLocked(w, now, tokens, requests, lim.tokenThresh, lim.requestThresh)
		}
		out = append(out, entry)
	}
	return out
}

// splitCapacityWindowKey reverses windowKey. It returns the provider, the
// keyID (only for a "key"-scoped gate) and the model. The split is positional
// from the LEFT, so a model name that itself contains "|" is preserved intact.
//
//   - "provider"        → (provider, "", "")
//   - "provider_model"  → (provider, "", model)
//   - "key"             → (provider, keyID, model)
//
// A "key"-scoped entry that carries no credential (the documented fallback for
// unattributable usage) is stored as "provider|model", i.e. only one separator;
// it is reported as a key-scoped row with an empty KeyID so the operator can
// tell it apart from a real credential.
func splitCapacityWindowKey(key, scope string) (schemas.ModelProvider, string, string) {
	switch scope {
	case CapacityScopeKey:
		i := strings.IndexByte(key, '|')
		if i < 0 {
			return schemas.ModelProvider(key), "", ""
		}
		rest := key[i+1:]
		if j := strings.IndexByte(rest, '|'); j >= 0 {
			return schemas.ModelProvider(key[:i]), rest[:j], rest[j+1:]
		}
		// Fallback bucket: "provider|model" with no credential.
		return schemas.ModelProvider(key[:i]), "", rest
	case CapacityScopeProviderModel:
		if i := strings.IndexByte(key, '|'); i >= 0 {
			return schemas.ModelProvider(key[:i]), "", key[i+1:]
		}
		return schemas.ModelProvider(key), "", ""
	default:
		return schemas.ModelProvider(key), "", ""
	}
}

// dedupeCapacityUsage keeps one entry per (provider, keyID, model, scope),
// preferring the provider-specific gate over a stale default-gate entry, then
// sorts by provider, key and model for a stable, diffable response.
//
// The keyID is part of the identity: under the "key" scope several credentials
// of the same provider can serve the same model, and collapsing them would make
// one key's usage overwrite another's.
func dedupeCapacityUsage(in []ProviderCapacityUsage) []ProviderCapacityUsage {
	if len(in) == 0 {
		return in
	}
	idxByKey := make(map[string]int, len(in))
	out := in[:0]
	for _, e := range in {
		key := string(e.Provider) + "\x00" + e.KeyID + "\x00" + e.Model + "\x00" + e.Scope
		if idx, ok := idxByKey[key]; ok {
			if out[idx].Source == capacityUsageSourceDefault && e.Source == capacityUsageSourceProvider {
				out[idx] = e
			}
			continue
		}
		idxByKey[key] = len(out)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		if out[i].KeyID != out[j].KeyID {
			return out[i].KeyID < out[j].KeyID
		}
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].Source < out[j].Source
	})
	return out
}
