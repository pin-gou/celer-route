package logstore

import (
	"context"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// CleanupScope selects which rows a manual cleanup operation targets.
//
// The values are the same string values the HTTP layer accepts over the wire
// (POST /api/logs/cleanup.scope). Keep this enum and the HTTP request body in
// sync.
type CleanupScope string

const (
	// CleanupScopeAll deletes every row in the log store. Used by the
	// "clear everything" emergency path; the UI demands the user type
	// "CLEAN" to confirm before this is accepted.
	CleanupScopeAll CleanupScope = "all"

	// CleanupScopeOlderThan deletes rows whose Timestamp is strictly before
	// CleanupMeta.Cutoff.
	CleanupScopeOlderThan CleanupScope = "older_than"

	// CleanupScopeFilter deletes rows matching the embedded SearchFilters
	// (time window plus providers/models/status/etc.). The filter set is
	// authoritative for the run; passing an empty Filters with this scope is
	// a server-side error.
	CleanupScopeFilter CleanupScope = "filter"
)

// CleanupJobMeta is the durable state of a single log-cleanup sidekiq job.
//
// It is stored verbatim as the sidekiq job's metadata JSON so the worker can
// resume from a checkpoint after a restart, and so the polling status endpoint
// can show the same numbers the worker is committing.
type CleanupJobMeta struct {
	// Scope is one of the CleanupScope constants. Determines how the rest of
	// the meta is interpreted.
	Scope CleanupScope `json:"scope"`

	// Cutoff is the row.Timestamp < cutoff boundary used by CleanupScopeOlderThan.
	// Unused for other scopes.
	Cutoff *time.Time `json:"cutoff,omitempty"`

	// Filters is the row-matching filter set used by CleanupScopeFilter. The
	// caller is expected to have already resolved any period-style window
	// into StartTime / EndTime so the meta is a frozen snapshot.
	Filters SearchFilters `json:"filters,omitempty"`

	// StripPayloadsOnly selects payload stripping (zero out content columns)
	// instead of full-row deletion. Only valid for CleanupScopeOlderThan and
	// CleanupScopeFilter; CleanupScopeAll always hard-deletes.
	StripPayloadsOnly bool `json:"strip_payloads_only,omitempty"`

	// Total is the in-scope row count taken at enqueue time. Used for the
	// progress indicator. Treated as approximate: it can drift slightly if
	// rows are written or deleted between enqueue and the first batch.
	Total int64 `json:"total"`

	// Processed is how many in-scope rows the worker has touched so far.
	// For deletion scopes this equals Deleted; for StripPayloadsOnly this
	// equals Stripped.
	Processed int64 `json:"processed"`

	// Deleted is the number of rows hard-deleted by this job.
	Deleted int64 `json:"deleted"`

	// Stripped is the number of rows whose payload columns were zeroed by
	// this job (rows are kept).
	Stripped int64 `json:"stripped"`

	// Message is a human-readable completion note for the UI.
	Message string `json:"message,omitempty"`
}

// CleanupPreview is the lightweight estimate returned to the UI before the
// user commits to a real cleanup run. It counts rows only — no payload read.
type CleanupPreview struct {
	// MatchedLogs is the number of rows the cleanup scope would touch.
	MatchedLogs int64 `json:"matched_logs"`

	// EstimatedSizeBytes is an order-of-magnitude volume estimate computed
	// from the store's overall size plus the matched/total row ratio. It is
	// explicitly an estimate: callers should label it as such in the UI.
	EstimatedSizeBytes int64 `json:"estimated_size_bytes"`

	// Oldest and Newest bound the matched timestamp window when known; nil
	// for empty match sets or for SQLite where the aggregate is intentionally
	// skipped on the preview path to keep the latency low.
	Oldest *time.Time `json:"oldest,omitempty"`
	Newest *time.Time `json:"newest,omitempty"`
}

// StorageStats summarises the on-disk footprint of the request log store.
// Returned by LogStore.StorageStats so the settings page can show how much
// space the database is currently using without the user having to SSH in.
//
// The PayloadBreakdown fields split TotalLogs and EstimatedSizeBytes by where
// the row's request/response content lives:
//
//   - WithPayload: row carries its own payload columns (TEXT blobs of input,
//     output, tool_calls, raw_request, raw_response, …). The bulk of the
//     table's footprint lives here.
//   - Stripped:    payload columns were zeroed by the retention cleaner; the
//     row is kept for search/analytics.
//   - Offloaded:   payload lives in object storage (hybrid mode); the DB row
//     carries only metadata. SizeOffloadedBytes reports the S3 footprint
//     separately so the UI can label it "not counted in DB size".
//   - Hidden:      content logging was disabled for this request, so the
//     payload was never written. The row is metadata-only.
//
// SizeWithoutPayloadBytes + SizeWithPayloadBytes cover what the DB actually
// holds. Their sum may differ slightly from EstimatedSizeBytes because the
// per-row estimate rounds; the UI labels everything "estimated" already.
type StorageStats struct {
	// StoreType echoes the configured backend (sqlite | postgres | clickhouse)
	// so the UI can render a backend-specific note (e.g. SQLite path) without
	// having to query a second endpoint.
	StoreType string `json:"store_type"`

	// TotalLogs is the row count of the logs table.
	TotalLogs int64 `json:"total_logs"`

	// EstimatedSizeBytes is the estimated on-disk size of the logs table in
	// bytes. For SQLite this is the file size; for Postgres it is
	// pg_total_relation_size('logs'); for ClickHouse it is the sum of
	// parts.bytes_on_disk. The label is honest about being an estimate on
	// shared tablespaces where the sum can include indexes the logs table
	// shares with other tables.
	EstimatedSizeBytes int64 `json:"estimated_size_bytes"`

	// OldestLogAt and NewestLogAt bound the timestamp window actually present
	// in the table. Nil on an empty table.
	OldestLogAt *time.Time `json:"oldest_log_at,omitempty"`
	NewestLogAt *time.Time `json:"newest_log_at,omitempty"`

	// PayloadBreakdown: row counts by payload-state. Together they sum to
	// TotalLogs (any row is in exactly one bucket by construction).
	LogsWithPayload int64 `json:"logs_with_payload"`
	LogsStripped    int64 `json:"logs_stripped"`
	LogsOffloaded   int64 `json:"logs_offloaded"`
	LogsHidden      int64 `json:"logs_hidden"`

	// SizeWithoutPayloadBytes is the estimated on-disk footprint of the rows
	// in their metadata-only form (id, timestamps, provider, model, status,
	// latency, cost, token_usage, …). Roughly: stripped + hidden + offloaded
	// contribute their full per-row cost; with_payload rows contribute only
	// the metadata part.
	//
	// SizeWithPayloadBytes is the estimated footprint of the payload columns
	// themselves — what the user can claw back by stripping (without deleting
	// rows).
	//
	// SizeOffloadedBytes is the S3 footprint of offloaded payloads, reported
	// separately so the UI can label it "not in DB". Zero when hybrid mode
	// is off; on object-storage-backed setups this can be the largest of the
	// three numbers.
	SizeWithoutPayloadBytes int64 `json:"size_without_payload_bytes"`
	SizeWithPayloadBytes    int64 `json:"size_with_payload_bytes"`
	SizeOffloadedBytes      int64 `json:"size_offloaded_bytes"`
}

// LogCleanupManager is the narrow surface the cleanup worker / preview endpoint
// need from the underlying LogStore. Splitting it out keeps LogStore from
// growing new methods every time the cleanup UI gains a new mode; mock
// implementations only need to satisfy this surface.
//
// Every method honours ctx cancellation: a cancelled context returns ctx.Err()
// promptly without committing a partial batch.
type LogCleanupManager interface {
	// StorageStats returns a single snapshot of the request log store's row
	// count, on-disk size, and timestamp range. Cheap enough to call on
	// every UI poll.
	StorageStats(ctx context.Context) (*StorageStats, error)

	// CountByFilter returns how many rows the supplied filters would match
	// and the matched timestamp range. It must use the same row-visibility
	// rules SearchLogs applies, so a preview number equals what the worker
	// will eventually process. The size estimate is computed in the
	// implementation; see RDBLogStore.CountByFilter for the formula.
	CountByFilter(ctx context.Context, filters SearchFilters) (*CleanupPreview, error)

	// DeleteByFilterBatch deletes up to batchSize rows matching the supplied
	// filters, returning how many were deleted. Looping is the caller's
	// responsibility — the implementation only commits a single batch per
	// call so checkpointing can happen between batches. A nil/empty filter
	// set means "delete everything", which only CleanupScopeAll should ever
	// pass.
	DeleteByFilterBatch(ctx context.Context, filters SearchFilters, batchSize int) (deletedCount int64, err error)

	// StripPayloadsByFilterBatch clears payload columns on up to batchSize
	// rows matching the supplied filters that have not already been stripped,
	// returning how many were touched. Same looping contract as
	// DeleteByFilterBatch.
	StripPayloadsByFilterBatch(ctx context.Context, filters SearchFilters, batchSize int) (strippedCount int64, err error)
}

// Logger is the small interface the cleanup helpers need to emit progress.
// Accepting an interface here (rather than concrete *zap.Logger) keeps the
// cleanup logic free of any specific logging backend; production passes
// schemas.Logger, tests pass a no-op.
type cleanupLogger interface {
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}

// Compile-time check: schemas.Logger satisfies cleanupLogger. Keeps the
// helper signatures decoupled from the concrete type.
var _ cleanupLogger = (schemas.Logger)(nil)
