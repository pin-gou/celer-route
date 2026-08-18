package rtk

import (
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