package renderers

import (
	"regexp"
	"strings"

)

// gitDiffPlusMinusRe matches `git diff` change lines: a line starting with
// `+` or `-` that is NOT immediately followed by another `+`/`-` (so file
// headers `+++ b/path` and `--- a/path` are excluded).
//
// The OmniRoute source uses /^[+-](?![+-])/, but Go's regexp package is
// RE2-based and does NOT support negative lookahead. We translate the
// same intent to a negated character class: the line must start with `+`
// or `-` and have a second character that is NOT `+` or `-`. A single-
// character `+` or `-` line is rare in real git diffs and is treated as
// not-a-change (which is conservative and safe).
var gitDiffPlusMinusRe = regexp.MustCompile(`^[+-][^+-]`)

// renderGitDiff is the RTK semantic renderer for `git diff` / `git show`
// output. It keeps only:
//
//   - `diff --git a/... b/...` file header lines
//   - `@@ ... @@` hunk headers
//   - Change lines starting with `+` or `-` (but NOT `+++`/`---`)
//
// Context lines (space-prefixed), index lines, mode lines, and file-path
// metadata are dropped. If no hunk header is present, the input is not a
// real diff and the renderer no-ops (preserves the original text).
//
// Aligned with OmniRoute's renderers/gitDiff.ts.
func renderGitDiff(text string, _ DetectionInfo) (RenderResult, bool) {
	if !strings.Contains(text, "@@ ") {
		return NoRender(text)
	}

	var kept []string
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			kept = append(kept, line)
		case strings.HasPrefix(line, "@@ "):
			kept = append(kept, line)
		case gitDiffPlusMinusRe.MatchString(line):
			kept = append(kept, line)
		}
		// Drop everything else: context (space-prefixed), index, --- a/,
		// +++ b/, mode lines, similarity index, etc.
	}

	out := strings.Join(kept, "\n")
	if out == text {
		return NoRender(text)
	}
	return RenderResult{Text: out, Changed: true, Renderer: "git-diff"}, true
}
