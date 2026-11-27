package logstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// usageLog builds a minimal terminal-status log row for usage aggregation tests.
func usageLog(id, provider, model, keyID, keyName, status string, ts time.Time, prompt, completion, cached, total int) *Log {
	return &Log{
		ID:               id,
		Timestamp:        ts,
		Object:           "chat.completion",
		Provider:         provider,
		Model:            model,
		SelectedKeyID:    keyID,
		SelectedKeyName:  keyName,
		Status:           status,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		CachedReadTokens: cached,
		TotalTokens:      total,
	}
}

func TestProviderKeyModelUsageGroupsAndOrdering(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	start := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	rows := []*Log{
		// Target provider: two models, two named keys plus one keyless group.
		usageLog("u1", "sensenova", "model-a", "key-1", "simon", "success", start.Add(5*time.Minute), 1000, 100, 500, 1100),
		// key-2 has one error and one cancelled row, so its counters differ.
		usageLog("u2", "sensenova", "model-a", "key-2", "lily", "error", start.Add(10*time.Minute), 300, 30, 0, 330),
		usageLog("u3", "sensenova", "model-a", "key-2", "lily", "cancelled", start.Add(11*time.Minute), 40, 4, 0, 44),
		usageLog("u4", "sensenova", "model-b", "key-1", "simon", "success", start.Add(20*time.Minute), 2000, 200, 1500, 2200),
		// Keyless provider path: no selected key.
		usageLog("u5", "sensenova", "model-b", "", "", "success", start.Add(30*time.Minute), 10, 1, 0, 11),
		// A different provider must never leak into the result.
		usageLog("u6", "openai", "model-a", "key-9", "other", "success", start.Add(30*time.Minute), 50000, 49999, 0, 99999),
		// In-flight rows are excluded: they have no terminal outcome.
		usageLog("u7", "sensenova", "model-a", "key-1", "simon", "processing", start.Add(40*time.Minute), 7, 7, 0, 14),
	}
	for _, row := range rows {
		require.NoError(t, store.Create(ctx, row), "Create(%s)", row.ID)
	}

	result, err := store.GetProviderKeyModelUsage(ctx, "sensenova", start, end)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "sensenova", result.Provider)
	require.Equal(t, start, result.StartTime)
	require.Equal(t, end, result.EndTime)

	// Ordered by total_tokens desc. model-a/key-2 (374) sorts before the
	// keyless model-b group (11); the 330/374 distinction keeps the order
	// unambiguous.
	require.Len(t, result.Entries, 4)

	first := result.Entries[0]
	require.Equal(t, ProviderKeyModelUsageEntry{
		Model:             "model-b",
		KeyID:             "key-1",
		KeyName:           "simon",
		Requests:          1,
		Successes:         1,
		PromptTokens:      2000,
		CompletionTokens:  200,
		CachedReadTokens:  1500,
		TotalTokens:       2200,
		AvgTotalTokens:    2200,
		RequestsPerMinute: 1.0 / 60.0,
		TokensPerMinute:   2200.0 / 60.0,
	}, first)

	second := result.Entries[1]
	require.Equal(t, "model-a", second.Model)
	require.Equal(t, "key-1", second.KeyID)
	require.Equal(t, int64(1), second.Requests)
	require.Equal(t, int64(1), second.Successes)
	require.Equal(t, int64(0), second.Errors)
	require.Equal(t, int64(0), second.Cancelled)
	require.Equal(t, int64(1000), second.PromptTokens)
	require.Equal(t, int64(100), second.CompletionTokens)
	require.Equal(t, int64(500), second.CachedReadTokens)
	require.Equal(t, int64(1100), second.TotalTokens)
	require.InDelta(t, 1100.0, second.AvgTotalTokens, 1e-9)

	third := result.Entries[2]
	require.Equal(t, "model-a", third.Model)
	require.Equal(t, "key-2", third.KeyID)
	require.Equal(t, "lily", third.KeyName)
	require.Equal(t, int64(2), third.Requests)
	require.Equal(t, int64(0), third.Successes)
	require.Equal(t, int64(1), third.Errors)
	require.Equal(t, int64(1), third.Cancelled)
	require.Equal(t, int64(340), third.PromptTokens)
	require.Equal(t, int64(34), third.CompletionTokens)
	require.Equal(t, int64(374), third.TotalTokens)
	require.InDelta(t, 187.0, third.AvgTotalTokens, 1e-9)
	require.InDelta(t, 2.0/60.0, third.RequestsPerMinute, 1e-12)
	require.InDelta(t, 374.0/60.0, third.TokensPerMinute, 1e-12)
	// Terminal statuses account for every counted request in the group.
	require.Equal(t, third.Requests, third.Successes+third.Errors+third.Cancelled)

	// Keyless group: empty key id / name is a normal group, not dropped.
	keyless := result.Entries[3]
	require.Equal(t, "model-b", keyless.Model)
	require.Equal(t, "", keyless.KeyID)
	require.Equal(t, "", keyless.KeyName)
	require.Equal(t, int64(1), keyless.Requests)
	require.Equal(t, int64(11), keyless.TotalTokens)
}

