package logstore

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// autoMigrateLogStoreSchema creates the logs + log_payloads tables the way the
// production payload-split migrations leave them: payload columns exist only on
// log_payloads, not on the logs heap row. Test stores that exercise SearchLogs /
// GetStats must use this instead of a bare AutoMigrate(&Log{}) — the list query
// LEFT JOINs log_payloads, and a logs table that still carries payload columns
// would make the unqualified column references ambiguous.
func autoMigrateLogStoreSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&Log{}, &LogPayload{}))
	for _, col := range stripPayloadOmitColumns {
		require.NoError(t, db.Exec(fmt.Sprintf("ALTER TABLE logs DROP COLUMN %s", col)).Error)
	}
}

// payloadSplitTestEntry builds a Log with representative payload fields set, so
// the write/read round-trip exercises the side table.
func payloadSplitTestEntry(id string, ts time.Time) *Log {
	return &Log{
		ID:        id,
		Timestamp: ts,
		CreatedAt: ts,
		Provider:  "openai",
		Model:     "gpt-4o",
		Status:    "success",
		Object:    "chat.completion",
		InputHistoryParsed: []schemas.ChatMessage{
			{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: &id}},
		},
		OutputMessageParsed: &schemas.ChatMessage{
			Content: &schemas.ChatMessageContent{ContentStr: &id},
		},
		ParamsParsed: map[string]any{"temperature": 0.5},
		TokenUsageParsed: &schemas.BifrostLLMUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
}

// TestPayloadSplitSchemaPinsColumnPlacement verifies the migrated schema keeps
// payload columns off the logs heap: the logs table has none of the strip-able
// columns, and log_payloads carries them.
func TestPayloadSplitSchemaPinsColumnPlacement(t *testing.T) {
	store, err := newSqliteLogStore(context.Background(), &SQLiteConfig{Path: filepath.Join(t.TempDir(), "split.db")}, hybridTestLogger{})
	require.NoError(t, err)

	var logsCols []struct{ Name string }
	require.NoError(t, store.db.Raw("SELECT name FROM pragma_table_info('logs')").Scan(&logsCols).Error)
	got := make(map[string]bool, len(logsCols))
	for _, c := range logsCols {
		got[c.Name] = true
	}
	for _, col := range stripPayloadOmitColumns {
		require.Falsef(t, got[col], "logs table must not retain payload column %q after the split", col)
	}

	var payloadCols []struct{ Name string }
	require.NoError(t, store.db.Raw("SELECT name FROM pragma_table_info('log_payloads')").Scan(&payloadCols).Error)
	gotPayload := make(map[string]bool, len(payloadCols))
	for _, c := range payloadCols {
		gotPayload[c.Name] = true
	}
	for _, col := range stripPayloadOmitColumns {
		require.Truef(t, gotPayload[col], "log_payloads table must carry payload column %q", col)
	}
}

// TestPayloadSplitCreateThenFindByIDRoundTripsPayload pins the write/read path:
// CreateIfNotExists writes the heap row plus the side row, and FindByID returns
// the parsed payload (message history, output, params) plus the token_usage
// that stays on the heap row.
func TestPayloadSplitCreateThenFindByIDRoundTripsPayload(t *testing.T) {
	store, err := newSqliteLogStore(context.Background(), &SQLiteConfig{Path: filepath.Join(t.TempDir(), "split.db")}, hybridTestLogger{})
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()

	entry := payloadSplitTestEntry("split-rt", now)
	require.NoError(t, store.CreateIfNotExists(ctx, entry))

	found, err := store.FindByID(ctx, "split-rt")
	require.NoError(t, err)
	require.Len(t, found.InputHistoryParsed, 1)
	require.Equal(t, "split-rt", *found.InputHistoryParsed[0].Content.ContentStr)
	require.NotNil(t, found.OutputMessageParsed)
	require.Equal(t, "split-rt", *found.OutputMessageParsed.Content.ContentStr)
	require.Equal(t, 0.5, found.ParamsParsed.(map[string]any)["temperature"])
	// token_usage stays DB-resident on the heap row.
	require.NotNil(t, found.TokenUsageParsed)
	require.Equal(t, 15, found.TokenUsageParsed.TotalTokens)
}

