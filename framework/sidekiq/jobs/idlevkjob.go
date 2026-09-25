// Package jobs - idlevkjob.ts: implements the US24 idle-VK detector.
// Every 30 minutes the job walks the logs to discover which virtual keys
// were touched since the last sweep, then UPDATEs governance_virtual_keys
// .last_used_at in bulk. The /api/reports/idle-keys endpoint reads the
// same column directly: idle = (now - last_used_at) >= threshold, with
// NULL counted as "never used" and surfaced alongside the timed-out rows
// so brand-new keys that have never been called don't silently disappear.
//
// The job does NOT look at provider keys — those have no owner field and
// are out of scope (see temp/team/01-identity/data-model.md §5.1).
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

// IdleVKLogStore is the subset of logstore the idle-VK detector needs.
// We read the log store to find VKs that saw traffic in the trailing
// window; reading the log store (instead of stat'ing every VK) keeps the
// job O(active-VK-since-last-sweep) rather than O(all-VK).
type IdleVKLogStore interface {
	// DistinctVirtualKeyIDsSince returns every distinct virtual_key_id
	// that has at least one log row with timestamp >= since. We use a
	// distinct-only query so the result scales with active VKs, not
	// request count.
	DistinctVirtualKeyIDsSince(ctx context.Context, since time.Time) ([]string, error)
}

// IdleVKConfigStore is the subset of configstore the idle-VK detector
// needs. Splitting the interface keeps the sidekiq job free of the
// governance-specific dependency (the full RDBConfigStore is too wide).
type IdleVKConfigStore interface {
	// TouchVirtualKeyLastUsedAt bulk-updates last_used_at for the given
	// VK ids. The store owns the timestamp it writes (now) so the
	// caller does not have to pass it.
	TouchVirtualKeyLastUsedAt(ctx context.Context, ids []string) (int64, error)
	// ListIdleVirtualKeys returns VKs whose last_used_at is older than
	// the threshold (or NULL — i.e. never used). Used by both the
	// job (to surface the report) and the handler that exposes the
	// report over HTTP.
	ListIdleVirtualKeys(ctx context.Context, threshold time.Time, limit, offset int) ([]tables.TableVirtualKey, int64, error)
}

// IdleVKJob walks the log store on a sidekiq tick and refreshes
// governance_virtual_keys.last_used_at for every VK that received traffic
// since the previous tick. The idle-VK HTTP handler reads the same
// column through IdleVKConfigStore.ListIdleVirtualKeys.
type IdleVKJob struct {
	logStore   IdleVKLogStore
	store      IdleVKConfigStore
	interval   time.Duration
	lastSwept  time.Time
}

// NewIdleVKJob builds the job. interval is the sweep window — defaults
// are applied at the caller when zero is passed so the job can be used
// in tests with a one-second window without ceremony.
func NewIdleVKJob(logStore IdleVKLogStore, store IdleVKConfigStore, interval time.Duration) *IdleVKJob {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &IdleVKJob{logStore: logStore, store: store, interval: interval}
}

// Kind is the sidekiq kind string used at Register and enqueue time.
func (j *IdleVKJob) Kind() string { return "idle_vk_sweep" }

// Handle satisfies sidekiq.HandlerFunc. The job:
//   1. queries DistinctVirtualKeyIDsSince(lastSwept) to learn which VKs
//      received traffic since the previous tick;
//   2. bulk-updates last_used_at via TouchVirtualKeyLastUsedAt;
//   3. advances lastSwept to now so the next tick only sees fresh logs.
//
// Errors from (1) and (2) are returned so sidekiq records the failure
// in the jobs table; the next tick re-runs the same window.
func (j *IdleVKJob) Handle(ctx context.Context, _ tables.TableSidekiqJob, _ sidekiq.ProgressFunc) (string, error) {
	if j.store == nil {
		return "", fmt.Errorf("idle-vk job: store is nil")
	}
	if j.logStore == nil {
		// Without a log store we cannot know which VKs received traffic
		// since the last sweep, so we just advance the cursor and report
		// 0 touched — the next tick will retry once logs come back.
		j.lastSwept = time.Now().UTC()
		metadata, _ := json.Marshal(map[string]any{
			"touched": 0,
			"skipped": "log store not configured",
		})
		return string(metadata), nil
	}
	since := j.lastSwept
	if since.IsZero() {
		since = time.Now().UTC().Add(-j.interval)
	}
	ids, err := j.logStore.DistinctVirtualKeyIDsSince(ctx, since)
	if err != nil {
		return "", fmt.Errorf("idle-vk job: distinct VKs since %s: %w", since.Format(time.RFC3339), err)
	}
	touched, err := j.store.TouchVirtualKeyLastUsedAt(ctx, ids)
	if err != nil {
		return "", fmt.Errorf("idle-vk job: touch last_used_at: %w", err)
	}
	j.lastSwept = time.Now().UTC()
	metadata, _ := json.Marshal(map[string]any{
		"touched":      touched,
		"distinct_ids": len(ids),
		"swept_at":     j.lastSwept.Format(time.RFC3339),
	})
	return string(metadata), nil
}
// EnqueueIdleVKSweep schedules one idle-VK sweep tick on the next
// available sidekiq slot. The handler returns immediately so the periodic
// ticker (in server.go) does not block on the sweep — even though the
// actual scan is fast (single DISTINCT query + bulk UPDATE), it should
// not share the server's request goroutine.
func EnqueueIdleVKSweep(r *sidekiq.Runner, job *IdleVKJob) error {
	if r == nil {
		return fmt.Errorf("idle-vk sweep enqueue: runner is nil")
	}
	if job == nil {
		return fmt.Errorf("idle-vk sweep enqueue: job is nil")
	}
	id := "idle_vk_sweep-" + uuid.NewString()
	return r.Enqueue(context.Background(), id, job.Kind(), "{}", "")
}
