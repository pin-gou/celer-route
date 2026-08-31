package rtk

import (
	"regexp"
	"strings"
)

// cavemanValidation is the result of a fidelity check: valid reports whether
// every protected item present in the original is still present verbatim in
// the compressed text; errors/warnings carry human-readable reasons. Mirrors
// OmniRoute's ValidationResult.
type cavemanValidation struct {
	valid    bool
	errors   []string
	warnings []string
}

// validateCavemanCompression runs the fidelity checks between the original
// message text and its compressed form. Because preservation already restores
// every protected block verbatim before this runs, the checks here are a
// second line of defence: they catch content the preservation pass missed
// (or that a rule introduced contextually) and signal the caller to fall back
// to the original. Ported from OmniRoute's validateCompression.
func validateCavemanCompression(original, compressed string) cavemanValidation {
	v := cavemanValidation{valid: true}
	if original == compressed {
		return v
	}

	checks := []struct {
		label string
		items []string
	}{
		{"fenced code block", collectFencedCodeBlocks(original)},
		{"inline code", collectInlineCode(original)},
		{"url", collectURLs(original)},
		{"markdown link", collectMarkdownLinks(original)},
		{"frontmatter", collectFrontmatter(original)},
		{"heading", collectHeadings(original)},
		{"table row", collectTableRows(original)},
		{"math block", collectMathBlocks(original)},
	}
	for _, c := range checks {
		for _, item := range c.items {
			if !strings.Contains(compressed, item) {
				v.valid = false
				v.errors = append(v.errors, c.label+" missing: "+truncateForValidation(item))
			}
		}
	}
	return v
}

// truncateForValidation shortens a failed item in error output so log lines
// stay readable even for huge blocks.
func truncateForValidation(s string) string {
	const max = 120
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// collectFencedCodeBlocks finds fenced code blocks (``` / ~~~) in the text.
func collectFencedCodeBlocks(text string) []string {
	re := regexp.MustCompile("(?s)```.*?```|~~~.*?~~~")
	matches := re.FindAllString(text, -1)
	return nonEmptyStrings(matches)
}

// collectInlineCode finds paired backticks on a single line.
func collectInlineCode(text string) []string {
	re := regexp.MustCompile("`[^`\\n]+`")
	return nonEmptyStrings(re.FindAllString(text, -1))
}

// collectURLs finds http/https URLs terminated by a URL-ender character.
func collectURLs(text string) []string {
	re := regexp.MustCompile(`https?://[^\s)"'>]+`)
	return nonEmptyStrings(re.FindAllString(text, -1))
}

// collectMarkdownLinks finds [label](url "title") links.
func collectMarkdownLinks(text string) []string {
	re := regexp.MustCompile(`\[[^\]\n]{1,1000}\]\([^)[ \t\n]{1,1000}(?:[ \t]+"[^"]{0,1000}")?\)`)
	return nonEmptyStrings(re.FindAllString(text, -1))
}

// collectFrontmatter finds a YAML frontmatter block at the start of the text.
func collectFrontmatter(text string) []string {
	re := regexp.MustCompile("(?ms)^---.*?---")
	if m := re.FindString(text); m != "" {
		return []string{m}
	}
	return nil
}

// collectHeadings finds markdown ATX headings (1-6 # markers).
func collectHeadings(text string) []string {
	re := regexp.MustCompile("(?m)^#{1,6}\\s+[^\\n]+")
	return nonEmptyStrings(re.FindAllString(text, -1))
}

// collectTableRows finds pipe-delimited table lines.
func collectTableRows(text string) []string {
	re := regexp.MustCompile("(?m)^\\s*\\|.*\\|\\s*$")
	rows := re.FindAllString(text, -1)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if strings.Count(r, "|") >= 2 {
			out = append(out, r)
		}
	}
	return out
}

// collectMathBlocks finds $$...$$ and \[...\] math blocks.
func collectMathBlocks(text string) []string {
	re := regexp.MustCompile(`\$\$[\s\S]{0,1000}?\$\$|\\\[[\s\S]{0,1000}?\\\]`)
	return nonEmptyStrings(re.FindAllString(text, -1))
}

// nonEmptyStrings filters out empty matches and trims surrounding whitespace.
func nonEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
