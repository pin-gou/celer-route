package opencode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// newGoProvider returns a fresh Opencode Go provider for the keyed-tier header
// tests.
func newGoProvider(t *testing.T) *opencodeProvider {
	t.Helper()
	cfg := &schemas.ProviderConfig{}
	cfg.CheckAndSetDefaults()
	p, err := NewOpencodeGoProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewOpencodeGoProvider: %v", err)
	}
	return p
}

func TestApplyConsoleIdentityHeaders_GoProvider_Injects(t *testing.T) {
	p := newGoProvider(t)
	headers := map[string]string{}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	p.applyConsoleIdentityHeaders(ctx, headers)

	sessionID := headers["x-opencode-session"]
	if sessionID == "" {
		t.Fatalf("x-opencode-session was not set: the Go Console rejects every request without it (400 MissingSessionID)")
	}
	if matched, _ := regexp.MatchString(sessionHeaderRegex, sessionID); !matched {
		t.Errorf("x-opencode-session %q does not match %s", sessionID, sessionHeaderRegex)
	}
	for _, h := range []string{"x-opencode-request", "x-opencode-client", "x-opencode-project", "User-Agent"} {
		if headers[h] == "" {
			t.Errorf("%s was not set", h)
		}
	}
	if got, want := headers["x-opencode-client"], "celer-route"; got != want {
		t.Errorf("x-opencode-client = %q, want %q", got, want)
	}
	if got, want := headers["User-Agent"], opencodeConsoleUserAgent; got != want {
		t.Errorf("User-Agent = %q, want %q (Console asks clients not to send a generic HTTP-library UA)", got, want)
	}
}

func TestApplyConsoleIdentityHeaders_ZenProvider_Injects(t *testing.T) {
	p := newPaidProvider(t)
	headers := map[string]string{}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	p.applyConsoleIdentityHeaders(ctx, headers)

	if headers["x-opencode-session"] == "" {
		t.Fatalf("x-opencode-session was not set for the Zen pay-as-you-go tier")
	}
}

// The bare keyless provider owns its outbound identity in applyFreeTierHeaders
// (free-tier gate: Authorization: Bearer public + opencode UA). The console
// injector must stay out of its way so paid-model passthrough is unchanged.
func TestApplyConsoleIdentityHeaders_BareProvider_NoInject(t *testing.T) {
	p := newBareProvider(t)
	headers := map[string]string{}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	p.applyConsoleIdentityHeaders(ctx, headers)

	if len(headers) != 0 {
		t.Errorf("expected no headers for the bare keyless provider, got %v", headers)
	}
}

func TestApplyConsoleIdentityHeaders_CallerOverridesRespected(t *testing.T) {
	ctxSession := func() *schemas.BifrostContext {
		return schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	}

	t.Run("auth header map", func(t *testing.T) {
		p := newGoProvider(t)
		headers := map[string]string{
			"x-opencode-session": "ses_user_supplied_session",
			"x-opencode-client":  "my-custom-client",
			"User-Agent":         "my-custom-ua/1.0",
		}

		p.applyConsoleIdentityHeaders(ctxSession(), headers)

		for header, want := range map[string]string{
			"x-opencode-session": "ses_user_supplied_session",
			"x-opencode-client":  "my-custom-client",
			"User-Agent":         "my-custom-ua/1.0",
		} {
			if got := headers[header]; got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
	})

	t.Run("network config extra headers", func(t *testing.T) {
		p := newGoProvider(t)
		p.networkConfig.ExtraHeaders = map[string]string{"X-Opencode-Session": "ses_network_session"}
		headers := map[string]string{}

		p.applyConsoleIdentityHeaders(ctxSession(), headers)

		if _, ok := headers["x-opencode-session"]; ok {
			t.Errorf("x-opencode-session should not be injected when NetworkConfig.ExtraHeaders sets one, got %q", headers["x-opencode-session"])
		}
	})

	t.Run("context extra headers", func(t *testing.T) {
		p := newGoProvider(t)
		ctx := ctxSession()
		ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, map[string][]string{"x-opencode-session": {"ses_ctx_session"}})
		headers := map[string]string{}

		p.applyConsoleIdentityHeaders(ctx, headers)

		if _, ok := headers["x-opencode-session"]; ok {
			t.Errorf("x-opencode-session should not be injected when the request extra headers set one, got %q", headers["x-opencode-session"])
		}
	})
}

// The Console routes a conversation to one backend and reuses its prompt cache
// off x-opencode-session, so the id must be stable for every turn of a
// conversation. celer-route's per-conversation identity is x-bf-session-id
// (BifrostContextKeySessionID); the derived id must follow it.
func TestApplyConsoleIdentityHeaders_StablePerConversation(t *testing.T) {
	p := newGoProvider(t)

	sessionIDFor := func(gatewaySession string) string {
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		if gatewaySession != "" {
			ctx.SetValue(schemas.BifrostContextKeySessionID, gatewaySession)
		}
		headers := map[string]string{}
		p.applyConsoleIdentityHeaders(ctx, headers)
		return headers["x-opencode-session"]
	}

	first := sessionIDFor("conversation-a")
	second := sessionIDFor("conversation-a")
	if first != second {
		t.Errorf("session id changed between turns of one conversation: %q != %q", first, second)
	}
	if matched, _ := regexp.MatchString(sessionHeaderRegex, first); !matched {
		t.Errorf("derived session id %q does not match %s", first, sessionHeaderRegex)
	}

	if other := sessionIDFor("conversation-b"); other == first {
		t.Errorf("two conversations share the same session id %q", other)
	}

	// Without a gateway session id the request still needs a fresh id.
	if a, b := sessionIDFor(""), sessionIDFor(""); a == b {
		t.Errorf("expected per-request session ids without a gateway session, got duplicate %q", a)
	}
}

