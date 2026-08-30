package rtk

import (
	"fmt"
	"strings"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/plugins/rtk/renderers"
)

// ProcessStats holds token statistics for a single text compression pass.
type ProcessStats struct {
	OriginalTokens    int                    `json:"originalTokens"`
	CompressedTokens  int                    `json:"compressedTokens"`
	Techniques        []string               `json:"techniques"`
	FilterMatched     string                 `json:"filterMatched,omitempty"`
	RawOutputPointers []*RtkRawOutputPointer `json:"rawOutputPointers,omitempty"`
	// OriginalBytes is the UTF-8 byte count of the input text before compression.
	// Embedded in the [rtk:raw_output_id=...] marker so the LLM can tell at a
	// glance how much was dropped and decide whether recovery is worth the
	// round-trip. Distinct from OriginalTokens (which is an estimate) and from
	// len(persistedBytes) (which is what the recovery endpoint will actually
	// return — may differ when redaction or RawOutputMaxBytes kicks in).
	OriginalBytes int `json:"originalBytes,omitempty"`
	// Truncated signals that the result actually dropped content (smartTruncate
	// or charlimit fired). Set by the per-text pipeline so the caller can decide
	// whether to append a raw-output pointer hint to the tool_result. Distinct
	// from "compressed" — a text can be deduped/grouped without being truncated.
	Truncated bool `json:"truncated,omitempty"`
}

