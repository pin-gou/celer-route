package rtk

import (
	"regexp"
	"strings"
)

// Package-level pre-compiled regex patterns for normalizeLine.
// All compiled at init time via regexp.MustCompile so hot-path
// calls to normalizeLine avoid compilation overhead.
var (
	// ISO-8601 timestamps: 2024-01-15T10:30:00Z, 2024-01-15 10:30:00,
	// 2024-01-15T10:30:00.123Z, 2024-01-15T10:30:00+08:00
	reISOTimestamp = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?`)

	// Bracketed date-time: [2024-01-01 10:00:00]
	reBracketedDT = regexp.MustCompile(`\[\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\]`)

	// Hex strings 6-40 chars: a1b2c3d4e5f6, DeadBeefCafe
	reHexID = regexp.MustCompile(`\b[0-9a-fA-F]{6,40}\b`)

	// Semantic version tokens: v1.2.3, 1.2.3, 1.2.3.4
	reSemVer = regexp.MustCompile(`\bv?\d+\.\d+\.\d+(?:\.\d+)*\b`)

	// Standalone integers: 42, 12345
	reStandaloneInt = regexp.MustCompile(`\b\d+\b`)

	// Repeated whitespace (any whitespace characters)
	reWhitespace = regexp.MustCompile(`\s+`)
)

// GroupingOptions controls the grouping behavior for groupSimilarLines.
type GroupingOptions struct {
	// Threshold is the minimum run length to trigger grouping.
	// Values below 2 are clamped to 2 internally.
	Threshold int
}

// GroupingResult holds the result of grouping similar lines.
type GroupingResult struct {
	// Text is the compressed text with grouped lines replaced by markers.
	Text string
	// Grouped is the total number of lines that were removed by grouping
	// (runLength - 1 for each group that was collapsed).
	Grouped int
}

// normalizeLine applies 7-step normalization to replace volatile bits
// (timestamps, hex IDs, version numbers, integers) with a stable
// placeholder "<N>", collapses repeated whitespace, and trims.
//
// Two lines that normalize to the same string are considered "similar"
// and can be grouped by groupSimilarLines.
//
// Normalization steps (order matters — broadest patterns first):
//  1. ISO-8601-style timestamps: 2024-01-15T10:30:00Z
//  2. Date-time in brackets:     [2024-01-01 10:00:00]
//  3. Hex strings (6-40 chars):  a1b2c3d4e5f6
//  4. Semantic-version tokens:   v1.2.3
//  5. Standalone integers:       42
//  6. Collapse repeated whitespace to single space.
//  7. Trim leading/trailing whitespace.
func normalizeLine(line string) string {
	// 1. ISO timestamps
	s := reISOTimestamp.ReplaceAllString(line, "<N>")
	// 2. Bracketed date-time
	s = reBracketedDT.ReplaceAllString(s, "[<N>]")
	// 3. Hex strings (6-40 chars)
	s = reHexID.ReplaceAllString(s, "<N>")
	// 4. Semantic version tokens
	s = reSemVer.ReplaceAllString(s, "<N>")
	// 5. Standalone integers
	s = reStandaloneInt.ReplaceAllString(s, "<N>")
	// 6. Collapse repeated whitespace
	s = reWhitespace.ReplaceAllString(s, " ")
	// 7. Trim leading/trailing whitespace
	return strings.TrimSpace(s)
}

// groupSimilarLines collapses consecutive lines that normalize to the same
// canonical form (via normalizeLine) into a single representative line plus
// a count marker: "<first line> [rtk:grouped ×N]".
//
// Only consecutive runs of length >= threshold are collapsed (threshold is
// clamped to max(2, value)). Non-similar lines are passed through unchanged.
// The function performs a single O(lines) pass.
func groupSimilarLines(text string, options GroupingOptions) GroupingResult {
	// Clamp threshold to minimum 2.
	threshold := options.Threshold
	if threshold < 2 {
		threshold = 2
	}

	if text == "" {
		return GroupingResult{Text: text, Grouped: 0}
	}

	lines := contentLines(text)
	if len(lines) == 0 {
		return GroupingResult{Text: "", Grouped: 0}
	}

	var output []string
	grouped := 0
	index := 0

	for index < len(lines) {
		line := lines[index]
		normalized := normalizeLine(line)

		// Count consecutive lines that normalize to the same form.
		runLength := 1
		for index+runLength < len(lines) && normalizeLine(lines[index+runLength]) == normalized {
			runLength++
		}

		if runLength >= threshold {
			// Keep the first (representative) line + the count marker.
			output = append(output, line+" [rtk:grouped ×"+itoa(runLength)+"]")
			grouped += runLength - 1
			index += runLength
		} else {
			output = append(output, line)
			index++
		}
	}

	result := strings.Join(output, "\n")
	if hasTrailingNewline(text) {
		result += "\n"
	}
	return GroupingResult{Text: result, Grouped: grouped}
}

// itoa is a small integer-to-string helper without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
