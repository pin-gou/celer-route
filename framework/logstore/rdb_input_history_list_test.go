package logstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLiteListTruncatesToLastUserMessage proves the list projection keeps only
// the last user-role message in input_history / responses_input_history.
//
// Without this, conversations whose final element is a system / developer /
// tool message — a "<system-reminder>"-style injection, an Anthropic trailing
// safety block, or any agent loop that ends on a tool_call_id — render that
// trailing message in the LLM-logs list instead of the user's prompt,
// contradicting the SSE / content_summary preview which already shows the last
// user message. The fix is in rdb.go's sqliteLastUserInputHistoryExpr: scan
// with json_each and pick role='user' closest to the end, falling back to the
// literal last array element when no user message exists so an assistant-only
// or developer-only row still gets a preview.
func TestSQLiteListTruncatesToLastUserMessage(t *testing.T) {
	store := newSqliteInputHistoryTestStore(t)
	ctx := context.Background()

	// Conversation with a trailing non-user message after the user's actual
	// prompt — the shape that surfaced "<system-reminder>" in production.
	multiTurnMessages := []schemas.ChatMessage{
		{Role: schemas.ChatMessageRoleSystem, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("You are a helpful assistant.")}},
		{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("first user prompt")}},
		{Role: schemas.ChatMessageRoleAssistant, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("assistant reply 1")}},
		{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("real user question?")}},
		{Role: schemas.ChatMessageRoleSystem, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("<system-reminder>trailing safety</system-reminder>")}},
	}

	// A row whose final element is already a user message — must stay as-is.
	plainUserMessages := []schemas.ChatMessage{
		{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("first")}},
		{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("latest user prompt")}},
	}

	// No user-role message at all (developer / assistant only). The fallback
	// expression must still hand back one element so the row renders something.
	developerOnly := []schemas.ChatMessage{
		{Role: schemas.ChatMessageRoleDeveloper, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("first developer block")}},
		{Role: schemas.ChatMessageRoleAssistant, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("only assistant reply")}},
	}

	// Empty array — projection must pass the column through.
	emptyMessages := []schemas.ChatMessage{}

	// Responses API shape with a trailing non-user block.
	msgType := schemas.ResponsesMessageType("message")
	responsesMixed := []schemas.ResponsesMessage{
		{Role: schemas.Ptr(schemas.ResponsesInputMessageRoleUser), Content: &schemas.ResponsesMessageContent{ContentStr: schemas.Ptr("real user request")}, Type: &msgType},
		{Role: schemas.Ptr(schemas.ResponsesInputMessageRoleSystem), Content: &schemas.ResponsesMessageContent{ContentStr: schemas.Ptr("trailing system")}, Type: &msgType},
	}

	rows := []*Log{
		newSQLiteInputHistoryRow("multi-turn", "chat.completion", multiTurnMessages, nil),
		newSQLiteInputHistoryRow("user-only", "chat.completion", plainUserMessages, nil),
		newSQLiteInputHistoryRow("no-user", "chat.completion", developerOnly, nil),
		newSQLiteInputHistoryRow("empty", "chat.completion", emptyMessages, nil),
		newSQLiteInputHistoryRow("responses-mixed", "responses", nil, responsesMixed),
	}
	for i, r := range rows {
		r.Timestamp = time.Now().UTC().Add(time.Duration(i) * time.Second)
		require.NoError(t, r.SerializeFields())
		require.NoError(t, store.Create(ctx, r))
	}

	result, err := store.SearchLogs(ctx, SearchFilters{}, PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Logs, len(rows))

	byID := make(map[string]*Log, len(result.Logs))
	for i := range result.Logs {
		byID[result.Logs[i].ID] = &result.Logs[i]
	}

	require.NoError(t, byID["multi-turn"].DeserializeFields())
	require.Len(t, byID["multi-turn"].InputHistoryParsed, 1,
		"trailing system must not survive the list projection")
	assert.Equal(t, schemas.ChatMessageRoleUser, byID["multi-turn"].InputHistoryParsed[0].Role)
	assert.Equal(t, "real user question?", *byID["multi-turn"].InputHistoryParsed[0].Content.ContentStr,
		"the kept message must be the last user-role element, not the trailing system one")

	require.NoError(t, byID["user-only"].DeserializeFields())
	require.Len(t, byID["user-only"].InputHistoryParsed, 1)
	assert.Equal(t, "latest user prompt", *byID["user-only"].InputHistoryParsed[0].Content.ContentStr)

	require.NoError(t, byID["no-user"].DeserializeFields())
	require.Len(t, byID["no-user"].InputHistoryParsed, 1,
		"fallback must still yield one element when no user role is present")
	assert.Equal(t, schemas.ChatMessageRoleAssistant, byID["no-user"].InputHistoryParsed[0].Role,
		"fallback must be the literal last element so the row preview is not empty")

	require.NoError(t, byID["empty"].DeserializeFields())
	assert.Empty(t, byID["empty"].InputHistoryParsed, "empty arrays must round-trip as empty")

	require.NoError(t, byID["responses-mixed"].DeserializeFields())
	require.Len(t, byID["responses-mixed"].ResponsesInputHistoryParsed, 1)
	if role := byID["responses-mixed"].ResponsesInputHistoryParsed[0].Role; assert.NotNil(t, role) {
		assert.Equal(t, schemas.ResponsesInputMessageRoleUser, *role,
			"the kept ResponsesMessage must be the last user-role one")
	}
}

