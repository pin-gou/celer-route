package rtk

import (
	"reflect"
	"strings"
	"testing"
)

// sliceContains reports whether the slice contains the given string.
// Used for membership assertions on pattern slices whose order may be
// non-deterministic (map iteration).
func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// TestCommandToId verifies the slug conversion from OmniRoute learn.ts
// commandToId: trim → toLower → replace non-[a-z0-9] runs with "-" → strip
// leading/trailing "-".
//
// TDD red phase: CommandToId does not exist yet (compile error expected).
func TestCommandToId(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Basic cases
		{name: "two_words", input: "npm install", want: "npm-install"},
		{name: "extra_whitespace", input: "  pip   install   ", want: "pip-install"},
		{name: "single_word", input: "npm", want: "npm"},
		{name: "empty_string", input: "", want: ""},

		// Case folding
		{name: "mixed_case", input: "NPM Install", want: "npm-install"},

		// Multi-word replacement
		{name: "long_command", input: "git push --set-upstream origin main", want: "git-push-set-upstream-origin-main"},

		// Leading/trailing dash stripping
		{name: "leading_trailing_dashes", input: "  --force  ", want: "force"},
		{name: "underscore_to_dash", input: "npm_install", want: "npm-install"},

		// Contiguous dashes collapsed
		{name: "single_dash_preserved", input: "a-b", want: "a-b"},
		{name: "double_dash_collapsed", input: "a--b", want: "a-b"},
		{name: "surrounding_dashes_stripped", input: "-npm-", want: "npm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CommandToId(tt.input)
			if got != tt.want {
				t.Errorf("CommandToId(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSuggestFilterEmptySkeleton verifies that SuggestFilter with zero samples
// returns a valid skeleton filter (no rules derived).
//
// TDD red phase: SuggestFilter, SuggestedFilter, and related types do not
// exist yet (compile error expected).
func TestSuggestFilterEmptySkeleton(t *testing.T) {
	sf := SuggestFilter("npm install", nil)

	// Identity
	if sf.ID != "suggested-npm-install" {
		t.Errorf("ID = %q, want %q", sf.ID, "suggested-npm-install")
	}
	if sf.Label != "npm install" {
		t.Errorf("Label = %q, want %q", sf.Label, "npm install")
	}
	if !strings.Contains(sf.Description, "0 samples") {
		t.Errorf("Description should mention '0 samples', got %q", sf.Description)
	}

	// Category and priority
	if sf.Category != "generic" {
		t.Errorf("Category = %q, want %q", sf.Category, "generic")
	}
	if sf.Priority != 50 {
		t.Errorf("Priority = %d, want 50", sf.Priority)
	}

	// Match block
	if !reflect.DeepEqual(sf.Match.OutputTypes, []string{}) {
		t.Errorf("Match.OutputTypes = %v, want empty", sf.Match.OutputTypes)
	}
	wantCommands := []string{"^npm\\s+install\\b"}
	if !reflect.DeepEqual(sf.Match.Commands, wantCommands) {
		t.Errorf("Match.Commands = %v, want %v", sf.Match.Commands, wantCommands)
	}
	if len(sf.Match.Patterns) != 0 {
		t.Errorf("Match.Patterns = %v, want empty", sf.Match.Patterns)
	}

	// Rules block
	if !sf.Rules.StripAnsi {
		t.Error("Rules.StripAnsi should be true")
	}
	if len(sf.Rules.DropPatterns) != 0 {
		t.Errorf("Rules.DropPatterns = %v, want empty", sf.Rules.DropPatterns)
	}
	if len(sf.Rules.CollapsePatterns) != 0 {
		t.Errorf("Rules.CollapsePatterns = %v, want empty", sf.Rules.CollapsePatterns)
	}
	if len(sf.Rules.IncludePatterns) != 0 {
		t.Errorf("Rules.IncludePatterns = %v, want empty", sf.Rules.IncludePatterns)
	}
	if !sf.Rules.Deduplicate {
		t.Error("Rules.Deduplicate should be true")
	}
	if sf.Rules.MaxLines != 200 {
		t.Errorf("Rules.MaxLines = %d, want 200", sf.Rules.MaxLines)
	}
	if sf.Rules.HeadLines != 30 {
		t.Errorf("Rules.HeadLines = %d, want 30", sf.Rules.HeadLines)
	}
	if sf.Rules.TailLines != 40 {
		t.Errorf("Rules.TailLines = %d, want 40", sf.Rules.TailLines)
	}
	if sf.Rules.OnEmpty != "npm-install: ok" {
		t.Errorf("Rules.OnEmpty = %q, want %q", sf.Rules.OnEmpty, "npm-install: ok")
	}

	// Preserve block
	if len(sf.Preserve.ErrorPatterns) != 0 {
		t.Errorf("Preserve.ErrorPatterns = %v, want empty", sf.Preserve.ErrorPatterns)
	}
	if len(sf.Preserve.SummaryPatterns) != 0 {
		t.Errorf("Preserve.SummaryPatterns = %v, want empty", sf.Preserve.SummaryPatterns)
	}

	// Meta
	if sf.Meta.LearnedFromSamples != 0 {
		t.Errorf("Meta.LearnedFromSamples = %d, want 0", sf.Meta.LearnedFromSamples)
	}
	if sf.Meta.DropThreshold != 50 {
		t.Errorf("Meta.DropThreshold = %d, want 50", sf.Meta.DropThreshold)
	}

	// Tests (empty for phase 5)
	if len(sf.Tests) != 0 {
		t.Errorf("Tests = %v, want empty", sf.Tests)
	}
}

// TestSuggestFilterStructure verifies that SuggestFilter with 3 samples
// produces a valid filter structure: id derived from CommandToId, match.commands
// contains the escaped command pattern, dropPatterns derived from recurring noise,
// preserve blocks present, and meta reflects the learning run.
//
// V-plugins-3.
func TestSuggestFilterStructure(t *testing.T) {
	samples := []CommandSample{
		{
			Command: "npm install",
			Output:  "Compiling src 1 of 3\nnpm ERR! code ENOENT\nadded 42 packages\n",
		},
		{
			Command: "npm install",
			Output:  "Compiling src 4 of 7\nnpm ERR! code EPERM\n",
		},
		{
			Command: "npm install",
			Output:  "Compiling src 10 of 20\nnpm warn EBADENGINE 6\n",
		},
	}
	sf := SuggestFilter("npm install", samples)

	// Identity
	if sf.ID != "suggested-npm-install" {
		t.Errorf("ID = %q, want %q", sf.ID, "suggested-npm-install")
	}
	if sf.Priority != 50 {
		t.Errorf("Priority = %d, want 50", sf.Priority)
	}
	if sf.Category != "generic" {
		t.Errorf("Category = %q, want %q", sf.Category, "generic")
	}

	// Match block
	wantCommands := []string{"^npm\\s+install\\b"}
	if !reflect.DeepEqual(sf.Match.Commands, wantCommands) {
		t.Errorf("Match.Commands = %v, want %v", sf.Match.Commands, wantCommands)
	}

	// dropPatterns: "Compiling src N of N" recurs in all 3 samples → drop;
	// "npm ERR! code <CODE>" recurred in 2 samples but conflict guard
	// excludes it because it matches preserved error lines.
	wantDrop := []string{"^Compiling src [\\S]+ of [\\S]+"}
	if !reflect.DeepEqual(sf.Rules.DropPatterns, wantDrop) {
		t.Errorf("Rules.DropPatterns = %v, want %v", sf.Rules.DropPatterns, wantDrop)
	}

	// includePatterns: errorPatterns + summaryPatterns
	// (deterministic: 1 errorPattern + 1 summaryPattern)
	wantInclude := []string{
		"npm [A-Z][A-Z0-9]+! code [A-Z][A-Z0-9]+",
		"added [\\S]+ packages",
	}
	if !reflect.DeepEqual(sf.Rules.IncludePatterns, wantInclude) {
		t.Errorf("Rules.IncludePatterns = %v, want %v", sf.Rules.IncludePatterns, wantInclude)
	}

	// Preserve block
	wantErr := []string{"npm [A-Z][A-Z0-9]+! code [A-Z][A-Z0-9]+"}
	if !reflect.DeepEqual(sf.Preserve.ErrorPatterns, wantErr) {
		t.Errorf("Preserve.ErrorPatterns = %v, want %v", sf.Preserve.ErrorPatterns, wantErr)
	}
	wantSum := []string{"added [\\S]+ packages"}
	if !reflect.DeepEqual(sf.Preserve.SummaryPatterns, wantSum) {
		t.Errorf("Preserve.SummaryPatterns = %v, want %v", sf.Preserve.SummaryPatterns, wantSum)
	}

	// Rules defaults
	if !sf.Rules.StripAnsi {
		t.Error("Rules.StripAnsi should be true")
	}
	if !sf.Rules.Deduplicate {
		t.Error("Rules.Deduplicate should be true")
	}
	if sf.Rules.MaxLines != 200 {
		t.Errorf("Rules.MaxLines = %d, want 200", sf.Rules.MaxLines)
	}
	if sf.Rules.HeadLines != 30 {
		t.Errorf("Rules.HeadLines = %d, want 30", sf.Rules.HeadLines)
	}
	if sf.Rules.TailLines != 40 {
		t.Errorf("Rules.TailLines = %d, want 40", sf.Rules.TailLines)
	}
	if sf.Rules.OnEmpty != "npm-install: ok" {
		t.Errorf("Rules.OnEmpty = %q, want %q", sf.Rules.OnEmpty, "npm-install: ok")
	}
	if len(sf.Rules.CollapsePatterns) != 0 {
		t.Errorf("Rules.CollapsePatterns = %v, want empty", sf.Rules.CollapsePatterns)
	}

	// Meta
	if sf.Meta.LearnedFromSamples != 3 {
		t.Errorf("Meta.LearnedFromSamples = %d, want 3", sf.Meta.LearnedFromSamples)
	}
	if sf.Meta.DropThreshold != 50 {
		t.Errorf("Meta.DropThreshold = %d, want 50", sf.Meta.DropThreshold)
	}

	// Tests (phase 5: empty)
	if len(sf.Tests) != 0 {
		t.Errorf("Tests = %v, want empty", sf.Tests)
	}
}

// TestSuggestFilterError verifies that ERROR_PATTERN (ERR! / error[:/] / failed
// / fatal / panic / critical / exception) correctly identifies error lines and
// populates preserve.errorPatterns.
//
// V-plugins-4 (error recognition).
func TestSuggestFilterError(t *testing.T) {
	samples := []CommandSample{
		{
			Command: "make",
			Output:  "npm ERR! code ENOENT\nfatal: out of memory\npanic: runtime error\nBuild completed\n",
		},
	}
	sf := SuggestFilter("make", samples)

	// Three distinct error patterns expected
	if len(sf.Preserve.ErrorPatterns) != 3 {
		t.Fatalf("Preserve.ErrorPatterns len = %d, want 3; got %v", len(sf.Preserve.ErrorPatterns), sf.Preserve.ErrorPatterns)
	}

	expected := []string{
		"npm [A-Z][A-Z0-9]+! code [A-Z][A-Z0-9]+", // "npm ERR! code ENOENT"
		"fatal: out of memory",                       // "fatal: out of memory"
		"panic: runtime error",                       // "panic: runtime error"
	}
	for _, pat := range expected {
		if !sliceContains(sf.Preserve.ErrorPatterns, pat) {
			t.Errorf("Preserve.ErrorPatterns should contain %q, got %v", pat, sf.Preserve.ErrorPatterns)
		}
	}

	// "Build completed" is a summary, not an error
	if len(sf.Preserve.SummaryPatterns) != 1 {
		t.Fatalf("Preserve.SummaryPatterns len = %d, want 1; got %v", len(sf.Preserve.SummaryPatterns), sf.Preserve.SummaryPatterns)
	}
	if !sliceContains(sf.Preserve.SummaryPatterns, "Build completed") {
		t.Errorf("Preserve.SummaryPatterns should contain %q, got %v", "Build completed", sf.Preserve.SummaryPatterns)
	}
}

// TestSuggestFilterSummary verifies that SUMMARY_PATTERN (success / done /
// complete / built / added / installed / finished / passed) correctly identifies
// summary lines and populates preserve.summaryPatterns.
//
// V-plugins-4 (summary recognition).
func TestSuggestFilterSummary(t *testing.T) {
	samples := []CommandSample{
		{
			Command: "npm install",
			Output:  "Build completed\nTests passed: 42\n1 package installed\n",
		},
	}
	sf := SuggestFilter("npm install", samples)

	if len(sf.Preserve.SummaryPatterns) != 3 {
		t.Fatalf("Preserve.SummaryPatterns len = %d, want 3; got %v", len(sf.Preserve.SummaryPatterns), sf.Preserve.SummaryPatterns)
	}

	expected := []string{
		"Build completed",           // "Build completed" (complete+d)
		"Tests passed: [\\S]+",      // "Tests passed: 42"
		"[\\S]+ package installed",  // "1 package installed"
	}
	for _, pat := range expected {
		if !sliceContains(sf.Preserve.SummaryPatterns, pat) {
			t.Errorf("Preserve.SummaryPatterns should contain %q, got %v", pat, sf.Preserve.SummaryPatterns)
		}
	}

	// No error lines in this fixture
	if len(sf.Preserve.ErrorPatterns) != 0 {
		t.Errorf("Preserve.ErrorPatterns should be empty, got %v", sf.Preserve.ErrorPatterns)
	}
}

// TestSuggestFilterConflictGuard verifies that a drop candidate whose pattern
// matches a preserved (error/summary) line is excluded from dropPatterns.
//
// The fixture has two samples, each with an error line ("npm ERR! code EPERM/ENOENT")
// and a plain noise line ("Fetching data N"). The noise candidate for error lines
// ("npm <CODE>! code <CODE>") qualifies (hits 2 ≥ threshold 2) but is excluded
// by the conflict guard. The clean noise candidate is retained.
//
// V-plugins-4 (conflict guard).
func TestSuggestFilterConflictGuard(t *testing.T) {
	samples := []CommandSample{
		{
			Command: "npm install",
			Output:  "npm ERR! code EPERM\nFetching data 1\n",
		},
		{
			Command: "npm install",
			Output:  "npm ERR! code ENOENT\nFetching data 2\n",
		},
	}
	sf := SuggestFilter("npm install", samples)

	// The error-code noise candidate ("npm <CODE>! code <CODE>") should be
	// excluded by conflict guard — only "Fetching data <N>" remains.
	wantDrop := []string{"^Fetching data [\\S]+"}
	if !reflect.DeepEqual(sf.Rules.DropPatterns, wantDrop) {
		t.Errorf("Rules.DropPatterns = %v, want %v", sf.Rules.DropPatterns, wantDrop)
	}

	// Explicitly confirm the conflicting pattern is NOT present
	conflictPattern := "npm [A-Z][A-Z0-9]+! code [A-Z][A-Z0-9]+"
	if sliceContains(sf.Rules.DropPatterns, conflictPattern) {
		t.Errorf("dropPatterns should NOT contain %q (conflict guard failed), got %v", conflictPattern, sf.Rules.DropPatterns)
	}
}

// TestSuggestFilterThreshold verifies that a normalised line appearing in only
// 1 of 3 samples is excluded from dropPatterns by the DROP_THRESHOLD_RATIO (0.5).
// A recurring line in all 3 samples is retained.
func TestSuggestFilterThreshold(t *testing.T) {
	samples := []CommandSample{
		{
			Command: "npm run build",
			Output:  "Compiling src 1 of 3\nunique line 42\n",
		},
		{
			Command: "npm run build",
			Output:  "Compiling src 4 of 7\n",
		},
		{
			Command: "npm run build",
			Output:  "Compiling src 10 of 20\n",
		},
	}
	sf := SuggestFilter("npm run build", samples)

	// "Compiling src N of N" appears in all 3 samples → retained
	wantDrop := []string{"^Compiling src [\\S]+ of [\\S]+"}
	if !reflect.DeepEqual(sf.Rules.DropPatterns, wantDrop) {
		t.Errorf("Rules.DropPatterns = %v, want %v", sf.Rules.DropPatterns, wantDrop)
	}

	// "unique line <N>" appears only in 1 sample → excluded by threshold
	excludedPattern := "^unique line [\\S]+"
	if sliceContains(sf.Rules.DropPatterns, excludedPattern) {
		t.Errorf("dropPatterns should NOT contain %q (threshold filter failed), got %v", excludedPattern, sf.Rules.DropPatterns)
	}

	if sf.Meta.LearnedFromSamples != 3 {
		t.Errorf("Meta.LearnedFromSamples = %d, want 3", sf.Meta.LearnedFromSamples)
	}
}