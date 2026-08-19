package rtk

import (
	"regexp"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// CommandSample represents a captured command invocation with its combined
// stdout/stderr text.
type CommandSample struct {
	Command string `json:"command"`
	Output  string `json:"output"`
}

// NoiseCandidate represents a recurring line-template surfaced by
// DiscoverRepeatedNoise. The Pattern is a regex-compatible string derived
// from the normalised line.
type NoiseCandidate struct {
	Pattern string `json:"pattern"`
	Hits    int    `json:"hits"`
}

// ---------------------------------------------------------------------------
// Extend grouper normalization for cross-sample mining
// ---------------------------------------------------------------------------

// Package-level pre-compiled regex patterns for discoverNormalizeLine.
// All compiled at init time via regexp.MustCompile so hot-path calls avoid
// compilation overhead.
var (
	// Step 8: npm/pip package identifiers with version: word@version → <PKG>@<N>
	// Bounded quantifiers ({0,128}) are mandatory: [\w.-] followed by a required
	// @ is the classic catastrophic-backtracking shape on a long word-char run
	// with no @. Real package names are short.
	reDiscoverPkg = regexp.MustCompile(`[\w][\w.-]{0,128}@(?:<N>|\d[\w.-]{0,64})`)

	// Step 9: Error/exit codes like E404, ENOENT, E2BIG, EACCES
	reDiscoverCode = regexp.MustCompile(`\bE[A-Z0-9]{2,}\b`)

	// Step 10a: Numeric values with attached units: 5s, 120ms, 4kb, 12MB, 0.5s
	reDiscoverNumUnit = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?(?:ms|[smhd]|[kmg]b?)\b`)

	// Step 10b: Already-substituted <N> followed by a unit suffix leftover
	reDiscoverNUnit = regexp.MustCompile(`(?i)<N>(?:ms|[smhd]|[kmg]b?)\b`)

	// Whitespace collapse (after substitutions)
	reDiscoverSpaces = regexp.MustCompile(`\s+`)
)

// DiscoverNormalizeLine extends grouper.normalizeLine with additional
// substitutions relevant for mining command output across many samples:
//
//  8. npm/pip package names + versions:  left-pad@1.2.3 → <PKG>@<N>
//  9. Exit codes and error codes:        E404, ENOENT, E2BIG → <CODE>
//  10. Numeric suffixes (time/size):     5s, 120ms, 4kb → <N>
//
// Applies AFTER grouper normalizeLine so all its substitutions are already done.
func DiscoverNormalizeLine(line string) string {
	s := normalizeLine(line)
	// Step 8: npm/pip package identifiers with version
	s = reDiscoverPkg.ReplaceAllString(s, "<PKG>@<N>")
	// Step 9: Error/exit codes
	s = reDiscoverCode.ReplaceAllString(s, "<CODE>")
	// Step 10a: Numeric values with attached units
	s = reDiscoverNumUnit.ReplaceAllString(s, "<N>")
	// Step 10b: <N> followed by a unit suffix leftover
	s = reDiscoverNUnit.ReplaceAllString(s, "<N>")
	// Collapse repeated whitespace again after substitutions
	s = reDiscoverSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// normalizedToPattern converts a normalised line (with <N> placeholders) to a
// regex-safe pattern that can be used for matching.
//
// Strategy:
//  1. Escape all regex special chars in the normalised form.
//  2. Replace the literal placeholder <N> with [\S]+, <PKG> with [\S]+,
//     <CODE> with [A-Z][A-Z0-9]+.
//  3. Anchor with a leading ^ so it matches from the start of a line.
func normalizedToPattern(normalised string) string {
	// Escape regex special chars (< and > are not special, so placeholders
	// survive intact)
	escaped := regexp.QuoteMeta(normalised)
	// Replace placeholder tokens with wildcard regex fragments
	withWildcards := escaped
	withWildcards = strings.ReplaceAll(withWildcards, "<N>", "[\\S]+")
	withWildcards = strings.ReplaceAll(withWildcards, "<PKG>", "[\\S]+")
	withWildcards = strings.ReplaceAll(withWildcards, "<CODE>", "[A-Z][A-Z0-9]+")
	return "^" + withWildcards
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// DiscoverRepeatedNoise scans a set of command output samples and returns a
// ranked list of line-templates that appear frequently enough to be useful
// DROP candidates.
//
// Only templates that appear in MORE THAN ONE sample are included
// (single-occurrence lines are noise-specific, not structural noise). Results
// are sorted descending by hit count, then alphabetically for deterministic
// output.
func DiscoverRepeatedNoise(samples []CommandSample) []NoiseCandidate {
	if len(samples) == 0 {
		return nil
	}

	// Count how many samples contain each normalised line (at least once per
	// sample). Map from normalised form → set of sample indices.
	hitsBySample := make(map[string]map[int]struct{})

	for i := range samples {
		lines := splitLines(samples[i].Output)
		seenInThisSample := make(map[string]struct{})

		for _, raw := range lines {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				continue
			}
			norm := DiscoverNormalizeLine(trimmed)
			if norm == "" {
				continue
			}
			// Count each normalised form once per sample
			if _, seen := seenInThisSample[norm]; seen {
				continue
			}
			seenInThisSample[norm] = struct{}{}

			if _, ok := hitsBySample[norm]; !ok {
				hitsBySample[norm] = make(map[int]struct{})
			}
			hitsBySample[norm][i] = struct{}{}
		}
	}

	var candidates []NoiseCandidate
	for norm, sampleSet := range hitsBySample {
		if len(sampleSet) <= 1 {
			// Must appear in more than one sample
			continue
		}
		candidates = append(candidates, NoiseCandidate{
			Pattern: normalizedToPattern(norm),
			Hits:    len(sampleSet),
		})
	}

	// Sort descending by hits, then alphabetically for deterministic output
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Hits != candidates[j].Hits {
			return candidates[i].Hits > candidates[j].Hits
		}
		return candidates[i].Pattern < candidates[j].Pattern
	})

	return candidates
}

// splitLines splits a string into lines, handling both \n and \r\n.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}
