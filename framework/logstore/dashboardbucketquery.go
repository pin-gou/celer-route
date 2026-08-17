package logstore

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// DashboardBucketReader serves the dashboard's histogram / stats queries off
// the dashboard_bucket_metrics pre-aggregate. It folds the fixed-size
// aggregator buckets into the bucket size the caller asks for, so the read
// path is independent of which bucket size the user picked in the UI.
type DashboardBucketReader struct {
	db            *gorm.DB
	bucketSeconds int64
	freshness     func() bool
}

// DashboardBucketReaderConfig configures the reader.
type DashboardBucketReaderConfig struct {
	// AggregatorBucketSeconds must match the aggregator's BucketSeconds so
	// the read path can fold buckets correctly. Different values are
	// treated as a config mismatch and the reader returns nil.
	AggregatorBucketSeconds int64
	// FreshnessMaxAge is how stale the aggregator may be before we treat
	// its output as unfit. Zero means "always assume fresh" (tests only).
	FreshnessMaxAge time.Duration
	// IsFresh is the live freshness check — usually a closure over the
	// aggregator's IsFresh method. nil means "always assume fresh".
	IsFresh func() bool
}

// NewDashboardBucketReader builds a reader pinned to the given *gorm.DB and
// aggregator bucket size.
func NewDashboardBucketReader(db *gorm.DB, cfg DashboardBucketReaderConfig) *DashboardBucketReader {
	if db == nil || cfg.AggregatorBucketSeconds <= 0 {
		return nil
	}
	if cfg.IsFresh == nil {
		cfg.IsFresh = func() bool { return true }
	}
	return &DashboardBucketReader{
		db:            db,
		bucketSeconds: cfg.AggregatorBucketSeconds,
		freshness:     cfg.IsFresh,
	}
}

// Fresh reports whether the aggregator has caught up. False → caller must
// fall back to the raw logs path.
func (r *DashboardBucketReader) Fresh() bool {
	if r == nil || r.freshness == nil {
		return false
	}
	return r.freshness()
}

// SupportsBucketSize reports whether the reader can serve a query that asks
// for buckets of bucketSizeSeconds. The reader can fold its fixed
// aggregator_bucket rows into any larger integer multiple, but cannot split
// them into smaller buckets.
func (r *DashboardBucketReader) SupportsBucketSize(bucketSizeSeconds int64) bool {
	if r == nil || r.bucketSeconds <= 0 || bucketSizeSeconds <= 0 {
		return false
	}
	if bucketSizeSeconds < r.bucketSeconds {
		return false
	}
	return bucketSizeSeconds%r.bucketSeconds == 0
}

// SupportsFilters returns true when every active filter can be evaluated by
// the bucket SELECTs. Filters the reader can't honour (content_search,
// metadata filters, missing_cost_only, latency/token/cost ranges, etc.)
// fall through to the raw logs path.
func (r *DashboardBucketReader) SupportsFilters(filters SearchFilters) bool {
	if filters.ContentSearch != "" {
		return false
	}
	if len(filters.MetadataFilters) > 0 {
		return false
	}
	if filters.MissingCostOnly {
		return false
	}
	if filters.MinLatency != nil || filters.MaxLatency != nil {
		return false
	}
	if filters.MinTokens != nil || filters.MaxTokens != nil {
		return false
	}
	if filters.MinCost != nil || filters.MaxCost != nil {
		return false
	}
	if filters.RootsOnly {
		return false
	}
	if filters.ParentRequestID != "" {
		return false
	}
	if len(filters.SelectedKeyIDs) > 0 {
		return false
	}
	if len(filters.VirtualKeyIDs) > 0 {
		return false
	}
	if len(filters.RoutingRuleIDs) > 0 {
		return false
	}
	if len(filters.TeamIDs) > 0 {
		return false
	}
	if len(filters.CustomerIDs) > 0 {
		return false
	}
	if len(filters.UserIDs) > 0 {
		return false
	}
	if len(filters.BusinessUnitIDs) > 0 {
		return false
	}
	if len(filters.RoutingEngineUsed) > 0 {
		return false
	}
	if len(filters.UserAgents) > 0 {
		return false
	}
	if len(filters.Apps) > 0 {
		return false
	}
	if len(filters.StopReasons) > 0 {
		return false
	}
	if len(filters.Aliases) > 0 {
		return false
	}
	if len(filters.CacheHitTypes) > 0 {
		return false
	}
	return true
}

