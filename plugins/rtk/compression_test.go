package rtk

import (
	"testing"
)

// TestProcessRtkText_RawOutputBypass verifies that processRtkText short-
// circuits when the input carries the server-side sentinel prefix, returning
// the persisted body verbatim and recording the bypass in stats.Techniques.
// This is the contract that breaks the "raw-output recursion" bug: every
// LLM fetch of /api/context/rtk/raw-output/{id} must reach the LLM without
// triggering another round of compression + raw-output persistence.
func TestProcessRtkText_RawOutputBypass(t *testing.T) {
	originalBody := "PASS\nok github.com/foo/bar 0.123s\nFAIL\nfail github.com/foo/baz\n"
	wrapped := WrapRawOutputForHTTP(originalBody, "abc123456789abcdef01234567", len(originalBody), "")

	cfg := &Config{
		Enabled:            true,
		MaxCharsPerResult:  50, // would normally truncate; bypass must skip this
		MaxLinesPerResult:  1,
		RawOutputRetention: string(RawOutputRetentionAlways), // would normally persist; bypass must skip
	}

	out, stats := processRtkText(wrapped, cfg)

	if out != originalBody {
		t.Fatalf("bypass should return original body unchanged\n got: %q\nwant: %q", out, originalBody)
	}
	if stats.Truncated {
		t.Fatalf("bypass must not mark Truncated=true (would re-emit a marker)")
	}
	if !hasTechnique(stats.Techniques, "rtk-raw-output-bypass") {
		t.Fatalf("bypass technique should be recorded, got %v", stats.Techniques)
	}
	if len(stats.RawOutputPointers) != 0 {
		t.Fatalf("bypass must not produce new raw_output_pointers, got %d", len(stats.RawOutputPointers))
	}
}

// TestProcessRtkText_NoSentinel_Compresses confirms that input without the
// sentinel flows through the normal pipeline (no bypass). We use a string
// that produces a stable, easily-asserted result.
func TestProcessRtkText_NoSentinel_Compresses(t *testing.T) {
	// Empty input returns early without bypass (caught by the empty-input
	// guard); choose an input that does NOT match the sentinel and that
	// the normal pipeline will pass through.
	cfg := &Config{
		Enabled:            true,
		MaxCharsPerResult:  12_000,
		MaxLinesPerResult:  120,
		RawOutputRetention: string(RawOutputRetentionNever), // skip persistence side-effect
	}
	input := "short body without sentinel"

	out, stats := processRtkText(input, cfg)

	if out != input {
		t.Fatalf("non-sentinel input should round-trip; got %q want %q", out, input)
	}
	if hasTechnique(stats.Techniques, "rtk-raw-output-bypass") {
		t.Fatalf("non-sentinel input must not record bypass technique, got %v", stats.Techniques)
	}
}

// TestProcessRtkText_BypassEmptyBody covers the degenerate case where the
// sentinel wraps an empty body. The bypass must return "" and not crash.
func TestProcessRtkText_BypassEmptyBody(t *testing.T) {
	wrapped := WrapRawOutputForHTTP("", "0123456789abcdef01234567", 0, "")

	cfg := &Config{Enabled: true}
	out, stats := processRtkText(wrapped, cfg)

	if out != "" {
		t.Fatalf("bypass empty body should return empty string, got %q", out)
	}
	if !hasTechnique(stats.Techniques, "rtk-raw-output-bypass") {
		t.Fatalf("bypass technique should be recorded, got %v", stats.Techniques)
	}
}

// hasTechnique is a tiny local helper. Distinct from rtk_test.go's contains
// (which has a string×string signature); the two coexist in the same package.
func hasTechnique(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
