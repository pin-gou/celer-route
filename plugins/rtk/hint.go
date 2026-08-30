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
const rtkRecoveryHintText = "[rtk‑recovery] celer‑route RTK compression can truncate tool_result blocks. Only when tool_result ends with `[rtk:raw_output_id=<24hex>; orig=<size>; ttl=24h; redacted=true[; fetch=GET <url>]]`, original output is truncated and saved to disk (default 24h TTL, secrets redacted).\n- `orig=`：original output size, to evaluate if recovery is worthwhile.\n- `fetch=`：if present, copy‑pasteable GET URL (no Authorization header), returns redacted raw output as text/plain. use commands like `curl -s http://localhost:9080/api/context/rtk/raw-output/d32ab592936056c95698bfa0` directly without saving it to disk, only this way, the result will not be truncated again\n- No `fetch=`：recovery unavailable; gateway lacks resolvable base URL, raw path cannot be accessed."

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