package rtk

import (
	"github.com/pin-gou/pg-gateway/core/schemas"
)

// PreLLMHook implements schemas.LLMPlugin. It scans the request's input
// messages for tool output (role=tool and tool_result blocks), applies the
// RTK compression pipeline, and stores the per-request compression state
// for PostLLMHook to consume.
//
// Supported request types:
//   - ChatCompletionRequest / ChatCompletionStreamRequest (OpenAI chat format)
//   - ResponsesRequest / ResponsesStreamRequest     (Anthropic / Responses API)
func (p *Plugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if req == nil {
		return req, nil, nil
	}
	if !p.config.Enabled {
		return req, nil, nil
	}

	switch req.RequestType {
	case schemas.ChatCompletionRequest, schemas.ChatCompletionStreamRequest:
		if req.ChatRequest == nil {
			return req, nil, nil
		}
		state := applyRtkCompression(req, p.config)
		p.setState(ctx, state)
	case schemas.ResponsesRequest, schemas.ResponsesStreamRequest:
		if req.ResponsesRequest == nil {
			return req, nil, nil
		}
		state := applyRtkCompressionResponses(req, p.config)
		p.setState(ctx, state)
	}

	return req, nil, nil
}

// PostLLMHook implements schemas.LLMPlugin. It retrieves the compression
// state from PreLLMHook, rewrites the usage prompt tokens to the compressed
// value, and propagates the original/compressed token counts to the context
// for downstream plugins (e.g. logging) to read.
func (p *Plugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	state := p.getCompressionState(ctx)
	if state == nil || !state.Compressed {
		return resp, bifrostErr, nil
	}

	// Rewrite usage when a chat response exists.
	if resp != nil && resp.ChatResponse != nil && resp.ChatResponse.Usage != nil {
		usage := resp.ChatResponse.Usage
		usage.PromptTokens = state.CompressedTokens
		usage.OriginalPromptTokens = &state.OriginalTokens
		usage.CompressedPromptTokens = &state.CompressedTokens
		// Recompute total tokens to reflect the compressed prompt count.
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	// Rewrite usage when a responses-API response exists (Anthropic route /
	// OpenAI Responses API). InputTokens is the responses-format input count.
	if resp != nil && resp.ResponsesResponse != nil && resp.ResponsesResponse.Usage != nil {
		usage := resp.ResponsesResponse.Usage
		usage.InputTokens = state.CompressedTokens
		usage.OriginalPromptTokens = &state.OriginalTokens
		usage.CompressedPromptTokens = &state.CompressedTokens
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}

	// Set context values for downstream plugins (e.g. logging).
	ctx.SetValue(schemas.BifrostContextKeyOriginalPromptTokens, state.OriginalTokens)
	ctx.SetValue(schemas.BifrostContextKeyCompressedPromptTokens, state.CompressedTokens)

	// Clean up the per-request state to prevent memory leaks.
	p.clearCompressionState(ctx)

	return resp, bifrostErr, nil
}