func TestProviderKeyModelUsageTieBreaksByModelThenKey(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	start := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	// Same total_tokens everywhere: the stable order must be model asc, then
	// key_id asc, regardless of insertion order.
	rows := []*Log{
		usageLog("t1", "sensenova", "model-b", "key-2", "b2", "success", start.Add(time.Minute), 10, 0, 0, 10),
		usageLog("t2", "sensenova", "model-b", "key-1", "b1", "success", start.Add(time.Minute), 10, 0, 0, 10),
		usageLog("t3", "sensenova", "model-a", "key-2", "a2", "success", start.Add(time.Minute), 10, 0, 0, 10),
		usageLog("t4", "sensenova", "model-a", "key-1", "a1", "success", start.Add(time.Minute), 10, 0, 0, 10),
	}
	for _, row := range rows {
		require.NoError(t, store.Create(ctx, row), "Create(%s)", row.ID)
	}

	result, err := store.GetProviderKeyModelUsage(ctx, "sensenova", start, end)
	require.NoError(t, err)
	require.Len(t, result.Entries, 4)

	got := make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		got = append(got, entry.Model+"/"+entry.KeyID)
	}
	require.Equal(t, []string{"model-a/key-1", "model-a/key-2", "model-b/key-1", "model-b/key-2"}, got)
}

func TestProviderKeyModelUsageEmptyWindow(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	start := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

	result, err := store.GetProviderKeyModelUsage(ctx, "sensenova", start, start.Add(time.Hour))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Entries, "entries must be an empty slice, never nil")
	require.Empty(t, result.Entries)

	// A window with rows for another provider is still empty.
	require.NoError(t, store.Create(ctx, usageLog("e1", "openai", "gpt-4o", "k", "n", "success", start.Add(time.Minute), 1, 1, 0, 2)))
	result, err = store.GetProviderKeyModelUsage(ctx, "sensenova", start, start.Add(time.Hour))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result.Entries)
}

func TestProviderKeyModelUsageWindowBoundary(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	start := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	rows := []*Log{
		usageLog("b-before", "sensenova", "model-a", "key-1", "simon", "success", start.Add(-time.Second), 1, 1, 0, 30),
		usageLog("b-start", "sensenova", "model-a", "key-1", "simon", "success", start, 1, 1, 0, 10),
		usageLog("b-end", "sensenova", "model-a", "key-1", "simon", "success", end, 1, 1, 0, 20),
		usageLog("b-after", "sensenova", "model-a", "key-1", "simon", "success", end.Add(time.Second), 1, 1, 0, 40),
	}
	for _, row := range rows {
		require.NoError(t, store.Create(ctx, row), "Create(%s)", row.ID)
	}

	result, err := store.GetProviderKeyModelUsage(ctx, "sensenova", start, end)
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	// Both bounds are inclusive (>= start, <= end); the outside rows are not.
	require.Equal(t, int64(2), result.Entries[0].Requests)
	require.Equal(t, int64(30), result.Entries[0].TotalTokens)
}
