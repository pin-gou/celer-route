package opencode

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

const requestHeaderRegex = `^msg_[0-9a-f]{12}[0-9A-Za-z]{14}$`

func ptr[T any](v T) *T { return &v }

func toolNamed(name string) schemas.ChatTool {
	return schemas.ChatTool{
		Type: schemas.ChatToolTypeFunction,
		Function: &schemas.ChatToolFunction{
			Name:        name,
			Description: ptr("client-supplied " + name),
			Parameters: &schemas.ToolFunctionParameters{
				Type:       "object",
				Properties: schemas.NewOrderedMapFromPairs(schemas.KV("p", map[string]interface{}{"type": "string"})),
			},
		},
	}
}

func TestNewRequestID_MatchesOpencodeClientShape(t *testing.T) {
	for i := 0; i < 32; i++ {
		id := newRequestID()
		if matched, _ := regexp.MatchString(requestHeaderRegex, id); !matched {
			t.Fatalf("request id %q fails opencode spec %s", id, requestHeaderRegex)
		}
		if strings.HasPrefix(id, "ses_") {
			t.Fatalf("request id %q must not use the ses_ prefix", id)
		}
	}
}

func TestApplyFreeTierHeaders_RequestAndProjectInjected(t *testing.T) {
	p := newBareProvider(t)
	headers := map[string]string{}
	prepared := &schemas.BifrostChatRequest{Model: "big-pickle"}
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	p.applyFreeTierHeaders(ctx, headers, prepared)

	reqID := headers["x-opencode-request"]
	if matched, _ := regexp.MatchString(requestHeaderRegex, reqID); !matched {
		t.Errorf("x-opencode-request %q does not match opencode request format %s", reqID, requestHeaderRegex)
	}
	if got, want := headers["x-opencode-project"], "global"; got != want {
		t.Errorf("x-opencode-project = %q, want %q", got, want)
	}
}

func TestApplyFreeTierBody_NoTools_InjectsToolsAndPinsNone(t *testing.T) {
	prepared := &schemas.BifrostChatRequest{Model: "big-pickle"}

	got := applyFreeTierBody(prepared)

	if got.Params == nil {
		t.Fatal("Params not created")
	}
	if !hasOfficialFreeTierTools(got.Params.Tools) {
		t.Errorf("official free-tier tools missing: got %d tools", len(got.Params.Tools))
	}
	if got.Params.ToolChoice == nil || got.Params.ToolChoice.ChatToolChoiceStr == nil {
		t.Fatalf("tool_choice not pinned for tool-less free-tier request")
	}
	if v := *got.Params.ToolChoice.ChatToolChoiceStr; v != "none" {
		t.Errorf("tool_choice = %q, want none", v)
	}
	// The caller's request must be untouched.
	if prepared.Params != nil {
		t.Errorf("caller request Params was mutated by applyFreeTierBody")
	}
}

func TestApplyFreeTierBody_ClientTools_PreservedAndMerged(t *testing.T) {
	clientTool := toolNamed("custom-fetch")
	clientTools := []schemas.ChatTool{clientTool}
	original := &schemas.BifrostChatRequest{
		Model:  "deepseek-v4.1-flash-free",
		Params: &schemas.ChatParameters{Tools: clientTools},
	}

	got := applyFreeTierBody(original)

	// Client tool kept, official set merged, no duplicate official names.
	names := map[string]int{}
	for _, tool := range got.Params.Tools {
		if tool.Function != nil {
			names[tool.Function.Name]++
		}
	}
	if names["custom-fetch"] != 1 {
		t.Errorf("client tool custom-fetch missing from merged set: %v", names)
	}
	for _, official := range opencodeFreeTierTools {
		if official.Function == nil {
			continue
		}
		if names[official.Function.Name] != 1 {
			t.Errorf("official tool %q duplicated or missing in merged set: %v", official.Function.Name, names)
		}
	}
	// A client that supplied tools keeps control of tool_choice (not pinned).
	if got.Params.ToolChoice != nil {
		t.Errorf("tool_choice should stay nil for tool-providing clients, got %+v", got.Params.ToolChoice)
	}
	// Caller's request untouched.
	if len(original.Params.Tools) != 1 || original.Params.Tools[0].Function.Name != "custom-fetch" {
		t.Errorf("caller request tools were mutated: %+v", original.Params.Tools)
	}
}

func TestApplyFreeTierBody_NonFreeModel_Noop(t *testing.T) {
	prepared := &schemas.BifrostChatRequest{Model: "gpt-5"}
	got := applyFreeTierBody(prepared)
	if got != prepared {
		t.Errorf("applyFreeTierBody should return the request unchanged for non-free models")
	}
	if got.Params != nil {
		t.Errorf("Params should not be created for non-free models")
	}
}

