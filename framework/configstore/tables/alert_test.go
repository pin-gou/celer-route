package tables

import (
	"strings"
	"testing"
)

// TestTableAlertRuleBeforeSaveNormalises covers the table-level
// normalisation: scope_type / metric / comparison / status are
// lowercased and trimmed so the column index matches case-insensitive
// queries without surprising the admin UI.
func TestTableAlertRuleBeforeSaveNormalises(t *testing.T) {
	rule := &TableAlertRule{
		Name:            "  后端组预算 80%  ",
		ScopeType:       " Team ",
		ScopeID:         " t_xxx ",
		Metric:          " Budget_Usage_Percent ",
		Comparison:      " GTE ",
		Status:          " ENABLED ",
		CooldownMinutes: 60,
		Threshold:       80,
		Channels: []AlertChannel{
			{Type: AlertChannelTypeWebhook, WebhookID: "wh_1"},
		},
	}
	if err := rule.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave failed: %v", err)
	}
	if rule.ScopeType != "team" {
		t.Fatalf("scope_type not lowercased: %q", rule.ScopeType)
	}
	if rule.Metric != "budget_usage_percent" {
		t.Fatalf("metric not lowercased: %q", rule.Metric)
	}
	if rule.Comparison != "gte" {
		t.Fatalf("comparison not lowercased: %q", rule.Comparison)
	}
	if rule.Status != "enabled" {
		t.Fatalf("status not lowercased: %q", rule.Status)
	}
	if rule.ScopeID != "t_xxx" {
		t.Fatalf("scope_id not trimmed: %q", rule.ScopeID)
	}
	if rule.ChannelsJSON == "" {
		t.Fatalf("channels_json empty after BeforeSave")
	}
}

// TestTableAlertRuleRejectsCacheMissRate locks in the P1 block: a rule
// with metric=cache_miss_rate must be rejected at save time so the API
// layer never has to chase after a 422 on a rule that someone tried to
// smuggle in via config-file import.
func TestTableAlertRuleRejectsCacheMissRate(t *testing.T) {
	rule := &TableAlertRule{
		Name:            "p1 cache miss",
		ScopeType:       AlertScopeGlobal,
		Metric:          AlertMetricCacheMissRate,
		Threshold:       10,
		Comparison:      AlertComparisonGTE,
		CooldownMinutes: 60,
		Status:          AlertStatusEnabled,
	}
	err := rule.BeforeSave(nil)
	if err == nil {
		t.Fatalf("BeforeSave accepted cache_miss_rate — P1 metric should be rejected")
	}
	if !strings.Contains(err.Error(), "P1") {
		t.Fatalf("BeforeSave error should mention P1, got: %v", err)
	}
}

// TestTableAlertRuleGlobalScopeClearsScopeID makes the global-scope
// special case explicit: scope_id must be empty for global rules
// regardless of what the caller sent. Otherwise the column index would
// scatter empty-string rows among real ids and the API list filter
// would surprise the admin.
func TestTableAlertRuleGlobalScopeClearsScopeID(t *testing.T) {
	rule := &TableAlertRule{
		Name:            "global",
		ScopeType:       AlertScopeGlobal,
		ScopeID:         "should-be-cleared",
		Metric:          AlertMetricBudgetUsagePercent,
		Threshold:       50,
		Comparison:      AlertComparisonGTE,
		CooldownMinutes: 60,
		Status:          AlertStatusEnabled,
	}
	if err := rule.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave failed: %v", err)
	}
	if rule.ScopeID != "" {
		t.Fatalf("scope_id should be empty for global rules, got %q", rule.ScopeID)
	}
}

// TestTableAlertRuleNonGlobalRequiresScopeID covers the inverse: any
// non-global scope must carry a non-empty scope_id, so the API list
// filter doesn't return orphan rows.
func TestTableAlertRuleNonGlobalRequiresScopeID(t *testing.T) {
	rule := &TableAlertRule{
		Name:            "team rule",
		ScopeType:       AlertScopeTeam,
		ScopeID:         "",
		Metric:          AlertMetricBudgetUsagePercent,
		Threshold:       80,
		Comparison:      AlertComparisonGTE,
		CooldownMinutes: 60,
		Status:          AlertStatusEnabled,
	}
	if err := rule.BeforeSave(nil); err == nil {
		t.Fatalf("BeforeSave should reject non-global rule with empty scope_id")
	}
}

// TestTableAlertRuleShouldFire ensures the comparison helper agrees
// with the spec: gte fires when value >= threshold; lte fires when
// value <= threshold.
func TestTableAlertRuleShouldFire(t *testing.T) {
	above := &TableAlertRule{Threshold: 80, Comparison: AlertComparisonGTE}
	if !above.ShouldFire(80) || !above.ShouldFire(95) {
		t.Fatalf("gte should fire at and above threshold")
	}
	if above.ShouldFire(79) {
		t.Fatalf("gte should not fire below threshold")
	}
	below := &TableAlertRule{Threshold: 80, Comparison: AlertComparisonLTE}
	if !below.ShouldFire(80) || !below.ShouldFire(50) {
		t.Fatalf("lte should fire at and below threshold")
	}
	if below.ShouldFire(95) {
		t.Fatalf("lte should not fire above threshold")
	}
}

// TestTableAlertEventDeliveryStatusEnforced locks in the four-state
// delivery status enum. A bogus value would leave the row in an
// unfilterable state for the API list.
func TestTableAlertEventDeliveryStatusEnforced(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{AlertEventDeliveryStatusPending, true},
		{AlertEventDeliveryStatusDelivered, true},
		{AlertEventDeliveryStatusFailed, true},
		{AlertEventDeliveryStatusSkipped, true},
		{"bogus", false},
		{"", true}, // empty defaults to pending
	}
	for _, c := range cases {
		e := &TableAlertEvent{
			RuleID:         "r1",
			ScopeType:      AlertScopeGlobal,
			Event:          string(WebhookEventAlertBudgetThreshold),
			DeliveryStatus: c.status,
		}
		err := e.BeforeSave(nil)
		if c.want && err != nil {
			t.Fatalf("status %q should be accepted, got %v", c.status, err)
		}
		if !c.want && err == nil {
			t.Fatalf("status %q should be rejected", c.status)
		}
	}
}

// TestTableBudgetSnapshotBeforeSave covers the budget_snapshots table's
// input validation. The negative-amount guard is the load-bearing one:
// a downstream cost report could mis-attribute spend if a corrupt row
// slipped through.
func TestTableBudgetSnapshotBeforeSave(t *testing.T) {
	good := &TableBudgetSnapshot{BudgetID: "b1", UsedAmount: 10, MaxAmount: 100}
	if err := good.BeforeSave(nil); err != nil {
		t.Fatalf("good snapshot rejected: %v", err)
	}
	neg := &TableBudgetSnapshot{BudgetID: "b1", UsedAmount: -1, MaxAmount: 100}
	if err := neg.BeforeSave(nil); err == nil {
		t.Fatalf("negative used_amount should be rejected")
	}
	missingID := &TableBudgetSnapshot{UsedAmount: 10, MaxAmount: 100}
	if err := missingID.BeforeSave(nil); err == nil {
		t.Fatalf("missing budget_id should be rejected")
	}
}