package rtk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFilterLoaderBuiltin verifies that built-in filters are loaded correctly
// and the loader returns non-nil filters for known commands.
func TestFilterLoaderBuiltin(t *testing.T) {
	loader := NewFilterLoader(DefaultConfig())
	if loader == nil {
		t.Fatal("NewFilterLoader returned nil")
	}

	// Built-in filters should be pre-loaded
	builtinFilters := loader.ListBuiltinFilters()
	if builtinFilters == nil {
		t.Fatal("ListBuiltinFilters returned nil")
	}
	if len(builtinFilters) == 0 {
		t.Fatal("ListBuiltinFilters returned empty slice")
	}

	// Verify expected filter names exist
	expectedFilters := []string{"git-status", "npm-install", "docker-logs", "generic-output"}
	for _, name := range expectedFilters {
		found := false
		for _, f := range builtinFilters {
			if f.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected builtin filter %q not found in loaded filters", name)
		}
	}
}

// TestFilterLoaderMatch verifies that the correct filter is matched for
// different command types.
func TestFilterLoaderMatch(t *testing.T) {
	loader := NewFilterLoader(DefaultConfig())
	if loader == nil {
		t.Fatal("NewFilterLoader returned nil")
	}

	tests := []struct {
		name        string
		commandType string
		command     string
		wantFilter  string // expected filter name
		wantMatch   bool
	}{
		{
			name:        "git_status_matches_git_filter",
			commandType: "shell",
			command:     "git status",
			wantFilter:  "git-status",
			wantMatch:   true,
		},
		{
			name:        "npm_install_matches_npm_filter",
			commandType: "shell",
			command:     "npm install express",
			wantFilter:  "npm-install",
			wantMatch:   true,
		},
		{
			name:        "docker_logs_matches_docker_filter",
			commandType: "shell",
			command:     "docker logs mycontainer",
			wantFilter:  "docker-logs",
			wantMatch:   true,
		},
		{
			name:        "unknown_command_falls_to_generic",
			commandType: "shell",
			command:     "some-unknown-tool --verbose",
			wantFilter:  "generic-output",
			wantMatch:   true,
		},
		{
			name:        "non_shell_returns_no_filter",
			commandType: "api",
			command:     "",
			wantFilter:  "",
			wantMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := loader.Match(tt.commandType, tt.command)
			if tt.wantMatch && filter == nil {
				t.Fatalf("Match(%q, %q) returned nil, expected a filter", tt.commandType, tt.command)
			}
			if !tt.wantMatch && filter != nil {
				t.Fatalf("Match(%q, %q) returned %q, expected nil", tt.commandType, tt.command, filter.Name)
			}
			if tt.wantMatch && filter.Name != tt.wantFilter {
				t.Errorf("Match(%q, %q).Name = %q, want %q", tt.commandType, tt.command, filter.Name, tt.wantFilter)
			}
		})
	}
}

// TestFilterLoaderPriority verifies the priority matching order:
// project > global > builtin > generic-output fallback.
func TestFilterLoaderPriority(t *testing.T) {
	loader := NewFilterLoader(DefaultConfig())
	if loader == nil {
		t.Fatal("NewFilterLoader returned nil")
	}

	// Register a project-level override for git-status
	projectFilter := &Filter{
		Name:    "git-status-project",
		Command: "git status",
		Rules:   []LineRule{},
	}
	loader.RegisterProjectFilter(projectFilter)

	// Register a global-level override
	globalFilter := &Filter{
		Name:    "git-status-global",
		Command: "git status",
		Rules:   []LineRule{},
	}
	loader.RegisterGlobalFilter(globalFilter)

	// Project filter should take priority over global
	matched := loader.Match("shell", "git status")
	if matched == nil {
		t.Fatal("Match returned nil for git status")
	}
	if matched.Name != "git-status-project" {
		t.Errorf("project filter should have priority, got %q, want %q", matched.Name, "git-status-project")
	}
}

// TestFilterLoaderReDoSProtection verifies that ReDoS-prone regex patterns
// are rejected by the filter loader.
func TestFilterLoaderReDoSProtection(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantBad bool // true = pattern should be detected as ReDoS-prone
	}{
		{
			name:    "nested_quantifier_redos",
			pattern: `(a+)+b`,
			wantBad: true,
		},
		{
			name:    "alternation_with_repetition",
			pattern: `(a|b)+c`,
			wantBad: true,
		},
		{
			name:    "nested_groups",
			pattern: `((a+)+)+b`,
			wantBad: true,
		},
		{
			name:    "safe_simple_pattern",
			pattern: `^On branch`,
			wantBad: false,
		},
		{
			name:    "safe_character_class",
			pattern: `^modified:\s+`,
			wantBad: false,
		},
		{
			name:    "safe_literal",
			pattern: `fatal:`,
			wantBad: false,
		},
		{
			name:    "safe_optional_quantifier",
			pattern: `error:\s?`,
			wantBad: false,
		},
		{
			name:    "empty_pattern",
			pattern: ``,
			wantBad: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isReDoSProne(tt.pattern)
			if got != tt.wantBad {
				t.Errorf("isReDoSProne(%q) = %v, want %v", tt.pattern, got, tt.wantBad)
			}
		})
	}
}

