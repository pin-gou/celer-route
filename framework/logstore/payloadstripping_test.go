package logstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStripTestEntry(id string, createdAt time.Time) *Log {
	input := "user message for strip test"
	output := "assistant response for strip test"
	return &Log{
		ID:        id,
		Timestamp: createdAt,
		CreatedAt: createdAt,
		Provider:  "openai",
		Model:     "gpt-4o",
		Status:    "success",
		Object:    "chat.completion",
		InputHistoryParsed: []schemas.ChatMessage{
			{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: &input}},
		},
		OutputMessageParsed: &schemas.ChatMessage{
			Content: &schemas.ChatMessageContent{ContentStr: &output},
		},
		ParamsParsed: map[string]any{"temperature": 0.5},
		TokenUsageParsed: &schemas.BifrostLLMUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		ErrorDetailsParsed: &schemas.BifrostError{
			IsBifrostError: true,
			Error: &schemas.ErrorField{
				Message: "test error details",
			},
		},
	}
}

func TestStripPayloadFieldNamesExcludesExemptColumns(t *testing.T) {
	fields := StripPayloadFieldNames()
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	// token_usage and error_details are exempt (cost recompute / error diagnosis).
	_, hasTokenUsage := set["token_usage"]
	_, hasErrorDetails := set["error_details"]
	assert.False(t, hasTokenUsage, "token_usage must not be stripped")
	assert.False(t, hasErrorDetails, "error_details must not be stripped")
	// Representative payload columns are present.
	_, hasInput := set["input_history"]
	_, hasOutput := set["output_message"]
	_, hasParams := set["params"]
	assert.True(t, hasInput)
	assert.True(t, hasOutput)
	assert.True(t, hasParams)
	// metadata / content_summary are not payload columns at all.
	assert.NotContains(t, fields, "metadata")
	assert.NotContains(t, fields, "content_summary")
}

func TestRDBStripPayloadsBatchClearsOnlyEligibleColumns(t *testing.T) {
	ctx := context.Background()
	store, err := newSqliteLogStore(ctx, &SQLiteConfig{Path: filepath.Join(t.TempDir(), "strip.db")}, hybridTestLogger{})
	require.NoError(t, err)

	now := time.Now().UTC()
	old := now.AddDate(0, 0, -10)
	newer := now.AddDate(0, 0, -2)

	oldEntry := newStripTestEntry("strip-old", old)
	require.NoError(t, store.CreateIfNotExists(ctx, oldEntry))

	newEntry := newStripTestEntry("strip-new", newer)
	require.NoError(t, store.CreateIfNotExists(ctx, newEntry))

	// Cutoff between the two entries: only "strip-old" qualifies.
	cutoff := now.AddDate(0, 0, -5)
	count, err := store.StripPayloadsBatch(ctx, cutoff, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	stripped, err := store.FindByID(ctx, "strip-old")
	require.NoError(t, err)
	assert.True(t, stripped.PayloadStripped)
	assert.Empty(t, stripped.InputHistory)
	assert.Empty(t, stripped.OutputMessage)
	assert.Empty(t, stripped.Params)
	// Exempt columns are retained.
	assert.NotEmpty(t, stripped.TokenUsage)
	require.NotNil(t, stripped.TokenUsageParsed)
	assert.Equal(t, int(10), stripped.TokenUsageParsed.PromptTokens)
	assert.NotEmpty(t, stripped.ErrorDetails)
	require.NotNil(t, stripped.ErrorDetailsParsed)
	assert.Equal(t, "test error details", stripped.ErrorDetailsParsed.Error.Message)
	// Summary is retained for list/search UX.
	assert.Contains(t, stripped.ContentSummary, "user message for strip test")

	untouched, err := store.FindByID(ctx, "strip-new")
	require.NoError(t, err)
	assert.False(t, untouched.PayloadStripped)
	assert.NotEmpty(t, untouched.InputHistory)

	// A second pass strips nothing new (already marked).
	count, err = store.StripPayloadsBatch(ctx, cutoff, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestLogsCleanerStripsPayloadsBeforeDeleting(t *testing.T) {
	ctx := context.Background()
	store, err := newSqliteLogStore(ctx, &SQLiteConfig{Path: filepath.Join(t.TempDir(), "cleaner-strip.db")}, hybridTestLogger{})
	require.NoError(t, err)

	now := time.Now().UTC()
	// 60 days old: within payload retention (30) and retention (365) → stripped only.
	entry := newStripTestEntry("cleaner-old", now.AddDate(0, 0, -60))
	require.NoError(t, store.CreateIfNotExists(ctx, entry))

	cleaner := NewLogsCleaner(store, CleanerConfig{
		RetentionDays:        365,
		PayloadRetentionDays: 30,
	}, hybridTestLogger{})
	cleaner.cleanupOldLogs(ctx)

	row, err := store.FindByID(ctx, "cleaner-old")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.True(t, row.PayloadStripped)
	assert.Empty(t, row.InputHistory)
	// Still present (not deleted — retention is 365).
	assert.NotEmpty(t, row.ContentSummary)
}