// TestSQLiteListInputHistoryPassThroughOnBadJSON pins the malformed-input
// fallback. /api/logs must never abort on bad input_history: the list query
// passes the column through unchanged so detail / search can recover.
//
// Drives the same ELSE branch in sqliteLastUserInputHistoryExpr that the
// user-message subquery sits inside of.
func TestSQLiteListInputHistoryPassThroughOnBadJSON(t *testing.T) {
	store := newSqliteInputHistoryTestStore(t)
	ctx := context.Background()

	entry := newSQLiteInputHistoryRow("bad", "chat.completion",
		[]schemas.ChatMessage{
			{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("valid transient")}},
		}, nil)
	entry.Timestamp = time.Now().UTC()
	require.NoError(t, entry.SerializeFields())
	require.NoError(t, store.Create(ctx, entry))

	// Clobber the column to a non-array JSON literal at the SQL layer so the
	// projection's json_type(...)='array' guard triggers.
	require.NoError(t, store.ScopedDB(ctx).Exec(
		"UPDATE logs SET input_history = ? WHERE id = ?", `"just-a-string-not-array"`, "bad",
	).Error)

	result, err := store.SearchLogs(ctx, SearchFilters{}, PaginationOptions{Limit: 10})
	require.NoError(t, err, "malformed input_history must not break the list query")
	require.Len(t, result.Logs, 1)
	assert.Equal(t, `"just-a-string-not-array"`, result.Logs[0].InputHistory,
		"non-array / malformed column must be passed through verbatim")
}

func newSqliteInputHistoryTestStore(t *testing.T) *RDBLogStore {
	t.Helper()
	store, err := newSqliteLogStore(
		context.Background(),
		&SQLiteConfig{Path: filepath.Join(t.TempDir(), "input-history-list.db")},
		testLogger{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	return store
}

func newSQLiteInputHistoryRow(id, object string, chat []schemas.ChatMessage, responses []schemas.ResponsesMessage) *Log {
	row := &Log{
		ID:        id,
		Object:    object,
		Provider:  "openai",
		Model:     "gpt-test",
		Status:    "success",
		Timestamp: time.Now().UTC(),
	}
	if chat != nil {
		row.InputHistoryParsed = chat
	}
	if responses != nil {
		row.ResponsesInputHistoryParsed = responses
	}
	return row
}
