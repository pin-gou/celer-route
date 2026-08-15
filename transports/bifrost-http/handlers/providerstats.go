package handlers

import (
	"context"
	"fmt"
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
		return 0, 0, "", "", 0, fmt.Errorf("get today stats for provider %s: %w", provider, err)
	}

	// Today's error count (log status "error")
	errorStatus := "error"
	errorStats, err := l.store.GetStats(ctx, logstore.SearchFilters{
		Providers: []string{provider},
		Status:    []string{errorStatus},
		StartTime: &startOfToday,
	})
	if err != nil {
		return 0, 0, "", "", 0, fmt.Errorf("get today error stats for provider %s: %w", provider, err)
	}

	// 24h average latency
	latencyStats, err := l.store.GetStats(ctx, logstore.SearchFilters{
		Providers: []string{provider},
		StartTime: &startOf24h,
	})
	if err != nil {
		return 0, 0, "", "", 0, fmt.Errorf("get 24h latency stats for provider %s: %w", provider, err)
	}

	// Most recent successful request / error request (all time)
	lastUsedAt, err := latestLogTimestamp(ctx, l.store, provider, []string{"success"})
	if err != nil {
		return 0, 0, "", "", 0, fmt.Errorf("get latest success timestamp for provider %s: %w", provider, err)
	}
	lastErrorAt, err := latestLogTimestamp(ctx, l.store, provider, []string{errorStatus})
	if err != nil {
		return 0, 0, "", "", 0, fmt.Errorf("get latest error timestamp for provider %s: %w", provider, err)
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

// AggregateAllProviderLogStats returns the log-derived aggregation values for
// every requested provider in one batch call. It is the list-handler
// entrypoint so a providers list never loops over providers with per-provider
// store queries (N+1) — the store round trips are owned here, in a single
// chokepoint, one batch call per list request.
//
// Note on batching depth: logstore.GetStats / SearchLogs return a single
// cross-provider aggregate (no per-provider GROUP BY surface — GetStats folds
// every matching row into one SearchStats and SearchLogs caps at
// defaultMaxSearchLimit rows), and SearchFilters has no provider grouping
// dimension. So the exact per-provider aggregates still require one
// per-provider query run, executed here rather than in the caller. Adding a
// provider-grouped aggregate to logstore (a framework/logstore change outside
// this module's root) would collapse this to a constant number of queries
// without touching the handler wiring.
func (l *logStoreProviderStats) AggregateAllProviderLogStats(ctx context.Context, providerNames []schemas.ModelProvider) (map[schemas.ModelProvider]LogAgg, error) {
	result := make(map[schemas.ModelProvider]LogAgg, len(providerNames))
	if l == nil || l.store == nil {
		return result, nil
	}

	for _, providerName := range providerNames {
		todayRequests, todayErrors, lastUsedAt, lastErrorAt, avgLatencyMs, err := l.AggregateProviderLogStats(ctx, providerName)
		if err != nil {
			return nil, fmt.Errorf("batch aggregate log stats for provider %s: %w", providerName, err)
		}
		result[providerName] = LogAgg{
			TodayRequests: todayRequests,
			TodayErrors:   todayErrors,
			LastUsedAt:    lastUsedAt,
			LastErrorAt:   lastErrorAt,
			AvgLatencyMs:  avgLatencyMs,
		}
	}
	return result, nil
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
		return "", fmt.Errorf("search latest logs for provider %s with statuses %v: %w", provider, statuses, err)
	}
	if result == nil || len(result.Logs) == 0 {
		return "", nil
	}
	return result.Logs[0].Timestamp.UTC().Format(time.RFC3339), nil
}