// GetStats returns the dashboard's "Overview" stats card. The raw logs path
// stays the source of truth; this is the cheap equivalent for callers whose
// filters the bucket table can satisfy.
func (r *DashboardBucketReader) GetStats(ctx context.Context, filters SearchFilters) (*SearchStats, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) {
		return nil, errAggregatorStale
	}

	var rows []statsRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		status,
		SUM(request_count) AS request_count,
		SUM(prompt_tokens) AS prompt_tokens,
		SUM(completion_tokens) AS complete_tokens,
		SUM(total_tokens) AS total_tokens,
		SUM(cost) AS cost,
		SUM(total_latency_ms) AS total_latency,
		SUM(latency_count) AS latency_count
	`).Group("status").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket stats scan: %w", err)
	}

	var (
		totalCount    int64
		completed     int64
		successCount  int64
		successTokens int64
		promptTokens  int64
		completeToks  int64
		totalCost     float64
		latencySum    float64
		latencyCount  int64
	)
	for _, row := range rows {
		totalCount += row.RequestCount
		completed += row.RequestCount
		switch row.Status {
		case "success":
			successCount += row.RequestCount
			latencySum += row.TotalLatency
			latencyCount += row.LatencyCount
		}
		successTokens += row.TotalTokens
		promptTokens += row.PromptTokens
		completeToks += row.CompleteTokens
		totalCost += row.Cost
	}

	stats := &SearchStats{TotalRequests: totalCount}
	stats.CacheHitRateTotalRequests = &completed

	if completed > 0 {
		stats.SuccessRate = float64(successCount) / float64(completed) * 100
		if latencyCount > 0 {
			stats.AverageLatency = latencySum / float64(latencyCount)
		}
		stats.TotalTokens = successTokens
		stats.PromptTokens = promptTokens
		stats.CompletionTokens = completeToks
		stats.TotalCost = totalCost
	}

	return stats, nil
}

// GetHistogram returns a request-count histogram, bucketed at the user-requested
// size (folded in memory from the aggregator's smaller buckets).
func (r *DashboardBucketReader) GetHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*HistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []histogramBucketRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select("bucket_start, status, SUM(request_count) AS request_count").
		Group("bucket_start, status").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket histogram scan: %w", err)
	}

	buckets := foldHistogramBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &HistogramResult{Buckets: buckets, BucketSizeSeconds: bucketSizeSeconds}, nil
}

// GetTokenHistogram returns a per-bucket token-usage histogram.
func (r *DashboardBucketReader) GetTokenHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*TokenHistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []tokenHistogramRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		bucket_start,
		SUM(prompt_tokens) AS prompt_tokens,
		SUM(completion_tokens) AS completion_tokens,
		SUM(total_tokens) AS total_tokens
	`).Group("bucket_start").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket token histogram scan: %w", err)
	}

	buckets := foldTokenBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &TokenHistogramResult{Buckets: buckets, BucketSizeSeconds: bucketSizeSeconds}, nil
}

// GetCostHistogram returns a per-bucket cost histogram, optionally broken down
// by model.
func (r *DashboardBucketReader) GetCostHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*CostHistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []costHistogramRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		bucket_start,
		model,
		SUM(cost) AS cost
	`).Group("bucket_start, model").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket cost histogram scan: %w", err)
	}

	buckets, models := foldCostBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &CostHistogramResult{
		Buckets:           buckets,
		BucketSizeSeconds: bucketSizeSeconds,
		Models:            models,
	}, nil
}

// GetLatencyHistogram returns per-bucket avg latency. p90/p95/p99 are not
// pre-aggregable from sums alone — they stay at zero and the UI shows only
// the avg line.
func (r *DashboardBucketReader) GetLatencyHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*LatencyHistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []latencyHistogramRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		bucket_start,
		SUM(total_latency_ms) AS total_latency_ms,
		SUM(latency_count) AS latency_count,
		SUM(request_count) AS request_count
	`).Group("bucket_start").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket latency histogram scan: %w", err)
	}

	buckets := foldLatencyBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &LatencyHistogramResult{Buckets: buckets, BucketSizeSeconds: bucketSizeSeconds}, nil
}

