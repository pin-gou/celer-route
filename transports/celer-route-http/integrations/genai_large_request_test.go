package integrations

import (
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/valyala/fasthttp"
)

func TestExtractModelAndRequestType_ParsesBodyWhenLargePayloadMetadataIsAbsent(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("model", "gemini-2.5-pro:generateContent")
	// OSS has no large-payload metadata (resolveLargePayloadMetadata returns nil),
	// so type detection for a small request parses the body instead.
	ctx.Request.SetBodyString(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)

	model, reqType := extractModelAndRequestType(ctx)
	if model != "gemini-2.5-pro" {
		t.Fatalf("expected normalized model gemini-2.5-pro, got %q", model)
	}
	if reqType != schemas.ResponsesRequest {
		t.Fatalf("expected responses request type from body parse, got %q", reqType)
	}
}

func TestExtractModelAndRequestType_LargeBodyHeuristicSkipsParse(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("model", "gemini-2.5-pro:generateContent")
	ctx.Request.SetBodyStream(strings.NewReader(`{"contents":[INVALID`), schemas.DefaultLargePayloadRequestThresholdBytes+1)

	model, reqType := extractModelAndRequestType(ctx)
	if model != "gemini-2.5-pro" {
		t.Fatalf("expected normalized model gemini-2.5-pro, got %q", model)
	}
	if reqType != schemas.ResponsesRequest {
		t.Fatalf("expected responses request type from large-body heuristic, got %q", reqType)
	}
}
