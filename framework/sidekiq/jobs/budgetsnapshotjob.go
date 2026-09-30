// Package jobs holds the durable background-job handlers the gateway runs
// on its own sidekiq runner (framework/sidekiq). They are intentionally
// split out of the runner's own file so adding a new background job is
// "drop one file in this directory and Register() it from server.go".
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/sidekiq"
)

// BudgetSnapshotStore is the subset of configstore the budget snapshot
// sampler needs. Splitting it keeps the sidekiq job free of any
// governance-specific dependency and lets tests use a tiny in-memory fake.
type BudgetSnapshotStore interface {
	AllBudgetIDs(ctx context.Context) ([]string, error)
	GetBudgetByID(ctx context.Context, id string) (*tables.TableBudget, error)
	CreateBudgetSnapshot(ctx context.Context, snap *tables.TableBudgetSnapshot) error
}

// BudgetSnapshotJob runs every hour (scheduled via sidekiq timer) and
// writes one budget_snapshots row per budget. The projection endpoint
// fits a line through the trailing samples to compute "expected time to
// exhaustion" (see 02-alerting/data-model.md §3).
//
// Failure modes:
//   - a single budget that errors stops the iteration (logged) but does
//     not mark the job failed; remaining budgets in the same tick still
//     get sampled. A persistently failing budget shows up as missing
//     samples and surfaces in the UI as "no projection available".
//   - the job only ever enqueues one row per budget per tick, so a slow
//     tick cannot flood the snapshots table.
type BudgetSnapshotJob struct {
	store BudgetSnapshotStore
}

// NewBudgetSnapshotJob builds a job; pass it to sidekiq.Runner.Register
// once the runner is constructed.
func NewBudgetSnapshotJob(store BudgetSnapshotStore) *BudgetSnapshotJob {
	return &BudgetSnapshotJob{store: store}
}

// Kind is the sidekiq kind string used at enqueue and Register time.
func (s *BudgetSnapshotJob) Kind() string { return "budget_snapshot" }

// Handle satisfies sidekiq.HandlerFunc. progress is unused: the sampler
// completes in well under the heartbeat interval for typical budget counts.
func (s *BudgetSnapshotJob) Handle(ctx context.Context, _ tables.TableSidekiqJob, _ sidekiq.ProgressFunc) (string, error) {
	if s.store == nil {
		return "", fmt.Errorf("budget snapshot job: store is nil")
	}
	ids, err := s.store.AllBudgetIDs(ctx)
	if err != nil {
		return "", fmt.Errorf("budget snapshot job: list budgets: %w", err)
	}
	sampled := 0
	now := time.Now().UTC()
	for _, id := range ids {
		budget, err := s.store.GetBudgetByID(ctx, id)
		if err != nil {
			// Skip individual failures so a single corrupt row doesn't
			// drop the whole tick. The error is surfaced via the
			// returned metadata so an operator can spot drift.
			continue
		}
		started := budget.LastReset
		windowStart := budget.WindowStart(now)
		snap := &tables.TableBudgetSnapshot{
			BudgetID:    budget.ID,
			UsedAmount:  budget.CurrentUsage,
			MaxAmount:   budget.EffectiveMaxLimit(),
			StartedAt:   &started,
			WindowStart: &windowStart,
			SampledAt:   now,
		}
		if err := s.store.CreateBudgetSnapshot(ctx, snap); err != nil {
			continue
		}
		sampled++
	}
	metadata, _ := json.Marshal(map[string]int{
		"sampled_budgets": sampled,
		"total_budgets":   len(ids),
	})
	return string(metadata), nil
}

// EnqueueBudgetSnapshotNow schedules a snapshot tick to run on the next
// available sidekiq slot. The handler returns immediately so the caller
// (admin "test rule" button, an explicit "force sample" admin action,
// or the periodic ticker) does not block. The id is uuid-derived so
// repeated triggers within the same second all run independently.
func EnqueueBudgetSnapshotNow(r *sidekiq.Runner, store BudgetSnapshotStore) error {
	if r == nil {
		return fmt.Errorf("sidekiq runner is required")
	}
	id := "budget_snapshot-" + uuid.NewString()
	job := NewBudgetSnapshotJob(store)
	r.Register(job.Kind(), job.Handle)
	return r.Enqueue(context.Background(), id, job.Kind(), "{}", "")
}