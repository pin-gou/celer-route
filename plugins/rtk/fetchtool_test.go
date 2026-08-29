package rtk

// Tests for the rtk_fetch_raw_output MCP tool (design.md "MCP Tool 形态" +
// "Tool Schema" + PreLLMHook 注入).
//
//   - TestRtkFetchRawOutputTool_Schema            (V-plugins-2) schema 字段
//   - TestRtkFetchRawOutputTool_StableSerialization (V-plugins-2) byte-stable
//   - TestPreLLMHook_InjectsFetchToolSchema       (V-plugins-3) 注入分支
//   - TestPreLLMHook_SkipsInjectWhenRTKDisabled   (V-plugins-3) opt-out: disabled
//   - TestPreLLMHook_SkipsInjectWhenInjectFetchToolFalse (V-plugins-3) opt-out: flag false
//   - TestRawOutputReadHandler_ValidID            (V-plugins-1, V-plugins-7)
//   - TestRawOutputReadHandler_MissingFile        (V-plugins-1)
//   - TestRawOutputReadHandler_InvalidID          (V-plugins-1)
//   - TestRawOutputReadHandler_EmptyArgs          (V-plugins-1)
//   - TestRawOutputReadHandler_BytesEqualHTTPEndpoint (V-plugins-7)

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/pin-gou/celer-route/core/schemas"
)

// ---------------------------------------------------------------------------
// Tool schema (V-plugins-2)
// ---------------------------------------------------------------------------

// TestRtkFetchRawOutputTool_Schema verifies the tool schema contract: the
// function name carries the full bifrostInternal- prefix (must match what
// MCPManager.RegisterTool stores), the required param is "id", and the id
// property declares the 24-hex pattern so the LLM is told what a valid id
// looks like before it calls the tool.
func TestRtkFetchRawOutputTool_Schema(t *testing.T) {
	got := RtkFetchRawOutputTool
	if got.Function == nil {
		t.Fatal("RtkFetchRawOutputTool.Function is nil")
	}
	if got.Function.Name != rtkFetchRawOutputToolName {
		t.Errorf("Function.Name = %q, want %q", got.Function.Name, rtkFetchRawOutputToolName)
	}
	if got.Function.Name != "bifrostInternal-rtk_fetch_raw_output" {
		t.Errorf("Function.Name = %q, want the prefixed internal name", got.Function.Name)
	}
	if got.Type != schemas.ChatToolTypeFunction {
		t.Errorf("Type = %q, want %q", got.Type, schemas.ChatToolTypeFunction)
	}
	if got.Function.Parameters == nil {
		t.Fatal("Function.Parameters is nil")
	}
	if len(got.Function.Parameters.Required) != 1 || got.Function.Parameters.Required[0] != "id" {
		t.Errorf("Required = %v, want [id]", got.Function.Parameters.Required)
	}
	idVal, ok := got.Function.Parameters.Properties.Get("id")
	if !ok {
		t.Fatal("parameters.properties.id missing")
	}
	idMap, ok := idVal.(map[string]any)
	if !ok {
		t.Fatalf("id property type = %T, want map[string]any", idVal)
	}
	if pat, _ := idMap["pattern"].(string); pat != "^[0-9a-f]{24}$" {
		t.Errorf("id.pattern = %q, want ^[0-9a-f]{24}$", pat)
	}
}

// TestRtkFetchRawOutputTool_StableSerialization runs sonic.Marshal twice and
// asserts byte-identical output — the byte-stability property Anthropic /
// OpenAI prompt caches depend on when the schema is appended to the tools list
// (design.md 行为约束 #1).
func TestRtkFetchRawOutputTool_StableSerialization(t *testing.T) {
	first, err := sonic.Marshal(RtkFetchRawOutputTool)
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}
	second, err := sonic.Marshal(RtkFetchRawOutputTool)
	if err != nil {
		t.Fatalf("second marshal: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("RtkFetchRawOutputTool is not byte-stable:\n%s\nvs\n%s", first, second)
	}
}

