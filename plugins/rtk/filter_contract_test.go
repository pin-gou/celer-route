package rtk

import (
	"testing"
)

// TestBuiltinFilterContracts executes every `tests` contract embedded in the
// 134 builtin filter JSONs (filters/builtin/*.json). Each filter declares an
// inline "data + assertion" pair — Input feeds the filter's own compression
// semantics and Expected is the exact output the filter must produce. This is
// the missing executor for the builtin contracts: RunRtkFilterTests
// (verify.go) presence-checks builtin tests only and executes contracts solely
// for custom-loaded filters. Skips vacuous stub entries (empty Input AND empty
// Expected — these are placeholder rows that assert nothing).
//
// The executor mirrors the filter-specific steps of the production pipeline
// (compression.go): line filter rules → matchOutput short-circuit → smart
// head/tail truncation → onEmpty fallback. Contrast-sensitive and trailing
// newline-insensitive via trimComparable, matching RunRtkFilterTests.
func TestBuiltinFilterContracts(t *testing.T) {
	loader := NewFilterLoader(&Config{Enabled: true})
	if err := loader.Load(""); err != nil {
		t.Fatalf("loader.Load: %v", err)
	}

	total, executed, skipped := 0, 0, 0
	for _, f := range loader.builtins {
		if f == nil || len(f.Tests) == 0 {
			continue
		}
		for _, tt := range f.Tests {
			total++
			if tt.Input == "" && tt.Expected == "" {
				skipped++
				continue
			}
			executed++
			tt := tt
			name := f.Name + "/" + tt.Name
			t.Run(name, func(t *testing.T) {
				got := applyFilterContract(tt.Input, f)
				if trimComparable(got) != trimComparable(tt.Expected) {
					t.Errorf("filter %s contract %q mismatch\n  expected: %q\n  actual:   %q",
						f.Name, tt.Name, tt.Expected, got)
				}
			})
		}
	}
	if executed == 0 {
		t.Fatal("no builtin filter contracts executed — something is wrong")
	}
	t.Logf("builtin filter contracts: executed=%d skipped-vacuous=%d (total=%d)",
		executed, skipped, total)
}

// applyFilterContract runs one Input through the filter's own compression
// semantics: line-level rules (strip/keep/collapse/replace), matchOutput
// short-circuit, smart head/tail truncation (standard intensity), and the
// onEmpty fallback when the pipeline reduces the output to nothing. Mirror of
// the filter-specific steps in processRtkTextWithCommand.
func applyFilterContract(input string, f *Filter) string {
	if f == nil {
		return input
	}
	out := applyLineFilter(input, f)
	if rep, hit := applyMatchOutputRules(out, f); hit {
		return rep
	}
	eff := scaleFilterForIntensity(f, "standard")
	out, _ = applySmartTruncate(out, eff)
	if trimComparable(out) == "" && f.OnEmpty != "" {
		return f.OnEmpty
	}
	return out
}