// applyRtkCompression is the top-level entry point for the RTK compression
// pipeline. It scans the request's messages for tool output (both OpenAI
// role=tool and Anthropic tool_result blocks), applies the compression
// pipeline through the EngineCatalog + PipelineRunner, and returns the
// per-request compression state.
//
// The request is mutated in place: compressed message content is written
// directly to the input slice. The returned CompressionState carries the
// aggregate token counts for the request.
func applyRtkCompression(ctx *schemas.BifrostContext, req *schemas.BifrostRequest, p *Plugin, runner *PipelineRunner, pipeline *Pipeline, defaultCfg EngineConfig) *CompressionState {
	state := NewCompressionState()
	if req == nil || p == nil || p.config == nil || !p.config.Enabled {
		return state
	}
	config := p.config

	if req.ChatRequest == nil || len(req.ChatRequest.Input) == 0 {
		return state
	}

	// Forward the gateway's public base URL (already resolved at request time in
	// lib.ConvertToBifrostContext) into the engine config so the raw-output
	// recovery hint can embed a complete fetch URL. Empty when the gateway is
	// reached over a channel where Host is not set (e.g. Go SDK); in that case
	// the hint falls back to the relative path form, which the system-level
	// recovery instruction already documents.
	if ctx != nil {
		if v, ok := ctx.Value(schemas.BifrostContextKeyGatewayBaseURL).(string); ok {
			defaultCfg.RecoveryBaseURL = v
		}
	}

	originalTotal := 0
	compressedTotal := 0
	anyCompressed := false

	// Build the tool call lookup for command hint resolution.
	lookup := buildToolCallLookup(req.ChatRequest.Input)
	// pendingToolCalls tracks the tool calls from the most recent assistant
	// message, used for Anthropic positional correlation of tool_result blocks
	// to the immediately preceding assistant's tool_use blocks.
	var pendingToolCalls []schemas.ChatAssistantMessageToolCall

	for i := range req.ChatRequest.Input {
		msg := &req.ChatRequest.Input[i]

		// Update pending tool calls when we see an assistant message with
		// tool_use calls (Anthropic order: assistant tool_use -> user
		// tool_result in the immediately following message).
		if msg.Role == schemas.ChatMessageRoleAssistant && msg.ChatAssistantMessage != nil && len(msg.ChatAssistantMessage.ToolCalls) > 0 {
			pendingToolCalls = msg.ChatAssistantMessage.ToolCalls
		}

		// --- OpenAI-style: role=tool with ToolCallID ---
		if msg.Role == schemas.ChatMessageRoleTool {
			text, ok := getToolContent(msg)
			if !ok || text == "" {
				continue
			}
			origTokens := estimateTokens(text)
			originalTotal += origTokens

			// Resolve the command hint from the tool call lookup.
			cmd, _ := getOpenAICommand(msg, lookup)
			cfg := defaultCfg
			cfg.CommandHint = cmd

			// Capture pre-compression text for the log detail snapshot before
			// the pipeline mutates the message.
			appendSnapshot(state, i, string(msg.Role), extractToolName(msg), text)

			// Compress through the PipelineRunner (EngineCatalog + pipeline).
			result, _, techs, filterMatched, err, ptrs := runner.Run(ctx, pipeline, text, cfg)
			if err != nil || result == "" || result == text {
				compressedTotal += origTokens
				continue
			}
			if len(ptrs) > 0 {
				state.RawOutputPointers = append(state.RawOutputPointers, ptrs...)
			}
			if filterMatched != "" && state.FilterMatched == "" {
				state.FilterMatched = filterMatched
			}

			compressedEst := estimateTokens(result)
			ratio := 1.0 - float64(compressedEst)/float64(origTokens)
			if ratio >= 0.05 {
				applyToolContent(msg, result)
				anyCompressed = true
				compressedTotal += compressedEst
				if len(techs) > 0 {
					state.Techniques = append(state.Techniques, techs...)
				} else {
					state.Techniques = append(state.Techniques, "pipeline-runner")
				}
				continue
			}
			compressedTotal += origTokens
		}

		// --- Anthropic-style: user message with tool_result blocks ---
		if msg.Content != nil && len(msg.Content.ContentBlocks) > 0 {
			blockIndex := 0
			for j := range msg.Content.ContentBlocks {
				block := &msg.Content.ContentBlocks[j]
				if !isToolResultBlock(block) {
					continue
				}
				// Preserve cache_control blocks verbatim.
				if config.PreserveCacheControl && shouldPreserveCacheControl(block) {
					blockIndex++
					continue
				}
				if block.Text == nil || *block.Text == "" {
					blockIndex++
					continue
				}
				text := *block.Text
				origTokens := estimateTokens(text)
				originalTotal += origTokens

				// Resolve the command hint from the anthropic positional correlation.
				cmd, _ := getAnthropicCommand(blockIndex, pendingToolCalls)
				cfg := defaultCfg
				cfg.CommandHint = cmd
				blockIndex++

				// Capture the original block text so the snapshot survives
				// the in-place mutation that follows.
				appendSnapshot(state, i*100+j, string(msg.Role), extractToolName(msg), text)

				// Compress through the PipelineRunner.
				result, _, techs, filterMatched, err, ptrs := runner.Run(ctx, pipeline, text, cfg)
				if err != nil || result == "" || result == text {
					compressedTotal += origTokens
					continue
				}
				if len(ptrs) > 0 {
					state.RawOutputPointers = append(state.RawOutputPointers, ptrs...)
				}
				if filterMatched != "" && state.FilterMatched == "" {
					state.FilterMatched = filterMatched
				}

				compressedEst := estimateTokens(result)
				ratio := 1.0 - float64(compressedEst)/float64(origTokens)
				if ratio >= 0.05 {
					block.Text = &result
					anyCompressed = true
					compressedTotal += compressedEst
					if len(techs) > 0 {
						state.Techniques = append(state.Techniques, techs...)
					} else {
						state.Techniques = append(state.Techniques, "pipeline-runner")
					}
					continue
				}
				compressedTotal += origTokens
			}
		}

		// --- Assistant message compression (V-plugins-2) ---
		// Compress assistant text when ApplyToAssistantMessages or
		// ApplyToCodeBlocks (with ``` fences) is enabled.
		if msg.Role == schemas.ChatMessageRoleAssistant && msg.Content != nil {
			mode := assistantCompressionMode(msg, config)
			if mode != "" {
				// OpenAI-style: ContentStr
				if msg.Content.ContentStr != nil {
					text := *msg.Content.ContentStr
					if text != "" {
						origTokens := estimateTokens(text)
						originalTotal += origTokens
						appendSnapshot(state, i, string(msg.Role), extractToolName(msg), text)
						// Assistant messages go through the pipeline runner too.
						result, _, techs, filterMatched, err, ptrs := runner.Run(ctx, pipeline, text, defaultCfg)
						if err == nil && result != "" && result != text {
							if len(ptrs) > 0 {
								state.RawOutputPointers = append(state.RawOutputPointers, ptrs...)
							}
							if filterMatched != "" && state.FilterMatched == "" {
								state.FilterMatched = filterMatched
							}
							ratio := 1.0 - float64(estimateTokens(result))/float64(origTokens)
							if ratio >= 0.05 {
								msg.Content.ContentStr = &result
								anyCompressed = true
								compressedTotal += estimateTokens(result)
								if len(techs) > 0 {
									state.Techniques = append(state.Techniques, techs...)
								} else {
									state.Techniques = append(state.Techniques, "pipeline-runner")
								}
							} else {
								compressedTotal += origTokens
							}
						} else {
							compressedTotal += origTokens
						}
					}
				}
				// Anthropic-style: ContentBlocks with text blocks
				if len(msg.Content.ContentBlocks) > 0 {
					for j := range msg.Content.ContentBlocks {
						block := &msg.Content.ContentBlocks[j]
						if block.Type != schemas.ChatContentBlockTypeText || block.Text == nil {
							continue
						}
						// Preserve cache_control-marked text blocks verbatim.
						if config.PreserveCacheControl && block.CacheControl != nil {
							continue
						}
						text := *block.Text
						if text == "" {
							continue
						}
						origTokens := estimateTokens(text)
						originalTotal += origTokens
						appendSnapshot(state, i*100+j, string(msg.Role), extractToolName(msg), text)
						result, _, techs, filterMatched, err, ptrs := runner.Run(ctx, pipeline, text, defaultCfg)
						if err == nil && result != "" && result != text {
							if len(ptrs) > 0 {
								state.RawOutputPointers = append(state.RawOutputPointers, ptrs...)
							}
							if filterMatched != "" && state.FilterMatched == "" {
								state.FilterMatched = filterMatched
							}
							ratio := 1.0 - float64(estimateTokens(result))/float64(origTokens)
							if ratio >= 0.05 {
								block.Text = &result
								anyCompressed = true
								compressedTotal += estimateTokens(result)
								if len(techs) > 0 {
									state.Techniques = append(state.Techniques, techs...)
								} else {
									state.Techniques = append(state.Techniques, "pipeline-runner")
								}
							} else {
								compressedTotal += origTokens
							}
						} else {
							compressedTotal += origTokens
						}
					}
				}
			}
		}
	}

	if anyCompressed {
		state.Compressed = true
		state.OriginalTokens = originalTotal
		state.CompressedTokens = compressedTotal
	}
	if p.metrics != nil {
		p.metrics.RecordInvocation(anyCompressed, originalTotal, compressedTotal)
	}
	return state
}

