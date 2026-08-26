package bifrost

import (
	"context"
	"fmt"
	"testing"
	"time"

	schemas "github.com/pin-gou/pg-gateway/core/schemas"
)

// Benchmark helper: builds a Bifrost backed by a MockAccount with nProviders
// providers, each holding nKeys keys.
func benchBifrostWithProviders(nProviders, nKeys int) *Bifrost {
	ma := NewMockAccount()
	for i := 0; i < nProviders; i++ {
		p := schemas.ModelProvider(fmt.Sprintf("provider-%02d", i))
		ma.AddProvider(p, 1000, 5000)
		if nKeys > 1 {
			keys := make([]schemas.Key, 0, nKeys)
			for k := 0; k < nKeys; k++ {
				keys = append(keys, schemas.Key{
					ID:     fmt.Sprintf("key-%d-%d", i, k),
					Value:  *schemas.NewSecretVar(fmt.Sprintf("sk-%d-%d", i, k)),
					Weight: 100,
				})
			}
			ma.SetKeysForProvider(p, keys)
		}
	}
	return &Bifrost{account: ma}
}

func benchCtx() *schemas.BifrostContext {
	ctx, cancel := schemas.NewBifrostContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return ctx
}

// BenchmarkStampProviderKeysOnContext measures the per-attempt cost of
// stampProviderKeysOnContext (added 08-23 via PreProviderHook work) at
// increasing provider counts. This runs on EVERY attempt of EVERY request.
func BenchmarkStampProviderKeysOnContext(b *testing.B) {
	for _, np := range []int{1, 5, 20} {
		b.Run(fmt.Sprintf("providers=%d", np), func(b *testing.B) {
			bf := benchBifrostWithProviders(np, 1)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := benchCtx()
				bf.stampProviderKeysOnContext(ctx, "provider-00")
			}
		})
	}
}

// BenchmarkStampProviderKeysOnContextManyKeys measures how per-key filtering
// (governance include-only header path is excluded here) scales with keys.
func BenchmarkStampProviderKeysOnContextManyKeys(b *testing.B) {
	for _, nk := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("keys=%d", nk), func(b *testing.B) {
			bf := benchBifrostWithProviders(5, nk)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := benchCtx()
				bf.stampProviderKeysOnContext(ctx, "provider-00")
			}
		})
	}
}

// BenchmarkStampCooldownPolicyOnContext measures the per-attempt cost of
// stampProviderCooldownPolicyOnContext (added alongside the key stamp).
func BenchmarkStampCooldownPolicyOnContext(b *testing.B) {
	bf := benchBifrostWithProviders(5, 1)
	ctx := benchCtx()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bf.stampProviderCooldownPolicyOnContext(ctx, "provider-00")
	}
}

// BenchmarkFullPreProviderStamp is the realistic per-attempt cost: both stamps
// that tryRequest/tryStreamRequest now do on every attempt.
func BenchmarkFullPreProviderStamp(b *testing.B) {
	for _, np := range []int{1, 5, 20} {
		b.Run(fmt.Sprintf("providers=%d", np), func(b *testing.B) {
			bf := benchBifrostWithProviders(np, 1)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := benchCtx()
				bf.stampProviderKeysOnContext(ctx, "provider-00")
				bf.stampProviderCooldownPolicyOnContext(ctx, "provider-00")
			}
		})
	}
}
