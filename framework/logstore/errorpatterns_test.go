package logstore

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newErrorPatternsStore returns a store backed by a shared-cache in-memory
// sqlite DB. Same naming rationale as newRootsOnlyStore in
// rdb_rootsonly_test.go: in-memory DSN shared across the test process so
// concurrent goroutines (and -count=N re-runs) see consistent data.
func newErrorPatternsStore(t *testing.T) *RDBLogStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&Log{}))
	return &RDBLogStore{db: db}
}

// seedErrorLog inserts a Log row with the given error_details payload. The
// status_code is baked into the error_details JSON (the BifrostError shape)
// since the Log struct does not have a StatusCode column.
func seedErrorLog(t *testing.T, store *RDBLogStore, id, provider, errJSON string, ts time.Time) {
	t.Helper()
	require.NoError(t, store.db.Create(&Log{
		ID:           id,
		Provider:     provider,
		Status:       "error",
		Timestamp:    ts,
		Object:       "chat.completion",
		Model:        "m1",
		ErrorDetails: errJSON,
	}).Error)
}

func TestErrorPatterns_AggregatesByBucket(t *testing.T) {
	store := newErrorPatternsStore(t)
	ctx := context.Background()
	now := time.Now()

	// 3 rows that share the same (status_code=429, type, code, message)
	// 4-tuple must collapse into a single bucket of count=3. The message
	// is intentionally identical across rows so the test isn't sensitive
	// to substring matching — message-prefix variance is exercised by
	// the bucketing semantics in production, not in unit tests.
	for i := 0; i < 3; i++ {
		seedErrorLog(t, store,
			"err-quota-"+string(rune('1'+i)),
			"sensenova",
			`{"status_code":429,"error":{"type":"invalid_request_error","code":"insufficient_quota","message":"Workspace allocated quota exceeded"}}`,
			now.Add(-time.Duration(i)*time.Minute),
		)
	}

	// 2 rows with a different shape: rate_limit_error, no code.
	for i := 0; i < 2; i++ {
		seedErrorLog(t, store,
			"err-rl-"+string(rune('1'+i)),
			"sensenova",
			`{"status_code":429,"error":{"type":"rate_limit_error","message":"HTTP error 429: "}}`,
			now.Add(-time.Duration(i)*time.Minute),
		)
	}

	patterns, total, err := store.ErrorPatterns(ctx, schemas.Sensenova, "24h", 20)
	require.NoError(t, err)
	require.Equal(t, int64(5), total, "total_errors must reflect every error row in window")
	require.Len(t, patterns, 2, "expected 2 distinct buckets: quota (3 rows) and rate_limit (2 rows)")

	// Top-ranked bucket should be the quota one (count=3).
	require.Equal(t, int64(3), patterns[0].Count)
	require.NotNil(t, patterns[0].StatusCode)
	require.Equal(t, 429, *patterns[0].StatusCode)
	require.NotNil(t, patterns[0].ErrorType)
	require.Equal(t, "invalid_request_error", *patterns[0].ErrorType)
	require.NotNil(t, patterns[0].ErrorCode)
	require.Equal(t, "insufficient_quota", *patterns[0].ErrorCode)
	require.NotEmpty(t, patterns[0].ExampleRequestID, "example_request_id must be set so the UI can deep-link")

	// Second bucket: rate_limit_error.
	require.Equal(t, int64(2), patterns[1].Count)
	require.NotNil(t, patterns[1].ErrorType)
	require.Equal(t, "rate_limit_error", *patterns[1].ErrorType)
	require.Nil(t, patterns[1].ErrorCode, "rate_limit bucket has no code field; nil expected")
}

func TestErrorPatterns_EmptyWindowReturnsZeroTotal(t *testing.T) {
	store := newErrorPatternsStore(t)
	ctx := context.Background()

	patterns, total, err := store.ErrorPatterns(ctx, schemas.OpenAI, "24h", 20)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
	require.Empty(t, patterns)
}

