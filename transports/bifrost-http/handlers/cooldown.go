package handlers

import (
	"github.com/fasthttp/router"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/plugins/providercooldown"
	"github.com/maximhq/bifrost/transports/bifrost-http/lib"
	"github.com/valyala/fasthttp"
)

// CooldownStateResolver returns the currently-loaded provider-cooldown
// plugin or nil if none is loaded. Called on every request so plugin
// lifecycle (POST/PUT/DELETE /api/plugins) is honored — without this, the
// handler would hold a stale pointer after a plugin reload and the routes
// would silently misbehave (mirrors CacheClearerResolver in cache.go).
type CooldownStateResolver func() *providercooldown.CooldownPlugin

// CooldownHandler exposes read/management endpoints over the
// provider-cooldown plugin's in-memory cooldown state:
//
//	GET    /api/plugins/provider-cooldown/state                    — dump active entries
//	DELETE /api/plugins/provider-cooldown/state/{provider}/{keyId} — manually un-cool a key
//
// The handler is safe to wire unconditionally — when the plugin is not
// loaded, each request returns HTTP 400 with a clear message rather than
// the route being absent.
type CooldownHandler struct {
	resolve CooldownStateResolver
}

// NewCooldownHandler returns a CooldownHandler that resolves the current
// plugin at request time.
func NewCooldownHandler(resolve CooldownStateResolver) *CooldownHandler {
	return &CooldownHandler{resolve: resolve}
}

// RegisterRoutes registers the cooldown state endpoints under /api/plugins.
// The middlewares typically include the auth middleware so only operators
// can inspect or clear cooldown state.
func (h *CooldownHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/plugins/provider-cooldown/state", lib.ChainMiddlewares(h.getState, middlewares...))
	r.DELETE("/api/plugins/provider-cooldown/state/{provider}/{keyId}", lib.ChainMiddlewares(h.clearKey, middlewares...))
}

// cooldownStateResponse is the JSON shape returned by GET state.
type cooldownStateResponse struct {
	Plugin  string                           `json:"plugin"`
	Count   int                              `json:"count"`
	Entries []providercooldown.CooldownEntry `json:"entries"`
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
	SendJSON(ctx, cooldownStateResponse{
		Plugin:  providercooldown.PluginName,
		Count:   len(entries),
		Entries: entries,
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
	if !plugin.ClearKey(schemas.ModelProvider(provider), keyID) {
		SendError(ctx, fasthttp.StatusNotFound, "No active cooldown for this (provider, key)")
		return
	}
	SendJSON(ctx, map[string]any{
		"message":  "Cooldown cleared",
		"provider": provider,
		"key_id":   keyID,
	})
}