// applyRtkCompressionWithDefaults wraps applyRtkCompression with a default
// PipelineRunner and pipeline built from the plugin's config. This is a
// convenience wrapper for backward compatibility with existing tests.
func applyRtkCompressionWithDefaults(req *schemas.BifrostRequest, p *Plugin) *CompressionState {
	if p == nil || p.config == nil {
		return applyRtkCompression(nil, req, p, nil, nil, EngineConfig{})
	}
	// Ensure config defaults are applied so Pipeline is non-nil.
	applyConfigDefaults(p.config)
	// Create a local catalog with the rtk engine registered, so the pipeline
	// runner can find and execute it.
	catalog := NewEngineCatalog()
	catalog.RegisterEngine("rtk", &rtkEngine{plugin: p})
	runner := NewPipelineRunner(catalog)
	pipeline := &Pipeline{Engines: make([]string, len(p.config.Pipeline))}
	for i, step := range p.config.Pipeline {
		pipeline.Engines[i] = step.ID
	}
	return applyRtkCompression(nil, req, p, runner, pipeline, EngineConfig{Enabled: true})
}

// applyRtkCompressionResponsesWithDefaults wraps applyRtkCompressionResponses
// with a default PipelineRunner and pipeline. Convenience for backward compat.
func applyRtkCompressionResponsesWithDefaults(req *schemas.BifrostRequest, p *Plugin) *CompressionState {
	if p == nil || p.config == nil {
		return applyRtkCompressionResponses(nil, req, p, nil, nil, EngineConfig{})
	}
	// Ensure config defaults are applied so Pipeline is non-nil.
	applyConfigDefaults(p.config)
	catalog := NewEngineCatalog()
	catalog.RegisterEngine("rtk", &rtkEngine{plugin: p})
	runner := NewPipelineRunner(catalog)
	pipeline := &Pipeline{Engines: make([]string, len(p.config.Pipeline))}
	for i, step := range p.config.Pipeline {
		pipeline.Engines[i] = step.ID
	}
	return applyRtkCompressionResponses(nil, req, p, runner, pipeline, EngineConfig{Enabled: true})
}

// applyRtkCompressionResponses is the Responses-API / Anthropic-route entry
// point for the RTK compression pipeline. It scans the responses-format input
// items for tool output (Anthropic tool_result blocks normalise into
// function_call_output items carrying the tool text in
// ResponsesToolMessage.Output), applies the compression pipeline, and returns
// the per-request compression state. The request is mutated in place.
//
// cache_control protection is honoured: function_call_output items carrying a
// CacheControl (Anthropic tool_result with cache_control) are preserved
// verbatim when config.PreserveCacheControl is enabled.
func applyRtkCompressionResponses(ctx *schemas.BifrostContext, req *schemas.BifrostRequest, p *Plugin, runner *PipelineRunner, pipeline *Pipeline, defaultCfg EngineConfig) *CompressionState {
	state := NewCompressionState()
	if req == nil || p == nil || p.config == nil || !p.config.Enabled {
		return state
	}
	config := p.config

	if req.ResponsesRequest == nil || len(req.ResponsesRequest.Input) == 0 {
		return state
	}

	// Forward the gateway's public base URL (resolved at request time) into the
	// engine config so the raw-output recovery hint can embed a complete fetch
	// URL. See applyRtkCompression for the full rationale.
	if ctx != nil {
		if v, ok := ctx.Value(schemas.BifrostContextKeyGatewayBaseURL).(string); ok {
			defaultCfg.RecoveryBaseURL = v
		}
	}

	input := req.ResponsesRequest.Input

	originalTotal := 0
	compressedTotal := 0
	anyCompressed := false

	// Build the command lookup from function_call messages for command hint
	// resolution (positional correlation with function_call_output items).
	commands := buildResponsesCommandLookup(input)
	callIdx := 0

	for i := range input {
		msg := &input[i]
		if msg.Type == nil || *msg.Type != schemas.ResponsesMessageTypeFunctionCallOutput {
			continue
		}
		if msg.ResponsesToolMessage == nil || msg.ResponsesToolMessage.Output == nil {
			callIdx++
			continue
		}
		out := msg.ResponsesToolMessage.Output

		// Preserve cache_control-marked tool outputs verbatim.
		if config.PreserveCacheControl && msg.CacheControl != nil {
			callIdx++
			continue
		}

		// Extract the tool output text.
		var text string
		if out.ResponsesToolCallOutputStr != nil {
			text = *out.ResponsesToolCallOutputStr
		} else if len(out.ResponsesFunctionToolCallOutputBlocks) > 0 {
			for _, block := range out.ResponsesFunctionToolCallOutputBlocks {
				if block.Text != nil {
					text += *block.Text
				}
			}
		}
		if text == "" {
			callIdx++
			continue
		}

		origTokens := estimateTokens(text)
		originalTotal += origTokens

		// Resolve the command hint from the positional correlation.
		cmd, _ := responsesCommandAt(commands, callIdx)
		cfg := defaultCfg
		cfg.CommandHint = cmd
		callIdx++

		// Capture pre-compression text for the log detail snapshot.
		name := ""
		if msg.ResponsesToolMessage.Name != nil {
			name = *msg.ResponsesToolMessage.Name
		}
		appendSnapshot(state, i, "tool", name, text)

		// Compress through the PipelineRunner.
		result, _, techs, filterMatched, err, ptrs := runner.Run(ctx, pipeline, text, cfg)
		if err != nil || result == "" || result == text {
			compressedTotal += origTokens
			continue
		}
		if len(ptrs) > 0 {
			state.RawOutputPointers = append(state.RawOutputPointers, ptrs...)
		}
		if filterMatched != "" && state.FilterMatched == "" {
			state.FilterMatched = filterMatched
		}

		compressedEst := estimateTokens(result)
		ratio := 1.0 - float64(compressedEst)/float64(origTokens)
		if ratio >= 0.05 {
			applyResponsesToolOutput(out, config, result)
			anyCompressed = true
			compressedTotal += compressedEst
			if len(techs) > 0 {
				state.Techniques = append(state.Techniques, techs...)
			} else {
				state.Techniques = append(state.Techniques, "pipeline-runner")
			}
			continue
		}
		compressedTotal += origTokens
	}

	if anyCompressed {
		state.Compressed = true
		state.OriginalTokens = originalTotal
		state.CompressedTokens = compressedTotal
	}
	if p.metrics != nil {
		p.metrics.RecordInvocation(anyCompressed, originalTotal, compressedTotal)
	}
	return state
}

