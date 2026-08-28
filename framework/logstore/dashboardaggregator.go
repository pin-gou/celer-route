package logstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DashboardAggregatorConfig configures the dashboard bucket aggregator.
type DashboardAggregatorConfig struct {
	// Interval is how often the aggregator refreshes the most recent buckets.
	// Default: 1 minute.
	Interval time.Duration
	// BucketSeconds is the size of one aggregation bucket in seconds.
	// Default: 300 (5 minutes).
	BucketSeconds int64
	// LookbackBuckets is the number of trailing buckets refreshed on every tick.
	// Should be large enough to absorb late-arriving log rows. Default: 6.
	LookbackBuckets int
	// Logger receives lifecycle messages. May be nil.
	Logger schemas.Logger
}

// DashboardAggregator periodically rolls the `logs` table up into the
// `dashboard_bucket_metrics` pre-aggregate so dashboard reads don't have to scan
// the full history on every page load.
//
// Refresh strategy: only the trailing LookbackBuckets buckets are recomputed on
// each tick. Older buckets are assumed stable and only re-emitted by an
// explicit Backfill call.
type DashboardAggregator struct {
	db     *gorm.DB
	config DashboardAggregatorConfig
	logger schemas.Logger

	mu        sync.Mutex
	stopOnce  sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
	inFlight  bool
	lastError error
	lastRun   time.Time
}

// NewDashboardAggregator builds an aggregator for the given *gorm.DB.
// The caller owns lifecycle: call Start to spin up the goroutine and Stop to
// shut it down cleanly.
func NewDashboardAggregator(db *gorm.DB, cfg DashboardAggregatorConfig) *DashboardAggregator {
	if cfg.Interval <= 0 {
		cfg.Interval = 1 * time.Minute
	}
	if cfg.BucketSeconds <= 0 {
		cfg.BucketSeconds = 300
	}
	if cfg.LookbackBuckets <= 0 {
		cfg.LookbackBuckets = 6
	}
	return &DashboardAggregator{
		db:      db,
		config:  cfg,
		logger:  cfg.Logger,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		lastRun: time.Now().UTC(),
	}
}

// Start launches the background goroutine. Calling Start more than once is a
// no-op.
func (a *DashboardAggregator) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.doneCh == nil {
		return // already stopped
	}
	select {
	case <-a.doneCh:
		// previously stopped; allow restart by re-initialising channels.
	default:
		if a.stopCh == nil {
			return
		}
	}

	go a.loop()
}

// Stop signals the background goroutine to exit and waits for it to drain.
// Safe to call multiple times.
func (a *DashboardAggregator) Stop() {
	a.stopOnce.Do(func() {
		a.mu.Lock()
		ch := a.stopCh
		done := a.doneCh
		a.stopCh = nil
		a.doneCh = nil
		a.mu.Unlock()
		if ch == nil {
			return
		}
		close(ch)
		if done != nil {
			<-done
		}
	})
}

// LastRun returns the wall-clock time of the most recent successful (or
// attempted) refresh. Used by the freshness check that gates dashboard reads
// against the pre-aggregate.
func (a *DashboardAggregator) LastRun() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastRun
}

// LastError returns the error from the most recent refresh, or nil if the last
// run succeeded. Used by the freshness check that gates dashboard reads.
func (a *DashboardAggregator) LastError() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastError
}

// IsFresh reports whether the aggregator has run within maxAge. A nil
// aggregator (e.g. dashboard-bucket path not enabled) returns false so the
// caller falls back to scanning the raw logs table.
func (a *DashboardAggregator) IsFresh(maxAge time.Duration) bool {
	if a == nil {
		return false
	}
	return time.Since(a.LastRun()) <= maxAge
}

// BucketSeconds returns the configured bucket size so the dashboard read path
// can decide whether a requested bucket size is supported.
func (a *DashboardAggregator) BucketSeconds() int64 {
	if a == nil {
		return 0
	}
	return a.config.BucketSeconds
}

