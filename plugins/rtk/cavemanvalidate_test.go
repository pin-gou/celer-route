package rtk

import (
	"strings"
	"testing"
)

// TestCavemanValidateFidelity verifies the fidelity checks flag content loss.
func TestCavemanValidateFidelity(t *testing.T) {
	cases := []struct {
		name       string
		original   string
		compressed string
		wantValid  bool
	}{
		{
			name:       "identical",
			original:   "the quick brown fox",
			compressed: "the quick brown fox",
			wantValid:  true,
		},
		{
			name:       "fenced code preserved",
			original:   "```go\nfmt.Println(\"hi\")\n```\nplease",
			compressed: "```go\nfmt.Println(\"hi\")\n```\n",
			wantValid:  true,
		},
		{
			name:       "inline code dropped",
			original:   "use `ctx` variable",
			compressed: "use ctx variable",
			wantValid:  false,
		},
		{
			name:       "url dropped",
			original:   "see https://example.com/x please",
			compressed: "see example.com please",
			wantValid:  false,
		},
		{
			name:       "heading dropped",
			original:   "# Title\nbody",
			compressed: "Title\nbody",
			wantValid:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := validateCavemanCompression(tc.original, tc.compressed)
			if v.valid != tc.wantValid {
				t.Errorf("validate(%q, %q).valid = %v, want %v (errors=%v)", tc.original, tc.compressed, v.valid, tc.wantValid, v.errors)
			}
		})
	}
}

// TestCavemanValidateCollectors verifies the individual item collectors find
// what they should.
func TestCavemanValidateCollectors(t *testing.T) {
	text := "use `ctx` and see https://example.com/a [label](https://ex.co) with:\n" +
		"# H1\n\n| a | b |\n|---|---|\n\n$$x+y$$\n\n```py\nprint(1)\n```"
	if got := collectInlineCode(text); len(got) != 1 {
		t.Errorf("collectInlineCode = %v, want 1", got)
	}
	if got := collectURLs(text); len(got) != 2 {
		t.Errorf("collectURLs = %v, want 2", got)
	}
	if got := collectMarkdownLinks(text); len(got) != 1 {
		t.Errorf("collectMarkdownLinks = %v, want 1", got)
	}
	if got := collectHeadings(text); len(got) != 1 {
		t.Errorf("collectHeadings = %v, want 1", got)
	}
	if got := collectFencedCodeBlocks(text); len(got) != 1 {
		t.Errorf("collectFencedCodeBlocks = %v, want 1", got)
	}
	if got := collectMathBlocks(text); len(got) != 1 {
		t.Errorf("collectMathBlocks = %v, want 1", got)
	}
	if got := collectTableRows(text); len(got) == 0 {
		t.Errorf("collectTableRows = %v, want >=1", got)
	}
}

// TestCavemanValidateFrontmatter ensures frontmatter detection works.
func TestCavemanValidateFrontmatter(t *testing.T) {
	text := "---\nname: test\nthe: value\n---\nbody"
	if got := collectFrontmatter(text); len(got) != 1 {
		t.Errorf("collectFrontmatter = %v, want 1", got)
	}
	if got := collectFrontmatter("no frontmatter here"); got != nil {
		t.Errorf("collectFrontmatter = %v, want nil", got)
	}
}

// TestTruncateForValidation ensures diagnostics stay bounded.
func TestTruncateForValidation(t *testing.T) {
	long := strings.Repeat("x", 500)
	short := truncateForValidation(long)
	if len(short) > 130 {
		t.Errorf("truncateForValidation too long: %d", len(short))
	}
}
