package rtk

import (
	"testing"
)

// TestShouldSkipNonShellTool_DefaultDisabled verifies that with
// ShellToolsOnly=false (the default), every tool name resolves to false —
// the compression pipeline runs unchanged for shell and non-shell tools alike.
func TestShouldSkipNonShellTool_DefaultDisabled(t *testing.T) {
	cfg := &Config{} // ShellToolsOnly default false
	for _, name := range []string{"bash", "Read", "read_file", "Glob", "sh", "", "web_search"} {
		if shouldSkipNonShellTool(name, cfg) {
			t.Errorf("shouldSkipNonShellTool(%q, ShellToolsOnly=false) = true, want false", name)
		}
	}
}

// TestShouldSkipNonShellTool_ShellToolsOnlyTrue verifies that with
// ShellToolsOnly=true, every known shell tool passes through (compress
// path runs) and every non-shell tool is skipped (bypass).
func TestShouldSkipNonShellTool_ShellToolsOnlyTrue(t *testing.T) {
	cfg := &Config{ShellToolsOnly: true}

	// Shell tools — must NOT be skipped.
	shellTools := []string{
		"bash", "sh", "shell", "zsh", "fish", "ksh", "dash",
		"pwsh", "powershell", "cmd", "command", "terminal",
		"exec", "run", "run_command", "command_executor",
		"execute_command", "execute",
	}
	for _, name := range shellTools {
		if shouldSkipNonShellTool(name, cfg) {
			t.Errorf("shouldSkipNonShellTool(%q) = true, want false (shell tool must pass through)", name)
		}
	}

	// Non-shell tools — must be skipped.
	nonShellTools := []string{
		"Read", "read_file", "Glob", "Grep", "list_dir", "find_files",
		"get_file_info", "search_files", "read_pdf",
		"web_search", "http_get", "fetch",
		"skill", "get_skill", "list_skills",
	}
	for _, name := range nonShellTools {
		if !shouldSkipNonShellTool(name, cfg) {
			t.Errorf("shouldSkipNonShellTool(%q) = false, want true (non-shell tool must skip)", name)
		}
	}
}

// TestShouldSkipNonShellTool_UnknownNameSkipped verifies the conservative
// fail-open-in-the-skip-direction rule: when the correlated tool name is
// missing (empty string under ShellToolsOnly), the output is skipped rather
// than compressed. This matches the docstring rationale on
// Config.ShellToolsOnly — a false negative is recoverable, a false positive
// could eat content the LLM needed.
func TestShouldSkipNonShellTool_UnknownNameSkipped(t *testing.T) {
	cfg := &Config{ShellToolsOnly: true}
	if !shouldSkipNonShellTool("", cfg) {
		t.Errorf("shouldSkipNonShellTool(\"\", ShellToolsOnly=true) = false, want true (unknown name must skip)")
	}
}

// TestShouldSkipNonShellTool_NilCfg verifies a nil config never skips
// (matches the contract of shouldSkipReadFileTool).
func TestShouldSkipNonShellTool_NilCfg(t *testing.T) {
	for _, name := range []string{"bash", "Read", ""} {
		if shouldSkipNonShellTool(name, nil) {
			t.Errorf("shouldSkipNonShellTool(%q, nil) = true, want false (nil cfg must disable)", name)
		}
	}
}

// TestShellToolsOnly_OverridesSkipReadFileTool documents the precedence:
// when ShellToolsOnly=true, the SkipReadFileTools whitelist is irrelevant
// because every non-shell tool is bypassed anyway. shouldSkipReadFileTool
// still fires for shell-class tools that happen to appear in the whitelist,
// but the higher-level compressor runs ShellToolsOnly first and short-circuits
// before the whitelist check. This test pins the helper-level contract; the
// end-to-end precedence is exercised by the compression integration tests.
func TestShellToolsOnly_OverridesSkipReadFileTool(t *testing.T) {
	cfg := &Config{
		ShellToolsOnly:    true,
		SkipReadFileTools: []string{"Read"}, // narrowed to a single name
	}
	// A non-shell tool whose name is NOT in the whitelist would still
	// be skipped by the read-file helper — but ShellToolsOnly bypasses
	// that path entirely.
	if !shouldSkipNonShellTool("web_search", cfg) {
		t.Errorf("web_search must be skipped under ShellToolsOnly regardless of whitelist")
	}
	// A non-shell tool in the whitelist: both rules agree → skip.
	if !shouldSkipNonShellTool("Read", cfg) {
		t.Errorf("Read must be skipped under ShellToolsOnly")
	}
	if !shouldSkipReadFileTool("Read", `{"file_path": "/etc/hostname"}`, cfg) {
		t.Errorf("Read with file_path must also be skipped by the read-file helper")
	}
	// A shell tool: ShellToolsOnly passes through; whitelist irrelevant.
	if shouldSkipNonShellTool("bash", cfg) {
		t.Errorf("bash must NOT be skipped under ShellToolsOnly")
	}
}

