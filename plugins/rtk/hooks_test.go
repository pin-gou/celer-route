package rtk

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// newTestPlugin creates a Plugin with default config for tests.
func newTestPlugin(t *testing.T) *Plugin {
	t.Helper()
	cfg := DefaultConfig()
	loader := NewFilterLoader(cfg)
	return &Plugin{
		name:   PluginName,
		config: cfg,
		logger: nil,
		loader: loader,
	}
}

// newTestPluginWithConfig creates a Plugin with the given config for tests.
// The loader is initialised with builtin filters only (no Load call).
func newTestPluginWithConfig(t *testing.T, cfg *Config) *Plugin {
	t.Helper()
	loader := NewFilterLoader(cfg)
	return &Plugin{
		name:   PluginName,
		config: cfg,
		logger: nil,
		loader: loader,
	}
}

// newTestCtx creates a BifrostContext for tests.
func newTestCtx(t *testing.T) *schemas.BifrostContext {
	t.Helper()
	return schemas.NewBifrostContext(context.Background(), time.Time{})
}

// TestPreLLMHookModifiesToolMessages verifies that PreLLMHook compresses
// tool messages and populates compression state.
func TestPreLLMHookModifiesToolMessages(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	// Long git status output
	toolContent := `On branch feature/foo
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

	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Model: "gpt-4o",
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleAssistant,
					Content: &schemas.ChatMessageContent{
						ContentStr: strPtr("Let me check git status"),
					},
					ChatAssistantMessage: &schemas.ChatAssistantMessage{
						ToolCalls: []schemas.ChatAssistantMessageToolCall{
							{ID: strPtr("call_git_1"), Function: schemas.ChatAssistantMessageToolCallFunction{
								Name:      strPtr("bash"),
								Arguments: "git status",
							}},
						},
					},
				},
				{
					Role: schemas.ChatMessageRoleTool,
					Content: &schemas.ChatMessageContent{
						ContentStr: &toolContent,
					},
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_git_1"),
					},
				},
			},
		},
	}

	outReq, sc, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook returned error: %v", err)
	}
	if sc != nil {
		t.Fatalf("PreLLMHook returned unexpected short-circuit: %+v", sc)
	}
	if outReq != req {
		t.Error("PreLLMHook should return the same request (mutated in place)")
	}

	// The tool message should have been compressed (shorter than original).
	// Input[0] is the RTK recovery hint prepended by PreLLMHook; the tool
	// message under test now sits at Input[2].
	compressed := *outReq.ChatRequest.Input[2].Content.ContentStr
	if len(compressed) >= len(toolContent) {
		t.Errorf("expected tool content to be compressed, original len=%d compressed len=%d", len(toolContent), len(compressed))
	}

	// Key info should be preserved
	if !contains(compressed, "On branch feature/foo") {
		t.Error("On branch info should be preserved")
	}
}

// TestPreLLMHookNoCompressionWhenDisabled verifies that when the plugin is
// disabled, PreLLMHook does not modify messages.
func TestPreLLMHookNoCompressionWhenDisabled(t *testing.T) {
	p := &Plugin{
		name: PluginName,
		config: &Config{
			Enabled: false,
		},
	}
	ctx := newTestCtx(t)

	content := "On branch main\n  modified:   src/main.go\n  modified:   src/utils.go\n"
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleTool,
					Content: &schemas.ChatMessageContent{
						ContentStr: &content,
					},
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_1"),
					},
				},
			},
		},
	}

	outReq, _, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook with disabled plugin returned error: %v", err)
	}
	if outReq == nil {
		t.Fatal("PreLLMHook returned nil request")
	}
	if *outReq.ChatRequest.Input[0].Content.ContentStr != content {
		t.Error("tool message should not be modified when plugin is disabled")
	}
}

// TestPostLLMHookRewritesUsage verifies that PostLLMHook rewrites
// Usage.PromptTokens to the compressed value and populates the
// Original/Compressed fields.
func TestPostLLMHookRewritesUsage(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	// Simulate a compressed request
	state := &CompressionState{
		OriginalTokens:   1000,
		CompressedTokens: 350,
		Compressed:       true,
	}
	p.setState(ctx, state)

	resp := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			ID:      "chatcmpl-123",
			Choices: []schemas.BifrostResponseChoice{},
			Usage: &schemas.BifrostLLMUsage{
				PromptTokens: 1000,
				TotalTokens:  1100,
			},
		},
	}

	outResp, outErr, err := p.PostLLMHook(ctx, resp, nil)
	if err != nil {
		t.Fatalf("PostLLMHook returned error: %v", err)
	}
	if outErr != nil {
		t.Fatal("PostLLMHook returned non-nil error")
	}
	if outResp == nil {
		t.Fatal("PostLLMHook returned nil response")
	}

	usage := outResp.ChatResponse.Usage
	if usage.PromptTokens != 350 {
		t.Errorf("Usage.PromptTokens = %d, want 350 (compressed)", usage.PromptTokens)
	}
	if usage.OriginalPromptTokens == nil || *usage.OriginalPromptTokens != 1000 {
		t.Error("Usage.OriginalPromptTokens should be 1000")
	}
	if usage.CompressedPromptTokens == nil || *usage.CompressedPromptTokens != 350 {
		t.Error("Usage.CompressedPromptTokens should be 350")
	}
}

// TestPostLLMHookContextPropagation verifies that ctx values are set
// for downstream plugins (e.g. logging) to read.
func TestPostLLMHookContextPropagation(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	state := &CompressionState{
		OriginalTokens:   500,
		CompressedTokens: 200,
		Compressed:       true,
		Techniques:       []string{"dedup", "linefilter"},
		FilterMatched:    "git-status",
	}
	p.setState(ctx, state)

	resp := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			Usage: &schemas.BifrostLLMUsage{
				PromptTokens: 500,
				TotalTokens:  600,
			},
		},
	}

	_, _, err := p.PostLLMHook(ctx, resp, nil)
	if err != nil {
		t.Fatalf("PostLLMHook returned error: %v", err)
	}

	// Verify ctx values are set
	origVal := ctx.Value(schemas.BifrostContextKeyOriginalPromptTokens)
	if origVal == nil {
		t.Error("BifrostContextKeyOriginalPromptTokens not set in ctx")
	} else if origVal.(int) != 500 {
		t.Errorf("OriginalPromptTokens = %d, want 500", origVal.(int))
	}

	compVal := ctx.Value(schemas.BifrostContextKeyCompressedPromptTokens)
	if compVal == nil {
		t.Error("BifrostContextKeyCompressedPromptTokens not set in ctx")
	} else if compVal.(int) != 200 {
		t.Errorf("CompressedPromptTokens = %d, want 200", compVal.(int))
	}

	// New observability keys (added in support of the log detail view).
	techs := ctx.Value(schemas.BifrostContextKeyRTKTechniques)
	if techs == nil {
		t.Error("BifrostContextKeyRTKTechniques not set in ctx")
	} else if got, ok := techs.([]string); !ok || len(got) != 2 || got[0] != "dedup" {
		t.Errorf("RTKTechniques = %v, want [dedup linefilter]", techs)
	}
	filterMatched := ctx.Value(schemas.BifrostContextKeyRTKFilterMatched)
	if filterMatched == nil || filterMatched.(string) != "git-status" {
		t.Errorf("RTKFilterMatched = %v, want git-status", filterMatched)
	}
	ratio := ctx.Value(schemas.BifrostContextKeyRTKCompressionRatio)
	if ratio == nil {
		t.Error("BifrostContextKeyRTKCompressionRatio not set in ctx")
	} else if r, ok := ratio.(float64); !ok || r < 0.59 || r > 0.61 {
		t.Errorf("RTKCompressionRatio = %v, want ~0.6 (1 - 200/500)", ratio)
	}
}

// TestPostLLMHookSnapshotGeneration verifies that the pre-compression
// snapshot is written to ctx when the plugin ran (state.Compressed == true).
// When state.OriginalSnapshot is non-empty, BifrostContextKeyRTKOriginalSnapshot
// should be populated. When snapshot_mode is "off", no snapshot should be
// emitted.
func TestPostLLMHookSnapshotGeneration(t *testing.T) {
	t.Run("split_mode_writes_snapshots", func(t *testing.T) {
		p := newTestPluginWithConfig(t, &Config{Enabled: true, SnapshotMode: "split", SnapshotMaxBytes: 30 * 1024})
		ctx := newTestCtx(t)
		state := &CompressionState{
			OriginalTokens:   100,
			CompressedTokens: 60,
			Compressed:       true,
			OriginalSnapshot: []SnapshotEntry{{Index: 0, Role: "tool", Content: "long original text"}},
		}
		p.setState(ctx, state)
		resp := &schemas.BifrostResponse{ChatResponse: &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{}}}
		_, _, err := p.PostLLMHook(ctx, resp, nil)
		if err != nil {
			t.Fatalf("PostLLMHook returned error: %v", err)
		}
		if v := ctx.Value(schemas.BifrostContextKeyRTKOriginalSnapshot); v == nil {
			t.Error("BifrostContextKeyRTKOriginalSnapshot not set")
		}
		if v := ctx.Value(schemas.BifrostContextKeyRTKSnapshotMode); v != "split" {
			t.Errorf("BifrostContextKeyRTKSnapshotMode = %v, want split", v)
		}
	})

	t.Run("off_mode_skips_snapshots", func(t *testing.T) {
		p := newTestPluginWithConfig(t, &Config{Enabled: true, SnapshotMode: "off"})
		ctx := newTestCtx(t)
		state := &CompressionState{
			OriginalTokens:   100,
			CompressedTokens: 60,
			Compressed:       true,
			OriginalSnapshot: []SnapshotEntry{{Index: 0, Role: "tool", Content: "long original text"}},
		}
		p.setState(ctx, state)
		resp := &schemas.BifrostResponse{ChatResponse: &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{}}}
		_, _, err := p.PostLLMHook(ctx, resp, nil)
		if err != nil {
			t.Fatalf("PostLLMHook returned error: %v", err)
		}
		if v := ctx.Value(schemas.BifrostContextKeyRTKOriginalSnapshot); v != nil {
			t.Errorf("BifrostContextKeyRTKOriginalSnapshot should be nil when snapshot_mode=off, got %T", v)
		}
	})

	t.Run("no_original_snapshot_skips_snapshots", func(t *testing.T) {
		p := newTestPlugin(t) // default config: snapshot_mode=off
		ctx := newTestCtx(t)
		state := &CompressionState{
			OriginalTokens:   100,
			CompressedTokens: 60,
			Compressed:       true,
		}
		p.setState(ctx, state)
		resp := &schemas.BifrostResponse{ChatResponse: &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{}}}
		_, _, err := p.PostLLMHook(ctx, resp, nil)
		if err != nil {
			t.Fatalf("PostLLMHook returned error: %v", err)
		}
		if v := ctx.Value(schemas.BifrostContextKeyRTKOriginalSnapshot); v != nil {
			t.Errorf("expected nil snapshot, got %T", v)
		}
	})

	t.Run("raw_output_pointer_propagates", func(t *testing.T) {
		p := newTestPlugin(t)
		ctx := newTestCtx(t)
		state := &CompressionState{
			OriginalTokens:    100,
			CompressedTokens:  60,
			Compressed:        true,
			RawOutputPointers: []*RtkRawOutputPointer{{ID: "abcdef0123456789abcdef01", Path: "/tmp/rtk/abc.log", Bytes: 1024}},
		}
		p.setState(ctx, state)
		resp := &schemas.BifrostResponse{ChatResponse: &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{}}}
		_, _, err := p.PostLLMHook(ctx, resp, nil)
		if err != nil {
			t.Fatalf("PostLLMHook returned error: %v", err)
		}
		if v := ctx.Value(schemas.BifrostContextKeyRTKRawOutputID); v != "abcdef0123456789abcdef01" {
			t.Errorf("RTKRawOutputID = %v, want abcdef0123456789abcdef01", v)
		}
	})
}

// TestBuildSnapshotModes covers the snapshot wire-shape builder: split,
// merged, and off modes. It also exercises the byte-budget guard which sets
// truncated=true when the payload exceeds SnapshotMaxBytes.
func TestBuildSnapshotModes(t *testing.T) {
	t.Run("split_preserves_order", func(t *testing.T) {
		state := &CompressionState{
			OriginalSnapshot: []SnapshotEntry{
				{Index: 0, Role: "tool", Content: "first"},
				{Index: 1, Role: "tool", Content: "second"},
			},
		}
		orig := buildSnapshot(state, "split", 1<<20)
		if len(orig) == 0 {
			t.Fatalf("expected non-empty original snapshot")
		}
		if !bytesContains(orig, `"items":[`) {
			t.Errorf("split mode should produce an items array; got %s", orig)
		}
		if !bytesContains(orig, `"content":"first"`) || !bytesContains(orig, `"content":"second"`) {
			t.Errorf("expected both items in original snapshot, got %s", orig)
		}
	})
	t.Run("merged_concatenates_with_separator", func(t *testing.T) {
		state := &CompressionState{
			OriginalSnapshot: []SnapshotEntry{
				{Index: 0, Role: "tool", Content: "alpha"},
				{Index: 1, Role: "tool", Content: "beta"},
			},
		}
		orig := buildSnapshot(state, "merged", 1<<20)
		// Merged mode collapses all entries into a single one with index=-1.
		// The actual JSON encoding escapes \n as \\n inside the content field,
		// so check for the escaped sequence rather than literal newlines.
		if !bytesContains(orig, `alpha\n\nbeta`) {
			t.Errorf("merged mode should join contents with \\n\\n, got %s", orig)
		}
		if !bytesContains(orig, `"index":-1`) {
			t.Errorf("merged mode should collapse into one item with index=-1, got %s", orig)
		}
	})
	t.Run("off_returns_nil", func(t *testing.T) {
		state := &CompressionState{
			OriginalSnapshot: []SnapshotEntry{{Index: 0, Role: "tool", Content: "x"}},
		}
		orig := buildSnapshot(state, "off", 1<<20)
		if orig != nil {
			t.Errorf("off mode should produce nil snapshot, got %s", orig)
		}
	})
	t.Run("empty_state_returns_nil", func(t *testing.T) {
		state := &CompressionState{}
		orig := buildSnapshot(state, "split", 1<<20)
		if orig != nil {
			t.Errorf("empty state should produce nil snapshot, got %s", orig)
		}
	})
	t.Run("truncation_marks_when_oversized", func(t *testing.T) {
		// Build a payload larger than 1 KiB; cap at 256 bytes; verify truncated flag
		// is set and items slice was shrunk to fit. With per-item content of 200 chars,
		// each entry marshals to ~250+ bytes, so even one item exceeds the 256-byte cap.
		var big []SnapshotEntry
		for i := 0; i < 20; i++ {
			big = append(big, SnapshotEntry{Index: i, Role: "tool", Content: strings.Repeat("x", 200)})
		}
		state := &CompressionState{OriginalSnapshot: big}
		orig := buildSnapshot(state, "split", 256)
		if !bytesContains(orig, `"truncated":true`) {
			t.Errorf("expected truncated=true when payload exceeds budget, got %s", orig)
		}
	})
}

// TestPostLLMHookNoStatePassthrough verifies that when no compression state
// exists, PostLLMHook passes through the response unchanged.
func TestPostLLMHookNoStatePassthrough(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	// No state set
	resp := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			Usage: &schemas.BifrostLLMUsage{
				PromptTokens: 100,
				TotalTokens:  150,
			},
		},
	}

	outResp, _, err := p.PostLLMHook(ctx, resp, nil)
	if err != nil {
		t.Fatalf("PostLLMHook returned error: %v", err)
	}
	if outResp == nil {
		t.Fatal("PostLLMHook returned nil response")
	}
	if outResp.ChatResponse.Usage.PromptTokens != 100 {
		t.Errorf("Usage.PromptTokens should be unchanged (100), got %d", outResp.ChatResponse.Usage.PromptTokens)
	}
	if outResp.ChatResponse.Usage.OriginalPromptTokens != nil {
		t.Error("OriginalPromptTokens should not be set when no compression occurred")
	}
}

// TestPostLLMHookNilResponse verifies nil response safety.
func TestPostLLMHookNilResponse(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	state := &CompressionState{
		Compressed: true,
	}
	p.setState(ctx, state)

	outResp, outErr, err := p.PostLLMHook(ctx, nil, nil)
	if err != nil {
		t.Fatalf("PostLLMHook with nil response returned error: %v", err)
	}
	if outResp != nil {
		t.Error("PostLLMHook with nil response should return nil response")
	}
	if outErr != nil {
		t.Error("PostLLMHook with nil response should return nil error")
	}
}

// TestPreLLMHookNonChatRequest verifies that PreLLMHook handles non-chat
// requests gracefully.
func TestPreLLMHookNonChatRequest(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	req := &schemas.BifrostRequest{
		RequestType: schemas.EmbeddingRequest,
		EmbeddingRequest: &schemas.BifrostEmbeddingRequest{
			Input: &schemas.EmbeddingInput{Texts: []string{"test"}},
		},
	}

	outReq, _, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook with non-chat request returned error: %v", err)
	}
	if outReq == nil {
		t.Fatal("PreLLMHook returned nil request")
	}
}

// TestPreLLMHookNilRequest verifies nil request safety.
func TestPreLLMHookNilRequest(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	outReq, _, err := p.PreLLMHook(ctx, nil)
	if err != nil {
		t.Fatalf("PreLLMHook with nil request returned error: %v", err)
	}
	if outReq != nil {
		t.Error("PreLLMHook with nil request should return nil request")
	}
}

// TestPreLLMHookNilChatRequest verifies nil ChatRequest safety.
func TestPreLLMHookNilChatRequest(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
	}

	outReq, _, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook with nil ChatRequest returned error: %v", err)
	}
	if outReq == nil {
		t.Fatal("PreLLMHook returned nil request")
	}
}

// TestPreLLMHookNoToolMessages verifies requests without tool messages
// are not compressed.
func TestPreLLMHookNoToolMessages(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{
					Role:    schemas.ChatMessageRoleUser,
					Content: &schemas.ChatMessageContent{ContentStr: strPtr("Hello")},
				},
			},
		},
	}

	outReq, _, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook returned error: %v", err)
	}
	if outReq != req {
		t.Error("PreLLMHook should return the same request for non-tool messages")
	}

	state := p.getState(ctx)
	if state != nil && state.Compressed {
		t.Error("request without tool messages should not be compressed")
	}
}

// TestPostLLMHookErrorPassthrough verifies that err is passed through unchanged.
func TestPostLLMHookErrorPassthrough(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	state := &CompressionState{
		Compressed: true,
	}
	p.setState(ctx, state)

	bifrostErr := &schemas.BifrostError{
		Error: &schemas.ErrorField{
			Message: "test error",
		},
	}

	outResp, outErr, err := p.PostLLMHook(ctx, nil, bifrostErr)
	if err != nil {
		t.Fatalf("PostLLMHook returned error: %v", err)
	}
	if outResp != nil {
		t.Error("PostLLMHook should return nil response for error")
	}
	if outErr != bifrostErr {
		t.Error("PostLLMHook should pass through the original error")
	}
}

// ============================================================================
// Task 6.2: MinTokensToCompress threshold boundary tests (red phase)
//
// TDD red phase: Config.MinTokensToCompress does not exist yet. All tests
// referencing it will fail at compile time with "undefined" errors.
//
// After dev, the hook must:
//   - MinTokensToCompress=0 (default) → compress all messages (no skip)
//   - MinTokensToCompress=1000000 & req tokens=10 → skip compression entirely
//     (output bytes identical to input)
// ============================================================================

// TestPreLLMHookMinTokensZeroCompressesAll verifies that when
// MinTokensToCompress=0 (the default), the hook compresses all tool messages
// regardless of size. This preserves the current behaviour — zero means
// "no minimum threshold, always compress." After dev, the new field must not
// block compression when set to its zero value.
func TestPreLLMHookMinTokensZeroCompressesAll(t *testing.T) {
	// Config with MinTokensToCompress=0 (zero value, default)
	cfg := &Config{
		Enabled:             true,
		Intensity:           "standard",
		MaxLinesPerResult:   120,
		MaxCharsPerResult:   12000,
		DedupThreshold:      3,
		MinTokensToCompress: 0,
	}
	p := newTestPluginWithConfig(t, cfg)
	ctx := newTestCtx(t)

	// Tool output (should still be compressed when MinTokensToCompress=0)
	toolContent := `On branch feature/foo
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

	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Model: "gpt-4o",
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleAssistant,
					Content: &schemas.ChatMessageContent{
						ContentStr: strPtr("Let me check git status"),
					},
					ChatAssistantMessage: &schemas.ChatAssistantMessage{
						ToolCalls: []schemas.ChatAssistantMessageToolCall{
							{ID: strPtr("call_1"), Function: schemas.ChatAssistantMessageToolCallFunction{
								Name:      strPtr("bash"),
								Arguments: "git status",
							}},
						},
					},
				},
				{
					Role: schemas.ChatMessageRoleTool,
					Content: &schemas.ChatMessageContent{
						ContentStr: &toolContent,
					},
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_1"),
					},
				},
			},
		},
	}

	outReq, _, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook returned error: %v", err)
	}

	// With MinTokensToCompress=0, compression should occur.
	// Input[0] is now the RTK recovery hint prepended by PreLLMHook;
	// the tool message under test moved from [1] to [2].
	compressed := *outReq.ChatRequest.Input[2].Content.ContentStr
	if len(compressed) >= len(toolContent) {
		t.Errorf("expected compression with MinTokensToCompress=0, original len=%d compressed len=%d",
			len(toolContent), len(compressed))
	}

	// Key info should still be preserved
	if !contains(compressed, "On branch feature/foo") {
		t.Error("On branch info should be preserved after compression")
	}
}

