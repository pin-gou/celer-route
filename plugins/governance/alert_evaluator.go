package governance

import (
	"context"
	"fmt"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/webhooks"
)

// AlertEvaluator runs after EvaluateGovernanceRequest to evaluate alert
// rules against the budgets it just consulted. Soft thresholds (below 100%)
// are evaluated inline on the request hot path; hard blocks (>= 100%) defer
// to EnqueueBudgetExceeded so the 402 response is never gated on alert I/O.
//
// The evaluator never returns an error to the caller: any failure is logged
// at warn and swallowed. This is the "失败绝不阻断" contract from
// temp/team/02-alerting/data-model.md §7, and it matches the webhook
// dispatcher's own behaviour for queue / delivery failures.
type AlertEvaluator struct {
	store      AlertEvaluatorStore
	logger     schemas.Logger
	dispatcher *webhooks.Dispatcher
	now        func() time.Time
}

// AlertEvaluatorStore is the subset of configstore + dispatcher glue the
// evaluator needs. Splitting it from configstore.ConfigStore keeps the
// interface narrow: the resolver / tracker / engine in main.go can use the
// full ConfigStore without dragging alert wiring into them.
type AlertEvaluatorStore interface {
	// ListAlertRulesForScope returns every enabled rule for the given
	// (scope_type, scope_id), including global rules.
	ListAlertRulesForScope(ctx context.Context, scopeType, scopeID string) ([]configstoreTables.TableAlertRule, error)
	// LatestAlertEventForRule powers the cooldown check.
	LatestAlertEventForRule(ctx context.Context, ruleID, scopeType, scopeID string, since time.Time) (*configstoreTables.TableAlertEvent, error)
	// CreateAlertEvent persists the new event row.
	CreateAlertEvent(ctx context.Context, event *configstoreTables.TableAlertEvent) error
	// UpdateAlertEventDeliveryStatus flips an event row from pending to
	// delivered/skipped after the enqueue attempt.
	UpdateAlertEventDeliveryStatus(ctx context.Context, id, status string) error
}

