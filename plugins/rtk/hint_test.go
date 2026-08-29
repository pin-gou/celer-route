package rtk

// Tests for the LLM-facing recovery surface:
//   - TestRecoveryHint_AlwaysPrepended            (V-rtk-8)
//   - TestRecoveryHint_Idempotent                (V-rtk-9)
//   - TestRecoveryHint_ContainsRecoveryEndpoint  (V-rtk-10)
//   - TestRecoveryHint_OnResponsesPath           (V-rtk-11)
//   - TestRecoveryHint_NotInjectedWhenDisabled   (V-rtk-12)
//   - TestRawOutputHint_AppendedOnTruncate       (V-rtk-13)
//   - TestRawOutputHint_NotAppendedWhenNoPointer (V-rtk-14)
//
// These complement the existing compression tests; they specifically
// exercise the system-message prepend (cache-friendly literal constant)
// and the in-result pointer id format.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

const chatCompletion = schemas.ChatCompletionRequest

func makeChatRequest() *schemas.BifrostRequest {
	return &schemas.BifrostRequest{
		RequestType: chatCompletion,
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleUser, Content: chatContent("hi")},
			},
		},
	}
}

func newBareCtx() *schemas.BifrostContext {
	return schemas.NewBifrostContext(context.Background(), time.Time{})
}

func chatContent(text string) *schemas.ChatMessageContent {
	return &schemas.ChatMessageContent{ContentStr: &text}
}

func TestRecoveryHint_AlwaysPrepended(t *testing.T) {
	ctx := newBareCtx()
	req := makeChatRequest()

	injectRtkRecoveryHint(ctx, req)

	if got := req.ChatRequest.Input[0].Role; got != schemas.ChatMessageRoleSystem {
		t.Fatalf("Input[0].Role = %q, want system", got)
	}
	if got := req.ChatRequest.Input[0].Content.ContentStr; got == nil {
		t.Fatal("system hint content is nil")
	} else if *got != rtkRecoveryHintText {
		t.Errorf("system hint text mismatch: got %d bytes, want %d bytes", len(*got), len(rtkRecoveryHintText))
	}
}

func TestRecoveryHint_Idempotent(t *testing.T) {
	ctx := newBareCtx()
	req := makeChatRequest()

	injectRtkRecoveryHint(ctx, req)
	injectRtkRecoveryHint(ctx, req)
	injectRtkRecoveryHint(ctx, req)

	// Three prepended calls with the same ctx must result in exactly one
	// hint at the head of Input.
	hintCount := 0
	for _, m := range req.ChatRequest.Input {
		if m.Role == schemas.ChatMessageRoleSystem && m.Content != nil && m.Content.ContentStr != nil && *m.Content.ContentStr == rtkRecoveryHintText {
			hintCount++
		}
	}
	if hintCount != 1 {
		t.Errorf("expected 1 system hint after 3 inject calls, got %d", hintCount)
	}
}

func TestRecoveryHint_ContainsRecoveryEndpoint(t *testing.T) {
	ctx := newBareCtx()
	req := makeChatRequest()
	injectRtkRecoveryHint(ctx, req)

	hint := *req.ChatRequest.Input[0].Content.ContentStr
	// Signals the LLM needs to act on the marker:
	//   - the marker format itself (raw_output_id, orig=, ttl=)
	//   - the URL path it should call when the marker carries a fetch= URL
	//   - the 24-hour retention cue
	//   - the no-auth cue (the dedicated fetch= URL requires no Authorization header
	//     when the gateway serves the endpoint unauthenticated; we still mention
	//     "Authorization" because the recovery hint text explicitly tells the LLM
	//     the dedicated URL path does not need it, and that statement mentions
	//     "Authorization" verbatim).
	for _, want := range []string{
		"[rtk:raw_output_id=<24hex>",
		"/api/context/rtk/raw-output/",
		"24h",
		"Authorization",
	} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing required phrase %q", want)
		}
	}
}

func TestRecoveryHint_OnResponsesPath(t *testing.T) {
	ctx := newBareCtx()
	req := &schemas.BifrostRequest{
		RequestType: schemas.ResponsesRequest,
		ResponsesRequest: &schemas.BifrostResponsesRequest{
			Input: []schemas.ResponsesMessage{
				{Type: ptrResponsesMessageType(schemas.ResponsesMessageTypeMessage),
					Role:    ptrResponsesMessageRole(schemas.ResponsesInputMessageRoleUser),
					Content: &schemas.ResponsesMessageContent{ContentStr: ptrResponsesString("hi")}},
			},
		},
	}
	injectRtkRecoveryHint(ctx, req)

	if got := *req.ResponsesRequest.Input[0].Role; got != schemas.ResponsesInputMessageRoleSystem {
		t.Errorf("Input[0].Role = %q, want system", got)
	}
	if got := req.ResponsesRequest.Input[0].Content; got == nil || got.ContentStr == nil || *got.ContentStr != rtkRecoveryHintText {
		t.Errorf("system hint not prepended to Responses input")
	}
}

