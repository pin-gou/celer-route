package rtk

import (
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

// TestAnthropicToolResultBlocks verifies that Anthropic-style tool_result
// content blocks are identified and compressed correctly.
func TestAnthropicToolResultBlocks(t *testing.T) {
	gitOutput := `On branch main
Changes not staged for commit:
  modified:   src/main.go
  modified:   src/utils.go
  modified:   go.mod
  modified:   go.sum
  modified:   Makefile
  modified:   README.md
  modified:   .gitignore
  modified:   docker-compose.yml
  modified:   config.json
  modified:   tests/test_main.go
  modified:   docs/README.md
  modified:   scripts/build.sh
`

	tests := []struct {
		name           string
		messages       []schemas.ChatMessage
		wantCompressed bool
		keyPhrases     []string
	}{
		{
			name: "tool_result_block_with_text",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{
						ContentBlocks: []schemas.ChatContentBlock{
							{
								Type: "tool_result",
								Text: &gitOutput,
							},
						},
					},
				},
			},
			wantCompressed: true,
			keyPhrases:     []string{"On branch main", "modified:   src/main.go"},
		},
		{
			name: "mixed_content_blocks_only_tool_result_compressed",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{
						ContentBlocks: []schemas.ChatContentBlock{
							{
								Type: "text",
								Text: strPtr("Here is the git status:"),
							},
							{
								Type: "tool_result",
								Text: &gitOutput,
							},
							{
								Type: "text",
								Text: strPtr("Please analyze the changes."),
							},
						},
					},
				},
			},
			wantCompressed: true,
			keyPhrases:     []string{"On branch main", "Here is the git status:", "Please analyze the changes."},
		},
		{
			name: "no_tool_result_blocks_no_compression",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{
						ContentBlocks: []schemas.ChatContentBlock{
							{
								Type: "text",
								Text: strPtr("What is the capital of France?"),
							},
						},
					},
				},
			},
			wantCompressed: false,
		},
		{
			name: "tool_result_with_cache_control_preserved",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{
						ContentBlocks: []schemas.ChatContentBlock{
							{
								Type: "text",
								Text: strPtr("System instructions"),
							},
							{
								Type: "tool_result",
								Text: &gitOutput,
								CacheControl: &schemas.CacheControl{
									Type: "ephemeral",
								},
							},
						},
					},
				},
			},
			wantCompressed: false, // cache_control blocks should be preserved as-is
			keyPhrases:     []string{"On branch main", "modified:   src/main.go"},
		},
		{
			name: "tool_result_empty_text",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{
						ContentBlocks: []schemas.ChatContentBlock{
							{
								Type: "tool_result",
								Text: strPtr(""),
							},
						},
					},
				},
			},
			wantCompressed: false,
		},
		{
			name: "tool_result_with_nil_text",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{
						ContentBlocks: []schemas.ChatContentBlock{
							{
								Type: "tool_result",
							},
						},
					},
				},
			},
			wantCompressed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &schemas.BifrostRequest{
				ChatRequest: &schemas.BifrostChatRequest{
					Input: tt.messages,
				},
			}

			state := applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, &Config{
				Enabled:              true,
				Intensity:            "standard",
				PreserveCacheControl: true,
			}))

			if state == nil {
				t.Fatal("applyRtkCompression returned nil state")
			}

			if tt.wantCompressed && !state.Compressed {
				t.Errorf("expected compression to occur, but state.Compressed=false")
			}
			if !tt.wantCompressed && state.Compressed {
				t.Errorf("expected no compression, but state.Compressed=true")
			}

			// Verify key phrases survive
			if len(tt.keyPhrases) > 0 {
				allText := ""
				for _, msg := range req.ChatRequest.Input {
					if msg.Content != nil && msg.Content.ContentBlocks != nil {
						for _, block := range msg.Content.ContentBlocks {
							if block.Text != nil {
								allText += *block.Text + "\n"
							}
						}
					}
					if msg.Content != nil && msg.Content.ContentStr != nil {
						allText += *msg.Content.ContentStr + "\n"
					}
				}
				for _, phrase := range tt.keyPhrases {
					if !contains(allText, phrase) {
						t.Errorf("key phrase %q not found in compressed output", phrase)
					}
				}
			}
		})
	}
}

// TestDetectAnthropicToolResultBlock verifies that the Anthropic adapter
// correctly identifies tool_result blocks.
func TestDetectAnthropicToolResultBlock(t *testing.T) {
	tests := []struct {
		name     string
		block    schemas.ChatContentBlock
		wantTool bool
	}{
		{
			name: "tool_result_type",
			block: schemas.ChatContentBlock{
				Type: "tool_result",
				Text: strPtr("command output"),
			},
			wantTool: true,
		},
		{
			name: "text_type_not_tool",
			block: schemas.ChatContentBlock{
				Type: "text",
				Text: strPtr("some text"),
			},
			wantTool: false,
		},
		{
			name: "image_type_not_tool",
			block: schemas.ChatContentBlock{
				Type: "image_url",
			},
			wantTool: false,
		},
		{
			name: "empty_type",
			block: schemas.ChatContentBlock{
				Text: strPtr("some text"),
			},
			wantTool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isToolResultBlock(&tt.block)
			if got != tt.wantTool {
				t.Errorf("isToolResultBlock(%+v) = %v, want %v", tt.block, got, tt.wantTool)
			}
		})
	}
}

