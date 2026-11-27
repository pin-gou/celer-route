package opencode

import (
	"strings"
	"time"

	schemas "github.com/pin-gou/celer-route/core/schemas"
)

// opencodeFreeTierUserAgent spoofs the current OpenCode desktop/CLI client
// identity the zen free-tier gate keys on. The gate rejects non-opencode
// User-Agents with 403 FreeTierError and parses the client version out of the
// UA, answering HTTP 426 UpgradeRequired when it is older than 1.18.0 — so
// the embedded version must stay on the current release channel, not just
// "look like opencode".
const opencodeFreeTierUserAgent = "opencode/1.18.34 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14"

// opencodeFreeTierTools is the set of OpenCode built-in tool definitions that
// must be present in the request body for the zen free tier to accept a call.
// Since September 2026 the gate validates the request body as well as the
// identity headers: a request without the OpenCode tool set (or without
// "stream": true) is rejected with 403 FreeTierError no matter how faithful
// the headers are.
//
// These eleven built-ins mirror the tools the real client sends. The set was
// verified accepted against https://opencode.ai/zen/v1/chat/completions; the
// gate re-checks tool names/shapes on upstream releases, so this list may need
// refreshing when OpenCode rotates it.
var opencodeFreeTierTools = func() []schemas.ChatTool {
	strPtr := func(s string) *string { return &s }
	boolPtr := func(b bool) *bool { return &b }
	additionalPropsFalse := &schemas.AdditionalPropertiesStruct{AdditionalPropertiesBool: boolPtr(false)}

	// params builds the JSON-schema parameters object for one tool. Props are
	// declared in the order the upstream OpenCode client emits them
	// (OrderedMap preserves insertion order for stable wire bytes).
	params := func(props []schemas.Pair, required []string) *schemas.ToolFunctionParameters {
		om := schemas.NewOrderedMapWithCapacity(len(props))
		for _, p := range props {
			om.Set(p.Key, p.Value)
		}
		return &schemas.ToolFunctionParameters{
			Type:                 "object",
			Properties:           om,
			Required:             required,
			AdditionalProperties: additionalPropsFalse,
		}
	}
	tool := func(name, desc string, props []schemas.Pair, required []string) schemas.ChatTool {
		return schemas.ChatTool{
			Type: schemas.ChatToolTypeFunction,
			Function: &schemas.ChatToolFunction{
				Name:        name,
				Description: strPtr(desc),
				Parameters:  params(props, required),
			},
		}
	}
	str := func(name string) schemas.Pair {
		return schemas.KV(name, map[string]interface{}{"type": "string"})
	}
	num := func(name string) schemas.Pair {
		return schemas.KV(name, map[string]interface{}{"type": "number"})
	}
	boolean := func(name string) schemas.Pair {
		return schemas.KV(name, map[string]interface{}{"type": "boolean"})
	}

	return []schemas.ChatTool{
		tool("bash", "Run a shell command. You can run any command that would work in a bash terminal, including commands that need to be run with sudo.",
			[]schemas.Pair{str("command"), num("timeout"), str("description")},
			[]string{"command", "timeout", "description"}),
		tool("edit", "Edit a file. Replaces the oldString with the newString in the file.",
			[]schemas.Pair{str("filePath"), str("oldString"), str("newString"), boolean("replaceAll")},
			[]string{"filePath", "oldString", "newString", "replaceAll"}),
		tool("glob", "Find files by name or glob pattern in the working tree.",
			[]schemas.Pair{str("pattern"), str("path")},
			[]string{"pattern", "path"}),
		tool("grep", "Search file contents with a regular expression.",
			[]schemas.Pair{str("pattern"), str("path"), str("include")},
			[]string{"pattern", "path", "include"}),
		tool("read", "Reads a file or directory. If a path to a file, returns the contents of the file.",
			[]schemas.Pair{str("filePath"), num("offset"), num("limit")},
			[]string{"filePath", "offset", "limit"}),
		tool("write", "Write a file. Use to create and write files.",
			[]schemas.Pair{str("filePath"), str("content")},
			[]string{"filePath", "content"}),
		tool("todowrite", "Write the todo list to the todo file.",
			[]schemas.Pair{schemas.KV("todos", map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "object"},
			})},
			[]string{"todos"}),
		tool("todoread", "Read the todo list from the todo file.",
			[]schemas.Pair{str("x")},
			nil),
		tool("webfetch", "Fetch a URL from the internet. This is useful for getting up-to-date information from web pages.",
			[]schemas.Pair{str("url"), str("format"), num("timeout")},
			[]string{"url", "format", "timeout"}),
		tool("task", "Launch a subagent.",
			[]schemas.Pair{str("description"), str("prompt"), str("subagent_type")},
			[]string{"description", "prompt", "subagent_type"}),
		tool("skill", "Load a skill.",
			[]schemas.Pair{str("name")},
			[]string{"name"}),
	}
}()

// hasOfficialFreeTierTools reports whether every official free-tier tool name
// is already declared in tools (matched by function name). Used to keep the
// injected set additive and idempotent.
func hasOfficialFreeTierTools(tools []schemas.ChatTool) bool {
	present := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		if t.Type == schemas.ChatToolTypeFunction && t.Function != nil {
			present[t.Function.Name] = struct{}{}
		}
	}
	for _, official := range opencodeFreeTierTools {
		if official.Function == nil {
			continue
		}
		if _, ok := present[official.Function.Name]; !ok {
			return false
		}
	}
	return true
}