// TestFilterLoaderReDoSRejectsBadFilter verifies that a filter containing
// a ReDoS-prone pattern is rejected during loading.
func TestFilterLoaderReDoSRejectsBadFilter(t *testing.T) {
	badFilter := &Filter{
		Name:    "bad-regex",
		Command: "evil-command",
		Rules: []LineRule{
			{
				Type:    "strip",
				Pattern: `(a+)+b`, // ReDoS-prone
			},
		},
	}

	loader := NewFilterLoader(DefaultConfig())
	if loader == nil {
		t.Fatal("NewFilterLoader returned nil")
	}

	err := loader.RegisterProjectFilter(badFilter)
	if err == nil {
		t.Error("expected error for ReDoS-prone filter, got nil")
	}
}

// TestFilterLoaderListGlobalFilters verifies global filter listing.
func TestFilterLoaderListGlobalFilters(t *testing.T) {
	loader := NewFilterLoader(DefaultConfig())
	globalFilters := loader.ListGlobalFilters()
	if globalFilters == nil {
		t.Fatal("ListGlobalFilters returned nil")
	}
}

// TestFilterLoaderListProjectFilters verifies project filter listing.
func TestFilterLoaderListProjectFilters(t *testing.T) {
	loader := NewFilterLoader(DefaultConfig())
	projectFilters := loader.ListProjectFilters()
	if projectFilters == nil {
		t.Fatal("ListProjectFilters returned nil")
	}
}

// TestFilterLoaderMatchNilSafety verifies nil-safety.
func TestFilterLoaderMatchNilSafety(t *testing.T) {
	var loader *FilterLoader
	filter := loader.Match("shell", "git status")
	if filter != nil {
		t.Error("Match on nil loader should return nil")
	}
}

// TestFilterLoaderMultipleProjectFilters verifies that multiple project filters
// are matched in order and the most specific one wins.
func TestFilterLoaderMultipleProjectFilters(t *testing.T) {
	loader := NewFilterLoader(DefaultConfig())

	// Register a generic git filter
	genericGit := &Filter{
		Name:    "git-generic",
		Command: "git",
		Rules:   []LineRule{},
	}
	loader.RegisterProjectFilter(genericGit)

	// Register a specific git status filter
	gitStatus := &Filter{
		Name:    "git-status-specific",
		Command: "git status",
		Rules:   []LineRule{},
	}
	loader.RegisterProjectFilter(gitStatus)

	// The more specific match should win
	matched := loader.Match("shell", "git status")
	if matched == nil {
		t.Fatal("Match returned nil")
	}
	if matched.Name != "git-status-specific" {
		t.Errorf("expected more specific filter to win, got %q, want %q", matched.Name, "git-status-specific")
	}
}

// TestFilter structure tests
func TestFilterStructure(t *testing.T) {
	f := &Filter{
		Name:    "test-filter",
		Command: "test-command",
		Rules: []LineRule{
			{
				Type:    "strip",
				Pattern: `^DEBUG`,
			},
			{
				Type:    "keep",
				Pattern: `ERROR`,
			},
			{
				Type:    "collapse",
				Pattern: `^\s*$`,
			},
			{
				Type:    "replace",
				Pattern: `old`,
				Replacement: "new",
			},
		},
		Head:    10,
		Tail:    5,
		MaxLines: 100,
	}

	if f.Name != "test-filter" {
		t.Errorf("Filter.Name = %q, want %q", f.Name, "test-filter")
	}
	if len(f.Rules) != 4 {
		t.Errorf("Filter.Rules length = %d, want 4", len(f.Rules))
	}
	if f.Head != 10 {
		t.Errorf("Filter.Head = %d, want 10", f.Head)
	}
	if f.Tail != 5 {
		t.Errorf("Filter.Tail = %d, want 5", f.Tail)
	}
	if f.MaxLines != 100 {
		t.Errorf("Filter.MaxLines = %d, want 100", f.MaxLines)
	}
}