// TestAnthropicToolResultLookup verifies that the assistant message's tool_calls
// are used to build a lookup for tool_result blocks.
func TestAnthropicToolResultLookup(t *testing.T) {
	messages := []schemas.ChatMessage{
		{
			Role: schemas.ChatMessageRoleAssistant,
			ChatAssistantMessage: &schemas.ChatAssistantMessage{
				ToolCalls: []schemas.ChatAssistantMessageToolCall{
					{
						ID: strPtr("toolu_1"),
						Function: schemas.ChatAssistantMessageToolCallFunction{
							Name:      strPtr("bash"),
							Arguments: "git status",
						},
					},
					{
						ID: strPtr("toolu_2"),
						Function: schemas.ChatAssistantMessageToolCallFunction{
							Name:      strPtr("bash"),
							Arguments: "npm install",
						},
					},
				},
			},
		},
		{
			Role: schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{
				ContentBlocks: []schemas.ChatContentBlock{
					{
						Type: "tool_result",
						Text: strPtr("git output here"),
					},
					{
						Type: "tool_result",
						Text: strPtr("npm output here"),
					},
				},
			},
		},
	}

	lookup := buildToolCallLookup(messages)
	if lookup == nil {
		t.Fatal("buildToolCallLookup returned nil")
	}

	// Should find both tool calls
	if len(lookup) != 2 {
		t.Errorf("lookup should have 2 entries, got %d", len(lookup))
	}

	entry1, ok := lookup["toolu_1"]
	if !ok {
		t.Error("toolu_1 should be in lookup")
	} else {
		if entry1.ToolName != "bash" {
			t.Errorf("toolu_1 tool name = %q, want %q", entry1.ToolName, "bash")
		}
		if entry1.Command != "git status" {
			t.Errorf("toolu_1 command = %q, want %q", entry1.Command, "git status")
		}
		_ = entry1
	}

	entry2, ok := lookup["toolu_2"]
	if !ok {
		t.Error("toolu_2 should be in lookup")
	} else {
		if entry2.ToolName != "bash" {
			t.Errorf("toolu_2 tool name = %q, want %q", entry2.ToolName, "bash")
		}
		if entry2.Command != "npm install" {
			t.Errorf("toolu_2 command = %q, want %q", entry2.Command, "npm install")
		}
	}
}

// TestAnthropicToolResultLookupEmpty verifies empty lookup behavior.
func TestAnthropicToolResultLookupEmpty(t *testing.T) {
	messages := []schemas.ChatMessage{
		{
			Role: schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{
				ContentStr: strPtr("hello"),
			},
		},
	}

	lookup := buildToolCallLookup(messages)
	if lookup == nil {
		t.Fatal("buildToolCallLookup returned nil")
	}
	if len(lookup) != 0 {
		t.Errorf("lookup should be empty, got %d entries", len(lookup))
	}
}

// TestAnthropicToolResultLookupNoAssistantMsg verifies nil assistant message safety.
func TestAnthropicToolResultLookupNoAssistantMsg(t *testing.T) {
	messages := []schemas.ChatMessage{
		{
			Role: schemas.ChatMessageRoleTool,
			Content: &schemas.ChatMessageContent{
				ContentStr: strPtr("output"),
			},
		},
	}

	lookup := buildToolCallLookup(messages)
	if lookup == nil {
		t.Fatal("buildToolCallLookup returned nil")
	}
	if len(lookup) != 0 {
		t.Errorf("lookup should be empty, got %d entries", len(lookup))
	}
}

// TestAnthropicToolResultLookupNilIDSafety verifies that tool calls with nil ID
// don't crash the lookup.
func TestAnthropicToolResultLookupNilIDSafety(t *testing.T) {
	messages := []schemas.ChatMessage{
		{
			Role: schemas.ChatMessageRoleAssistant,
			ChatAssistantMessage: &schemas.ChatAssistantMessage{
				ToolCalls: []schemas.ChatAssistantMessageToolCall{
					{
						Function: schemas.ChatAssistantMessageToolCallFunction{
							Name: strPtr("bash"),
						},
					},
				},
			},
		},
	}

	lookup := buildToolCallLookup(messages)
	if lookup == nil {
		t.Fatal("buildToolCallLookup returned nil")
	}
}

// TestAnthropicToolResultBlockNilSafety verifies nil block safety.
func TestAnthropicToolResultBlockNilSafety(t *testing.T) {
	got := isToolResultBlock(nil)
	if got {
		t.Error("isToolResultBlock(nil) should return false")
	}
}

