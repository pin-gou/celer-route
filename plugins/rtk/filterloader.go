package rtk

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed filters/builtin/*.json
var builtinFiltersFS embed.FS

// FilterLoader loads, validates and matches filters. Match priority is
// project > global > builtin, with a generic-output fallback for unmatched
// shell commands. All rule regexes are validated for ReDoS-proneness at load
// time and cached for reuse across requests.
type FilterLoader struct {
	mu       sync.RWMutex
	config   *Config
	builtins []*Filter
	globals  []*Filter
	projects []*Filter
	generic  *Filter
	// reCache caches compiled rule patterns (pattern -> *regexp.Regexp).
	reCache map[string]*regexp.Regexp
}

// NewFilterLoader creates a loader, loading built-in filters from the
// embedded filesystem. Invalid or ReDoS-prone built-in filters are skipped
// rather than failing startup (fail-open).
func NewFilterLoader(config *Config) *FilterLoader {
	loader := &FilterLoader{
		config:  config,
		reCache: make(map[string]*regexp.Regexp),
	}
	loader.loadBuiltins()
	return loader
}

// loadBuiltins loads all JSON filter files from filters/builtin/ via embed.
func (l *FilterLoader) loadBuiltins() {
	entries, err := builtinFiltersFS.ReadDir("filters/builtin")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := builtinFiltersFS.ReadFile("filters/builtin/" + entry.Name())
		if err != nil {
			continue
		}
		var filter Filter
		if err := json.Unmarshal(data, &filter); err != nil {
			continue
		}
		if filter.Name == "" {
			continue
		}
		if err := l.validateFilter(&filter); err != nil {
			continue
		}
		l.builtins = append(l.builtins, &filter)
		if filter.Name == "generic-output" {
			l.generic = &filter
		}
	}
	// Ensure a generic fallback always exists.
	if l.generic == nil {
		l.generic = &Filter{
			Name: "generic-output",
			Rules: []LineRule{
				{Type: "collapse", Pattern: `^\s*$`},
			},
			Head: 10,
			Tail: 5,
		}
	}
}

// validateFilter compiles and ReDoS-checks every rule pattern in the filter.
func (l *FilterLoader) validateFilter(filter *Filter) error {
	for _, rule := range filter.Rules {
		if rule.Pattern == "" {
			continue
		}
		if isReDoSProne(rule.Pattern) {
			return fmt.Errorf("rtk: filter %q rule %q: ReDoS-prone pattern rejected", filter.Name, rule.Pattern)
		}
		if _, err := l.compilePattern(rule.Pattern); err != nil {
			return fmt.Errorf("rtk: filter %q rule %q: invalid regex: %w", filter.Name, rule.Pattern, err)
		}
	}
	for _, p := range filter.PriorityPatterns {
		if p == "" {
			continue
		}
		if isReDoSProne(p) {
			return fmt.Errorf("rtk: filter %q priority pattern %q: ReDoS-prone pattern rejected", filter.Name, p)
		}
		if _, err := l.compilePattern(p); err != nil {
			return fmt.Errorf("rtk: filter %q priority pattern %q: invalid regex: %w", filter.Name, p, err)
		}
	}
	return nil
}

// compilePattern compiles a regex pattern, caching the result.
func (l *FilterLoader) compilePattern(pattern string) (*regexp.Regexp, error) {
	l.mu.RLock()
	re, ok := l.reCache[pattern]
	l.mu.RUnlock()
	if ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.reCache[pattern] = re
	l.mu.Unlock()
	return re, nil
}

// Match returns the most specific filter for a shell command, or nil for
// non-shell command types. Priority: project > global > builtin, then the
// generic-output fallback for unmatched shell commands.
func (l *FilterLoader) Match(commandType, command string) *Filter {
	if l == nil {
		return nil
	}
	if commandType != "shell" {
		return nil
	}
	if command == "" {
		l.mu.RLock()
		defer l.mu.RUnlock()
		return l.generic
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	if f := matchLongest(l.projects, command); f != nil {
		return f
	}
	if f := matchLongest(l.globals, command); f != nil {
		return f
	}
	if f := matchLongest(l.builtins, command); f != nil {
		return f
	}
	return l.generic
}

// matchLongest returns the filter whose Command is the longest prefix of the
// given command. Filters with an empty Command never match here (they are
// fallbacks handled by the caller).
func matchLongest(filters []*Filter, command string) *Filter {
	var best *Filter
	bestLen := -1
	for _, f := range filters {
		if f == nil || f.Command == "" {
			continue
		}
		if command == f.Command || strings.HasPrefix(command, f.Command) {
			if len(f.Command) > bestLen {
				best = f
				bestLen = len(f.Command)
			}
		}
	}
	return best
}

// ListBuiltinFilters returns a copy of the loaded built-in filters.
func (l *FilterLoader) ListBuiltinFilters() []*Filter {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*Filter, len(l.builtins))
	copy(out, l.builtins)
	return out
}

// ListGlobalFilters returns a copy of the registered global filters.
func (l *FilterLoader) ListGlobalFilters() []*Filter {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*Filter, len(l.globals))
	copy(out, l.globals)
	return out
}

// ListProjectFilters returns a copy of the registered project filters.
func (l *FilterLoader) ListProjectFilters() []*Filter {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*Filter, len(l.projects))
	copy(out, l.projects)
	return out
}

// RegisterProjectFilter registers a project-level filter (highest priority).
// The filter is validated for ReDoS-prone patterns and regex compilability
// before being accepted.
func (l *FilterLoader) RegisterProjectFilter(filter *Filter) error {
	if l == nil {
		return fmt.Errorf("rtk: nil filter loader")
	}
	if filter == nil || filter.Name == "" {
		return fmt.Errorf("rtk: project filter must have a name")
	}
	if err := l.validateFilter(filter); err != nil {
		return err
	}
	l.mu.Lock()
	l.projects = append(l.projects, filter)
	sort.SliceStable(l.projects, func(i, j int) bool {
		return len(l.projects[i].Command) > len(l.projects[j].Command)
	})
	l.mu.Unlock()
	return nil
}

// RegisterGlobalFilter registers a global-level filter (middle priority).
func (l *FilterLoader) RegisterGlobalFilter(filter *Filter) error {
	if l == nil {
		return fmt.Errorf("rtk: nil filter loader")
	}
	if filter == nil || filter.Name == "" {
		return fmt.Errorf("rtk: global filter must have a name")
	}
	if err := l.validateFilter(filter); err != nil {
		return err
	}
	l.mu.Lock()
	l.globals = append(l.globals, filter)
	sort.SliceStable(l.globals, func(i, j int) bool {
		return len(l.globals[i].Command) > len(l.globals[j].Command)
	})
	l.mu.Unlock()
	return nil
}

// isReDoSProne heuristically detects catastrophic-backtracking-prone regex
// patterns: a group containing a quantifier or alternation that is itself
// followed by an unbounded quantifier (e.g. (a+)+, (a|b)+).
func isReDoSProne(pattern string) bool {
	if pattern == "" {
		return false
	}
	// Remove escaped characters and character classes — they are atomic.
	p := stripEscapesAndClasses(pattern)
	if !strings.Contains(p, "(") {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] != '(' {
			continue
		}
		// Find the matching close paren.
		depth := 0
		end := -1
		for j := i; j < len(p); j++ {
			switch p[j] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = j
					j = len(p)
				}
			}
		}
		if end == -1 {
			continue
		}
		content := p[i+1 : end]
		// Is this group followed by an unbounded quantifier?
		if !unboundedQuantifierAfter(p, end+1) {
			i = end
			continue
		}
		// Nested quantifier or alternation under a quantifier → ReDoS-prone.
		if strings.ContainsAny(content, "*+") || strings.Contains(content, "|") {
			return true
		}
		i = end
	}
	return false
}

// stripEscapesAndClasses removes escaped characters and [...] classes so the
// ReDoS scanner only sees structural tokens.
func stripEscapesAndClasses(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++ // skip escaped char
			continue
		}
		if c == '[' {
			i++
			for i < len(s) && s[i] != ']' {
				i++
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// unboundedQuantifierAfter reports whether the token at index i (in p) is an
// unbounded quantifier: *, +, or {n,} without a closing bound.
func unboundedQuantifierAfter(p string, i int) bool {
	if i < 0 || i >= len(p) {
		return false
	}
	switch p[i] {
	case '*', '+':
		return true
	case '{':
		// Look for a comma followed by a closing brace or a second number.
		j := i + 1
		for j < len(p) && (p[j] >= '0' && p[j] <= '9' || p[j] == ' ' || p[j] == '\t') {
			j++
		}
		if j < len(p) && p[j] == '}' {
			return false // {n} exact count — bounded
		}
		if j < len(p) && p[j] == ',' {
			// {n,} unbounded or {n,m} bounded. Check for closing number.
			k := j + 1
			for k < len(p) && (p[k] >= '0' && p[k] <= '9' || p[k] == ' ' || p[k] == '\t') {
				k++
			}
			if k < len(p) && p[k] == '}' {
				return false // {n,m} bounded
			}
			return true // {n,} unbounded
		}
		return false
	}
	return false
}