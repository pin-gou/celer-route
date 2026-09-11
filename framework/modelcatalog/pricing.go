package modelcatalog

import (
	"context"

	"github.com/pin-gou/celer-route/core/schemas"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/modelcatalog/datasheet"
)

// GetModelCapabilityEntryForModel returns capability metadata for a
// (model, provider) pair. Prefers chat, then responses, then text-completion
// entries; falls back to the lexicographically first available mode for
// deterministic behavior.
func (mc *ModelCatalog) GetModelCapabilityEntryForModel(model string, provider schemas.ModelProvider) *PricingEntry {
	return mc.datasheet.GetCapabilityEntry(model, provider)
}

// IsRequestTypeSupported preserves the historical (model, provider,
// requestType) signature; provider is ignored (the underlying datasheet
// index is keyed by model only).
func (mc *ModelCatalog) IsRequestTypeSupported(model string, provider schemas.ModelProvider, requestType schemas.RequestType) bool {
	return mc.datasheet.IsRequestTypeSupported(model, requestType)
}

func (mc *ModelCatalog) GetSupportedParameters(model string) []string {
	return mc.datasheet.GetSupportedParameters(model)
}

// ResolveModelParameters reads the model-parameters row for model, resolving
// provider-qualified or bare aliases to the datasheet's stored key (exact →
// provider-prefix-stripped → base model → provider-qualified variants).
func (mc *ModelCatalog) ResolveModelParameters(ctx context.Context, model string) (*configstoreTables.TableModelParameters, error) {
	return mc.datasheet.ResolveModelParameters(ctx, model)
}

func (mc *ModelCatalog) IsTextCompletionSupported(model string, provider schemas.ModelProvider) bool {
	return mc.datasheet.IsTextCompletionSupported(model, provider)
}

// GetPricingEntryForModel returns any pricing entry for the model across
// known modes. Used by the inference handler to enrich list-models responses.
func (mc *ModelCatalog) GetPricingEntryForModel(model string, provider schemas.ModelProvider) *PricingEntry {
	return mc.datasheet.GetPricingEntryForModel(model, provider)
}

// CalculateCost computes the dollar cost for a Bifrost response.
func (mc *ModelCatalog) CalculateCost(result *schemas.BifrostResponse, scopes *PricingLookupScopes) float64 {
	return mc.datasheet.CalculateCost(result, (*datasheet.LookupScopes)(scopes))
}

// CalculateCostForUsage computes the dollar cost from a bare usage object when
// no full BifrostResponse is available — used to bill partial usage carried on
// a failed/cancelled request (BifrostError.ExtraFields.BilledUsage).
func (mc *ModelCatalog) CalculateCostForUsage(usage *schemas.BifrostLLMUsage, provider schemas.ModelProvider, model string, requestType schemas.RequestType, scopes *PricingLookupScopes) float64 {
	return mc.datasheet.CalculateCostForUsage(usage, provider, model, requestType, (*datasheet.LookupScopes)(scopes))
}

// CalculateGuardrailCost computes the aggregate cost of guardrail judge calls.
func (mc *ModelCatalog) CalculateGuardrailCost(debug *schemas.BifrostGuardrailDebug, scopes *PricingLookupScopes) float64 {
	return mc.datasheet.CalculateGuardrailCost(debug, (*datasheet.LookupScopes)(scopes))
}

// CalculateCacheEmbeddingCost computes the semantic-cache embedding lookup cost.
func (mc *ModelCatalog) CalculateCacheEmbeddingCost(debug *schemas.BifrostCacheDebug, scopes *PricingLookupScopes) float64 {
	return mc.datasheet.CalculateCacheEmbeddingCost(debug, (*datasheet.LookupScopes)(scopes))
}

// UpsertModelPricingAttributes writes additional_attributes for every row
// matching (model, provider) and reloads the pricing cache.
func (mc *ModelCatalog) UpsertModelPricingAttributes(ctx context.Context, model string, provider schemas.ModelProvider, attrs map[string]string) (int64, error) {
	return mc.datasheet.UpsertModelPricingAttributes(ctx, model, provider, attrs)
}

// DeleteModelPricing deletes the pricing rows keyed by (model, provider) and
// reloads the pricing cache. Returns the number of rows deleted.
func (mc *ModelCatalog) DeleteModelPricing(ctx context.Context, model string, provider schemas.ModelProvider) (int64, error) {
	return mc.datasheet.DeleteModelPricing(ctx, model, provider)
}

// RenameModelPricing renames every pricing row keyed by (model, provider) to
// newModel and reloads the pricing cache. Returns the number of rows renamed.
func (mc *ModelCatalog) RenameModelPricing(ctx context.Context, model string, provider schemas.ModelProvider, newModel string) (int64, error) {
	return mc.datasheet.RenameModelPricing(ctx, model, provider, newModel)
}

// ReconcileProviderPricing deletes every non-custom pricing row for provider
// whose model is absent from latest, so the datasheet view converges to the
// latest key-discovered results. Skipped for providers whose /v1/models is a
// strict subset of their callable catalog (providersWithPartialListModels),
// where the datasheet is authoritative and must not be pruned. Custom rows are
// always kept. Returns the number of rows deleted.
func (mc *ModelCatalog) ReconcileProviderPricing(ctx context.Context, provider schemas.ModelProvider, latest []string) (int64, error) {
	if providersWithPartialListModels[provider] {
		return 0, nil
	}
	return mc.datasheet.ReconcileProviderPricing(ctx, provider, latest)
}

// PruneOrphanPricingForProvider deletes every non-custom pricing row for
// provider that is absent from the last successfully synced datasheet. Custom
// rows always survive. Used by the Sync button so stale entries disappear
// even when the provider's key list-models call fails. Returns the number of
// rows deleted.
func (mc *ModelCatalog) PruneOrphanPricingForProvider(ctx context.Context, provider schemas.ModelProvider) (int64, error) {
	return mc.datasheet.PruneOrphanPricingForProvider(ctx, provider)
}

// IsCustomModel reports whether the pricing row backing (model, provider) was
// seeded through the management API (Add Custom Model). Only custom models may
// be renamed/deleted from the provider detail Models tab.
func (mc *ModelCatalog) IsCustomModel(model string, provider schemas.ModelProvider) bool {
	return mc.datasheet.IsCustomModel(model, provider)
}

func (mc *ModelCatalog) SetPricingOverrides(rows []configstoreTables.TablePricingOverride) error {
	return mc.datasheet.SetOverrides(rows)
}

func (mc *ModelCatalog) UpsertPricingOverrides(rows ...*configstoreTables.TablePricingOverride) error {
	return mc.datasheet.UpsertOverrides(rows...)
}

func (mc *ModelCatalog) DeletePricingOverride(id string) {
	mc.datasheet.DeleteOverride(id)
}