// TestChatCompletion_GoProvider_SendsConsoleIdentityHeaders is the wire-level
// regression test: before the fix, opencode-go requests reached
// https://opencode.ai/zen/go/v1/... without x-opencode-session and the Console
// answered 400 MissingSessionID.
func TestChatCompletion_GoProvider_SendsConsoleIdentityHeaders(t *testing.T) {
	t.Parallel()

	var captured http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "glm-5.3-flash",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8}
		}`)
	}))
	defer server.Close()

	cfg := &schemas.ProviderConfig{
		NetworkConfig: schemas.NetworkConfig{
			BaseURL:                        server.URL,
			DefaultRequestTimeoutInSeconds: 5,
		},
	}
	cfg.CheckAndSetDefaults()
	provider, err := NewOpencodeGoProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewOpencodeGoProvider: %v", err)
	}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeySessionID, "conversation-a")
	key := schemas.Key{ID: "go-key", Value: *schemas.NewSecretVar("sk-test")}
	hello := "Hello"
	req := &schemas.BifrostChatRequest{
		Provider: schemas.OpencodeGo,
		Model:    "glm-5.3-flash",
		Input: []schemas.ChatMessage{{
			Role:    schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{ContentStr: &hello},
		}},
	}

	if _, bifrostErr := provider.ChatCompletion(ctx, key, req); bifrostErr != nil {
		t.Fatalf("ChatCompletion returned error: %s", bifrostErr.Error.Message)
	}

	if got := captured.Get("x-opencode-session"); got == "" {
		t.Fatalf("outgoing request is missing x-opencode-session (Console answers 400 MissingSessionID)")
	}
	if got := captured.Get("x-opencode-client"); got != "celer-route" {
		t.Errorf("x-opencode-client = %q, want celer-route", got)
	}
}

// Claude-format models are routed through the Anthropic-compatible /v1/messages
// endpoint, which enforces the same session header as /v1/chat/completions.
func TestChatCompletion_GoProvider_ClaudeFormat_SendsConsoleIdentityHeaders(t *testing.T) {
	t.Parallel()

	var captured http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"qwen3.7-plus","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()

	cfg := &schemas.ProviderConfig{
		NetworkConfig: schemas.NetworkConfig{
			BaseURL:                        server.URL,
			DefaultRequestTimeoutInSeconds: 5,
		},
	}
	cfg.CheckAndSetDefaults()
	provider, err := NewOpencodeGoProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewOpencodeGoProvider: %v", err)
	}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	key := schemas.Key{ID: "go-key", Value: *schemas.NewSecretVar("sk-test")}
	hello := "Hello"
	req := &schemas.BifrostChatRequest{
		Provider: schemas.OpencodeGo,
		Model:    "qwen3.7-plus",
		Input: []schemas.ChatMessage{{
			Role:    schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{ContentStr: &hello},
		}},
	}

	if _, bifrostErr := provider.ChatCompletion(ctx, key, req); bifrostErr != nil {
		t.Fatalf("ChatCompletion returned error: %s", bifrostErr.Error.Message)
	}

	if got := captured.Get("x-opencode-session"); got == "" {
		t.Fatalf("claude-format outgoing request is missing x-opencode-session")
	}
}

// TestChatCompletionStream_GoProvider_SendsConsoleIdentityHeaders pins the
// streaming surface, which builds its own request with the streaming client.
func TestChatCompletionStream_GoProvider_SendsConsoleIdentityHeaders(t *testing.T) {
	t.Parallel()

	headersCh := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case headersCh <- r.Header.Clone():
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	cfg := &schemas.ProviderConfig{
		NetworkConfig: schemas.NetworkConfig{
			BaseURL:                        server.URL,
			DefaultRequestTimeoutInSeconds: 5,
			StreamIdleTimeoutInSeconds:     5,
		},
	}
	cfg.CheckAndSetDefaults()
	provider, err := NewOpencodeGoProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewOpencodeGoProvider: %v", err)
	}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	key := schemas.Key{ID: "go-key", Value: *schemas.NewSecretVar("sk-test")}
	hello := "Hello"
	req := &schemas.BifrostChatRequest{
		Provider: schemas.OpencodeGo,
		Model:    "glm-5.3-flash",
		Input: []schemas.ChatMessage{{
			Role:    schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{ContentStr: &hello},
		}},
	}

	stream, bifrostErr := provider.ChatCompletionStream(ctx, noopPostHook, nil, key, req)
	if bifrostErr != nil {
		t.Fatalf("ChatCompletionStream returned error: %s", bifrostErr.Error.Message)
	}
	for range stream {
	}

	select {
	case headers := <-headersCh:
		if got := headers.Get("x-opencode-session"); got == "" {
			t.Fatalf("streaming request is missing x-opencode-session")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mock server did not receive a streaming request within timeout")
	}
}
