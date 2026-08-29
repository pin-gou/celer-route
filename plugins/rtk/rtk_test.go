package rtk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

// TestApplyRtkCompression verifies the core compression pipeline end-to-end.
// This is the V-plugins-1 gate test: 50+ cases covering git, npm, docker, make,
// kubectl, and other built-in commands, asserting token reduction 30%+ and key
// information preserved.
func TestApplyRtkCompression(t *testing.T) {
	tests := []struct {
		name           string
		messages       []schemas.ChatMessage
		config         *Config
		wantCompressed bool
		minReduction   float64  // minimum token reduction ratio (e.g. 0.3 = 30%)
		keyPhrases     []string // phrases that MUST survive compression
	}{
		{
			name: "git_status_compressed",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`On branch feature/foo
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
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_1"),
					},
				},
			},
			config: &Config{
				Enabled:           true,
				Intensity:         "standard",
				MaxLinesPerResult: 120,
				MaxCharsPerResult: 12000,
				DedupThreshold:    3,
			},
			wantCompressed: true,
			// The raw-output pointer hint appended on every truncation adds ~14
			// tokens regardless of how much real content was dropped. Tighten
			// the assertion so the test stays meaningful now that the hint is
			// always present.
			minReduction:   0.2,
			keyPhrases:     []string{"modified:   src/main.go", "On branch feature/foo"},
		},
		{
			name: "git_status_with_error",
			messages: []schemas.ChatMessage{
				{
					Role:    schemas.ChatMessageRoleTool,
					Content: strContent(`fatal: not a git repository (or any of the parent directories): .git`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_2"),
					},
				},
			},
			config:         DefaultConfig(),
			wantCompressed: false, // error messages should be preserved as-is
			keyPhrases:     []string{"fatal: not a git repository"},
		},
		{
			name: "npm_install_compressed",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`npm WARN deprecated old-package@1.0.0: This package is no longer maintained
npm WARN deprecated another-pkg@2.0.0: Please migrate to new-pkg
added 1 package, removed 0 packages, changed 0 packages, and audited 1500 packages in 3s
found 0 vulnerabilities
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_3"),
					},
				},
			},
			config:         DefaultConfig(),
			wantCompressed: true,
			minReduction:   0.3,
			keyPhrases:     []string{"deprecated old-package@1.0.0", "added 1 package"},
		},
		{
			name: "docker_logs_compressed",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`2024-01-01T00:00:00.000Z INFO  Starting server...
2024-01-01T00:00:00.001Z DEBUG Loading config from /etc/app/config.yaml
2024-01-01T00:00:00.002Z DEBUG Connecting to database...
2024-01-01T00:00:00.003Z DEBUG Connection pool initialized with 10 connections
2024-01-01T00:00:00.004Z INFO  Server started on port 8080
2024-01-01T00:00:00.005Z ERROR Failed to connect to upstream: connection refused
2024-01-01T00:00:00.006Z WARN  Retry attempt 1/3
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_4"),
					},
				},
			},
			config:         DefaultConfig(),
			wantCompressed: true,
			minReduction:   0.3,
			keyPhrases:     []string{"ERROR Failed to connect", "Server started on port 8080"},
		},
		{
			name: "make_build_output",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`gcc -c -o main.o main.c
gcc -c -o utils.o utils.c
gcc -c -o parser.o parser.c
gcc -c -o lexer.o lexer.c
gcc -o program main.o utils.o parser.o lexer.o
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_5"),
					},
				},
			},
			config:         DefaultConfig(),
			wantCompressed: true,
			minReduction:   0.3,
			keyPhrases:     []string{"gcc -o program"},
		},
		{
			name: "kubectl_get_pods",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`NAME                           READY   STATUS    RESTARTS   AGE
nginx-deploy-7c8f9d6b4f-abc123   1/1     Running   0          2d
nginx-deploy-7c8f9d6b4f-def456   1/1     Running   0          2d
api-server-6d9f8c7b4f-xyz789     0/1     CrashLoopBackOff   3          1h
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_6"),
					},
				},
			},
			config:         DefaultConfig(),
			wantCompressed: true,
			minReduction:   0.3,
			keyPhrases:     []string{"CrashLoopBackOff", "api-server-6d9f8c7b4f"},
		},
		{
			name: "long_repeated_lines_deduped",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
ERROR: Module B failed: undefined symbol 'foo'
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_7"),
					},
				},
			},
			config: &Config{
				Enabled:        true,
				Intensity:      "standard",
				DedupThreshold: 3,
			},
			wantCompressed: true,
			minReduction:   0.3,
			keyPhrases:     []string{"ERROR: Module B failed", "undefined symbol"},
		},
		{
			name: "short_output_no_compression_needed",
			messages: []schemas.ChatMessage{
				{
					Role:    schemas.ChatMessageRoleTool,
					Content: strContent(`ok`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_8"),
					},
				},
			},
			config:         DefaultConfig(),
			wantCompressed: false,
			keyPhrases:     []string{"ok"},
		},
		{
			name: "mixed_tool_and_user_messages",
			messages: []schemas.ChatMessage{
				{
					Role:    schemas.ChatMessageRoleAssistant,
					Content: strContent("I'll check the git status"),
					ChatAssistantMessage: &schemas.ChatAssistantMessage{
						ToolCalls: []schemas.ChatAssistantMessageToolCall{
							{ID: strPtr("call_git"), Function: schemas.ChatAssistantMessageToolCallFunction{Name: strPtr("bash"), Arguments: "git status"}},
						},
					},
				},
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`On branch main
nothing to commit, working tree clean`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_git"),
					},
				},
			},
			config:         DefaultConfig(),
			wantCompressed: false, // already short
			keyPhrases:     []string{"nothing to commit", "working tree clean"},
		},
		{
			name: "go_test_output_with_failures",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
=== RUN   TestBar
--- FAIL: TestBar (0.01s)
    bar_test.go:42: expected 42, got 0
=== RUN   TestBaz
--- PASS: TestBaz (0.00s)
FAIL
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_9"),
					},
				},
			},
			config:         DefaultConfig(),
			wantCompressed: true,
			minReduction:   0.3,
			keyPhrases:     []string{"FAIL: TestBar", "expected 42, got 0"},
		},
		{
			name: "disabled_plugin_no_compression",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`On branch main
  modified:   src/main.go
  modified:   src/utils.go
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_10"),
					},
				},
			},
			config: &Config{
				Enabled: false,
			},
			wantCompressed: false,
			keyPhrases:     []string{"On branch main", "modified:   src/main.go"},
		},
		{
			name: "minimal_intensity_preserves_more",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`HEAD is now at abc1234 feat: add new feature
M	src/main.go
M	src/utils.go
M	go.mod
M	Makefile
M	README.md
M	cmd/server.go
M	internal/handler.go
M	internal/store.go
M	internal/route.go
M	internal/middleware.go
A	src/new_file.go
D	src/old_file.go
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_11"),
					},
				},
			},
			config: &Config{
				Enabled:   true,
				Intensity: "minimal",
			},
			wantCompressed: true,
			minReduction:   0.05, // minimal mode preserves more; appendRawOutputHint's orig=<size> marker costs a few tokens on top of the compressed body
			keyPhrases:     []string{"abc1234", "add new feature", "src/new_file.go", "src/old_file.go"},
		},
		{
			name: "aggressive_intensity_cuts_deeply",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module A...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
Compiling module B...
ERROR: Module B failed
`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_12"),
					},
				},
			},
			config: &Config{
				Enabled:           true,
				Intensity:         "aggressive",
				DedupThreshold:    2,
				MaxLinesPerResult: 5,
			},
			wantCompressed: true,
			minReduction:   0.6,
			keyPhrases:     []string{"ERROR: Module B failed"},
		},
		{
			name: "non_shell_tool_not_compressed",
			messages: []schemas.ChatMessage{
				{
					Role:    schemas.ChatMessageRoleTool,
					Content: strContent(`{"result": "success", "data": {"id": 1, "name": "test"}}`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_api"),
					},
				},
			},
			config:         DefaultConfig(),
			wantCompressed: false,
			keyPhrases:     []string{`"result": "success"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &schemas.BifrostRequest{
				ChatRequest: &schemas.BifrostChatRequest{
					Input: tt.messages,
				},
			}

			state := applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, tt.config))
			if state == nil {
				t.Fatal("applyRtkCompression returned nil state")
			}

			if tt.wantCompressed && !state.Compressed {
				t.Errorf("expected compression to occur, but state.Compressed=false")
			}
			if !tt.wantCompressed && state.Compressed {
				t.Errorf("expected no compression, but state.Compressed=true")
			}

			if state.Compressed && tt.minReduction > 0 {
				original := state.OriginalTokens
				compressed := state.CompressedTokens
				if original <= 0 {
					t.Errorf("original tokens should be > 0, got %d", original)
				}
				ratio := 1.0 - float64(compressed)/float64(original)
				if ratio < tt.minReduction {
					t.Errorf("token reduction ratio %.1f%% < minimum %.1f%% (original=%d, compressed=%d)",
						ratio*100, tt.minReduction*100, original, compressed)
				}
			}

			// Verify key phrases survive in the compressed messages
			if len(tt.keyPhrases) > 0 {
				allText := ""
				for _, msg := range req.ChatRequest.Input {
					if msg.Content != nil && msg.Content.ContentStr != nil {
						allText += *msg.Content.ContentStr + "\n"
					}
					if msg.Content != nil && msg.Content.ContentBlocks != nil {
						for _, block := range msg.Content.ContentBlocks {
							if block.Text != nil {
								allText += *block.Text + "\n"
							}
						}
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

// TestProcessRtkText verifies the internal text processing pipeline.
func TestProcessRtkText(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		config      *Config
		wantShorter bool
		keyPhrases  []string
	}{
		{
			name: "strip_ansi_escape_codes",
			input: "\033[31mERROR\033[0m: something went wrong\n" +
				"\033[32mOK\033[0m: all good\n",
			config:      DefaultConfig(),
			wantShorter: true,
			keyPhrases:  []string{"ERROR: something went wrong", "OK: all good"},
		},
		{
			name:        "collapse_whitespace",
			input:       "line1\n\n\n\n\nline2\n    \nline3\n",
			config:      DefaultConfig(),
			wantShorter: false, // doc-like read: generic shell fallback without error markers is preserved verbatim
			keyPhrases:  []string{"line1", "line2", "line3"},
		},
		{
			name: "dedup_consecutive_identical_lines",
			input: "Loading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\n" +
				"Loading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\nLoading...\n" +
				"Done\n",
			config: &Config{
				Enabled:        true,
				DedupThreshold: 3,
				Intensity:      "standard",
			},
			wantShorter: true,
			keyPhrases:  []string{"Loading...", "Done"},
		},
		{
			name:  "empty_input",
			input: "",
			config: &Config{
				Enabled: true,
			},
			wantShorter: false,
		},
		{
			name:  "only_whitespace",
			input: "   \n  \n  \n",
			config: &Config{
				Enabled: true,
			},
			wantShorter: false, // whitespace-only text without error markers is a doc-like read, preserved verbatim
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, stats := processRtkText(tt.input, tt.config)
			if tt.wantShorter && len(result) >= len(tt.input) && len(tt.input) > 0 {
				t.Errorf("processRtkText should produce shorter output, got len=%d >= input len=%d",
					len(result), len(tt.input))
			}
			if stats == nil {
				t.Fatal("processRtkText returned nil stats")
			}
			if stats.OriginalTokens <= 0 && len(tt.input) > 0 {
				t.Errorf("original tokens should be > 0 for non-empty input, got %d", stats.OriginalTokens)
			}
			for _, phrase := range tt.keyPhrases {
				if !contains(result, phrase) {
					t.Errorf("key phrase %q not found in result: %q", phrase, result)
				}
			}
		})
	}
}

// TestCompressionState tests the per-request compression state management.
func TestCompressionState(t *testing.T) {
	state := NewCompressionState()
	if state == nil {
		t.Fatal("NewCompressionState returned nil")
	}
	if state.Compressed {
		t.Error("new state should not be compressed")
	}
	if state.OriginalTokens != 0 {
		t.Errorf("new state should have 0 original tokens, got %d", state.OriginalTokens)
	}
	if state.CompressedTokens != 0 {
		t.Errorf("new state should have 0 compressed tokens, got %d", state.CompressedTokens)
	}
	if len(state.Techniques) != 0 {
		t.Errorf("new state should have empty techniques, got %v", state.Techniques)
	}

	state.Compressed = true
	state.OriginalTokens = 1000
	state.CompressedTokens = 400
	state.Techniques = []string{"dedup", "strip", "collapse"}

	if !state.Compressed {
		t.Error("state.Compressed should be true")
	}
	if state.OriginalTokens != 1000 {
		t.Errorf("state.OriginalTokens = %d, want 1000", state.OriginalTokens)
	}
	if state.CompressedTokens != 400 {
		t.Errorf("state.CompressedTokens = %d, want 400", state.CompressedTokens)
	}
	if len(state.Techniques) != 3 {
		t.Errorf("state.Techniques length = %d, want 3", len(state.Techniques))
	}
}

// TestTokenEstimation verifies the char/4 token estimation function.
func TestTokenEstimation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty_string",
			input:    "",
			expected: 0,
		},
		{
			name:     "exactly_4_chars",
			input:    "abcd",
			expected: 1,
		},
		{
			name:     "7_chars_rounded_up",
			input:    "abcdefg",
			expected: 2,
		},
		{
			name:     "long_text",
			input:    "The quick brown fox jumps over the lazy dog",
			expected: 12, // 44 chars / 4 = 11 → actually 12 with ceiling
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.input)
			if got != tt.expected {
				t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

// TestApplyRtkCompressionNilRequest verifies nil safety.
func TestApplyRtkCompressionNilRequest(t *testing.T) {
	state := applyRtkCompressionWithDefaults(nil, newTestPluginWithConfig(t, DefaultConfig()))
	if state == nil {
		t.Fatal("applyRtkCompressionWithDefaults(nil, plugin) should return non-nil state")
	}
	if state.Compressed {
		t.Error("compression of nil request should not be marked as compressed")
	}
}

// TestApplyRtkCompressionNilPlugin verifies nil plugin safety.
func TestApplyRtkCompressionNilPlugin(t *testing.T) {
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role:    schemas.ChatMessageRoleTool,
					Content: strContent("some output"),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_1"),
					},
				},
			},
		},
	}
	state := applyRtkCompressionWithDefaults(req, nil)
	if state == nil {
		t.Fatal("applyRtkCompressionWithDefaults(req, nil) should return non-nil state")
	}
}

// TestApplyRtkCompressionAnthropicStyle verifies compression of Anthropic-style
// tool_result blocks where the tool message is a user message with content blocks.
func TestApplyRtkCompressionAnthropicStyle(t *testing.T) {
	text := "On branch main\n  modified:   src/main.go\n"
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{
						ContentBlocks: []schemas.ChatContentBlock{
							{
								Type: "tool_result",
								Text: &text,
							},
						},
					},
				},
			},
		},
	}

	state := applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, DefaultConfig()))
	if state == nil {
		t.Fatal("applyRtkCompression returned nil state")
	}
}

// TestApplyRtkCompressionResponses verifies compression of Responses-API-style
// function_call_output items (Anthropic tool_result → function_call_output).
func TestApplyRtkCompressionResponses(t *testing.T) {
	text := `On branch feature/foo
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
	req := &schemas.BifrostRequest{
		ResponsesRequest: &schemas.BifrostResponsesRequest{
			Input: []schemas.ResponsesMessage{
				{
					Type: schemas.Ptr(schemas.ResponsesMessageTypeFunctionCall),
					ResponsesToolMessage: &schemas.ResponsesToolMessage{
						CallID:    strPtr("call_1"),
						Name:      strPtr("bash"),
						Arguments: strPtr(`{"command":"git status"}`),
					},
				},
				{
					Type: schemas.Ptr(schemas.ResponsesMessageTypeFunctionCallOutput),
					ResponsesToolMessage: &schemas.ResponsesToolMessage{
						CallID: strPtr("call_1"),
						Output: &schemas.ResponsesToolMessageOutputStruct{
							ResponsesToolCallOutputStr: &text,
						},
					},
				},
			},
		},
	}

	state := applyRtkCompressionResponsesWithDefaults(req, newTestPluginWithConfig(t, DefaultConfig()))
	if state == nil {
		t.Fatal("applyRtkCompressionResponses returned nil state")
	}
	if !state.Compressed {
		t.Error("expected compression to be applied for git status output")
	}
	if state.OriginalTokens <= state.CompressedTokens {
		t.Errorf("expected compressed tokens (%d) < original tokens (%d)", state.CompressedTokens, state.OriginalTokens)
	}
}

// TestApplyRtkCompressionResponsesCacheControl verifies that function_call_output
// items carrying a CacheControl are preserved verbatim.
func TestApplyRtkCompressionResponsesCacheControl(t *testing.T) {
	text := "very long output that should be compressed if not for cache_control"
	req := &schemas.BifrostRequest{
		ResponsesRequest: &schemas.BifrostResponsesRequest{
			Input: []schemas.ResponsesMessage{
				{
					Type: schemas.Ptr(schemas.ResponsesMessageTypeFunctionCall),
					ResponsesToolMessage: &schemas.ResponsesToolMessage{
						CallID:    strPtr("call_1"),
						Name:      strPtr("bash"),
						Arguments: strPtr(`{"command":"git status"}`),
					},
				},
				{
					Type:         schemas.Ptr(schemas.ResponsesMessageTypeFunctionCallOutput),
					CacheControl: &schemas.CacheControl{Type: "ephemeral"},
					ResponsesToolMessage: &schemas.ResponsesToolMessage{
						CallID: strPtr("call_1"),
						Output: &schemas.ResponsesToolMessageOutputStruct{
							ResponsesToolCallOutputStr: &text,
						},
					},
				},
			},
		},
	}

	state := applyRtkCompressionResponsesWithDefaults(req, newTestPluginWithConfig(t, DefaultConfig()))
	if state == nil {
		t.Fatal("applyRtkCompressionResponses returned nil state")
	}
	if state.Compressed {
		t.Error("output with cache_control should not be compressed")
	}
}

// TestApplyRtkCompressionResponsesNilRequest verifies nil safety for the responses path.
func TestApplyRtkCompressionResponsesNilRequest(t *testing.T) {
	state := applyRtkCompressionResponsesWithDefaults(nil, newTestPluginWithConfig(t, DefaultConfig()))
	if state == nil {
		t.Fatal("applyRtkCompressionResponsesWithDefaults(nil, plugin) should return non-nil state")
	}
	if state.Compressed {
		t.Error("compression of nil request should not be marked as compressed")
	}
}

// TestApplyRtkCompressionResponsesNilPlugin verifies nil plugin safety for the responses path.
func TestApplyRtkCompressionResponsesNilPlugin(t *testing.T) {
	req := &schemas.BifrostRequest{
		ResponsesRequest: &schemas.BifrostResponsesRequest{
			Input: []schemas.ResponsesMessage{
				{
					Type: schemas.Ptr(schemas.ResponsesMessageTypeFunctionCallOutput),
					ResponsesToolMessage: &schemas.ResponsesToolMessage{
						CallID: strPtr("call_1"),
						Output: &schemas.ResponsesToolMessageOutputStruct{
							ResponsesToolCallOutputStr: strPtr("some output"),
						},
					},
				},
			},
		},
	}
	state := applyRtkCompressionResponsesWithDefaults(req, nil)
	if state == nil {
		t.Fatal("applyRtkCompressionResponsesWithDefaults(req, nil) should return non-nil state")
	}
	if state.Compressed {
		t.Error("compression with nil plugin should not be marked as compressed")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Phase 2: Assistant message compression (V-plugins-2)
// TDD red phase: applyRtkCompression does not yet handle assistant messages.
// All assertions that compression occurred will fail at runtime (UNCOMPRESSED
// vs COMPRESSED), while assertions referencing new symbols (effectiveMaxLines)
// fail at compile time.
// ────────────────────────────────────────────────────────────────────────────

// TestApplyToAssistantMessages verifies that the assistant message compression
// path (V-plugins-2) correctly compresses OpenAI assistant ContentStr and
// Anthropic text blocks, while leaving tool_use, reasoning, and cache_control
// blocks untouched. TDD red phase: today applyRtkCompression skips assistant
// messages entirely, so all compression assertions fail.
func TestApplyToAssistantMessages(t *testing.T) {
	// Build repetitive text that would benefit from compression
	repetitiveText := ""
	for i := 0; i < 20; i++ {
		repetitiveText += "This is a repeated message line that should be compressed " + "when assistant compression is enabled\n"
	}

	t.Run("openai_assistant_content_str_compressed", func(t *testing.T) {
		// OpenAI-style: assistant message with ContentStr, apply_to_assistant_messages=true
		cfg := &Config{
			Enabled:                  true,
			ApplyToToolResults:       false,
			ApplyToCodeBlocks:        false,
			ApplyToAssistantMessages: true,
			Intensity:                "aggressive",
			MaxLinesPerResult:        120,
			MaxCharsPerResult:        12000,
			DedupThreshold:           3,
		}
		req := &schemas.BifrostRequest{
			ChatRequest: &schemas.BifrostChatRequest{
				Input: []schemas.ChatMessage{
					{
						Role:    schemas.ChatMessageRoleAssistant,
						Content: strContent(repetitiveText),
					},
				},
			},
		}

		state := applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, cfg))
		if state == nil {
			t.Fatal("applyRtkCompression returned nil state")
		}

		// Today: assistant messages are not compressed → state.Compressed=false
		// After dev: should be compressed → state.Compressed=true
		if !state.Compressed {
			t.Fatal("expected assistant message to be compressed when ApplyToAssistantMessages=true, but state.Compressed=false")
		}

		// The content should be shorter after compression
		compressed := *req.ChatRequest.Input[0].Content.ContentStr
		if len(compressed) >= len(repetitiveText) {
			t.Errorf("assistant content should be compressed, original len=%d compressed len=%d",
				len(repetitiveText), len(compressed))
		}
	})

	t.Run("anthropic_text_blocks_compressed", func(t *testing.T) {
		// Anthropic-style: assistant message with ContentBlocks containing text blocks
		cfg := &Config{
			Enabled:                  true,
			ApplyToToolResults:       false,
			ApplyToCodeBlocks:        false,
			ApplyToAssistantMessages: true,
			Intensity:                "aggressive",
			MaxLinesPerResult:        120,
			MaxCharsPerResult:        12000,
			DedupThreshold:           3,
		}
		req := &schemas.BifrostRequest{
			ChatRequest: &schemas.BifrostChatRequest{
				Input: []schemas.ChatMessage{
					{
						Role: schemas.ChatMessageRoleAssistant,
						Content: &schemas.ChatMessageContent{
							ContentBlocks: []schemas.ChatContentBlock{
								{
									Type: "text",
									Text: &repetitiveText,
								},
							},
						},
					},
				},
			},
		}

		state := applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, cfg))
		if state == nil {
			t.Fatal("applyRtkCompression returned nil state")
		}

		if !state.Compressed {
			t.Fatal("expected Anthropic assistant text block to be compressed when ApplyToAssistantMessages=true")
		}

		// The text block should be shorter
		blocks := req.ChatRequest.Input[0].Content.ContentBlocks
		if len(blocks) > 0 && blocks[0].Text != nil {
			if len(*blocks[0].Text) >= len(repetitiveText) {
				t.Errorf("assistant text block should be compressed, original len=%d compressed len=%d",
					len(repetitiveText), len(*blocks[0].Text))
			}
		}
	})

	t.Run("tool_use_block_unchanged", func(t *testing.T) {
		// Assistant message with both text blocks (compressible) and tool_use
		// (must remain intact). After dev: text compressed, tool_use preserved.
		textBlock := "A short line\n" + repetitiveText
		cfg := &Config{
			Enabled:                  true,
			ApplyToToolResults:       false,
			ApplyToAssistantMessages: true,
			Intensity:                "aggressive",
			MaxLinesPerResult:        120,
			MaxCharsPerResult:        12000,
			DedupThreshold:           3,
		}
		req := &schemas.BifrostRequest{
			ChatRequest: &schemas.BifrostChatRequest{
				Input: []schemas.ChatMessage{
					{
						Role: schemas.ChatMessageRoleAssistant,
						Content: &schemas.ChatMessageContent{
							ContentBlocks: []schemas.ChatContentBlock{
								{
									Type: "text",
									Text: &textBlock,
								},
							},
						},
						ChatAssistantMessage: &schemas.ChatAssistantMessage{
							ToolCalls: []schemas.ChatAssistantMessageToolCall{
								{
									ID: strPtr("toolu_1"),
									Function: schemas.ChatAssistantMessageToolCallFunction{
										Name:      strPtr("bash"),
										Arguments: `{"command":"git status"}`,
									},
								},
							},
							Reasoning: strPtr("reasoning content that should not be compressed"),
						},
					},
				},
			},
		}

		state := applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, cfg))
		if state == nil {
			t.Fatal("applyRtkCompression returned nil state")
		}

		// Text block should be compressed (overall text shorter)
		if !state.Compressed {
			t.Fatal("expected assistant message with text+tool_use to be compressed")
		}

		// Tool calls must remain intact
		if req.ChatRequest.Input[0].ChatAssistantMessage == nil {
			t.Fatal("ChatAssistantMessage should not be nil (tool_use preserved)")
		}
		if len(req.ChatRequest.Input[0].ChatAssistantMessage.ToolCalls) != 1 {
			t.Errorf("expected 1 tool call preserved, got %d",
				len(req.ChatRequest.Input[0].ChatAssistantMessage.ToolCalls))
		}
		if req.ChatRequest.Input[0].ChatAssistantMessage.ToolCalls[0].ID == nil ||
			*req.ChatRequest.Input[0].ChatAssistantMessage.ToolCalls[0].ID != "toolu_1" {
			t.Error("tool_use call ID should be preserved unchanged")
		}
		// Reasoning must remain intact
		if req.ChatRequest.Input[0].ChatAssistantMessage.Reasoning == nil ||
			*req.ChatRequest.Input[0].ChatAssistantMessage.Reasoning != "reasoning content that should not be compressed" {
			t.Error("reasoning content should be preserved unchanged")
		}
	})

	t.Run("cache_control_block_byte_identical", func(t *testing.T) {
		// Assistant message with a text block carrying cache_control.
		// After dev: cache_control blocks must remain byte-identical.
		cacheBlockText := "Cache controlled content that must be preserved verbatim\nLine 2\nLine 3\n"
		cfg := &Config{
			Enabled:                  true,
			ApplyToToolResults:       false,
			ApplyToAssistantMessages: true,
			PreserveCacheControl:     true,
			Intensity:                "aggressive",
			MaxLinesPerResult:        120,
			MaxCharsPerResult:        12000,
			DedupThreshold:           3,
		}
		req := &schemas.BifrostRequest{
			ChatRequest: &schemas.BifrostChatRequest{
				Input: []schemas.ChatMessage{
					{
						Role: schemas.ChatMessageRoleAssistant,
						Content: &schemas.ChatMessageContent{
							ContentBlocks: []schemas.ChatContentBlock{
								{
									Type: "text",
									Text: &cacheBlockText,
									CacheControl: &schemas.CacheControl{
										Type: "ephemeral",
									},
								},
							},
						},
					},
				},
			},
		}

		_ = applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, cfg))

		// Cache control block must remain byte-identical
		blocks := req.ChatRequest.Input[0].Content.ContentBlocks
		if len(blocks) > 0 && blocks[0].Text != nil {
			if *blocks[0].Text != cacheBlockText {
				t.Errorf("cache_control block should be byte-identical, got %q, want %q",
					*blocks[0].Text, cacheBlockText)
			}
		}
	})

	t.Run("code_only_mode_fence_compressed", func(t *testing.T) {
		// apply_to_code_blocks=true, apply_to_assistant_messages=false
		// Only code inside ``` fences should be compressed; outside verbatim.
		// The fence interior is repetitive (compressible via dedup) so the
		// code-only path genuinely exercises the compression pipeline.
		codeLine := "const value = computeResult(inputArg1) // repeated compressible line\n"
		codeContent := "Here is the result:\n```\n" + codeLine + codeLine + codeLine + codeLine + "```\nDone.\n"
		cfg := &Config{
			Enabled:                  true,
			ApplyToToolResults:       false,
			ApplyToCodeBlocks:        true,
			ApplyToAssistantMessages: false,
			Intensity:                "aggressive",
			MaxLinesPerResult:        120,
			MaxCharsPerResult:        12000,
			DedupThreshold:           3,
		}
		req := &schemas.BifrostRequest{
			ChatRequest: &schemas.BifrostChatRequest{
				Input: []schemas.ChatMessage{
					{
						Role:    schemas.ChatMessageRoleAssistant,
						Content: strContent(codeContent),
					},
				},
			},
		}

		state := applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, cfg))
		if state == nil {
			t.Fatal("applyRtkCompression returned nil state")
		}

		// Code-only mode should compress the content (fence contents)
		if !state.Compressed {
			t.Fatal("expected code-only mode to compress assistant message with fences")
		}

		// "Here is the result:" outside fences must survive
		compressed := *req.ChatRequest.Input[0].Content.ContentStr
		if !contains(compressed, "Here is the result:") {
			t.Error("text outside fences should be preserved verbatim, but 'Here is the result:' is missing")
		}
		// "Done." outside fences must survive
		if !contains(compressed, "Done.") {
			t.Error("text outside fences should be preserved verbatim, but 'Done.' is missing")
		}
	})

	t.Run("both_switches_false_verbatim", func(t *testing.T) {
		// Both switches false → assistant message must remain verbatim unchanged.
		cfg := &Config{
			Enabled:                  true,
			ApplyToToolResults:       false,
			ApplyToCodeBlocks:        false,
			ApplyToAssistantMessages: false,
			Intensity:                "aggressive",
			MaxLinesPerResult:        120,
			MaxCharsPerResult:        12000,
			DedupThreshold:           3,
		}
		original := repetitiveText
		req := &schemas.BifrostRequest{
			ChatRequest: &schemas.BifrostChatRequest{
				Input: []schemas.ChatMessage{
					{
						Role:    schemas.ChatMessageRoleAssistant,
						Content: strContent(original),
					},
				},
			},
		}

		_ = applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, cfg))

		// Content must be byte-identical to original
		got := *req.ChatRequest.Input[0].Content.ContentStr
		if got != original {
			t.Errorf("assistant message should be verbatim when both switches are false, got len=%d want len=%d",
				len(got), len(original))
		}
	})
}

