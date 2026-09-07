package rtk

import (
	"encoding/json"
	"strings"
)

// DefaultSkipReadFileTools is the tool-name whitelist applied to
// skip_read_file_tools when the operator leaves it unset. It targets the
// Read-class MCP tools commonly shipped with Anthropic / OpenCode / Claude
// Code / Cursor / Continue, in both their PascalCase (Claude Code) and
// snake_case (OpenCode / generic MCP server) naming conventions. Operators
// may override this list in config.json.
var DefaultSkipReadFileTools = []string{
	"read_file",
	"Read",
	"Glob",
	"Grep",
	"get_file_info",
	"list_dir",
	"find_files",
	"read_file_range",
	"ReadFile",
	"ReadRange",
	"GetFileInfo",
	"ListDir",
	"FindFiles",
	"search_files",
	"read_pdf",
	"ReadPdf",
}

// skipReadFilePathKeys is the set of top-level JSON keys that identify a
// tool call argument as carrying a filesystem path. Matching is
// case-insensitive. The list is intentionally narrow — adding generic keys
// like "uri" or "url" would risk false positives on network-style tools
// (fetch, http_get) that happen to take a URL.
var skipReadFilePathKeys = []string{
	"file_path",
	"filepath",
	"target_path",
	"offset_path",
	"path",
	"file",
}

// argumentsContainPathKey reports whether args carries a path-like key at
// the top level. It performs a shallow JSON unmarshal into a map of raw
// JSON values — deep recursion would risk flagging nested "path" keys on
// non-read tools. args that is empty or not valid JSON returns false so
// the caller falls through to the normal compression path (fail-open).
func argumentsContainPathKey(args string) bool {
	if args == "" {
		return false
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(args), &top); err != nil {
		return false
	}
	for k := range top {
		kl := strings.ToLower(k)
		for _, want := range skipReadFilePathKeys {
			if strings.EqualFold(want, kl) {
				return true
			}
		}
	}
	return false
}

// shouldSkipReadFileTool reports whether a tool_result for toolName should
// bypass the RTK compression pipeline entirely. The decision is two-stage:
//
//  1. toolName must appear in cfg.SkipReadFileTools (case-insensitive).
//     Empty whitelist or nil cfg returns false (skip-list disabled).
//  2. args must carry a path-like key at the top level (see
//     argumentsContainPathKey). This protects against a same-named tool
//     being used for non-file purposes — e.g. a custom MCP tool called
//     "Read" that takes {"query": "..."} would not be skipped.
//
// The skip path is opt-in per call: returning true here short-circuits
// applyRtkCompression{,Responses} before PipelineRunner.Run is called, so
// the message is passed through untouched, no raw-output pointer is
// written, no ScannedIndices entry is recorded, and OriginalTokens /
// CompressedTokens are not perturbed.
func shouldSkipReadFileTool(toolName, args string, cfg *Config) bool {
	if cfg == nil || len(cfg.SkipReadFileTools) == 0 {
		return false
	}
	if toolName == "" {
		return false
	}
	for _, n := range cfg.SkipReadFileTools {
		if strings.EqualFold(n, toolName) {
			return argumentsContainPathKey(args)
		}
	}
	return false
}
