package rtk

import (
	"sync"

	"github.com/pin-gou/pg-gateway/core/schemas"
)

// ProcessStats holds token statistics for a single text compression pass.
type ProcessStats struct {
	OriginalTokens   int
	CompressedTokens int
	Techniques       []string
}

var (
	loaderOnce     sync.Once
	globalLoader   *FilterLoader
	defaultConfig  = &Config{Enabled: true}
)

// getFilterLoader returns the package-level singleton FilterLoader.
func getFilterLoader() *FilterLoader {
	loaderOnce.Do(func() {
		globalLoader = NewFilterLoader(defaultConfig)
	})
	return globalLoader
}

// applyRtkCompression is the top-level entry point for the RTK compression
// pipeline. It scans the request's messages for tool output (both OpenAI
// role=tool and Anthropic tool_result blocks), applies the compression
// pipeline, and returns the per-request compression state.
//
// The request is mutated in place: compressed message content is written
// directly to the input slice. The returned CompressionState carries the
// aggregate token counts for the request.
func applyRtkCompression(req *schemas.BifrostRequest, config *Config) *CompressionState {
	state := NewCompressionState()
	if req == nil || config == nil || !config.Enabled {
		return state
	}
	if req.ChatRequest == nil || len(req.ChatRequest.Input) == 0 {
		return state
	}

	// Build the tool call lookup from assistant messages.
	lookup := buildToolCallLookup(req.ChatRequest.Input)

	originalTotal := 0
	compressedTotal := 0
	anyCompressed := false

	// Track pending tool calls for Anthropic-style positional correlation.
	var pendingToolCalls []schemas.ChatAssistantMessageToolCall

	for i := range req.ChatRequest.Input {
		msg := &req.ChatRequest.Input[i]

		// --- OpenAI-style: role=tool with ToolCallID ---
		if msg.Role == schemas.ChatMessageRoleTool {
			text, ok := getToolContent(msg)
			if !ok || text == "" {
				continue
			}
			origTokens := estimateTokens(text)
			originalTotal += origTokens

			// Try to get the command from the tool call lookup.
			command, hasCommand := getOpenAICommand(msg, lookup)

			var result string
			var stats *ProcessStats
			if hasCommand {
				result, stats = processRtkTextWithCommand(text, config, command)
			} else {
				result, stats = processRtkText(text, config)
			}

			// Apply the result if savings are meaningful and we didn't
			// empty the content entirely.
			if result != "" && result != text && stats.CompressedTokens < stats.OriginalTokens {
				ratio := 1.0 - float64(stats.CompressedTokens)/float64(stats.OriginalTokens)
				if ratio >= 0.05 {
					applyToolContent(msg, result)
					anyCompressed = true
					compressedTotal += estimateTokens(result)
					state.Techniques = append(state.Techniques, stats.Techniques...)
					continue
				}
			}
			compressedTotal += origTokens
		}

		// --- Anthropic-style: user message with tool_result blocks ---
		if msg.Content != nil && len(msg.Content.ContentBlocks) > 0 {
			blockIdx := 0
			for j := range msg.Content.ContentBlocks {
				block := &msg.Content.ContentBlocks[j]
				if !isToolResultBlock(block) {
					continue
				}
				// Preserve cache_control blocks verbatim.
				if config.PreserveCacheControl && shouldPreserveCacheControl(block) {
					blockIdx++
					continue
				}
				if block.Text == nil || *block.Text == "" {
					blockIdx++
					continue
				}
				text := *block.Text
				origTokens := estimateTokens(text)
				originalTotal += origTokens

				// Try positional correlation.
				command, hasCommand := getAnthropicCommand(blockIdx, pendingToolCalls)

				var result string
				var stats *ProcessStats
				if hasCommand {
					result, stats = processRtkTextWithCommand(text, config, command)
				} else {
					result, stats = processRtkText(text, config)
				}

				if result != "" && result != text && stats.CompressedTokens < stats.OriginalTokens {
					ratio := 1.0 - float64(stats.CompressedTokens)/float64(stats.OriginalTokens)
					if ratio >= 0.05 {
						block.Text = &result
						anyCompressed = true
						compressedTotal += estimateTokens(result)
						state.Techniques = append(state.Techniques, stats.Techniques...)
						blockIdx++
						continue
					}
				}
				compressedTotal += origTokens
				blockIdx++
			}
		}

		// Track the last assistant message's tool calls for Anthropic correlation.
		if msg.Role == schemas.ChatMessageRoleAssistant && msg.ChatAssistantMessage != nil &&
			len(msg.ChatAssistantMessage.ToolCalls) > 0 {
			pendingToolCalls = msg.ChatAssistantMessage.ToolCalls
		}
	}

	if anyCompressed {
		state.Compressed = true
		state.OriginalTokens = originalTotal
		state.CompressedTokens = compressedTotal
	}
	return state
}

