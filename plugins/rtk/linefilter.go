package rtk

import (
	"regexp"
	"strings"
)

// Filter defines a set of line-level rules and truncation parameters for
// compressing tool output of a particular command.
type Filter struct {
	// Name is a human-readable identifier for this filter (e.g. "git-status").
	Name string `json:"name"`

	// Command is the command string this filter matches (e.g. "git status").
	// Empty string matches all commands (generic fallback).
	Command string `json:"command,omitempty"`

	// Rules are the line-level processing rules applied in order:
	// strip → keep → collapse → replace.
	Rules []LineRule `json:"rules,omitempty"`

	// Head is the number of lines to keep from the start of the output.
	Head int `json:"head,omitempty"`

	// Tail is the number of lines to keep from the end of the output.
	Tail int `json:"tail,omitempty"`

	// MaxLines caps the total number of lines after truncation.
	MaxLines int `json:"max_lines,omitempty"`

	// PriorityPatterns are regex patterns that rescue matching lines from
	// the truncated middle section.
	PriorityPatterns []string `json:"priority_patterns,omitempty"`
}

// LineRule defines a single line-level processing rule.
type LineRule struct {
	// Type is the rule type: "strip", "keep", "collapse", or "replace".
	Type string `json:"type"`

	// Pattern is the regex pattern for matching lines.
	Pattern string `json:"pattern,omitempty"`

	// Replacement is the replacement text for "replace" rules.
	Replacement string `json:"replacement,omitempty"`
}

// applyLineFilter applies the filter's rules to the input text. Rules are
// applied in order: strip → keep → collapse → replace. If the filter is nil
// or has no rules, the input is returned unchanged.
func applyLineFilter(input string, filter *Filter) string {
	if filter == nil || len(filter.Rules) == 0 || input == "" {
		return input
	}

	content := contentLines(input)
	if len(content) == 0 {
		return input
	}

	hasTrailing := hasTrailingNewline(input)

	// Track which lines are kept.
	kept := make([]bool, len(content))
	for i := range kept {
		kept[i] = true
	}

	// Compile and apply each rule in order.
	for _, rule := range filter.Rules {
		if rule.Pattern == "" {
			continue
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}

		switch rule.Type {
		case "strip":
			for i, line := range content {
				if kept[i] && re.MatchString(line) {
					kept[i] = false
				}
			}

		case "keep":
			for i, line := range content {
				if kept[i] && !re.MatchString(line) {
					kept[i] = false
				}
			}

		case "collapse":
			prevBlank := false
			for i, line := range content {
				if !kept[i] {
					continue
				}
				isBlank := re.MatchString(line)
				if isBlank && prevBlank {
					kept[i] = false
				}
				prevBlank = isBlank
			}

		case "replace":
			for i, line := range content {
				if kept[i] {
					content[i] = re.ReplaceAllString(line, rule.Replacement)
				}
			}
		}
	}

	// Build result.
	result := make([]string, 0, len(content))
	for i, ok := range kept {
		if ok {
			result = append(result, content[i])
		}
	}

	if len(result) == 0 {
		return ""
	}

	out := strings.Join(result, "\n")
	if hasTrailing {
		out += "\n"
	}

	return out
}