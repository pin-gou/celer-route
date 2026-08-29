package rtk

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Filter defines a set of line-level rules and truncation parameters for
// compressing tool output of a particular command. The struct supports both
// legacy (7-field) and canonical (27-field) JSON formats via a custom
// UnmarshalJSON that arbitrates between equivalent fields.
type Filter struct {
	// === Legacy fields (retained for zero-migration of 53 builtin JSON files) ===
	Name             string     `json:"name,omitempty"`
	Command          string     `json:"command,omitempty"`
	Rules            []LineRule `json:"rules,omitempty"`
	Head             int        `json:"head,omitempty"`
	Tail             int        `json:"tail,omitempty"`
	MaxLines         int        `json:"max_lines,omitempty"`
	PriorityPatterns []string   `json:"priority_patterns,omitempty"`

	// === Canonical fields (aligned with OmniRoute RtkFilterDefinition) ===
	ID          string       `json:"id,omitempty"`
	Label       string       `json:"label,omitempty"`
	Description string       `json:"description,omitempty"`
	Category    string       `json:"category,omitempty"`    // git|test|build|shell|docker|package|infra|cloud|generic
	Priority    int          `json:"priority,omitempty"`    // 0-100, default 50
	Tests       []FilterTest `json:"tests,omitempty"`

	// Canonical match block
	CommandPatterns []string `json:"commandPatterns,omitempty"` // regex
	MatchPatterns   []string `json:"matchPatterns,omitempty"`   // content regex
	OutputTypes     []string `json:"outputTypes,omitempty"`     // shell|api|doc-read

	// Canonical rules block
	StripPatterns    []string          `json:"stripPatterns,omitempty"`    // legacy alias of Rules[strip]
	KeepPatterns     []string          `json:"keepPatterns,omitempty"`     // legacy alias of Rules[keep]
	CollapsePatterns []string          `json:"collapsePatterns,omitempty"` // legacy alias of Rules[collapse]
	StripAnsi        bool              `json:"stripAnsi,omitempty"`
	Replace          []ReplaceRule     `json:"replace,omitempty"`
	MatchOutput      []MatchOutputRule `json:"matchOutput,omitempty"`
	TruncateLineAt   int               `json:"truncateLineAt,omitempty"`
	OnEmpty          string            `json:"onEmpty,omitempty"`
	FilterStderr     bool              `json:"filterStderr,omitempty"`
	Deduplicate      bool              `json:"deduplicate,omitempty"`
	HeadLines        int               `json:"head_lines,omitempty"`  // canonical; arbitration with Head
	TailLines        int               `json:"tail_lines,omitempty"`  // canonical; arbitration with Tail

	// MaxBytes is a per-filter UTF-8 byte ceiling on the post-rule output
	// (after strip/keep/collapse/replace/matchOutput). Aligned with
	// OmniRoute RtkFilterDefinition.maxBytes (rtk TOML max_bytes). When 0,
	// only the global Config.MaxCharsPerResult applies. When > 0, the
	// line filter truncates the post-rule text to at most MaxBytes bytes
	// (UTF-8 safe boundary). Defaults to 0 = no per-filter cap.
	MaxBytes int `json:"maxBytes,omitempty"`

	// Canonical preserve block
	ErrorPatterns   []string `json:"errorPatterns,omitempty"`
	SummaryPatterns []string `json:"summaryPatterns,omitempty"`
}

