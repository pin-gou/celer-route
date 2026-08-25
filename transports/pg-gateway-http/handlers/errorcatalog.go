package handlers

import (
	"github.com/fasthttp/router"
	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/transports/pg-gateway-http/lib"
	"github.com/valyala/fasthttp"
)

// ErrorCatalogHandler serves the static type/code catalog per provider, used
// by the CooldownPolicy UI's type/code <select> pickers. The catalog is part
// of the gateway binary (see core/schemas/cooldown_dict.go) — this handler
// only wraps it for the HTTP boundary.
type ErrorCatalogHandler struct{}

// NewErrorCatalogHandler returns a handler ready for route registration.
func NewErrorCatalogHandler() *ErrorCatalogHandler {
	return &ErrorCatalogHandler{}
}

// RegisterRoutes mounts GET /api/cooldown/error-catalog. No RBAC required:
// the catalog is a public reference (operator-only, no secret data), and
// even anonymous operators can safely learn "what does OpenAI call a rate
// limit" since the data is from public provider documentation.
func (h *ErrorCatalogHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/cooldown/error-catalog", lib.ChainMiddlewares(h.getErrorCatalog, middlewares...))
}

// errorCatalogResponse is the wire shape. Provider is echoed back so the UI
// can confirm which catalog the server returned (matters for custom providers
// that get the generic fallback).
type errorCatalogResponse struct {
	Provider string   `json:"provider"`
	Types    []string `json:"types"`
	Codes    []string `json:"codes"`
}

// getErrorCatalog returns the catalog for ?provider=X. An empty provider
// returns the generic catalog; an unknown provider also returns the generic
// catalog (the catalog is never empty so the UI dropdown always has options).
func (h *ErrorCatalogHandler) getErrorCatalog(ctx *fasthttp.RequestCtx) {
	provider := schemas.ModelProvider(string(ctx.QueryArgs().Peek("provider")))
	catalog := schemas.LookupProviderErrorCatalog(provider)
	SendJSON(ctx, errorCatalogResponse{
		Provider: string(provider),
		Types:    catalog.Types,
		Codes:    catalog.Codes,
	})
}