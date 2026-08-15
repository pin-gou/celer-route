package handlers

import (
	"context"
	"math"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/logstore"
)

// logStoreProviderStats is the production implementation of ProviderLogStats.
// It derives per-provider request/error/latency aggregates from a
// logstore.LogStore (config-db style logs table) using the store's own query
// surface, so it keeps working across the sqlite / postgres / hybrid backends.
type logStoreProviderStats struct {
	store logstore.LogStore
}

// newLogStatsFromLogStore wraps a log store into a ProviderLogStats adapter.
func newLogStatsFromLogStore(store logstore.LogStore) *logStoreProviderStats {
	return &logStoreProviderStats{store: store}
}

// AggregateProviderLogStats returns the log-derived aggregation values for a
// single provider: today's requests and errors (UTC day boundary), the most
// recent success / error timestamps (RFC3339), and the 24h average latency in
// milliseconds.
func (l *logStoreProviderStats) AggregateProviderLogStats(ctx context.Context, providerName schemas.ModelProvider) (int, int, string, string, int, error) {
	if l == nil || l.store == nil {
		return 0, 0, "", "", 0, nil
	}

	provider := string(providerName)
	now := time.Now().UTC()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	startOf24h := now.Add(-24 * time.Hour)

	// Today's request count
	todayStats, err := l.store.GetStats(ctx, logstore.SearchFilters{
		Providers: []string{provider},
		StartTime: &startOfToday,
	})
	if err != nil {
		return 0, 0, "", "", 0, err
	}

	// Today's error count (log status "error")
	errorStatus := "error"
	errorStats, err := l.store.GetStats(ctx, logstore.SearchFilters{
		Providers: []string{provider},
		Status:    []string{errorStatus},
		StartTime: &startOfToday,
	})
	if err != nil {
		return 0, 0, "", "", 0, err
	}

	// 24h average latency
	latencyStats, err := l.store.GetStats(ctx, logstore.SearchFilters{
		Providers: []string{provider},
		StartTime: &startOf24h,
	})
	if err != nil {
		return 0, 0, "", "", 0, err
	}

	// Most recent successful request / error request (all time)
	lastUsedAt, err := latestLogTimestamp(ctx, l.store, provider, []string{"success"})
	if err != nil {
		return 0, 0, "", "", 0, err
	}
	lastErrorAt, err := latestLogTimestamp(ctx, l.store, provider, []string{errorStatus})
	if err != nil {
		return 0, 0, "", "", 0, err
	}

	var avgLatencyMs int
	if latencyStats != nil && latencyStats.AverageLatency > 0 {
		avgLatencyMs = int(math.Round(latencyStats.AverageLatency))
	}

	todayRequests, todayErrors := 0, 0
	if todayStats != nil {
		todayRequests = int(todayStats.TotalRequests)
	}
	if errorStats != nil {
		todayErrors = int(errorStats.TotalRequests)
	}

	return todayRequests, todayErrors, lastUsedAt, lastErrorAt, avgLatencyMs, nil
}

// latestLogTimestamp returns the RFC3339 timestamp of the most recent log row
// matching the provider and statuses, or an empty string when none exists.
func latestLogTimestamp(ctx context.Context, store logstore.LogStore, provider string, statuses []string) (string, error) {
	result, err := store.SearchLogs(ctx, logstore.SearchFilters{
		Providers: []string{provider},
		Status:    statuses,
	}, logstore.PaginationOptions{
		Limit:  1,
		SortBy: "timestamp",
		Order:  "desc",
	})
	if err != nil {
		return "", err
	}
	if result == nil || len(result.Logs) == 0 {
		return "", nil
	}
	return result.Logs[0].Timestamp.UTC().Format(time.RFC3339), nil
}
