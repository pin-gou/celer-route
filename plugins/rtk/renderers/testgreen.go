package renderers

import (
	"regexp"
	"strings"

)

// ansiStripRe removes ANSI color escape codes (e.g. `\x1b[31m`) before
// running the failure guards. Some jest/vitest CLIs emit a colored "FAIL"
// header where the preceding ANSI byte (`m` of `\x1b[31m`) is a word char,
// defeating a `\bFAIL\b` boundary on the raw string. Stripping ANSI first
// is the only safe way to detect failure reliably. Mirrors OmniRoute's
// `text.replace(/\x1b\[[0-9;]*m/g, "")`.
var ansiStripRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// testGreenFailureGuards returns the list of regexes that, when ANY matches
// the (ANSI-stripped) text, force a no-op so the caller retains the full
// diagnostic output. Aligned with OmniRoute's testGreen.ts failure guards.
func testGreenFailureGuards() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile(`\bFAIL\b`),         // jest/vitest "FAIL"
		regexp.MustCompile(`✖`),                // jest/vitest ✖ symbol
		regexp.MustCompile(`Error`),            // any "Error" mention
		regexp.MustCompile(`Traceback`),        // Python tracebacks
		regexp.MustCompile(`AssertionError`),   // Python assertion failures
		regexp.MustCompile(`(\d+)\s+failed`),   // numeric failed counts ("3 failed")
		regexp.MustCompile(`failed[:\s]+(\d+)`), // "failed: 3"
	}
}

// testGreenPytestRe matches pytest summary lines like
// "============ 142 passed in 3.21s ============" (also variants with
// warnings, e.g. "============ 142 passed, 5 warnings ============").
var testGreenPytestRe = regexp.MustCompile(`={3,}\s+\d+\s+passed`)

// testGreenJestRe matches jest summary lines like
// "Tests: 3 passed, 3 total" or "Test Suites: 1 passed, 1 total".
var testGreenJestRe = regexp.MustCompile(`Tests:\s+\d+\s+passed`)

// testGreenVitestRe matches vitest/jest run summary lines like
// "✓ 5 tests passed" or "8 tests passed".
var testGreenVitestRe = regexp.MustCompile(`\d+\s+tests?\s+passed`)

// renderTestGreen is the RTK semantic renderer for test suite output
// (pytest, jest, vitest, eslint). It ONLY collapses when the output
// indicates TOTAL success — ANY sign of failure forces a no-op to preserve
// full diagnostics. Aligned with OmniRoute's testGreen.ts.
//
// Failure guards (any one triggers no-op):
//   - `\bFAIL\b`         (jest/vitest "FAIL")
//   - `✖`               (jest/vitest failure symbol)
//   - `Error`           (any error mention)
//   - `Traceback`       (Python traceback)
//   - `AssertionError`  (Python assertion failure)
//   - `\d+\s+failed`    (numeric failed counts: "3 failed")
//   - `failed[:\s]+\d+` ("failed: 3")
//
// When green, the renderer extracts the first matching summary line:
//   - pytest: `==== N passed in X.Xs ====`
//   - jest:   `Tests: N passed, N total`
//   - vitest: `✓ N tests passed` / `N tests passed`
//   - eslint empty output (no errors): synthesises "ESLint: 0 problems found"
func renderTestGreen(text string, _ DetectionInfo) (RenderResult, bool) {
	stripped := ansiStripRe.ReplaceAllString(text, "")

	// Failure guards FIRST — never weaken. The renderer is fail-safe: any
	// ambiguity is treated as a failure to preserve diagnostics.
	for _, re := range testGreenFailureGuards() {
		if re.MatchString(stripped) {
			return NoRender(text)
		}
	}

	// Extract the first matching summary line.
	for _, line := range strings.Split(stripped, "\n") {
		if testGreenPytestRe.MatchString(line) {
			return RenderResult{Text: strings.TrimSpace(line), Changed: true, Renderer: "test-green"}, true
		}
		if testGreenJestRe.MatchString(line) {
			return RenderResult{Text: strings.TrimSpace(line), Changed: true, Renderer: "test-green"}, true
		}
		if testGreenVitestRe.MatchString(line) {
			return RenderResult{Text: strings.TrimSpace(line), Changed: true, Renderer: "test-green"}, true
		}
	}

	// eslint with no findings: empty output (after strip) is treated as clean.
	trimmed := strings.TrimSpace(stripped)
	if trimmed == "" {
		return RenderResult{Text: "ESLint: 0 problems found", Changed: true, Renderer: "test-green"}, true
	}

	return NoRender(text)
}
