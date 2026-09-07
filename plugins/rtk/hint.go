package rtk

import (
	"github.com/pin-gou/celer-route/core/schemas"
)

// rtkRecoveryHintText is prepended to the leading run of system messages
// of every LLM request once RTK is enabled. It is a literal constant so
// its bytes never change — that's the whole point. Anthropic and OpenAI
// both key their prompt caches on the byte-equality of the system prefix;
// as long as this string stays byte-stable across calls, every cache hit
// upstream remains valid.
//
// The text tells the LLM what the [rtk:raw_output_id=...] markers it
// occasionally sees inside tool_result blocks mean and how to recover the
// original. The instruction is deliberately framed as guidance, not a
// hard contract, because the LLM may legitimately ignore it (e.g. when the
// original isn't worth the round-trip).
//
// The leading marker is shared via schemas.RTKRecoveryHintMarker so the
// logging plugin and the UI can recognise the gateway-injected hint without
// duplicating the hint body; the rest of the text stays here and remains
// byte-stable across calls.
const rtkRecoveryHintText = schemas.RTKRecoveryHintMarker + " celer\u2011route (LLM gateway) RTK compression can truncate tool_result blocks. Only when tool_result ends with `[rtk:raw_output_id=<24hex>; orig=<size>; ttl=24h; redacted=true[; fetch=GET <url>]]`, original output canb be recovered from celer-route gateway (default 24h TTL, secrets redacted).\n- `orig=`：original output size, to evaluate if recovery is worthwhile.\n- `fetch=`：if present, copy\u2011pasteable GET URL (no Authorization header), returns redacted raw output as text/plain. Use commands like `curl -s http://celer-route-host:9080/api/context/rtk/raw-output/d32ab592936056c95698bfa0` (example) directly without saving the curl result to disk or modifing the curl result, only this way, the curl result will not be truncated again by celer-route gateway\n- No `fetch=`：recovery unavailable; celer-route lacks resolvable base URL, raw path cannot be accessed."

// injectRtkRecoveryHint prepends rtkRecoveryHintText to the request's
// leading system messages. It is idempotent within a single hook chain
// thanks to the BifrostContextKeyRTKRawOutputHintInjected dedupe marker,
// so re-entrancy from another plugin calling PreLLMHook does not duplicate
// the hint.
//
// The function is a no-op when the request is nil, when the relevant
// sub-request is nil, or when the hint has already been injected earlier
// in this chain. It deliberately does NOT check p.config.Enabled at this
// layer — the caller gates that so the trace stays predictable.
func injectRtkRecoveryHint(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) {
	if req == nil {
		return
	}
	if ctx.Value(schemas.BifrostContextKeyRTKRawOutputHintInjected) != nil {
		return
	}
	switch req.RequestType {
	case schemas.ChatCompletionRequest, schemas.ChatCompletionStreamRequest:
		if req.ChatRequest == nil {
			return
		}
		prependChatSystemMessage(&req.ChatRequest.Input, rtkRecoveryHintText)
	case schemas.ResponsesRequest, schemas.ResponsesStreamRequest:
		if req.ResponsesRequest == nil {
			return
		}
		prependResponsesSystemMessage(&req.ResponsesRequest.Input, rtkRecoveryHintText)
	default:
		return
	}
	ctx.SetValue(schemas.BifrostContextKeyRTKRawOutputHintInjected, true)
}

// prependChatSystemMessage prepends a role=system ChatMessage to the input
// slice. It allocates a new slice to keep the original references intact
// (callers upstream may hold len/cap expectations, and modifying in place
// would race with concurrent readers in some flows).
func prependChatSystemMessage(input *[]schemas.ChatMessage, text string) {
	if input == nil {
		return
	}
	hint := schemas.ChatMessage{
		Role: schemas.ChatMessageRoleSystem,
		Content: &schemas.ChatMessageContent{
			ContentStr: &text,
		},
	}
	combined := make([]schemas.ChatMessage, 0, len(*input)+1)
	combined = append(combined, hint)
	combined = append(combined, *input...)
	*input = combined
}

