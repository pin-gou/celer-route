package tables

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// TableBudgetSnapshot is one periodic sample of a budget's usage, written
// by the budget_snapshot_job (framework/sidekiq/jobs/budgetsnapshotjob.go)
// once an hour. The projection endpoint fits a line through the last N rows
// for a budget and computes "expected time to exhaustion"; the linear-fit
// math lives in plugins/governance/alert_evaluator.go rather than here, so
// this struct stays a plain snapshot.
//
// bigint id (auto-increment) is intentional: writes happen at fixed cadence
// (one row per budget per hour) and the table grows unboundedly between
// retention sweeps, so the index/b-tree density from auto-increment beats
// the UUID footprint for the (budget_id, sampled_at) lookup pattern.
type TableBudgetSnapshot struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BudgetID     string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_budget_snapshots_budget_sampled,priority:1" json:"budget_id"`
	UsedAmount   float64   `gorm:"type:decimal(20,6);not null" json:"used_amount"`
	MaxAmount    float64   `gorm:"type:decimal(20,6);not null" json:"max_amount"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	WindowStart  *time.Time `json:"window_start,omitempty"`
	SampledAt    time.Time `gorm:"not null;uniqueIndex:idx_budget_snapshots_budget_sampled,priority:2" json:"sampled_at"`
}

// TableName sets the backing table.
func (TableBudgetSnapshot) TableName() string { return "budget_snapshots" }

// UsagePercent returns the percentage of the budget's effective max that
// has been spent at sampling time. Used by the projection endpoint to
// classify risk (low / medium / high) without re-running the cost math.
func (s *TableBudgetSnapshot) UsagePercent() float64 {
	if s == nil || s.MaxAmount <= 0 {
		return 0
	}
	return s.UsedAmount / s.MaxAmount * 100
}

// BeforeSave rejects negative amounts and missing sample times. The sample
// time is auto-stamped by GORM on create only when zero, so a backfill
// (e.g. seed test rows) can supply its own.
func (s *TableBudgetSnapshot) BeforeSave(_ *gorm.DB) error {
	if s == nil {
		return errors.New("budget snapshot cannot be nil")
	}
	if s.BudgetID == "" {
		return errors.New("budget snapshot budget_id cannot be empty")
	}
	if s.MaxAmount < 0 || s.UsedAmount < 0 {
		return errors.New("budget snapshot amounts cannot be negative")
	}
	if s.SampledAt.IsZero() {
		s.SampledAt = time.Now().UTC()
	}
	return nil
}