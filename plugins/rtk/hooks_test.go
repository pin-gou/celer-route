package rtk

import (
	"context"
	"testing"
	"time"

	"github.com/pin-gou/pg-gateway/core/schemas"
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

	// The tool message should have been compressed (shorter than original)
	compressed := *outReq.ChatRequest.Input[1].Content.ContentStr
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
			ID: "chatcmpl-123",
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