// prependResponsesSystemMessage prepends a "message" ResponsesMessage of
// role=system to the Responses-style input slice. Responses does not
// natively model system as a distinct field — the spec folds it into the
// generic message type — so we synthesise one with role=system and a
// text content part. Upstream adapters (OpenAI Responses API / Anthropic
// via the Responses shim) accept this shape.
func prependResponsesSystemMessage(input *[]schemas.ResponsesMessage, text string) {
	if input == nil {
		return
	}
	hint := schemas.ResponsesMessage{
		Type: ptrResponsesMessageType(schemas.ResponsesMessageTypeMessage),
		Role: ptrResponsesMessageRole(schemas.ResponsesInputMessageRoleSystem),
		Content: &schemas.ResponsesMessageContent{
			ContentStr: &text,
		},
	}
	combined := make([]schemas.ResponsesMessage, 0, len(*input)+1)
	combined = append(combined, hint)
	combined = append(combined, *input...)
	*input = combined
}

// ptrResponsesMessageType / ptrResponsesMessageRole are tiny local helpers so
// we don't have to import strings/unsafe just to take an address of a constant.
func ptrResponsesMessageType(t schemas.ResponsesMessageType) *schemas.ResponsesMessageType {
	return &t
}

func ptrResponsesMessageRole(r schemas.ResponsesMessageRoleType) *schemas.ResponsesMessageRoleType {
	return &r
}

// chatHintScanOffset returns 1 when the first chat message is the RTK recovery
// hint (prepended at input[0] by injectRtkRecoveryHint), else 0.
//
// Recorded ScannedIndices / RawOutputEntries are canonicalised to the array
// WITHOUT the hint — the client-sent message positions the log detail diff view
// computes from input_history. The hint shifts every later position by one, and
// which array ends up persisted as input_history depends on the pre-hook plugin
// order and on fallback inheritance:
//
//   - primary pass: the pipeline scans the pre-injection array (hint injected
//     after compression), so the offset is 0;
//   - fallback pass: the request inherits the primary's hint-injected array
//     (the dedupe marker prevents re-injection), so the offset is 1 and the raw
//     scan positions must be shifted back by one;
//   - plugin chains where logging runs before RTK: the persisted input_history
//     is the pre-injection array while a preceding injection would have shifted
//     the scan — the frontend skips a leading hint when numbering, so both
//     sides agree on the canonical (hint-free) positions.
func chatHintScanOffset(input []schemas.ChatMessage) int {
	if len(input) > 0 && isChatRtkHintMessage(&input[0]) {
		return 1
	}
	return 0
}

// isChatRtkHintMessage reports whether the chat message is the literal RTK
// recovery hint. Byte-equality against rtkRecoveryHintText keeps the detection
// precise: the hint is the only gateway-injected message RTK itself writes, and
// the constant is deliberately byte-stable for prompt-cache prefix stability.
func isChatRtkHintMessage(m *schemas.ChatMessage) bool {
	return m != nil &&
		m.Role == schemas.ChatMessageRoleSystem &&
		m.Content != nil &&
		m.Content.ContentStr != nil &&
		*m.Content.ContentStr == rtkRecoveryHintText
}

// responsesHintScanOffset mirrors chatHintScanOffset for Responses-format input,
// where the hint is a role=system "message" item.
func responsesHintScanOffset(input []schemas.ResponsesMessage) int {
	if len(input) > 0 && isResponsesRtkHintMessage(&input[0]) {
		return 1
	}
	return 0
}

// isResponsesRtkHintMessage reports whether the responses item is the RTK
// recovery hint (see prependResponsesSystemMessage for the shape).
func isResponsesRtkHintMessage(m *schemas.ResponsesMessage) bool {
	return m != nil &&
		m.Type != nil && *m.Type == schemas.ResponsesMessageTypeMessage &&
		m.Role != nil && *m.Role == schemas.ResponsesInputMessageRoleSystem &&
		m.Content != nil &&
		m.Content.ContentStr != nil &&
		*m.Content.ContentStr == rtkRecoveryHintText
}

// rtkCanonicalIndex subtracts the hint offset from a scanned array position,
// clamping at 0 so a defensive record can never underflow. The result is the
// position in the hint-free (client-sent) array — the canonical index the log
// detail diff view aligns on.
func rtkCanonicalIndex(scanIndex, hintOffset int) int {
	if hintOffset <= 0 {
		return scanIndex
	}
	if scanIndex <= 0 {
		return 0
	}
	return scanIndex - hintOffset
}
