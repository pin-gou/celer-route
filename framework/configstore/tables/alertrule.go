package tables

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

// AlertScope identifies which dimension an alert rule applies to. It mirrors
// the five governance budget owners plus a "global" catch-all and a "user"
// dimension for member-scoped thresholds. Models are intentionally out of
// scope here; per-model alerting lives on the standard_prices +
// team_pricing_profiles layer (US17 cost projection) and would re-enter via
// budget_usage_amount on the model's owning budget.
const (
	AlertScopeGlobal     = "global"
	AlertScopeTeam       = "team"
	AlertScopeVirtualKey = "virtual_key"
	AlertScopeCustomer   = "customer"
	AlertScopeUser       = "user"
	AlertScopeModel      = "model"
)

// ValidAlertScopes is the canonical scope set accepted by alert rules. Used
// by BeforeSave validation and by the API layer for filter parsing.
var ValidAlertScopes = []string{
	AlertScopeGlobal,
	AlertScopeTeam,
	AlertScopeVirtualKey,
	AlertScopeCustomer,
	AlertScopeUser,
	AlertScopeModel,
}

// AlertMetric enumerates the metric axis the rule evaluates. The first three
// are P0 (live); cache_miss_rate is P1 and rejected at create-time per
// 02-alerting/data-model.md §1 — the UI hides it and the backend returns 422
// so callers learn early.
const (
	AlertMetricBudgetUsagePercent = "budget_usage_percent"
	AlertMetricBudgetUsageAmount  = "budget_usage_amount"
	AlertMetricSpendRate          = "spend_rate"
	AlertMetricErrorRate          = "error_rate"
	AlertMetricCacheMissRate      = "cache_miss_rate"
)

// ValidAlertMetrics is the canonical metric set; AlertMetricCacheMissRate is
// present so the enum surface is complete, but BeforeSave refuses to persist
// a rule with it until 04-security CacheStatsTracker lands.
var ValidAlertMetrics = []string{
	AlertMetricBudgetUsagePercent,
	AlertMetricBudgetUsageAmount,
	AlertMetricSpendRate,
	AlertMetricErrorRate,
	AlertMetricCacheMissRate,
}

// IsAlertMetricP1Only reports whether the metric is reserved for the P1
// phase. Phase-3 callers MUST refuse persistence with these metrics so the
// UI never sees a half-built rule.
func IsAlertMetricP1Only(metric string) bool {
	return metric == AlertMetricCacheMissRate
}

// AlertComparison is the threshold direction. "lte" is supplied so future
// "spend dropped below floor" style rules have a clean expression; the
// default at the API layer is "gte".
const (
	AlertComparisonGTE = "gte"
	AlertComparisonLTE = "lte"
)

// AlertStatus drives the rule's enabled/disabled toggle. Disabled rules keep
// their event history but skip evaluation.
const (
	AlertStatusEnabled  = "enabled"
	AlertStatusDisabled = "disabled"
)

// AlertChannelType is the channel shape that receives the notification. Only
// webhook is implemented in P0; slack / smtp are reserved for a future phase
// but their union shape is declared now so the JSON column stays stable.
const (
	AlertChannelTypeWebhook = "webhook"
	AlertChannelTypeSlack   = "slack"
	AlertChannelTypeSMTP    = "smtp"
)

