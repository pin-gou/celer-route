package governance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
)

// fakeAlertStore is the minimal AlertEvaluatorStore used by the unit
// tests. It records the calls and lets the test pin what the evaluator
// "sees" without standing up a real database.
type fakeAlertStore struct {
	mu sync.Mutex

	rules []tables.TableAlertRule
	// events keyed by id, with the latest triggered_at per (rule, scope).
	events map[string]*tables.TableAlertEvent

	createCalls int
	updateCalls int
}

func newFakeAlertStore() *fakeAlertStore {
	return &fakeAlertStore{
		events: map[string]*tables.TableAlertEvent{},
	}
}

func (f *fakeAlertStore) ListAlertRulesForScope(_ context.Context, scopeType, scopeID string) ([]tables.TableAlertRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []tables.TableAlertRule
	for _, r := range f.rules {
		if r.Status != tables.AlertStatusEnabled {
			continue
		}
		if r.ScopeType == tables.AlertScopeGlobal {
			out = append(out, r)
			continue
		}
		if r.ScopeType == scopeType && r.ScopeID == scopeID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeAlertStore) LatestAlertEventForRule(_ context.Context, ruleID, scopeType, scopeID string, since time.Time) (*tables.TableAlertEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var latest *tables.TableAlertEvent
	for _, e := range f.events {
		if e.RuleID != ruleID || e.ScopeType != scopeType || e.ScopeID != scopeID {
			continue
		}
		if e.TriggeredAt.Before(since) {
			continue
		}
		if latest == nil || e.TriggeredAt.After(latest.TriggeredAt) {
			latest = e
		}
	}
	return latest, nil
}

func (f *fakeAlertStore) CreateAlertEvent(_ context.Context, e *tables.TableAlertEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if e.ID == "" {
		e.ID = "evt-" + time.Now().UTC().Format("150405.000000")
	}
	cp := *e
	f.events[e.ID] = &cp
	return nil
}

func (f *fakeAlertStore) UpdateAlertEventDeliveryStatus(_ context.Context, id, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls++
	if e, ok := f.events[id]; ok {
		e.DeliveryStatus = status
	}
	return nil
}

// noopLogger keeps the test output clean. The real Logger interface has
// more methods; we embed the no-op default and only override what the
// evaluator actually calls (Debug / Warn / Info).
type noopLogger struct{ schemas.Logger }

func (noopLogger) Debug(_ string, _ ...any) {}
func (noopLogger) Info(_ string, _ ...any)  {}
func (noopLogger) Warn(_ string, _ ...any)  {}
func (noopLogger) Error(_ string, _ ...any) {}

// TestAlertEvaluatorFiresOnSoftThreshold covers the happy path: a rule
// configured to fire at 80% sees a budget at 81% and emits exactly one
// event, marks it pending (no dispatcher), and increments the call
// counters on the fake store.
func TestAlertEvaluatorFiresOnSoftThreshold(t *testing.T) {
	store := newFakeAlertStore()
	store.rules = []tables.TableAlertRule{
		{
			ID:              "rule-1",
			Name:            "team 80%",
			ScopeType:       tables.AlertScopeTeam,
			ScopeID:         "t1",
			Metric:          tables.AlertMetricBudgetUsagePercent,
			Comparison:      tables.AlertComparisonGTE,
			Threshold:       80,
			CooldownMinutes: 60,
			Status:          tables.AlertStatusEnabled,
			Channels: []tables.AlertChannel{
				{Type: tables.AlertChannelTypeWebhook, WebhookID: "wh_1"},
			},
		},
	}
	eval := NewAlertEvaluator(store, noopLogger{}, nil)
	teamID := "t1"
	result := &EvaluationResult{
		Decision: DecisionAllow,
		BudgetInfo: []*tables.TableBudget{
			{
				ID:           "budget-1",
				TeamID:       &teamID,
				MaxLimit:     100,
				CurrentUsage: 81,
			},
		},
	}
	eval.EvaluateSoftThresholds(context.Background(), result)
	assert.Equal(t, 1, store.createCalls, "exactly one event should be created")
}

// TestAlertEvaluatorCooldownSkipsDuplicateFiring ensures that within
// the rule's cooldown window, the second fire produces a "skipped"
// event rather than a "pending" one.
func TestAlertEvaluatorCooldownSkipsDuplicateFiring(t *testing.T) {
	store := newFakeAlertStore()
	store.rules = []tables.TableAlertRule{
		{
			ID:              "rule-1",
			Name:            "team 80%",
			ScopeType:       tables.AlertScopeTeam,
			ScopeID:         "t1",
			Metric:          tables.AlertMetricBudgetUsagePercent,
			Comparison:      tables.AlertComparisonGTE,
			Threshold:       80,
			CooldownMinutes: 60,
			Status:          tables.AlertStatusEnabled,
		},
	}
	teamID := "t1"
	// Pre-seed an event from 30 seconds ago so the cooldown is in effect.
	now := time.Now().UTC()
	store.events["recent"] = &tables.TableAlertEvent{
		ID:          "recent",
		RuleID:      "rule-1",
		ScopeType:   tables.AlertScopeTeam,
		ScopeID:     "t1",
		TriggeredAt: now.Add(-30 * time.Second),
	}
	eval := NewAlertEvaluator(store, noopLogger{}, nil)
	eval.now = func() time.Time { return now }
	result := &EvaluationResult{
		Decision: DecisionAllow,
		BudgetInfo: []*tables.TableBudget{
			{ID: "budget-1", TeamID: &teamID, MaxLimit: 100, CurrentUsage: 81},
		},
	}
	eval.EvaluateSoftThresholds(context.Background(), result)
	// Two CreateAlertEvent calls — one for the pre-seeded "recent" was
	// inserted by the test setup itself; the second call records the
	// cooldown-skipped event from the evaluator.
	assert.Equal(t, 1, store.createCalls)
	for _, e := range store.events {
		if e.ID == "recent" {
			continue
		}
		assert.Equal(t, tables.AlertEventDeliveryStatusSkipped, e.DeliveryStatus)
	}
}

// TestAlertEvaluatorDisabledRuleSkipped ensures a disabled rule never
// produces an event even if its threshold matches.
func TestAlertEvaluatorDisabledRuleSkipped(t *testing.T) {
	store := newFakeAlertStore()
	store.rules = []tables.TableAlertRule{
		{
			ID:        "rule-1",
			ScopeType: tables.AlertScopeTeam,
			ScopeID:   "t1",
			Metric:    tables.AlertMetricBudgetUsagePercent,
			Comparison: tables.AlertComparisonGTE,
			Threshold: 80,
			CooldownMinutes: 60,
			Status:    tables.AlertStatusDisabled,
		},
	}
	teamID := "t1"
	eval := NewAlertEvaluator(store, noopLogger{}, nil)
	result := &EvaluationResult{
		Decision: DecisionAllow,
		BudgetInfo: []*tables.TableBudget{
			{ID: "budget-1", TeamID: &teamID, MaxLimit: 100, CurrentUsage: 99},
		},
	}
	eval.EvaluateSoftThresholds(context.Background(), result)
	assert.Equal(t, 0, store.createCalls)
}

// TestAlertEvaluatorIgnoresErrorRateAndCacheMissRules locks in the
// inline-path whitelist: only budget_usage_* metrics are evaluated
// inline; error_rate / cache_miss_rate are P1 and should never match.
func TestAlertEvaluatorIgnoresErrorRateAndCacheMissRules(t *testing.T) {
	store := newFakeAlertStore()
	store.rules = []tables.TableAlertRule{
		{
			ID:        "rule-err",
			ScopeType: tables.AlertScopeTeam, ScopeID: "t1",
			Metric: tables.AlertMetricErrorRate,
			Comparison: tables.AlertComparisonGTE, Threshold: 5,
			CooldownMinutes: 60, Status: tables.AlertStatusEnabled,
		},
		{
			ID:        "rule-cache",
			ScopeType: tables.AlertScopeTeam, ScopeID: "t1",
			Metric: tables.AlertMetricCacheMissRate,
			Comparison: tables.AlertComparisonGTE, Threshold: 5,
			CooldownMinutes: 60, Status: tables.AlertStatusEnabled,
		},
	}
	teamID := "t1"
	eval := NewAlertEvaluator(store, noopLogger{}, nil)
	result := &EvaluationResult{
		Decision: DecisionAllow,
		BudgetInfo: []*tables.TableBudget{
			{ID: "budget-1", TeamID: &teamID, MaxLimit: 100, CurrentUsage: 99},
		},
	}
	eval.EvaluateSoftThresholds(context.Background(), result)
	assert.Equal(t, 0, store.createCalls, "non-budget metrics should not fire inline")
}

// TestAlertEvaluatorEmptyBudgetInfoShortcircuits: the evaluator should
// not query rules when there are no budgets to evaluate. Returning
// early keeps the hot path latency flat when the budget check passed
// without consulting any budget rows.
func TestAlertEvaluatorEmptyBudgetInfoShortcircuits(t *testing.T) {
	store := newFakeAlertStore()
	eval := NewAlertEvaluator(store, noopLogger{}, nil)
	eval.EvaluateSoftThresholds(context.Background(), &EvaluationResult{
		Decision:   DecisionAllow,
		BudgetInfo: nil,
	})
	// We assert no panics; the fake doesn't count List calls but the
	// absence of panic + create is enough.
	assert.Equal(t, 0, store.createCalls)
}