// TestLineRuleStructure tests the LineRule structure.
func TestLineRuleStructure(t *testing.T) {
	rule := LineRule{
		Type:        "strip",
		Pattern:     `^DEBUG.*`,
		Replacement: "",
	}
	if rule.Type != "strip" {
		t.Errorf("LineRule.Type = %q, want %q", rule.Type, "strip")
	}
	if rule.Pattern != `^DEBUG.*` {
		t.Errorf("LineRule.Pattern = %q, want %q", rule.Pattern, `^DEBUG.*`)
	}
}

// ============================================================================
// Phase 3: Custom filters (TDD red phase)
//
// The tests below reference the stage-3 design contract from design.md:
//   - Filter gained canonical fields (id/label/category/priority/tests/...)
//     plus a dual-format UnmarshalJSON arbitrator.
//   - FilterLoader gained Load(appDir), Diagnostics(), cachedFilters, appDir.
//   - Config gained CustomFiltersEnabled / TrustProjectFilters /
//     EnabledFilters / DisabledFilters.
//   - Plugin.Init now takes appDir and Plugin holds a *FilterLoader.
//
// NONE of these symbols exist in production code yet, so this whole package
// fails to compile — the expected TDD red-phase result. The dev phase makes
// the package compile and the assertions below turn green.
// ============================================================================

// Test helpers (shared with diagnostics_test.go).

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// filtersSHA256Hex returns the hex-encoded SHA-256 of a filters.json payload,
// matching the value a trust.json filtersSha256 field must contain.
func filtersSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func findFilterByName(t *testing.T, filters []*Filter, name string) *Filter {
	t.Helper()
	for _, f := range filters {
		if f != nil && f.Name == name {
			return f
		}
	}
	t.Fatalf("filter %q not found", name)
	return nil
}

// filterIDs returns the (canonical) ids of all cached filters, in load order.
func filterIDs(t *testing.T, loader *FilterLoader) []string {
	t.Helper()
	if loader == nil {
		t.Fatal("nil loader")
	}
	ids := make([]string, 0, len(loader.cachedFilters))
	for _, f := range loader.cachedFilters {
		if f != nil {
			ids = append(ids, f.ID)
		}
	}
	return ids
}

// ============================================================================
// 1.1 V-plugins-1: dual-format Filter JSON parsing (legacy + canonical)
// ============================================================================

