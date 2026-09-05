package governance

import (
	"github.com/pin-gou/celer-route/core/schemas"
)

// MultimodalFlags summarizes the multimodal content blocks present in a request
// body. It is derived from the normalized BifrostRequest so routing rules can
// branch on request content via CEL variables like has_image / image_count
// without re-parsing provider-specific raw payloads.
type MultimodalFlags struct {
	HasImage   bool
	HasAudio   bool
	HasFile    bool
	ImageCount int
	AudioCount int
	FileCount  int
}

// emptyMultimodalFlags returns a zero MultimodalFlags for request types that do
// not carry a scanned message body. A zero-value struct is equivalent; this
// helper exists to document the intent at call sites.
func emptyMultimodalFlags() MultimodalFlags {
	return MultimodalFlags{}
}

// extractMultimodalFlags scans the request body for multimodal content blocks.
// It supports the two primary multimodal paths — Chat Completions and the
// Responses API — across both streaming and non-streaming request types. All
// other request types report zero flags.
func extractMultimodalFlags(req *schemas.BifrostRequest) MultimodalFlags {
	if req == nil {
		return emptyMultimodalFlags()
	}

	switch req.RequestType {
	case schemas.ChatCompletionRequest, schemas.ChatCompletionStreamRequest:
		if req.ChatRequest == nil {
			return emptyMultimodalFlags()
		}
		return flagsFromChatMessages(req.ChatRequest.Input)
	case schemas.ResponsesRequest, schemas.ResponsesStreamRequest:
		if req.ResponsesRequest == nil {
			return emptyMultimodalFlags()
		}
		return flagsFromResponsesMessages(req.ResponsesRequest.Input)
	default:
		return emptyMultimodalFlags()
	}
}

// flagsFromChatMessages scans chat message content blocks for image, audio, and
// file parts. Text-only messages (ContentStr or the text block type) contribute
// nothing.
func flagsFromChatMessages(messages []schemas.ChatMessage) MultimodalFlags {
	var flags MultimodalFlags
	for i := range messages {
		content := messages[i].Content
		if content == nil || len(content.ContentBlocks) == 0 {
			continue
		}
		for _, block := range content.ContentBlocks {
			switch block.Type {
			case schemas.ChatContentBlockTypeImage:
				flags.HasImage = true
				flags.ImageCount++
			case schemas.ChatContentBlockTypeInputAudio:
				flags.HasAudio = true
				flags.AudioCount++
			case schemas.ChatContentBlockTypeFile:
				flags.HasFile = true
				flags.FileCount++
			}
		}
	}
	return flags
}

// flagsFromResponsesMessages scans Responses API input items for image, audio,
// and file content blocks. Text-only items (input_text / output_text) contribute
// nothing.
func flagsFromResponsesMessages(messages []schemas.ResponsesMessage) MultimodalFlags {
	var flags MultimodalFlags
	for i := range messages {
		content := messages[i].Content
		if content == nil || len(content.ContentBlocks) == 0 {
			continue
		}
		for _, block := range content.ContentBlocks {
			switch block.Type {
			case schemas.ResponsesInputMessageContentBlockTypeImage:
				flags.HasImage = true
				flags.ImageCount++
			case schemas.ResponsesInputMessageContentBlockTypeAudio:
				flags.HasAudio = true
				flags.AudioCount++
			case schemas.ResponsesInputMessageContentBlockTypeFile:
				flags.HasFile = true
				flags.FileCount++
			}
		}
	}
	return flags
}
