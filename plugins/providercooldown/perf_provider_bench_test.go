package providercooldown

import (
	"fmt"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// BenchmarkProviderCapabilityOffHotPath measures the plugin-level request path with
// every optional capability turned off (the default deployment), across 20
// providers. Each iteration carries a CooldownPolicy with no capacity /
// escalation / learning section, which is exactly what core stamps for a
// provider that configures none of them: resolution must fall through to the
// (inactive) plugin defaults for every provider without allocating.
//
// The two hooks measured here are the ones streaming calls per chunk, so a
// non-zero allocs/op would be a per-chunk regression, not a per-request one.
func BenchmarkProviderCapabilityOffHotPath(b *testing.B) {
	const providerCount = 20

	plugin := &CooldownPlugin{State: NewCooldownState(10 * time.Minute)}
	ctxs := make([]*schemas.BifrostContext, providerCount)
	reqs := make([]*schemas.BifrostRequest, providerCount)
	resps := make([]*schemas.BifrostResponse, providerCount)
	providers := make([]schemas.ModelProvider, providerCount)

	for i := 0; i < providerCount; i++ {
		prov := schemas.ModelProvider(fmt.Sprintf("bench-provider-%02d", i))
		providers[i] = prov
		policy := &schemas.CooldownPolicy{}
		ctx := schemas.NewBifrostContext(nil, time.Time{})
		ctx.SetValue(schemas.BifrostContextKeyCooldownPolicy, policy)
		ctxs[i] = ctx
		reqs[i] = capacityReq(prov, "bench-model")
		resps[i] = capacityResp(prov, "bench-model", 10)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i % providerCount
		if _, sc, _ := plugin.PreProviderHook(ctxs[idx], reqs[idx]); sc != nil {
			b.Fatalf("unexpected short-circuit for %s", providers[idx])
		}
		if _, _, err := plugin.PostLLMHook(ctxs[idx], resps[idx], nil); err != nil {
			b.Fatal(err)
		}
	}
}