// buildResponsesCommandLookup scans input items for function_call messages and
// returns a slice of commands keyed by call index, in order. The command is the
// full tool-call arguments JSON — the same convention the OpenAI chat adapter
// (getOpenAICommand) uses — so filter matching behaves identically on both
// request paths. Non-shell tools contribute an empty slot (no command hint).
func buildResponsesCommandLookup(input []schemas.ResponsesMessage) []string {
	var commands []string
	for i := range input {
		msg := &input[i]
		if msg.Type == nil || *msg.Type != schemas.ResponsesMessageTypeFunctionCall {
			continue
		}
		if msg.ResponsesToolMessage == nil {
			commands = append(commands, "")
			continue
		}
		name := ""
		if msg.ResponsesToolMessage.Name != nil {
			name = *msg.ResponsesToolMessage.Name
		}
		if !isShellTool(name) || msg.ResponsesToolMessage.Arguments == nil {
			commands = append(commands, "")
			continue
		}
		commands = append(commands, extractCommandFromArguments(*msg.ResponsesToolMessage.Arguments))
	}
	return commands
}

// responsesCommandAt returns the command at the given call index (positional
// correlation), or empty when out of range.
func responsesCommandAt(commands []string, idx int) (string, bool) {
	if idx < 0 || idx >= len(commands) {
		return "", false
	}
	return commands[idx], commands[idx] != ""
}

// applyResponsesToolOutput writes the compressed text back to a
// function_call_output item, preserving the block/kangourou shape and cache_control.
func applyResponsesToolOutput(out *schemas.ResponsesToolMessageOutputStruct, config *Config, text string) {
	if out == nil {
		return
	}
	if out.ResponsesToolCallOutputStr != nil {
		out.ResponsesToolCallOutputStr = &text
		return
	}
	if len(out.ResponsesFunctionToolCallOutputBlocks) > 0 {
		// Preserve per-block cache_control (compress the text on the first
		// text block, leave cache_control-marked text blocks untouched to
		// honour cache_control protection).
		for i := range out.ResponsesFunctionToolCallOutputBlocks {
			block := &out.ResponsesFunctionToolCallOutputBlocks[i]
			if block.Type == schemas.ResponsesInputMessageContentBlockTypeText && block.Text != nil {
				if config.PreserveCacheControl && block.CacheControl != nil {
					continue
				}
				block.Text = &text
				return
			}
		}
	}
}

// processRtkText is the external text processing pipeline. It strips ANSI,
// detects the command, applies the matched filter, deduplicates, and
// truncates. Used by tests directly.
func processRtkText(input string, config *Config) (string, *ProcessStats) {
	return processRtkTextWithCommand(nil, input, config, nil, "", "")
}

