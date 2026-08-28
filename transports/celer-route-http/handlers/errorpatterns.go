package handlers

import (
	"fmt"
	"strconv"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/valyala/fasthttp"
)

// errorPatternsResponse is the wire shape for GET /api/logs/error-patterns.
// TotalErrors lets the UI render "showing top N of M" even when limit is
// smaller than the actual bucket count. Patterns is an empty slice (not nil)
// when there are no errors in the window.
type errorPatternsResponse struct {
	Provider    string         `json:"provider"`
	Window      string         `json:"window"`
	TotalErrors int64          `json:"total_errors"`
	Patterns    []errorPattern `json:"patterns"`
}

// errorPattern is a single aggregated bucket from the RDBLogStore.
type errorPattern struct {
	Rank             int    `json:"rank"`
	Count            int64  `json:"count"`
	FirstSeen        string `json:"first_seen,omitempty"`
	LastSeen         string `json:"last_seen,omitempty"`
	StatusCode       *int   `json:"status_code,omitempty"`
	ErrorType        string `json:"error_type,omitempty"`
	ErrorCode        string `json:"error_code,omitempty"`
	SampleMessage    string `json:"sample_message,omitempty"`
	ExampleRequestID string `json:"example_request_id,omitempty"`
}

// getErrorPatterns returns error pattern clusters for the given provider.
// The store layer aggregates by (status_code, type, code, message-prefix)
// so near-duplicate errors (e.g. the same quota message with different
// workspace IDs) collapse into one bucket.
func (h *LoggingHandler) getErrorPatterns(ctx *fasthttp.RequestCtx) {
	provider := string(ctx.QueryArgs().Peek("provider"))
	if provider == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "provider is required")
		return
	}

	window := string(ctx.QueryArgs().Peek("window"))
	if window == "" {
		window = "1h"
	}
	if window != "1h" && window != "24h" {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid window %q: must be 1h or 24h", window))
		return
	}

	limitStr := string(ctx.QueryArgs().Peek("limit"))
	limit := 20
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
			if limit > 100 {
				limit = 100
			}
		}
	}

	patterns, totalErrors, err := h.logManager.ErrorPatterns(
		ctx,
		schemas.ModelProvider(provider),
		window,
		limit,
	)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to query error patterns: %v", err))
		return
	}

	resp := errorPatternsResponse{
		Provider:    provider,
		Window:      window,
		TotalErrors: totalErrors,
		Patterns:    make([]errorPattern, 0, len(patterns)),
	}
	for _, p := range patterns {
		ep := errorPattern{
			Rank:             p.Rank,
			Count:            p.Count,
			ExampleRequestID: p.ExampleRequestID,
		}
		if !p.FirstSeen.IsZero() {
			ep.FirstSeen = p.FirstSeen.UTC().Format("2006-01-02T15:04:05Z")
		}
		if !p.LastSeen.IsZero() {
			ep.LastSeen = p.LastSeen.UTC().Format("2006-01-02T15:04:05Z")
		}
		if p.StatusCode != nil {
			ep.StatusCode = p.StatusCode
		}
		if p.ErrorType != nil && *p.ErrorType != "" {
			ep.ErrorType = *p.ErrorType
		}
		if p.ErrorCode != nil && *p.ErrorCode != "" {
			ep.ErrorCode = *p.ErrorCode
		}
		if p.SampleMessage != nil && *p.SampleMessage != "" {
			ep.SampleMessage = *p.SampleMessage
		}
		resp.Patterns = append(resp.Patterns, ep)
	}

	SendJSON(ctx, resp)
}