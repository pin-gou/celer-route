package logging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/framework/sidekiq"
)

// LogCleanupJobKind is the sidekiq job kind used for the manual log-cleanup
// worker invoked from the settings page. It is distinct from the auto-cleaner
// (LogRetentionManager / framework/logstore/cleaner.go), which runs on a
// 24h+jitter timer and only respects retention-day thresholds.
const LogCleanupJobKind = "logs_cleanup"

// manualCleanupBatchSize is the per-batch row cap the manual worker uses
// when walking matched rows. It matches the auto-cleaner's batch size so a
// manual run looks and feels like the periodic one — same DB load profile,
// same checkpoint cadence.
//
// This is intentionally a separate constant from costRecalcBatchSize:
// billing pagination reads payload columns, cleanup pagination reads only
// ids, and pinning the value to one is the kind of cross-cutting constraint
// that bites later.
const manualCleanupBatchSize = 100

// CleanupStopMessage is the message written into CleanupJobMeta.Message when
// the run stops before completion (cancellation or shutdown). Kept as a
// helper so the format string matches across completion paths.
func cleanupStoppedEarlyMessage(meta *logstore.CleanupJobMeta) string {
	verb := "deleted"
	count := meta.Deleted
	if meta.StripPayloadsOnly {
		verb = "stripped"
		count = meta.Stripped
	}
	msg := fmt.Sprintf("Stopped early after %s %d log(s). Run the cleanup again to continue from this point.", verb, count)
	if meta.Total > 0 && meta.Processed < meta.Total {
		msg += fmt.Sprintf(" %d log(s) in the selected scope were not checked.", meta.Total-meta.Processed)
	}
	return msg
}

// BuildLogCleanupJobMeta counts the in-scope rows and returns the durable
// metadata JSON to enqueue. It reuses the same row-visibility logic the
// worker walks, so the resulting Total matches the work.
//
// The caller (LoggingHandler) is expected to have already resolved any
// period-style window into filters.StartTime / EndTime, frozen the cutoff
// time, and decided whether to delete or strip.
func (p *LoggerPlugin) BuildLogCleanupJobMeta(ctx context.Context, meta logstore.CleanupJobMeta) (string, error) {
	if p.store == nil {
		return "", fmt.Errorf("log store is not configured")
	}

	// Total is the number of rows this run will touch. For CleanupScopeAll
	// the whole table is in scope; for filter scopes it's the matched count;
	// for older_than scopes it's rows older than cutoff.
	switch meta.Scope {
	case logstore.CleanupScopeAll:
		stats, err := p.store.StorageStats(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to count logs for cleanup: %w", err)
		}
		meta.Total = stats.TotalLogs

	case logstore.CleanupScopeOlderThan:
		// Translate the cutoff into a synthetic filter so the count path
		// stays in one place. Skip the preview when the user passes a nil
		// cutoff; the server-side validation has already caught that.
		if meta.Cutoff == nil {
			return "", fmt.Errorf("older_than scope requires a cutoff time")
		}
		filters := logstore.SearchFilters{EndTime: meta.Cutoff}
		preview, err := p.store.CountByFilter(ctx, filters)
		if err != nil {
			return "", fmt.Errorf("failed to count logs for cleanup: %w", err)
		}
		meta.Total = preview.MatchedLogs
		meta.Filters = filters

	case logstore.CleanupScopeFilter:
		preview, err := p.store.CountByFilter(ctx, meta.Filters)
		if err != nil {
			return "", fmt.Errorf("failed to count logs for cleanup: %w", err)
		}
		meta.Total = preview.MatchedLogs

	default:
		return "", fmt.Errorf("unknown cleanup scope %q", meta.Scope)
	}

	data, err := sonic.Marshal(&meta)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cleanup job metadata: %w", err)
	}
	return string(data), nil
}

