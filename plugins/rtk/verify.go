package rtk

import (
	"sort"
	"strings"
)

// FilterTestOutcome records the result of a single inline filter test.
type FilterTestOutcome struct {
	FilterID string
	TestName string
	Passed   bool
	Actual   string
	Expected string
}

// FilterBenchmarkRow aggregates test savings by filter category.
type FilterBenchmarkRow struct {
	Category             string
	Filters              int
	Tests                int
	AverageSavingsPercent float64
}

// VerifyResult is the aggregated outcome of RunRtkFilterTests.
type VerifyResult struct {
	Passed              bool
	Outcomes            []FilterTestOutcome
	FiltersWithoutTests []string
	Benchmark           []FilterBenchmarkRow
	Diagnostics         []FilterLoadDiagnostic
}

// VerifyOptions configures RunRtkFilterTests.
type VerifyOptions struct {
	RequireAll           bool   // require every filter to carry tests → Passed=false otherwise
	CustomFiltersEnabled bool   // passed through to loader.Load
	TrustProjectFilters  bool   // passed through to loader.Load
	AppDir               string // directory containing .rtk/filters.json (project source)
}

// RunRtkFilterTests is the verify entry point (design D3: pure function, Go
// test entry only). It loads custom (project/global) filters from AppDir and
// runs each filter's inline `tests` field through applyLineFilter, comparing
// the actual output against trimComparable(expected). Filters without tests are
// reported in FiltersWithoutTests (when RequireAll is set, they fail the run).
//
// Builtin filters are validated for presence of tests only — outcome/benchmark
// computation is limited to custom-loaded filters so a per-filter regression
// can be attributed precisely (design D4/D5).
func RunRtkFilterTests(opts *VerifyOptions) VerifyResult {
	if opts == nil {
		opts = &VerifyOptions{}
	}

	cfg := &Config{
		Enabled:              true,
		TrustProjectFilters:  opts.TrustProjectFilters,
		CustomFiltersEnabled: opts.CustomFiltersEnabled,
	}
	loader := NewFilterLoader(cfg)
	if opts.AppDir != "" {
		_ = loader.Load(opts.AppDir)
	}

	result := VerifyResult{
		Passed:      true,
		Outcomes:    make([]FilterTestOutcome, 0),
		Diagnostics: loader.Diagnostics(),
	}

	// Determine which filters to consider. After Load the unified cache is
	// populated (project + global + builtin); without Load only builtins are
	// available.
	filters := loader.cachedFilters
	if len(filters) == 0 {
		filters = loader.builtins
	}

	// Map builtin pointers for exclusion from test execution.
	builtinSet := make(map[*Filter]bool, len(loader.builtins))
	for _, b := range loader.builtins {
		if b != nil {
			builtinSet[b] = true
		}
	}

	// Per-category benchmark accumulators.
	type benchAccum struct {
		category   string
		filters    int
		tests      int
		sumSavings float64
	}
	bench := make(map[string]*benchAccum)
	benchCatSeen := make(map[string]bool)
	filterSeenInCat := make(map[string]map[string]bool) // category → filterID → counted

	for _, f := range filters {
		if f == nil {
			continue
		}
		filterID := f.ID
		if filterID == "" {
			filterID = f.Name
		}

		if len(f.Tests) == 0 {
			result.FiltersWithoutTests = append(result.FiltersWithoutTests, filterID)
			continue
		}

		// Tests attached to a builtin filter are presence-validated only
		// (see file-level doc comment); custom filters run their tests.
		if builtinSet[f] {
			continue
		}

		category := f.Category
		if category == "" {
			category = "generic"
		}
		if !benchCatSeen[category] {
			benchCatSeen[category] = true
			bench[category] = &benchAccum{category: category}
		}
		if filterSeenInCat[category] == nil {
			filterSeenInCat[category] = make(map[string]bool)
		}
		if !filterSeenInCat[category][filterID] {
			filterSeenInCat[category][filterID] = true
			bench[category].filters++
		}

		for _, test := range f.Tests {
			actual := applyLineFilter(test.Input, f)
			passed := trimComparable(actual) == trimComparable(test.Expected)
			result.Outcomes = append(result.Outcomes, FilterTestOutcome{
				FilterID: filterID,
				TestName: test.Name,
				Passed:   passed,
				Actual:   actual,
				Expected: test.Expected,
			})
			if !passed {
				result.Passed = false
			}

			// Benchmark savings: token-based delta between input and the
			// filtered output (V-plugins-8).
			orig := estimateTokens(test.Input)
			comp := estimateTokens(actual)
			var savings float64
			if orig > 0 {
				savings = float64(orig-comp) / float64(orig)
			}
			bench[category].tests++
			bench[category].sumSavings += savings
		}
	}

	// RequireAll: filters without tests fail the run.
	if opts.RequireAll && len(result.FiltersWithoutTests) > 0 {
		result.Passed = false
	}

	// Build the benchmark rows sorted by category ascending.
	rows := make([]FilterBenchmarkRow, 0, len(bench))
	for _, acc := range bench {
		row := FilterBenchmarkRow{
			Category: acc.category,
			Filters:  acc.filters,
			Tests:    acc.tests,
		}
		if acc.tests > 0 {
			row.AverageSavingsPercent = acc.sumSavings / float64(acc.tests) * 100
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Category < rows[j].Category
	})
	result.Benchmark = rows

	return result
}

// trimComparable removes all trailing newlines from a string so filter tests
// can be compared regardless of whether the pipeline preserves the input's
// trailing newline.
func trimComparable(value string) string {
	return strings.TrimRight(value, "\n")
}