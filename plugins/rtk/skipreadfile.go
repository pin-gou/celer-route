package rtk

import (
	"encoding/json"
	"strings"
)

// DefaultSkipReadFileTools is the tool-name whitelist applied to
// skip_read_file_tools when the operator leaves it unset. It targets the
// Read-class MCP tools commonly shipped with Anthropic / OpenCode / Claude
// Code / Cursor / Continue, in both their PascalCase (Claude Code) and
// snake_case (OpenCode / generic MCP server) naming conventions, plus the
// skill-loading tools (opencode's `skill`, Claude Code's `get_skill` /
// `list_skills`, generic MCP `load_skill`) whose results carry the full
// SKILL.md body and must stay verbatim for the LLM. Operators may override
// this list in config.json.
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
	"skill",
	"Skill",
	"get_skill",
	"GetSkill",
	"list_skills",
	"ListSkills",
	"load_skill",
	"LoadSkill",
}

// skillToolNames is the subset of DefaultSkipReadFileTools whose arguments
// identify a skill by name rather than a filesystem path (opencode `skill`
// takes {"name": ...}, Claude Code `get_skill` takes {"skill_name": ...}).
// Tools in this list are skipped when their args carry a skill-name key;
// every other whitelisted tool still requires a path-like key.
var skillToolNames = []string{
	"skill",
	"get_skill",
	"GetSkill",
	"list_skills",
	"ListSkills",
	"load_skill",
	"LoadSkill",
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

// skipReadFileSkillKeys is the set of top-level JSON keys that identify a
// tool call argument as carrying a skill name rather than a filesystem
// path. Matching is case-insensitive. This set is deliberately separate
// from skipReadFilePathKeys: `name` is too generic to apply to arbitrary
// whitelisted tools (a custom "Read" taking {"name": ...}), so skill keys
// are only consulted for tools classified in skillToolNames.
var skipReadFileSkillKeys = []string{
	"name",
	"skill_name",
	"skill",
	"skill_id",
}

// argumentsContainAnyKey reports whether args carries any of keys at the
// top level. It performs a shallow JSON unmarshal into a map of raw JSON
// values — deep recursion would risk flagging nested keys on non-read
// tools. args that is empty or not valid JSON returns false so the caller
// falls through to the normal compression path (fail-open).
func argumentsContainAnyKey(args string, keys []string) bool {
	if args == "" {
		return false
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(args), &top); err != nil {
		return false
	}
	for k := range top {
		for _, want := range keys {
			if strings.EqualFold(want, k) {
				return true
			}
		}
	}
	return false
}

// argumentsContainPathKey reports whether args carries a path-like key at
// the top level (see skipReadFilePathKeys).
func argumentsContainPathKey(args string) bool {
	return argumentsContainAnyKey(args, skipReadFilePathKeys)
}

// argumentsContainSkillKey reports whether args carries a skill-name key at
// the top level (see skipReadFileSkillKeys).
func argumentsContainSkillKey(args string) bool {
	return argumentsContainAnyKey(args, skipReadFileSkillKeys)
}

// isSkillToolName reports whether name is one of the built-in skill-loading
// tool names (case-insensitive).
func isSkillToolName(name string) bool {
	for _, n := range skillToolNames {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// shouldSkipReadFileTool reports whether a tool_result for toolName should
// bypass the RTK compression pipeline entirely. The decision is two-stage:
//
//  1. toolName must appear in cfg.SkipReadFileTools (case-insensitive).
//     Empty whitelist or nil cfg returns false (skip-list disabled).
//  2. args must carry a matching key at the top level. For skill-loading
//     tools (see skillToolNames) this is a skill-name key (name /
//     skill_name / skill / skill_id); for every other whitelisted tool it
//     is a path-like key (see argumentsContainPathKey). This protects
//     against a same-named tool being used for non-file purposes — e.g. a
//     custom MCP tool called "Read" that takes {"query": "..."} would not
//     be skipped.
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
			if isSkillToolName(toolName) {
				return argumentsContainSkillKey(args) || argumentsContainPathKey(args)
			}
			return argumentsContainPathKey(args)
		}
	}
	return false
}