// ---------------------------------------------------------------------------
// PreLLMHook tool-schema injection (V-plugins-3)
// ---------------------------------------------------------------------------

// mockMCPManagerLike is a scriptable MCPManagerLike for unit tests. It
// reports the tools each client exposes via GetToolPerClient, letting the
// plugin decide whether the fetch tool is registered without a real manager.
type mockMCPManagerLike struct {
	perClient map[string][]schemas.ChatTool
}

func (m *mockMCPManagerLike) GetToolPerClient(context.Context) map[string][]schemas.ChatTool {
	return m.perClient
}

// fetchToolRegisteredManager returns a mock whose client exposes the fetch
// tool under its prefixed name (the shape mcpManagerHasFetchTool looks for).
func fetchToolRegisteredManager() *mockMCPManagerLike {
	return &mockMCPManagerLike{
		perClient: map[string][]schemas.ChatTool{
			"bifrostInternal": {RtkFetchRawOutputTool},
		},
	}
}

// chatRequestWithParams builds a chat-completions request whose Params exist
// (Params.Tools is the slice PreLLMHook appends to).
func chatRequestWithParams() *schemas.BifrostRequest {
	return &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleUser, Content: chatContent("hi")},
			},
			Params: &schemas.ChatParameters{},
		},
	}
}

// newFetchToolPlugin returns a Plugin whose config opts in/out of injection.
// The manager is injected via injectFetchToolSchemaIfManagerAvailable (the
// parameterised core), so no real bifrost instance or MCP server is needed.
func newFetchToolPlugin(t *testing.T, enabled, inject bool) *Plugin {
	t.Helper()
	cfg := &Config{
		Enabled:         enabled,
		Intensity:       "standard",
		InjectFetchTool: &inject,
	}
	return &Plugin{
		name:   PluginName,
		config: cfg,
		logger: nil,
	}
}

// TestPreLLMHook_InjectsFetchToolSchema verifies that when RTK is enabled,
// InjectFetchTool is true, and the fetch tool is registered, the injection
// core appends RtkFetchRawOutputTool to req.ChatRequest.Params.Tools exactly
// once. (PreLLMHook delegates to this via injectFetchToolSchemaIfAvailable.)
func TestPreLLMHook_InjectsFetchToolSchema(t *testing.T) {
	p := newFetchToolPlugin(t, true, true)
	req := chatRequestWithParams()

	p.injectFetchToolSchemaIfManagerAvailable(req, fetchToolRegisteredManager())
	if len(req.ChatRequest.Params.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1 (the fetch tool)", len(req.ChatRequest.Params.Tools))
	}
	if got := req.ChatRequest.Params.Tools[0].Function.Name; got != rtkFetchRawOutputToolName {
		t.Errorf("injected tool name = %q, want %q", got, rtkFetchRawOutputToolName)
	}
}

// TestPreLLMHook_SkipsInjectWhenRTKDisabled verifies the opt-out branch: RTK
// disabled → no injection even when InjectFetchTool is true.
func TestPreLLMHook_SkipsInjectWhenRTKDisabled(t *testing.T) {
	p := newFetchToolPlugin(t, false, true)
	req := chatRequestWithParams()

	p.injectFetchToolSchemaIfManagerAvailable(req, fetchToolRegisteredManager())
	if len(req.ChatRequest.Params.Tools) != 0 {
		t.Errorf("Tools len = %d, want 0 (RTK disabled)", len(req.ChatRequest.Params.Tools))
	}
}