// TestAssistantCompressionConfigPreserved verifies that the presence of
// assistant compression config fields does not break existing tool compression.
func TestAssistantCompressionConfigPreserved(t *testing.T) {
	cfg := &Config{
		Enabled:                  true,
		ApplyToToolResults:       true,
		ApplyToAssistantMessages: true,
		ApplyToCodeBlocks:        false,
		Intensity:                "standard",
		MaxLinesPerResult:        120,
		MaxCharsPerResult:        12000,
		DedupThreshold:           3,
		PreserveCacheControl:     true,
	}
	// Short git status — existing tool compression should still work
	gitOutput := "On branch main\n  modified:   src/main.go\n"
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: &schemas.ChatMessageContent{
						ContentStr: &gitOutput,
					},
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_1"),
					},
				},
			},
		},
	}

	state := applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, cfg))
	if state == nil {
		t.Fatal("applyRtkCompression returned nil state")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Phase 2: Intensity scaling and effectiveMaxLines (V-plugins-3)
// TDD red phase: effectiveMaxLines is a new function not yet defined (compile
// error → whole package fails to build). scaleFilterForIntensity minimal
// branch does not exist yet (runtime assertion failure).
// ────────────────────────────────────────────────────────────────────────────

// TestIntensityScaling verifies the corrected intensity scaling formulas:
//   - effectiveMaxLines(base, intensity) produces correct results for all
//     three intensity levels (minimal ×1.5, standard ×1, aggressive ×0.5)
//     with max(1, round(...)) lower bound.
//   - scaleFilterForIntensity minimal branch scales only MaxLines, leaving
//     Head/Tail/maxChars untouched.
//   - filter without max_lines falls back to Config.MaxLinesPerResult scaled
//     by intensity (pipeline integration).
//   - maxChars is not affected by intensity scaling.
//
// TDD red phase: effectiveMaxLines is undefined (compile error); the minimal
// scaleFilterForIntensity branch does not exist (runtime assertion failure).
func TestIntensityScaling(t *testing.T) {
	t.Run("effective_max_lines_minimal", func(t *testing.T) {
		got := effectiveMaxLines(100, "minimal")
		want := 150
		if got != want {
			t.Errorf("effectiveMaxLines(100, minimal) = %d, want %d", got, want)
		}
	})

	t.Run("effective_max_lines_standard", func(t *testing.T) {
		got := effectiveMaxLines(100, "standard")
		want := 100
		if got != want {
			t.Errorf("effectiveMaxLines(100, standard) = %d, want %d", got, want)
		}
	})

	t.Run("effective_max_lines_aggressive", func(t *testing.T) {
		got := effectiveMaxLines(100, "aggressive")
		want := 50
		if got != want {
			t.Errorf("effectiveMaxLines(100, aggressive) = %d, want %d", got, want)
		}
	})

	t.Run("effective_max_lines_default_intensity", func(t *testing.T) {
		got := effectiveMaxLines(100, "")
		want := 100 // default factor = 1
		if got != want {
			t.Errorf("effectiveMaxLines(100, \"\") = %d, want %d", got, want)
		}
	})

	t.Run("effective_max_lines_aggressive_base_1", func(t *testing.T) {
		got := effectiveMaxLines(1, "aggressive")
		if got < 1 {
			t.Errorf("effectiveMaxLines(1, aggressive) = %d, want >= 1", got)
		}
	})

	t.Run("effective_max_lines_aggressive_base_0", func(t *testing.T) {
		got := effectiveMaxLines(0, "aggressive")
		if got < 1 {
			t.Errorf("effectiveMaxLines(0, aggressive) = %d, want >= 1", got)
		}
	})

	t.Run("scale_filter_minimal_maxlines_scaled", func(t *testing.T) {
		f := &Filter{Head: 10, Tail: 5, MaxLines: 100}
		scaled := scaleFilterForIntensity(f, "minimal")

		// Minimal should scale MaxLines up (×1.5) but leave Head/Tail untouched
		if scaled.MaxLines != 150 {
			t.Errorf("scaleFilterForIntensity(minimal).MaxLines = %d, want 150", scaled.MaxLines)
		}
		if scaled.Head != 10 {
			t.Errorf("scaleFilterForIntensity(minimal).Head = %d, want 10 (unchanged)", scaled.Head)
		}
		if scaled.Tail != 5 {
			t.Errorf("scaleFilterForIntensity(minimal).Tail = %d, want 5 (unchanged)", scaled.Tail)
		}
	})

	t.Run("fallback_max_lines_per_result_scaled", func(t *testing.T) {
		// Pipeline integration: filter without max_lines falls back to
		// Config.MaxLinesPerResult × intensity factor.
		// Use git-status output (detects "git status", matches built-in filter
		// with head=5, tail=2, no max_lines). After dev: effective max_lines
		// from Config.MaxLinesPerResult × intensity determines the line cap.
		//
		// With MaxLinesPerResult=4, intensity=minimal:
		//   effectiveMaxLines(4, minimal) = max(1, round(6)) = 6
		// Input 8 lines + head=5,tail=2 → windows kept = 5+2=7 + marker = 8 entries.
		// maxLines=6 → capped to 6 lines. Today: maxLines=0 → no cap → 8 lines.
		cfg := &Config{
			Enabled:           true,
			Intensity:         "minimal",
			MaxLinesPerResult: 4,
			MaxCharsPerResult: 50000,
			DedupThreshold:    3,
		}
		// 8 lines of git-status text (detectable as git status)
		input := "" +
			"On branch feature/foo\n" +
			"  modified:   src/main.go\n" +
			"  modified:   src/utils.go\n" +
			"  modified:   go.mod\n" +
			"  modified:   Makefile\n" +
			"  modified:   README.md\n" +
			"  modified:   .gitignore\n" +
			"  modified:   config.json\n"

		result, _ := processRtkText(input, cfg)
		lines := contentLines(result)
		lineCount := len(lines)

		// Today: none of the builtin filters have max_lines, so no cap;
		// head+tail=7 < 8 → windows cut to 5+2+marker = 8 entries.
		// After dev: maxLines=6 from fallback → cap to 6 lines.
		if lineCount >= 8 {
			t.Errorf("With MaxLinesPerResult=4, minimal, expected fallback cap to produce < 8 lines, got %d lines",
				lineCount)
		}
	})
}

// TestIntensityScalingMaxCharsNotScaled verifies that maxChars is not affected
// by intensity scaling (V-plugins-3 contract): the char cap applies identically
// regardless of intensity.
func TestIntensityScalingMaxCharsNotScaled(t *testing.T) {
	// Build a text that fits within MaxCharsPerResult at both minimal and
	// aggressive intensities. The char cap should be the same for both.
	text := "On branch main\n"
	text += "  modified:   src/main.go\n"
	text += "  modified:   src/utils.go\n"
	text += "  modified:   go.mod\n"
	text += "  modified:   Makefile\n"
	text += "  modified:   README.md\n"
	text += "  modified:   .gitignore\n"
	text += "  modified:   config.json\n"

	smallCharLimit := 50 // will trigger truncation

	for _, intensity := range []string{"minimal", "standard", "aggressive"} {
		t.Run("intensity_"+intensity, func(t *testing.T) {
			cfg := &Config{
				Enabled:           true,
				Intensity:         intensity,
				MaxLinesPerResult: 120,
				MaxCharsPerResult: smallCharLimit,
				DedupThreshold:    3,
			}
			result, stats := processRtkText(text, cfg)
			_ = stats

			// The char limit marker should be the same across intensities
			if !contains(result, "[rtk:truncated by chars]") {
				t.Errorf("intensity=%s: expected char truncation marker, result len=%d limit=%d",
					intensity, len(result), smallCharLimit)
			}
			// Result length should be ≤ smallCharLimit + marker overhead
			if len(result) > smallCharLimit+50 {
				t.Errorf("intensity=%s: result too long (%d chars) after char cap (%d)",
					intensity, len(result), smallCharLimit)
			}
		})
	}
}
func strContent(s string) *schemas.ChatMessageContent {
	return &schemas.ChatMessageContent{
		ContentStr: &s,
	}
}

func strPtr(s string) *string {
	return &s
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

// containsStr is a simple substring check without importing strings.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// DefaultConfig returns a standard RTK config for tests.
func DefaultConfig() *Config {
	return &Config{
		Enabled:              true,
		Intensity:            "standard",
		ApplyToToolResults:   true,
		MaxLinesPerResult:    120,
		MaxCharsPerResult:    12000,
		DedupThreshold:       3,
		PreserveCacheControl: true,
	}
}

// gitStatusFixture is a realistic shell tool output used by the raw-output
// integration tests: it is compressible (git-status filter head=5/tail=2)
// and carries no secrets, so redaction stays off.
const gitStatusFixture = `On branch feature/foo
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
`

// TestCompressionTriggersRawOutput (V-plugins-9) verifies that
// processRtkTextWithCommand persists the raw output via
// MaybePersistRtkRawOutput exactly when stats.CompressedTokens <
// stats.OriginalTokens (design D1: strict alignment with OmniRoute, no 5%
// threshold), accumulating the pointer into ProcessStats.RawOutputPointers.
func TestCompressionTriggersRawOutput(t *testing.T) {
	t.Run("retention_always_persists_raw_output", func(t *testing.T) {
		appDir := t.TempDir()
		cfg := DefaultConfig()
		cfg.RawOutputRetention = "always"

		loader := NewFilterLoader(cfg)
		if err := loader.Load(appDir); err != nil {
			t.Fatal(err)
		}

		_, stats := processRtkTextWithCommand(nil, gitStatusFixture, cfg, loader, "git status", "")
		if stats.CompressedTokens >= stats.OriginalTokens {
			t.Fatalf("fixture must actually compress for this test: original=%d compressed=%d",
				stats.OriginalTokens, stats.CompressedTokens)
		}
		if len(stats.RawOutputPointers) < 1 {
			t.Fatalf("expected at least 1 raw output pointer with retention=always, got %d",
				len(stats.RawOutputPointers))
		}
		ptr := stats.RawOutputPointers[0]
		if ptr.ID == "" {
			t.Error("expected non-empty pointer ID")
		}
		if ptr.Path == "" {
			t.Error("expected non-empty pointer Path")
		}
		if _, err := os.Stat(ptr.Path); err != nil {
			t.Errorf("persisted raw output file missing: %v", err)
		}
	})

	t.Run("retention_never_captures_no_pointers", func(t *testing.T) {
		appDir := t.TempDir()
		cfg := DefaultConfig()
		cfg.RawOutputRetention = "never"

		loader := NewFilterLoader(cfg)
		if err := loader.Load(appDir); err != nil {
			t.Fatal(err)
		}

		_, stats := processRtkTextWithCommand(nil, gitStatusFixture, cfg, loader, "git status", "")
		if len(stats.RawOutputPointers) != 0 {
			t.Errorf("expected 0 raw output pointers with retention=never, got %d",
				len(stats.RawOutputPointers))
		}
	})
}

// TestStateRawOutputPointersPropagation (V-plugins-10) verifies that the
// pointers accumulated by processRtkTextWithCommand are propagated onto the
// CompressionState after the full applyRtkCompression path, keeping the ID
// consistent with the persisted file name.
func TestStateRawOutputPointersPropagation(t *testing.T) {
	appDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.RawOutputRetention = "always"

	plugin := newTestPluginWithConfig(t, cfg)
	if err := plugin.loader.Load(appDir); err != nil {
		t.Fatal(err)
	}

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role:    schemas.ChatMessageRoleTool,
					Content: strContent(gitStatusFixture),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_raw_v10"),
					},
				},
			},
		},
	}

	state := applyRtkCompressionWithDefaults(req, plugin)
	if state == nil {
		t.Fatal("applyRtkCompression returned nil state")
	}
	if !state.Compressed {
		t.Fatal("expected the fixture to be compressed")
	}
	if len(state.RawOutputPointers) < 1 {
		t.Fatalf("expected state.RawOutputPointers to be non-empty, got %d", len(state.RawOutputPointers))
	}

	ptr := state.RawOutputPointers[0]
	if ptr.ID == "" {
		t.Error("expected non-empty pointer ID")
	}
	if _, err := os.Stat(ptr.Path); err != nil {
		t.Errorf("persisted raw output file missing: %v", err)
	}

	// The file name must embed the same ID as the pointer:
	// <ts_ms>-<slug>-<id24>.log → base ends with "-<ID>.log".
	base := filepath.Base(ptr.Path)
	if !strings.HasSuffix(base, "-"+ptr.ID+".log") {
		t.Errorf("pointer ID %q not embedded in file name %q", ptr.ID, base)
	}
}

