package rtk

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// SuggestedFilter represents a suggested RTK filter draft in the canonical
// RtkFilterPack shape (same JSON structure as filters/pip.json, etc.) so it
// can be reviewed and saved as a real filter without conversion.
type SuggestedFilter struct {
	ID          string                  `json:"id"`
	Label       string                  `json:"label"`
	Description string                  `json:"description"`
	Category    string                  `json:"category"` // "generic"
	Priority    int                     `json:"priority"` // 50
	Match       SuggestedFilterMatch    `json:"match"`
	Rules       SuggestedFilterRules    `json:"rules"`
	Preserve    SuggestedFilterPreserve `json:"preserve"`
	Tests       []FilterTest            `json:"tests"`
	Meta        SuggestedFilterMeta     `json:"_meta"`
}

// SuggestedFilterMatch defines the match block for a SuggestedFilter.
type SuggestedFilterMatch struct {
	OutputTypes []string `json:"outputTypes"`
	Commands    []string `json:"commands"`
	Patterns    []string `json:"patterns"`
}

// SuggestedFilterRules defines the rules block for a SuggestedFilter.
type SuggestedFilterRules struct {
	StripAnsi        bool     `json:"stripAnsi"`
	DropPatterns     []string `json:"dropPatterns"`
	CollapsePatterns []string `json:"collapsePatterns"`
	IncludePatterns  []string `json:"includePatterns"`
	Deduplicate      bool     `json:"deduplicate"`
	MaxLines         int      `json:"maxLines"`
	HeadLines        int      `json:"headLines"`
	TailLines        int      `json:"tailLines"`
	OnEmpty          string   `json:"onEmpty"`
}

// SuggestedFilterPreserve defines the preserve block for a SuggestedFilter.
type SuggestedFilterPreserve struct {
	ErrorPatterns   []string `json:"errorPatterns"`
	SummaryPatterns []string `json:"summaryPatterns"`
}

// SuggestedFilterMeta contains metadata about the learning run.
type SuggestedFilterMeta struct {
	LearnedFromSamples int `json:"learnedFromSamples"`
	DropThreshold      int `json:"dropThreshold"`
}

// ---------------------------------------------------------------------------
// Internal constants and helpers
// ---------------------------------------------------------------------------

// A normalised line template must appear in at least this fraction of samples
// to be included as a drop pattern. 50% is conservative — it avoids flagging
// lines that happen to appear in only a handful of runs.
const dropThresholdRatio = 0.5

// Matched against the RAW (untrimmed) output line, case-insensitive.
//
// Error heuristic: lines that strongly signal a failure or warning worth
// preserving. "WARN deprecated" (npm deprecation noise) is deliberately
// excluded — it is structural noise, not an actionable error signal.
//
// We match "ERR!" (npm error prefix), "error:" / "error " patterns, etc.
var reErrorPattern = regexp.MustCompile(`(?i)(?:\bERR!|\berror\s*[:/]|\bfailed?\b|\bfailure\b|\bcritical\b|\bexception\b|\bfatal\b|\bpanic\b)`)

// Matched against the RAW output line, case-insensitive.
// Summary heuristic: lines that indicate a successful outcome or final tally.
var reSummaryPattern = regexp.MustCompile(`(?i)(?:\bsuccess(?:ful(?:ly)?)?\b|\bdone\b|\bcomplete(?:d)?\b|\bbuilt\b|\badded\b|\binstalled\b|\bfinished?\b|\bpassed?\b)`)

// CommandToId derives a slug-friendly id from a command string, e.g.:
//
//	"npm install"  → "npm-install"
//	"pip install"  → "pip-install"
func CommandToId(command string) string {
	s := strings.TrimSpace(command)
	s = strings.ToLower(s)
	// Replace runs of non-[a-z0-9] chars with a single "-"
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	// Strip leading/trailing dashes
	s = strings.Trim(s, "-")
	return s
}

