package handlers

import (
	"strings"
	"time"

	"github.com/fasthttp/router"
	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/plugins/providercooldown"
	"github.com/pin-gou/pg-gateway/transports/pg-gateway-http/lib"
	"github.com/valyala/fasthttp"
)

// CooldownStateResolver returns the currently-loaded provider-cooldown
// plugin or nil if none is loaded. Called on every request so plugin
// lifecycle (POST/PUT/DELETE /api/plugins) is honored — without this, the
// handler would hold a stale pointer after a plugin reload and the routes
// would silently misbehave (mirrors CacheClearerResolver in cache.go).
type CooldownStateResolver func() *providercooldown.CooldownPlugin

// KeyNameResolver resolves a provider key's display name from its keyID.
// It lets the state handler surface human-friendly names without coupling
// the provider-cooldown plugin to the config store. Returning "" omits the
// key_name field entirely (e.g. when no resolver is wired or the key is no
// longer configured).
type KeyNameResolver func(provider schemas.ModelProvider, keyID string) string

// CooldownHandler exposes read/management endpoints over the
// provider-cooldown plugin's in-memory cooldown state:
//
//	GET    /api/plugins/provider-cooldown/state                    — dump active entries
//	GET    /api/plugins/provider-cooldown/stats                    — lifetime counters + active count
//	DELETE /api/plugins/provider-cooldown/state/{provider}/{keyId} — manually un-cool a key
//
// The handler is safe to wire unconditionally — when the plugin is not
// loaded, each request returns HTTP 400 with a clear message rather than
// the route being absent.
type CooldownHandler struct {
	resolve        CooldownStateResolver
	resolveKeyName KeyNameResolver
}

// NewCooldownHandler returns a CooldownHandler that resolves the current
// plugin at request time. An optional KeyNameResolver enriches each state
// entry with the key's display name.
func NewCooldownHandler(resolve CooldownStateResolver, keyNameResolvers ...KeyNameResolver) *CooldownHandler {
	var resolveKeyName KeyNameResolver
	if len(keyNameResolvers) > 0 {
		resolveKeyName = keyNameResolvers[0]
	}
	return &CooldownHandler{resolve: resolve, resolveKeyName: resolveKeyName}
}

// RegisterRoutes registers the cooldown state endpoints under /api/plugins.
// The middlewares typically include the auth middleware so only operators
// can inspect or clear cooldown state.
func (h *CooldownHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/plugins/provider-cooldown/state", lib.ChainMiddlewares(h.getState, middlewares...))
	r.GET("/api/plugins/provider-cooldown/stats", lib.ChainMiddlewares(h.getStats, middlewares...))
	r.DELETE("/api/plugins/provider-cooldown/state/{provider}/{keyId}", lib.ChainMiddlewares(h.clearKey, middlewares...))
}

// cooldownStateEntryResponse is the per-entry JSON shape returned by GET
// state. It mirrors providercooldown.CooldownEntry but adds an optional
// KeyName resolved from the configured provider keys and surfaces the
// Reason (= the CooldownKind that triggered the mark, or "quota"/
// "rate_limit" enum strings) so the UI can render "速率限制" / "配额耗尽"
// badges without a separate lookup.
//
// `Reason` is the human-readable counterpart of `Kind` from the
// providercooldown package — they are deliberately aliased here rather
// than renaming so the existing UI field name keeps working.
type cooldownStateEntryResponse struct {
	Provider  string        `json:"provider"`
	KeyID     string        `json:"key_id"`
	KeyName   string        `json:"key_name,omitempty"`
	Model     string        `json:"model,omitempty"`
	ExpiresAt time.Time     `json:"expires_at"`
	Remaining time.Duration `json:"remaining"`
	Reason    string        `json:"reason,omitempty"`
}

// cooldownStateResponse is the JSON shape returned by GET state.
type cooldownStateResponse struct {
	Plugin  string                       `json:"plugin"`
	Count   int                          `json:"count"`
	Entries []cooldownStateEntryResponse `json:"entries"`
}

