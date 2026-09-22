package tables

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// AlertEventDeliveryStatus mirrors the webhook delivery outcomes so an event
// row can answer "did this fire arrive at its target" without joining the
// delivery history table. The "skipped" value is for cooldown-suppressed
// soft-threshold hits where we still want a row (helps rate-of-fire reports)
// but no actual delivery was attempted.
const (
	AlertEventDeliveryStatusPending   = "pending"
	AlertEventDeliveryStatusDelivered = "delivered"
	AlertEventDeliveryStatusFailed    = "failed"
	AlertEventDeliveryStatusSkipped   = "skipped"
)

// TableAlertEvent is one rule firing: a snapshot of the metric value at
// trigger time plus the scope/type pair copied off the rule so background
// queries (filter by scope_type / scope_id without a rule join) stay cheap.
//
// webhook_delivery_id is nullable: a soft-threshold event written via the
// inline path has no per-channel delivery row at insert time (those are
// produced lazily by the webhook enqueue + delivery worker), and a
// budget.exceeded event emitted by the async job may or may not have one
// depending on whether any channel succeeded.
type TableAlertEvent struct {
	ID                string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	RuleID            string    `gorm:"type:varchar(36);not null;index:idx_alert_events_rule" json:"rule_id"`
	ScopeType         string    `gorm:"type:varchar(32);not null;index:idx_alert_events_scope" json:"scope_type"`
	ScopeID           string    `gorm:"type:varchar(255);not null;default:'';index:idx_alert_events_scope" json:"scope_id"`
	Event             string    `gorm:"type:varchar(64);not null;index:idx_alert_events_event" json:"event"`
	MetricValue       float64   `gorm:"type:decimal(20,6);not null" json:"metric_value"`
	Threshold         float64   `gorm:"type:decimal(20,6);not null" json:"threshold"`
	Message           string    `gorm:"type:text" json:"message"`
	DeliveryStatus    string    `gorm:"type:varchar(16);not null;default:'pending'" json:"delivery_status"`
	WebhookDeliveryID string    `gorm:"type:varchar(36);null;index" json:"webhook_delivery_id,omitempty"`
	TriggeredAt       time.Time `gorm:"index;not null" json:"triggered_at"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
}

// TableName sets the backing table.
func (TableAlertEvent) TableName() string { return "alert_events" }

// BeforeSave normalises string fields and refuses nonsense statuses so the
// row never lands in a "neither fish nor fowl" delivery state. The trigger
// timestamp and audit fields are stamped by the caller; we never overwrite
// them here because the in-memory struct may carry values the database
// cannot reconstruct (e.g. triggered_at before Now() on a backfill).
func (e *TableAlertEvent) BeforeSave(_ *gorm.DB) error {
	e.RuleID = strings.TrimSpace(e.RuleID)
	e.ScopeType = strings.ToLower(strings.TrimSpace(e.ScopeType))
	e.Event = strings.ToLower(strings.TrimSpace(e.Event))
	if e.DeliveryStatus == "" {
		e.DeliveryStatus = AlertEventDeliveryStatusPending
	}
	if e.RuleID == "" {
		return errors.New("alert event rule_id cannot be empty")
	}
	if e.Event == "" {
		return errors.New("alert event name cannot be empty")
	}
	switch e.DeliveryStatus {
	case AlertEventDeliveryStatusPending,
		AlertEventDeliveryStatusDelivered,
		AlertEventDeliveryStatusFailed,
		AlertEventDeliveryStatusSkipped:
	default:
		return errors.New("alert event delivery_status is not one of pending/delivered/failed/skipped")
	}
	return nil
}