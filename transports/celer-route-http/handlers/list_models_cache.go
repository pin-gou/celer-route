package handlers

import (
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// modelListCacheEnabled reports whether the DB-backed /v1/models fast path is
// wired up for this instance. The cache itself is opt-in: it only activates
// when a ConfigStore (sqlite/postgres) is configured. Config.json-only
// deployments (ConfigStore == nil) keep the previous always-fan-out behavior.
func (h *CompletionHandler) modelListCacheEnabled() bool {
	return h != nil && h.config != nil && h.config.ConfigStore != nil
}

// listModelsRequestIsVKScoped reports whether the request carries a resolved
// virtual key whose provider allowlist narrowed the fan-out. The cached
// aggregate list is built across ALL providers, so it must never be served to
// a VK-scoped request — doing so would leak models from providers the VK
// cannot access.
func listModelsRequestIsVKScoped(bifrostCtx *schemas.BifrostContext) bool {
	if bifrostCtx == nil {
		return false
	}
	// Any request carrying a virtual key is scoped — whether or not the key
	// resolves. Governance returns an empty list for an unresolvable VK, so
	// serving the unscoped aggregate cache to such a request would leak
	// providers the key cannot access.
	if vk := bifrostCtx.Value(schemas.BifrostContextKeyVirtualKey); vk != nil && vk != "" {
		return true
	}
	if resolvedVKFromBifrostContext(bifrostCtx) != nil {
		return true
	}
	if raw := bifrostCtx.Value(schemas.BifrostContextKeyAvailableProviders); raw != nil {
		if ap, ok := raw.([]schemas.ModelProvider); ok && len(ap) > 0 {
			return true
		}
	}
	return false
}

// tryServeListModelsFromCache attempts to serve GET /v1/models (aggregate, no
// provider) entirely from the DB-backed cache produced by a previous fan-out.
// It returns true when the response was written, meaning the caller should
// return. On a cache miss (or any decode error) it returns false so the caller
// falls through to the live ListAllModels fan-out — the cache is a pure
// optimization and must never turn a decodable situation into a failure.
func (h *CompletionHandler) tryServeListModelsFromCache(ctx *fasthttp.RequestCtx, bifrostCtx *schemas.BifrostContext, pageSize int, pageToken string) bool {
	if !h.modelListCacheEnabled() || listModelsRequestIsVKScoped(bifrostCtx) {
		return false
	}

	start := time.Now()
	cached, err := h.config.ConfigStore.GetCachedModelList(ctx, configstoreTables.ModelListCacheAll)
	if err != nil || cached == nil {
		return false
	}

	var models []schemas.Model
	if cached.ModelsJSON != "" {
		if err := schemas.Unmarshal([]byte(cached.ModelsJSON), &models); err != nil {
			return false
		}
	}
	var keyStatuses []schemas.KeyStatus
	if cached.KeyStatusesJSON != "" {
		if err := schemas.Unmarshal([]byte(cached.KeyStatusesJSON), &keyStatuses); err != nil {
			return false
		}
	}

	resp := &schemas.BifrostListModelsResponse{
		Data:        models,
		KeyStatuses: keyStatuses,
		ExtraFields: schemas.BifrostResponseExtraFields{
			RequestType: schemas.ListModelsRequest,
			Latency:     time.Since(start).Milliseconds(),
		},
	}
	resp = resp.ApplyPagination(pageSize, pageToken)

	h.finishListModelsResponse(ctx, bifrostCtx, resp)
	return true
}

// persistListModelsCache writes the result of a full /v1/models fan-out into
// the DB-backed cache so subsequent requests skip the expensive provider calls.
// Best-effort: a failure to persist is swallowed (the fan-out already happened,
// so the caller still returns a correct live response; the next request just
// re-fans-out). Empty results are deliberately NOT cached — a transient
// provider outage would otherwise freeze an empty list in place until the next
// config change invalidates it.
func (h *CompletionHandler) persistListModelsCache(ctx *fasthttp.RequestCtx, resp *schemas.BifrostListModelsResponse) {
	if resp == nil || len(resp.Data) == 0 {
		return
	}
	modelsJSON, err := schemas.Marshal(resp.Data)
	if err != nil {
		return
	}
	keyStatusesJSON := "[]"
	if len(resp.KeyStatuses) > 0 {
		if ks, err := schemas.Marshal(resp.KeyStatuses); err == nil {
			keyStatusesJSON = string(ks)
		}
	}
	entry := &configstoreTables.TableModelListCache{
		Provider:        configstoreTables.ModelListCacheAll,
		ModelsJSON:      string(modelsJSON),
		KeyStatusesJSON: keyStatusesJSON,
		UpdatedAt:       time.Now(),
	}
	_ = h.config.ConfigStore.UpsertCachedModelList(ctx, entry)
}

// finishListModelsResponse runs the shared response tail for GET /v1/models:
// large-response streaming, model-catalog enrichment, routing-rule virtual
// model backfill, routed-identity headers, and the JSON write. Shared by the
// live fan-out path and the DB-cache fast path so the response shape can never
// diverge between the two.
func (h *CompletionHandler) finishListModelsResponse(ctx *fasthttp.RequestCtx, bifrostCtx *schemas.BifrostContext, resp *schemas.BifrostListModelsResponse) {
	if streamLargeResponseIfActive(ctx, bifrostCtx) {
		return
	}
	enrichListModelsResponse(resp, h.config.ModelCatalog)
	h.enrichListModelsWithRoutingRuleBackfill(resp, bifrostCtx)
	if resp != nil {
		lib.ApplyBifrostResponseHeaders(ctx, bifrostCtx, resp.ExtraFields)
	}
	SendJSON(ctx, resp)
}