// AlertChannel is one delivery destination attached to a rule. webhook_id is
// only meaningful for type=webhook; slack url / smtp fields are reserved
// for the corresponding channel types so the JSON shape does not change
// when they go live.
type AlertChannel struct {
	Type      string `json:"type"`
	WebhookID string `json:"webhook_id,omitempty"`
	URL       string `json:"url,omitempty"`
	// SMTP / Slack-specific knobs are reserved (omitempty) so existing rows
	// don't see a JSON shape change when the corresponding channels land.
	Recipients []string               `json:"recipients,omitempty"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

// TableAlertRule defines one reusable alert: a scope + metric + threshold
// + delivery channel list. Cooldown is per (rule_id, scope_type, scope_id):
// when a soft-threshold event fires, subsequent calls in the same cooldown
// window skip the write/webhook enqueue but still let the request through.
//
// Channels are persisted as a JSON blob rather than a join table because the
// set is small (typically 1-3) and the shape varies by channel type.
type TableAlertRule struct {
	ID              string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name            string    `gorm:"type:varchar(255);not null" json:"name"`
	ScopeType       string    `gorm:"type:varchar(32);not null;index:idx_alert_rules_scope" json:"scope_type"`
	ScopeID         string    `gorm:"type:varchar(255);not null;default:'';index:idx_alert_rules_scope" json:"scope_id"`
	Metric          string    `gorm:"type:varchar(64);not null" json:"metric"`
	Threshold       float64   `gorm:"type:decimal(20,6);not null" json:"threshold"`
	Comparison      string    `gorm:"type:varchar(8);not null;default:'gte'" json:"comparison"`
	CooldownMinutes int       `gorm:"not null;default:60" json:"cooldown_minutes"`
	Status          string    `gorm:"type:varchar(16);not null;default:'enabled'" json:"status"`
	ChannelsJSON    string    `gorm:"type:text;not null;default:'[]'" json:"-"`
	Channels        []AlertChannel `gorm:"-" json:"channels"`
	CreatedAt       time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt       time.Time `gorm:"index;not null" json:"updated_at"`
}

// TableName sets the backing table.
func (TableAlertRule) TableName() string { return "alert_rules" }

// CooldownDuration returns the rule's cooldown window as a time.Duration,
// clamping zero/negative values to one minute so a misconfigured rule
// cannot become a tight firehose.
func (r *TableAlertRule) CooldownDuration() time.Duration {
	mins := r.CooldownMinutes
	if mins <= 0 {
		mins = 60
	}
	return time.Duration(mins) * time.Minute
}

// ShouldFire reports whether the rule's comparison direction agrees with the
// supplied metric value. Centralised so the evaluator's soft-threshold
// branch and the budget-exceeded branch share one definition of "fired".
func (r *TableAlertRule) ShouldFire(value float64) bool {
	if r == nil {
		return false
	}
	switch r.Comparison {
	case AlertComparisonLTE:
		return value <= r.Threshold
	default:
		return value >= r.Threshold
	}
}

// BeforeSave validates scope / metric / status / comparison, enforces the
// P1 (cache_miss_rate) block, and serializes Channels into the JSON column.
// Normalisation matches the existing migration safety rules: trim + lowercase
// is enough; everything stricter lives at the handler layer.
func (r *TableAlertRule) BeforeSave(tx *gorm.DB) error {
	r.Name = strings.TrimSpace(r.Name)
	r.ScopeType = strings.ToLower(strings.TrimSpace(r.ScopeType))
	r.ScopeID = strings.TrimSpace(r.ScopeID)
	r.Metric = strings.ToLower(strings.TrimSpace(r.Metric))
	r.Comparison = strings.ToLower(strings.TrimSpace(r.Comparison))
	r.Status = strings.ToLower(strings.TrimSpace(r.Status))
	if r.Status == "" {
		r.Status = AlertStatusEnabled
	}
	if r.Comparison == "" {
		r.Comparison = AlertComparisonGTE
	}
	if r.Name == "" {
		return errors.New("alert rule name cannot be empty")
	}
	if !containsString(ValidAlertScopes, r.ScopeType) {
		return fmt.Errorf("alert rule scope_type %q is not supported", r.ScopeType)
	}
	if r.ScopeType != AlertScopeGlobal && strings.TrimSpace(r.ScopeID) == "" {
		return fmt.Errorf("alert rule scope_id is required for scope_type=%s", r.ScopeType)
	}
	if r.ScopeType == AlertScopeGlobal {
		r.ScopeID = ""
	}
	if !containsString(ValidAlertMetrics, r.Metric) {
		return fmt.Errorf("alert rule metric %q is not supported", r.Metric)
	}
	if IsAlertMetricP1Only(r.Metric) {
		return fmt.Errorf("alert rule metric %q is reserved for a future release (P1)", r.Metric)
	}
	switch r.Comparison {
	case AlertComparisonGTE, AlertComparisonLTE:
	default:
		return fmt.Errorf("alert rule comparison %q is not supported", r.Comparison)
	}
	if math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) {
		return errors.New("alert rule threshold must be finite")
	}
	if r.CooldownMinutes < 0 {
		return errors.New("alert rule cooldown_minutes cannot be negative")
	}
	switch r.Status {
	case AlertStatusEnabled, AlertStatusDisabled:
	default:
		return fmt.Errorf("alert rule status %q is not supported", r.Status)
	}
	for _, ch := range r.Channels {
		switch ch.Type {
		case AlertChannelTypeWebhook:
			if strings.TrimSpace(ch.WebhookID) == "" {
				return errors.New("alert channel of type webhook must include webhook_id")
			}
		default:
			return fmt.Errorf("alert channel type %q is not implemented in this release", ch.Type)
		}
	}
	data, err := json.Marshal(r.Channels)
	if err != nil {
		return fmt.Errorf("failed to encode alert channels: %w", err)
	}
	if data == nil {
		data = []byte("[]")
	}
	r.ChannelsJSON = string(data)
	return nil
}

// AfterFind repopulates Channels from the JSON column so callers reading
// the rule through the store see a single struct.
func (r *TableAlertRule) AfterFind(tx *gorm.DB) error {
	if r.ChannelsJSON == "" {
		r.Channels = nil
		return nil
	}
	var channels []AlertChannel
	if err := json.Unmarshal([]byte(r.ChannelsJSON), &channels); err != nil {
		return fmt.Errorf("failed to decode alert channels: %w", err)
	}
	if channels == nil {
		channels = []AlertChannel{}
	}
	r.Channels = channels
	return nil
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}