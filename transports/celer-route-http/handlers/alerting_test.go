package handlers

import (
	"testing"
	"time"

	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeBudgetProjectionStableZeroRate covers the "no projection"
// case: when the rate is non-positive (spend flat or refunding), the
// projection is reported as low risk and HoursToExhaust stays nil so
// the UI does not surface "will exhaust in -3 hours".
func TestComputeBudgetProjectionStableZeroRate(t *testing.T) {
	now := time.Now().UTC()
	samples := []tables.TableBudgetSnapshot{
		{BudgetID: "b1", UsedAmount: 10, MaxAmount: 100, SampledAt: now.Add(-2 * time.Hour)},
		{BudgetID: "b1", UsedAmount: 10, MaxAmount: 100, SampledAt: now.Add(-1 * time.Hour)},
		{BudgetID: "b1", UsedAmount: 10, MaxAmount: 100, SampledAt: now},
	}
	proj := computeBudgetProjection(samples)
	assert.Equal(t, "low", proj.Risk)
	assert.Nil(t, proj.HoursToExhaust, "no hours-to-exhaust when rate <= 0")
}

// TestComputeBudgetProjectionFastExhaust covers the high-risk case:
// the spend rate projects exhaustion within 24 hours, so the
// classification lands on "high" even though the current usage is
// still under 50%.
func TestComputeBudgetProjectionFastExhaust(t *testing.T) {
	now := time.Now().UTC()
	samples := []tables.TableBudgetSnapshot{
		{BudgetID: "b1", UsedAmount: 10, MaxAmount: 100, SampledAt: now.Add(-2 * time.Hour)},
		{BudgetID: "b1", UsedAmount: 30, MaxAmount: 100, SampledAt: now.Add(-1 * time.Hour)},
		{BudgetID: "b1", UsedAmount: 50, MaxAmount: 100, SampledAt: now},
	}
	proj := computeBudgetProjection(samples)
	assert.Equal(t, "high", proj.Risk)
	require.NotNil(t, proj.HoursToExhaust)
	assert.Less(t, *proj.HoursToExhaust, 24.0, "should exhaust within 24h")
}

// TestComputeBudgetProjectionTooFewSamples: with < 2 samples we cannot
// fit a line, so the function falls back to the simple percentage
// classifier. The risk must reflect the latest sample's usage, not
// the threshold buckets.
func TestComputeBudgetProjectionTooFewSamples(t *testing.T) {
	now := time.Now().UTC()
	samples := []tables.TableBudgetSnapshot{
		{BudgetID: "b1", UsedAmount: 95, MaxAmount: 100, SampledAt: now},
	}
	proj := computeBudgetProjection(samples)
	assert.Equal(t, "high", proj.Risk, "95% usage → high even without enough samples")
}

// TestComputeBudgetProjectionEmpty: with no samples at all we get a
// zero-rate / low-risk projection. The handler's fallback path covers
// this in the API layer.
func TestComputeBudgetProjectionEmpty(t *testing.T) {
	proj := computeBudgetProjection(nil)
	assert.Equal(t, "low", proj.Risk)
}

// TestRiskLevelDirect covers the threshold buckets so a future edit to
// the buckets (e.g. 75 → 70) surfaces here rather than in a UI test.
func TestRiskLevelDirect(t *testing.T) {
	cases := []struct {
		percent, hours float64
		want           string
	}{
		{95, 0, "high"},
		{80, 0, "medium"},
		{50, 0, "low"},
		{50, 12, "high"},   // 12h to exhaust → high
		{50, 48, "medium"}, // 48h → medium
		{50, 100, "low"},   // 100h → low
	}
	for _, c := range cases {
		got := riskLevel(c.percent, c.hours)
		assert.Equal(t, c.want, got, "percent=%.0f hours=%.0f", c.percent, c.hours)
	}
}

// TestSafePercentZeroMax covers the degenerate case where the budget
// has no max (zero or negative) — the helper returns 0 to avoid NaN
// propagating into the JSON response.
func TestSafePercentZeroMax(t *testing.T) {
	assert.Equal(t, 0.0, safePercent(50, 0))
	assert.Equal(t, 0.0, safePercent(50, -10))
	assert.Equal(t, 75.0, safePercent(75, 100))
}