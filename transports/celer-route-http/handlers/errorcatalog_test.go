package handlers

import (
	"encoding/json"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/valyala/fasthttp"
)

// decodeBody is a tiny helper for tests that need to inspect the JSON body
// the handler wrote to the response context.
func decodeBody(t *testing.T, ctx *fasthttp.RequestCtx) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("decode response body: %v\nraw=%s", err, ctx.Response.Body())
	}
	return body
}

func TestErrorCatalog_KnownProvider(t *testing.T) {
	h := NewErrorCatalogHandler()
	ctx := &fasthttp.RequestCtx{}
	ctx.QueryArgs().Set("provider", "openai")

	h.getErrorCatalog(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Fatalf("status=%d, want 200", ctx.Response.StatusCode())
	}
	body := decodeBody(t, ctx)
	if body["provider"] != "openai" {
		t.Errorf("provider=%v, want openai", body["provider"])
	}
	types, _ := body["types"].([]any)
	if len(types) == 0 {
		t.Fatalf("expected non-empty types for openai")
	}
	codes, _ := body["codes"].([]any)
	if len(codes) == 0 {
		t.Fatalf("expected non-empty codes for openai")
	}
}

func TestErrorCatalog_UnknownProviderFallsBackToGeneric(t *testing.T) {
	h := NewErrorCatalogHandler()
	ctx := &fasthttp.RequestCtx{}
	ctx.QueryArgs().Set("provider", "totally-made-up")

	h.getErrorCatalog(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Fatalf("status=%d, want 200 (unknown provider still returns 200 with generic fallback)", ctx.Response.StatusCode())
	}
	body := decodeBody(t, ctx)
	types, _ := body["types"].([]any)
	if len(types) == 0 {
		t.Fatalf("generic fallback must not be empty — UI dropdown would be dead")
	}
}

func TestErrorCatalog_EmptyProviderReturnsGeneric(t *testing.T) {
	// No ?provider= set → empty provider → generic catalog.
	h := NewErrorCatalogHandler()
	ctx := &fasthttp.RequestCtx{}

	h.getErrorCatalog(ctx)

	body := decodeBody(t, ctx)
	if body["provider"] != "" {
		t.Errorf("provider=%v, want empty string", body["provider"])
	}
	types, _ := body["types"].([]any)
	if len(types) == 0 {
		t.Fatalf("generic fallback must not be empty for empty-provider requests")
	}
}

func TestErrorCatalog_SensenovaContainsWorkspaceQuotaCode(t *testing.T) {
	// Regression: sensenova's quota exhaustion uses error.code="insufficient_quota"
	// with error.type="invalid_request_error" — the catalog must surface the
	// code so operators can match against it from the UI.
	h := NewErrorCatalogHandler()
	ctx := &fasthttp.RequestCtx{}
	ctx.QueryArgs().Set("provider", string(schemas.Sensenova))

	h.getErrorCatalog(ctx)

	body := decodeBody(t, ctx)
	codes, _ := body["codes"].([]any)
	found := false
	for _, c := range codes {
		if c == "insufficient_quota" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("sensenova catalog missing insufficient_quota in codes: %v", codes)
	}
}