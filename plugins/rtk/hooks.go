package rtk

import (
	"context"

	"github.com/pin-gou/celer-route/core/mcp"
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
				p.injectFetchToolSchemaIfAvailable(req)
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

	// Inject the rtk_fetch_raw_output tool schema into the request's tools=
	// list when the plugin opts in (InjectFetchTool default true) and the
	// tool is actually registered with the MCP manager. Source of truth is
	// the manager's live tool map (mcpManagerHasFetchTool), so multi-instance
	// / reload / test scenarios where registration did not happen are safe:
	// the schema is only exposed when the tool can actually be called.
	p.injectFetchToolSchemaIfAvailable(req)

	return req, nil, nil
}

// injectFetchToolSchemaIfAvailable appends the rtk_fetch_raw_output tool
// schema to req.Params.Tools when the plugin is enabled, InjectFetchTool is
// true, the bifrost instance is attached, the MCP manager exists, and the
// tool is currently registered in the manager's live tool map. It is a no-op
// otherwise, so callers can invoke it unconditionally at every PreLLMHook
// exit point (including the min-tokens skip path) without extra gating.
func (p *Plugin) injectFetchToolSchemaIfAvailable(req *schemas.BifrostRequest) {
	if p.bifrost == nil {
		return
	}
	p.injectFetchToolSchemaIfManagerAvailable(req, p.bifrost.GetMCPManager())
}

// injectFetchToolSchemaIfManagerAvailable is the parameterised core of
// injectFetchToolSchemaIfAvailable: it decides whether to inject based solely
// on the config and the given manager, so unit tests can exercise the branch
// logic with a mock MCPManagerLike instead of a full bifrost instance.
func (p *Plugin) injectFetchToolSchemaIfManagerAvailable(req *schemas.BifrostRequest, mgr MCPManagerLike) {
	if p == nil || p.config == nil {
		return
	}
	if !p.config.Enabled || !IsTruePtr(p.config.InjectFetchTool) || mgr == nil {
		return
	}
	if p.mcpManagerHasFetchTool(mgr) {
		p.injectFetchToolSchema(req)
	}
}

// MCPManagerLike is the minimal surface the RTK plugin needs from the MCP
// manager to discover whether the fetch tool is registered. It is declared so
// unit tests can inject a fake without spinning up a full MCPManager (which
// would require clients, network setup, etc.). *mcp.MCPManager satisfies it
// (utils.go GetToolPerClient), enforced by the var _ assertion below.
type MCPManagerLike interface {
	// GetToolPerClient returns all currently available tools grouped by
	// client name. The agent loop uses this when the LLM response includes
	// tool calls; the RTK plugin uses it as a source of truth to know
	// whether the fetch tool has been registered (and therefore should be
	// exposed in req.Params.Tools).
	GetToolPerClient(ctx context.Context) map[string][]schemas.ChatTool
}

// Compile-time assertion that the concrete MCP manager satisfies the minimal
// interface the plugin depends on.
var _ MCPManagerLike = (*mcp.MCPManager)(nil)

// mcpManagerHasFetchTool returns true when the rtk_fetch_raw_output tool is
// currently registered in the manager's tool map (under its prefixed name).
// Avoids duplicating the registration list in the plugin: instead of the
// plugin remembering whether AttachBifrost succeeded, it asks the manager.
func (p *Plugin) mcpManagerHasFetchTool(mgr MCPManagerLike) bool {
	if mgr == nil {
		return false
	}
	for _, tools := range mgr.GetToolPerClient(context.Background()) {
		for _, t := range tools {
			if t.Function != nil && t.Function.Name == rtkFetchRawOutputToolName {
				return true
			}
		}
	}
	return false
}

// injectFetchToolSchema appends RtkFetchRawOutputTool to req.Params.Tools when
// not already present. Idempotent: a no-op when the schema is already in the
// list (e.g. the client supplied it directly). Handles both the chat
// completions shape (Params.Tools []ChatTool) and the responses shape
// (Params.Tools []ResponsesTool, converted via ToResponsesTool so the schema
// is emitted in the responses-API tool shape).
func (p *Plugin) injectFetchToolSchema(req *schemas.BifrostRequest) {
	switch req.RequestType {
	case schemas.ChatCompletionRequest, schemas.ChatCompletionStreamRequest:
		if req.ChatRequest == nil || req.ChatRequest.Params == nil {
			return
		}
		if !hasTool(req.ChatRequest.Params.Tools, rtkFetchRawOutputToolName) {
			req.ChatRequest.Params.Tools = append(req.ChatRequest.Params.Tools, RtkFetchRawOutputTool)
		}
	case schemas.ResponsesRequest, schemas.ResponsesStreamRequest:
		if req.ResponsesRequest == nil || req.ResponsesRequest.Params == nil {
			return
		}
		if !hasResponsesTool(req.ResponsesRequest.Params.Tools, rtkFetchRawOutputToolName) {
			rt := RtkFetchRawOutputTool.ToResponsesTool()
			if rt != nil {
				req.ResponsesRequest.Params.Tools = append(req.ResponsesRequest.Params.Tools, *rt)
			}
		}
	}
}

// hasTool reports whether a chat-completions tools list already contains the
// given prefixed function name.
func hasTool(tools []schemas.ChatTool, name string) bool {
	for _, t := range tools {
		if t.Function != nil && t.Function.Name == name {
			return true
		}
	}
	return false
}

// hasResponsesTool reports whether a responses-API tools list already contains
// the given prefixed function name (ResponsesTool carries the name in its
// Name field, not inside a nested Function).
func hasResponsesTool(tools []schemas.ResponsesTool, name string) bool {
	for _, t := range tools {
		if t.Name != nil && *t.Name == name {
			return true
		}
	}
	return false
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
