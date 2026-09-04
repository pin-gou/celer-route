package logstore

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

const (
	cleanupInterval      = 24 * time.Hour
	minJitter            = 15 * time.Minute
	maxJitter            = 30 * time.Minute
	batchSize            = 100
	defaultRetentionDays = 365
)

// LogRetentionManager defines the interface for managing log retention and deletion
type LogRetentionManager interface {
	DeleteLogsBatch(ctx context.Context, cutoff time.Time, batchSize int) (deletedCount int64, err error)
	// StripPayloadsBatch clears the payload content columns of logs older than
	// cutoff that have not been stripped yet. The rows themselves are kept, so
	// list/search/analytics over summary, metadata, and token usage still work.
	// Returns the number of rows stripped.
	StripPayloadsBatch(ctx context.Context, cutoff time.Time, batchSize int) (strippedCount int64, err error)
}

// CleanerConfig holds configuration for the log cleaner
type CleanerConfig struct {
	RetentionDays int
	// PayloadRetentionDays is the age (in days) after which a log's payload
	// content columns are stripped while the row is kept. 0 (or any value below
	// RetentionDays) disables stripping. Must be <= RetentionDays to have an effect.
	PayloadRetentionDays int
}

// LogsCleaner manages the cleanup of old logs
type LogsCleaner struct {
	manager     LogRetentionManager
	config      CleanerConfig
	logger      schemas.Logger
	stopCleanup chan struct{}
	mu          sync.Mutex
}

// NewLogsCleaner creates a new LogsCleaner instance
func NewLogsCleaner(manager LogRetentionManager, config CleanerConfig, logger schemas.Logger) *LogsCleaner {
	return &LogsCleaner{
		manager: manager,
		config:  config,
		logger:  logger,
	}
}

// StartCleanupRoutine starts a goroutine that periodically cleans up old logs
func (c *LogsCleaner) StartCleanupRoutine() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return early if already running
	if c.stopCleanup != nil {
		c.logger.Debug("log cleanup routine already running")
		return
	}

	c.stopCleanup = make(chan struct{})
	stopCh := c.stopCleanup

	go func() {
		// At the beginning, we will cleanup the logs
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		c.cleanupOldLogs(ctx)
		cancel()
		// Calculate initial delay with jitter
		timer := time.NewTimer(calculateNextRunDuration())
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				// Run cleanup
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				c.cleanupOldLogs(ctx)
				cancel()

				// Reset timer with new jitter for next run
				timer.Reset(calculateNextRunDuration())

			case <-stopCh:
				c.logger.Info("log cleanup routine stopped")
				return
			}
		}
	}()
	c.logger.Info("log cleanup routine started")
}

// StopCleanupRoutine gracefully stops the cleanup goroutine
func (c *LogsCleaner) StopCleanupRoutine() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return early if already stopped
	if c.stopCleanup == nil {
		c.logger.Debug("log cleanup routine already stopped")
		return
	}

	close(c.stopCleanup)
	c.stopCleanup = nil
}

// cleanupOldLogs strips payloads older than the payload retention period (when
// configured) and deletes logs older than the retention period in batches.
func (c *LogsCleaner) cleanupOldLogs(ctx context.Context) {
	retentionDays := c.config.RetentionDays
	if retentionDays < 1 {
		retentionDays = defaultRetentionDays
	}

	// Phase 1: strip payload content columns from logs older than the payload
	// retention period, keeping rows (summary/metadata/token usage) for
	// list/search/analytics. Skipped when disabled (0 or >= retention).
	if c.config.PayloadRetentionDays > 0 && c.config.PayloadRetentionDays < retentionDays {
		stripCutoff := time.Now().UTC().AddDate(0, 0, -c.config.PayloadRetentionDays)
		c.logger.Info("starting log payload stripping: stripping payloads older than %s (payload retention: %d days)",
			stripCutoff.Format(time.RFC3339), c.config.PayloadRetentionDays)
		stripped, err := c.stripOldPayloads(ctx, stripCutoff)
		if err != nil {
			c.logger.Error("log payload stripping failed: %v", err)
		}
		if stripped > 0 {
			c.logger.Info("log payload stripping completed: stripped %d logs", stripped)
		}
	}

	// Calculate cutoff time for deletion
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	c.logger.Info("starting log cleanup: deleting logs older than %s (retention: %d days)", cutoff.Format(time.RFC3339), retentionDays)

	totalDeleted := int64(0)
	batchCount := 0

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			c.logger.Warn("log cleanup cancelled: %v", ctx.Err())
			return
		default:
		}

		// Delete logs in batches using the manager
		deleted, err := c.manager.DeleteLogsBatch(ctx, cutoff, batchSize)
		if err != nil {
			c.logger.Error("failed to delete old logs: %v", err)
			return
		}

		if deleted == 0 {
			// No more logs to delete
			break
		}

		totalDeleted += deleted
		batchCount++
		c.logger.Debug("deleted batch %d: %d logs", batchCount, deleted)

		// If we deleted fewer than the batch size, we're done
		if deleted < int64(batchSize) {
			break
		}
	}

	if totalDeleted > 0 {
		c.logger.Info("log cleanup completed: deleted %d logs in %d batches", totalDeleted, batchCount)
	} else {
		c.logger.Debug("log cleanup completed: no old logs to delete")
	}
}

// stripOldPayloads strips payload columns in batches until no more eligible
// rows remain, returning the total number of stripped rows.
func (c *LogsCleaner) stripOldPayloads(ctx context.Context, cutoff time.Time) (int64, error) {
	total := int64(0)
	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}

		stripped, err := c.manager.StripPayloadsBatch(ctx, cutoff, batchSize)
		if err != nil {
			return total, err
		}

		if stripped == 0 {
			break
		}
		total += stripped
		c.logger.Debug("stripped batch: %d logs", stripped)

		if stripped < int64(batchSize) {
			break
		}
	}
	return total, nil
}

// calculateNextRunDuration returns 24 hours plus a random jitter between 15-30 minutes
func calculateNextRunDuration() time.Duration {
	jitter := minJitter + time.Duration(rand.Int63n(int64(maxJitter-minJitter)))
	return cleanupInterval + jitter
}
