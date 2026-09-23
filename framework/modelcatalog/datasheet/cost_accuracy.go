package datasheet

// Cost accuracy bands attached to every costed log row. Stored on
// framework/logstore.Log.CostAccuracy so the actual-cost column carries a
// confidence tag alongside the dollar figure.
//
// Semantics (see temp/team/03-cost-allocation/data-model §5):
//   - provider_reported: response carried usage, cost comes from
//     provider-reported tokens at the datasheet rate; highest fidelity.
//   - gateway_estimated: no provider usage (failed/cancelled/truncated
//     stream), gateway estimated tokens itself; typically within 5% but
//     worse on long-context / multilingual payloads.
//   - unknown: no usage and no token estimate; the cost is 0 and the row is
//     filtered out of reconciliation reports.
const (
	CostAccuracyProviderReported = "provider_reported"
	CostAccuracyGatewayEstimated = "gateway_estimated"
	CostAccuracyUnknown          = "unknown"
)

// ClassifyCostAccuracy picks the right accuracy band given whether the
// provider returned usage and whether the gateway had enough information to
// estimate it on its own. Pure function — no I/O, no allocation.
//
// Inputs:
//   - hasProviderUsage: response carried a populated BifrostLLMUsage
//     (prompt/completion token counts from the provider).
//   - hasEstimatedTokens: gateway reconstructed tokens without provider
//     cooperation (failed request, cancelled stream, payload offloaded).
//
// Truth table:
//
//	providerUsage=true                     → provider_reported (highest)
//	providerUsage=false, estimated=true    → gateway_estimated
//	neither                                 → unknown (cost is 0)
func ClassifyCostAccuracy(hasProviderUsage, hasEstimatedTokens bool) string {
	switch {
	case hasProviderUsage:
		return CostAccuracyProviderReported
	case hasEstimatedTokens:
		return CostAccuracyGatewayEstimated
	default:
		return CostAccuracyUnknown
	}
}
