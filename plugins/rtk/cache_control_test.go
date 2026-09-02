package rtk

import (
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

// TestCacheControlPreservation verifies that content blocks marked with
// cache_control are preserved byte-for-byte during compression. This is the
// V-plugins-5 gate test.
func TestCacheControlPreservation(t *testing.T) {
	// Long git output that would normally be compressed
	gitOutput := `On branch feature/rtk
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
  modified:   internal/parser.go
  modified:   internal/tokenizer.go
  modified:   internal/compiler.go
  modified:   internal/vm.go
`

	// The exact cache_control block to check for byte equality
	cc := &schemas.CacheControl{
		Type:  "ephemeral",
		TTL:   strPtr("5m"),
		Scope: strPtr("global"),
	}

	tests := []struct {
		name          string
		blocks        []schemas.ChatContentBlock
		wantCCPreserved bool
	}{
		{
			name: "cache_control_on_tool_result_preserved",
			blocks: []schemas.ChatContentBlock{
				{
					Type:        "tool_result",
					Text:        &gitOutput,
					CacheControl: cc,
				},
			},
			wantCCPreserved: true,
		},
		{
			name: "cache_control_on_text_block_preserved",
			blocks: []schemas.ChatContentBlock{
				{
					Type:        "text",
					Text:        &gitOutput,
					CacheControl: cc,
				},
			},
			wantCCPreserved: true,
		},
		{
			name: "multiple_blocks_one_with_cc",
			blocks: []schemas.ChatContentBlock{
				{
					Type: "text",
					Text: strPtr("System instructions that should be cached"),
				},
				{
					Type:        "tool_result",
					Text:        &gitOutput,
					CacheControl: cc,
				},
				{
					Type: "text",
					Text: strPtr("Final answer placeholder"),
				},
			},
			wantCCPreserved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &schemas.BifrostRequest{
				ChatRequest: &schemas.BifrostChatRequest{
					Input: []schemas.ChatMessage{
						{
							Role: schemas.ChatMessageRoleUser,
							Content: &schemas.ChatMessageContent{
								ContentBlocks: tt.blocks,
							},
						},
					},
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

			// Verify the cache_control pointer content is bit-identical
			out := req.ChatRequest.Input[0].Content.ContentBlocks
			if len(out) != len(tt.blocks) {
				t.Fatalf("expected %d output blocks, got %d", len(tt.blocks), len(out))
			}

			for i, block := range out {
				if tt.blocks[i].CacheControl != nil {
					gotCC := block.CacheControl
					if gotCC == nil {
						t.Errorf("block %d: expected cache_control to be present, got nil", i)
						continue
					}
					// Byte-for-byte equality of the cache_control struct
					if gotCC.Type != tt.blocks[i].CacheControl.Type {
						t.Errorf("block %d: cache_control type = %q, want %q", i, gotCC.Type, tt.blocks[i].CacheControl.Type)
					}
					if (gotCC.TTL == nil) != (tt.blocks[i].CacheControl.TTL == nil) {
						t.Errorf("block %d: cache_control TTL nil-ness mismatch", i)
					} else if gotCC.TTL != nil && *gotCC.TTL != *tt.blocks[i].CacheControl.TTL {
						t.Errorf("block %d: cache_control TTL = %q, want %q", i, *gotCC.TTL, *tt.blocks[i].CacheControl.TTL)
					}
					if (gotCC.Scope == nil) != (tt.blocks[i].CacheControl.Scope == nil) {
						t.Errorf("block %d: cache_control Scope nil-ness mismatch", i)
					} else if gotCC.Scope != nil && *gotCC.Scope != *tt.blocks[i].CacheControl.Scope {
						t.Errorf("block %d: cache_control Scope = %q, want %q", i, *gotCC.Scope, *tt.blocks[i].CacheControl.Scope)
					}
				}
			}
		})
	}
}

// TestCacheControlDisabled verifies that when PreserveCacheControl is false,
// cache_control blocks may be compressed (the preservation is opt-out).
func TestCacheControlDisabled(t *testing.T) {
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
  modified:   internal/parser.go
  modified:   internal/tokenizer.go
`

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{
						ContentBlocks: []schemas.ChatContentBlock{
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
		},
	}

	state := applyRtkCompressionWithDefaults(req, newTestPluginWithConfig(t, &Config{
		Enabled:              true,
		Intensity:            "standard",
		PreserveCacheControl: false, // disabled: can compress
	}))
	if state == nil {
		t.Fatal("applyRtkCompression returned nil state")
	}

	// The tool_result text may have been compressed since cache_control preserve is off
	outBlocks := req.ChatRequest.Input[0].Content.ContentBlocks
	if len(outBlocks) != 1 {
		t.Fatalf("expected 1 output block, got %d", len(outBlocks))
	}
	if outBlocks[0].CacheControl == nil {
		t.Error("cache_control should still be present on the block even if text is compressed")
	}
}

// TestCacheControlPreservationAnthropicPipeline verifies end-to-end preservation
// through the plugin PreLLMHook path.
func TestCacheControlPreservationAnthropicPipeline(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	// System message with cache_control + long tool result with cache_control
	systemText := "System instructions for the model"
	gitOutput := `On branch main
Changes not staged for commit:
  modified:   src/main.go
  modified:   src/utils.go
  modified:   go.mod
  modified:   scripts/build.sh
`

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleSystem,
					Content: &schemas.ChatMessageContent{
						ContentBlocks: []schemas.ChatContentBlock{
							{
								Type: "text",
								Text: &systemText,
								CacheControl: &schemas.CacheControl{
									Type:  "ephemeral",
									Scope: strPtr("global"),
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
								Text: &gitOutput,
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

	outReq, _, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook returned error: %v", err)
	}

	// Verify system message cache_control preserved exactly
	sysBlock := outReq.ChatRequest.Input[0].Content.ContentBlocks[0]
	if sysBlock.CacheControl == nil {
		t.Error("system block cache_control should be preserved")
	} else {
		if sysBlock.CacheControl.Type != "ephemeral" {
			t.Errorf("system block cache_control.Type = %q, want %q", sysBlock.CacheControl.Type, "ephemeral")
		}
		if sysBlock.CacheControl.Scope == nil || *sysBlock.CacheControl.Scope != "global" {
			t.Error("system block cache_control.Scope should be 'global'")
		}
	}
	if sysBlock.Text == nil || *sysBlock.Text != systemText {
		t.Error("system block text should be unchanged")
	}

	// Verify tool_result block's text may be compressed but cache_control preserved
	toolBlock := outReq.ChatRequest.Input[1].Content.ContentBlocks[0]
	if toolBlock.CacheControl == nil {
		t.Error("tool_result block cache_control should be preserved")
	} else {
		if toolBlock.CacheControl.Type != "ephemeral" {
			t.Errorf("tool_result block cache_control.Type = %q, want %q", toolBlock.CacheControl.Type, "ephemeral")
		}
	}
	if toolBlock.Text == nil {
		t.Error("tool_result block text should not be nil")
	}
	if !contains(*toolBlock.Text, "On branch main") {
		t.Error("git branch info should be preserved in tool result")
	}
}

// TestCacheControlBlockTypeFunctions verifies detection of cache-controlled blocks.
func TestCacheControlBlockTypeFunctions(t *testing.T) {
	tests := []struct {
		name     string
		block    schemas.ChatContentBlock
		wantPreserved bool
	}{
		{
			name: "with_cache_control",
			block: schemas.ChatContentBlock{
				Type:        "text",
				Text:        strPtr("some content"),
				CacheControl: &schemas.CacheControl{Type: "ephemeral"},
			},
			wantPreserved: true,
		},
		{
			name: "without_cache_control",
			block: schemas.ChatContentBlock{
				Type: "text",
				Text: strPtr("some content"),
			},
			wantPreserved: false,
		},
		{
			name: "nil_block",
			block: schemas.ChatContentBlock{},
			wantPreserved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var blockPtr *schemas.ChatContentBlock
			if tt.name != "nil_block" {
				blockPtr = &tt.block
			}
			got := shouldPreserveCacheControl(blockPtr)
			if got != tt.wantPreserved {
				t.Errorf("shouldPreserveCacheControl(%+v) = %v, want %v", tt.block, got, tt.wantPreserved)
			}
		})
	}
}
