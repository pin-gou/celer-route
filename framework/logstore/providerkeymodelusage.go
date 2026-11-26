package logstore

import (
	"context"
	"fmt"
	"time"
)

// providerKeyModelUsageRow is the scanned shape of the aggregate query in
// GetProviderKeyModelUsage.
type providerKeyModelUsageRow struct {
	Model            string `gorm:"column:model"`
	KeyID            string `gorm:"column:key_id"`
	KeyName          string `gorm:"column:key_name"`
	Requests         int64  `gorm:"column:requests"`
	Successes        int64  `gorm:"column:successes"`
	Errors           int64  `gorm:"column:errors"`
	Cancelled        int64  `gorm:"column:cancelled"`
	PromptTokens     int64  `gorm:"column:prompt_tokens"`
	CompletionTokens int64  `gorm:"column:completion_tokens"`
	CachedReadTokens int64  `gorm:"column:cached_read_tokens"`
	TotalTokens      int64  `gorm:"column:total_tokens"`
}

// GetProviderKeyModelUsage returns one row per (model, selected_key_id) inside
// [startTime, endTime] for the given provider, ordered by total_tokens
// descending and then by model / key id ascending for a stable order.
//
// Everything here reads denormalized Log columns. Parsing token_usage or
// error_details would be both slow and dialect-dependent (the JSON functions
// differ between sqlite and postgres), so the counters and the status column
// are the only inputs.
//
// The query is restricted to terminalLogStatuses (success / error / cancelled)
// like the other log aggregates: in-flight "processing" rows have no terminal
// outcome and no final token totals, so counting them would inflate a group
// with a request that may still fail. As a direct consequence the
// successes/errors/cancelled counters always sum to requests. If a future
// status is added to Log.Status without being added to terminalLogStatuses, it
// is excluded from every counter rather than folded into one.
func (s *RDBLogStore) GetProviderKeyModelUsage(ctx context.Context, provider string, startTime, endTime time.Time) (*ProviderKeyModelUsageResult, error) {
	const selectClause = `
		model,
		selected_key_id AS key_id,
		selected_key_name AS key_name,
		COUNT(*) AS requests,
		COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS successes,
		COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0) AS errors,
		COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0) AS cancelled,
		COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
		COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
		COALESCE(SUM(cached_read_tokens), 0) AS cached_read_tokens,
		COALESCE(SUM(total_tokens), 0) AS total_tokens
	`

	rows := make([]providerKeyModelUsageRow, 0)
	if err := s.ScopedDB(ctx).Model(&Log{}).
		Select(selectClause).
		Where("provider = ?", provider).
		Where("timestamp >= ? AND timestamp <= ?", startTime, endTime).
		Where("status IN ?", terminalLogStatuses).
		Group("model, selected_key_id, selected_key_name").
		// Order on the aggregate expression rather than the `total_tokens`
		// alias so the intent is unambiguous on every dialect.
		Order("COALESCE(SUM(total_tokens), 0) DESC, model ASC, selected_key_id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get provider key/model usage: %w", err)
	}

	windowMinutes := endTime.Sub(startTime).Minutes()
	entries := make([]ProviderKeyModelUsageEntry, 0, len(rows))
	for _, r := range rows {
		entry := ProviderKeyModelUsageEntry{
			Model:            r.Model,
			KeyID:            r.KeyID,
			KeyName:          r.KeyName,
			Requests:         r.Requests,
			Successes:        r.Successes,
			Errors:           r.Errors,
			Cancelled:        r.Cancelled,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			CachedReadTokens: r.CachedReadTokens,
			TotalTokens:      r.TotalTokens,
		}
		// Guard the divisions so an empty group (defensive: COUNT(*) can never
		// be 0 for a returned row) or a zero-length window yields 0 instead of
		// NaN/Inf, which sonic and the JSON encoders would refuse to emit.
		if r.Requests > 0 {
			entry.AvgTotalTokens = float64(r.TotalTokens) / float64(r.Requests)
		}
		if windowMinutes > 0 {
			entry.RequestsPerMinute = float64(r.Requests) / windowMinutes
			entry.TokensPerMinute = float64(r.TotalTokens) / windowMinutes
		}
		entries = append(entries, entry)
	}

	return &ProviderKeyModelUsageResult{
		Provider:  provider,
		StartTime: startTime,
		EndTime:   endTime,
		Entries:   entries,
	}, nil
}