func TestArgumentsContainPathKey(t *testing.T) {
	cases := []struct {
		name string
		args string
		want bool
	}{
		{"empty", "", false},
		{"non-json", "not a json object", false},
		{"json-array", `["file_path", "x"]`, false},
		{"file_path", `{"file_path": "/etc/hostname"}`, true},
		{"filePath mixed case", `{"filePath": "/etc/hostname"}`, true},
		{"filepath", `{"filepath": "/etc/hostname"}`, true},
		{"path", `{"path": "/etc/hostname"}`, true},
		{"target_path", `{"target_path": "/etc/hostname"}`, true},
		{"offset_path", `{"offset_path": "/etc/hostname"}`, true},
		{"file", `{"file": "/etc/hostname"}`, true},
		{"nested-file-path not matched", `{"options": {"file_path": "/etc"}}`, false},
		{"unrelated key", `{"query": "*.go"}`, false},
		{"mix of unrelated + path", `{"limit": 10, "path": "/etc/hostname"}`, true},
		{"weird unicode but valid key", `{"FILE_PATH": "/etc/hostname"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := argumentsContainPathKey(tc.args)
			if got != tc.want {
				t.Errorf("argumentsContainPathKey(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestArgumentsContainSkillKey(t *testing.T) {
	cases := []struct {
		name string
		args string
		want bool
	}{
		{"empty", "", false},
		{"non-json", "not a json object", false},
		{"json-array", `["name", "x"]`, false},
		{"name", `{"name": "pg-build"}`, true},
		{"skill_name", `{"skill_name": "docs-writer"}`, true},
		{"skill", `{"skill": "add-pricing-field"}`, true},
		{"skill_id", `{"skill_id": "s1"}`, true},
		{"mixed case", `{"SKILL_NAME": "docs-writer"}`, true},
		{"nested name not matched", `{"options": {"name": "x"}}`, false},
		{"path key not matched", `{"file_path": "/etc/hostname"}`, false},
		{"unrelated key", `{"query": "*.go"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := argumentsContainSkillKey(tc.args)
			if got != tc.want {
				t.Errorf("argumentsContainSkillKey(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestShouldSkipReadFileTool_DefaultWhitelist(t *testing.T) {
	cfg := &Config{SkipReadFileTools: append([]string{}, DefaultSkipReadFileTools...)}
	cases := []struct {
		name     string
		toolName string
		args     string
		want     bool
	}{
		{"read_file with path", "read_file", `{"file_path": "/etc/hostname"}`, true},
		{"Read PascalCase", "Read", `{"file_path": "/etc/hostname"}`, true},
		{"Glob with file", "Glob", `{"file": "*.go"}`, true},
		{"Grep with path", "Grep", `{"path": "/repo"}`, true},
		{"list_dir with path", "list_dir", `{"path": "/repo"}`, true},
		{"non-whitelisted name", "cat_file", `{"file_path": "/etc/hostname"}`, false},
		{"whitelisted but no path key", "Read", `{"query": "*.go"}`, false},
		{"empty args with whitelisted name", "Read", "", false},
		{"empty toolName", "", `{"file_path": "/etc"}`, false},
		{"name matches but args not JSON", "Read", "not json", false},
		{"case-insensitive name match", "READ", `{"file_path": "/etc"}`, true},
		{"opencode skill by name", "skill", `{"name": "pg-build"}`, true},
		{"opencode Skill PascalCase", "Skill", `{"name": "pg-build"}`, true},
		{"claude get_skill by skill_name", "get_skill", `{"skill_name": "docs-writer"}`, true},
		{"claude GetSkill PascalCase", "GetSkill", `{"skill": "api-validator"}`, true},
		{"MCP load_skill", "load_skill", `{"skill_id": "s1"}`, true},
		{"skill tool with path key", "skill", `{"path": "skills/x/SKILL.md"}`, true},
		{"skill tool with unrelated key", "skill", `{"query": "x"}`, false},
		{"skill tool with empty args", "skill", "", false},
		{"list_skills empty args", "list_skills", `{}`, false},
		{"whitelisted read tool ignores name key", "Read", `{"name": "file.txt"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldSkipReadFileTool(tc.toolName, tc.args, cfg)
			if got != tc.want {
				t.Errorf("shouldSkipReadFileTool(%q, %q) = %v, want %v", tc.toolName, tc.args, got, tc.want)
			}
		})
	}
}

func TestShouldSkipReadFileTool_DisabledWhenWhitelistEmpty(t *testing.T) {
	cfg := &Config{SkipReadFileTools: []string{}}
	if shouldSkipReadFileTool("read_file", `{"file_path": "/etc"}`, cfg) {
		t.Errorf("empty whitelist must disable skip even for whitelisted name")
	}
}

func TestShouldSkipReadFileTool_NilCfg(t *testing.T) {
	if shouldSkipReadFileTool("read_file", `{"file_path": "/etc"}`, nil) {
		t.Errorf("nil cfg must disable skip")
	}
}

func TestShouldSkipReadFileTool_CustomWhitelist(t *testing.T) {
	cfg := &Config{SkipReadFileTools: []string{"cat_file", "view_image"}}
	if !shouldSkipReadFileTool("cat_file", `{"file_path": "/etc"}`, cfg) {
		t.Errorf("custom whitelist should match cat_file")
	}
	if shouldSkipReadFileTool("read_file", `{"file_path": "/etc"}`, cfg) {
		t.Errorf("default-list name not in custom whitelist should not match")
	}
	// view_image has no path arg → not skipped
	if shouldSkipReadFileTool("view_image", `{"image_id": "x"}`, cfg) {
		t.Errorf("custom whitelist hit without path key must not skip")
	}
}