// TestUnmarshalDualFormat verifies that a single Filter struct correctly
// parses BOTH the legacy omniroute format ({name, command, rules, head, tail})
// and the canonical format ({id, label, category, priority, ...}).
// TDD red phase: the canonical fields do not exist yet (compile error).
func TestUnmarshalDualFormat(t *testing.T) {
	dir := t.TempDir()

	// legacy format — field-for-field maps onto the current Filter struct.
	legacyPath := filepath.Join(dir, "legacy.json")
	mustWriteFile(t, legacyPath, []byte(`{
		"name": "git-status",
		"command": "git status",
		"rules": [{"type": "strip", "pattern": "^DEBUG"}],
		"head": 10,
		"tail": 5,
		"max_lines": 200,
		"priority_patterns": ["^fatal:"]
	}`))

	legacyData, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", legacyPath, err)
	}
	var legacy Filter
	if err := json.Unmarshal(legacyData, &legacy); err != nil {
		t.Fatalf("failed to unmarshal legacy JSON: %v", err)
	}
	// (a) legacy fields all populate.
	if legacy.Name != "git-status" {
		t.Errorf("legacy Name = %q, want %q", legacy.Name, "git-status")
	}
	if legacy.Command != "git status" {
		t.Errorf("legacy Command = %q, want %q", legacy.Command, "git status")
	}
	if len(legacy.Rules) != 1 || legacy.Rules[0].Type != "strip" || legacy.Rules[0].Pattern != "^DEBUG" {
		t.Errorf("legacy Rules = %+v, want [{strip ^DEBUG}]", legacy.Rules)
	}
	if legacy.Head != 10 {
		t.Errorf("legacy Head = %d, want 10", legacy.Head)
	}
	if legacy.Tail != 5 {
		t.Errorf("legacy Tail = %d, want 5", legacy.Tail)
	}
	if legacy.MaxLines != 200 {
		t.Errorf("legacy MaxLines = %d, want 200", legacy.MaxLines)
	}
	if len(legacy.PriorityPatterns) != 1 || legacy.PriorityPatterns[0] != "^fatal:" {
		t.Errorf("legacy PriorityPatterns = %v, want [^fatal:]", legacy.PriorityPatterns)
	}
	// UnmarshalJSON arbitration step 3: Name → ID fallback.
	if legacy.ID != "git-status" {
		t.Errorf("legacy ID (arbitration Name→ID) = %q, want %q", legacy.ID, "git-status")
	}

	// canonical format — flat fields matching the design.md struct tags.
	canonicalPath := filepath.Join(dir, "canonical.json")
	mustWriteFile(t, canonicalPath, []byte(`{
		"id": "git-status-canonical",
		"label": "Git Status Canonical",
		"description": "Canonical format git status filter",
		"category": "git",
		"priority": 80,
		"commandPatterns": ["^git\\s+status"],
		"matchPatterns": ["^On branch"],
		"outputTypes": ["shell"],
		"stripPatterns": ["^\\s*$"],
		"keepPatterns": ["^On branch"],
		"collapsePatterns": ["^.+$"],
		"stripAnsi": true,
		"replace": [{"pattern": "/home/user", "replacement": "~"}],
		"matchOutput": [{"pattern": "fatal:", "message": "git fatal error"}],
		"truncateLineAt": 120,
		"onEmpty": "ignore",
		"filterStderr": true,
		"deduplicate": true,
		"head_lines": 15,
		"tail_lines": 8,
		"errorPatterns": ["^fatal:"],
		"summaryPatterns": ["files changed"],
		"tests": [
			{"name": "canonical-ok", "input": "On branch main", "expected": "On branch main", "command": "git status"}
		]
	}`))

	canonicalData, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", canonicalPath, err)
	}
	var canonical Filter
	if err := json.Unmarshal(canonicalData, &canonical); err != nil {
		t.Fatalf("failed to unmarshal canonical JSON: %v", err)
	}
	// (b) canonical fields all populate.
	if canonical.ID != "git-status-canonical" {
		t.Errorf("canonical ID = %q, want %q", canonical.ID, "git-status-canonical")
	}
	if canonical.Label != "Git Status Canonical" {
		t.Errorf("canonical Label = %q, want %q", canonical.Label, "Git Status Canonical")
	}
	if canonical.Description != "Canonical format git status filter" {
		t.Errorf("canonical Description = %q, want %q", canonical.Description, "Canonical format git status filter")
	}
	if canonical.Category != "git" {
		t.Errorf("canonical Category = %q, want %q", canonical.Category, "git")
	}
	if canonical.Priority != 80 {
		t.Errorf("canonical Priority = %d, want 80", canonical.Priority)
	}
	if len(canonical.CommandPatterns) != 1 || canonical.CommandPatterns[0] != "^git\\s+status" {
		t.Errorf("canonical CommandPatterns = %v, want [^git\\s+status]", canonical.CommandPatterns)
	}
	if len(canonical.MatchPatterns) != 1 || canonical.MatchPatterns[0] != "^On branch" {
		t.Errorf("canonical MatchPatterns = %v, want [^On branch]", canonical.MatchPatterns)
	}
	if len(canonical.OutputTypes) != 1 || canonical.OutputTypes[0] != "shell" {
		t.Errorf("canonical OutputTypes = %v, want [shell]", canonical.OutputTypes)
	}
	if len(canonical.StripPatterns) != 1 || canonical.StripPatterns[0] != "^\\s*$" {
		t.Errorf("canonical StripPatterns = %v, want [^\\s*$]", canonical.StripPatterns)
	}
	if len(canonical.KeepPatterns) != 1 || canonical.KeepPatterns[0] != "^On branch" {
		t.Errorf("canonical KeepPatterns = %v, want [^On branch]", canonical.KeepPatterns)
	}
	if len(canonical.CollapsePatterns) != 1 || canonical.CollapsePatterns[0] != "^.+$" {
		t.Errorf("canonical CollapsePatterns = %v, want [^.+$]", canonical.CollapsePatterns)
	}
	if !canonical.StripAnsi {
		t.Error("canonical StripAnsi = false, want true")
	}
	if len(canonical.Replace) != 1 || canonical.Replace[0].Pattern != "/home/user" || canonical.Replace[0].Replacement != "~" {
		t.Errorf("canonical Replace = %+v, want [{/home/user ~}]", canonical.Replace)
	}
	if len(canonical.MatchOutput) != 1 || canonical.MatchOutput[0].Pattern != "fatal:" || canonical.MatchOutput[0].Message != "git fatal error" {
		t.Errorf("canonical MatchOutput = %+v, want [{fatal: git fatal error}]", canonical.MatchOutput)
	}
	if canonical.TruncateLineAt != 120 {
		t.Errorf("canonical TruncateLineAt = %d, want 120", canonical.TruncateLineAt)
	}
	if canonical.OnEmpty != "ignore" {
		t.Errorf("canonical OnEmpty = %q, want %q", canonical.OnEmpty, "ignore")
	}
	if !canonical.FilterStderr {
		t.Error("canonical FilterStderr = false, want true")
	}
	if !canonical.Deduplicate {
		t.Error("canonical Deduplicate = false, want true")
	}
	if len(canonical.ErrorPatterns) != 1 || canonical.ErrorPatterns[0] != "^fatal:" {
		t.Errorf("canonical ErrorPatterns = %v, want [^fatal:]", canonical.ErrorPatterns)
	}
	if len(canonical.SummaryPatterns) != 1 || canonical.SummaryPatterns[0] != "files changed" {
		t.Errorf("canonical SummaryPatterns = %v, want [files changed]", canonical.SummaryPatterns)
	}
	if len(canonical.Tests) != 1 {
		t.Fatalf("canonical Tests = %d entries, want 1", len(canonical.Tests))
	}
	ft := canonical.Tests[0]
	if ft.Name != "canonical-ok" || ft.Input != "On branch main" || ft.Expected != "On branch main" || ft.Command != "git status" {
		t.Errorf("canonical Tests[0] = %+v, want {canonical-ok On branch main On branch main git status}", ft)
	}
	// UnmarshalJSON arbitration: head_lines/tail_lines fill the legacy fields,
	// and ID → Name fallback.
	if canonical.Head != 15 || canonical.HeadLines != 15 {
		t.Errorf("canonical Head=%d HeadLines=%d, want 15/15 (head_lines arbitration)", canonical.Head, canonical.HeadLines)
	}
	if canonical.Tail != 8 || canonical.TailLines != 8 {
		t.Errorf("canonical Tail=%d TailLines=%d, want 8/8 (tail_lines arbitration)", canonical.Tail, canonical.TailLines)
	}
	if canonical.Name != "git-status-canonical" {
		t.Errorf("canonical Name (arbitration ID→Name) = %q, want %q", canonical.Name, "git-status-canonical")
	}
}

