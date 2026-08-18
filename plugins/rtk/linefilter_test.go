package rtk

import (
	"testing"
)

// TestLineFilterStrip verifies the strip rule removes matching lines.
func TestLineFilterStrip(t *testing.T) {
	input := `DEBUG Starting connection pool
INFO Server started
DEBUG Connecting to database
INFO Request received
TRACE Entering handler
INFO Response sent
`

	filter := &Filter{
		Name: "test-strip",
		Rules: []LineRule{
			{Type: "strip", Pattern: `^DEBUG`},
			{Type: "strip", Pattern: `^TRACE`},
		},
	}

	result := applyLineFilter(input, filter)
	expected := "INFO Server started\nINFO Request received\nINFO Response sent\n"
	if result != expected {
		t.Errorf("applyLineFilter strip:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterKeep verifies the keep rule only keeps matching lines.
func TestLineFilterKeep(t *testing.T) {
	input := `INFO Server started
WARN Disk space low
ERROR Connection refused
INFO Request received
DEBUG Writing to disk
`

	filter := &Filter{
		Name: "test-keep",
		Rules: []LineRule{
			{Type: "keep", Pattern: `^(ERROR|WARN)`},
		},
	}

	result := applyLineFilter(input, filter)
	expected := "WARN Disk space low\nERROR Connection refused\n"
	if result != expected {
		t.Errorf("applyLineFilter keep:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterCollapse verifies the collapse rule reduces blank lines.
func TestLineFilterCollapse(t *testing.T) {
	input := `line1


line2


line3
`

	filter := &Filter{
		Name: "test-collapse",
		Rules: []LineRule{
			{Type: "collapse", Pattern: `^\s*$`},
		},
	}

	result := applyLineFilter(input, filter)
	// The collapse should reduce consecutive blank lines to a single blank line
	expected := "line1\n\nline2\n\nline3\n"
	if result != expected {
		t.Errorf("applyLineFilter collapse:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterReplace verifies the replace rule substitutes text.
func TestLineFilterReplace(t *testing.T) {
	input := `/home/user/project/src/main.go:42: error: undefined variable
/home/user/project/src/utils.go:10: warning: unused import
`

	filter := &Filter{
		Name: "test-replace",
		Rules: []LineRule{
			{
				Type:        "replace",
				Pattern:     `/home/user/project/`,
				Replacement: "~/project/",
			},
		},
	}

	result := applyLineFilter(input, filter)
	expected := "~/project/src/main.go:42: error: undefined variable\n~/project/src/utils.go:10: warning: unused import\n"
	if result != expected {
		t.Errorf("applyLineFilter replace:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterDedup verifies consecutive duplicate lines are merged.
func TestLineFilterDedup(t *testing.T) {
	input := `Compiling...
Compiling...
Compiling...
Compiling...
Compiling...
Done!
`

	result := applyDedup(input, 3)
	expected := "Compiling...\nDone!\n"
	if result != expected {
		t.Errorf("applyDedup (threshold=3):\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterDedupBelowThreshold verifies that duplicates below threshold
// are NOT deduplicated.
func TestLineFilterDedupBelowThreshold(t *testing.T) {
	input := `Compiling...
Compiling...
Done!
`
	result := applyDedup(input, 3)
	// With threshold=3 and only 2 identical lines, nothing should be deduplicated
	expected := input
	if result != expected {
		t.Errorf("applyDedup below threshold:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterHead verifies truncation to first N lines.
func TestLineFilterHead(t *testing.T) {
	input := `line1
line2
line3
line4
line5
`

	filter := &Filter{
		Name: "test-head",
		Head: 3,
	}

	result := applySmartTruncate(input, filter)
	expected := "line1\nline2\nline3\n"
	if result != expected {
		t.Errorf("applySmartTruncate head=3:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterTail verifies truncation to last N lines.
func TestLineFilterTail(t *testing.T) {
	input := `line1
line2
line3
line4
line5
`

	filter := &Filter{
		Name: "test-tail",
		Tail: 2,
	}

	result := applySmartTruncate(input, filter)
	expected := "line4\nline5\n"
	if result != expected {
		t.Errorf("applySmartTruncate tail=2:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterHeadAndTail verifies both head and tail truncation.
func TestLineFilterHeadAndTail(t *testing.T) {
	input := `line1
line2
line3
line4
line5
line6
line7
line8
line9
line10
`

	filter := &Filter{
		Name: "test-head-tail",
		Head: 3,
		Tail: 2,
	}

	result := applySmartTruncate(input, filter)
	// Should keep first 3 and last 2 lines
	if !contains(result, "line1") || !contains(result, "line2") || !contains(result, "line3") {
		t.Errorf("head lines missing from result:\n%q", result)
	}
	if !contains(result, "line9") || !contains(result, "line10") {
		t.Errorf("tail lines missing from result:\n%q", result)
	}
	// Middle lines should be truncated
	if contains(result, "line5") || contains(result, "line6") {
		t.Errorf("middle lines should be truncated, but found in:\n%q", result)
	}
}

// TestLineFilterEmptyInput verifies empty input handling.
func TestLineFilterEmptyInput(t *testing.T) {
	filter := &Filter{
		Name: "test-empty",
		Rules: []LineRule{
			{Type: "strip", Pattern: `^DEBUG`},
		},
	}

	result := applyLineFilter("", filter)
	if result != "" {
		t.Errorf("applyLineFilter with empty input should return empty, got %q", result)
	}
}

// TestLineFilterAllLinesStripped verifies that stripping all lines returns empty.
func TestLineFilterAllLinesStripped(t *testing.T) {
	input := `DEBUG line1
DEBUG line2
DEBUG line3
`
	filter := &Filter{
		Name: "test-strip-all",
		Rules: []LineRule{
			{Type: "strip", Pattern: `^DEBUG`},
		},
	}

	result := applyLineFilter(input, filter)
	if result != "" {
		t.Errorf("expected empty result after stripping all lines, got %q", result)
	}
}

// TestLineFilterMultipleRulesOrder verifies that rules are applied in the
// correct order: strip → keep → collapse → replace.
func TestLineFilterMultipleRulesOrder(t *testing.T) {
	input := `DEBUG Connecting
ERROR Failed: old_module
DEBUG Retrying
ERROR Failed: old_module
`

	filter := &Filter{
		Name: "test-multi-rule",
		Rules: []LineRule{
			{Type: "strip", Pattern: `^DEBUG`},
			{
				Type:        "replace",
				Pattern:     `old_module`,
				Replacement: "new_module",
			},
		},
	}

	result := applyLineFilter(input, filter)
	// After stripping DEBUG lines, the ERROR lines should have replacement applied
	expected := "ERROR Failed: new_module\nERROR Failed: new_module\n"
	if result != expected {
		t.Errorf("applyLineFilter multi-rule:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterPriorityPatterns verifies that priority patterns survive
// head/tail truncation.
func TestLineFilterPriorityPatterns(t *testing.T) {
	input := `line1
line2
line3
line4
ERROR: critical failure
line5
line6
line7
line8
line9
line10
`

	filter := &Filter{
		Name: "test-priority",
		Head: 2,
		Tail: 2,
		PriorityPatterns: []string{
			`ERROR: critical failure`,
		},
	}

	result := applySmartTruncate(input, filter)
	// The priority pattern should survive even though it's in the middle
	if !contains(result, "ERROR: critical failure") {
		t.Errorf("priority pattern should survive truncation, but was not found in:\n%q", result)
	}
}

// TestLineFilterMaxLines enforces maximum line count.
func TestLineFilterMaxLines(t *testing.T) {
	input := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n"

	filter := &Filter{
		Name:     "test-maxlines",
		MaxLines: 5,
	}

	result := applySmartTruncate(input, filter)
	lineCount := countLines(result)
	if lineCount > 5 {
		t.Errorf("expected at most 5 lines, got %d", lineCount)
	}
}

// TestLineFilterKeepWithStrip verifies keep + strip interaction.
func TestLineFilterKeepWithStrip(t *testing.T) {
	input := `INFO normal log
ERROR system failure
DEBUG debug log
WARN disk space low
ERROR another failure
`

	filter := &Filter{
		Name: "test-keep-strip",
		Rules: []LineRule{
			{Type: "keep", Pattern: `^(ERROR|WARN)`},
			{Type: "strip", Pattern: `^ERROR normal`},
		},
	}

	result := applyLineFilter(input, filter)
	// Keep: ERROR, WARN lines; strip only removes ERROR lines matching "normal"
	// Since keep already filtered out non-ERROR/WARN lines, the strip has no effect on those
	// But all ERROR lines are kept (none match the strip pattern)
	expected := "ERROR system failure\nWARN disk space low\nERROR another failure\n"
	if result != expected {
		t.Errorf("applyLineFilter keep+strip:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterNilFilterSafety verifies nil filter safety.
func TestLineFilterNilFilterSafety(t *testing.T) {
	result := applyLineFilter("some text", nil)
	if result != "some text" {
		t.Errorf("applyLineFilter with nil filter should return original text, got %q", result)
	}
}

// TestLineFilterNilRules verifies nil rules safety.
func TestLineFilterNilRules(t *testing.T) {
	filter := &Filter{
		Name: "test-nil-rules",
	}

	result := applyLineFilter("some text", filter)
	if result != "some text" {
		t.Errorf("applyLineFilter with nil rules should return original text, got %q", result)
	}
}

// Helper: count lines in a string.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			count++
		}
	}
	return count
}