// processRtkTextWithCommand is the internal pipeline that accepts an optional
// command hint from the tool call lookup. When commandHint is empty, content
// detection is used. When loader is nil, a throwaway builtin-only loader is
// created (for backward-compat test paths).
//
// ctx is optional: when non-nil and carrying a positive
// BifrostContextKeyRTKSentinelStripped count, the PreLLMHook entry strip has
// already removed the sentinel from this request's tool messages, so the
// pipeline's own sentinel check on the same body is redundant and skipped.
// When ctx is nil (admin test path), the in-pipeline check still fires so the
// admin handler can drive the bypass deliberately with wrapped input.
func processRtkTextWithCommand(ctx *schemas.BifrostContext, input string, config *Config, loader *FilterLoader, commandHint string, recoveryBaseURL string) (string, *ProcessStats) {
	stats := &ProcessStats{
		OriginalTokens: estimateTokens(input),
		OriginalBytes:  len(input),
		Techniques:     make([]string, 0),
	}

	if input == "" {
		stats.CompressedTokens = 0
		return input, stats
	}

	// Pre-strip bypass: when PreLLMHook already stripped a sentinel off this
	// request's tool messages, the body we are about to compress is already
	// sentinel-free. Skip our own StripRawOutputSentinel check so we do not
	// re-run the prefix match on every per-message compress call. The
	// standalone bypass path (no ctx, e.g. admin test handler) still falls
	// through to the explicit check below so the bypass contract is testable
	// from outside the hook chain.
	preStripped := false
	if ctx != nil {
		if v, ok := ctx.Value(schemas.BifrostContextKeyRTKSentinelStripped).(int); ok && v > 0 {
			preStripped = true
			stats.Techniques = append(stats.Techniques, "rtk-raw-output-bypass")
			stats.CompressedTokens = estimateTokens(input)
			return input, stats
		}
	}

	// Anti-recursion bypass: tool messages produced by an LLM calling
	// /api/context/rtk/raw-output/{id} arrive with a server-side sentinel
	// prefix. Re-compressing them yields a smaller subset plus a fresh
	// [rtk:raw_output_id=...] marker, which is the exact recursion the
	// LLM is trying to escape. Strip the sentinel and pass the body
	// through unchanged. Detection, filtering, and redaction are all
	// skipped on the bypass path because the persisted body was already
	// redacted at write time (RedactRtkRawOutput).
	if !preStripped {
		if stripped, ok := StripRawOutputSentinel(input); ok {
			stats.CompressedTokens = estimateTokens(stripped)
			stats.Techniques = append(stats.Techniques, "rtk-raw-output-bypass")
			return stripped, stats
		}
	}

	// 1. Always strip ANSI escape codes.
	text := stripANSI(input)

	// 2. Early exit for short single-line error messages.
	if isShortErrorMessage(text) {
		stats.CompressedTokens = stats.OriginalTokens
		return input, stats
	}

	// 2b. Split composite command hints: when the command hint is a composite
	// shell command (e.g. "cd frontend && npm run build"), extract the last
	// meaningful segment so the detector can match against the actual command
	// that produced the output. This is a no-op when the command has no
	// top-level && / || / ; separators.
	commandHint = lastCommandSegment(commandHint)

	// 3. Command detection.
	detection := defaultDetector.detect(text, commandHint)
	cmd := commandHint
	if cmd == "" {
		cmd = detection.Command
	}

	// 4. Non-shell output is not compressed (skip when type is unknown or
	// pure JSON). Granular types ("git-diff", "test-pytest", ...) are routed
	// through the filter matching path so they can pick a type-specific filter.
	if detection.Type == "" || detection.Type == "unknown" {
		stats.CompressedTokens = stats.OriginalTokens
		return input, stats
	}

	// 5. Document-like read protection: when detection falls back to the
	// generic shell output ({Type:"shell", Command:""}) and the text carries
	// no generic error markers, treat it as a document read. Preserve the
	// full text — only ANSI strip (already done) + dedup + grouping + the
	// hard char safety cap apply; the filter, line-filter, and smart head/tail
	// truncation steps are skipped so the document is not cut.
	isDocumentLikeRead := detection.Type == "shell" && detection.Command == "" && !hasGenericErrorMarkers(text)
	if isDocumentLikeRead {
		threshold := config.DedupThreshold
		if threshold <= 1 {
			threshold = 3
		}
		deduped, _ := applyDedup(text, threshold)
		if deduped != text && deduped != "" {
			stats.Techniques = append(stats.Techniques, "dedup")
		}
		result := deduped
		// 7b. Semantic renderers — apply even on the document-like read
		// path so a renderer registered for the `shell` (generic) type can
		// still act if ever added. Today no renderer keys on `shell`, so
		// this is a no-op for the default registry.
		if config.EnableRenderers {
			res := renderers.ApplyRenderer(result, renderers.DetectionInfo{
				Type:     detection.Type,
				Command:  detection.Command,
				Category: detection.Category,
			}, renderers.RenderConfig{
				AllowedRenderers: config.Renderers,
			})
			if res.Changed {
				result = res.Text
				stats.Techniques = append(stats.Techniques, "rtk-render:"+res.Renderer)
			}
		}
		// R5: grouping — opt-in via enable_grouping flag (default OFF).
		if config.EnableGrouping {
			groupResult := groupSimilarLines(result, GroupingOptions{Threshold: config.GroupingThreshold})
			if groupResult.Grouped > 0 {
				result = groupResult.Text
				stats.Techniques = append(stats.Techniques, "rtk-grouping")
			}
		}
		if config.MaxCharsPerResult > 0 && len(result) > config.MaxCharsPerResult {
			result = truncateToCharLimit(result, config.MaxCharsPerResult)
			result += "\n[rtk:truncated by chars]\n"
			stats.Truncated = true
			stats.Techniques = append(stats.Techniques, "charlimit")
		}
		stats.CompressedTokens = estimateTokens(result)
		maybePersistRawOutput(stats, text, config, loader, cmd)
		result = appendRawOutputHint(result, stats, recoveryBaseURL)
		stats.CompressedTokens = estimateTokens(result)
		return result, stats
	}

	// 6. Match a filter.
	if loader == nil {
		loader = NewFilterLoader(config)
	}
	filter := loader.Match(detection.Type, cmd)
	if filter == nil {
		stats.CompressedTokens = stats.OriginalTokens
		return input, stats
	}
	stats.FilterMatched = nonEmpty(filter.ID, filter.Name)

	// 7. Apply line filter rules.
	stripped := applyLineFilter(text, filter)
	if stripped != text {
		stats.Techniques = append(stats.Techniques, "linefilter")
	}

	// 7a. MatchOutput rules — content-level pattern match that collapses the
	// entire input into a single message line when a recognizable success
	// (or single-line failure) signature is detected. Runs after line filter
	// so the rule's regex sees the post-line-filter text. When a rule hits,
	// the filter pipeline terminates early — no renderers, dedup, grouping,
	// or truncate steps run on the collapsed text.
	if replaced, hit := applyMatchOutputRules(stripped, filter); hit {
		stats.CompressedTokens = estimateTokens(replaced)
		stats.Techniques = append(stats.Techniques, "matchOutput")
		stats.Truncated = stats.CompressedTokens < stats.OriginalTokens
		maybePersistRawOutput(stats, text, config, loader, cmd)
		result := appendRawOutputHint(replaced, stats, recoveryBaseURL)
		stats.CompressedTokens = estimateTokens(result)
		return result, stats
	}

	// 7b. Semantic renderers — opt-in via EnableRenderers, fail-open.
	// Aligned with OmniRoute's processRtkText step 5: a renderer applies
	// AFTER line filtering (so the input to the renderer is already
	// trimmed) and BEFORE dedup/grouping/truncate (so the renderer's
	// output is the canonical form that those later steps operate on).
	if config.EnableRenderers {
		res := renderers.ApplyRenderer(stripped, renderers.DetectionInfo{
			Type:     detection.Type,
			Command:  detection.Command,
			Category: detection.Category,
		}, renderers.RenderConfig{
			AllowedRenderers: config.Renderers,
		})
		if res.Changed {
			stripped = res.Text
			stats.Techniques = append(stats.Techniques, "rtk-render:"+res.Renderer)
		}
	}

	// 8. Deduplicate consecutive identical lines.
	threshold := config.DedupThreshold
	if threshold <= 1 {
		threshold = 3
	}
	deduped, _ := applyDedup(stripped, threshold)
	if deduped != stripped && deduped != "" {
		stats.Techniques = append(stats.Techniques, "dedup")
	}

	// 8b. R5: grouping — opt-in via enable_grouping flag (default OFF).
	// Grouping runs after dedup and before intensity-scaled truncation so
	// near-equivalent lines (differing only by timestamps/hex/numbers) are
	// collapsed before the line budget is applied.
	grouped := deduped
	if config.EnableGrouping {
		groupResult := groupSimilarLines(deduped, GroupingOptions{Threshold: config.GroupingThreshold})
		if groupResult.Grouped > 0 {
			grouped = groupResult.Text
			stats.Techniques = append(stats.Techniques, "rtk-grouping")
		}
	}

	// 9. Smart truncate with intensity-adjusted head/tail.
	effectiveFilter := scaleFilterForIntensity(filter, config.Intensity)
	// If the filter has no MaxLines, fall back to Config.MaxLinesPerResult
	// scaled by the intensity factor (aligns with OmniRoute index.ts:250).
	if effectiveFilter.MaxLines == 0 && config.MaxLinesPerResult > 0 {
		eff := *effectiveFilter
		eff.MaxLines = effectiveMaxLines(config.MaxLinesPerResult, config.Intensity)
		effectiveFilter = &eff
	}
	truncated, dropped := applySmartTruncate(grouped, effectiveFilter)
	if truncated != grouped && truncated != "" {
		stats.Techniques = append(stats.Techniques, "smarttruncate")
		if dropped > 0 {
			stats.Truncated = true
		}
	}

	// 10. Apply the char hard limit from config.
	result := truncated
	if config.MaxCharsPerResult > 0 && len(result) > config.MaxCharsPerResult {
		result = truncateToCharLimit(result, config.MaxCharsPerResult)
		result += "\n[rtk:truncated by chars]\n"
		stats.Truncated = true
		stats.Techniques = append(stats.Techniques, "charlimit")
	}

	stats.CompressedTokens = estimateTokens(result)
	maybePersistRawOutput(stats, text, config, loader, cmd)
	result = appendRawOutputHint(result, stats, recoveryBaseURL)
	// Recompute CompressedTokens so it reflects the size of the result the
	// LLM actually sees — including the optional raw-output pointer hint.
	// Tests and the logging layer rely on CompressedTokens being the wire size.
	stats.CompressedTokens = estimateTokens(result)
	return result, stats
}