// TestUnmarshalDualFormatArbitration verifies the head/tail arbitration rule:
// canonical head_lines/tail_lines override legacy head/tail when both are set,
// and the canonical field is backfilled from the legacy one when only the
// legacy field is present. TDD red phase: the canonical fields do not exist
// yet (compile error).
func TestUnmarshalDualFormatArbitration(t *testing.T) {
	cases := []struct {
		name        string
		data        string
		wantHead    int
		wantHeadLn  int
		wantTail    int
		wantTailLn  int
	}{
		{
			name:       "canonical_head_lines_override_legacy_head",
			data:       `{"head_lines": 15, "head": 5, "tail_lines": 9, "tail": 4}`,
			wantHead:   15,
			wantHeadLn: 15,
			wantTail:   9,
			wantTailLn: 9,
		},
		{
			name:       "legacy_only_backfills_canonical",
			data:       `{"name": "x", "head": 20, "tail": 4}`,
			wantHead:   20,
			wantHeadLn: 20,
			wantTail:   4,
			wantTailLn: 4,
		},
		{
			name:       "canonical_only_populates_legacy",
			data:       `{"id": "y", "head_lines": 7}`,
			wantHead:   7,
			wantHeadLn: 7,
			wantTail:   0,
			wantTailLn: 0,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var f Filter
			if err := json.Unmarshal([]byte(tt.data), &f); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if f.Head != tt.wantHead {
				t.Errorf("Head = %d, want %d", f.Head, tt.wantHead)
			}
			if f.HeadLines != tt.wantHeadLn {
				t.Errorf("HeadLines = %d, want %d", f.HeadLines, tt.wantHeadLn)
			}
			if f.Tail != tt.wantTail {
				t.Errorf("Tail = %d, want %d", f.Tail, tt.wantTail)
			}
			if f.TailLines != tt.wantTailLn {
				t.Errorf("TailLines = %d, want %d", f.TailLines, tt.wantTailLn)
			}
		})
	}
}

