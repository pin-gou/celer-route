package handlers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/valyala/fasthttp"
)

const (
	// providerUsageDefaultWindowMinutes is used when window_minutes is absent.
	providerUsageDefaultWindowMinutes = 60
	// providerUsageMinWindowMinutes / providerUsageMaxWindowMinutes bound the
	// observation window. Out-of-range values are rejected rather than
	// silently clamped so a caller can never believe it asked for a window it
	// did not get.
	providerUsageMinWindowMinutes = 5
	providerUsageMaxWindowMinutes = 1440
)

// ProviderUsageResponse is the wire shape of GET /api/providers/{provider}/usage.
type ProviderUsageResponse struct {
	Provider      string                                `json:"provider"`
	WindowMinutes int                                   `json:"window_minutes"`
	StartTime     string                                `json:"start_time"`
	EndTime       string                                `json:"end_time"`
	Entries       []logstore.ProviderKeyModelUsageEntry `json:"entries"`
}

// getProviderUsage handles GET /api/providers/{provider}/usage.
//
// It reports, per (model, selected key) pair, how many requests a provider
// served in the last N minutes and how many tokens they consumed. This is the
// operational view the dashboard's dimension histograms cannot express: they
// only break down by provider/team/customer/user/business_unit/app/user_agent,
// never by model x key, which is the pair that actually carries the quota.
func (h *ProviderHandler) getProviderUsage(ctx *fasthttp.RequestCtx) {
	provider, err := getProviderFromCtx(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	windowMinutes := providerUsageDefaultWindowMinutes
	// An absent (or empty) window_minutes falls back to the default; a
	// present-but-unparseable or out-of-range value is rejected outright.
	if raw := string(ctx.QueryArgs().Peek("window_minutes")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf(
				"invalid window_minutes %q: must be an integer between %d and %d",
				raw, providerUsageMinWindowMinutes, providerUsageMaxWindowMinutes))
			return
		}
		if parsed < providerUsageMinWindowMinutes || parsed > providerUsageMaxWindowMinutes {
			SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf(
				"invalid window_minutes %d: must be between %d and %d",
				parsed, providerUsageMinWindowMinutes, providerUsageMaxWindowMinutes))
			return
		}
		windowMinutes = parsed
	}

	if h.inMemoryStore == nil || h.inMemoryStore.LogsStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Logs store is not configured")
		return
	}

	// Truncate to the second: the contract's timestamps are RFC3339 without
	// sub-second precision, and the window bounds should match what is
	// rendered back to the caller.
	endTime := time.Now().UTC().Truncate(time.Second)
	startTime := endTime.Add(-time.Duration(windowMinutes) * time.Minute)

	result, err := h.inMemoryStore.LogsStore.GetProviderKeyModelUsage(ctx, string(provider), startTime, endTime)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get provider usage: %v", err))
		return
	}

	response := ProviderUsageResponse{
		Provider:      string(provider),
		WindowMinutes: windowMinutes,
		StartTime:     startTime.Format(time.RFC3339),
		EndTime:       endTime.Format(time.RFC3339),
		Entries:       result.Entries,
	}
	// An empty window must serialize as [] rather than null.
	if response.Entries == nil {
		response.Entries = []logstore.ProviderKeyModelUsageEntry{}
	}

	SendJSON(ctx, response)
}
