package logstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStorageStatsPayloadBreakdown pins the wire shape returned by
// RDBLogStore.StorageStats' payload-state breakdown. The four buckets must
// cover every row exactly once — Hidden > Stripped > Offloaded > WithPayload
// — and the four counts together must sum to total_logs.
//
// Regression target: a previous implementation passed integer placeholders
// (1/0) to the boolean columns (`content_hidden`, `payload_stripped`,
// `has_object`). SQLite's type coercion made the comparison succeed, so the
// unit test on SQLite still passed and the bug only surfaced on Postgres,
// which rejects `boolean = integer`. The handler also swallowed the
// `estimatePayloadBreakdown` error, so the settings page received 0/0/0/0
// and silently misreported the breakdown on every refresh.
//
// The fix uses bool placeholders (true/false). Both backends accept them.
func TestStorageStatsPayloadBreakdown(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1 Hidden row, 2 Stripped rows, 3 Offloaded rows, 4 WithPayload rows.
	// 1+2+3+4 = 10 rows; the test asserts the four counts equal these
	// buckets separately and that they sum to total_logs=10.
	seed := []*Log{
		{ID: "hidden", Timestamp: now, Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", ContentHidden: true},
		{ID: "stripped-1", Timestamp: now.Add(time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", PayloadStripped: true},
		{ID: "stripped-2", Timestamp: now.Add(2 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", PayloadStripped: true},

		{ID: "offloaded-1", Timestamp: now.Add(3 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", HasObject: true},
		{ID: "offloaded-2", Timestamp: now.Add(4 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", HasObject: true},
		{ID: "offloaded-3", Timestamp: now.Add(5 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", HasObject: true},

		{ID: "with-payload-1", Timestamp: now.Add(6 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success"},
		{ID: "with-payload-2", Timestamp: now.Add(7 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success"},
		{ID: "with-payload-3", Timestamp: now.Add(8 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success"},
		{ID: "with-payload-4", Timestamp: now.Add(9 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success"},
	}
	require.NoError(t, store.db.Create(seed).Error)

	stats, err := store.StorageStats(ctx)
	require.NoError(t, err, "StorageStats should not fail on a populated table")

	require.Equal(t, int64(10), stats.TotalLogs, "all 10 seeded rows must be counted")
	require.Equal(t, int64(1), stats.LogsHidden, "1 hidden row")
	require.Equal(t, int64(2), stats.LogsStripped, "2 stripped rows")
	require.Equal(t, int64(3), stats.LogsOffloaded, "3 offloaded rows")
	require.Equal(t, int64(4), stats.LogsWithPayload, "4 with-payload rows")

	// Buckets must partition the table — no row counted twice, none dropped.
	require.Equal(t,
		stats.TotalLogs,
		stats.LogsHidden+stats.LogsStripped+stats.LogsOffloaded+stats.LogsWithPayload,
		"bucket counts must sum to total_logs")

	// Size estimates depend on the same breakdown — guard the size split too.
	// Without the fix, all bucket counts were zero, which makes
	// SizeWithPayloadBytes and SizeOffloadedBytes zero and the "estimated
	// size by state" card show only the metadata cost.
	require.Greater(t, stats.SizeWithPayloadBytes, int64(0), "size_with_payload_bytes must reflect the WithPayload bucket (4 rows × ~8 KB)")
	require.Greater(t, stats.SizeOffloadedBytes, int64(0), "size_offloaded_bytes must reflect the Offloaded bucket (3 rows × ~8 KB)")
}

// TestStorageStatsPayloadBreakdownHiddenTakesPrecedence pins the precedence
// order: a row that has BOTH content_hidden=true and payload_stripped=true
// must land in the Hidden bucket, not Stripped. The Hidden bucket is
// evaluated first in the CASE-WHEN chain so a hidden row is hidden whether
// or not the retention cleaner later stripped its payload columns.
//
// Without this guarantee, a hidden-then-stripped row would be counted
// twice (once in Hidden, once in Stripped) and the four counts would
// overshoot total_logs.
func TestStorageStatsPayloadBreakdownHiddenTakesPrecedence(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Two hidden-and-stripped rows. If Stripped were evaluated before
	// Hidden, both would land in Stripped too, and the totals wouldn't
	// reconcile.
	require.NoError(t, store.db.Create(&[]*Log{
		{ID: "hidden-stripped-1", Timestamp: now, Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", ContentHidden: true, PayloadStripped: true},
		{ID: "hidden-stripped-2", Timestamp: now.Add(time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", ContentHidden: true, PayloadStripped: true},
	}).Error)

	stats, err := store.StorageStats(ctx)
	require.NoError(t, err)

	require.Equal(t, int64(2), stats.TotalLogs)
	require.Equal(t, int64(2), stats.LogsHidden, "hidden-then-stripped rows must land in Hidden")
	require.Equal(t, int64(0), stats.LogsStripped, "hidden rows must not double-count as Stripped")
	require.Equal(t, int64(0), stats.LogsOffloaded)
	require.Equal(t, int64(0), stats.LogsWithPayload)
}

// TestStorageStatsPayloadBreakdownEmptyTable pins the empty-table shape: a
// 0-row table must produce 0/0/0/0 with no error. This guards against a
// future refactor that uses a different SQL form (e.g. FILTER) which might
// raise on empty input.
func TestStorageStatsPayloadBreakdownEmptyTable(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	stats, err := store.StorageStats(ctx)
	require.NoError(t, err, "StorageStats on an empty table must not error")

	require.Equal(t, int64(0), stats.TotalLogs)
	require.Equal(t, int64(0), stats.LogsHidden)
	require.Equal(t, int64(0), stats.LogsStripped)
	require.Equal(t, int64(0), stats.LogsOffloaded)
	require.Equal(t, int64(0), stats.LogsWithPayload)
	require.Equal(t, int64(0), stats.SizeWithoutPayloadBytes)
	require.Equal(t, int64(0), stats.SizeWithPayloadBytes)
	require.Equal(t, int64(0), stats.SizeOffloadedBytes)
}

// TestStorageStatsPayloadBreakdownPostgres pins the breakdown SQL against
// Postgres specifically. The bug fixed alongside this test was Postgres-only:
// integer placeholders (1/0) sent to boolean columns raised
// `operator does not exist: boolean = integer`, and the surrounding handler
// swallowed the error so the settings page received 0/0/0/0 even on a
// 100k-row table. SQLite's permissive type coercion masked the regression
// from the SQLite tests above, so this test is the only one that catches
// a re-introduction of the same kind of mistake.
//
// Skipped when Postgres isn't reachable — the suite must still pass on
// SQLite-only dev machines. CI runs against the framework/docker-compose
// stack so this test runs there.
func TestStorageStatsPayloadBreakdownPostgres(t *testing.T) {
	db := trySetupPostgresDB(t)
	if db == nil {
		t.Skip("Postgres not available, skipping breakdown regression test")
	}

	// Drop existing matviews and tables so we own a clean schema; the
	// logstoreparity_test does the same dance.
	dropAllManagedMatViews(db)
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS logs CASCADE").Error)
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS migrations (id VARCHAR(255) PRIMARY KEY)").Error)
	require.NoError(t, db.Exec("DELETE FROM migrations").Error)
	require.NoError(t, triggerMigrations(context.Background(), db, testLogger{}))

	store := &RDBLogStore{db: db, logger: testLogger{}}
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed one row per bucket — exactly the same fixture as the SQLite test
	// so the Postgres path is exercising the same code path.
	seed := []*Log{
		{ID: "pg-hidden", Timestamp: now, Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", ContentHidden: true},
		{ID: "pg-stripped", Timestamp: now.Add(time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", PayloadStripped: true},
		{ID: "pg-offloaded", Timestamp: now.Add(2 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success", HasObject: true},
		{ID: "pg-with-payload", Timestamp: now.Add(3 * time.Second), Object: "chat_completion", Provider: "openai", Model: "gpt-4o-mini", Status: "success"},
	}
	require.NoError(t, db.Create(seed).Error)

	stats, err := store.StorageStats(ctx)
	require.NoError(t, err, "StorageStats must not fail on Postgres — the previous bug raised `operator does not exist: boolean = integer` here")

	require.Equal(t, int64(4), stats.TotalLogs)
	require.Equal(t, int64(1), stats.LogsHidden, "1 hidden row on Postgres")
	require.Equal(t, int64(1), stats.LogsStripped, "1 stripped row on Postgres")
	require.Equal(t, int64(1), stats.LogsOffloaded, "1 offloaded row on Postgres")
	require.Equal(t, int64(1), stats.LogsWithPayload, "1 with-payload row on Postgres")

	require.Equal(t,
		stats.TotalLogs,
		stats.LogsHidden+stats.LogsStripped+stats.LogsOffloaded+stats.LogsWithPayload,
		"bucket counts must sum to total_logs on Postgres")

	// Clean up so this test doesn't leak rows into the shared Postgres
	// schema for the next test.
	t.Cleanup(func() {
		dropAllManagedMatViews(db)
		db.Exec("DELETE FROM logs")
	})
}
