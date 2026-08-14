package logstore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// timelineEventColumns is the column set the design.md "新表 timeline_events"
// section specifies for the timeline_events table.
var timelineEventColumns = []string{
	"id",
	"log_id",
	"phase",
	"source",
	"plugin_name",
	"level",
	"message",
	"time_offset_ms",
	"duration_ms",
	"timestamp",
}

// setupTimelineEventTestStore opens a brand-new SQLite-backed LogStore. Schema
// migration runs inside newSqliteLogStore, so once TimelineEvent is registered
// in the logstore migration list this exercises that AutoMigrate path too.
// Each test gets its own temp DB so writes never leak across tests.
func setupTimelineEventTestStore(t *testing.T) LogStore {
	t.Helper()
	store, err := newSqliteLogStore(context.Background(), &SQLiteConfig{
		Path: filepath.Join(t.TempDir(), "timeline_events.db"),
	}, testLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	return store
}

// pragmaColumnInfo mirrors one row of SQLite's PRAGMA table_info output.
type pragmaColumnInfo struct {
	Name string
	Type string
	PK   int
}

// declaredTypeAffinity reports whether the declared SQLite type (as returned by
// PRAGMA table_info) carries the given affinity. SQLite emits the declared type
// verbatim (case varies: "varchar(255)" / "TEXT" / "REAL" / "datetime"), so we
// match case-insensitively against the family keywords.
func declaredTypeAffinity(declared string, keywords ...string) bool {
	lower := strings.ToLower(declared)
	for _, k := range keywords {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func requireTypeAffinity(t *testing.T, declared string, columns []pragmaColumnInfo, col string, keywords ...string) {
	t.Helper()
	for _, c := range columns {
		if c.Name == col {
			assert.Truef(t, declaredTypeAffinity(c.Type, keywords...),
				"column %q has declared type %q which does not carry affinity %v", col, c.Type, keywords)
			return
		}
	}
	t.Fatalf("column %q missing from PRAGMA table_info output", col)
}

// TestTimelineEventSchema verifies the timeline_events table schema: it is
// created without error by GORM AutoMigrate, all ten designed columns exist
// with the designed types, id is the primary key, and log_id carries the
// by-log_id lookup index. Covers V-framework-1 at the unit level.
func TestTimelineEventSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "timeline_schema.db")), &gorm.Config{})
	require.NoError(t, err)

	// AutoMigrate must succeed once TimelineEvent exists (TDD red phase: the
	// struct is not defined yet, so this file does not compile until dev lands).
	require.NoError(t, db.AutoMigrate(&TimelineEvent{}))

	migrator := db.Migrator()
	require.True(t, migrator.HasTable(&TimelineEvent{}), "timeline_events table must exist after AutoMigrate")
	assert.Equal(t, "timeline_events", TimelineEvent{}.TableName())

	// Every designed column must exist.
	for _, col := range timelineEventColumns {
		assert.Truef(t, migrator.HasColumn(&TimelineEvent{}, col), "timeline_events is missing column %q", col)
	}

	// Declared SQLite types must carry the designed affinities.
	var cols []pragmaColumnInfo
	require.NoError(t, db.Raw("PRAGMA table_info(timeline_events)").Scan(&cols).Error)

	requireTypeAffinity(t, "", cols, "id", "char", "text")
	requireTypeAffinity(t, "", cols, "log_id", "char", "text")
	requireTypeAffinity(t, "", cols, "phase", "char", "text")
	requireTypeAffinity(t, "", cols, "source", "char", "text")
	requireTypeAffinity(t, "", cols, "plugin_name", "char", "text")
	requireTypeAffinity(t, "", cols, "level", "char", "text")
	requireTypeAffinity(t, "", cols, "message", "char", "text")
	requireTypeAffinity(t, "", cols, "time_offset_ms", "real", "float", "double", "numeric")
	requireTypeAffinity(t, "", cols, "duration_ms", "real", "float", "double", "numeric")
	requireTypeAffinity(t, "", cols, "timestamp", "time", "date", "stamp")

	// id is the primary key.
	for _, c := range cols {
		if c.Name == "id" {
			assert.Equal(t, 1, c.PK, "id must be the primary key of timeline_events")
		}
	}

	// log_id must be indexed for the by-log_id read (design: "log_id | string (index)").
	assert.True(t, migrator.HasIndex(&TimelineEvent{}, "idx_timeline_events_log_id"),
		"timeline_events must expose idx_timeline_events_log_id on log_id")
}

// TestTimelineEventWriteReadByLogID verifies the LogStore timeline event
// methods: writing an event round-trips the full field set, reading by log_id
// returns all events for that log, and an unknown log_id yields an empty result
// rather than an error. Covers V-framework-1/V-framework-2 write/read path.
func TestTimelineEventWriteReadByLogID(t *testing.T) {
	store := setupTimelineEventTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	pre := &TimelineEvent{
		ID:           "evt-pre-1",
		LogID:        "log-1",
		Phase:        "pre_llm",
		Source:       "plugin_logging",
		PluginName:   "logging",
		Level:        "info",
		Message:      "pre-llm hook executed",
		TimeOffsetMS: 0.0,
		DurationMS:   8.2,
		Timestamp:    now,
	}
	post := &TimelineEvent{
		ID:           "evt-post-1",
		LogID:        "log-1",
		Phase:        "post_llm",
		Source:       "plugin_logging",
		PluginName:   "logging",
		Level:        "info",
		Message:      "post-llm hook executed",
		TimeOffsetMS: 1128.0,
		DurationMS:   6.5,
		Timestamp:    now.Add(time.Second),
	}
	require.NoError(t, store.CreateTimelineEvent(ctx, pre))
	require.NoError(t, store.CreateTimelineEvent(ctx, post))

	events, err := store.ListTimelineEventsByLogID(ctx, "log-1")
	require.NoError(t, err)
	require.Len(t, events, 2)

	found := map[string]*TimelineEvent{}
	for i := range events {
		found[events[i].Phase] = &events[i]
	}

	preFound, ok := found["pre_llm"]
	require.True(t, ok, "pre_llm event must be returned for log-1")
	assert.Equal(t, "evt-pre-1", preFound.ID)
	assert.Equal(t, "log-1", preFound.LogID)
	assert.Equal(t, "plugin_logging", preFound.Source)
	assert.Equal(t, "logging", preFound.PluginName)
	assert.Equal(t, "info", preFound.Level)
	assert.Equal(t, "pre-llm hook executed", preFound.Message)
	assert.Equal(t, 0.0, preFound.TimeOffsetMS)
	assert.Equal(t, 8.2, preFound.DurationMS)
	assert.Equal(t, now, preFound.Timestamp)

	postFound, ok := found["post_llm"]
	require.True(t, ok, "post_llm event must be returned for log-1")
	assert.Equal(t, "evt-post-1", postFound.ID)
	assert.Equal(t, "log-1", postFound.LogID)
	assert.Equal(t, "plugin_logging", postFound.Source)
	assert.Equal(t, "logging", postFound.PluginName)
	assert.Equal(t, "info", postFound.Level)
	assert.Equal(t, "post-llm hook executed", postFound.Message)
	assert.Equal(t, 1128.0, postFound.TimeOffsetMS)
	assert.Equal(t, 6.5, postFound.DurationMS)
	assert.Equal(t, now.Add(time.Second), postFound.Timestamp)

	// Unknown log_id -> empty result, not an error.
	empty, err := store.ListTimelineEventsByLogID(ctx, "no-such-log")
	require.NoError(t, err)
	assert.Empty(t, empty)
}