// processRtkText is the external text processing pipeline. It strips ANSI,
// detects the command, applies the matched filter, deduplicates, and
// truncates. Used by tests directly.
func processRtkText(input string, config *Config) (string, *ProcessStats) {
	return processRtkTextWithCommand(input, config, "")
}

// processRtkTextWithCommand is the internal pipeline that accepts an optional
// command hint from the tool call lookup. When commandHint is empty, content
// detection is used.
func processRtkTextWithCommand(input string, config *Config, commandHint string) (string, *ProcessStats) {
	stats := &ProcessStats{
		OriginalTokens: estimateTokens(input),
		Techniques:     make([]string, 0),
	}

	if input == "" {
		stats.CompressedTokens = 0
		return input, stats
	}

	// 1. Always strip ANSI escape codes.
	text := stripANSI(input)

	// 2. Early exit for short single-line error messages.
	if isShortErrorMessage(text) {
		stats.CompressedTokens = stats.OriginalTokens
		return input, stats
	}

	// 3. Command detection.
	detection := defaultDetector.detect(text)
	cmd := commandHint
	if cmd == "" {
		cmd = detection.Command
	}

	// 4. Non-shell output is not compressed.
	if detection.Type != "shell" {
		stats.CompressedTokens = stats.OriginalTokens
		return input, stats
	}

	// 5. Match a filter.
	loader := getFilterLoader()
	filter := loader.Match(detection.Type, cmd)
	if filter == nil {
		stats.CompressedTokens = stats.OriginalTokens
		return input, stats
	}

	// 6. Apply line filter rules.
	stripped := applyLineFilter(text, filter)
	if stripped != text {
		stats.Techniques = append(stats.Techniques, "linefilter")
	}

	// 7. Deduplicate consecutive identical lines.
	threshold := config.DedupThreshold
	if threshold <= 1 {
		threshold = 3
	}
	deduped := applyDedup(stripped, threshold)
	if deduped != stripped && deduped != "" {
		stats.Techniques = append(stats.Techniques, "dedup")
	}

	// 8. Smart truncate with intensity-adjusted head/tail.
	effectiveFilter := scaleFilterForIntensity(filter, config.Intensity)
	truncated := applySmartTruncate(deduped, effectiveFilter)
	if truncated != deduped && truncated != "" {
		stats.Techniques = append(stats.Techniques, "smarttruncate")
	}

	// 9. Apply the char hard limit from config.
	result := truncated
	if config.MaxCharsPerResult > 0 && len(result) > config.MaxCharsPerResult {
		result = truncateToCharLimit(result, config.MaxCharsPerResult)
		stats.Techniques = append(stats.Techniques, "charlimit")
	}

	stats.CompressedTokens = estimateTokens(result)
	return result, stats
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
	case "aggressive":
		if c.Head > 0 {
			c.Head = max(1, c.Head/2)
		}
		if c.Tail > 0 {
			c.Tail = max(1, c.Tail/2)
		}
		if c.MaxLines > 0 {
			c.MaxLines = max(1, c.MaxLines/2)
		}
	}
	return &c
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