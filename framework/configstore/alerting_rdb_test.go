package configstore

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newAlertingTestStore builds an in-memory sqlite-backed RDBConfigStore
// with the alert + budget tables pre-migrated. Each test gets a fresh
// schema so package-level globals cannot leak between tests.
func newAlertingTestStore(t *testing.T) *RDBConfigStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "create test database")
	require.NoError(t, db.AutoMigrate(
		&tables.TableAlertRule{},
		&tables.TableAlertEvent{},
		&tables.TableBudgetSnapshot{},
		&tables.TableBudget{},
	), "migrate alerting tables")
	s := &RDBConfigStore{logger: nil}
	s.db.Store(db)
	s.migrateOnFreshFn = func(ctx context.Context, fn func(context.Context, *gorm.DB) error) error {
		return fn(ctx, s.DB())
	}
	return s
}

// TestAlertRuleCreateRoundtrip exercises Create + Get + List + Update +
// Delete so a regression in any layer (validator, channel serialisation,
// soft delete) shows up immediately rather than in a downstream caller.
func TestAlertRuleCreateRoundtrip(t *testing.T) {
	store := newAlertingTestStore(t)
	ctx := context.Background()
	rule := &tables.TableAlertRule{
		Name:            "team 80%",
		ScopeType:       tables.AlertScopeTeam,
		ScopeID:         "t1",
		Metric:          tables.AlertMetricBudgetUsagePercent,
		Comparison:      tables.AlertComparisonGTE,
		Threshold:       80,
		CooldownMinutes: 30,
		Status:          tables.AlertStatusEnabled,
		Channels: []tables.AlertChannel{
			{Type: tables.AlertChannelTypeWebhook, WebhookID: "wh_1"},
		},
	}
	require.NoError(t, store.CreateAlertRule(ctx, rule))
	assert.NotEmpty(t, rule.ID, "CreateAlertRule should mint an id")
	assert.False(t, rule.CreatedAt.IsZero(), "CreatedAt should be stamped")
	assert.Equal(t, rule.UpdatedAt, rule.CreatedAt, "UpdatedAt starts equal to CreatedAt")

	got, err := store.GetAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "team 80%", got.Name)
	assert.Len(t, got.Channels, 1, "Channels should round-trip from JSON column")
	assert.Equal(t, "wh_1", got.Channels[0].WebhookID)

	// Update flips status, changes channels.
	got.Status = tables.AlertStatusDisabled
	got.Threshold = 90
	require.NoError(t, store.UpdateAlertRule(ctx, got))
	got2, err := store.GetAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	assert.Equal(t, tables.AlertStatusDisabled, got2.Status)
	assert.Equal(t, 90.0, got2.Threshold)

	// List with scope filter returns the row.
	rules, total, err := store.ListAlertRules(ctx, AlertRulesQueryParams{
		ScopeType: tables.AlertScopeTeam,
		ScopeID:   "t1",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, rules, 1)

	// Delete + Get returns (nil, nil).
	require.NoError(t, store.DeleteAlertRule(ctx, rule.ID))
	gone, err := store.GetAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
}

// TestAlertRuleCreateRejectsCacheMissRate locks in the P1 block at the
// store layer as well — a regression in the table hook would otherwise
// let a config-file import smuggle in the metric and the API layer
// would not catch it (the API only sees what the store accepts).
func TestAlertRuleCreateRejectsCacheMissRate(t *testing.T) {
	store := newAlertingTestStore(t)
	ctx := context.Background()
	rule := &tables.TableAlertRule{
		Name:            "p1",
		ScopeType:       tables.AlertScopeGlobal,
		Metric:          tables.AlertMetricCacheMissRate,
		Comparison:      tables.AlertComparisonGTE,
		Threshold:       10,
		CooldownMinutes: 60,
		Status:          tables.AlertStatusEnabled,
	}
	err := store.CreateAlertRule(ctx, rule)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "P1") || strings.Contains(err.Error(), "reserved"),
		"error should reference P1 block: %v", err)
}

// TestListAlertRulesForScopeMixed covers the AlertEvaluator lookup:
// a team-scoped rule + a global rule must both surface for the team
// scope, while a different team's rule must not.
func TestListAlertRulesForScopeMixed(t *testing.T) {
	store := newAlertingTestStore(t)
	ctx := context.Background()
	mk := func(name, scopeType, scopeID string) {
		err := store.CreateAlertRule(ctx, &tables.TableAlertRule{
			Name:            name,
			ScopeType:       scopeType,
			ScopeID:         scopeID,
			Metric:          tables.AlertMetricBudgetUsagePercent,
			Comparison:      tables.AlertComparisonGTE,
			Threshold:       80,
			CooldownMinutes: 60,
			Status:          tables.AlertStatusEnabled,
		})
		require.NoError(t, err)
	}
	mk("global", tables.AlertScopeGlobal, "")
	mk("team A", tables.AlertScopeTeam, "tA")
	mk("team B", tables.AlertScopeTeam, "tB")
	mk("disabled team A", tables.AlertScopeTeam, "tA")
	// flip the last one off
	rules, _, _ := store.ListAlertRules(ctx, AlertRulesQueryParams{ScopeType: tables.AlertScopeTeam, ScopeID: "tA"})
	require.Len(t, rules, 2)
	for i := range rules {
		if rules[i].Name == "disabled team A" {
			rules[i].Status = tables.AlertStatusDisabled
			require.NoError(t, store.UpdateAlertRule(ctx, &rules[i]))
		}
	}
	matched, err := store.ListAlertRulesForScope(ctx, tables.AlertScopeTeam, "tA")
	require.NoError(t, err)
	require.Len(t, matched, 2, "expected global + team A enabled rules")
	names := []string{matched[0].Name, matched[1].Name}
	assert.Contains(t, names, "global")
	assert.Contains(t, names, "team A")
}