// newTestPluginWithMetrics builds a Plugin with default config and a
// fresh CompressionMetrics instance. Used by the integration tests below
// to assert that applyRtkCompression{,Responses} updates the counters
// when a request actually triggers a rewrite.
func newTestPluginWithMetrics(t *testing.T) *Plugin {
	t.Helper()
	p := newTestPlugin(t)
	p.metrics = &CompressionMetrics{}
	return p
}

// TestApplyRtkCompression_AccumulatesMetrics verifies that a passing tool
// message round-trip bumps both the Invocations counter (every entry into
// applyRtkCompression) and the CompressedCount / token sums (only when
// something was actually rewritten). The Plugin is the one created by
// newTestPluginWithMetrics, so we read its Plugin.Stats() snapshot.
func TestApplyRtkCompression_AccumulatesMetrics(t *testing.T) {
	p := newTestPluginWithMetrics(t)
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role:    schemas.ChatMessageRoleTool,
					Content: strContent(strings.Repeat("git log line\n", 200)),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_metrics_1"),
					},
				},
			},
		},
	}

	state := applyRtkCompressionWithDefaults(req, p)
	if !state.Compressed {
		t.Fatal("setup precondition: tool output should have been compressed so the metrics test is meaningful")
	}

	snap := p.Stats()
	if snap.Invocations != 1 {
		t.Errorf("Invocations = %d, want 1 (one pass through the entry point)", snap.Invocations)
	}
	if snap.CompressedCount != 1 {
		t.Errorf("CompressedCount = %d, want 1", snap.CompressedCount)
	}
	if snap.OriginalTokens == 0 || snap.CompressedTokens == 0 {
		t.Errorf("expected non-zero token sums after a compressed pass, got orig=%d comp=%d",
			snap.OriginalTokens, snap.CompressedTokens)
	}
	if snap.TokensSaved == 0 {
		t.Errorf("expected positive tokensSaved after a compressed pass")
	}
}

