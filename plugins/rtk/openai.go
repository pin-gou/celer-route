package rtk

import "github.com/pin-gou/celer-route/core/schemas"

// getOpenAICommand extracts the command associated with a tool message by
// looking up its ToolCallID in the provided lookup map. Returns the command
// string and true if the tool is a shell tool; returns "", false otherwise.
//
// This is the OpenAI-compatible adapter: tool messages have role="tool" and
// carry a ToolCallID that correlates back to the assistant's tool_call.
func getOpenAICommand(msg *schemas.ChatMessage, lookup map[string]*ToolCallLookupEntry) (string, bool) {
	if msg == nil || msg.ChatToolMessage == nil || msg.ChatToolMessage.ToolCallID == nil {
		return "", false
	}
	entry, ok := lookup[*msg.ChatToolMessage.ToolCallID]
	if !ok {
		return "", false
	}
	if !isShellTool(entry.ToolName) {
		return "", false
	}
	return entry.Command, true
}

// getAnthropicCommand returns the command for a positional tool_result block
// by correlating it with the pending tool calls from the most recent
// assistant message. The blockIndex counts how many tool_result blocks have
// been seen so far in the current user message.
//
// This is the Anthropic-compatible adapter: tool results are content blocks
// of type "tool_result" inside a user message, and they correlate
// positionally to the tool_use blocks in the preceding assistant message.
func getAnthropicCommand(blockIndex int, pendingToolCalls []schemas.ChatAssistantMessageToolCall) (string, bool) {
	if blockIndex >= len(pendingToolCalls) {
		return "", false
	}
	tc := pendingToolCalls[blockIndex]
	name := ""
	if tc.Function.Name != nil {
		name = *tc.Function.Name
	}
	if !isShellTool(name) {
		return "", false
	}
	return extractCommandFromArguments(tc.Function.Arguments), true
}