package webhooks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
)

// alertEventEnvelope is the JSON body of every alert webhook delivery. It
// is deliberately separate from eventEnvelope (the async-job envelope)
// because alert payloads carry different fields and would force an
// awkward `omitempty` cluster on the existing struct.
//
// Every receiver that handles async_job.* can ignore this shape by gating on
// the event field; receivers that handle alert.* / budget.exceeded should
// only depend on the documented fields below.
type alertEventEnvelope struct {
	Event     string            `json:"event"`
	CreatedAt time.Time         `json:"created_at"`
	Data      alertEventData    `json:"data"`
	Alert     *alertEventDetail `json:"alert,omitempty"`
}

type alertEventData struct {
	// RuleID identifies the firing rule. Receivers use this for dedupe and
	// routing. Stable across re-firings within a cooldown window because
	// the writer short-circuits suppressed events.
	RuleID string `json:"rule_id"`
	// EventID is the alert_events row id, useful for tying the delivery
	// back to the in-app event list.
	EventID     string  `json:"event_id"`
	ScopeType   string  `json:"scope_type"`
	ScopeID     string  `json:"scope_id"`
	Metric      string  `json:"metric"`
	MetricValue float64 `json:"metric_value"`
	Threshold   float64 `json:"threshold"`
	Message     string  `json:"message,omitempty"`
	TriggeredAt time.Time `json:"triggered_at"`
}

// alertEventDetail carries the rule-shaped context the receiver wants for
// routing / labelling (rule name, delivery channels). It is optional and
// omitted for budget.exceeded emissions that are not tied to a user rule.
type alertEventDetail struct {
	Name           string  `json:"name,omitempty"`
	Comparison     string  `json:"comparison,omitempty"`
	CooldownMinutes int    `json:"cooldown_minutes,omitempty"`
}

// RenderAlertPayload builds the delivery body for an alert firing. Pass the
// fired rule and event so the envelope carries stable IDs (RuleID/EventID)
// receivers can dedupe on. The endpoint's IncludeResponse setting does not
// apply: alert events never carry response payloads.
func RenderAlertPayload(rule *tables.TableAlertRule, event *tables.TableAlertEvent, now time.Time) ([]byte, error) {
	if rule == nil || event == nil {
		return nil, fmt.Errorf("render alert payload: rule and event are required")
	}
	envelope := alertEventEnvelope{
		Event:     string(event.Event),
		CreatedAt: now,
		Data: alertEventData{
			RuleID:      rule.ID,
			EventID:     event.ID,
			ScopeType:   event.ScopeType,
			ScopeID:     event.ScopeID,
			Metric:      rule.Metric,
			MetricValue: event.MetricValue,
			Threshold:   event.Threshold,
			Message:     event.Message,
			TriggeredAt: event.TriggeredAt,
		},
		Alert: &alertEventDetail{
			Name:            rule.Name,
			Comparison:      rule.Comparison,
			CooldownMinutes: rule.CooldownMinutes,
		},
	}
	return json.Marshal(envelope)
}

// RenderBudgetExceededPayload builds the delivery body for a 402
// budget.exceeded event. There is no rule here — the event is emitted once
// per hard-rejection regardless of whether an alert rule covers the
// budget — so the envelope omits rule_id and event_id.
func RenderBudgetExceededPayload(scopeType, scopeID string, usedAmount, maxAmount float64, now time.Time) []byte {
	envelope := alertEventEnvelope{
		Event:     string(tables.WebhookEventBudgetExceeded),
		CreatedAt: now,
		Data: alertEventData{
			ScopeType:   scopeType,
			ScopeID:     scopeID,
			Metric:      tables.AlertMetricBudgetUsageAmount,
			MetricValue: usedAmount,
			Threshold:   maxAmount,
			Message:     fmt.Sprintf("Budget exceeded: %.4f / %.4f", usedAmount, maxAmount),
			TriggeredAt: now,
		},
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		// Fall back to a minimal envelope: marshalling this fixed-shape
		// value can only fail if time.Time somehow misbehaves, which would
		// mean the runtime clock is broken; logging the raw error is more
		// useful than swallowing it.
		data = []byte(fmt.Sprintf(`{"event":"budget.exceeded","created_at":%q,"data":{}}`, now.Format(time.RFC3339Nano)))
	}
	return data
}

// NewAlertJobID returns a fresh webhook job id for an alert delivery row.
// Exported so the sidekiq-driven budget.exceeded path (which never goes
// through EnqueueAlertEvent) can mint the id with the same generator as the
// dispatcher does for its async-job enqueues.
func NewAlertJobID() string { return uuid.NewString() }