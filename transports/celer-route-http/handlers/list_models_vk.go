package handlers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	governanceplugin "github.com/pin-gou/celer-route/plugins/governance"
	"github.com/valyala/fasthttp"
)

// listModelsResolvedVKKey is the bifrost context key under which
// applyListModelsVirtualKeyProviderFilter stashes the resolved *TableVirtualKey
// for the duration of a single GET /v1/models call. The backfill stage reads
// it to compute the routing-rule scope chain without re-querying the config
// store.
//
// Kept handler-local (not on schemas.BifrostContextKey) because the value
// type is a concrete configstore row and core/schemas must stay free of that
// dependency.
var listModelsResolvedVKKey schemas.BifrostContextKey = "list-models-resolved-virtual-key"

// applyListModelsVirtualKeyProviderFilter narrows provider fan-out for GET /v1/models
// when the request is made with a virtual key. Without this, ListAllModels asks every
// configured provider to list models and governance rejects providers outside the VK,
// creating noisy, expected errors in request logs.
func (h *CompletionHandler) applyListModelsVirtualKeyProviderFilter(ctx *fasthttp.RequestCtx, bifrostCtx *schemas.BifrostContext) bool {
	vkValue := governanceplugin.ParseVirtualKeyFromFastHTTPRequest(ctx)
	if vkValue == nil {
		return true
	}

	trimmedVKValue := strings.TrimSpace(*vkValue)
	if trimmedVKValue == "" {
		return true
	}

	if h.config == nil || h.config.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "database store unavailable")
		return false
	}

	vk, err := h.config.ConfigStore.GetVirtualKeyByValue(ctx, trimmedVKValue)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			return true
		}
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to resolve virtual key: %v", err))
		return false
	}
	if vk == nil || vk.IsActive == nil || !*vk.IsActive {
		return true
	}

	availableProviders := make([]schemas.ModelProvider, 0, len(vk.ProviderConfigs))
	for _, providerConfig := range vk.ProviderConfigs {
		provider := strings.TrimSpace(providerConfig.Provider)
		if provider == "" {
			continue
		}
		availableProviders = append(availableProviders, schemas.ModelProvider(provider))
	}

	bifrostCtx.SetValue(schemas.BifrostContextKeyAvailableProviders, availableProviders)
	// Stash the resolved VK for the backfill stage below. We hand the caller
	// back a copy of the pointer so any later state mutation on the store's
	// in-memory copy doesn't desync the snapshot we recorded here.
	if vk != nil {
		bifrostCtx.SetValue(listModelsResolvedVKKey, vk)
	}
	return true
}

// resolvedVKFromBifrostContext returns the *TableVirtualKey that
// applyListModelsVirtualKeyProviderFilter stashed on bifrostCtx, or nil
// when the request did not carry a virtual key. Returns a typed *TableVirtualKey
// to avoid an interface{} assertion site at every caller.
func resolvedVKFromBifrostContext(bifrostCtx *schemas.BifrostContext) *configstoreTables.TableVirtualKey {
	if bifrostCtx == nil {
		return nil
	}
	v, _ := bifrostCtx.Value(listModelsResolvedVKKey).(*configstoreTables.TableVirtualKey)
	return v
}