// TestBuiltinFiltersUnchanged verifies zero-migration: all builtin legacy JSON
// filters still parse under the extended Filter struct with their legacy
// fields intact, and the canonical ID is backfilled from Name.
// TDD red phase: FilterLoader.Load does not exist yet (compile error).
func TestBuiltinFiltersUnchanged(t *testing.T) {
	loader := NewFilterLoader(DefaultConfig())
	emptyDir := t.TempDir()
	if err := loader.Load(emptyDir); err != nil {
		t.Fatalf("Load(empty appDir) should succeed for builtins, got: %v", err)
	}

	builtins := loader.ListBuiltinFilters()
	if len(builtins) < 50 {
		t.Fatalf("expected at least 50 builtin filters (embed FS ships 52), got %d", len(builtins))
	}
	names := map[string]bool{}
	for _, f := range builtins {
		if f == nil {
			t.Error("nil filter in builtin list")
			continue
		}
		if f.Name == "" {
			t.Errorf("builtin filter with empty Name: %+v", f)
		}
		if f.ID != f.Name {
			t.Errorf("builtin %q: ID should mirror Name (arbitration), got ID=%q", f.Name, f.ID)
		}
		names[f.Name] = true
	}
	for _, want := range []string{"git-status", "npm-install", "docker-logs", "go-test", "kubectl-get", "generic-output"} {
		if !names[want] {
			t.Errorf("expected builtin filter %q not loaded", want)
		}
	}

	// git-status legacy fields must remain intact (zero migration).
	gs := findFilterByName(t, builtins, "git-status")
	if gs.Command != "git status" {
		t.Errorf("git-status Command = %q, want %q", gs.Command, "git status")
	}
	if gs.Head != 5 {
		t.Errorf("git-status Head = %d, want 5", gs.Head)
	}
	if gs.Tail != 2 {
		t.Errorf("git-status Tail = %d, want 2", gs.Tail)
	}
	if len(gs.PriorityPatterns) == 0 || gs.PriorityPatterns[0] != "^fatal:" {
		t.Errorf("git-status PriorityPatterns = %v, want leading ^fatal:", gs.PriorityPatterns)
	}

	// The cached filter set must be populated after Load.
	if len(loader.cachedFilters) == 0 {
		t.Error("loader.cachedFilters empty after Load")
	}
}

// ============================================================================
// 1.2 V-plugins-2: project > global > builtin priority + whitelist/blacklist
// ============================================================================

// writeProjectGlobalSources lays out a three-tier fixture under root:
//
//	root/.rtk/filters.json  → project source (rank=3)
//	root/rtk/filters.json   → global source (rank=2)
//	builtin embed.FS        → builtin source (rank=1, ships git-status)
func writeProjectGlobalSources(t *testing.T, root string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(root, ".rtk", "filters.json"), []byte(`[
		{"id": "a-proj", "command": "ptest alpha", "priority": 90, "head": 1, "tail": 1},
		{"id": "git-status-proj", "command": "git status", "priority": 80, "head": 3, "tail": 1},
		{"id": "tie-a", "command": "ptest a", "priority": 50, "head": 1, "tail": 1},
		{"id": "tie-b", "command": "ptest b", "priority": 50, "head": 1, "tail": 1},
		{"id": "z-proj", "command": "ptest zed", "priority": 50, "head": 1, "tail": 1}
	]`))
	mustWriteFile(t, filepath.Join(root, "rtk", "filters.json"), []byte(`[
		{"id": "git-status-glob", "command": "git status", "priority": 70, "head": 6, "tail": 2}
	]`))
}

// TestLoaderPriority verifies three-tier ordering: Match on "git status"
// returns the project-level filter (rank=3) over the global (rank=2) and
// builtin (rank=1) ones, and cachedFilters is sorted by
// sourceRank desc → formatRank desc → priority desc → id asc.
// TDD red phase: Load/appDir/cachedFilters do not exist yet (compile error).
func TestLoaderPriority(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".rtk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "rtk"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectGlobalSources(t, root)

	cfg := &Config{Enabled: true, TrustProjectFilters: true}
	loader := NewFilterLoader(cfg)
	if err := loader.Load(root); err != nil {
		t.Fatalf("Load(%q) failed: %v", root, err)
	}

	// (a) project wins over global and builtin.
	matched := loader.Match("shell", "git status")
	if matched == nil {
		t.Fatal("Match returned nil for git status")
	}
	if matched.ID != "git-status-proj" {
		t.Errorf("Match returned %q, want project filter git-status-proj (rank=3)", matched.ID)
	}

	// (d) load ordering: sourceRank desc, formatRank desc, priority desc, id asc.
	ids := filterIDs(t, loader)
	idx := map[string]int{}
	for i, id := range ids {
		idx[id] = i
	}
	// project tier before global tier before builtin tier
	if !(idx["git-status-proj"] < idx["git-status-glob"] && idx["git-status-glob"] < idx["git-status"]) {
		t.Errorf("sourceRank ordering violated, ids=%v", ids)
	}
	// within a tier: priority desc, id asc for ties
	if !(idx["a-proj"] < idx["git-status-proj"] && idx["git-status-proj"] < idx["z-proj"]) {
		t.Errorf("priority desc ordering violated within project tier, ids=%v", ids)
	}
	if idx["tie-a"] >= idx["tie-b"] {
		t.Errorf("id asc tie-break violated (tie-a should precede tie-b), ids=%v", ids)
	}
}

