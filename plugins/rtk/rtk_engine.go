package rtk

import (
	"github.com/pin-gou/pg-gateway/core/schemas"
)

// rtkEngine implements CompressionEngine by wrapping the existing RTK
// compression pipeline (applyRtkCompression). It is registered as "rtk"
// in the EngineCatalog during Init and can be used as a compression step
// in a pipeline.
type rtkEngine struct {
	plugin *Plugin
}

// Compress applies the RTK compression pipeline to the input text.
// It wraps the text in a minimal BifrostRequest, delegates to
// applyRtkCompression, and extracts the compressed result. When the
// plugin is nil or compression is disabled, the input is returned
// unchanged with a passthrough stats.
func (e *rtkEngine) Compress(text string, opts map[string]any) (string, *ProcessStats, error) {
	if e.plugin == nil || e.plugin.config == nil || !e.plugin.config.Enabled {
		stats := &ProcessStats{
			OriginalTokens:   estimateTokens(text),
			CompressedTokens: estimateTokens(text),
		}
		return text, stats, nil
	}

	// Wrap the text in a minimal BifrostRequest for the compression pipeline.
	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: &schemas.ChatMessageContent{
						ContentStr: &text,
					},
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strRef("rtk-engine"),
					},
				},
			},
		},
	}

	state := applyRtkCompression(req, e.plugin)
	if state == nil || !state.Compressed {
		stats := &ProcessStats{
			OriginalTokens:   estimateTokens(text),
			CompressedTokens: estimateTokens(text),
		}
		return text, stats, nil
	}

	// Extract the compressed text from the request.
	result := text
	if len(req.ChatRequest.Input) > 0 {
		msg := req.ChatRequest.Input[0]
		if msg.Content != nil && msg.Content.ContentStr != nil {
			result = *msg.Content.ContentStr
		}
	}

	stats := &ProcessStats{
		OriginalTokens:   state.OriginalTokens,
		CompressedTokens: state.CompressedTokens,
		Techniques:       state.Techniques,
	}
	return result, stats, nil
}

// strRef returns a pointer to the given string.
func strRef(s string) *string {
	return &s
}