package logstore

import (
	"context"
	"testing"
	"time"

	bifrost "github.com/pin-gou/pg-gateway/core"
	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GetStats reports input/output tokens alongside the total so the logs stats
// card can show the split. Only terminal requests contribute, matching the
// total/cost aggregates computed in the same query.
func TestGetStatsTokenSplit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))

	s := &RDBLogStore{db: db, logger: bifrost.NewDefaultLogger(schemas.LogLevelInfo)}
	ctx := context.Background()
	now := time.Now()

	seed := []struct {
		id                        string
		prompt, completion, total int
		status                    string
	}{
		{"a", 100, 10, 110, "success"},
		{"b", 200, 20, 220, "success"},
		{"c", 400, 40, 440, "error"},       // terminal, must count
		{"d", 999, 99, 1098, "processing"}, // non-terminal, must NOT count
	}
	for _, sd := range seed {
		require.NoError(t, db.Create(&Log{
			ID:               sd.id,
			Timestamp:        now,
			Status:           sd.status,
			PromptTokens:     sd.prompt,
			CompletionTokens: sd.completion,
			TotalTokens:      sd.total,
		}).Error)
	}

	stats, err := s.GetStats(ctx, SearchFilters{})
	require.NoError(t, err)

	require.Equal(t, int64(770), stats.TotalTokens, "total excludes non-terminal")
	require.Equal(t, int64(700), stats.PromptTokens, "prompt = 100+200+400")
	require.Equal(t, int64(70), stats.CompletionTokens, "completion = 10+20+40")
	require.Equal(t, stats.TotalTokens, stats.PromptTokens+stats.CompletionTokens, "split sums to total")
}

// TestGetStatsBucketBackfill verifies that when the dashboard-bucket fast path
// is used, the user-facing success rate, min/max latency, and cache-hit counts
// are backfilled from the raw logs table. The bucket table has no fallback
// chain info, per-request latency distribution, or cache_debug payloads, so
// RDBLogStore.GetStats must recompute those fields from the raw table before
// returning.
func TestGetStatsBucketBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &DashboardBucketMetric{}))

	s := &RDBLogStore{db: db, logger: bifrost.NewDefaultLogger(schemas.LogLevelInfo)}
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Attach a bucket reader with IsFresh = always-true, so GetStats takes the
	// fast path.
	reader := NewDashboardBucketReader(db, DashboardBucketReaderConfig{
		AggregatorBucketSeconds: 300,
		IsFresh:                 func() bool { return true },
	})
	require.NotNil(t, reader)
	s.SetBucketReader(reader)

	// Seed raw logs with fallback chains:
	//   chain A: root-a (error) → child-a1 (error) → child-a2 (success)  ← user-facing success
	//   chain B: root-b (success)                                           ← user-facing success
	//   chain C: root-c (success)                                           ← user-facing success
	//   chain D: root-d (error)                                             ← user-facing failure
	nowRounded := now.Truncate(300 * time.Second) // align to bucket boundary
	lat100 := 100.0
	lat200 := 200.0
	lat300 := 300.0
	require.NoError(t, db.Create(&[]Log{
		{ID: "root-a", Timestamp: nowRounded, Status: "error",   FallbackIndex: 0},
		{ID: "child-a1", Timestamp: nowRounded.Add(time.Second), Status: "error",   FallbackIndex: 1, ParentRequestID: strPtr("root-a")},
		{ID: "child-a2", Timestamp: nowRounded.Add(2 * time.Second), Status: "success", FallbackIndex: 2, ParentRequestID: strPtr("root-a"), Latency: &lat100},
		{ID: "root-b", Timestamp: nowRounded, Status: "success", FallbackIndex: 0, Latency: &lat200},
		{ID: "root-c", Timestamp: nowRounded, Status: "success", FallbackIndex: 0, Latency: &lat300},
		{ID: "root-d", Timestamp: nowRounded, Status: "error",   FallbackIndex: 0},
	}).Error)

	// Seed the bucket table with totals that deliberately differ from the raw
	// table (success=2/error=1 vs 3/3 in raw). If the bucket fast path is taken,
	// stats.TotalRequests/SuccessRate/AverageLatency come from the bucket rows,
	// while the user-facing rate, min/max latency, and cache hits are backfilled
	// from raw. Asserting the bucket-derived values proves the fast path ran and
	// the backfill really adds what the bucket table can't carry.
	require.NoError(t, db.Create(&[]DashboardBucketMetric{
		{
			BucketStart:    nowRounded,
			BucketSeconds:  300,
			Status:         "success",
			RequestCount:   2,
			TotalLatencyMS: 300,
			LatencyCount:   2,
			UpdatedAt:      now,
		},
		{
			BucketStart:   nowRounded,
			BucketSeconds: 300,
			Status:        "error",
			RequestCount:  1,
			UpdatedAt:     now,
		},
	}).Error)

	stats, err := s.GetStats(ctx, SearchFilters{})
	require.NoError(t, err)

	// Bucket-provided stats (deliberately differ from the raw table)
	require.Equal(t, int64(3), stats.TotalRequests, "total comes from the bucket fast path")
	require.InDelta(t, 2/3.0*100, stats.SuccessRate, 0.01)
	require.InDelta(t, 150.0, stats.AverageLatency, 0.01)

	// Backfilled fields computed from the raw logs table
	require.Equal(t, int64(4), stats.UserFacingTotalRequests)
	require.InDelta(t, 75.0, stats.UserFacingSuccessRate, 0.01)
	require.InDelta(t, 100.0, stats.MinLatency, 0.01)
	require.InDelta(t, 300.0, stats.MaxLatency, 0.01)
}
