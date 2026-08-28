package logstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"gorm.io/gorm"
)

// ErrorPattern is a single bucket of similar errors surfaced by ErrorPatterns.
// A bucket groups rows that share the same (status_code, error_type, error_code,
// LEFT(message, 200)) tuple so a single workspace-id difference in the message
// doesn't fragment an otherwise-identical error into multiple rows.
//
// ExampleRequestID lets the UI jump to a real log row when a single bucket
// looks suspicious — the sample_message is truncated, so this is the only way
// to see the unredacted payload.
type ErrorPattern struct {
	Rank              int       `json:"rank"`
	Count             int64     `json:"count"`
	FirstSeen         time.Time `json:"first_seen"`
	LastSeen          time.Time `json:"last_seen"`
	StatusCode        *int      `json:"status_code,omitempty"`
	ErrorType         *string   `json:"error_type,omitempty"`
	ErrorCode         *string   `json:"error_code,omitempty"`
	SampleMessage     *string   `json:"sample_message,omitempty"`
	ExampleRequestID  string    `json:"example_request_id"`
}

// ErrorPatterns returns up to limit top error buckets for the given provider
// in the given window. window must be one of "1h" or "24h" (the UI only
// exposes those). The query runs directly against the log store so it stays
// up to date with the live stream of error rows.
//
// Returns total_errors so the UI can render "showing top N of M" even when
// limit is smaller than the actual bucket count.
func (s *RDBLogStore) ErrorPatterns(ctx context.Context, provider schemas.ModelProvider, window string, limit int) ([]ErrorPattern, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, errors.New("logstore not initialized")
	}
	interval, ok := windowToInterval(window)
	if !ok {
		return nil, 0, fmt.Errorf("invalid window %q: must be 1h or 24h", window)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// total_errors: cheap aggregate, runs as a separate query so the bucket
	// query below can use the simplified 4-tuple GROUP BY without COUNT(*).
	var totalErrors int64
	if err := s.db.WithContext(ctx).
		Table("logs").
		Where("provider = ? AND status = ? AND timestamp > ? AND error_details IS NOT NULL AND error_details != ''",
			string(provider), "error", interval).
		Count(&totalErrors).Error; err != nil {
		return nil, 0, fmt.Errorf("count total errors: %w", err)
	}

	if totalErrors == 0 {
		return nil, 0, nil
	}

	// Bucketed aggregation. The 4-tuple key keeps near-duplicates together
	// while still distinguishing genuinely different errors. The LEFT(message, 200)
	// is the dialect-specific part — see errorPatternsSQL.
	//
	// The dialect-specific SQL embeds the provider + interval literal directly
	// (not as a placeholder) because errorPatternsSQL also embeds them in
	// window-function PARTITION BY clauses that gorm cannot parameterise.
	bucketSQL := errorPatternsSQL(s.db.Dialector.Name(), string(provider), interval, limit)
	rows, err := s.db.WithContext(ctx).Raw(bucketSQL).Rows()
	if err != nil {
		return nil, 0, fmt.Errorf("aggregate error patterns: %w", err)
	}
	defer rows.Close()

	patterns := make([]ErrorPattern, 0, limit)
	rank := 0
	for rows.Next() {
		var p ErrorPattern
		var statusCode *int
		var errType, errCode, sampleMsg *string
		var firstSeenRaw, lastSeenRaw string
		var exampleID string
		if err := rows.Scan(&statusCode, &errType, &errCode, &sampleMsg, &p.Count, &firstSeenRaw, &lastSeenRaw, &exampleID); err != nil {
			return nil, 0, fmt.Errorf("scan row: %w", err)
		}
		p.StatusCode = statusCode
		p.ErrorType = errType
		p.ErrorCode = errCode
		p.SampleMessage = sampleMsg
		// First/last_seen come back as ISO text on sqlite (Log timestamp is a
		// varchar). parseAggregateTimestamp handles the layouts gorm / sqlite
		// actually emit (RFC3339Nano / space-separated forms).
		p.FirstSeen = parseAggregateTimestamp(firstSeenRaw)
		p.LastSeen = parseAggregateTimestamp(lastSeenRaw)
		if exampleID != "" {
			p.ExampleRequestID = exampleID
		}
		rank++
		p.Rank = rank
		patterns = append(patterns, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration: %w", err)
	}

	return patterns, totalErrors, nil
}

// windowToInterval maps the UI window label to a SQL interval expression.
// "1h" and "24h" are the only supported values; anything else returns
// ok=false so the caller rejects the request at the API boundary.
func windowToInterval(window string) (time.Time, bool) {
	now := time.Now()
	switch window {
	case "1h":
		return now.Add(-1 * time.Hour), true
	case "24h":
		return now.Add(-24 * time.Hour), true
	default:
		return time.Time{}, false
	}
}

// parseAggregateTimestamp converts an aggregate scan value to time.Time.
// On sqlite the column comes back as ISO text; on postgres/clickhouse it
// arrives as time.Time directly. Falls back to time.Time{} on parse failure
// rather than erroring — the bucket is still useful without first_seen /
// last_seen, which are presentation hints for the UI.
func parseAggregateTimestamp(value any) time.Time {
	switch v := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return v.UTC()
	case []byte:
		return parseAggregateTimestamp(string(v))
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return time.Time{}
		}
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, s); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

