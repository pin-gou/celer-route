package rtk

import (
	"github.com/pin-gou/celer-route/core/schemas"
)

// estimateRequestTokens estimates the total token count across all messages
// in the request. It handles both ChatCompletion and Responses request types.
// Returns 0 when the request is nil or has no messages.
func estimateRequestTokens(req *schemas.BifrostRequest) int {
	if req == nil {
		return 0
	}

	var total int

	// Estimate tokens from chat request messages.
	if req.ChatRequest != nil {
		for _, msg := range req.ChatRequest.Input {
			if msg.Content != nil {
				if msg.Content.ContentStr != nil {
					total += estimateTokens(*msg.Content.ContentStr)
				}
				for _, block := range msg.Content.ContentBlocks {
					if block.Text != nil {
						total += estimateTokens(*block.Text)
					}
				}
			}
			// Include tool call arguments in the estimation.
			if msg.ChatAssistantMessage != nil {
				for _, tc := range msg.ChatAssistantMessage.ToolCalls {
					if tc.Function.Arguments != "" {
						total += estimateTokens(tc.Function.Arguments)
					}
				}
			}
		}
	}

	// Estimate tokens from responses-API request messages.
	if req.ResponsesRequest != nil {
		for _, msg := range req.ResponsesRequest.Input {
			if msg.ResponsesToolMessage != nil {
				if msg.ResponsesToolMessage.Output != nil {
					if msg.ResponsesToolMessage.Output.ResponsesToolCallOutputStr != nil {
						total += estimateTokens(*msg.ResponsesToolMessage.Output.ResponsesToolCallOutputStr)
					}
					for _, block := range msg.ResponsesToolMessage.Output.ResponsesFunctionToolCallOutputBlocks {
						if block.Text != nil {
							total += estimateTokens(*block.Text)
						}
					}
				}
				if msg.ResponsesToolMessage.Arguments != nil {
					total += estimateTokens(*msg.ResponsesToolMessage.Arguments)
				}
			}
		}
	}

	return total
}

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

	// MinTokensToCompress threshold check: when the estimated request tokens
	// are below the configured minimum, skip the entire compression pipeline.
	// This is a performance optimisation: small requests that don't benefit
	// from compression are passed through unchanged.
	//
	// The recovery-hint system message is still injected on this path — the
	// hint is about telling the LLM how to fetch a truncated tool_result in
	// the FUTURE, not about what happened to this request. Skipping it here
	// would make the hint appear only on large requests, which would break
	// prompt-cache prefix stability (the system block would then vary in
	// content from turn to turn).
	if p.config.MinTokensToCompress > 0 {
		estimated := estimateRequestTokens(req)
		if estimated < p.config.MinTokensToCompress {
			if p.logger != nil {
				p.logger.Debug("rtk", "skipping compression: estimated=%d < min_tokens_to_compress=%d", estimated, p.config.MinTokensToCompress)
			}
			if p.config.Enabled {
				injectRtkRecoveryHint(ctx, req)
			}
			return req, nil, nil
		}
	}

	// Build the pipeline runner from the global catalog and the config's pipeline.
	// This is the production path that routes through EngineCatalog + PipelineRunner,
	// ensuring the CompressionEngine interface is actually used at runtime.
	// Ensure the rtk engine is registered in the catalog (safe to call multiple times).
	globalCatalog.RegisterEngine("rtk", &rtkEngine{plugin: p})
	applyConfigDefaults(p.config)
	runner := NewPipelineRunner(globalCatalog)
	pipeline := &Pipeline{Engines: make([]string, len(p.config.Pipeline))}
	for i, step := range p.config.Pipeline {
		pipeline.Engines[i] = step.ID
	}
	defaultCfg := EngineConfig{
		Enabled:  true,
		Settings: nil,
	}

	switch req.RequestType {
	case schemas.ChatCompletionRequest, schemas.ChatCompletionStreamRequest:
		if req.ChatRequest == nil {
			return req, nil, nil
		}
		state := applyRtkCompression(ctx, req, p, runner, pipeline, defaultCfg)
		p.setState(ctx, state)
	case schemas.ResponsesRequest, schemas.ResponsesStreamRequest:
		if req.ResponsesRequest == nil {
			return req, nil, nil
		}
		state := applyRtkCompressionResponses(ctx, req, p, runner, pipeline, defaultCfg)
		p.setState(ctx, state)
	}

	// System-message hint: whenever RTK is enabled, prepend a literal-constant
	// instruction to the request's leading system messages so the LLM knows
	// how to recover a truncated tool_result via /api/context/rtk/raw-output.
	// The string is byte-stable across calls so Anthropic / OpenAI prompt
	// caches still hit on the system prefix.
	if p.config.Enabled {
		injectRtkRecoveryHint(ctx, req)
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
	ctx.SetValue(schemas.BifrostContextKeyRTKTechniques, state.Techniques)
	ctx.SetValue(schemas.BifrostContextKeyRTKFilterMatched, state.FilterMatched)
	if state.OriginalTokens > 0 {
		ratio := 1.0 - float64(state.CompressedTokens)/float64(state.OriginalTokens)
		if ratio < 0 {
			ratio = 0
		}
		ctx.SetValue(schemas.BifrostContextKeyRTKCompressionRatio, ratio)
	}
	if len(state.RawOutputPointers) > 0 && state.RawOutputPointers[0] != nil {
		ctx.SetValue(schemas.BifrostContextKeyRTKRawOutputID, state.RawOutputPointers[0].ID)
	}

	// Build the per-message pre-compression snapshot so the log detail view
	// can render a side-by-side diff. The compressed side is derived in the
	// UI from the (now in-place-mutated) request body, so we don't store it
	// here. Snapshot mode is configured per-plugin; "off" yields no
	// snapshot at all.
	if p.config != nil {
		original := buildSnapshot(state, p.config.SnapshotMode, p.config.SnapshotMaxBytes)
		if original != nil {
			ctx.SetValue(schemas.BifrostContextKeyRTKOriginalSnapshot, original)
			ctx.SetValue(schemas.BifrostContextKeyRTKSnapshotMode, p.config.SnapshotMode)
		}
	}

	// Clean up the per-request state to prevent memory leaks.
	p.clearCompressionState(ctx)

	return resp, bifrostErr, nil
}