// maybePersistRawOutput persists the raw tool output when the pipeline has
// actually compressed it (stats.CompressedTokens < stats.OriginalTokens — the
// D1 decision: strict alignment with OmniRoute, no 5% threshold) and the
// config's RawOutputRetention policy allows it. The returned pointer (if any)
// is accumulated onto stats.RawOutputPointers so the caller can attach it to
// the request-level CompressionState. Disk failures are best-effort — a nil
// pointer is discarded and the caller continues unaffected.
func maybePersistRawOutput(stats *ProcessStats, text string, config *Config, loader *FilterLoader, cmd string) {
	if stats == nil || config == nil {
		return
	}
	if stats.CompressedTokens >= stats.OriginalTokens {
		return
	}
	if config.RawOutputRetention == "" || config.RawOutputRetention == string(RawOutputRetentionNever) {
		return
	}
	appDir := ""
	if loader != nil {
		appDir = loader.appDir
	}
	ptr := MaybePersistRtkRawOutput(text, PersistOptions{
		Retention: RtkRawOutputRetention(config.RawOutputRetention),
		Command:   cmd,
		MaxBytes:  config.RawOutputMaxBytes,
		AppDir:    appDir,
		Dir:       config.RawOutputDir,
	})
	if ptr != nil {
		stats.RawOutputPointers = append(stats.RawOutputPointers, ptr)
	}
}