// TestPreLLMHook_SkipsInjectWhenInjectFetchToolFalse verifies the opt-out
// branch: InjectFetchTool=false → no injection even when RTK is enabled.
func TestPreLLMHook_SkipsInjectWhenInjectFetchToolFalse(t *testing.T) {
	p := newFetchToolPlugin(t, true, false)
	req := chatRequestWithParams()

	p.injectFetchToolSchemaIfManagerAvailable(req, fetchToolRegisteredManager())
	if len(req.ChatRequest.Params.Tools) != 0 {
		t.Errorf("Tools len = %d, want 0 (InjectFetchTool=false)", len(req.ChatRequest.Params.Tools))
	}
}

// TestPreLLMHook_DoesNotInjectWhenToolUnregistered verifies the source-of-truth
// gate: even when the plugin opts in, a manager that does NOT expose the fetch
// tool must not trigger injection (the tool is not callable).
func TestPreLLMHook_DoesNotInjectWhenToolUnregistered(t *testing.T) {
	p := newFetchToolPlugin(t, true, true)
	req := chatRequestWithParams()

	p.injectFetchToolSchemaIfManagerAvailable(req, &mockMCPManagerLike{
		perClient: map[string][]schemas.ChatTool{
			"otherClient": {
				{Type: schemas.ChatToolTypeFunction, Function: &schemas.ChatToolFunction{Name: "unrelated_tool"}},
			},
		},
	})
	if len(req.ChatRequest.Params.Tools) != 0 {
		t.Errorf("Tools len = %d, want 0 (tool not registered in manager)", len(req.ChatRequest.Params.Tools))
	}
}

// TestPreLLMHook_InjectsFetchToolSchema_Responses verifies the responses-API
// branch: the schema is converted to the ResponsesTool shape (name in the
// Name field) before being appended to Params.Tools.
func TestPreLLMHook_InjectsFetchToolSchema_Responses(t *testing.T) {
	p := newFetchToolPlugin(t, true, true)
	req := &schemas.BifrostRequest{
		RequestType: schemas.ResponsesRequest,
		ResponsesRequest: &schemas.BifrostResponsesRequest{
			Params: &schemas.ResponsesParameters{},
		},
	}

	p.injectFetchToolSchemaIfManagerAvailable(req, fetchToolRegisteredManager())
	tools := req.ResponsesRequest.Params.Tools
	if len(tools) != 1 {
		t.Fatalf("Tools len = %d, want 1", len(tools))
	}
	if tools[0].Name == nil || *tools[0].Name != rtkFetchRawOutputToolName {
		t.Errorf("Responses tool name = %v, want %q", tools[0].Name, rtkFetchRawOutputToolName)
	}
}

// ---------------------------------------------------------------------------
// RawOutputReadHandler (V-plugins-1, V-plugins-7)
// ---------------------------------------------------------------------------

// writeFixtureRawOutput persists a raw-output fixture under
// <appDir>/rtk/raw-output/ and returns its 24-hex id (the trailing id
// segment of the filename), mirroring MaybePersistRtkRawOutput's layout.
func writeFixtureRawOutput(t *testing.T, appDir, body string) string {
	t.Helper()
	id := strings.Repeat("a", 24) // valid 24-hex id
	dir := filepath.Join(appDir, "rtk", "raw-output")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	name := "1700000000000-fixture-" + id + ".log"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return id
}

// newReadPlugin returns a Plugin rooted at appDir with a bare Config (Enabled
// irrelevant for the handler) so RawOutputReadHandler resolves paths through
// GetAppDir / RawOutputDir.
func newReadPlugin(appDir string) *Plugin {
	return &Plugin{
		name:   PluginName,
		config: &Config{Enabled: true},
		appDir: appDir,
	}
}