// ptrResponsesString is a local helper (the rtk package already exports
// ptrResponsesMessageType / ptrResponsesMessageRole, but not a string
// ptr).
func ptrResponsesString(s string) *string { return &s }

func TestRawOutputHint_AppendedOnTruncate(t *testing.T) {
	// Simulate the post-pipeline state: truncated + a raw-output pointer.
	stats := &ProcessStats{
		OriginalBytes: 1024 * 48 + 231,
		Truncated:     true,
		RawOutputPointers: []*RtkRawOutputPointer{
			{ID: "0123456789abcdef01234567", Path: "/tmp/x.log", Bytes: 42, SHA256: "deadbeef", Redacted: false},
		},
	}
	got := appendRawOutputHint("truncated output...", stats, "")
	if !strings.Contains(got, "[rtk:raw_output_id=0123456789abcdef01234567") {
		t.Errorf("expected pointer id in result, got %q", got)
	}
	if !strings.Contains(got, "orig=48.2KB") {
		t.Errorf("expected orig=<size> in result, got %q", got)
	}
	if !strings.Contains(got, "ttl=24h") {
		t.Errorf("expected ttl=24h in result, got %q", got)
	}
	if !strings.Contains(got, "redacted=true") {
		t.Errorf("expected redacted=true in result, got %q", got)
	}
	if strings.Contains(got, "fetch=") {
		t.Errorf("expected no fetch= when recoveryBaseURL is empty, got %q", got)
	}
}

func TestRawOutputHint_AppendedWithFetchURL(t *testing.T) {
	stats := &ProcessStats{
		OriginalBytes: 2048,
		Truncated:     true,
		RawOutputPointers: []*RtkRawOutputPointer{
			{ID: "abcdef0123456789abcdef01", Path: "/tmp/x.log", Bytes: 2048, SHA256: "deadbeef", Redacted: true},
		},
	}
	got := appendRawOutputHint("truncated output...", stats, "http://192.168.3.18:20128")
	if !strings.Contains(got, "fetch=GET http://192.168.3.18:20128/api/context/rtk/raw-output/abcdef0123456789abcdef01") {
		t.Errorf("expected complete fetch=GET URL in result, got %q", got)
	}
	if !strings.Contains(got, "orig=2.0KB") {
		t.Errorf("expected orig=2.0KB in result, got %q", got)
	}
}

func TestRawOutputHint_NotAppendedWhenNotTruncated(t *testing.T) {
	stats := &ProcessStats{
		Truncated: false,
		RawOutputPointers: []*RtkRawOutputPointer{
			{ID: "0123456789abcdef01234567", Path: "/tmp/x.log", Bytes: 42, SHA256: "deadbeef", Redacted: false},
		},
	}
	original := "no truncation happened"
	got := appendRawOutputHint(original, stats, "")
	if got != original {
		t.Errorf("hint must not be appended when not truncated; got %q", got)
	}
}

func TestRawOutputHint_NotAppendedWhenNoPointer(t *testing.T) {
	// Edge case: text was truncated but retention is "never" so no
	// pointer is on disk. The hint would be useless (nothing to fetch),
	// so we must skip it to avoid dangling references.
	stats := &ProcessStats{Truncated: true}
	original := "truncated but no retention"
	got := appendRawOutputHint(original, stats, "")
	if got != original {
		t.Errorf("hint must not be appended when no pointer exists; got %q", got)
	}
}

// TestRecoveryHint_NotesBypass verifies the system-message hint tells the
// LLM that recovered bodies bypass re-compression. Without this note the
// LLM has no way to tell that a fresh [rtk:raw_output_id=...] marker after
// recovery means "fetch URL malformed" rather than the expected behaviour,
// so it would loop fetching forever.
func TestRecoveryHint_NotesBypass(t *testing.T) {
	must := []string{
		"unwraps the recovered body",
		"IS the file content",
		"disk copy expired",
	}
	for _, sub := range must {
		if !strings.Contains(rtkRecoveryHintText, sub) {
			t.Errorf("recovery hint must mention %q to break the recursion; full hint:\n%s", sub, rtkRecoveryHintText)
		}
	}
}