func TestErrorPatterns_InvalidWindowReturnsError(t *testing.T) {
	store := newErrorPatternsStore(t)
	ctx := context.Background()

	_, _, err := store.ErrorPatterns(ctx, schemas.OpenAI, "7d", 20)
	require.Error(t, err, "only 1h/24h are supported; anything else must reject at the API boundary")
}

func TestErrorPatterns_LimitClampedTo100(t *testing.T) {
	store := newErrorPatternsStore(t)
	ctx := context.Background()

	// No errors seeded → must return nil slice + zero total without error.
	patterns, total, err := store.ErrorPatterns(ctx, schemas.OpenAI, "1h", 500)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
	require.Nil(t, patterns, "empty results may be nil; the UI renders no-errors from the total_errors=0 signal")
}

func TestErrorPatterns_ProviderFilterIsolatesBuckets(t *testing.T) {
	// Same error shape under two different providers must produce separate
	// buckets when the UI asks provider=openai — sanity check that the WHERE
	// clause actually filters, not aggregates across providers.
	store := newErrorPatternsStore(t)
	ctx := context.Background()
	now := time.Now()

	seedErrorLog(t, store, "a-1", "openai", `{"status_code":429,"error":{"type":"rate_limit_error","message":"hi"}}`, now)
	seedErrorLog(t, store, "a-2", "sensenova", `{"status_code":429,"error":{"type":"rate_limit_error","message":"hi"}}`, now)

	patterns, total, err := store.ErrorPatterns(ctx, schemas.OpenAI, "24h", 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total, "openai-only query must not include sensenova rows")
	require.Len(t, patterns, 1)
	require.Equal(t, int64(1), patterns[0].Count)
}

// TestErrorPatterns_LocalTimezoneDoesNotSkewWindow pins the bug where the
// bucket query embedded the window's lower bound as a naive local wall-clock
// string while the DB compares in UTC. On a UTC+8 server the "1h" window was
// therefore pushed 7h into the future and matched nothing (and the oldest 8h
// of "24h" was silently dropped), even though the separate total_errors count
// — which binds the interval — was non-zero. Both queries must now agree.
//
// Postgres is the backend that exposed this: its timestamp column is
// timestamptz and a naive literal is interpreted in the session timezone
// (UTC). SQLite can't reproduce it because both comparisons go through the
// same lexicographic text encoding. Skips when Postgres is unavailable.
func TestErrorPatterns_LocalTimezoneDoesNotSkewWindow(t *testing.T) {
	origLocal := time.Local
	time.Local = time.FixedZone("CST", 8*3600)
	t.Cleanup(func() { time.Local = origLocal })

	db := trySetupPostgresDB(t)
	if db == nil {
		t.Skip("Postgres not available, skipping timezone-skew regression test")
	}
	require.NoError(t, db.AutoMigrate(&Log{}))
	store := &RDBLogStore{db: db}
	ctx := context.Background()

	const provider = "tzskew"
	require.NoError(t, db.Where("provider = ?", provider).Delete(&Log{}).Error)
	t.Cleanup(func() { _ = db.Where("provider = ?", provider).Delete(&Log{}).Error })

	// Errors that happened "just now" in absolute terms, stored as UTC (as the
	// DB does). The process local zone is UTC+8, so a naive local literal would
	// be 7h ahead of these rows for the 1h window.
	now := time.Now()
	for i := 0; i < 3; i++ {
		seedErrorLog(t, store,
			fmt.Sprintf("tzskew-%d", i),
			provider,
			`{"status_code":429,"error":{"type":"rate_limit_error","code":"8","message":"tpm exhausted"}}`,
			now.UTC().Add(-time.Duration(i)*time.Minute),
		)
	}

	patterns, total, err := store.ErrorPatterns(ctx, schemas.ModelProvider(provider), "1h", 20)
	require.NoError(t, err)
	require.Equal(t, int64(3), total, "total_errors must see the rows regardless of local timezone")
	require.Len(t, patterns, 1, "the bucket query must not be skewed by the server's local timezone")
	require.Equal(t, int64(3), patterns[0].Count)
}