// DB returns the aggregator's underlying *gorm.DB. Exposed so the dashboard
// read path can run bucket SELECTs against the same connection pool.
func (a *DashboardAggregator) DB() *gorm.DB {
	if a == nil {
		return nil
	}
	return a.db
}

// Refresh re-aggregates the trailing LookbackBuckets buckets. Safe to call
// concurrently with the background loop; Refresh serialises via inFlight.
func (a *DashboardAggregator) Refresh(ctx context.Context) error {
	if a.db == nil {
		return fmt.Errorf("dashboard aggregator: nil db")
	}
	a.mu.Lock()
	if a.inFlight {
		a.mu.Unlock()
		return nil
	}
	a.inFlight = true
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.inFlight = false
		a.mu.Unlock()
	}()

	if err := a.refreshLocked(ctx); err != nil {
		a.mu.Lock()
		a.lastError = err
		a.mu.Unlock()
		return err
	}

	a.mu.Lock()
	a.lastRun = time.Now().UTC()
	a.lastError = nil
	a.mu.Unlock()
	return nil
}

// Backfill re-aggregates every bucket from `since` up to now. Use it once on
// first deployment to populate the pre-aggregate from existing log history.
func (a *DashboardAggregator) Backfill(ctx context.Context, since time.Time) error {
	if err := a.aggregateRange(ctx, since, time.Now().UTC()); err != nil {
		return fmt.Errorf("dashboard aggregator backfill: %w", err)
	}
	return nil
}

func (a *DashboardAggregator) loop() {
	defer close(a.doneCh)

	// Run once immediately so a fresh deployment has data within seconds, not
	// after the first Interval tick.
	if err := a.Refresh(context.Background()); err != nil && a.logger != nil {
		a.logger.Warn("[dashboard-aggregator] initial refresh failed: %v", err)
	}

	ticker := time.NewTicker(a.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.Refresh(context.Background()); err != nil && a.logger != nil {
				a.logger.Warn("[dashboard-aggregator] refresh failed: %v", err)
			}
		case <-a.stopCh:
			return
		}
	}
}

// refreshLocked aggregates the trailing LookbackBuckets buckets.
func (a *DashboardAggregator) refreshLocked(ctx context.Context) error {
	now := time.Now().UTC()
	latestBucketStart := alignBucket(now, a.config.BucketSeconds)
	earliestBucketStart := latestBucketStart.Add(-time.Duration(int64(a.config.LookbackBuckets)*a.config.BucketSeconds) * time.Second)

	if err := a.aggregateRange(ctx, earliestBucketStart, now); err != nil {
		return fmt.Errorf("trailing refresh: %w", err)
	}
	return nil
}