// TestPreLLMHookMinTokensHighSkipsCompression verifies that when
// MinTokensToCompress is set to a very high value (e.g. 1000000) and the
// request's estimated tokens are well below that threshold, the hook skips
// compression entirely. The output bytes must be identical to the input.
// After dev, this is the threshold guard: small outputs are not worth
// compressing when the minimum is set high.
func TestPreLLMHookMinTokensHighSkipsCompression(t *testing.T) {
	// Config with MinTokensToCompress set very high
	cfg := &Config{
		Enabled:             true,
		Intensity:           "standard",
		MaxLinesPerResult:   120,
		MaxCharsPerResult:   12000,
		DedupThreshold:      3,
		MinTokensToCompress: 1000000,
	}
	p := newTestPluginWithConfig(t, cfg)
	ctx := newTestCtx(t)

	// Small tool output (~10 tokens estimated) — well below the threshold
	originalContent := `On branch main
  modified:   src/main.go
`

	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Model: "gpt-4o",
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleAssistant,
					Content: &schemas.ChatMessageContent{
						ContentStr: strPtr("Check git status"),
					},
					ChatAssistantMessage: &schemas.ChatAssistantMessage{
						ToolCalls: []schemas.ChatAssistantMessageToolCall{
							{ID: strPtr("call_skip"), Function: schemas.ChatAssistantMessageToolCallFunction{
								Name:      strPtr("bash"),
								Arguments: "git status",
							}},
						},
					},
				},
				{
					Role: schemas.ChatMessageRoleTool,
					Content: &schemas.ChatMessageContent{
						ContentStr: &originalContent,
					},
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_skip"),
					},
				},
			},
		},
	}

	// Save the original content bytes to compare after the hook.
	// Before the hook runs the layout is [assistant, tool]; PreLLMHook
	// prepends the RTK recovery hint at [0] shifting the tool message to [2].
	originalBytes := *req.ChatRequest.Input[1].Content.ContentStr

	outReq, _, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook returned error: %v", err)
	}

	// With MinTokensToCompress=1000000 and small input, compression should be skipped
	// Output bytes must be identical to input.
	afterHook := *outReq.ChatRequest.Input[2].Content.ContentStr
	if afterHook != originalBytes {
		t.Errorf("output should be byte-identical to input when MinTokensToCompress threshold is not met, "+
			"got %q, want %q", afterHook, originalBytes)
	}
}

