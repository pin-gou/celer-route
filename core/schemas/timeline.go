// Package schemas — TimelineEvent definitions shared between core (the
// per-request writer in handleProviderRequest / handleProviderStreamRequest)
// and the logstore persistence layer. Both packages can import schemas
// without creating an import cycle (logstore depends on schemas, never the
// other way).
//
// The fields here mirror the columns on framework/logstore.TimelineEvent;
// the logstore struct embeds/extends this with GORM tags. Construction in
// core uses these fields directly; the logstore BatchCreate MCPToolLogs-
// equivalent path serializes them as-is.
package schemas

import "time"

// TimelineEvent is the in-process representation of one row in the
// timeline_events table. Core writes instances of this type into
// BifrostContextKeyUpstreamSpans; the logging plugin copies the slice into
// its pending batch and the logstore persists the final form.
type TimelineEvent struct {
	ID           string
	LogID        string
	Phase        string
	Source       string
	PluginName   string
	Level        string
	Message      string
	TimeOffsetMS float64
	DurationMS   float64
	Timestamp    time.Time
	Provider     string
	Model        string
	KeyID        string
	Status       string
}