// TestPayloadSplitTerminalUpdateUpsertsPayload pins the terminal-update path:
// an initial insert writes input_history/params, then Update(map) writes
// output_message onto the same side row without wiping the earlier fields.
func TestPayloadSplitTerminalUpdateUpsertsPayload(t *testing.T) {
	store, err := newSqliteLogStore(context.Background(), &SQLiteConfig{Path: filepath.Join(t.TempDir(), "split.db")}, hybridTestLogger{})
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()

	entry := payloadSplitTestEntry("split-upd", now)
	require.NoError(t, store.CreateIfNotExists(ctx, entry))

	// Terminal update carries only the output; input_history must survive.
	require.NoError(t, store.Update(ctx, "split-upd", map[string]interface{}{
		"status":         "success",
		"output_message": `{"role":"assistant","content":"final answer"}`,
	}))

	found, err := store.FindByID(ctx, "split-upd")
	require.NoError(t, err)
	require.Len(t, found.InputHistoryParsed, 1, "input_history written by the initial insert must survive the terminal update")
	require.Equal(t, "final answer", *found.OutputMessageParsed.Content.ContentStr)
}

// TestPayloadSplitBatchUpsertRoundTripsPayload pins the batch-writer path.
func TestPayloadSplitBatchUpsertRoundTripsPayload(t *testing.T) {
	store, err := newSqliteLogStore(context.Background(), &SQLiteConfig{Path: filepath.Join(t.TempDir(), "split.db")}, hybridTestLogger{})
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()

	entries := []*Log{
		payloadSplitTestEntry("split-b1", now),
		payloadSplitTestEntry("split-b2", now.Add(time.Second)),
	}
	require.NoError(t, store.BatchUpsert(ctx, entries))

	for _, id := range []string{"split-b1", "split-b2"} {
		found, err := store.FindByID(ctx, id)
		require.NoError(t, err)
		require.Len(t, found.InputHistoryParsed, 1)
		require.NotNil(t, found.OutputMessageParsed)
	}
}

// TestPayloadSplitStripDeletesSideRow pins the strip path: after stripping, the
// payload is gone from reads AND the log_payloads row is physically deleted (the
// whole point of the split — Postgres autovacuum reclaims the space).
func TestPayloadSplitStripDeletesSideRow(t *testing.T) {
	store, err := newSqliteLogStore(context.Background(), &SQLiteConfig{Path: filepath.Join(t.TempDir(), "split.db")}, hybridTestLogger{})
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, store.CreateIfNotExists(ctx, payloadSplitTestEntry("split-strip", now.AddDate(0, 0, -10))))
	require.NoError(t, store.CreateIfNotExists(ctx, payloadSplitTestEntry("split-keep", now.AddDate(0, 0, -2))))

	count, err := store.StripPayloadsBatch(ctx, now.AddDate(0, 0, -5), 100)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	stripped, err := store.FindByID(ctx, "split-strip")
	require.NoError(t, err)
	require.True(t, stripped.PayloadStripped)
	require.Empty(t, stripped.InputHistory)
	require.Nil(t, stripped.InputHistoryParsed)
	// token_usage / cache_debug survive stripping (exempt, heap-resident).
	require.NotNil(t, stripped.TokenUsageParsed)
	require.Equal(t, 15, stripped.TokenUsageParsed.TotalTokens)

	kept, err := store.FindByID(ctx, "split-keep")
	require.NoError(t, err)
	require.False(t, kept.PayloadStripped)
	require.Len(t, kept.InputHistoryParsed, 1)

	// The side row is deleted, not blanked: a second strip pass strips nothing.
	var sideCount int64
	require.NoError(t, store.db.Raw("SELECT COUNT(*) FROM log_payloads WHERE log_id = ?", "split-strip").Scan(&sideCount).Error)
	require.Equal(t, int64(0), sideCount)
	count, err = store.StripPayloadsBatch(ctx, now.AddDate(0, 0, -5), 100)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