// TestRawOutputReadHandler_ValidID verifies the happy path: a persisted file
// resolves, and the returned body is sentinel-wrapped (begins with the magic
// and close tokens) — the same wire shape the HTTP endpoint emits (V-plugins-1,
// V-plugins-7).
func TestRawOutputReadHandler_ValidID(t *testing.T) {
	appDir := t.TempDir()
	body := "original command output\nsecond line\n"
	id := writeFixtureRawOutput(t, appDir, body)
	p := newReadPlugin(appDir)

	out, err := p.RawOutputReadHandler(context.Background(), map[string]any{"id": id})
	if err != nil {
		t.Fatalf("RawOutputReadHandler: %v", err)
	}
	if !strings.HasPrefix(out, rawOutputSentinelMagic) {
		t.Errorf("output must begin with %q, got %q", rawOutputSentinelMagic, out[:min(len(out), len(rawOutputSentinelMagic)+4)])
	}
	if !strings.Contains(out, rawOutputSentinelClose) {
		t.Error("output must contain the sentinel close token")
	}
	if !strings.HasSuffix(out, body) {
		t.Errorf("output must end with the raw body")
	}
}

// TestRawOutputReadHandler_MissingFile verifies the not-found error path: an
// unknown (but valid-format) id yields an error mentioning the TTL/expiry cue.
func TestRawOutputReadHandler_MissingFile(t *testing.T) {
	p := newReadPlugin(t.TempDir())
	unknownID := strings.Repeat("b", 24)

	_, err := p.RawOutputReadHandler(context.Background(), map[string]any{"id": unknownID})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "not found or expired") {
		t.Errorf("error = %q, want mention of 'not found or expired'", err.Error())
	}
}

// TestRawOutputReadHandler_InvalidID verifies the format validation: a
// non-hex / wrong-length id is rejected before any disk lookup.
func TestRawOutputReadHandler_InvalidID(t *testing.T) {
	p := newReadPlugin(t.TempDir())
	for _, bad := range []string{"", "zzzzzzzzzzzzzzzzzzzzzzzz", "abc123", "0123456789abcdef01234567G"} {
		_, err := p.RawOutputReadHandler(context.Background(), map[string]any{"id": bad})
		if err == nil {
			t.Errorf("id %q: expected error, got nil", bad)
			continue
		}
		if !strings.Contains(err.Error(), "invalid id") {
			t.Errorf("id %q: error = %q, want mention of 'invalid id'", bad, err.Error())
		}
	}
}

// TestRawOutputReadHandler_EmptyArgs verifies the args-shape validation: a
// non-map or missing "id" argument is rejected with an error.
func TestRawOutputReadHandler_EmptyArgs(t *testing.T) {
	p := newReadPlugin(t.TempDir())

	if _, err := p.RawOutputReadHandler(context.Background(), map[string]any{}); err == nil {
		t.Error("empty args: expected error, got nil")
	}
	if _, err := p.RawOutputReadHandler(context.Background(), "not-a-map"); err == nil {
		t.Error("non-map args: expected error, got nil")
	}
	if _, err := p.RawOutputReadHandler(context.Background(), nil); err == nil {
		t.Error("nil args: expected error, got nil")
	}
}

// TestRawOutputReadHandler_BytesEqualHTTPEndpoint verifies V-plugins-7: the
// MCP handler's output is byte-identical to the HTTP endpoint's body for the
// same id — both must go through WrapRawOutputForHTTP with the same bytes.
func TestRawOutputReadHandler_BytesEqualHTTPEndpoint(t *testing.T) {
	appDir := t.TempDir()
	body := "unique bytes that survived compression\nline2\n"
	id := writeFixtureRawOutput(t, appDir, body)
	p := newReadPlugin(appDir)

	handlerOut, err := p.RawOutputReadHandler(context.Background(), map[string]any{"id": id})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	// The HTTP endpoint (handlers/rtk.go putRawOutput / getRawOutput) emits
	// WrapRawOutputForHTTP(data, id, len(data), ""). The plugin handler uses
	// the same helper, so for equal inputs the bytes must match.
	httpOut := WrapRawOutputForHTTP(body, id, len(body), "")
	if handlerOut != httpOut {
		t.Errorf("handler output != HTTP endpoint output\nhandler: %q\nhttp:    %q", handlerOut, httpOut)
	}
}
