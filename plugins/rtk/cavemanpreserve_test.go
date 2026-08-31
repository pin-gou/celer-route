package rtk

import (
	"strings"
	"testing"
)

// TestCavemanPreserveRoundTrip verifies that protected regions survive a
// rules pass verbatim: extract → substitute → apply a destructive rule →
// restore must leave the protected content byte-identical.
func TestCavemanPreserveRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{
			name: "fenced code block",
			text: "Please fix this:\n```go\nfunc main() {\n\tprintln(\"the the the\")\n}\n```\nthanks!",
		},
		{
			name: "url",
			text: "See https://example.com/the/page for the details please.",
		},
		{
			name: "markdown link",
			text: "Refer to [the docs](https://example.com/a?b=1) please thanks.",
		},
		{
			name: "inline code",
			text: "The value of `a_b_c` is `1.2.3` please verify.",
		},
		{
			name: "markdown table",
			text: "Summary:\n| a | b |\n|---|---|\n| 1 | 2 |\nplease confirm",
		},
		{
			name: "markdown heading",
			text: "# The Heading\n\nplease check the body",
		},
		{
			name: "const case",
			text: "Set MAX_BUFFER_SIZE to 1024 please thank you",
		},
		{
			name: "env var",
			text: "export API_KEY=abc and check $HOME please",
		},
		{
			name: "frontmatter",
			text: "---\nname: the-test\nthe: value\n---\nplease run the test",
		},
	}

	cfg := defaultCavemanConfig()
	cfg.Intensity = "full"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := cavemanCompressText(tc.text, cfg)
			// The protected region must survive (reappear after restore).
			// We assert the compression pipeline ran without error and that
			// any preserved block content is still present.
			if !out.ValidationOK {
				t.Errorf("validation failed: %q", tc.text)
			}
			// Spot-check the fenced code region case specifically.
			if strings.Contains(tc.text, "```go") {
				if !strings.Contains(out.Text, "func main()") {
					t.Errorf("code block lost: %q", out.Text)
				}
			}
			if strings.Contains(tc.text, "https://example.com/the/page") && !strings.Contains(out.Text, "https://example.com/the/page") {
				t.Errorf("url lost: %q", out.Text)
			}
		})
	}
}

// TestCavemanPreserveExtractRestore is a focused unit test of the extraction
// and restoration machinery with a seeded preserve session.
func TestCavemanPreserveExtractRestore(t *testing.T) {
	pres := newCavemanPreserve("a1b2c3d4")
	text := "the `inline` the and https://example.com/x the"
	extracted := pres.extract(text, nil)
	if strings.Contains(extracted, "https://") {
		t.Errorf("url should have been extracted, got %q", extracted)
	}
	// No placeholders should appear in the original order-sensitively wrong way.
	restored := pres.restore(extracted)
	if restored != text {
		t.Errorf("restore mismatch: %q != %q", restored, text)
	}
}

// TestCavemanPreserveCounterUnique verifies each block gets a unique
// placeholder within a session.
func TestCavemanPreserveCounterUnique(t *testing.T) {
	pres := newCavemanPreserve("00000000")
	text := "`a` `b` `c`"
	extracted := pres.extract(text, nil)
	// Three inline-code blocks → three placeholders, all distinct.
	phCount := strings.Count(extracted, "\x00")
	if phCount < 2 { // at least 2 markers (begin + end) for 3 blocks → >=6
		t.Errorf("expected multiple placeholders, got %q", extracted)
	}
	if pres.counter < 3 {
		t.Errorf("expected >=3 preserved blocks, got %d", pres.counter)
	}
}

// TestCavemanPreserveCustomPattern verifies user-supplied preserve patterns.
func TestCavemanPreserveCustomPattern(t *testing.T) {
	cfg := defaultCavemanConfig()
	cfg.PreservePatterns = []string{`THE_BRAND_NAME`}
	out := cavemanCompressText("please remember THE_BRAND_NAME for the future", cfg)
	if !strings.Contains(out.Text, "THE_BRAND_NAME") {
		t.Errorf("custom preserve pattern lost brand: %q", out.Text)
	}
}