func (h *CooldownHandler) getState(ctx *fasthttp.RequestCtx) {
	plugin := h.resolve()
	if plugin == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "provider-cooldown plugin is not loaded")
		return
	}
	entries := plugin.Snapshot()
	if entries == nil {
		entries = []providercooldown.CooldownEntry{}
	}
	out := make([]cooldownStateEntryResponse, 0, len(entries))
	for _, e := range entries {
		var keyName string
		if h.resolveKeyName != nil {
			keyName = h.resolveKeyName(e.Provider, e.KeyID)
		}
		out = append(out, cooldownStateEntryResponse{
			Provider:  string(e.Provider),
			KeyID:     e.KeyID,
			KeyName:   keyName,
			Model:     e.Model,
			ExpiresAt: e.ExpiresAt,
			Remaining: e.Remaining,
			Reason:    string(e.Kind),
		})
	}
	SendJSON(ctx, cooldownStateResponse{
		Plugin:  providercooldown.PluginName,
		Count:   len(out),
		Entries: out,
	})
}

// cooldownStatsResponse is the JSON shape returned by GET stats. It pairs
// the lifetime counters with a point-in-time active count so an operator
// can spot a healthy steady state ("suppressed_count climbing faster than
// mark_count" means a filter is constantly vetoing) versus a stuck state
// ("mark_count grows but suppressed_count stays flat" means the filter
// was never installed — the classic single-key-provider symptom).
//
// ByKind and PerProvider are added on top of the legacy fields to break
// down the same counters by CooldownKind (rate_limit / quota) and by
// (provider, kind). The legacy fields are preserved verbatim so older
// clients keep working without changes.
type cooldownStatsResponse struct {
	Plugin             string                              `json:"plugin"`
	MarkCount          uint64                              `json:"mark_count"`
	SuppressedCount    uint64                              `json:"suppressed_count"`
	CurrentActiveCount int                                 `json:"current_active_count"`
	ByKind             providercooldown.ByKindCounters      `json:"by_kind"`
	PerProvider        map[string]providercooldown.ProviderKindCounters `json:"per_provider"`
	PerProviderModel   map[string]map[string]providercooldown.ProviderKindCounters `json:"per_provider_model,omitempty"`
}

func (h *CooldownHandler) getStats(ctx *fasthttp.RequestCtx) {
	plugin := h.resolve()
	if plugin == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "provider-cooldown plugin is not loaded")
		return
	}
	stats := plugin.Stats()
	// Marshal the per-provider map with string keys (UI uses string
	// provider names) for a friendlier on-the-wire shape than
	// ModelProvider (which is also a string alias but reads ambiguously
	// in raw JSON).
	perProvider := make(map[string]providercooldown.ProviderKindCounters, len(stats.PerProvider))
	for provider, counters := range stats.PerProvider {
		perProvider[string(provider)] = counters
	}
	// Flatten the "provider::model" keys from CooldownStats into a nested
	// { <provider>: { <model>: {...} } } shape so the UI can address a
	// provider's per-model stats directly. Split on the LAST "::" (mirrors
	// providercooldown.parseCooldownKey) so model names containing "::" are
	// handled; a missing separator falls back to the whole key as provider.
	perProviderModel := make(map[string]map[string]providercooldown.ProviderKindCounters, len(stats.PerProviderModel))
	for mk, counters := range stats.PerProviderModel {
		provider, model := mk, ""
		if ridx := strings.LastIndex(mk, "::"); ridx >= 0 {
			provider, model = mk[:ridx], mk[ridx+2:]
		}
		if model == "" {
			continue
		}
		if perProviderModel[provider] == nil {
			perProviderModel[provider] = make(map[string]providercooldown.ProviderKindCounters)
		}
		perProviderModel[provider][model] = counters
	}
	SendJSON(ctx, cooldownStatsResponse{
		Plugin:             providercooldown.PluginName,
		MarkCount:          stats.MarkCount,
		SuppressedCount:    stats.SuppressedCount,
		CurrentActiveCount: stats.CurrentActiveCount,
		ByKind:             stats.ByKind,
		PerProvider:        perProvider,
		PerProviderModel:   perProviderModel,
	})
}

func (h *CooldownHandler) clearKey(ctx *fasthttp.RequestCtx) {
	plugin := h.resolve()
	if plugin == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "provider-cooldown plugin is not loaded")
		return
	}
	provider, ok := ctx.UserValue("provider").(string)
	if !ok || provider == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid provider")
		return
	}
	keyID, ok := ctx.UserValue("keyId").(string)
	if !ok || keyID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid keyId")
		return
	}
	model := string(ctx.QueryArgs().Peek("model"))
	if !plugin.ClearKey(schemas.ModelProvider(provider), keyID, model) {
		SendError(ctx, fasthttp.StatusNotFound, "No active cooldown for this (provider, key)")
		return
	}
	SendJSON(ctx, map[string]any{
		"message":  "Cooldown cleared",
		"provider": provider,
		"key_id":   keyID,
		"model":    model,
	})
}