// effectiveMaxLines scales a line budget by the compression intensity.
// Returns max(1, round(base * factor)) where factor depends on intensity:
//   - minimal:   ×1.5
//   - standard:  ×1.0
//   - aggressive: ×0.5
//
// This ensures minimal/standard/aggressive produce meaningfully different
// output on truncation-based filters (V-plugins-3).
func effectiveMaxLines(base int, intensity string) int {
	switch intensity {
	case "minimal":
		// round(base * 1.5) = (base*3 + 1) / 2
		return max(1, (base*3+1)/2)
	case "aggressive":
		// round(base * 0.5) = (base + 1) / 2
		return max(1, (base+1)/2)
	default:
		return max(1, base)
	}
}

// scaleFilterForIntensity returns a copy of the filter with head/tail windows
// adjusted for the given compression intensity.
func scaleFilterForIntensity(f *Filter, intensity string) *Filter {
	if f == nil {
		return nil
	}
	if f.Head == 0 && f.Tail == 0 && f.MaxLines == 0 {
		return f
	}
	c := *f
	switch intensity {
	case "minimal":
		// Minimal: scale only MaxLines (×1.5), leave Head/Tail/maxChars untouched.
		if c.MaxLines > 0 {
			c.MaxLines = effectiveMaxLines(c.MaxLines, intensity)
		}
	case "aggressive":
		if c.Head > 0 {
			c.Head = max(1, c.Head/2)
		}
		if c.Tail > 0 {
			c.Tail = max(1, c.Tail/2)
		}
		if c.MaxLines > 0 {
			c.MaxLines = effectiveMaxLines(c.MaxLines, intensity)
		}
	}
	return &c
}

// appendRawOutputHint appends a single-line pointer to the LLM-visible
// truncated result so the LLM can recover the original via the
// /api/context/rtk/raw-output/{id} endpoint. Only fires when:
//   - stats.Truncated is true (something actually dropped)
//   - len(stats.RawOutputPointers) > 0 (retention kept an on-disk copy)
//
// The hint format is a single line so it doesn't get re-truncated by downstream
// heuristics, and uses the same bracket style as the existing
// [rtk:truncated ...] markers so the LLM treats them as a uniform family.
// The TTL value is hard-coded to "24h" to match the config default; operators
// who tune raw_output_ttl_hours can ignore the value because the LLM should
// not reason about precise retention windows — it just needs to know recovery
// is bounded.
//
// recoveryBaseURL is the gateway's public base URL, already resolved at request
// time (config override → Host header). When non-empty the marker embeds a
// complete, copy-pasteable fetch URL; when empty (Go SDK, anonymous transport)
// the marker falls back to the relative path form and the LLM is expected to
// not attempt recovery (the system-level recovery instruction makes that
// explicit).
func appendRawOutputHint(result string, stats *ProcessStats, recoveryBaseURL string) string {
	if stats == nil || !stats.Truncated {
		return result
	}
	if len(stats.RawOutputPointers) == 0 {
		return result
	}
	ptr := stats.RawOutputPointers[0]
	var b strings.Builder
	b.Grow(len(result) + 128)
	b.WriteString(result)
	b.WriteString("\n\n[rtk:raw_output_id=")
	b.WriteString(ptr.ID)
	b.WriteString("; orig=")
	b.WriteString(formatByteSize(stats.OriginalBytes))
	b.WriteString("; ttl=24h; redacted=true")
	if recoveryBaseURL != "" {
		b.WriteString("; fetch=GET ")
		b.WriteString(strings.TrimRight(recoveryBaseURL, "/"))
		b.WriteString("/api/context/rtk/raw-output/")
		b.WriteString(ptr.ID)
	}
	b.WriteString("]\n")
	return b.String()
}