// TestLoaderEnabledDisabled verifies whitelist/blacklist semantics:
// EnabledFilters=[] only loads the listed ids; DisabledFilters removes listed
// ids from the enabled set. TDD red phase: the Config fields and Load do not
// exist yet (compile error).
func TestLoaderEnabledDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".rtk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "rtk"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectGlobalSources(t, root)

	cfg := &Config{
		Enabled:           true,
		TrustProjectFilters: true,
		EnabledFilters:    []string{"git-status-proj", "git-status"},
		DisabledFilters:   []string{"git-status"},
	}
	loader := NewFilterLoader(cfg)
	if err := loader.Load(root); err != nil {
		t.Fatalf("Load(%q) failed: %v", root, err)
	}

	// (b) EnabledFilters whitelists: only git-status-proj + git-status are
	// candidates; everything else in the fixture is dropped.
	ids := filterIDs(t, loader)
	byID := map[string]bool{}
	for _, id := range ids {
		byID[id] = true
	}
	if byID["a-proj"] || byID["z-proj"] || byID["tie-a"] || byID["tie-b"] {
		t.Errorf("non-whitelisted project filters loaded: ids=%v", ids)
	}
	if byID["git-status-glob"] {
		t.Errorf("non-whitelisted global filter loaded: ids=%v", ids)
	}

	// (c) DisabledFilters removes git-status from the enabled set.
	if byID["git-status"] {
		t.Errorf("git-status should be removed by DisabledFilters, ids=%v", ids)
	}
	if !byID["git-status-proj"] {
		t.Errorf("git-status-proj should survive the whitelist, ids=%v", ids)
	}

	matched := loader.Match("shell", "git status")
	if matched == nil || matched.ID != "git-status-proj" {
		t.Errorf("Match after filtering = %v, want git-status-proj", matched)
	}
}

// ============================================================================
// 1.3 V-plugins-3: trust.json SHA256 4 scenarios + env var bypass
// ============================================================================

// TestTrustJSON4Scenarios verifies the project-source trust model:
//
//	(a) matching filtersSha256          → loaded
//	(b) wrong filtersSha256             → skipped + warn "SHA256 mismatch"
//	(c) missing trust.json              → skipped + warn "untrusted"
//	(d) legacy trustedFiltersSha256     → loaded (compat field)
//
// TDD red phase: Load/Diagnostics/FilterLoadDiagnostic do not exist yet
// (compile error).
func TestTrustJSON4Scenarios(t *testing.T) {
	projectFilters := []byte(`[
		{"id": "git-status-proj", "command": "git status", "priority": 80, "head": 3, "tail": 1}
	]`)
	hash := filtersSHA256Hex(projectFilters)

	mkFixture := func(t *testing.T, trust []byte, writeTrust bool) string {
		t.Helper()
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".rtk"), 0o755); err != nil {
			t.Fatal(err)
		}
		mustWriteFile(t, filepath.Join(root, ".rtk", "filters.json"), projectFilters)
		if writeTrust {
			mustWriteFile(t, filepath.Join(root, ".rtk", "trust.json"), trust)
		}
		return root
	}

	t.Run("a_matching_sha256_trusted", func(t *testing.T) {
		root := mkFixture(t, []byte(`{"filtersSha256": "`+hash+`"}`), true)
		loader := NewFilterLoader(&Config{Enabled: true})
		_ = loader.Load(root)
		matched := loader.Match("shell", "git status")
		if matched == nil || matched.ID != "git-status-proj" {
			t.Errorf("project filters should load when filtersSha256 matches, got %v", matched)
		}
	})

	t.Run("b_wrong_hash_skipped", func(t *testing.T) {
		root := mkFixture(t, []byte(`{"filtersSha256": "wrong_hash"}`), true)
		loader := NewFilterLoader(&Config{Enabled: true})
		_ = loader.Load(root)
		matched := loader.Match("shell", "git status")
		if matched != nil && matched.ID == "git-status-proj" {
			t.Error("project filters must be skipped on SHA256 mismatch")
		}
		// builtin fallback still matches.
		if matched == nil || matched.Name != "git-status" {
			t.Errorf("builtin git-status should still match, got %v", matched)
		}
		foundWarn := false
		for _, d := range loader.Diagnostics() {
			if d.Source == "project" && d.Level == "warning" && strings.Contains(d.Message, "SHA256 mismatch") {
				foundWarn = true
			}
		}
		if !foundWarn {
			t.Errorf("expected a warning diagnostic containing %q, got %+v", "SHA256 mismatch", loader.Diagnostics())
		}
	})

	t.Run("c_missing_trust_skipped", func(t *testing.T) {
		root := mkFixture(t, nil, false)
		loader := NewFilterLoader(&Config{Enabled: true})
		_ = loader.Load(root)
		matched := loader.Match("shell", "git status")
		if matched != nil && matched.ID == "git-status-proj" {
			t.Error("untrusted project filters must be skipped")
		}
		if matched == nil || matched.Name != "git-status" {
			t.Errorf("builtin git-status should still match, got %v", matched)
		}
		foundWarn := false
		for _, d := range loader.Diagnostics() {
			if d.Source == "project" && d.Level == "warning" && strings.Contains(d.Message, "untrusted") {
				foundWarn = true
			}
		}
		if !foundWarn {
			t.Errorf("expected a warning diagnostic containing %q, got %+v", "untrusted", loader.Diagnostics())
		}
	})

	t.Run("d_legacy_trusted_fields_sha256_trusted", func(t *testing.T) {
		root := mkFixture(t, []byte(`{"trustedFiltersSha256": "`+hash+`"}`), true)
		loader := NewFilterLoader(&Config{Enabled: true})
		_ = loader.Load(root)
		matched := loader.Match("shell", "git status")
		if matched == nil || matched.ID != "git-status-proj" {
			t.Errorf("legacy trustedFiltersSha256 field should still trust the project, got %v", matched)
		}
	})
}

