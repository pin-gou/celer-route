package renderers

import (
	"strings"
	"testing"
)

// TestRenderTestGreen_PytestAllGreen verifies that a pytest all-green run
// is collapsed to a single summary line.
func TestRenderTestGreen_PytestAllGreen(t *testing.T) {
	input := `============ test session starts ============
collected 142 items

tests/a.py ................
tests/b.py ................

============ 142 passed in 3.21s ============`
	res, ok := renderTestGreen(input, DetectionInfo{Type: "test-pytest"})
	if !ok {
		t.Fatal("renderTestGreen should have applied for green pytest output")
	}
	if !res.Changed {
		t.Error("Changed=true expected")
	}
	if !strings.Contains(res.Text, "142 passed") {
		t.Errorf("expected '142 passed' in result, got %q", res.Text)
	}
	if strings.Contains(res.Text, "................") {
		t.Error("test progress dots should be dropped")
	}
	if res.Renderer != "test-green" {
		t.Errorf("Renderer = %q, want %q", res.Renderer, "test-green")
	}
}

// TestRenderTestGreen_AnyFailureNoOp verifies that ANY failure signal
// forces a no-op so the caller retains the full diagnostic output.
func TestRenderTestGreen_AnyFailureNoOp(t *testing.T) {
	input := `tests/a.py ..F..
=== 1 failed, 4 passed in 1.0s ===
E   AssertionError: nope`
	res, ok := renderTestGreen(input, DetectionInfo{Type: "test-pytest"})
	if ok {
		t.Fatal("renderTestGreen should have returned no-op (failure detected)")
	}
	if res.Text != input {
		t.Errorf("Text should equal input on no-op, got %q", res.Text)
	}
}

// TestRenderTestGreen_AnsiColoredFAILNoOp is the regression test for the
// ANSI-byte-with-F regression: jest/vitest emit a colored "FAIL" header
// where the preceding ANSI byte ('m' of [31m) is a word char and would
// defeat a `\bFAIL\b` boundary on the raw string.
func TestRenderTestGreen_AnsiColoredFAILNoOp(t *testing.T) {
	// Note: the byte sequence below is the ESC char (0x1b) plus "[31m" plus
	// "FAIL" plus "[39m" plus "[22m" — verbatim from a real jest run.
	input := "\x1b[1m\x1b[31mFAIL\x1b[39m\x1b[22m src/auth.test.ts\nTests: 3 passed, 3 total"
	res, ok := renderTestGreen(input, DetectionInfo{Type: "test-jest"})
	if ok {
		t.Fatal("renderTestGreen should have no-op'd on ANSI-colored FAIL")
	}
	if res.Text != input {
		t.Errorf("Text should equal input on no-op, got %q", res.Text)
	}
}

// TestRenderTestGreen_NumericFailedCountNoOp verifies that a "3 failed"
// count forces a no-op even when no FAIL word is present.
func TestRenderTestGreen_NumericFailedCountNoOp(t *testing.T) {
	input := "tests/test_foo.py ....\n=== 3 failed, 4 passed in 1.0s ==="
	res, ok := renderTestGreen(input, DetectionInfo{Type: "test-pytest"})
	if ok {
		t.Fatal("renderTestGreen should have no-op'd on numeric failed count")
	}
	if res.Text != input {
		t.Errorf("Text should equal input on no-op, got %q", res.Text)
	}
}

// TestRenderTestGreen_EslintEmptyOutput verifies that empty output is
// synthesised into a clean "ESLint: 0 problems found" summary.
func TestRenderTestGreen_EslintEmptyOutput(t *testing.T) {
	res, ok := renderTestGreen("", DetectionInfo{Type: "build-eslint"})
	if !ok {
		t.Fatal("renderTestGreen should have applied for empty eslint output")
	}
	if !res.Changed {
		t.Error("Changed=true expected")
	}
	if !strings.Contains(res.Text, "ESLint: 0 problems found") {
		t.Errorf("expected 'ESLint: 0 problems found', got %q", res.Text)
	}
	_ = res
}

// TestRenderTestGreen_JestSummary verifies jest summary extraction.
func TestRenderTestGreen_JestSummary(t *testing.T) {
	input := "PASS src/foo.test.ts\nPASS src/bar.test.ts\n\nTests: 5 passed, 5 total"
	res, ok := renderTestGreen(input, DetectionInfo{Type: "test-jest"})
	if !ok {
		t.Fatal("renderTestGreen should have applied for jest summary")
	}
	if !strings.Contains(res.Text, "Tests: 5 passed") {
		t.Errorf("expected 'Tests: 5 passed', got %q", res.Text)
	}
}