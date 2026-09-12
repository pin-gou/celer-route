package rtk

import (
	"testing"
)

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
