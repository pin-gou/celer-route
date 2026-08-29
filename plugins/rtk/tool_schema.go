package rtk

import (
	"strings"

	"github.com/pin-gou/celer-route/core/schemas"
)

// rtkFetchRawOutputToolName is the full, prefixed tool name of the
// rtk_fetch_raw_output MCP tool. The bifrostInternal- prefix is load-bearing:
// MCPManager.RegisterTool (clientmanager.go:1736) stores every tool under the
// prefixed name (clientmanager.go:1767), so the schema PreLLMHook injects into
// req.Params.Tools must carry the same prefixed name to be recognised by the
// agent loop. Do NOT rename this without a full tool-call round-trip test.
const rtkFetchRawOutputToolName = "bifrostInternal-rtk_fetch_raw_output"

// rtkFetchRawOutputToolDescription is the LLM-facing description of the
// rtk_fetch_raw_output tool. It is byte-stable (a package-level var built once
// at init from string literals) so Anthropic / OpenAI prompt caches keep
// hitting on the system prefix across requests. The text mirrors the recovery
// hint's semantics: how to read the [rtk:raw_output_id=...] marker, what a
// valid id looks like, and that the recovered body is unwrapped by RTK on the
// next compression pass.
var rtkFetchRawOutputToolDescription = strings.Join([]string{
	"Reads the original (untruncated) output of a tool_result that the RTK",
	"compression plugin previously truncated. Use this when you see a marker",
	"like [rtk:raw_output_id=<24hex>; orig=<size>; ...] at the end of a",
	"tool_result and you need the full content. The 24-char hex id is the",
	"value after raw_output_id=. The body is automatically unwrapped by RTK",
	"on the next request so the response you receive is the raw file content.",
}, " ")

// RtkFetchRawOutputTool is the schemas.ChatTool declaration for the
// rtk_fetch_raw_output MCP tool. Exposed to the LLM as a regular function
// tool in the chat request's tools= array. The function name carries the
// bifrostInternal- prefix because MCPManager.RegisterTool (clientmanager.go:1767)
// prefixes the tool name with BifrostMCPClientKey when storing it.
//
// Byte-stable: keep this struct literal unchanged across releases so
// Anthropic and OpenAI prompt caches still hit on the system prefix when
// the same tool schema is appended to req.Params.Tools.
var RtkFetchRawOutputTool = schemas.ChatTool{
	Type: schemas.ChatToolTypeFunction,
	Function: &schemas.ChatToolFunction{
		Name:        rtkFetchRawOutputToolName,
		Description: &rtkFetchRawOutputToolDescription,
		Parameters: &schemas.ToolFunctionParameters{
			Type: "object",
			Properties: schemas.NewOrderedMapFromPairs(
				schemas.KV("id", map[string]any{
					"type":        "string",
					"description": "24-char lowercase hex id from the [rtk:raw_output_id=...] marker",
					"pattern":     "^[0-9a-f]{24}$",
				}),
			),
			Required: []string{"id"},
		},
	},
}