// GetThroughputHistogram returns per-bucket tokens/sec.
func (r *DashboardBucketReader) GetThroughputHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*ThroughputHistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []throughputHistogramRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		bucket_start,
		SUM(total_latency_ms) AS total_latency_ms,
		SUM(latency_count) AS latency_count,
		SUM(request_count) AS request_count,
		SUM(completion_tokens) AS completion_tokens
	`).Group("bucket_start").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket throughput histogram scan: %w", err)
	}

	buckets := foldThroughputBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &ThroughputHistogramResult{Buckets: buckets, BucketSizeSeconds: bucketSizeSeconds}, nil
}

// GetModelHistogram returns a per-model stacked histogram.
func (r *DashboardBucketReader) GetModelHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*ModelHistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []modelHistogramRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		bucket_start,
		model,
		status,
		SUM(request_count) AS request_count
	`).Group("bucket_start, model, status").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket model histogram scan: %w", err)
	}

	buckets, models := foldModelBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &ModelHistogramResult{
		Buckets:           buckets,
		BucketSizeSeconds: bucketSizeSeconds,
		Models:            models,
	}, nil
}

// GetProviderCostHistogram returns a per-provider stacked cost histogram.
func (r *DashboardBucketReader) GetProviderCostHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*ProviderCostHistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []providerCostRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		bucket_start,
		provider,
		SUM(cost) AS cost
	`).Group("bucket_start, provider").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket provider cost scan: %w", err)
	}

	buckets, providers := foldProviderCostBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &ProviderCostHistogramResult{
		Buckets:           buckets,
		BucketSizeSeconds: bucketSizeSeconds,
		Providers:         providers,
	}, nil
}

// GetProviderTokenHistogram returns a per-provider stacked token histogram.
func (r *DashboardBucketReader) GetProviderTokenHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*ProviderTokenHistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []providerTokenRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		bucket_start,
		provider,
		SUM(prompt_tokens) AS prompt_tokens,
		SUM(completion_tokens) AS completion_tokens,
		SUM(total_tokens) AS total_tokens
	`).Group("bucket_start, provider").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket provider token scan: %w", err)
	}

	buckets, providers := foldProviderTokenBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &ProviderTokenHistogramResult{
		Buckets:           buckets,
		BucketSizeSeconds: bucketSizeSeconds,
		Providers:         providers,
	}, nil
}