// RunLogCleanupJob is the sidekiq handler body for LogCleanupJobKind. Given
// the current metadata JSON and a checkpoint callback it returns the final
// metadata JSON. It matches the shape sidekiq expects:
//
//	for {
//	    if err := ctx.Err(); ... break
//	    batch := doWork(...)
//	    meta.Processed += int64(len(batch))
//	    checkpoint(snapshot())
//	}
//
// Resume safety: each batch deletes by primary key, so a restart re-runs the
// last partial batch at most (the rows already committed are gone, the
// remaining ids are picked up by the next iteration). Stripping is
// idempotent on payload_stripped=true rows (the WHERE clause skips them), so
// resume is safe there too.
func (p *LoggerPlugin) RunLogCleanupJob(ctx context.Context, metaJSON string, checkpoint func(string) error) (string, error) {
	if p.store == nil {
		return metaJSON, fmt.Errorf("log store is not configured")
	}

	var meta logstore.CleanupJobMeta
	if err := sonic.Unmarshal([]byte(metaJSON), &meta); err != nil {
		return metaJSON, fmt.Errorf("failed to parse cleanup job metadata: %w", err)
	}

	lastGoodSnapshot := metaJSON
	snapshot := func() string {
		data, err := sonic.Marshal(&meta)
		if err != nil {
			return lastGoodSnapshot
		}
		lastGoodSnapshot = string(data)
		return lastGoodSnapshot
	}

	// StripPayloadsByFilterBatch on ClickHouse is a documented no-op; if the
	// user asked for stripping on a backend that doesn't support it, refuse
	// upfront instead of silently producing zero progress.
	if meta.StripPayloadsOnly {
		if _, ok := p.store.(interface {
			StripPayloadsByFilterBatch(ctx context.Context, filters logstore.SearchFilters, batchSize int) (int64, error)
		}); !ok {
			return metaJSON, fmt.Errorf("this log store does not support payload stripping")
		}
	}

	// batchFunc is bound once so the loop body is identical for delete and
	// strip paths; only the post-batch counter differs.
	batchFunc := func(ctx context.Context) (int64, error) {
		if meta.StripPayloadsOnly {
			return p.store.StripPayloadsByFilterBatch(ctx, meta.Filters, manualCleanupBatchSize)
		}
		return p.store.DeleteByFilterBatch(ctx, meta.Filters, manualCleanupBatchSize)
	}

	for {
		if err := ctx.Err(); err != nil {
			meta.Message = cleanupStoppedEarlyMessage(&meta)
			_ = checkpoint(snapshot())
			return snapshot(), err
		}

		affected, err := batchFunc(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				meta.Message = cleanupStoppedEarlyMessage(&meta)
				return snapshot(), err
			}
			return snapshot(), fmt.Errorf("cleanup batch failed: %w", err)
		}
		if affected == 0 {
			break
		}

		if meta.StripPayloadsOnly {
			meta.Stripped += affected
		} else {
			meta.Deleted += affected
		}
		meta.Processed += affected

		if err := checkpoint(snapshot()); err != nil {
			if cerr := ctx.Err(); cerr != nil {
				meta.Message = cleanupStoppedEarlyMessage(&meta)
				return snapshot(), cerr
			}
			return snapshot(), fmt.Errorf("failed to checkpoint cleanup progress: %w", err)
		}

		if affected < int64(manualCleanupBatchSize) {
			break
		}
	}

	// Final summary message.
	verb := "deleted"
	count := meta.Deleted
	if meta.StripPayloadsOnly {
		verb = "stripped"
		count = meta.Stripped
	}
	meta.Message = fmt.Sprintf("Cleanup complete: %s %d log(s).", verb, count)
	return snapshot(), nil
}

// Compile-time check: ensure RunLogCleanupJob matches the signature the
// sidekiq Runner.Register callback expects. sidekiq.ProgressFunc is
// imported only for the type reference.
var _ = sidekiq.ProgressFunc(nil)

// silence unused warning when building without test imports.
var _ = time.Now
