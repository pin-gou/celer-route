package rtk

import (
	"regexp"
	"strings"
)

// applySmartTruncate truncates text to the head and tail windows specified
// in the filter. Priority patterns cause matching lines in the middle
// section to be preserved even if they fall outside the head/tail windows.
// MaxLines caps the total number of lines in the result.
//
// Head selects the first N lines; Tail selects the last M lines. A value of
// 0 for both means "no truncation" unless MaxLines is set, in which case the
// first MaxLines lines are kept. If head and tail windows overlap, all lines
// are kept.
func applySmartTruncate(input string, filter *Filter) string {
	if input == "" || filter == nil {
		return input
	}

	content := contentLines(input)
	if len(content) == 0 {
		return input
	}

	head := filter.Head
	tail := filter.Tail
	maxLines := filter.MaxLines
	priorityPatterns := filter.PriorityPatterns

	// If neither head nor tail is set, use MaxLines as the head window.
	if head <= 0 && tail <= 0 {
		if maxLines > 0 && maxLines < len(content) {
			head = maxLines
		} else {
			return input
		}
	}

	// If head+tail covers all lines, no truncation is needed.
	if head+tail >= len(content) {
		return input
	}

	headEnd := head
	if headEnd > len(content) {
		headEnd = len(content)
	}
	tailStart := len(content) - tail
	if tailStart < headEnd {
		tailStart = headEnd
	}

	// Mark lines to keep: head window, priority-matching middle lines, tail window.
	kept := make([]bool, len(content))
	for i := 0; i < headEnd; i++ {
		kept[i] = true
	}
	for i := tailStart; i < len(content); i++ {
		kept[i] = true
	}
	for _, pattern := range priorityPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		for i := headEnd; i < tailStart; i++ {
			if re.MatchString(content[i]) {
				kept[i] = true
			}
		}
	}

	result := make([]string, 0, len(content))
	for i, ok := range kept {
		if ok {
			result = append(result, content[i])
		}
	}

	if len(result) == 0 {
		return ""
	}

	// Apply the MaxLines cap if the result still exceeds it.
	if maxLines > 0 && len(result) > maxLines {
		result = result[:maxLines]
	}

	out := strings.Join(result, "\n")
	if hasTrailingNewline(input) {
		out += "\n"
	}
	return out
}