// NewAlertEvaluator builds a stopped evaluator. dispatcher may be nil when
// delivery is not configured; the evaluator still records events but skips
// the webhook enqueue, matching the dispatcher's nil-tolerant pattern.
func NewAlertEvaluator(store AlertEvaluatorStore, logger schemas.Logger, dispatcher *webhooks.Dispatcher) *AlertEvaluator {
	return &AlertEvaluator{
		store:      store,
		logger:     logger,
		dispatcher: dispatcher,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// EvaluateSoftThresholds inspects every budget in the result against the
// enabled rules whose scope matches the budget's owner. Rules whose
// comparison fires are recorded as alert_events and their channels are
// enqueued inline (soft threshold = no blocking).
//
// It returns silently on any error path. The hot path budget for the inline
// branch is "DB read of N rules + at most 1 event insert + at most K webhook
// queue inserts", expected to land well under 10ms for typical rule counts.
func (e *AlertEvaluator) EvaluateSoftThresholds(ctx context.Context, result *EvaluationResult) {
	if e == nil || result == nil || len(result.BudgetInfo) == 0 {
		return
	}
	for _, budget := range result.BudgetInfo {
		if budget == nil {
			continue
		}
		owners := budgetsToAlertScopes(budget)
		for _, owner := range owners {
			e.evaluateOne(ctx, owner.ScopeType, owner.ScopeID, budget)
		}
	}
}

// EnqueueBudgetExceeded is the async emission for the 100% hard block. It
// reads the budget, computes the scope, fans out to every webhook endpoint
// subscribed to budget.exceeded, and returns. The dispatcher is what does
// the actual delivery; this method never blocks the caller.
func (e *AlertEvaluator) EnqueueBudgetExceeded(ctx context.Context, budget *configstoreTables.TableBudget) {
	if e == nil || budget == nil || e.dispatcher == nil {
		return
	}
	// M-2: resolve the subscribers from the dispatcher's in-memory registry.
	//
	// This used to go through a subscribedEndpoints stub that returned an
	// empty list unconditionally, on the reasoning that the dispatcher filters
	// endpoints anyway — but the guard below treats an empty list as "nobody to
	// notify" and returns before the dispatcher is ever called, so that filter
	// never ran and budget.exceeded was delivered to no one. That is the only
	// signal an operator gets when a hard block starts answering 402, so the
	// failure was silent in exactly the case it exists for.
	//
	// EndpointIDsForEvent shares subscribesTo and the Disabled check with the
	// enqueue path, so discovery and delivery cannot disagree about who wants
	// this event. It reads memory, not the database, keeping the 402 path free
	// of a store round-trip. An empty result now genuinely means "no endpoint
	// subscribes".
	endpointIDs := e.dispatcher.EndpointIDsForEvent(configstoreTables.WebhookEventBudgetExceeded)
	if len(endpointIDs) == 0 {
		return
	}
	max := budget.EffectiveMaxLimit()
	queued := e.dispatcher.EnqueueBudgetExceeded(ctx, scopeTypeForBudget(budget), scopeIDForBudget(budget), budget.CurrentUsage, max, endpointIDs)
	if queued > 0 {
		e.logger.Debug("alert: budget.exceeded queued to %d endpoint(s) for budget %s", queued, budget.ID)
	}
}

// evaluateOne fans a single (scope, budget) pair through the rule matcher.
func (e *AlertEvaluator) evaluateOne(ctx context.Context, scopeType, scopeID string, budget *configstoreTables.TableBudget) {
	max := budget.EffectiveMaxLimit()
	if max <= 0 {
		return
	}
	percent := budget.CurrentUsage / max * 100
	rules, err := e.store.ListAlertRulesForScope(ctx, scopeType, scopeID)
	if err != nil {
		e.logger.Warn("alert: list rules for %s/%s failed: %v", scopeType, scopeID, err)
		return
	}
	for i := range rules {
		rule := &rules[i]
		if !ruleMetricMatches(rule, scopeType, budget) {
			continue
		}
		if !rule.ShouldFire(percent) {
			continue
		}
		e.fireAndEnqueue(ctx, rule, scopeType, scopeID, percent, max, budget)
	}
}

// fireAndEnqueue records the event row and (best-effort) enqueues webhook
// deliveries for every channel on the rule that resolves to a subscribed
// endpoint. The whole body is wrapped in defensive logging: a single rule
// that fails its enqueue must not leak back into the caller.
func (e *AlertEvaluator) fireAndEnqueue(
	ctx context.Context,
	rule *configstoreTables.TableAlertRule,
	scopeType, scopeID string,
	percent float64,
	max float64,
	budget *configstoreTables.TableBudget,
) {
	now := e.now()
	cooldownStart := now.Add(-rule.CooldownDuration())
	last, err := e.store.LatestAlertEventForRule(ctx, rule.ID, scopeType, scopeID, cooldownStart)
	if err != nil {
		e.logger.Warn("alert: cooldown lookup for rule %s failed: %v", rule.ID, err)
	} else if last != nil {
		// Suppressed by cooldown: still record the "skipped" event so the
		// rate-of-fire reports stay honest, but do not enqueue.
		skipped := &configstoreTables.TableAlertEvent{
			RuleID:         rule.ID,
			ScopeType:      scopeType,
			ScopeID:        scopeID,
			Event:          eventForMetric(rule.Metric),
			MetricValue:    percent,
			Threshold:      rule.Threshold,
			Message:        fmt.Sprintf("%s suppressed by cooldown (last fired %s)", rule.Name, last.TriggeredAt.Format(time.RFC3339)),
			DeliveryStatus: configstoreTables.AlertEventDeliveryStatusSkipped,
			TriggeredAt:    now,
		}
		if err := e.store.CreateAlertEvent(ctx, skipped); err != nil {
			e.logger.Warn("alert: write suppressed event for rule %s failed: %v", rule.ID, err)
		}
		return
	}

	event := &configstoreTables.TableAlertEvent{
		RuleID:         rule.ID,
		ScopeType:      scopeType,
		ScopeID:        scopeID,
		Event:          eventForMetric(rule.Metric),
		MetricValue:    percent,
		Threshold:      rule.Threshold,
		Message:        fmt.Sprintf("%s fired: %.2f%% of $%.4f (threshold %.2f)", rule.Name, percent, max, rule.Threshold),
		DeliveryStatus: configstoreTables.AlertEventDeliveryStatusPending,
		TriggeredAt:    now,
	}
	if err := e.store.CreateAlertEvent(ctx, event); err != nil {
		e.logger.Warn("alert: write event for rule %s failed: %v", rule.ID, err)
		return
	}
	if e.dispatcher == nil || len(rule.Channels) == 0 {
		return
	}
	endpointIDs := e.resolveChannelEndpoints(ctx, rule)
	if len(endpointIDs) == 0 {
		_ = e.store.UpdateAlertEventDeliveryStatus(ctx, event.ID, configstoreTables.AlertEventDeliveryStatusSkipped)
		return
	}
	queued := e.dispatcher.EnqueueAlertEvent(ctx, rule, event, endpointIDs)
	if queued > 0 {
		_ = e.store.UpdateAlertEventDeliveryStatus(ctx, event.ID, configstoreTables.AlertEventDeliveryStatusDelivered)
	} else {
		_ = e.store.UpdateAlertEventDeliveryStatus(ctx, event.ID, configstoreTables.AlertEventDeliveryStatusFailed)
	}
}

// resolveChannelEndpoints collects every endpoint id the rule's webhook
// channels reference. The dispatcher's EnqueueAlertEvent already filters
// missing / disabled / unsubscribed endpoints, so this function does not
// pre-check.
func (e *AlertEvaluator) resolveChannelEndpoints(_ context.Context, rule *configstoreTables.TableAlertRule) []string {
	ids := make([]string, 0, len(rule.Channels))
	for _, ch := range rule.Channels {
		if ch.Type == configstoreTables.AlertChannelTypeWebhook && ch.WebhookID != "" {
			ids = append(ids, ch.WebhookID)
		}
	}
	return ids
}

// budgetsToAlertScopes turns a budget row into the (scope_type, scope_id)
// pairs it can match. A VK-owned budget matches team and VK scopes (a team
// admin's rule and the VK's owner rule both apply); a customer-owned
// budget matches customer + global.
type alertScope struct {
	ScopeType string
	ScopeID   string
}

func budgetsToAlertScopes(budget *configstoreTables.TableBudget) []alertScope {
	out := make([]alertScope, 0, 3)
	if budget.VirtualKeyID != nil && *budget.VirtualKeyID != "" {
		out = append(out, alertScope{configstoreTables.AlertScopeVirtualKey, *budget.VirtualKeyID})
	}
	if budget.TeamID != nil && *budget.TeamID != "" {
		out = append(out, alertScope{configstoreTables.AlertScopeTeam, *budget.TeamID})
	}
	if budget.CustomerID != nil && *budget.CustomerID != "" {
		out = append(out, alertScope{configstoreTables.AlertScopeCustomer, *budget.CustomerID})
	}
	if budget.ProviderConfigID != nil {
		// Provider-config budgets don't map 1:1 to a scope type the UI
		// exposes; they fold into the global bucket for alerting purposes.
		out = append(out, alertScope{configstoreTables.AlertScopeGlobal, ""})
	}
	if budget.ModelConfigID != nil && *budget.ModelConfigID != "" {
		out = append(out, alertScope{configstoreTables.AlertScopeModel, *budget.ModelConfigID})
	}
	if len(out) == 0 {
		out = append(out, alertScope{configstoreTables.AlertScopeGlobal, ""})
	}
	return out
}

// ruleMetricMatches limits evaluation to the metrics the inline path can
// compute without an extra store round-trip. error_rate and cache_miss_rate
// need rolling-window sources we don't have on this path; they will go via
// their own sidekiq jobs in P1.
func ruleMetricMatches(rule *configstoreTables.TableAlertRule, scopeType string, _ *configstoreTables.TableBudget) bool {
	switch rule.Metric {
	case configstoreTables.AlertMetricBudgetUsagePercent,
		configstoreTables.AlertMetricBudgetUsageAmount:
		return true
	default:
		return false
	}
}

// eventForMetric maps the rule's metric to the webhook event the dispatch
// will fire. Centralised so the inline path and the async job agree.
func eventForMetric(metric string) string {
	switch metric {
	case configstoreTables.AlertMetricBudgetUsageAmount:
		return string(configstoreTables.WebhookEventAlertBudgetThreshold)
	case configstoreTables.AlertMetricSpendRate:
		return string(configstoreTables.WebhookEventAlertSpendRate)
	case configstoreTables.AlertMetricErrorRate:
		return string(configstoreTables.WebhookEventAlertErrorRate)
	default:
		return string(configstoreTables.WebhookEventAlertBudgetThreshold)
	}
}

// scopeTypeForBudget / scopeIDForBudget reduce a budget to the (type, id)
// pair the budget.exceeded event reports. Mirrors budgetsToAlertScopes
// but with a stable preference order so two events for the same budget
// do not disagree about which scope they announced.
func scopeTypeForBudget(b *configstoreTables.TableBudget) string {
	switch {
	case b.VirtualKeyID != nil && *b.VirtualKeyID != "":
		return configstoreTables.AlertScopeVirtualKey
	case b.TeamID != nil && *b.TeamID != "":
		return configstoreTables.AlertScopeTeam
	case b.CustomerID != nil && *b.CustomerID != "":
		return configstoreTables.AlertScopeCustomer
	default:
		return configstoreTables.AlertScopeGlobal
	}
}

func scopeIDForBudget(b *configstoreTables.TableBudget) string {
	switch {
	case b.VirtualKeyID != nil && *b.VirtualKeyID != "":
		return *b.VirtualKeyID
	case b.TeamID != nil && *b.TeamID != "":
		return *b.TeamID
	case b.CustomerID != nil && *b.CustomerID != "":
		return *b.CustomerID
	default:
		return ""
	}
}