// FilterTest defines a single test case for a filter, used in the canonical format.
type FilterTest struct {
	Name     string `json:"name"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
	Command  string `json:"command,omitempty"`
}

// ReplaceRule defines a pattern/replacement pair for canonical "replace" rules.
type ReplaceRule struct {
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
}

// MatchOutputRule defines a pattern/message pair for canonical "matchOutput" rules.
type MatchOutputRule struct {
	Pattern string `json:"pattern"`
	Message string `json:"message"`
	Unless  string `json:"unless,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler for dual-format (legacy + canonical)
// Filter JSON. The arbitration rules are:
//  1. head_lines (canonical) > 0 → Head = head_lines; Head > 0 → HeadLines = Head
//  2. tail_lines (canonical) > 0 → Tail = tail_lines; Tail > 0 → TailLines = Tail
//  3. ID empty && Name set → ID = Name; Name empty && ID set → Name = ID
func (f *Filter) UnmarshalJSON(data []byte) error {
	// Use alias to prevent infinite recursion.
	type filterAlias Filter
	alias := (*filterAlias)(f)
	if err := json.Unmarshal(data, alias); err != nil {
		return err
	}

	// Step 2: Arbitrate head/tail/max_lines bidirectionally (canonical wins).
	if f.HeadLines > 0 {
		f.Head = f.HeadLines
	} else if f.Head > 0 {
		f.HeadLines = f.Head
	}

	if f.TailLines > 0 {
		f.Tail = f.TailLines
	} else if f.Tail > 0 {
		f.TailLines = f.Tail
	}

	// Step 3: Arbitrate ID/Name bidirectionally.
	if f.ID == "" && f.Name != "" {
		f.ID = f.Name
	}
	if f.Name == "" && f.ID != "" {
		f.Name = f.ID
	}

	return nil
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

// applyMatchOutputRules checks the filter's MatchOutput rules against the
// input text. When a Pattern matches (and Unless does NOT match), the entire
// input is replaced with the rule's Message. Returns (text, hit) where hit
// indicates whether any rule fired. If no rules fire, returns (input, false)
// unchanged.
//
// Aligned with OmniRoute RtkFilterDefinition.matchOutput semantics: a
// content-level pattern that lets a filter collapse a recognizable output
// (e.g. "Bundle complete!" / "✓ bundle installed") into a single summary line,
// saving 99%+ of tokens for success-only outputs that our detail-oriented
// strip/keep rules cannot compress.
func applyMatchOutputRules(input string, filter *Filter) (string, bool) {
	if filter == nil || len(filter.MatchOutput) == 0 || input == "" {
		return input, false
	}
	for _, rule := range filter.MatchOutput {
		if rule.Pattern == "" {
			continue
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		if !re.MatchString(input) {
			continue
		}
		if rule.Unless != "" {
			unlessRe, err := regexp.Compile(rule.Unless)
			if err == nil && unlessRe.MatchString(input) {
				continue
			}
		}
		return rule.Message, true
	}
	return input, false
}

// applyLineFilter applies the filter's rules to the input text. Rules are
// applied in order: strip → keep → collapse → replace. After the rules, each
// surviving line is truncated to TruncateLineAt runes (when set) and the whole
// output is capped at MaxBytes bytes (when set). If the filter is nil, the
// input is empty, or the filter has no rules/truncation configured, the input
// is returned unchanged.
func applyLineFilter(input string, filter *Filter) string {
	if filter == nil || input == "" {
		return input
	}
	if len(filter.Rules) == 0 && filter.TruncateLineAt <= 0 && filter.MaxBytes <= 0 {
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

	// Build result. When TruncateLineAt is set, cap each surviving line to
	// that many runes (rune-safe so multi-byte characters are never split).
	// This mirrors rtk TOML's truncate_lines_at for wide outputs (ps, jq,
	// gcloud tables, long paths).
	result := make([]string, 0, len(content))
	for i, ok := range kept {
		if ok {
			line := content[i]
			if filter.TruncateLineAt > 0 {
				line = truncateLineToRunes(line, filter.TruncateLineAt)
			}
			result = append(result, line)
		}
	}

	if len(result) == 0 {
		return ""
	}

	out := strings.Join(result, "\n")
	if hasTrailing {
		out += "\n"
	}

	// Apply per-filter UTF-8 byte cap when configured. Trim at a rune
	// boundary so we never emit half a multi-byte character.
	if filter.MaxBytes > 0 && len(out) > filter.MaxBytes {
		out = truncateToMaxBytes(out, filter.MaxBytes)
	}

	return out
}

// truncateLineToRunes returns line truncated to at most n runes, appending an
// ellipsis marker when truncation occurred. Rune-based (not byte-based) so
// multi-byte characters are never split. Mirrors rtk TOML truncate_lines_at.
func truncateLineToRunes(line string, n int) string {
	if n <= 0 {
		return line
	}
	r := []rune(line)
	if len(r) <= n {
		return line
	}
	return string(r[:n]) + "…"
}

// truncateToMaxBytes returns the longest prefix of s whose UTF-8 encoding
// is at most maxBytes bytes. Mirrors the safeUtf8Slice helper in
// rawoutput.go but kept local to avoid an import cycle.
func truncateToMaxBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk back from maxBytes until we land on a non-continuation byte.
	for i := maxBytes; i > 0; i-- {
		if s[i]&0xC0 != 0x80 {
			return s[:i]
		}
	}
	return ""
}