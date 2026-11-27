package handlers

import (
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

func fallbackStrings(fallbacks []schemas.Fallback) []string {
	out := make([]string, 0, len(fallbacks))
	for _, f := range fallbacks {
		out = append(out, string(f.Provider)+"/"+f.Model)
	}
	return out
}

// TestParseFallbacksProviderModel covers the happy path: a known provider prefix
// followed by a model name is split into Provider + Model.
func TestParseFallbacksProviderModel(t *testing.T) {
	got, err := parseFallbacks([]string{"sensenova/deepseek-flash"})
	if err != nil {
		t.Fatalf("parseFallbacks returned unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 fallback, got %d: %#v", len(got), got)
	}
	if got[0].Provider != schemas.ModelProvider("sensenova") {
		t.Errorf("Provider = %q, want %q", got[0].Provider, "sensenova")
	}
	if got[0].Model != "deepseek-flash" {
		t.Errorf("Model = %q, want %q", got[0].Model, "deepseek-flash")
	}
}

// TestParseFallbacksDropsBareModelName documents CURRENT behaviour, not desired
// behaviour: a bare model name has no provider prefix, so ParseModelString
// returns ("", "<model>") and parseFallbacks silently drops the entry.
//
// This is a known defect, see the parseFallbacks doc comment. It is asserted
// here so that any future change (e.g. defaulting to the primary provider's
// provider) shows up as an intentional test update rather than a silent shift.
func TestParseFallbacksDropsBareModelName(t *testing.T) {
	cases := []string{
		"deepseek-flash",           // plain bare model name
		"glm-5.2",                  // bare model name with dots
		"meta-llama/Llama-3.1-8B",  // namespace prefix that is not a known provider
		"deepseek-flash ",          // bare name plus trailing space: still no provider prefix
		" openai/gpt-4o",           // leading whitespace -> provider prefix unknown -> dropped
		"Sensenova/deepseek-flash", // provider match is case-sensitive
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			got, err := parseFallbacks([]string{in})
			if err != nil {
				t.Fatalf("parseFallbacks(%q) returned unexpected error: %v", in, err)
			}
			if len(got) != 0 {
				t.Fatalf("expected %q to be dropped (current behaviour), got %#v", in, got)
			}
		})
	}
}

// TestParseFallbacksSkipsEmptyAndIncomplete ensures empty strings, whitespace-only
// entries and provider-prefixed entries with an empty model are skipped without
// panicking, and that the returned error stays nil.
func TestParseFallbacksSkipsEmptyAndIncomplete(t *testing.T) {
	cases := [][]string{
		nil,
		{},
		{""},
		{"   "},
		{"sensenova/"},
		{"openai/"},
		{"/gpt-4o"},
		{"/"},
	}
	for _, in := range cases {
		got, err := parseFallbacks(in)
		if err != nil {
			t.Fatalf("parseFallbacks(%#v) returned unexpected error: %v", in, err)
		}
		if len(got) != 0 {
			t.Fatalf("parseFallbacks(%#v) = %#v, want empty", in, got)
		}
	}
}

// TestParseFallbacksPreservesOrderAndCount verifies entries keep their relative
// order and that no entry is added or removed when all are well formed.
func TestParseFallbacksPreservesOrderAndCount(t *testing.T) {
	in := []string{
		"sensenova/deepseek-flash",
		"openai/gpt-4o",
		"anthropic/claude-sonnet-4-5",
		"bedrock/anthropic.claude-3-5-sonnet",
	}
	got, err := parseFallbacks(in)
	if err != nil {
		t.Fatalf("parseFallbacks returned unexpected error: %v", err)
	}
	want := fallbackStrings(got)
	if len(got) != len(in) {
		t.Fatalf("expected %d fallbacks, got %d: %#v", len(in), len(got), got)
	}
	for i := range got {
		if want[i] != in[i] {
			t.Errorf("fallback[%d] = %q, want %q (order must be preserved)", i, want[i], in[i])
		}
	}
}

// TestParseFallbacksDoesNotDeduplicate asserts current behaviour: duplicates are
// kept as-is. This is intentional - a client that repeats the same
// provider/model entry uses it to rotate through different API keys for the same
// model ("same-model fallback"), which is a valid and heavily used retry
// strategy in production.
func TestParseFallbacksDoesNotDeduplicate(t *testing.T) {
	in := []string{
		"sensenova/deepseek-flash",
		"sensenova/deepseek-flash",
		"sensenova/deepseek-flash",
	}
	got, err := parseFallbacks(in)
	if err != nil {
		t.Fatalf("parseFallbacks returned unexpected error: %v", err)
	}
	if len(got) != len(in) {
		t.Fatalf("expected duplicates to be preserved (%d), got %d: %#v", len(in), len(got), got)
	}
	for i, f := range got {
		if f.Provider != "sensenova" || f.Model != "deepseek-flash" {
			t.Errorf("fallback[%d] = %#v, want sensenova/deepseek-flash", i, f)
		}
	}
}

// TestParseFallbacksDoesNotTrimModelName documents that whitespace after the
// provider prefix is preserved verbatim, so "openai/gpt-4o " keeps the trailing
// space in Model (the provider prefix itself must not be padded).
func TestParseFallbacksDoesNotTrimModelName(t *testing.T) {
	got, err := parseFallbacks([]string{"openai/gpt-4o "})
	if err != nil {
		t.Fatalf("parseFallbacks returned unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 fallback, got %d: %#v", len(got), got)
	}
	if got[0].Model != "gpt-4o " {
		t.Errorf("Model = %q, want %q (not trimmed)", got[0].Model, "gpt-4o ")
	}
}

// TestParseFallbacksMixed covers a realistic payload mixing valid entries with
// an empty string and a bare model name: only the two provider-prefixed entries
// survive, in order.
func TestParseFallbacksMixed(t *testing.T) {
	in := []string{"sensenova/deepseek-flash", "", "glm-5.2", "openai/gpt-4o"}
	got, err := parseFallbacks(in)
	if err != nil {
		t.Fatalf("parseFallbacks returned unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 fallbacks, got %d: %#v", len(got), got)
	}
	if got[0].Provider != "sensenova" || got[0].Model != "deepseek-flash" {
		t.Errorf("fallback[0] = %#v, want sensenova/deepseek-flash", got[0])
	}
	if got[1].Provider != "openai" || got[1].Model != "gpt-4o" {
		t.Errorf("fallback[1] = %#v, want openai/gpt-4o", got[1])
	}
}

// TestParseFallbacksNeverReturnsError pins the error contract: whatever the
// input, the error return is nil (the signature is kept for callers that
// already handle it).
func TestParseFallbacksNeverReturnsError(t *testing.T) {
	inputs := [][]string{
		nil,
		{"garbage"},
		{"sensenova/deepseek-flash", "", "/", "unknown/model", "   "},
		{"a/b", "c/d", "e/f"},
	}
	for _, in := range inputs {
		if _, err := parseFallbacks(in); err != nil {
			t.Errorf("parseFallbacks(%#v) returned error %v, want nil", in, err)
		}
	}
}
