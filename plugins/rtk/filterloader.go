package rtk

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed filters/builtin/*.json
var builtinFiltersFS embed.FS

// source rank constants for three-tier ordering.
const (
	sourceRankBuiltin  = 1
	sourceRankGlobal   = 2
	sourceRankProject  = 3
	formatRankJSON     = 1
	formatRankTOML     = 2 // recognized but not parsed in stage 3
)

// FilterLoader loads, validates, matches and caches RTK filters from three
// sources (project > global > builtin). Match priority is:
//  1. project filters (rank=3)
//  2. global filters (rank=2)
//  3. builtin filters (rank=1)
//  4. generic-output fallback for unmatched shell commands
//
// All rule regexes are validated for ReDoS-proneness at load time and cached
// for reuse across requests.
type FilterLoader struct {
	mu       sync.RWMutex
	config   *Config
	builtins []*Filter
	globals  []*Filter
	projects []*Filter
	generic  *Filter
	// reCache caches compiled rule patterns (pattern -> *regexp.Regexp).
	reCache map[string]*regexp.Regexp

	// Phase 3 fields: unified cache populated by Load(appDir).
	cachedFilters []*Filter
	diagnostics   []FilterLoadDiagnostic
	appDir        string
}

// NewFilterLoader creates a loader, loading built-in filters from the embedded
// filesystem. Invalid or ReDoS-prone built-in filters are skipped rather than
// failing startup (fail-open). Call Load(appDir) to load project/global sources.
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

// Load implements the three-tier source loading pipeline:
//  1. project source: <appDir>/.rtk/filters.json (trust-checked)
//  2. global source:  <appDir>/rtk/filters.json (auto-trusted)
//  3. builtin source: embed.FS (always trusted)
//
// Sources are parsed, filtered by EnabledFilters/DisabledFilters, sorted by
// (sourceRank desc, formatRank desc, priority desc, id asc), and cached
// in loader.cachedFilters. Diagnostics are collected for each skip/warning.
func (l *FilterLoader) Load(appDir string) error {
	l.mu.Lock()
	l.appDir = appDir
	// Reset cached state before re-loading.
	l.cachedFilters = nil
	l.diagnostics = nil
	l.mu.Unlock()

	allFilters := make([]*Filter, 0)

	// 1. Project source: <appDir>/.rtk/filters.json
	if l.customFiltersEnabled() {
		projectPath := filepath.Join(appDir, ".rtk", "filters.json")
		if data, err := os.ReadFile(projectPath); err == nil {
			trusted, _ := l.projectFiltersTrusted(filepath.Dir(projectPath))
			if trusted {
				filters, diag := l.parseSourceFile("project", projectPath, sourceRankProject, data)
				allFilters = append(allFilters, filters...)
				if diag != nil {
					l.addDiagnostic(diag.Source, diag.Format, diag.Path, diag.Level, diag.Message)
				}
				// Also populate the per-tier list for sourceRankForFilter.
				for _, f := range filters {
					if f != nil {
						l.projects = append(l.projects, f)
					}
				}
			} else {
				// Trust check failed — skip project source.
				// The trust reason is handled inside projectFiltersTrusted which
				// already adds a diagnostic.
			}
		}

		// 2. Global source: <appDir>/rtk/filters.json (auto-trusted)
		globalPath := filepath.Join(appDir, "rtk", "filters.json")
		if data, err := os.ReadFile(globalPath); err == nil {
			filters, diag := l.parseSourceFile("global", globalPath, sourceRankGlobal, data)
			allFilters = append(allFilters, filters...)
			if diag != nil {
				l.addDiagnostic(diag.Source, diag.Format, diag.Path, diag.Level, diag.Message)
			}
			// Also populate the per-tier list for sourceRankForFilter.
			for _, f := range filters {
				if f != nil {
					l.globals = append(l.globals, f)
				}
			}
		}
	}

	// 3. Builtin source: embed.FS (always trusted, always loaded)
	for _, f := range l.builtins {
		allFilters = append(allFilters, f)
	}

	// 4. Apply EnabledFilters / DisabledFilters.
	allFilters = l.applyEnabledDisabled(allFilters)

	// 5. Sort by: sourceRank desc, formatRank desc, priority desc, id asc.
	sort.SliceStable(allFilters, func(i, j int) bool {
		// Compare source rank (descending).
		si := sourceRankForFilter(l, allFilters[i])
		sj := sourceRankForFilter(l, allFilters[j])
		if si != sj {
			return si > sj
		}
		// Compare format rank (descending) — all JSON → rank 1, tie.
		// (No TOML in stage 3, so this is always a tie.)
		// Compare priority (descending).
		if allFilters[i].Priority != allFilters[j].Priority {
			return allFilters[i].Priority > allFilters[j].Priority
		}
		// Compare id (ascending, case-sensitive).
		if allFilters[i].ID != allFilters[j].ID {
			return allFilters[i].ID < allFilters[j].ID
		}
		return allFilters[i].Name < allFilters[j].Name
	})

	l.mu.Lock()
	l.cachedFilters = allFilters
	l.mu.Unlock()
	return nil
}

// sourceRankForFilter returns the source rank for a given filter by checking
// which tier it belongs to. This is a heuristic: we check identity against
// the loader's builtins list, then projects, then globals.
func sourceRankForFilter(l *FilterLoader, f *Filter) int {
	for _, b := range l.builtins {
		if b == f {
			return sourceRankBuiltin
		}
	}
	for _, p := range l.projects {
		if p == f {
			return sourceRankProject
		}
	}
	for _, g := range l.globals {
		if g == f {
			return sourceRankGlobal
		}
	}
	// If not found in any list, it came from a file source. Determine by
	// checking if it has an ID (canonical) field — project/global sources
	// produce filters with IDs set.
	if f.ID != "" {
		// Could be project or global; since Load processes project before
		// global, we rely on the source being embedded in the sort.
		// Default to project rank (conservative) — this case only matters
		// for filters loaded via Load, not RegisterProjectFilter.
		return sourceRankProject
	}
	return sourceRankBuiltin
}

// customFiltersEnabled reports whether project/global custom filter sources
// should be loaded. See design.md: CustomFiltersEnabled defaults to true.
// A plain-bool zero value cannot distinguish "explicit false" from "unset",
// so any configuration signal (TrustProjectFilters, whitelist/blacklist) or
// an all-zero config is treated as enabled. Custom filters are always loaded
// (an explicit custom_filters_enabled=false is noted as a stage-6 refinement).
func (l *FilterLoader) customFiltersEnabled() bool {
	return true
}

// parseSourceFile parses a JSON filter file and returns the filters along with
// an optional diagnostic. TOML files are recognized and skipped with a warning
// (TOML support planned for stage 4).
func (l *FilterLoader) parseSourceFile(source, path string, sourceRank int, data []byte) ([]*Filter, *FilterLoadDiagnostic) {
	// Check for TOML (recognized by extension, not parsed).
	if strings.HasSuffix(path, ".toml") {
		return nil, &FilterLoadDiagnostic{
			Source:  source,
			Format:  "rtk-toml-v1",
			Path:    path,
			Level:   "warning",
			Message: "TOML support planned for stage 4, skipping",
		}
	}

	// Parse JSON array.
	var filters []*Filter
	if err := json.Unmarshal(data, &filters); err != nil {
		return nil, &FilterLoadDiagnostic{
			Source:  source,
			Format:  "omniroute-json",
			Path:    path,
			Level:   "warning",
			Message: fmt.Sprintf("failed to parse filter file: %v", err),
		}
	}

	valid := make([]*Filter, 0, len(filters))
	for _, f := range filters {
		if f == nil {
			continue
		}
		// Skip filters with empty ID and Name (no identifier).
		if f.ID == "" && f.Name == "" {
			l.addDiagnostic(source, "omniroute-json", path, "warning",
				fmt.Sprintf("filter at index %d has no id or name, skipping", len(valid)))
			continue
		}
		// Validate ReDoS.
		if err := l.validateFilter(f); err != nil {
			l.addDiagnostic(source, "omniroute-json", path, "warning", err.Error())
			continue
		}
		valid = append(valid, f)
	}

	return valid, nil
}

// applyEnabledDisabled filters the filters slice by EnabledFilters (whitelist)
// and DisabledFilters (blacklist). Matching is by ID first, then by Name.
// If EnabledFilters is empty, all filters pass. After whitelist, any filter
// whose ID or Name appears in DisabledFilters is removed.
func (l *FilterLoader) applyEnabledDisabled(filters []*Filter) []*Filter {
	if l.config == nil {
		return filters
	}

	enabled := l.config.EnabledFilters
	disabled := l.config.DisabledFilters

	if len(enabled) == 0 && len(disabled) == 0 {
		return filters
	}

	// Build a set of enabled IDs/names.
	enabledSet := make(map[string]bool, len(enabled))
	for _, id := range enabled {
		enabledSet[id] = true
	}

	// Build a set of disabled IDs/names.
	disabledSet := make(map[string]bool, len(disabled))
	for _, id := range disabled {
		disabledSet[id] = true
	}

	out := make([]*Filter, 0, len(filters))
	for _, f := range filters {
		if f == nil {
			continue
		}
		// Check whitelist (if non-empty).
		if len(enabledSet) > 0 {
			if !enabledSet[f.ID] && !enabledSet[f.Name] {
				continue
			}
		}
		// Check blacklist.
		if disabledSet[f.ID] || disabledSet[f.Name] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// projectFiltersTrusted checks whether the project filters at the given
// directory path are trusted to load. The check order is:
//  1. Env var bypass (OMNIROUTE_RTK_TRUST_PROJECT_FILTERS or
//     BIFROST_RTK_TRUST_PROJECT_FILTERS = "1") → trusted, reason "env bypass"
//  2. Config.TrustProjectFilters == true → trusted, reason "config bypass"
//  3. Read <dir>/trust.json, check filtersSha256 (or legacy trustedFiltersSha256)
//     against SHA256 of the filters.json file.
//
// Returns (trusted bool, reason string). When untrusted, a diagnostic is added.
func (l *FilterLoader) projectFiltersTrusted(filtersDir string) (bool, string) {
	// Step 1: Env var bypass.
	if os.Getenv("OMNIROUTE_RTK_TRUST_PROJECT_FILTERS") == "1" || os.Getenv("BIFROST_RTK_TRUST_PROJECT_FILTERS") == "1" {
		l.addDiagnostic("project", "omniroute-json", filtersDir, "info",
			"trust bypassed by env var")
		return true, "env bypass"
	}

	// Step 2: Config bypass.
	if l.config != nil && l.config.TrustProjectFilters {
		return true, "config bypass"
	}

	// Step 3: Read trust.json.
	trustPath := filepath.Join(filtersDir, "trust.json")
	trustData, err := os.ReadFile(trustPath)
	if err != nil {
		l.addDiagnostic("project", "omniroute-json", filtersDir, "warning",
			"project filters untrusted: trust.json missing")
		return false, "untrusted"
	}

	// Parse trust.json.
	var trust struct {
		FiltersSha256        string `json:"filtersSha256"`
		TrustedFiltersSha256 string `json:"trustedFiltersSha256"`
	}
	if err := json.Unmarshal(trustData, &trust); err != nil {
		l.addDiagnostic("project", "omniroute-json", filtersDir, "warning",
			fmt.Sprintf("project filters untrusted: trust.json parse error: %v", err))
		return false, "untrusted"
	}

	// Compute SHA256 of filters.json.
	filtersPath := filepath.Join(filtersDir, "filters.json")
	filtersData, err := os.ReadFile(filtersPath)
	if err != nil {
		l.addDiagnostic("project", "omniroute-json", filtersDir, "warning",
			"project filters untrusted: cannot read filters.json for SHA256 verification")
		return false, "untrusted"
	}
	hash := sha256.Sum256(filtersData)
	hashHex := hex.EncodeToString(hash[:])

	// Check filtersSha256 (primary) then trustedFiltersSha256 (compat).
	expectedHash := trust.FiltersSha256
	if expectedHash == "" {
		expectedHash = trust.TrustedFiltersSha256
	}

	if expectedHash == "" {
		l.addDiagnostic("project", "omniroute-json", filtersDir, "warning",
			"project filters untrusted: trust.json has no filtersSha256 or trustedFiltersSha256 field")
		return false, "untrusted"
	}

	if hashHex != expectedHash {
		l.addDiagnostic("project", "omniroute-json", filtersDir, "warning",
			"project filters SHA256 mismatch, skipping")
		return false, "SHA256 mismatch"
	}

	return true, "trusted"
}

// Match returns the most specific filter for a shell command, or nil for
// non-shell command types. When Load has been called, Match searches the
// unified cachedFilters slice. Otherwise, it falls back to the legacy
// per-tier search (project > global > builtin > generic-output).
//
// Dual-key matching (v2): when commandType is a granular detection type
// (e.g. "git-diff", "test-pytest", "terraform-plan"), Match first searches
// by Filter.CommandPatterns (canonical form) and falls back to name match.
// For legacy "shell" commandType, the original longest-prefix matching is
// used. Non-shell types ("json", "", "unknown", "api") return nil so that
// the caller knows not to apply shell-based line filtering.
//
// Priority: project > global > builtin, then generic-output fallback.
func (l *FilterLoader) Match(commandType, command string) *Filter {
	if l == nil {
		return nil
	}
	// Non-shell types are not matched.
	switch commandType {
	case "", "unknown", "json", "api":
		return nil
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	// Use the unified cache if populated (Load was called).
	if len(l.cachedFilters) > 0 {
		if commandType == "shell" {
			if command == "" {
				return l.generic
			}
			if f := matchLongest(l.cachedFilters, command); f != nil {
				return f
			}
			return l.generic
		}
		// Granular type: try direct ID/Name match, then command-prefix, then generic.
		if f := matchByIDOrName(l.cachedFilters, commandType); f != nil {
			return f
		}
		if command != "" {
			if f := matchLongest(l.cachedFilters, command); f != nil {
				return f
			}
		}
		return l.generic
	}

	// Legacy fallback: per-tier search (only "shell" path).
	if commandType == "shell" {
		if command == "" {
			return l.generic
		}
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
	// Granular type without cache: scan builtins (by ID/Name first, then command).
	if f := matchByIDOrName(l.builtins, commandType); f != nil {
		return f
	}
	if command != "" {
		if f := matchLongest(l.builtins, command); f != nil {
			return f
		}
	}
	return l.generic
}

// matchByIDOrName returns the first filter whose ID or Name matches the
// given key (exact match). For granular types like "git-diff", this lets
// the renderer pipeline dispatch to a filter specifically tailored for
// that detection type.
func matchByIDOrName(filters []*Filter, key string) *Filter {
	if key == "" {
		return nil
	}
	for _, f := range filters {
		if f == nil {
			continue
		}
		if f.ID == key || f.Name == key {
			return f
		}
	}
	return nil
}

// matchLongest returns the filter whose Command is the longest prefix of the
// given command. Filters with an empty Command never match here (they are
// fallbacks handled by the caller).
func matchLongest(filters []*Filter, command string) *Filter {
	var best *Filter
	bestLen := -1
	for _, f := range filters {
		if f == nil {
			continue
		}
		// Legacy path: literal Command prefix.
		if f.Command != "" {
			if command == f.Command || strings.HasPrefix(command, f.Command) {
				if len(f.Command) > bestLen {
					best = f
					bestLen = len(f.Command)
				}
			}
		}
		// Canonical path: regex match against CommandPatterns. Use the longest
		// matching pattern length as the tie-breaker so a specific pattern
		// (`^dotnet\s+build\b`) beats a generic one (`^dotnet\b`). The filter
		// regexes are pre-validated by validateFilter, so a compile error here
		// means a programmer mistake — fall back to skipping this filter.
		for _, p := range f.CommandPatterns {
			if p == "" {
				continue
			}
			re, err := f.commandPatternRe(p)
			if err != nil {
				continue
			}
			if re.MatchString(command) {
				if len(p) > bestLen {
					best = f
					bestLen = len(p)
				}
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

// validateFilter compiles and ReDoS-checks every rule pattern in the filter.
func (l *FilterLoader) validateFilter(filter *Filter) error {
	// Legacy path: Rules + PriorityPatterns.
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
	// Canonical path: CommandPatterns + MatchOutput + Replace. Without this,
	// filters written purely in the 27-field canonical shape slip past ReDoS
	// gating even though their patterns are compiled at runtime by
	// applyLineFilter / applyMatchOutputRules. The UnmarshalJSON step 4
	// materialises StripPatterns/KeepPatterns/CollapsePatterns into Rules
	// above, so they are already covered by the legacy loop.
	for _, p := range filter.CommandPatterns {
		if p == "" {
			continue
		}
		if isReDoSProne(p) {
			return fmt.Errorf("rtk: filter %q commandPatterns %q: ReDoS-prone pattern rejected", filter.Name, p)
		}
		if _, err := l.compilePattern(p); err != nil {
			return fmt.Errorf("rtk: filter %q commandPatterns %q: invalid regex: %w", filter.Name, p, err)
		}
	}
	for _, r := range filter.MatchOutput {
		if r.Pattern == "" {
			continue
		}
		if isReDoSProne(r.Pattern) {
			return fmt.Errorf("rtk: filter %q matchOutput %q: ReDoS-prone pattern rejected", filter.Name, r.Pattern)
		}
		if _, err := l.compilePattern(r.Pattern); err != nil {
			return fmt.Errorf("rtk: filter %q matchOutput %q: invalid regex: %w", filter.Name, r.Pattern, err)
		}
		if r.Unless != "" {
			if isReDoSProne(r.Unless) {
				return fmt.Errorf("rtk: filter %q matchOutput.unless %q: ReDoS-prone pattern rejected", filter.Name, r.Unless)
			}
			if _, err := l.compilePattern(r.Unless); err != nil {
				return fmt.Errorf("rtk: filter %q matchOutput.unless %q: invalid regex: %w", filter.Name, r.Unless, err)
			}
		}
	}
	for _, r := range filter.Replace {
		if r.Pattern == "" {
			continue
		}
		if isReDoSProne(r.Pattern) {
			return fmt.Errorf("rtk: filter %q replace %q: ReDoS-prone pattern rejected", filter.Name, r.Pattern)
		}
		if _, err := l.compilePattern(r.Pattern); err != nil {
			return fmt.Errorf("rtk: filter %q replace %q: invalid regex: %w", filter.Name, r.Pattern, err)
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