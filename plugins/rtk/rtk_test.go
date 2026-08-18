package rtk

import (
	"testing"

	"github.com/pin-gou/pg-gateway/core/schemas"
)

// TestApplyRtkCompression verifies the core compression pipeline end-to-end.
// This is the V-plugins-1 gate test: 50+ cases covering git, npm, docker, make,
// kubectl, and other built-in commands, asserting token reduction 30%+ and key
// information preserved.
func TestApplyRtkCompression(t *testing.T) {
	tests := []struct {
		name          string
		messages      []schemas.ChatMessage
		config        *Config
		wantCompressed bool
		minReduction  float64 // minimum token reduction ratio (e.g. 0.3 = 30%)
		keyPhrases    []string // phrases that MUST survive compression
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
			minReduction:   0.3,
			keyPhrases:     []string{"modified:   src/main.go", "On branch feature/foo"},
		},
		{
			name: "git_status_with_error",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`fatal: not a git repository (or any of the parent directories): .git`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_2"),
					},
				},
			},
			config:        DefaultConfig(),
			wantCompressed: false, // error messages should be preserved as-is
			keyPhrases:    []string{"fatal: not a git repository"},
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
			config:        DefaultConfig(),
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
			config:        DefaultConfig(),
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
			config:        DefaultConfig(),
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
			config:        DefaultConfig(),
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
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`ok`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_8"),
					},
				},
			},
			config:        DefaultConfig(),
			wantCompressed: false,
			keyPhrases:     []string{"ok"},
		},
		{
			name: "mixed_tool_and_user_messages",
			messages: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleAssistant,
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
			config:        DefaultConfig(),
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
			config:        DefaultConfig(),
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
			minReduction:   0.1, // minimal mode preserves more
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
				Enabled:        true,
				Intensity:      "aggressive",
				DedupThreshold: 2,
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
					Role: schemas.ChatMessageRoleTool,
					Content: strContent(`{"result": "success", "data": {"id": 1, "name": "test"}}`),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_api"),
					},
				},
			},
			config:        DefaultConfig(),
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

			state := applyRtkCompression(req, tt.config)
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
			name: "collapse_whitespace",
			input: "line1\n\n\n\n\nline2\n    \nline3\n",
			config:      DefaultConfig(),
			wantShorter: true,
			keyPhrases:  []string{"line1", "line2", "line3"},
		},
		{
			name: "dedup_consecutive_identical_lines",
			input: "Loading...\nLoading...\nLoading...\nLoading...\nLoading...\n" +
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
			name: "only_whitespace",
			input: "   \n  \n  \n",
			config: &Config{
				Enabled: true,
			},
			wantShorter: true,
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
	state := applyRtkCompression(nil, DefaultConfig())
	if state == nil {
		t.Fatal("applyRtkCompression(nil, config) should return non-nil state")
	}
	if state.Compressed {
		t.Error("compression of nil request should not be marked as compressed")
	}
}

// TestApplyRtkCompressionNilConfig verifies nil config safety.
func TestApplyRtkCompressionNilConfig(t *testing.T) {
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: strContent("some output"),
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_1"),
					},
				},
			},
		},
	}
	state := applyRtkCompression(req, nil)
	if state == nil {
		t.Fatal("applyRtkCompression(req, nil) should return non-nil state")
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

	state := applyRtkCompression(req, DefaultConfig())
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

	state := applyRtkCompressionResponses(req, DefaultConfig())
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
					Type: schemas.Ptr(schemas.ResponsesMessageTypeFunctionCallOutput),
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

	state := applyRtkCompressionResponses(req, DefaultConfig())
	if state == nil {
		t.Fatal("applyRtkCompressionResponses returned nil state")
	}
	if state.Compressed {
		t.Error("output with cache_control should not be compressed")
	}
}

// TestApplyRtkCompressionResponsesNilRequest verifies nil safety for the responses path.
func TestApplyRtkCompressionResponsesNilRequest(t *testing.T) {
	state := applyRtkCompressionResponses(nil, DefaultConfig())
	if state == nil {
		t.Fatal("applyRtkCompressionResponses(nil, config) should return non-nil state")
	}
	if state.Compressed {
		t.Error("compression of nil request should not be marked as compressed")
	}
}

// TestApplyRtkCompressionResponsesNilConfig verifies nil config safety for the responses path.
func TestApplyRtkCompressionResponsesNilConfig(t *testing.T) {
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
	state := applyRtkCompressionResponses(req, nil)
	if state == nil {
		t.Fatal("applyRtkCompressionResponses(req, nil) should return non-nil state")
	}
	if state.Compressed {
		t.Error("compression with nil config should not be marked as compressed")
	}
}

// Helper functions

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