// mergeFreeTierTools appends the official OpenCode built-in tool definitions
// for any name the caller did not already declare. Returns the input slice
// unchanged when nothing needs adding; the returned slice never aliases the
// input when it grows.
func mergeFreeTierTools(tools []schemas.ChatTool) []schemas.ChatTool {
	if hasOfficialFreeTierTools(tools) {
		return tools
	}
	present := make(map[string]struct{}, len(tools)+len(opencodeFreeTierTools))
	for _, t := range tools {
		if t.Type == schemas.ChatToolTypeFunction && t.Function != nil {
			present[t.Function.Name] = struct{}{}
		}
	}
	merged := make([]schemas.ChatTool, 0, len(tools)+len(opencodeFreeTierTools))
	merged = append(merged, tools...)
	for _, official := range opencodeFreeTierTools {
		if official.Function == nil {
			continue
		}
		if _, ok := present[official.Function.Name]; !ok {
			merged = append(merged, official)
		}
	}
	return merged
}

// applyFreeTierBody rewrites the request body for a free-tier model so it
// satisfies the zen free-tier gate: the OpenCode built-in tool definitions
// must be present and, for callers that did not ask for tool calling, the
// tool_choice is pinned to "none" so the injected tools cannot hijack the
// model into emitting tool calls the gateway has no executor for.
//
// The prepared request is already a shallow copy from prepareOpencodeRequest;
// Params is deep-copied before any mutation so the caller's request is never
// touched. Returns the request unchanged when the model is not a free-tier
// model.
func applyFreeTierBody(prepared *schemas.BifrostChatRequest) *schemas.BifrostChatRequest {
	if prepared == nil || !isFreeOpencodeModel(prepared.Model) {
		return prepared
	}

	// Work on a copy of the top-level struct so the caller's request is never
	// mutated; Params is deep-copied below before its fields are touched.
	preparedCopy := *prepared
	prepared = &preparedCopy

	hadTools := false
	var params *schemas.ChatParameters
	if prepared.Params != nil {
		pc := *prepared.Params
		params = &pc
		hadTools = len(pc.Tools) > 0
	} else {
		params = &schemas.ChatParameters{}
	}

	if !hasOfficialFreeTierTools(params.Tools) {
		params.Tools = mergeFreeTierTools(params.Tools)
	}
	if !hadTools && params.ToolChoice == nil {
		none := "none"
		params.ToolChoice = &schemas.ChatToolChoice{ChatToolChoiceStr: &none}
	}
	prepared.Params = params
	return prepared
}

// noopPostHook is the streaming post-hook route used when a free-tier request
// is executed through the streaming surface internally and there is no
// per-chunk transformation to run.
func noopPostHook(_ *schemas.BifrostContext, result *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError) {
	return result, bifrostErr
}

// assembleFreeTierChatResponse consumes a chat stream produced by the
// streaming surface and folds the chunk deltas into a single unary
// BifrostChatResponse, so free-tier requests (which the gate requires to be
// streamed) can be served through the unary chat-completion pipeline. The
// first error chunk wins; an error is returned as-is without a response.
func assembleFreeTierChatResponse(model string, stream chan *schemas.BifrostStreamChunk) (*schemas.BifrostChatResponse, *schemas.BifrostError) {
	var firstErr *schemas.BifrostError
	resp := &schemas.BifrostChatResponse{
		Object:  "chat.completion",
		Choices: []schemas.BifrostResponseChoice{{Index: 0}},
	}
	var contentBuilder strings.Builder
	var finishReason *string
	var usage *schemas.BifrostLLMUsage
	var serviceTier *schemas.BifrostServiceTier

	for chunk := range stream {
		if chunk == nil {
			continue
		}
		if chunk.BifrostError != nil {
			if firstErr == nil {
				firstErr = chunk.BifrostError
			}
			continue
		}
		cr := chunk.BifrostChatResponse
		if cr == nil {
			continue
		}
		if cr.ID != "" {
			resp.ID = cr.ID
		}
		if cr.Model != "" {
			resp.Model = cr.Model
		}
		if cr.Created != 0 {
			resp.Created = cr.Created
		}
		if cr.SystemFingerprint != "" {
			resp.SystemFingerprint = cr.SystemFingerprint
		}
		if cr.ServiceTier != nil {
			serviceTier = cr.ServiceTier
		}
		if cr.Usage != nil {
			usage = cr.Usage
		}
		if len(cr.Choices) == 0 {
			continue
		}
		choice := cr.Choices[0]
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}
		if choice.ChatStreamResponseChoice == nil || choice.ChatStreamResponseChoice.Delta == nil {
			continue
		}
		if d := choice.ChatStreamResponseChoice.Delta; d.Content != nil {
			contentBuilder.WriteString(*d.Content)
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}

	content := contentBuilder.String()
	resp.Choices[0].ChatNonStreamResponseChoice = &schemas.ChatNonStreamResponseChoice{
		Message: &schemas.ChatMessage{
			Role:    schemas.ChatMessageRoleAssistant,
			Content: &schemas.ChatMessageContent{ContentStr: &content},
		},
	}
	resp.Choices[0].FinishReason = finishReason
	resp.Usage = usage
	resp.ServiceTier = serviceTier
	if resp.Model == "" {
		resp.Model = model
	}
	if resp.Created == 0 {
		resp.Created = int(time.Now().Unix())
	}
	if resp.ExtraFields.RequestType == "" {
		resp.ExtraFields.RequestType = schemas.ChatCompletionRequest
	}
	return resp, nil
}
