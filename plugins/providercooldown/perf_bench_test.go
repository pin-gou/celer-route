package providercooldown

import (
	"fmt"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// benchKeyID builds a stable key id for benchmarks.
func benchKeyID(i int) string { return fmt.Sprintf("k%02d", i) }

// BenchmarkAsFilterKeyScope measures AsFilter when only key-granularity marks
// exist (the model != "" path still performs the model-scope lookup).
func BenchmarkAsFilterKeyScope(b *testing.B) {
	for _, nk := range []int{1, 10, 50} {
		b.Run("keys", func(b *testing.B) {
			st := NewCooldownState(10 * time.Minute)
			for i := 0; i < nk; i++ {
				st.MarkWithTTL(schemas.OpenAI, benchKeyID(i), "", time.Minute, CooldownKindQuota, schemas.CooldownScopeKey)
			}
			keys := make([]schemas.Key, 0, nk)
			for i := 0; i < nk; i++ {
				keys = append(keys, schemas.Key{ID: benchKeyID(i), Weight: 1})
			}
			f := st.AsFilter(nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := f(nil, schemas.OpenAI, "gpt-4o-mini", keys); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkAsFilterHappyPath measures the common case (no keys cooling): every
// key still runs lookupSuppressed -> key-scope + model-scope map lookups.
func BenchmarkAsFilterHappyPath(b *testing.B) {
	for _, nk := range []int{1, 10, 50} {
		b.Run("keys", func(b *testing.B) {
			st := NewCooldownState(10 * time.Minute)
			keys := make([]schemas.Key, 0, nk)
			for i := 0; i < nk; i++ {
				keys = append(keys, schemas.Key{ID: benchKeyID(i), Weight: 1})
			}
			f := st.AsFilter(nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := f(nil, schemas.OpenAI, "gpt-4o-mini", keys); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