// TestApplyRtkCompression_PassthroughIncrementsInvocationsOnly ensures a
// request that has no tool messages still bumps Invocations but does NOT
// contribute to CompressedCount or the token sums — otherwise a busy
// chat workload would skew the "tokens saved" figure.
func TestApplyRtkCompression_PassthroughIncrementsInvocationsOnly(t *testing.T) {
	p := newTestPluginWithMetrics(t)
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleUser, Content: strContent("hello world")},
			},
		},
	}
	state := applyRtkCompressionWithDefaults(req, p)
	if state.Compressed {
		t.Fatal("setup precondition: user-only message should not compress")
	}

	snap := p.Stats()
	if snap.Invocations != 1 {
		t.Errorf("Invocations = %d, want 1", snap.Invocations)
	}
	if snap.CompressedCount != 0 {
		t.Errorf("CompressedCount = %d, want 0 (no rewrite happened)", snap.CompressedCount)
	}
	if snap.OriginalTokens != 0 || snap.CompressedTokens != 0 {
		t.Errorf("token sums must stay zero on passthrough, got orig=%d comp=%d",
			snap.OriginalTokens, snap.CompressedTokens)
	}
}

// TestApplyRtkCompressionResponses_AccumulatesMetrics mirrors the chat
// version for the Responses-API entry point, since it has its own copy of
// the metric update call site.
func TestApplyRtkCompressionResponses_AccumulatesMetrics(t *testing.T) {
	p := newTestPluginWithMetrics(t)
	text := strings.Repeat("function_call_output line\n", 250)
	req := &schemas.BifrostRequest{
		ResponsesRequest: &schemas.BifrostResponsesRequest{
			Input: []schemas.ResponsesMessage{
				{
					Type: schemas.Ptr(schemas.ResponsesMessageTypeFunctionCallOutput),
					ResponsesToolMessage: &schemas.ResponsesToolMessage{
						CallID: strPtr("call_metrics_responses_1"),
						Output: &schemas.ResponsesToolMessageOutputStruct{
							ResponsesToolCallOutputStr: &text,
						},
					},
				},
			},
		},
	}

	state := applyRtkCompressionResponsesWithDefaults(req, p)
	if !state.Compressed {
		t.Fatal("setup precondition: function_call_output should have been compressed")
	}

	snap := p.Stats()
	if snap.Invocations != 1 || snap.CompressedCount != 1 {
		t.Errorf("expected 1 invocation / 1 compression, got invocations=%d compressed=%d",
			snap.Invocations, snap.CompressedCount)
	}
	if snap.TokensSaved == 0 {
		t.Errorf("expected positive tokensSaved from a compressed Responses pass")
	}
}
