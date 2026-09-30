package logstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// TestIsValidCostAccuracy pins the whitelist the filter applier relies on:
// only the three bands the cost writer produces may pass, everything else is
// rejected so an arbitrary query string never reaches the SQL text.
func TestIsValidCostAccuracy(t *testing.T) {
	for _, ok := range []string{
		CostAccuracyProviderReported,
		CostAccuracyGatewayEstimated,
		CostAccuracyUnknown,
	} {
		assert.True(t, IsValidCostAccuracy(ok), "%q should be valid", ok)
	}
	for _, bad := range []string{"", "all", "PROVIDER_REPORTED", "provider-reported", "bogus"} {
		assert.False(t, IsValidCostAccuracy(bad), "%q should be invalid", bad)
	}
}

// TestCanUseMatViewFilters_ExcludesCostAccuracy verifies that a cost-accuracy
// filter disqualifies the matview path: mv_logs_hourly has no cost_accuracy
// dimension, so serving a banded report from it would silently over-count.
func TestCanUseMatViewFilters_ExcludesCostAccuracy(t *testing.T) {
	assert.True(t, canUseMatViewFilters(SearchFilters{}), "empty filters → matview eligible")
	assert.True(t, canUseMatViewFilters(SearchFilters{Providers: []string{"openai"}}), "provider filter stays matview-eligible")
	assert.False(t, canUseMatViewFilters(SearchFilters{CostAccuracy: []string{CostAccuracyProviderReported}}),
		"cost_accuracy filter must force the raw path")
}

// TestApplyFiltersCostAccuracy checks the SQL the filter applier emits: the
// valid bands are bound to one IN clause and unknown values are dropped
// before they can widen the predicate.
func TestApplyFiltersCostAccuracy(t *testing.T) {
	s := newScopedDBTestLogStore(t)

	t.Run("valid bands are bound", func(t *testing.T) {
		filters := SearchFilters{CostAccuracy: []string{
			CostAccuracyProviderReported,
			"bogus", // must be dropped
			CostAccuracyGatewayEstimated,
		}}
		stmt := s.applyFilters(s.db.Table("logs"), filters).
			Session(&gorm.Session{DryRun: true}).Find(&struct{}{}).Statement

		assert.Contains(t, stmt.SQL.String(), "cost_accuracy IN")
		assert.Equal(t, []interface{}{CostAccuracyProviderReported, CostAccuracyGatewayEstimated}, stmt.Vars,
			"the unknown band must not be bound")
	})

	t.Run("all-invalid is a no-op", func(t *testing.T) {
		stmt := s.applyFilters(s.db.Table("logs"), SearchFilters{
			CostAccuracy: []string{"bogus", "PROVIDER_REPORTED"},
		}).Session(&gorm.Session{DryRun: true}).Find(&struct{}{}).Statement

		assert.NotContains(t, stmt.SQL.String(), "cost_accuracy",
			"an all-invalid filter must not narrow the query at all")
	})
}
