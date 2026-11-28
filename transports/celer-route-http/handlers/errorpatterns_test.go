package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/plugins/logging"
	"github.com/valyala/fasthttp"
)

// errorPatternsLogManager answers only ErrorPatterns; every other method of the
// embedded interface panics if the endpoint under test ever reaches it.
type errorPatternsLogManager struct {
	logging.LogManager
	patterns []logstore.ErrorPattern
	total    int64
}

func (m *errorPatternsLogManager) ErrorPatterns(context.Context, schemas.ModelProvider, string, int) ([]logstore.ErrorPattern, int64, error) {
	return m.patterns, m.total, nil
}

func strPtrLocal(s string) *string { return &s }
func intPtrLocal(i int) *int       { return &i }

// TestGetErrorPatterns_LimiterDimension pins the diagnostic echo added for the
// cooldown dimension split: every bucket carries the dimension the mark path
// would decide for the same error shape, so the provider page can show it
// instead of leaving the decision invisible.
func TestGetErrorPatterns_LimiterDimension(t *testing.T) {
	mgr := &errorPatternsLogManager{
		patterns: []logstore.ErrorPattern{
			{
				Rank:          1,
				Count:         12,
				StatusCode:    intPtrLocal(429),
				ErrorType:     strPtrLocal("rate_limit_error"),
				ErrorCode:     strPtrLocal("EndpointRPMExceeded"),
				SampleMessage: strPtrLocal("rpm exhausted"),
			},
			{
				Rank:          2,
				Count:         4,
				StatusCode:    intPtrLocal(429),
				ErrorType:     strPtrLocal("rate_limit_error"),
				ErrorCode:     strPtrLocal("EndpointTPMExceeded"),
				SampleMessage: strPtrLocal("tpm exhausted"),
			},
			{
				Rank:          3,
				Count:         1,
				StatusCode:    intPtrLocal(500),
				ErrorCode:     strPtrLocal("InternalError"),
				SampleMessage: strPtrLocal("upstream boom"),
			},
		},
		total: 17,
	}
	h := &LoggingHandler{logManager: mgr}

	ctx := &fasthttp.RequestCtx{}
	ctx.QueryArgs().Set("provider", "sensenova")
	ctx.QueryArgs().Set("window", "1h")
	h.getErrorPatterns(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d: %s", got, fasthttp.StatusOK, ctx.Response.Body())
	}

	var resp struct {
		Patterns []struct {
			Rank             int    `json:"rank"`
			LimiterDimension string `json:"limiter_dimension"`
		} `json:"patterns"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Patterns) != 3 {
		t.Fatalf("got %d patterns, want 3", len(resp.Patterns))
	}
	want := []string{"rpm", "tpm", "unknown"}
	for i, w := range want {
		if resp.Patterns[i].LimiterDimension != w {
			t.Fatalf("pattern[%d].limiter_dimension = %q, want %q", i, resp.Patterns[i].LimiterDimension, w)
		}
	}
}