// formatByteSize renders a byte count in a human-readable form for the
// raw_output_id marker (e.g. 48231 → "48.2KB", 1048576 → "1.0MB"). Stays below
// the marker's single-line invariant. Uses 1024-based units (KiB) and one
// decimal of precision, matching the convention used in the RTK admin UI.
func formatByteSize(n int) string {
	if n < 0 {
		n = 0
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1fGB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1fMB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1fKB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// truncateToCharLimit truncates the text to stay within the character limit,
// preserving full lines as much as possible.
func truncateToCharLimit(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	content := contentLines(text)
	var result []string
	chars := 0
	for _, line := range content {
		// +1 for the newline
		lineLen := len(line) + 1
		if chars+lineLen > limit {
			if len(result) == 0 {
				// Hard cut the first line.
				return text[:limit]
			}
			break
		}
		result = append(result, line)
		chars += lineLen
	}
	out := ""
	for i, line := range result {
		if i > 0 {
			out += "\n"
		}
		out += line
	}
	if hasTrailingNewline(text) {
		out += "\n"
	}
	return out
}

// getToolContent extracts the text content from a tool message, handling
// both ContentStr and ContentBlocks formats.
func getToolContent(msg *schemas.ChatMessage) (string, bool) {
	if msg == nil || msg.Content == nil {
		return "", false
	}
	if msg.Content.ContentStr != nil {
		return *msg.Content.ContentStr, true
	}
	if len(msg.Content.ContentBlocks) > 0 {
		// Concatenate text from all blocks.
		var text string
		for _, block := range msg.Content.ContentBlocks {
			if block.Text != nil {
				text += *block.Text
			}
		}
		return text, true
	}
	return "", false
}

// applyToolContent writes the compressed text back to the tool message.
func applyToolContent(msg *schemas.ChatMessage, text string) {
	if msg.Content.ContentStr != nil {
		msg.Content.ContentStr = &text
	} else if len(msg.Content.ContentBlocks) > 0 {
		// Write back to the first text block (or the tool_result block).
		for i := range msg.Content.ContentBlocks {
			if msg.Content.ContentBlocks[i].Text != nil {
				msg.Content.ContentBlocks[i].Text = &text
				return
			}
		}
		// Fallback: set the first block's text.
		if len(msg.Content.ContentBlocks) > 0 {
			msg.Content.ContentBlocks[0].Text = &text
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// assistantCompressionMode determines whether an assistant message should be
// compressed and if so, in which mode:
//   - "full": apply_to_assistant_messages=true — compress the entire text
//   - "codeOnly": apply_to_code_blocks=true AND the text contains ``` fences
//   - "" (empty): no compression applies
func assistantCompressionMode(msg *schemas.ChatMessage, config *Config) string {
	if msg == nil || config == nil || msg.Content == nil {
		return ""
	}
	if config.ApplyToAssistantMessages {
		// Always compress in full mode when the admin switch is on.
		return "full"
	}
	if config.ApplyToCodeBlocks && hasCodeFence(msg) {
		return "codeOnly"
	}
	return ""
}

// hasCodeFence returns true when the message content contains a ``` fence
// marker in any text block.
func hasCodeFence(msg *schemas.ChatMessage) bool {
	if msg == nil || msg.Content == nil {
		return false
	}
	if msg.Content.ContentStr != nil {
		return strings.Contains(*msg.Content.ContentStr, "```")
	}
	for _, block := range msg.Content.ContentBlocks {
		if block.Text != nil && strings.Contains(*block.Text, "```") {
			return true
		}
	}
	return false
}

// compressAssistantText applies the RTK compression pipeline to assistant
// text. When codeOnly is true, only the inside of ``` code fences is
// compressed; text outside the fences is preserved verbatim.
func compressAssistantText(text string, config *Config, codeOnly bool) (string, *ProcessStats) {
	if codeOnly {
		return compressCodeFences(text, config)
	}
	return processRtkText(text, config)
}

// compressCodeFences performs light-weight fence-splitting: it scans the text
// for ``` ... ``` blocks and compresses only the interior of each fence. Text
// outside fences is preserved byte-for-byte. A single O(text) pass is used to
// avoid extra whole-text copies.
func compressCodeFences(text string, config *Config) (string, *ProcessStats) {
	stats := &ProcessStats{
		OriginalTokens: estimateTokens(text),
		Techniques:     make([]string, 0),
	}
	if text == "" {
		stats.CompressedTokens = 0
		return text, stats
	}

	var sb strings.Builder
	rest := text
	changed := false

	for {
		openIdx := strings.Index(rest, "```")
		if openIdx < 0 {
			sb.WriteString(rest)
			break
		}
		// Write everything before the opening fence verbatim.
		sb.WriteString(rest[:openIdx])
		// Find the closing fence.
		fenceStart := openIdx
		closeIdx := strings.Index(rest[fenceStart+3:], "```")
		if closeIdx < 0 {
			// Unclosed fence: write the rest verbatim and stop.
			sb.WriteString(rest[fenceStart:])
			break
		}
		fenceEnd := fenceStart + 3 + closeIdx
		// The contents include the opening and closing fence delimiters.
		fullFence := rest[fenceStart : fenceEnd+3]

		// Compress the fence contents (including delimiters — the pipeline
		// treats the ``` as regular text).
		result, _ := processRtkText(fullFence, config)
		sb.WriteString(result)
		if result != fullFence {
			changed = true
		}

		// Move past the closing fence.
		rest = rest[fenceEnd+3:]
	}

	output := sb.String()
	stats.CompressedTokens = estimateTokens(output)
	if changed {
		stats.Techniques = append(stats.Techniques, "codeFence")
	}
	return output, stats
}
