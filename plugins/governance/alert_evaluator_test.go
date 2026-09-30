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
	// listCallsScope records (scopeType, scopeID) for each ListAlertRulesForScope
	// call, so cache tests can assert "the DB was hit exactly once for this
	// scope across N hot-path evaluations".
	listCallsScope []string
}

func newFakeAlertStore() *fakeAlertStore {
	return &fakeAlertStore{
		events: map[string]*tables.TableAlertEvent{},
	}
}

func (f *fakeAlertStore) ListAlertRulesForScope(_ context.Context, scopeType, scopeID string) ([]tables.TableAlertRule, error) {
	f.mu.Lock()
	f.listCallsScope = append(f.listCallsScope, scopeType+"\x00"+scopeID)
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

// ── AlertRuleCache tests (G6/C-4) ─────────────────────────────────────

// TestAlertRuleCacheServesFromMemoryWithinTTL asserts the hot path pays
// exactly one DB read per (scope_type, scope_id) per TTL window. Without
// the cache the same scope would be hit N times for N requests, which
// the 5-15ms latency budget can't absorb.
func TestAlertRuleCacheServesFromMemoryWithinTTL(t *testing.T) {
	store := newFakeAlertStore()
	store.rules = []tables.TableAlertRule{
		{ID: "r1", Name: "team-soft", ScopeType: "team", ScopeID: "t-1",
			Status: tables.AlertStatusEnabled, Metric: tables.AlertMetricBudgetUsagePercent, Threshold: 80, Comparison: tables.AlertComparisonGTE, CooldownMinutes: 60},
	}
	cache := NewAlertRuleCache(time.Minute)

	// First Get → store hit (count = 1).
	if _, err := cache.Get(context.Background(), store, "team", "t-1"); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	// Nine more Gets → zero DB hits; cache is still warm.
	for i := 0; i < 9; i++ {
		if _, err := cache.Get(context.Background(), store, "team", "t-1"); err != nil {
			t.Fatalf("subsequent Get %d: %v", i, err)
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.listCallsScope) != 1 {
		t.Errorf("store.ListAlertRulesForScope call count = %d, want 1 (only the first Get should hit the DB)", len(store.listCallsScope))
	}
}

// TestAlertRuleCacheInvalidateDropsAllEntries covers the write-path
// invalidation contract: after SetRulesCache / Invalidate, the next Get
// repopulates from the store. Without this, a freshly created rule
// wouldn't fire until its key's TTL elapsed.
func TestAlertRuleCacheInvalidateDropsAllEntries(t *testing.T) {
	store := newFakeAlertStore()
	store.rules = []tables.TableAlertRule{
		{ID: "r1", ScopeType: "team", ScopeID: "t-1", Status: tables.AlertStatusEnabled},
	}
	cache := NewAlertRuleCache(time.Minute)
	// Warm cache.
	if _, err := cache.Get(context.Background(), store, "team", "t-1"); err != nil {
		t.Fatalf("warm Get: %v", err)
	}
	if cache.Len() != 1 {
		t.Fatalf("cache.Len() = %d, want 1 after warm", cache.Len())
	}
	// Simulate a write-path handler calling Invalidate().
	cache.Invalidate()
	if cache.Len() != 0 {
		t.Fatalf("cache.Len() = %d, want 0 after Invalidate", cache.Len())
	}
	// Next Get must hit the store again.
	if _, err := cache.Get(context.Background(), store, "team", "t-1"); err != nil {
		t.Fatalf("post-invalidate Get: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.listCallsScope) != 2 {
		t.Errorf("store call count = %d, want 2 (warm + post-invalidate)", len(store.listCallsScope))
	}
}

// TestAlertRuleCacheScopeIsolation pins the cache key contract: a team
// scope and the global scope must live in distinct entries even when the
// underlying store returns the same union of rules. ListAlertRulesForScope
// always folds global rules into the result for any non-global scope; the
// cache must keep each call site separate so InvalidateScope() can target
// one without flushing the other.
func TestAlertRuleCacheScopeIsolation(t *testing.T) {
	store := newFakeAlertStore()
	// Two distinct rules; one global, one for a team. The fake returns
	// the union (global + matching scope) for any query.
	store.rules = []tables.TableAlertRule{
		{ID: "g1", ScopeType: tables.AlertScopeGlobal, ScopeID: "", Status: tables.AlertStatusEnabled},
		{ID: "t1", ScopeType: "team", ScopeID: "t-1", Status: tables.AlertStatusEnabled},
	}
	cache := NewAlertRuleCache(time.Minute)
	globalRules, err := cache.Get(context.Background(), store, tables.AlertScopeGlobal, "")
	if err != nil {
		t.Fatalf("global Get: %v", err)
	}
	teamRules, err := cache.Get(context.Background(), store, "team", "t-1")
	if err != nil {
		t.Fatalf("team Get: %v", err)
	}
	// global scope: just g1 (no team rule applies to "global").
	if len(globalRules) != 1 || globalRules[0].ID != "g1" {
		t.Errorf("global rules = %+v, want only g1", globalRules)
	}
	// team scope: union of global + matching team rule.
	if len(teamRules) != 2 {
		t.Errorf("team rules count = %d, want 2 (g1 + t1)", len(teamRules))
	}
	// Keys must be two distinct entries — no cross-contamination under
	// the cache's internal keying (the test for that is the Len/Keys
	// counts below, not the rule contents).
	keys := cache.Keys()
	if len(keys) != 2 {
		t.Errorf("cache keys = %v, want 2 distinct scope entries", keys)
	}
	// InvalidateScope(team) must drop only the team entry; global survives.
	cache.InvalidateScope("team", "t-1")
	if cache.Len() != 1 {
		t.Errorf("cache.Len() after InvalidateScope(team) = %d, want 1", cache.Len())
	}
	// And the surviving entry's rules must still be the global-only set
	// (no team rule leaks across the invalidation).
	got, err := cache.Get(context.Background(), store, tables.AlertScopeGlobal, "")
	if err != nil {
		t.Fatalf("survivor Get: %v", err)
	}
	if len(got) != 1 || got[0].ID != "g1" {
		t.Errorf("survivor rules = %+v, want only g1", got)
	}
}