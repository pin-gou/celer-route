package configstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"gorm.io/gorm"
)

// AlertRulesQueryParams filters the alert-rules list endpoint. Empty fields
// mean "no filter on this dimension"; status / scope_type / metric can each
// narrow independently so the admin UI can answer "all disabled rules" or
// "all spend_rate rules" without round-tripping through the table scan.
type AlertRulesQueryParams struct {
	ScopeType string
	ScopeID   string
	Metric    string
	Status    string
	Search    string
	Limit     int
	Offset    int
}

// AlertEventsQueryParams filters the alert-events list endpoint. As with
// AlertRulesQueryParams, empty fields are open-ended.
type AlertEventsQueryParams struct {
	RuleID    string
	ScopeType string
	ScopeID   string
	Event     string
	Status    string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// CreateAlertRule persists a new alert rule. The id is minted server-side
// if missing so callers do not need to know about the uuid machinery; the
// BeforeSave hook normalises string fields and rejects the cache_miss_rate
// P1 metric, so most validation happens before the row reaches the database.
func (s *RDBConfigStore) CreateAlertRule(ctx context.Context, rule *tables.TableAlertRule) error {
	if rule == nil {
		return errors.New("alert rule is required")
	}
	if strings.TrimSpace(rule.ID) == "" {
		rule.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	return s.DB().WithContext(ctx).Create(rule).Error
}

// GetAlertRuleByID returns the alert rule with the given id, or (nil, nil)
// when no row exists. Soft-deletes are not modelled: deletion is hard, so a
// missing row after a delete is the expected terminal state.
func (s *RDBConfigStore) GetAlertRuleByID(ctx context.Context, id string) (*tables.TableAlertRule, error) {
	var rule tables.TableAlertRule
	if err := s.DB().WithContext(ctx).Where("id = ?", id).First(&rule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// ListAlertRules returns one page of rules filtered by params, along with
// the total match count for pagination UIs. A scope + metric filter pair
// matches the API examples in 02-alerting/api.md §1.
func (s *RDBConfigStore) ListAlertRules(ctx context.Context, params AlertRulesQueryParams) ([]tables.TableAlertRule, int64, error) {
	query := s.DB().WithContext(ctx).Model(&tables.TableAlertRule{})
	if params.ScopeType != "" {
		query = query.Where("scope_type = ?", params.ScopeType)
	}
	if params.ScopeID != "" {
		query = query.Where("scope_id = ?", params.ScopeID)
	}
	if params.Metric != "" {
		query = query.Where("metric = ?", params.Metric)
	}
	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}
	if params.Search != "" {
		needle := "%" + strings.ToLower(params.Search) + "%"
		query = query.Where("LOWER(name) LIKE ?", needle)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	var rules []tables.TableAlertRule
	if err := query.Order("created_at DESC, id ASC").Offset(params.Offset).Limit(params.Limit).Find(&rules).Error; err != nil {
		return nil, 0, err
	}
	return rules, total, nil
}

// UpdateAlertRule saves an existing alert rule. Same as UpdateUser: full
// Save so the admin can edit threshold + cooldown + channels + status in
// one call without per-column plumbing.
func (s *RDBConfigStore) UpdateAlertRule(ctx context.Context, rule *tables.TableAlertRule) error {
	if rule == nil {
		return errors.New("alert rule is required")
	}
	rule.UpdatedAt = time.Now().UTC()
	return s.DB().WithContext(ctx).Save(rule).Error
}

// DeleteAlertRule hard-deletes the rule. alert_events rows keep the
// rule_id but no FK reference is enforced (GORM soft-deps here) — the UI
// shows "(deleted rule)" for orphans.
func (s *RDBConfigStore) DeleteAlertRule(ctx context.Context, id string) error {
	res := s.DB().WithContext(ctx).Where("id = ?", id).Delete(&tables.TableAlertRule{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAlertRulesForScope returns every enabled rule that matches the given
// (scope_type, scope_id) pair, including global rules. The AlertEvaluator
// calls this on every soft-threshold fire — the result is small (typically
// zero rows for most requests) and the query is indexed on (scope_type,
// scope_id) so it stays cheap.
func (s *RDBConfigStore) ListAlertRulesForScope(ctx context.Context, scopeType, scopeID string) ([]tables.TableAlertRule, error) {
	q := s.DB().WithContext(ctx).Model(&tables.TableAlertRule{}).
		Where("status = ?", tables.AlertStatusEnabled)
	if scopeType == "" || scopeType == tables.AlertScopeGlobal {
		q = q.Where("scope_type = ?", tables.AlertScopeGlobal)
	} else {
		q = q.Where(
			"(scope_type = ? AND scope_id = ?) OR scope_type = ?",
			scopeType, scopeID, tables.AlertScopeGlobal,
		)
	}
	var rules []tables.TableAlertRule
	if err := q.Order("created_at ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// CreateAlertEvent records a soft-threshold or async-emitted alert firing.
// The id is minted server-side; the caller supplies everything else
// (rule_id, scope_type/id, metric value, threshold, message). Used inline
// by the AlertEvaluator on soft-threshold fires; the budget.exceeded
// async job also calls this.
func (s *RDBConfigStore) CreateAlertEvent(ctx context.Context, event *tables.TableAlertEvent) error {
	if event == nil {
		return errors.New("alert event is required")
	}
	if strings.TrimSpace(event.ID) == "" {
		event.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if event.TriggeredAt.IsZero() {
		event.TriggeredAt = now
	}
	event.CreatedAt = now
	return s.DB().WithContext(ctx).Create(event).Error
}

// UpdateAlertEventDeliveryStatus marks the recorded event state once the
// webhook enqueue returns. Used by the AlertEvaluator's inline path after
// EnqueueAlertEvent completes so the API list reflects "delivered/skipped"
// promptly. Skipped is used when the cooldown suppressed the enqueue.
//
// We use UpdateColumn (not Update / Save) so GORM skips the row's
// BeforeSave hook. Update / Save would load a freshly-zeroed struct
// (because we only model the column to update) and the validator would
// reject the write on RuleID being empty — a status flip must not
// revalidate immutable fields.
func (s *RDBConfigStore) UpdateAlertEventDeliveryStatus(ctx context.Context, id, status string) error {
	if id == "" || status == "" {
		return errors.New("alert event id and status are required")
	}
	res := s.DB().WithContext(ctx).Model(&tables.TableAlertEvent{}).
		Where("id = ?", id).
		UpdateColumn("delivery_status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAlertEventByID returns a single event by id, or (nil, nil) for not-found.
func (s *RDBConfigStore) GetAlertEventByID(ctx context.Context, id string) (*tables.TableAlertEvent, error) {
	var event tables.TableAlertEvent
	if err := s.DB().WithContext(ctx).Where("id = ?", id).First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &event, nil
}

// ListAlertEvents returns one page of events plus the total match count.
// The list endpoint filters by rule/scope/event/status/time-window; the
// time-window is supplied as StartTime/EndTime so callers (UI) can pass
// a Date.now() range without needing to convert to a string.
func (s *RDBConfigStore) ListAlertEvents(ctx context.Context, params AlertEventsQueryParams) ([]tables.TableAlertEvent, int64, error) {
	query := s.DB().WithContext(ctx).Model(&tables.TableAlertEvent{})
	if params.RuleID != "" {
		query = query.Where("rule_id = ?", params.RuleID)
	}
	if params.ScopeType != "" {
		query = query.Where("scope_type = ?", params.ScopeType)
	}
	if params.ScopeID != "" {
		query = query.Where("scope_id = ?", params.ScopeID)
	}
	if params.Event != "" {
		query = query.Where("event = ?", params.Event)
	}
	if params.Status != "" {
		query = query.Where("delivery_status = ?", params.Status)
	}
	if params.StartTime != nil {
		query = query.Where("triggered_at >= ?", *params.StartTime)
	}
	if params.EndTime != nil {
		query = query.Where("triggered_at < ?", *params.EndTime)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	var events []tables.TableAlertEvent
	if err := query.Order("triggered_at DESC, id DESC").Offset(params.Offset).Limit(params.Limit).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// LatestAlertEventForRule returns the most-recent event for a (rule_id,
// scope_type, scope_id) tuple — used by the cooldown check on the inline
// path so we avoid a second write when one is already in the cooldown
// window. Returns (nil, nil) when no event has fired yet.
func (s *RDBConfigStore) LatestAlertEventForRule(ctx context.Context, ruleID, scopeType, scopeID string, since time.Time) (*tables.TableAlertEvent, error) {
	var event tables.TableAlertEvent
	err := s.DB().WithContext(ctx).
		Where("rule_id = ? AND scope_type = ? AND scope_id = ? AND triggered_at >= ?",
			ruleID, scopeType, scopeID, since).
		Order("triggered_at DESC, id DESC").
		First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// CreateBudgetSnapshot persists one periodic usage sample. The id is
// auto-increment so the caller does not need to mint one. The
// (budget_id, sampled_at) unique index prevents accidental duplicate
// writes if two jobs race on the same budget.
func (s *RDBConfigStore) CreateBudgetSnapshot(ctx context.Context, snap *tables.TableBudgetSnapshot) error {
	if snap == nil {
		return errors.New("budget snapshot is required")
	}
	if snap.SampledAt.IsZero() {
		snap.SampledAt = time.Now().UTC()
	}
	if snap.BudgetID == "" {
		return errors.New("budget snapshot budget_id is required")
	}
	return s.DB().WithContext(ctx).Create(snap).Error
}

// ListBudgetSnapshotsForBudget returns up to limit snapshots for the
// given budget, oldest-first, so the projection endpoint can fit a line
// through the trailing window.
func (s *RDBConfigStore) ListBudgetSnapshotsForBudget(ctx context.Context, budgetID string, limit int) ([]tables.TableBudgetSnapshot, error) {
	if limit <= 0 {
		limit = 200
	}
	var out []tables.TableBudgetSnapshot
	if err := s.DB().WithContext(ctx).
		Where("budget_id = ?", budgetID).
		Order("sampled_at ASC, id ASC").
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// LatestBudgetSnapshot returns the most-recent sample for a budget, or
// (nil, nil) when none exists. Used by the projection endpoint as the
// "current state" term.
func (s *RDBConfigStore) LatestBudgetSnapshot(ctx context.Context, budgetID string) (*tables.TableBudgetSnapshot, error) {
	var snap tables.TableBudgetSnapshot
	err := s.DB().WithContext(ctx).
		Where("budget_id = ?", budgetID).
		Order("sampled_at DESC, id DESC").
		First(&snap).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// AllBudgetIDs returns the set of budget ids present in the budgets table.
// Used by the periodic sampler so we don't have to maintain a "list of
// budgets to sample" separately.
func (s *RDBConfigStore) AllBudgetIDs(ctx context.Context) ([]string, error) {
	var ids []string
	if err := s.DB().WithContext(ctx).
		Model(&tables.TableBudget{}).
		Where("id IS NOT NULL AND id <> ''").
		Distinct("id").
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list budget ids: %w", err)
	}
	return ids, nil
}

// GetBudgetByID exposes the budget row the sampler needs (current usage
// and effective max). The existing resolver uses an in-memory view, so the
// sampler goes around it and reads the durable row.
func (s *RDBConfigStore) GetBudgetByID(ctx context.Context, id string) (*tables.TableBudget, error) {
	var b tables.TableBudget
	if err := s.DB().WithContext(ctx).Where("id = ?", id).First(&b).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &b, nil
}