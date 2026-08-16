package handlers

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/framework/logstore"
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
// single provider over the last 1-hour rolling window: requests and errors,
// the most recent success / error timestamps within that window (RFC3339), and
// the 1-hour average latency in milliseconds.
func (l *logStoreProviderStats) AggregateProviderLogStats(ctx context.Context, providerName schemas.ModelProvider) (int, int, string, string, int, error) {
	if l == nil || l.store == nil {
		return 0, 0, "", "", 0, nil
	}

	provider := string(providerName)
	now := time.Now().UTC()
	startOfWindow := now.Add(-1 * time.Hour)

	// Requests in the last hour
	windowStats, err := l.store.GetStats(ctx, logstore.SearchFilters{
		Providers: []string{provider},
		StartTime: &startOfWindow,
	})
	if err != nil {
		return 0, 0, "", "", 0, fmt.Errorf("get last-hour stats for provider %s: %w", provider, err)
	}

	// Errors in the last hour (log status "error")
	errorStatus := "error"
	errorStats, err := l.store.GetStats(ctx, logstore.SearchFilters{
		Providers: []string{provider},
		Status:    []string{errorStatus},
		StartTime: &startOfWindow,
	})
	if err != nil {
		return 0, 0, "", "", 0, fmt.Errorf("get last-hour error stats for provider %s: %w", provider, err)
	}

	// 1-hour average latency
	latencyStats := windowStats

	var avgLatencyMs int
	if latencyStats != nil && latencyStats.AverageLatency > 0 {
		avgLatencyMs = int(math.Round(latencyStats.AverageLatency))
	}

	hourlyRequests, hourlyErrors := 0, 0
	if windowStats != nil {
		hourlyRequests = int(windowStats.TotalRequests)
	}
	if errorStats != nil {
		hourlyErrors = int(errorStats.TotalRequests)
	}

	// Most recent successful request / error request within the window
	lastUsedAt, err := latestLogTimestamp(ctx, l.store, provider, []string{"success"}, &startOfWindow)
	if err != nil {
		return 0, 0, "", "", 0, fmt.Errorf("get latest success timestamp for provider %s: %w", provider, err)
	}
	lastErrorAt, err := latestLogTimestamp(ctx, l.store, provider, []string{errorStatus}, &startOfWindow)
	if err != nil {
		return 0, 0, "", "", 0, fmt.Errorf("get latest error timestamp for provider %s: %w", provider, err)
	}

	return hourlyRequests, hourlyErrors, lastUsedAt, lastErrorAt, avgLatencyMs, nil
}

// latestLogTimestamp returns the RFC3339 timestamp of the most recent log row
// matching the provider, statuses, and (when non-nil) startTime, or an empty
// string when none exists.
func latestLogTimestamp(ctx context.Context, store logstore.LogStore, provider string, statuses []string, startTime *time.Time) (string, error) {
	filters := logstore.SearchFilters{
		Providers: []string{provider},
		Status:    statuses,
	}
	if startTime != nil {
		filters.StartTime = startTime
	}
	result, err := store.SearchLogs(ctx, filters, logstore.PaginationOptions{
		Limit:  1,
		SortBy: "timestamp",
		Order:  "desc",
	})
	if err != nil {
		return "", fmt.Errorf("search latest logs for provider %s with statuses %v: %w", provider, statuses, err)
	}
	if result == nil || len(result.Logs) == 0 {
		return "", nil
	}
	return result.Logs[0].Timestamp.UTC().Format(time.RFC3339), nil
}