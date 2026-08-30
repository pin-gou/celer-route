package rtk

import "github.com/pin-gou/celer-route/core/schemas"

// ToolCallLookupEntry holds the metadata for a tool call extracted from an
// assistant message's tool_calls array.
type ToolCallLookupEntry struct {
	ToolName string
	Command  string
}

// isToolResultBlock returns true if the content block is an Anthropic-style
// tool_result block.
func isToolResultBlock(block *schemas.ChatContentBlock) bool {
	return block != nil && block.Type == "tool_result"
}

// buildToolCallLookup builds a map from tool_call_id to ToolCallLookupEntry
// by scanning assistant messages in the conversation. This is used for both
// OpenAI-style (role=tool, ToolCallID) and Anthropic-style (tool_result
// blocks correlated positionally to tool_use calls) compression.
func buildToolCallLookup(messages []schemas.ChatMessage) map[string]*ToolCallLookupEntry {
	lookup := make(map[string]*ToolCallLookupEntry)
	for i := range messages {
		msg := &messages[i]
		if msg.Role != schemas.ChatMessageRoleAssistant || msg.ChatAssistantMessage == nil {
			continue
		}
		for _, tc := range msg.ChatAssistantMessage.ToolCalls {
			if tc.ID == nil {
				continue
			}
			name := ""
			if tc.Function.Name != nil {
				name = *tc.Function.Name
			}
			lookup[*tc.ID] = &ToolCallLookupEntry{
				ToolName: name,
				Command:  extractCommandFromArguments(tc.Function.Arguments),
			}
		}
	}
	return lookup
}

// shouldPreserveCacheControl returns true when the block has a cache_control
// marker, indicating it should be preserved verbatim.
func shouldPreserveCacheControl(block *schemas.ChatContentBlock) bool {
	return block != nil && block.CacheControl != nil
}

// isShellTool returns true if the tool name indicates a shell/terminal
// execution whose output should be compressed.
func isShellTool(name string) bool {
	switch name {
	case "bash", "sh", "shell", "zsh", "fish", "ksh", "dash", "pwsh", "powershell",
		"cmd", "command", "terminal", "exec", "run", "run_command",
		"command_executor", "execute_command", "execute":
		return true
	}
	return false
}