// aggregateRange recomputes every bucket in [since, until). The window must be
// aligned to BucketSeconds; otherwise the aggregation skips rows that straddle
// two buckets. Caller is responsible for picking an aligned start.
func (a *DashboardAggregator) aggregateRange(ctx context.Context, since, until time.Time) error {
	if !since.Before(until) {
		return nil
	}

	dialect := a.db.Dialector.Name()

	// Build the aggregation query. We bucket on UTC-aligned timestamps:
	//   bucket_start = (timestamp_unix / BucketSeconds) * BucketSeconds
	// We use FLOOR(... / BucketSeconds) * BucketSeconds via integer arithmetic
	// in SQL so it's portable between SQLite and PostgreSQL.
	bucketExpr := fmt.Sprintf("(CAST(strftime('%%s', timestamp) AS INTEGER) / %d) * %d", a.config.BucketSeconds, a.config.BucketSeconds)
	if dialect == "postgres" {
		bucketExpr = fmt.Sprintf("(EXTRACT(EPOCH FROM timestamp)::BIGINT / %d) * %d", a.config.BucketSeconds, a.config.BucketSeconds)
	}

	selectSQL := fmt.Sprintf(`
		%s AS bucket_start_unix,
		%d AS bucket_seconds,
		provider,
		model,
		object_type AS object_type,
		status,
		COUNT(*) AS request_count,
		SUM(COALESCE(prompt_tokens, 0)) AS prompt_tokens,
		SUM(COALESCE(completion_tokens, 0)) AS completion_tokens,
		SUM(COALESCE(total_tokens, 0)) AS total_tokens,
		SUM(COALESCE(cost, 0)) AS cost,
		SUM(CASE WHEN status = 'success' THEN COALESCE(latency, 0) ELSE 0 END) AS total_latency_ms,
		SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS latency_count
	`, bucketExpr, a.config.BucketSeconds)

	type row struct {
		BucketStartUnix int64   `gorm:"column:bucket_start_unix"`
		BucketSeconds   int64   `gorm:"column:bucket_seconds"`
		Provider        string  `gorm:"column:provider"`
		Model           string  `gorm:"column:model"`
		ObjectType      string  `gorm:"column:object_type"`
		Status          string  `gorm:"column:status"`
		RequestCount    int64   `gorm:"column:request_count"`
		PromptTokens    int64   `gorm:"column:prompt_tokens"`
		CompletionTokens int64  `gorm:"column:completion_tokens"`
		TotalTokens     int64   `gorm:"column:total_tokens"`
		Cost            float64 `gorm:"column:cost"`
		TotalLatencyMS  float64 `gorm:"column:total_latency_ms"`
		LatencyCount    int64   `gorm:"column:latency_count"`
	}

	var rows []row
	err := a.db.WithContext(ctx).
		Model(&Log{}).
		Select(selectSQL).
		Where("timestamp >= ? AND timestamp < ?", since, until).
		Where("status IN ?", terminalLogStatuses).
		Group("bucket_start_unix").
		Group("provider").
		Group("model").
		Group("object_type").
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("aggregate scan: %w", err)
	}

	if len(rows) == 0 {
		return nil
	}

	metrics := make([]DashboardBucketMetric, 0, len(rows))
	for _, r := range rows {
		metrics = append(metrics, DashboardBucketMetric{
			BucketStart:      time.Unix(r.BucketStartUnix, 0).UTC(),
			BucketSeconds:    r.BucketSeconds,
			Provider:         r.Provider,
			Model:            r.Model,
			ObjectType:       r.ObjectType,
			Status:           r.Status,
			RequestCount:     r.RequestCount,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalTokens:      r.TotalTokens,
			Cost:             r.Cost,
			TotalLatencyMS:   r.TotalLatencyMS,
			LatencyCount:     r.LatencyCount,
		})
	}

	// Upsert. The unique key (bucket_start, bucket_seconds, provider, model,
	// object_type, status) is declared in DashboardBucketMetric's gorm tags; we
	// pass the same column list to ON CONFLICT so SQLite and Postgres agree.
	if err := a.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "bucket_start"},
			{Name: "bucket_seconds"},
			{Name: "provider"},
			{Name: "model"},
			{Name: "object_type"},
			{Name: "status"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"request_count":      gorm.Expr("excluded.request_count"),
			"prompt_tokens":      gorm.Expr("excluded.prompt_tokens"),
			"completion_tokens":  gorm.Expr("excluded.completion_tokens"),
			"total_tokens":       gorm.Expr("excluded.total_tokens"),
			"cost":               gorm.Expr("excluded.cost"),
			"total_latency_ms":   gorm.Expr("excluded.total_latency_ms"),
			"latency_count":      gorm.Expr("excluded.latency_count"),
			"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(&metrics).Error; err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	return nil
}

// alignBucket truncates t down to the nearest BucketSeconds boundary in UTC.
// Buckets are aligned to UTC so that the pre-aggregate is consistent across
// timezone shifts.
func alignBucket(t time.Time, bucketSeconds int64) time.Time {
	t = t.UTC()
	unix := t.Unix()
	aligned := (unix / bucketSeconds) * bucketSeconds
	return time.Unix(aligned, 0).UTC()
}