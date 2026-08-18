package rtk

import (
	"fmt"
	"strings"
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

// TestLineFilterDedup verifies consecutive duplicate lines are merged and
// markers are appended.
func TestLineFilterDedup(t *testing.T) {
	input := `Compiling...
Compiling...
Compiling...
Compiling...
Compiling...
Done!
`

	result, _ := applyDedup(input, 3)
	// With threshold=3, 5 consecutive "Compiling..." lines collapse to 1 line
	// plus markers: [line repeated 4x] + [rtk:dropped 4 repeated lines]
	if !strings.Contains(result, "[line repeated 4x]") {
		t.Errorf("applyDedup (threshold=3): missing [line repeated 4x] marker in:\n%q", result)
	}
	if !strings.Contains(result, "[rtk:dropped 4 repeated lines]") {
		t.Errorf("applyDedup (threshold=3): missing [rtk:dropped 4 repeated lines] marker in:\n%q", result)
	}
	if !strings.Contains(result, "Done!") {
		t.Errorf("applyDedup (threshold=3): missing Done! in:\n%q", result)
	}
}

// TestLineFilterDedupBelowThreshold verifies that duplicates below threshold
// are NOT deduplicated.
func TestLineFilterDedupBelowThreshold(t *testing.T) {
	input := `Compiling...
Compiling...
Done!
`
	result, _ := applyDedup(input, 3)
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

	result, _ := applySmartTruncate(input, filter)
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

	result, _ := applySmartTruncate(input, filter)
	expected := "line4\nline5\n"
	if result != expected {
		t.Errorf("applySmartTruncate tail=2:\ngot:\n%q\nwant:\n%q", result, expected)
	}
}

// TestLineFilterHeadAndTail verifies both head and tail truncation with
// truncation marker.
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

	result, _ := applySmartTruncate(input, filter)
	// Should keep first 3 and last 2 lines, with a truncation marker in between
	if !strings.Contains(result, "line1") || !strings.Contains(result, "line2") || !strings.Contains(result, "line3") {
		t.Errorf("head lines missing from result:\n%q", result)
	}
	if !strings.Contains(result, "line9") || !strings.Contains(result, "line10") {
		t.Errorf("tail lines missing from result:\n%q", result)
	}
	// Middle lines should be truncated
	if strings.Contains(result, "line5") || strings.Contains(result, "line6") {
		t.Errorf("middle lines should be truncated, but found in:\n%q", result)
	}
	// The truncation marker should be present
	if !strings.Contains(result, "[rtk:truncated 5 lines]") {
		t.Errorf("expected truncation marker [rtk:truncated 5 lines] in result:\n%q", result)
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

	result, _ := applySmartTruncate(input, filter)
	// The priority pattern should survive even though it's in the middle
	if !strings.Contains(result, "ERROR: critical failure") {
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

	result, _ := applySmartTruncate(input, filter)
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

// TestDocumentReadNotTruncated verifies that document-like reads (no error markers)
// are not truncated and retain the full text without truncation markers.
func TestDocumentReadNotTruncated(t *testing.T) {
	// Build a ~147 line document-like input that has no error markers.
	// Detection will fall back to {Type:"shell", Command:""} (generic fallback).
	// When isDocumentLikeRead is true, the pipeline skips truncation.
	var lines []string
	lines = append(lines, "package main")
	lines = append(lines, "")
	lines = append(lines, "import (")
	lines = append(lines, "	\"fmt\"")
	lines = append(lines, "	\"net/http\"")
	lines = append(lines, ")")
	lines = append(lines, "")
	lines = append(lines, "func main() {")
	lines = append(lines, "	http.HandleFunc(\"/\", handler)")
	lines = append(lines, "	fmt.Println(\"Starting server on :8080\")")
	lines = append(lines, "	log.Fatal(http.ListenAndServe(\":8080\", nil))")
	lines = append(lines, "}")
	lines = append(lines, "")
	for i := 0; i < 135; i++ {
		lines = append(lines, fmt.Sprintf("// line %d: some documentation content", i+1))
	}
	input := strings.Join(lines, "\n") + "\n"

	cfg := &Config{
		Enabled:           true,
		Intensity:         "standard",
		DedupThreshold:    3,
		MaxCharsPerResult: 50000,
	}

	result, _ := processRtkTextWithCommand(input, cfg, NewFilterLoader(cfg), "")
	// The result should contain the full text (no truncation markers)
	if strings.Contains(result, "[rtk:truncated") {
		t.Errorf("document-like read should not contain truncation marker, got:\n...%s...", result[len(result)-200:])
	}
	// The result should be at least as long as the input (no truncation)
	if len(result) < len(input)-100 {
		t.Errorf("document-like read should preserve full text, got len=%d, want >= %d", len(result), len(input)-100)
	}
}

// TestIsDocumentLikeReadWithErrorMarkers verifies that input containing
// generic error markers (Traceback) is NOT treated as document-like and
// goes through the full pipeline.
func TestIsDocumentLikeReadWithErrorMarkers(t *testing.T) {
	input := `Traceback (most recent call last):
  File "/usr/lib/python3.10/runpy.py", line 196, in _run_module_as_main
    return _run_code(code, main_globals, None,
  File "/usr/lib/python3.10/runpy.py", line 86, in _run_code
    exec(code, run_globals)
  File "/home/user/app/main.py", line 42, in <module>
    main()
  File "/home/user/app/main.py", line 38, in main
    result = process_data(data)
  File "/home/user/app/process.py", line 24, in process_data
    raise ValueError("invalid input")
ValueError: invalid input
`
	// hasGenericErrorMarkers should detect the Traceback line
	if !hasGenericErrorMarkers(input) {
		t.Errorf("hasGenericErrorMarkers should return true for input with Traceback")
	}

	cfg := DefaultConfig()
	result, _ := processRtkTextWithCommand(input, cfg, NewFilterLoader(cfg), "")
	// The result should go through the full pipeline (filter + smartTruncate + char limit)
	// and may be truncated, but should NOT be treated as document-like
	if !strings.Contains(result, "Traceback") {
		t.Errorf("result should contain Traceback, got:\n%q", result)
	}
}

// TestTruncateMarker verifies that applySmartTruncate inserts the
// [rtk:truncated N lines] marker between head and tail.
func TestTruncateMarker(t *testing.T) {
	input := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"

	filter := &Filter{
		Name: "test-truncate-marker",
		Head: 3,
		Tail: 2,
	}

	result, _ := applySmartTruncate(input, filter)
	// Should contain the head lines
	if !strings.Contains(result, "line1") || !strings.Contains(result, "line2") || !strings.Contains(result, "line3") {
		t.Errorf("head lines missing from result:\n%q", result)
	}
	// Should contain the tail lines
	if !strings.Contains(result, "line9") || !strings.Contains(result, "line10") {
		t.Errorf("tail lines missing from result:\n%q", result)
	}
	// Should contain the truncation marker with N = 10 - 3 - 2 = 5
	if !strings.Contains(result, "[rtk:truncated 5 lines]") {
		t.Errorf("expected [rtk:truncated 5 lines] marker in result:\n%q", result)
	}
}

// TestDedupMarker verifies that applyDedup appends [line repeated Nx] and
// [rtk:dropped N repeated lines] markers for consecutive duplicate lines.
func TestDedupMarker(t *testing.T) {
	input := "A\nA\nA\nA\nA\nB\n"

	result, _ := applyDedup(input, 3)
	// With threshold=3 and 5 consecutive identical lines, the run collapses to
	// 1 line + markers: [line repeated 4x] + [rtk:dropped 4 repeated lines]
	if !strings.Contains(result, "[line repeated 4x]") {
		t.Errorf("expected [line repeated 4x] marker in result:\n%q", result)
	}
	if !strings.Contains(result, "[rtk:dropped 4 repeated lines]") {
		t.Errorf("expected [rtk:dropped 4 repeated lines] marker in result:\n%q", result)
	}
	if !strings.Contains(result, "B") {
		t.Errorf("expected 'B' in result:\n%q", result)
	}
}

// TestCharTruncateMarker verifies that processRtkTextWithCommand appends
// [rtk:truncated by chars] when the output exceeds MaxCharsPerResult.
func TestCharTruncateMarker(t *testing.T) {
	// Build a long input that will exceed MaxCharsPerResult=100
	longLine := strings.Repeat("this is a very long line that will be truncated ", 10)
	input := longLine + "\n" + longLine + "\n" + longLine + "\n"

	cfg := &Config{
		Enabled:           true,
		Intensity:         "standard",
		MaxCharsPerResult: 100,
		DedupThreshold:    3,
	}

	result, _ := processRtkTextWithCommand(input, cfg, NewFilterLoader(cfg), "")
	// The result should contain the char truncation marker
	if !strings.Contains(result, "[rtk:truncated by chars]") {
		t.Errorf("expected [rtk:truncated by chars] marker in result:\n%q", result)
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