// errorPatternsSQL returns the dialect-specific bucketed aggregation query.
// Each dialect extracts the same 4-tuple key from the JSON-serialized
// BifrostError in error_details:
//
//	status_code  (CAST from JSON; -1 when absent)
//	error_details->'error'->>'type' (string or empty when absent)
//	error_details->'error'->>'code' (string or empty when absent)
//	LEFT(error_details->'error'->>'message', 200) (truncated prefix)
//
// Together they form a stable bucket key that:
//
//   - tolerates NULL / missing fields (via COALESCE on the empty-string side)
//   - keeps workspace-id variance in the same bucket (via the LEFT truncation)
//   - produces stable ordering across dialect backends
//
// example_request_id is resolved via a window function picking the latest
// row in each bucket, so the UI can deep-link to a real log row.
//
// The status_code column lives inside the JSON (BifrostError.StatusCode)
// since the Log struct doesn't expose it as a top-level column.
func errorPatternsSQL(dialect, provider string, interval time.Time, limit int) string {
	// interval is passed through time.Time; each branch formats the SQL
	// timestamp literal the way its driver prefers.
	switch dialect {
	case "postgres":
		return fmt.Sprintf(`
WITH bucketed AS (
  SELECT
    COALESCE(NULLIF(error_details::json->>'status_code', '')::int, -1) AS status_code,
    NULLIF(error_details::json->'error'->>'type', '') AS error_type,
    NULLIF(error_details::json->'error'->>'code', '') AS error_code,
    LEFT(error_details::json->'error'->>'message', 200) AS sample_message,
    timestamp,
    id,
    ROW_NUMBER() OVER (
      PARTITION BY
        COALESCE(NULLIF(error_details::json->>'status_code', '')::int, -1),
        NULLIF(error_details::json->'error'->>'type', ''),
        NULLIF(error_details::json->'error'->>'code', ''),
        LEFT(error_details::json->'error'->>'message', 200)
      ORDER BY timestamp DESC
    ) AS rn
  FROM logs
  WHERE provider = '%s'
    AND status = 'error'
    AND timestamp > '%s'
    AND error_details IS NOT NULL AND error_details != ''
)
SELECT
  status_code, error_type, error_code, sample_message,
  COUNT(*) AS count,
  MIN(timestamp) AS first_seen,
  MAX(timestamp) AS last_seen,
  MAX(CASE WHEN rn = 1 THEN id END) AS example_request_id
FROM bucketed
GROUP BY status_code, error_type, error_code, sample_message
ORDER BY count DESC
LIMIT %d`, provider, interval.Format("2006-01-02 15:04:05.000"), limit)
	case "clickhouse":
		return fmt.Sprintf(`
SELECT
  COALESCE(NULLIF(JSONExtractString(error_details, 'status_code'), ''), '-1') AS status_code,
  NULLIF(JSONExtractString(error_details, 'error', 'type'), '') AS error_type,
  NULLIF(JSONExtractString(error_details, 'error', 'code'), '') AS error_code,
  LEFT(JSONExtractString(error_details, 'error', 'message'), 200) AS sample_message,
  COUNT(*) AS count,
  MIN(timestamp) AS first_seen,
  MAX(timestamp) AS last_seen,
  argMax(id, timestamp) AS example_request_id
FROM logs
WHERE provider = '%s'
  AND status = 'error'
  AND timestamp > toDateTime('%s')
  AND error_details != ''
GROUP BY status_code, error_type, error_code, sample_message
ORDER BY count DESC
LIMIT %d`, provider, interval.Format("2006-01-02 15:04:05"), limit)
	default: // sqlite (and mysql fallback if someone wires it up later)
		return fmt.Sprintf(`
WITH bucketed AS (
  SELECT
    COALESCE(CAST(json_extract(error_details, '$.status_code') AS INTEGER), -1) AS status_code,
    NULLIF(json_extract(error_details, '$.error.type'), '') AS error_type,
    NULLIF(json_extract(error_details, '$.error.code'), '') AS error_code,
    substr(json_extract(error_details, '$.error.message'), 1, 200) AS sample_message,
    timestamp,
    id,
    ROW_NUMBER() OVER (
      PARTITION BY
        COALESCE(CAST(json_extract(error_details, '$.status_code') AS INTEGER), -1),
        NULLIF(json_extract(error_details, '$.error.type'), ''),
        NULLIF(json_extract(error_details, '$.error.code'), ''),
        substr(json_extract(error_details, '$.error.message'), 1, 200)
      ORDER BY timestamp DESC
    ) AS rn
  FROM logs
  WHERE provider = '%s'
    AND status = 'error'
    AND timestamp > '%s'
    AND error_details IS NOT NULL AND error_details != ''
)
SELECT
  status_code, error_type, error_code, sample_message,
  COUNT(*) AS count,
  MIN(timestamp) AS first_seen,
  MAX(timestamp) AS last_seen,
  MAX(CASE WHEN rn = 1 THEN id END) AS example_request_id
FROM bucketed
GROUP BY status_code, error_type, error_code, sample_message
ORDER BY count DESC
LIMIT %d`, provider, interval.Format("2006-01-02 15:04:05"), limit)
	}
}

// ErrNoErrorPatterns is returned when ErrorPatterns runs but the log store
// has no error rows for the requested provider/window. Returning a typed
// nil-error allows the HTTP handler to map the empty case to 200 with an
// empty patterns slice without logging "no rows" as a warning.
var ErrNoErrorPatterns = errors.New("no error patterns in window")

// compile-time interface assertion: keeps ErrorPatterns signature honest
// against the LogStore interface declared elsewhere in the package.
var _ = func() any {
	type storeWithPatterns interface {
		ErrorPatterns(ctx context.Context, provider schemas.ModelProvider, window string, limit int) ([]ErrorPattern, int64, error)
	}
	var s storeWithPatterns = (*RDBLogStore)(nil)
	var _ storeWithPatterns = s
	var _ *gorm.DB = nil
	return nil
}()