// TestAlertEventRoundtrip + cooldown lookup. Verifies CreateAlertEvent
// stamps the missing fields and LatestAlertEventForRule honors the
// "since" filter so the cooldown window is correct.
func TestAlertEventRoundtripAndCooldown(t *testing.T) {
	store := newAlertingTestStore(t)
	ctx := context.Background()
	e1 := &tables.TableAlertEvent{
		RuleID:      "r1",
		ScopeType:   tables.AlertScopeTeam,
		ScopeID:     "t1",
		Event:       string(tables.WebhookEventAlertBudgetThreshold),
		MetricValue: 81.0,
		Threshold:   80,
		Message:     "first",
		TriggeredAt: time.Now().Add(-2 * time.Minute),
	}
	require.NoError(t, store.CreateAlertEvent(ctx, e1))
	require.NotEmpty(t, e1.ID)

	e2 := &tables.TableAlertEvent{
		RuleID:      "r1",
		ScopeType:   tables.AlertScopeTeam,
		ScopeID:     "t1",
		Event:       string(tables.WebhookEventAlertBudgetThreshold),
		MetricValue: 85.0,
		Threshold:   80,
		Message:     "second",
		TriggeredAt: time.Now().Add(-30 * time.Second),
	}
	require.NoError(t, store.CreateAlertEvent(ctx, e2))

	// since = 1 minute ago → only e2 is in-window.
	latest, err := store.LatestAlertEventForRule(ctx, "r1", tables.AlertScopeTeam, "t1", time.Now().Add(-1*time.Minute))
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "second", latest.Message)
	assert.Equal(t, e2.ID, latest.ID)

	// since = 5 minutes ago → both events qualify; Latest picks the
	// most-recent trigger (e2 by timestamp).
	latest5, err := store.LatestAlertEventForRule(ctx, "r1", tables.AlertScopeTeam, "t1", time.Now().Add(-5*time.Minute))
	require.NoError(t, err)
	require.NotNil(t, latest5)
	assert.Equal(t, e2.ID, latest5.ID)

	// missing rule/scope returns (nil, nil).
	none, err := store.LatestAlertEventForRule(ctx, "ghost", tables.AlertScopeTeam, "t1", time.Now().Add(-time.Minute))
	require.NoError(t, err)
	assert.Nil(t, none)
}

// TestBudgetSnapshotCreateAndLatest covers the projection endpoint's
// data source: writes sample, reads back latest, lists by budget id.
func TestBudgetSnapshotCreateAndLatest(t *testing.T) {
	store := newAlertingTestStore(t)
	ctx := context.Background()
	for i, used := range []float64{1, 5, 12} {
		err := store.CreateBudgetSnapshot(ctx, &tables.TableBudgetSnapshot{
			BudgetID:   "b1",
			UsedAmount: used,
			MaxAmount:  100,
			SampledAt:  time.Now().Add(time.Duration(i) * time.Minute),
		})
		require.NoError(t, err)
	}
	latest, err := store.LatestBudgetSnapshot(ctx, "b1")
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, 12.0, latest.UsedAmount)

	all, err := store.ListBudgetSnapshotsForBudget(ctx, "b1", 100)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

// TestBudgetSnapshotUniqueIndex: writing two snapshots for the same
// (budget_id, sampled_at) must fail at the database layer. This guards
// against a future refactor that accidentally drops the index.
func TestBudgetSnapshotUniqueIndex(t *testing.T) {
	store := newAlertingTestStore(t)
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, store.CreateBudgetSnapshot(ctx, &tables.TableBudgetSnapshot{
		BudgetID: "b1", UsedAmount: 5, MaxAmount: 100, SampledAt: t0,
	}))
	err := store.CreateBudgetSnapshot(ctx, &tables.TableBudgetSnapshot{
		BudgetID: "b1", UsedAmount: 7, MaxAmount: 100, SampledAt: t0,
	})
	require.Error(t, err, "duplicate (budget_id, sampled_at) must be rejected")
}

// TestAlertEventDeliveryStatusUpdate is a quick smoke test for the
// enqueue → status update path the AlertEvaluator uses to flip an event
// from pending to delivered/failed after the dispatcher decides.
func TestAlertEventDeliveryStatusUpdate(t *testing.T) {
	store := newAlertingTestStore(t)
	ctx := context.Background()
	e := &tables.TableAlertEvent{
		RuleID: "r1", ScopeType: tables.AlertScopeTeam, ScopeID: "t1",
		Event:     string(tables.WebhookEventAlertBudgetThreshold),
		TriggeredAt: time.Now().UTC(),
	}
	require.NoError(t, store.CreateAlertEvent(ctx, e))
	require.NoError(t, store.UpdateAlertEventDeliveryStatus(ctx, e.ID, tables.AlertEventDeliveryStatusDelivered))
	got, err := store.GetAlertEventByID(ctx, e.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, tables.AlertEventDeliveryStatusDelivered, got.DeliveryStatus)
}

// _ ensures unused imports don't sneak in via future refactors.
var _ = gorm.ErrRecordNotFound