// TestEnvVarBypass verifies that either trust env var set to "1" bypasses the
// trust check entirely: an untrusted project source loads, and an info
// diagnostic records the bypass.
// TDD red phase: Load/Diagnostics/FilterLoadDiagnostic do not exist yet
// (compile error).
func TestEnvVarBypass(t *testing.T) {
	projectFilters := []byte(`[
		{"id": "git-status-proj", "command": "git status", "priority": 80, "head": 3, "tail": 1}
	]`)
	runBypass := func(t *testing.T) {
		t.Helper()
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".rtk"), 0o755); err != nil {
			t.Fatal(err)
		}
		// No trust.json → normally skipped (untrusted).
		mustWriteFile(t, filepath.Join(root, ".rtk", "filters.json"), projectFilters)

		loader := NewFilterLoader(&Config{Enabled: true})
		_ = loader.Load(root)
		matched := loader.Match("shell", "git status")
		if matched == nil || matched.ID != "git-status-proj" {
			t.Errorf("project filters should load when the trust env var is set, got %v", matched)
		}
		foundInfo := false
		for _, d := range loader.Diagnostics() {
			if d.Level == "info" && strings.Contains(d.Message, "trust bypassed by env var") {
				foundInfo = true
			}
		}
		if !foundInfo {
			t.Errorf("expected an info diagnostic containing %q, got %+v", "trust bypassed by env var", loader.Diagnostics())
		}
	}

	t.Run("omniroute_env_var", func(t *testing.T) {
		t.Setenv("OMNIROUTE_RTK_TRUST_PROJECT_FILTERS", "1")
		runBypass(t)
	})

	t.Run("bifrost_env_var_alias", func(t *testing.T) {
		t.Setenv("BIFROST_RTK_TRUST_PROJECT_FILTERS", "1")
		runBypass(t)
	})
}

// ============================================================================
// 1.4 Fixing the implicit bug: Plugin holds its own FilterLoader
// ============================================================================

// TestPluginInitHoldsLoader verifies that Init wires a per-plugin FilterLoader
// (built from appDir) into the plugin, so per-plugin config fields really take
// effect instead of a package-level singleton/compression.go defaultConfig.
// TDD red phase: the new Init signature (with appDir) and Plugin.loader do not
// exist yet (compile error).
func TestPluginInitHoldsLoader(t *testing.T) {
	appDir := t.TempDir()

	p, err := Init(nil, DefaultConfig(), nil, appDir)
	if err != nil {
		t.Fatalf("Init with valid config should succeed, got: %v", err)
	}
	if p == nil {
		t.Fatal("Init returned nil plugin")
	}
	if p.loader == nil {
		t.Fatal("expected plugin.loader to be non-nil after Init")
	}
	if p.loader.appDir != appDir {
		t.Errorf("plugin.loader.appDir = %q, want %q", p.loader.appDir, appDir)
	}
	builtin := p.loader.Match("shell", "git status")
	if builtin == nil {
		t.Fatal("plugin loader Match returned nil for git status")
	}
	if builtin.Name != "git-status" {
		t.Errorf("plugin loader builtin match = %q, want %q", builtin.Name, "git-status")
	}

	// A second Init with a different config must take effect on its own loader
	// (the implicit bug: the old global singleton ignored per-plugin config).
	p2, err := Init(nil, &Config{Enabled: true, DisabledFilters: []string{"git-status"}}, nil, appDir)
	if err != nil {
		t.Fatalf("Init with DisabledFilters config should succeed, got: %v", err)
	}
	if p2.loader == nil {
		t.Fatal("expected second plugin.loader to be non-nil")
	}
	matched := p2.loader.Match("shell", "git status")
	if matched != nil && matched.Name == "git-status" {
		t.Error("second plugin loader should honor DisabledFilters=[git-status]")
	}
}