// commandToMatchPattern builds a regex anchor pattern for the command so the
// filter's match.commands array targets this specific invocation, e.g.
// "^npm\\s+install\\b".
func commandToMatchPattern(command string) string {
	parts := strings.Fields(strings.TrimSpace(command))
	escaped := make([]string, len(parts))
	for i, p := range parts {
		escaped[i] = regexp.QuoteMeta(p)
	}
	return "^" + strings.Join(escaped, `\s+`) + `\b`
}

// matchesAny tests whether a raw output line (from any sample) is matched by
// any of the given regex patterns. Invalid patterns are silently skipped.
func matchesAny(line string, patterns []string) bool {
	for _, p := range patterns {
		re, err := regexp.Compile(`(?i)` + p)
		if err != nil {
			continue
		}
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// normsToPatterns converts a set of normalised lines into regex patterns
// (same escaping as normalizedToPattern in discover.go, but without the ^
// anchor since preserve patterns are substring-matched).
func normsToPatterns(norms map[string]struct{}) []string {
	if len(norms) == 0 {
		return nil
	}
	// Sort for deterministic output
	sorted := make([]string, 0, len(norms))
	for norm := range norms {
		sorted = append(sorted, norm)
	}
	sort.Strings(sorted)

	patterns := make([]string, len(sorted))
	for i, norm := range sorted {
		escaped := regexp.QuoteMeta(norm)
		withWildcards := escaped
		withWildcards = strings.ReplaceAll(withWildcards, "<N>", "[\\S]+")
		withWildcards = strings.ReplaceAll(withWildcards, "<PKG>", "[\\S]+")
		withWildcards = strings.ReplaceAll(withWildcards, "<CODE>", "[A-Z][A-Z0-9]+")
		patterns[i] = withWildcards
	}
	return patterns
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// SuggestFilter suggests an RTK filter for the given command based on the
// provided samples. The returned object is a valid RtkFilterPack-shaped draft
// that can be saved and loaded by the existing filter loader.
//
// Key design decisions:
//  1. Drop threshold: a normalised line template is included in dropPatterns
//     only if it recurs in ≥ DROP_THRESHOLD_RATIO of samples (default 50%).
//  2. Preserve-vs-drop conflict guard: a candidate drop pattern is silently
//     omitted if it matches ANY line that also matches an error or summary
//     preserve pattern.
//  3. Error/summary heuristics: lines matching ERROR_PATTERN or SUMMARY_PATTERN
//     (against the raw line) are preserved.
func SuggestFilter(command string, samples []CommandSample) SuggestedFilter {
	id := CommandToId(command)
	if id == "" {
		id = "unknown"
	}
	commandPattern := commandToMatchPattern(command)
	totalSamples := len(samples)

	// Empty samples skeleton
	if totalSamples == 0 {
		return SuggestedFilter{
			ID:          fmt.Sprintf("suggested-%s", id),
			Label:       command,
			Description: fmt.Sprintf("Auto-suggested filter for '%s' (0 samples — no rules derived).", command),
			Category:    "generic",
			Priority:    50,
			Match: SuggestedFilterMatch{
				OutputTypes: []string{},
				Commands:    []string{commandPattern},
				Patterns:    []string{},
			},
			Rules: SuggestedFilterRules{
				StripAnsi:        true,
				DropPatterns:     []string{},
				CollapsePatterns: []string{},
				IncludePatterns:  []string{},
				Deduplicate:      true,
				MaxLines:         200,
				HeadLines:        30,
				TailLines:        40,
				OnEmpty:          fmt.Sprintf("%s: ok", id),
			},
			Preserve: SuggestedFilterPreserve{
				ErrorPatterns:   []string{},
				SummaryPatterns: []string{},
			},
			Tests: []FilterTest{},
			Meta: SuggestedFilterMeta{
				LearnedFromSamples: 0,
				DropThreshold:      50,
			},
		}
	}

	// ── Step 1: discover recurring noise candidates ──────────
	noiseCandidates := DiscoverRepeatedNoise(samples)
	dropThresholdHits := int(math.Max(2, math.Ceil(float64(totalSamples)*dropThresholdRatio)))

	// ── Step 2: build error/summary preserve patterns from ALL lines ──
	// Scan every line in every sample and collect normalised forms that look
	// like errors or summaries. Each unique normalised form becomes one pattern.
	errorNorms := make(map[string]struct{})
	summaryNorms := make(map[string]struct{})

	for _, sample := range samples {
		for _, raw := range splitLines(sample.Output) {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				continue
			}
			norm := DiscoverNormalizeLine(trimmed)
			if norm == "" {
				continue
			}

			// Classify using the RAW line (before normalisation) so that
			// textual signals like "ERR!" and "added N packages" are not
			// obscured by placeholder substitutions.
			if reErrorPattern.MatchString(trimmed) {
				errorNorms[norm] = struct{}{}
			} else if reSummaryPattern.MatchString(trimmed) {
				summaryNorms[norm] = struct{}{}
			}
		}
	}

	errorPatterns := normsToPatterns(errorNorms)
	summaryPatterns := normsToPatterns(summaryNorms)
	allPreservePatterns := append([]string{}, errorPatterns...)
	allPreservePatterns = append(allPreservePatterns, summaryPatterns...)

	// ── Step 3: filter noise candidates into safe drop patterns ──
	// A candidate is safe to drop only if:
	//   (a) it recurs in >= dropThresholdHits samples, AND
	//   (b) its pattern does NOT match any preserve (error/summary) line from
	//       any sample (conflict guard).

	// Collect every raw line that any preserve pattern would protect.
	var preservedRawLines []string
	for _, sample := range samples {
		for _, raw := range splitLines(sample.Output) {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				continue
			}
			if matchesAny(trimmed, allPreservePatterns) {
				preservedRawLines = append(preservedRawLines, trimmed)
			}
		}
	}

	var dropPatterns []string
	for _, candidate := range noiseCandidates {
		if candidate.Hits < dropThresholdHits {
			continue // below threshold
		}
		// Conflict guard: skip if the drop pattern matches a preserved line
		conflictsWithPreserve := false
		for _, line := range preservedRawLines {
			re, err := regexp.Compile(`(?i)` + candidate.Pattern)
			if err != nil {
				continue
			}
			if re.MatchString(line) {
				conflictsWithPreserve = true
				break
			}
		}
		if conflictsWithPreserve {
			continue
		}
		dropPatterns = append(dropPatterns, candidate.Pattern)
	}

	// includePatterns = errorPatterns + summaryPatterns
	includePatterns := append([]string{}, errorPatterns...)
	includePatterns = append(includePatterns, summaryPatterns...)

	return SuggestedFilter{
		ID:          fmt.Sprintf("suggested-%s", id),
		Label:       command,
		Description: fmt.Sprintf("Auto-suggested filter for '%s' learned from %d sample(s).", command, totalSamples),
		Category:    "generic",
		Priority:    50,
		Match: SuggestedFilterMatch{
			OutputTypes: []string{},
			Commands:    []string{commandPattern},
			Patterns:    []string{},
		},
		Rules: SuggestedFilterRules{
			StripAnsi:        true,
			DropPatterns:     dropPatterns,
			CollapsePatterns: []string{},
			IncludePatterns:  includePatterns,
			Deduplicate:      true,
			MaxLines:         200,
			HeadLines:        30,
			TailLines:        40,
			OnEmpty:          fmt.Sprintf("%s: ok", id),
		},
		Preserve: SuggestedFilterPreserve{
			ErrorPatterns:   errorPatterns,
			SummaryPatterns: summaryPatterns,
		},
		Tests: []FilterTest{},
		Meta: SuggestedFilterMeta{
			LearnedFromSamples: totalSamples,
			DropThreshold:      50,
		},
	}
}
