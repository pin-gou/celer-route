package rtk

// Stage-4 verify module tests (TDD red phase):
//   - TestTrimComparable           trimComparable 去除尾部换行
//   - TestRunRtkFilterTests        (V-plugins-6) Passed/Outcomes/Benchmark 三段
//   - TestBuiltinFiltersHaveTests  (V-plugins-7) FiltersWithoutTests 为空
//   - TestBenchmarkAggregation     (V-plugins-8) benchmark 按 category 聚合数学
//
// 注意：fixture filters 通过 project source 注入（<appDir>/.rtk/filters.json +
// TrustProjectFilters 绕过 SHA256 验证），input 不含尾部换行以确保 applyLineFilter
// 输出与 trimComparable(expected) 在两种实现语义下均一致。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestTrimComparable verifies that trimComparable removes trailing newlines
// (\n) but preserves non-newline content.
func TestTrimComparable(t *testing.T) {
	tt := []struct {
		name     string
		input    string
		expected string
	}{
		{"single_trailing_newline", "hello\n", "hello"},
		{"multiple_lines", "hello\nworld\n", "hello\nworld"},
		{"multiple_trailing_newlines", "hello\n\n", "hello"},
		{"no_newline", "hello", "hello"},
		{"empty_string", "", ""},
		{"only_newline", "\n", ""},
		{"many_newlines", "hello\nworld\n\n\n", "hello\nworld"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := trimComparable(tc.input)
			if got != tc.expected {
				t.Errorf("trimComparable(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// writeFixtureFilters writes a slice of filters as the project-source
// filters.json (<appDir>/.rtk/filters.json) for RunRtkFilterTests to load.
func writeFixtureFilters(t *testing.T, appDir string, filters []Filter) {
	t.Helper()
	projectDir := filepath.Join(appDir, ".rtk")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(filters)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "filters.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunRtkFilterTests (V-plugins-6) verifies that RunRtkFilterTests returns
// the correct Passed/Outcomes/Benchmark/FiltersWithoutTests segments.
func TestRunRtkFilterTests(t *testing.T) {
	appDir := t.TempDir()
	writeFixtureFilters(t, appDir, []Filter{
		{
			Name: "pass-filter", ID: "pass-filter", Category: "test",
			Rules: []LineRule{{Type: "strip", Pattern: "\\bDEBUG\\b"}},
			Tests: []FilterTest{
				{Name: "pass-1", Input: "DEBUG: log\ninfo: ok", Expected: "info: ok"},
			},
		},
		{
			Name: "fail-filter", ID: "fail-filter", Category: "test",
			Rules: []LineRule{{Type: "strip", Pattern: "\\bTRACE\\b"}},
			Tests: []FilterTest{
				{Name: "fail-1", Input: "TRACE: debug\noutput line", Expected: "something else"},
			},
		},
	})

	result := RunRtkFilterTests(&VerifyOptions{
		AppDir:               appDir,
		CustomFiltersEnabled: true,
		TrustProjectFilters:  true,
	})

	if result.Passed {
		t.Error("expected Passed=false (one test fails), got true")
	}

	if len(result.Outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(result.Outcomes))
	}

	// Check pass outcome.
	passOutcome := findOutcome(result.Outcomes, "pass-filter", "pass-1")
	if passOutcome == nil {
		t.Fatal("outcome for pass-filter/pass-1 not found")
	}
	if !passOutcome.Passed {
		t.Errorf("pass-filter/pass-1: expected Passed=true, got false")
	}
	if passOutcome.Actual != "info: ok" {
		t.Errorf("pass-filter/pass-1: Actual=%q, want %q", passOutcome.Actual, "info: ok")
	}
	if passOutcome.Expected != "info: ok" {
		t.Errorf("pass-filter/pass-1: Expected=%q, want %q", passOutcome.Expected, "info: ok")
	}

	// Check fail outcome.
	failOutcome := findOutcome(result.Outcomes, "fail-filter", "fail-1")
	if failOutcome == nil {
		t.Fatal("outcome for fail-filter/fail-1 not found")
	}
	if failOutcome.Passed {
		t.Errorf("fail-filter/fail-1: expected Passed=false, got true")
	}
	if failOutcome.Actual != "output line" {
		t.Errorf("fail-filter/fail-1: Actual=%q, want %q", failOutcome.Actual, "output line")
	}
	if failOutcome.Expected != "something else" {
		t.Errorf("fail-filter/fail-1: Expected=%q, want %q", failOutcome.Expected, "something else")
	}

	// Benchmark should contain at least the "test" category row.
	if len(result.Benchmark) == 0 {
		t.Fatal("expected at least 1 benchmark row, got 0")
	}
	testRow := findBenchmarkRow(result.Benchmark, "test")
	if testRow == nil {
		t.Fatal("benchmark row for category 'test' not found")
	}
	if testRow.Filters != 2 {
		t.Errorf("benchmark 'test' Filters=%d, want 2", testRow.Filters)
	}
	if testRow.Tests != 2 {
		t.Errorf("benchmark 'test' Tests=%d, want 2", testRow.Tests)
	}
}

// findOutcome finds the first outcome matching the given filter ID and test name.
func findOutcome(outcomes []FilterTestOutcome, filterID, testName string) *FilterTestOutcome {
	for i := range outcomes {
		if outcomes[i].FilterID == filterID && outcomes[i].TestName == testName {
			return &outcomes[i]
		}
	}
	return nil
}

// findBenchmarkRow finds the first benchmark row with the given category.
func findBenchmarkRow(rows []FilterBenchmarkRow, cat string) *FilterBenchmarkRow {
	for i := range rows {
		if rows[i].Category == cat {
			return &rows[i]
		}
	}
	return nil
}

// TestBuiltinFiltersHaveTests (V-plugins-7) verifies that after all 52
// builtin filters are supplemented with tests, FiltersWithoutTests is empty.
// In the red phase this test fails because none of the builtins have tests.
func TestBuiltinFiltersHaveTests(t *testing.T) {
	result := RunRtkFilterTests(nil)
	if len(result.FiltersWithoutTests) > 0 {
		t.Errorf("expected 0 filters without tests, got %d: %v",
			len(result.FiltersWithoutTests), result.FiltersWithoutTests)
	}
}

// TestBenchmarkAggregation (V-plugins-8) verifies that benchmark rows are
// sorted by category ascending and that the aggregation math (filters/tests/
// averageSavingsPercent) is correct.
//
// Fixture layout:
//   - strip-build  (category "build"): 2 tests → savings 50.00%, 37.50%
//   - strip-test   (category "test"):  2 tests → savings 50.00%, 37.50%
//   - strip-test2  (category "test"):  1 test  → savings 100.00%
//
// Expected:
//   build: Filters=1, Tests=2, avg=(50.00+37.50)/2=43.75
//   test:  Filters=2, Tests=3, avg=(50.00+37.50+100.00)/3=62.50
func TestBenchmarkAggregation(t *testing.T) {
	appDir := t.TempDir()
	writeFixtureFilters(t, appDir, []Filter{
		{
			Name: "strip-build", ID: "strip-build", Category: "build",
			Rules: []LineRule{{Type: "strip", Pattern: "\\bDEBUG\\b"}},
			Tests: []FilterTest{
				{Name: "build-1", Input: "DEBUG line one\nkeep this line", Expected: "keep this line"},
				{Name: "build-2", Input: "line one\nDEBUG two\nline three", Expected: "line one\nline three"},
			},
		},
		{
			Name: "strip-test", ID: "strip-test", Category: "test",
			Rules: []LineRule{{Type: "strip", Pattern: "\\bTRACE\\b"}},
			Tests: []FilterTest{
				{Name: "test-1", Input: "TRACE line one\nkeep this line", Expected: "keep this line"},
				{Name: "test-2", Input: "line one\nTRACE two\nline three", Expected: "line one\nline three"},
			},
		},
		{
			Name: "strip-test2", ID: "strip-test2", Category: "test",
			Rules: []LineRule{{Type: "strip", Pattern: "\\bTRACE\\b"}},
			Tests: []FilterTest{
				{Name: "test-3", Input: "TRACE only line", Expected: ""},
			},
		},
	})

	result := RunRtkFilterTests(&VerifyOptions{
		AppDir:               appDir,
		CustomFiltersEnabled: true,
		TrustProjectFilters:  true,
	})

	// Verify rows are sorted by category ascending.
	if len(result.Benchmark) < 2 {
		t.Fatalf("expected at least 2 benchmark rows, got %d", len(result.Benchmark))
	}
	if result.Benchmark[0].Category > result.Benchmark[1].Category {
		t.Errorf("benchmark rows not sorted by category: %s > %s",
			result.Benchmark[0].Category, result.Benchmark[1].Category)
	}

	// Assert exact values for the "build" row.
	buildRow := findBenchmarkRow(result.Benchmark, "build")
	if buildRow == nil {
		t.Fatal("benchmark row for 'build' not found")
	}
	if buildRow.Filters != 1 {
		t.Errorf("build row Filters=%d, want 1", buildRow.Filters)
	}
	if buildRow.Tests != 2 {
		t.Errorf("build row Tests=%d, want 2", buildRow.Tests)
	}
	if buildRow.AverageSavingsPercent != 43.75 {
		t.Errorf("build row AverageSavingsPercent=%v, want 43.75", buildRow.AverageSavingsPercent)
	}

	// Assert exact values for the "test" row.
	testRow := findBenchmarkRow(result.Benchmark, "test")
	if testRow == nil {
		t.Fatal("benchmark row for 'test' not found")
	}
	if testRow.Filters != 2 {
		t.Errorf("test row Filters=%d, want 2", testRow.Filters)
	}
	if testRow.Tests != 3 {
		t.Errorf("test row Tests=%d, want 3", testRow.Tests)
	}
	if testRow.AverageSavingsPercent != 62.50 {
		t.Errorf("test row AverageSavingsPercent=%v, want 62.50", testRow.AverageSavingsPercent)
	}
}