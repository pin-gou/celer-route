package datasheet

import (
	"github.com/pin-gou/celer-route/core/schemas"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
)

// StandardCost returns the team-allocation cost for a single request priced
// off the standard_prices price book (Phase 4 cost-allocation D7/D11). It
// runs alongside the existing computeCostFromInput — the two prices are
// reported separately so the gateway-delta endpoint can show the difference
// between what the provider actually billed and what the team ledger carries.
//
// Inputs:
//   - price is the standard_prices row matching (provider, model, request
//     time). nil = no standard price configured; we fall back to the
//     provider-side rate so the team ledger is never silently zero.
//   - usage is the BifrostLLMUsage — provider-reported tokens when present,
//     gateway-estimated otherwise. The cost shape matches the standard
//     price book: input tokens at the input rate, output at the output rate,
//     cache_read tokens at the cache_read rate (or the input rate when the
//     row opts out of cache-aware pricing).
//
// D11-A: fx is intentionally absent — under the chosen USD-everywhere
// strategy the price book stays single-currency and the input rate is the
// USD rate directly. fx_rate on the row is reserved for historical compat.
func StandardCost(price *configstoreTables.TableStandardPrice, usage *schemas.BifrostLLMUsage) float64 {
	if price == nil || usage == nil {
		return 0
	}

	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens

	// Cache split mirrors computeTextCost: cached_read is charged at the
	// cache_read rate, everything else at the regular input rate.
	cachedReadTokens := 0
	if usage.PromptTokensDetails != nil {
		cachedReadTokens = usage.PromptTokensDetails.CachedReadTokens
	}
	if cachedReadTokens > promptTokens {
		cachedReadTokens = promptTokens
	}
	nonCachedPrompt := promptTokens - cachedReadTokens
	if nonCachedPrompt < 0 {
		nonCachedPrompt = 0
	}

	inputCost := float64(nonCachedPrompt) * price.TokenCost(false)
	inputCost += float64(cachedReadTokens) * price.CacheReadTokenCost()

	outputCost := float64(completionTokens) * price.TokenCost(true)

	total := inputCost + outputCost
	if price.CostPerRequest != nil {
		total += *price.CostPerRequest
	}
	return total
}

// StandardCostWithFallback prices off the standard book when present, else
// falls back to the actual cost computed by the gateway's pricing store.
// The fallback keeps the team ledger whole when an admin hasn't yet synced
// the standard book for a freshly-shipped model — better to charge at the
// real provider rate than to silently zero out a row.
func StandardCostWithFallback(price *configstoreTables.TableStandardPrice, usage *schemas.BifrostLLMUsage, fallback float64) float64 {
	if price == nil {
		return fallback
	}
	return StandardCost(price, usage)
}