// TestPreLLMHookMinTokensPartialCompression verifies that when the request
// token count exceeds MinTokensToCompress, compression proceeds normally
// (the threshold is not a hard cap, just a minimum gate). This test uses
// a large enough output to exceed the threshold, ensuring the compression
// pipeline still runs.
func TestPreLLMHookMinTokensPartialCompression(t *testing.T) {
	cfg := &Config{
		Enabled:             true,
		Intensity:           "standard",
		MaxLinesPerResult:   120,
		MaxCharsPerResult:   12000,
		DedupThreshold:      3,
		MinTokensToCompress: 5, // Very low threshold — most outputs exceed this
	}
	p := newTestPluginWithConfig(t, cfg)
	ctx := newTestCtx(t)

	// Moderate git output (>5 estimated tokens)
	toolContent := `On branch feature/foo
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
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Model: "gpt-4o",
			Input: []schemas.ChatMessage{
				{
					Role: schemas.ChatMessageRoleAssistant,
					Content: &schemas.ChatMessageContent{
						ContentStr: strPtr("Check git status"),
					},
					ChatAssistantMessage: &schemas.ChatAssistantMessage{
						ToolCalls: []schemas.ChatAssistantMessageToolCall{
							{ID: strPtr("call_partial"), Function: schemas.ChatAssistantMessageToolCallFunction{
								Name:      strPtr("bash"),
								Arguments: "git status",
							}},
						},
					},
				},
				{
					Role: schemas.ChatMessageRoleTool,
					Content: &schemas.ChatMessageContent{
						ContentStr: &toolContent,
					},
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: strPtr("call_partial"),
					},
				},
			},
		},
	}

	outReq, _, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook returned error: %v", err)
	}

	// With MinTokensToCompress=5 and output much larger, compression should occur.
	// Input[0] is now the RTK recovery hint (prepended by PreLLMHook when
	// RTK is enabled); the tool message under test moved from [1] to [2].
	compressed := *outReq.ChatRequest.Input[2].Content.ContentStr
	if len(compressed) >= len(toolContent) {
		t.Errorf("expected compression when input exceeds MinTokensToCompress threshold, "+
			"original len=%d compressed len=%d", len(toolContent), len(compressed))
	}

	// Key info should be preserved
	if !contains(compressed, "On branch feature/foo") {
		t.Error("branch info should be preserved after compression")
	}
	// Error patterns must survive
	if !contains(compressed, "modified:   src/main.go") {
		t.Error("key file changes should be preserved")
	}
}

// bytesContains reports whether the substring appears anywhere in raw. Used
// by the snapshot wire-shape tests; avoids pulling in a strings.Contains
// import shadow.
func bytesContains(raw []byte, substr string) bool {
	return bytes.Index(raw, []byte(substr)) >= 0
}

// TestPreLLMHookStripsSentinelFromToolMessages verifies the PreLLMHook entry
// strip removes the raw-output sentinel from every tool message field before
// the request leaves the plugin boundary. The wire-protocol prefix must NEVER
// reach the model — that was the leakage the regression test
// tests/manual/raw_output_recursion_regression.sh used to assert (in the
// earlier, recursive behaviour).
func TestPreLLMHookStripsSentinelFromToolMessages(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	const plainBody = "ok github.com/foo/bar 0.123s\n"
	wrappedBody := rawOutputSentinelMagic + "abc123456789abcdef01234567:42:" + rawOutputSentinelClose + plainBody

	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-5",
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: ptrString("ls -la")}},

				{
					Role: schemas.ChatMessageRoleAssistant,
					ChatAssistantMessage: &schemas.ChatAssistantMessage{
						ToolCalls: []schemas.ChatAssistantMessageToolCall{
							{ID: ptrString("call_1"), Type: ptrString("function"), Function: schemas.ChatAssistantMessageToolCallFunction{Name: ptrString("exec"), Arguments: `{"cmd":"ls"}`}},
						},
					},
				},
				{
					Role: schemas.ChatMessageRoleTool,
					ChatToolMessage: &schemas.ChatToolMessage{
						ToolCallID: ptrString("call_1"),
					},
					Content: &schemas.ChatMessageContent{ContentStr: ptrString(wrappedBody)},
				},
			},
		},
	}

	out, _, err := p.PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook returned error: %v", err)
	}
	if out == nil || out.ChatRequest == nil {
		t.Fatal("PreLLMHook must return the request unchanged structurally")
	}

	// PreLLMHook prepends a system-message recovery hint to Input, so the
	// tool message is no longer at index 2. Find it by role so the test
	// stays robust against hint-injection changes.
	var toolMsg *schemas.ChatMessage
	for i := range out.ChatRequest.Input {
		if out.ChatRequest.Input[i].Role == schemas.ChatMessageRoleTool {
			toolMsg = &out.ChatRequest.Input[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("tool message must still be present in input after PreLLMHook")
	}
	if toolMsg.Content == nil || toolMsg.Content.ContentStr == nil {
		t.Fatal("tool message ContentStr must remain populated after strip")
	}
	if got := *toolMsg.Content.ContentStr; got != plainBody {
		t.Errorf("sentinel must be stripped from tool message ContentStr; got prefix=%q", got[:min(40, len(got))])
	}
	if ctx.Value(schemas.BifrostContextKeyRTKSentinelStripped) == nil {
		t.Error("ctx must carry a positive RTKSentinelStripped count after a strip occurred")
	}

	// PostLLMHook must clear the marker so the next request on the same ctx
	// does not silently inherit a stale count.
	_, _, err = p.PostLLMHook(ctx, nil, nil)
	if err != nil {
		t.Fatalf("PostLLMHook returned error: %v", err)
	}
	if v := ctx.Value(schemas.BifrostContextKeyRTKSentinelStripped); v != nil {
		t.Errorf("PostLLMHook must clear RTKSentinelStripped; got %#v", v)
	}
}

// TestPreLLMHookStripsSentinelWhenDisabled verifies the strip runs even when
// RTK compression is disabled. The wire protocol prefix is a leak that has
// nothing to do with compression — the strip must happen unconditionally so
// the model never sees it regardless of config.
func TestPreLLMHookStripsSentinelWhenDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	p := newTestPluginWithConfig(t, cfg)
	ctx := newTestCtx(t)

	const plainBody = "15 degrees"
	wrappedBody := rawOutputSentinelMagic + "0123456789abcdef01234567:9:" + rawOutputSentinelClose + plainBody

	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-5",
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleTool, Content: &schemas.ChatMessageContent{ContentStr: ptrString(wrappedBody)}},
			},
		},
	}

	if _, _, err := p.PreLLMHook(ctx, req); err != nil {
		t.Fatalf("PreLLMHook returned error: %v", err)
	}

	// PreLLMHook prepends a system-message recovery hint to Input only when
	// compression is enabled; here it is disabled, so the tool message is
	// still at index 0.
	got := *req.ChatRequest.Input[0].Content.ContentStr
	if got != plainBody {
		t.Errorf("sentinel must be stripped even when RTK is disabled; got prefix=%q", got[:min(40, len(got))])
	}
	if v := ctx.Value(schemas.BifrostContextKeyRTKSentinelStripped); v == nil {
		t.Error("stripped-count marker must be set even when compression is disabled")
	}
}

// TestPreLLMHookNoSentinelLeavesRequestUntouched verifies the strip is a
// no-op when no sentinel is present (the common case). The strip count in
// ctx must be nil so downstream pipeline calls do not falsely take the
// pre-stripped bypass.
func TestPreLLMHookNoSentinelLeavesRequestUntouched(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newTestCtx(t)

	const plain = "ok github.com/foo/bar 0.123s\n"
	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-5",
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleTool, Content: &schemas.ChatMessageContent{ContentStr: ptrString(plain)}},
			},
		},
	}

	if _, _, err := p.PreLLMHook(ctx, req); err != nil {
		t.Fatalf("PreLLMHook returned error: %v", err)
	}

	// PreLLMHook prepends a system-message recovery hint to Input, so the
	// tool message is no longer at index 0 — find it by role instead.
	var toolMsg *schemas.ChatMessage
	for i := range req.ChatRequest.Input {
		if req.ChatRequest.Input[i].Role == schemas.ChatMessageRoleTool {
			toolMsg = &req.ChatRequest.Input[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("tool message must still be present in input after PreLLMHook")
	}
	if toolMsg.Content == nil || toolMsg.Content.ContentStr == nil {
		t.Fatal("tool message ContentStr must remain populated on the no-sentinel path")
	}
	if got := *toolMsg.Content.ContentStr; got != plain {
		t.Errorf("plain tool message must round-trip unchanged; got %q", got)
	}
	if v := ctx.Value(schemas.BifrostContextKeyRTKSentinelStripped); v != nil {
		t.Errorf("no-sentinel path must NOT set the stripped-count marker (would falsely short-circuit downstream pipeline); got %#v", v)
	}
}

// TestProcessRtkTextWithCommand_PreStrippedBypass verifies the in-pipeline
// bypass contract: when ctx carries a positive RTKSentinelStripped count,
// processRtkTextWithCommand returns the input unchanged with the bypass
// technique recorded and does NOT call StripRawOutputSentinel on the body
// (which would otherwise re-run the prefix match for every tool message
// field on every compress call).
func TestProcessRtkTextWithCommand_PreStrippedBypass(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCharsPerResult = 1 // would truncate any non-trivial input without bypass
	ctx := newTestCtx(t)
	ctx.SetValue(schemas.BifrostContextKeyRTKSentinelStripped, 1)

	const body = "ok github.com/foo/bar 0.123s\n"
	out, stats := processRtkTextWithCommand(ctx, body, cfg, NewFilterLoader(cfg), "", "")

	if out != body {
		t.Errorf("pre-stripped bypass must return body unchanged; got %q want %q", out, body)
	}
	if !hasTechnique(stats.Techniques, "rtk-raw-output-bypass") {
		t.Errorf("pre-stripped bypass must record the rtk-raw-output-bypass technique; got %v", stats.Techniques)
	}
}

// TestProcessRtkTextWithCommand_NilCtxStillStrips verifies the legacy admin
// test path: when ctx is nil (no hook chain), the in-pipeline
// StripRawOutputSentinel check must still fire so the bypass contract is
// testable from outside the hook chain (admin handler drives the bypass
// deliberately by feeding wrapped input through processRtkTextWithCommand).
func TestProcessRtkTextWithCommand_NilCtxStillStrips(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCharsPerResult = 1

	const body = "ok github.com/foo/bar 0.123s\n"
	wrapped := WrapRawOutputForHTTP(body, "abc123456789abcdef01234567", len(body), "")

	out, stats := processRtkTextWithCommand(nil, wrapped, cfg, NewFilterLoader(cfg), "", "")
	if out != body {
		t.Errorf("nil-ctx bypass must still strip via the in-pipeline check; got %q want %q", out, body)
	}
	if !hasTechnique(stats.Techniques, "rtk-raw-output-bypass") {
		t.Errorf("nil-ctx bypass must record the rtk-raw-output-bypass technique; got %v", stats.Techniques)
	}
}