func TestApplyFreeTierBody_AlreadyComplete_Stable(t *testing.T) {
	params := &schemas.ChatParameters{Tools: opencodeFreeTierTools}
	original := &schemas.BifrostChatRequest{Model: "big-pickle", Params: params}

	got := applyFreeTierBody(original)

	if len(got.Params.Tools) != len(opencodeFreeTierTools) {
		t.Errorf("tool set grew unexpectedly: %d != %d", len(got.Params.Tools), len(opencodeFreeTierTools))
	}
	if got.Params.ToolChoice != nil {
		t.Errorf("tool_choice pinned even though tools were already present: %+v", got.Params.ToolChoice)
	}
}

func TestMergeFreeTierTools_AddsOnlyMissing(t *testing.T) {
	client := []schemas.ChatTool{toolNamed("read"), toolNamed("custom")}
	merged := mergeFreeTierTools(client)

	if len(merged) != 2+len(opencodeFreeTierTools)-1 { // "read" already present, one custom extra
		t.Fatalf("unexpected merged length %d", len(merged))
	}
	names := map[string]int{}
	for _, tool := range merged {
		if tool.Function != nil {
			names[tool.Function.Name]++
		}
	}
	if names["read"] != 1 || names["custom"] != 1 {
		t.Errorf("client tool names duplicated: %v", names)
	}
}

func TestAssembleFreeTierChatResponse_HappyPath(t *testing.T) {
	role := string(schemas.ChatMessageRoleAssistant)
	finish := "stop"
	chunks := []*schemas.BifrostStreamChunk{
		{BifrostChatResponse: &schemas.BifrostChatResponse{
			ID: "chatcmpl-1", Model: "big-pickle", Created: 1234,
			Choices: []schemas.BifrostResponseChoice{{
				ChatStreamResponseChoice: &schemas.ChatStreamResponseChoice{
					Delta: &schemas.ChatStreamResponseChoiceDelta{Role: &role, Content: ptr("Hel")},
				},
			}},
		}},
		{BifrostChatResponse: &schemas.BifrostChatResponse{
			Choices: []schemas.BifrostResponseChoice{{
				ChatStreamResponseChoice: &schemas.ChatStreamResponseChoice{
					Delta: &schemas.ChatStreamResponseChoiceDelta{Content: ptr("lo, world")},
				},
			}},
		}},
		{BifrostChatResponse: &schemas.BifrostChatResponse{
			Choices: []schemas.BifrostResponseChoice{{FinishReason: &finish}},
			Usage:   &schemas.BifrostLLMUsage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8},
		}},
	}
	stream := make(chan *schemas.BifrostStreamChunk, len(chunks))
	for _, c := range chunks {
		stream <- c
	}
	close(stream)

	resp, bifrostErr := assembleFreeTierChatResponse("big-pickle", stream)
	if bifrostErr != nil {
		t.Fatalf("unexpected error: %+v", bifrostErr)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.ID != "chatcmpl-1" || resp.Model != "big-pickle" || resp.Created != 1234 {
		t.Errorf("response identity not carried over: %+v", resp)
	}
	msg := resp.Choices[0].ChatNonStreamResponseChoice.Message
	if msg == nil || msg.Content == nil || msg.Content.ContentStr == nil || *msg.Content.ContentStr != "Hello, world" {
		t.Errorf("assembled content = %+v, want Hello, world", msg)
	}
	if resp.Choices[0].FinishReason == nil || *resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish reason not carried over: %+v", resp.Choices[0].FinishReason)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 8 {
		t.Errorf("usage not carried over: %+v", resp.Usage)
	}
	if resp.Object != "chat.completion" {
		t.Errorf("Object = %q, want chat.completion", resp.Object)
	}
}

func TestAssembleFreeTierChatResponse_ErrorChunkWins(t *testing.T) {
	bifrostErr := &schemas.BifrostError{Error: &schemas.ErrorField{Message: "boom"}}
	stream := make(chan *schemas.BifrostStreamChunk, 1)
	stream <- &schemas.BifrostStreamChunk{BifrostError: bifrostErr}
	close(stream)

	resp, gotErr := assembleFreeTierChatResponse("big-pickle", stream)
	if resp != nil {
		t.Errorf("expected nil response on error chunk, got %+v", resp)
	}
	if gotErr == nil || gotErr.Error == nil || gotErr.Error.Message != "boom" {
		t.Errorf("error not propagated: %+v", gotErr)
	}
}