// TestPayloadSplitDeleteLogRemovesSideRow pins the delete paths.
func TestPayloadSplitDeleteLogRemovesSideRow(t *testing.T) {
	store, err := newSqliteLogStore(context.Background(), &SQLiteConfig{Path: filepath.Join(t.TempDir(), "split.db")}, hybridTestLogger{})
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, store.CreateIfNotExists(ctx, payloadSplitTestEntry("split-del-1", now)))
	require.NoError(t, store.CreateIfNotExists(ctx, payloadSplitTestEntry("split-del-2", now)))

	require.NoError(t, store.DeleteLog(ctx, "split-del-1"))
	require.NoError(t, store.DeleteLogs(ctx, []string{"split-del-2"}))

	var sideCount int64
	require.NoError(t, store.db.Raw("SELECT COUNT(*) FROM log_payloads").Scan(&sideCount).Error)
	require.Equal(t, int64(0), sideCount)
}

// TestPayloadSplitMigrationBackfillsExistingRows simulates an existing
// pre-split deployment: a logs table that still carries the payload columns and
// real data. Running the migrations must backfill log_payloads from those rows,
// drop the columns off logs, and leave FindByID serving the payload.
func TestPayloadSplitMigrationBackfillsExistingRows(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "presplit.db")), &gorm.Config{})
	require.NoError(t, err)

	// Old-style schema: logs with payload columns (from the full struct).
	require.NoError(t, db.AutoMigrate(&Log{}))

	now := time.Now().UTC()
	rows := []*Log{
		payloadSplitTestEntry("pre-1", now),
		payloadSplitTestEntry("pre-2", now.Add(time.Second)),
		// A stripped legacy row has cleared payload columns: no side row expected.
		{ID: "pre-stripped", Timestamp: now.Add(2 * time.Second), Object: "chat.completion", Provider: "openai", Model: "gpt-4o", Status: "success", PayloadStripped: true},
	}
	for _, row := range rows {
		require.NoError(t, row.SerializeFields())
		require.NoError(t, db.Create(row).Error)
	}

	// Run the whole migration chain (creates log_payloads, backfills, drops cols).
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS migrations (id VARCHAR(255) PRIMARY KEY)").Error)
	require.NoError(t, triggerMigrations(ctx, db, testLogger{}))

	store := &RDBLogStore{db: db, logger: testLogger{}}

	// Payload columns are gone from logs.
	var logsCols []struct{ Name string }
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('logs')").Scan(&logsCols).Error)
	colSet := map[string]bool{}
	for _, c := range logsCols {
		colSet[c.Name] = true
	}
	require.False(t, colSet["input_history"], "logs must not keep input_history after the split migration")

	// Backfilled rows read back through FindByID.
	for _, id := range []string{"pre-1", "pre-2"} {
		found, err := store.FindByID(ctx, id)
		require.NoError(t, err)
		require.Len(t, found.InputHistoryParsed, 1)
		require.NotNil(t, found.OutputMessageParsed)
	}
	// Stripped legacy rows backfill nothing and read as payload-less.
	stripped, err := store.FindByID(ctx, "pre-stripped")
	require.NoError(t, err)
	require.True(t, stripped.PayloadStripped)
	require.Empty(t, stripped.InputHistory)
	var sideCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM log_payloads").Scan(&sideCount).Error)
	require.Equal(t, int64(2), sideCount)
}

// TestPayloadSplitFlushRemovesSideRow pins Flush (processing-row cleanup).
func TestPayloadSplitFlushRemovesSideRow(t *testing.T) {
	store, err := newSqliteLogStore(context.Background(), &SQLiteConfig{Path: filepath.Join(t.TempDir(), "split.db")}, hybridTestLogger{})
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()

	processing := payloadSplitTestEntry("split-flush", now.Add(-10*time.Minute))
	processing.Status = "processing"
	require.NoError(t, store.CreateIfNotExists(ctx, processing))

	require.NoError(t, store.Flush(ctx, now.Add(-5*time.Minute)))

	var sideCount int64
	require.NoError(t, store.db.Raw("SELECT COUNT(*) FROM log_payloads").Scan(&sideCount).Error)
	require.Equal(t, int64(0), sideCount)
}
