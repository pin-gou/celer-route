package rtk

import (
	"fmt"
	"strings"
)

// applyDedup collapses consecutive identical lines when the number of
// consecutive duplicates reaches or exceeds the threshold. A run of k >=
// threshold identical lines collapses to a single line plus the markers
// "[line repeated Nx]" and "[rtk:dropped N repeated lines]" (N = k-1); a run
// of k < threshold is left untouched. The second return value is the total
// number of collapsed (dropped) lines.
//
// Example with threshold=3:
//
//	Input:  "A\nA\nA\nA\nB\n"
//	Output: "A\n[line repeated 3x]\n[rtk:dropped 3 repeated lines]\nB\n", 3
func applyDedup(input string, threshold int) (string, int) {
	if input == "" || threshold <= 1 {
		return input, 0
	}

	content := contentLines(input)
	if len(content) <= 1 {
		return input, 0
	}

	var result []string
	collapsed := 0
	runStart := 0
	for i := 1; i <= len(content); i++ {
		// Extend the run while consecutive lines are identical.
		if i < len(content) && content[i] == content[runStart] {
			continue
		}
		// Run is content[runStart:i].
		runLen := i - runStart
		if runLen >= threshold {
			// Collapse the whole run to its first line, appending markers.
			n := runLen - 1
			collapsed += n
			result = append(result, content[runStart])
			result = append(result, fmt.Sprintf("[line repeated %dx]", n))
			result = append(result, fmt.Sprintf("[rtk:dropped %d repeated lines]", n))
		} else {
			result = append(result, content[runStart:i]...)
		}
		runStart = i
	}

	if len(result) == 0 {
		return "", collapsed
	}

	out := strings.Join(result, "\n")
	if hasTrailingNewline(input) {
		out += "\n"
	}
	return out, collapsed
}

// contentLines splits a string into its content lines, dropping the empty
// segment produced by a trailing newline. Returns nil for an empty input.
func contentLines(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	if hasTrailingNewline(s) && len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// splitIntoLines is retained as a thin wrapper for callers that want the raw
// split (including the trailing empty segment).
func splitIntoLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// hasTrailingNewline returns true if the string ends with a newline.
func hasTrailingNewline(s string) bool {
	return len(s) > 0 && s[len(s)-1] == '\n'
}