// GetProviderLatencyHistogram returns a per-provider stacked latency histogram.
func (r *DashboardBucketReader) GetProviderLatencyHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*ProviderLatencyHistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []providerLatencyRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		bucket_start,
		provider,
		SUM(total_latency_ms) AS total_latency_ms,
		SUM(latency_count) AS latency_count,
		SUM(request_count) AS request_count
	`).Group("bucket_start, provider").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket provider latency scan: %w", err)
	}

	buckets, providers := foldProviderLatencyBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &ProviderLatencyHistogramResult{
		Buckets:           buckets,
		BucketSizeSeconds: bucketSizeSeconds,
		Providers:         providers,
	}, nil
}

// GetProviderThroughputHistogram returns a per-provider stacked throughput histogram.
func (r *DashboardBucketReader) GetProviderThroughputHistogram(ctx context.Context, filters SearchFilters, bucketSizeSeconds int64) (*ProviderThroughputHistogramResult, error) {
	if !r.Fresh() || !r.SupportsFilters(filters) || !r.SupportsBucketSize(bucketSizeSeconds) {
		return nil, errAggregatorStale
	}

	var rows []providerThroughputRow
	q := r.db.WithContext(ctx).Model(&DashboardBucketMetric{})
	q = r.applyFilters(q, filters)
	if err := q.Select(`
		bucket_start,
		provider,
		SUM(total_latency_ms) AS total_latency_ms,
		SUM(latency_count) AS latency_count,
		SUM(request_count) AS request_count,
		SUM(completion_tokens) AS completion_tokens
	`).Group("bucket_start, provider").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucket provider throughput scan: %w", err)
	}

	buckets, providers := foldProviderThroughputBuckets(rows, r.bucketSeconds, bucketSizeSeconds,
		filters.StartTime, filters.EndTime)

	return &ProviderThroughputHistogramResult{
		Buckets:           buckets,
		BucketSizeSeconds: bucketSizeSeconds,
		Providers:         providers,
	}, nil
}

// applyFilters attaches the bucket-table-known filters to the query. Caller
// MUST have gated the call with SupportsFilters to avoid silently dropping
// filters the bucket SELECT can't honour.
func (r *DashboardBucketReader) applyFilters(q *gorm.DB, filters SearchFilters) *gorm.DB {
	q = q.Where("bucket_seconds = ?", r.bucketSeconds)
	if start, end := bucketTimeWindow(filters); start != nil || end != nil {
		if start != nil {
			q = q.Where("bucket_start >= ?", *start)
		}
		if end != nil {
			q = q.Where("bucket_start < ?", *end)
		}
	}
	if len(filters.Providers) > 0 {
		q = q.Where("provider IN ?", filters.Providers)
	}
	if len(filters.Models) > 0 {
		q = q.Where("model IN ?", filters.Models)
	}
	if len(filters.Status) > 0 {
		q = q.Where("status IN ?", filters.Status)
	}
	if len(filters.Objects) > 0 {
		q = q.Where("object_type IN ?", filters.Objects)
	}
	return q
}

// bucketTimeWindow returns the [start, end) range to filter bucket_start
// against. start is aligned down to the nearest aggregator bucket boundary
// so the fold step doesn't have to handle a partial first bucket.
func bucketTimeWindow(filters SearchFilters) (*time.Time, *time.Time) {
	if filters.StartTime == nil && filters.EndTime == nil {
		return nil, nil
	}
	if filters.StartTime != nil {
		aligned := alignBucket(*filters.StartTime, bucketSecondsDefault)
		return &aligned, filters.EndTime
	}
	return nil, filters.EndTime
}

// bucketSecondsDefault is the fallback used by bucketTimeWindow when the
// reader's bucketSeconds isn't being threaded through. The reader's own
// applyFilters overrides it via WHERE bucket_seconds = ?.
const bucketSecondsDefault = 300

var errAggregatorStale = fmt.Errorf("dashboard aggregator stale; falling back to logs table")

// histogramRow is one (bucket_start, status, count) row returned by the
// bucket SELECT; feed to foldHistogramBuckets.
type histogramBucketRow struct {
	BucketStart  time.Time
	Status       string
	RequestCount int64
}

// tokenHistogramRow is one (bucket_start, prompt, completion, total) row.
type tokenHistogramRow struct {
	BucketStart      time.Time
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

// costHistogramRow is one (bucket_start, model, cost) row.
type costHistogramRow struct {
	BucketStart time.Time
	Model       string
	Cost        float64
}

// latencyHistogramRow is one (bucket_start, latency_total, latency_count, requests) row.
type latencyHistogramRow struct {
	BucketStart    time.Time
	TotalLatencyMS float64
	LatencyCount   int64
	RequestCount   int64
}

// throughputHistogramRow is one (bucket_start, latency_total, completion) row.
type throughputHistogramRow struct {
	BucketStart      time.Time
	TotalLatencyMS   float64
	LatencyCount     int64
	RequestCount     int64
	CompletionTokens int64
}

// modelHistogramRow is one (bucket_start, model, status, count) row.
type modelHistogramRow struct {
	BucketStart  time.Time
	Model        string
	Status       string
	RequestCount int64
}

// providerCostRow is one (bucket_start, provider, cost) row.
type providerCostRow struct {
	BucketStart time.Time
	Provider    string
	Cost        float64
}

// providerTokenRow is one (bucket_start, provider, prompt, completion, total) row.
type providerTokenRow struct {
	BucketStart      time.Time
	Provider         string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

// providerLatencyRow is one (bucket_start, provider, latency_total, latency_count, requests) row.
type providerLatencyRow struct {
	BucketStart    time.Time
	Provider       string
	TotalLatencyMS float64
	LatencyCount   int64
	RequestCount   int64
}

// providerThroughputRow is one (bucket_start, provider, latency_total, completion, requests) row.
type providerThroughputRow struct {
	BucketStart      time.Time
	Provider         string
	TotalLatencyMS   float64
	LatencyCount     int64
	RequestCount     int64
	CompletionTokens int64
}

// statsRow is one (status, sum_of_metrics) row for the stats aggregation.
type statsRow struct {
	Status         string
	RequestCount   int64
	PromptTokens   int64
	CompleteTokens int64
	TotalTokens    int64
	Cost           float64
	TotalLatency   float64
	LatencyCount   int64
}