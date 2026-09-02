// Package rtk — admin endpoints support: Caveman rule catalog.
//
// This file exposes the built-in Caveman rule set as a static catalog so
// the RTK admin UI can render skip_rules as a proper multi-select with
// search, grouping and per-rule descriptions. The catalog is computed once
// from the package-level cavemanRulesEn / cavemanRulesZh slices, both of
// which are immutable after process start, so we cache the result behind
// sync.Once for cheap repeated reads.
//
// Endpoints served through this surface:
//   - GET /api/context/rtk/caveman/rules
//     Returns CavemanRuleCatalog (rules + built-in preserve patterns) for
//     the active plugin instance. No request body required.
package rtk

import (
	"sort"
	"sync"
)

// CavemanRuleCatalogEntry is one row of the rule catalog returned by
// GetCavemanRuleCatalog. The shape is consumed by the RTK admin UI to
// populate the skip_rules multi-select and to render inline tooltips.
type CavemanRuleCatalogEntry struct {
	// Name is the canonical identifier — the value the operator puts in
	// config.caveman.skip_rules to disable this rule.
	Name string `json:"name"`
	// Label is a human-friendly one-liner ("Strip conversational
	// openers and closers like 'thanks', 'happy to'."). Falls back to
	// Name when no description is registered.
	Label string `json:"label"`
	// Category groups the rule for the UI ("filler", "terse",
	// "structural", "context", "dedup", "ultra"). Empty for rules
	// that pre-date the canonical field set.
	Category string `json:"category,omitempty"`
	// Context is the role gate ("all", "user", "assistant", "system").
	Context string `json:"context"`
	// Language tags which language pack the rule belongs to ("en",
	// "zh"). The "all" tag is intentionally absent — language packs
	// are mutually exclusive.
	Language string `json:"language"`
	// MinIntensity is the lowest intensity at which the rule fires
	// ("lite", "full", "ultra").
	MinIntensity string `json:"minIntensity"`
}

// CavemanRuleCatalog is the response shape for
// GET /api/context/rtk/caveman/rules.
type CavemanRuleCatalog struct {
	// Rules is the union of the en and zh rule sets, tagged with the
	// language so the UI can render two groups when the operator's
	// prompts mix English and Chinese.
	Rules []CavemanRuleCatalogEntry `json:"rules"`
	// BuiltInPreservePatterns lists the names of the 17 built-in
	// preserve regions (frontmatter, fenced-code, math-block, ...). The
	// UI surfaces these next to the preserve_patterns input so the
	// operator knows which categories are already protected by default
	// before adding their own regexes.
	BuiltInPreservePatterns []string `json:"builtInPreservePatterns"`
}

// cavemanRuleCatalogOnce guards lazy initialisation of the catalog so
// repeated calls from /api/context/rtk/caveman/rules don't pay the
// construction cost on every request. The rules themselves are static
// package-level state, so the cached slice is safe to share.
var (
	cavemanRuleCatalogOnce sync.Once
	cavemanRuleCatalogVal  CavemanRuleCatalog
)

// GetCavemanRuleCatalog returns a defensive snapshot of the built-in
// Caveman rule catalog. Safe to call on a nil receiver (returns an empty
// catalog). The result is cached after the first successful build; callers
// that need an own copy of the rules slice must deep-copy it themselves.
func (p *Plugin) GetCavemanRuleCatalog() CavemanRuleCatalog {
	// Plugin presence is not required to answer this endpoint — the rule
	// catalog is package-static — but we honour the convention from
	// GetFilterCatalog / PreviewCompression that a nil plugin returns
	// an empty (but still well-shaped) catalog.
	if p == nil {
		return CavemanRuleCatalog{
			Rules:                  []CavemanRuleCatalogEntry{},
			BuiltInPreservePatterns: []string{},
		}
	}

	cavemanRuleCatalogOnce.Do(func() {
		cavemanRuleCatalogVal = buildCavemanRuleCatalog()
	})
	return cavemanRuleCatalogVal
}

// buildCavemanRuleCatalog walks cavemanRulesEn and cavemanRulesZh once and
// produces a stable, JSON-serialisable catalog. The result is memoised by
// GetCavemanRuleCatalog and never mutated afterwards.
func buildCavemanRuleCatalog() CavemanRuleCatalog {
	seen := make(map[string]bool)
	out := make([]CavemanRuleCatalogEntry, 0, len(cavemanRulesEn)+len(cavemanRulesZh))

	for _, r := range cavemanRulesEn {
		if r == nil || seen[r.name] {
			continue
		}
		seen[r.name] = true
		out = append(out, entryFromCompiledRule(r, "en"))
	}
	for _, r := range cavemanRulesZh {
		if r == nil || seen[r.name] {
			continue
		}
		seen[r.name] = true
		out = append(out, entryFromCompiledRule(r, "zh"))
	}

	// Stable order: language first (en before zh), then category, then
	// name. The sort is alphabetic on Category values so the UI groups
	// stay predictable across rebuilds.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Language != out[j].Language {
			return out[i].Language < out[j].Language
		}
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})

	preserve := make([]string, 0, len(preservePatterns))
	for _, p := range preservePatterns {
		preserve = append(preserve, p.name)
	}

	return CavemanRuleCatalog{
		Rules:                  out,
		BuiltInPreservePatterns: preserve,
	}
}

// entryFromCompiledRule projects a compiled cavemanRule into the
// JSON-serialisable catalog entry. Label falls back to the rule name when
// no description is registered.
func entryFromCompiledRule(r *cavemanRule, lang string) CavemanRuleCatalogEntry {
	label := r.description
	if label == "" {
		label = r.name
	}
	return CavemanRuleCatalogEntry{
		Name:         r.name,
		Label:        label,
		Category:     r.category,
		Context:      string(r.context),
		Language:     lang,
		MinIntensity: string(r.minIntensity),
	}
}

// ResetCavemanRuleCatalogCache clears the cached catalog. Test-only —
// production code never needs to invalidate since the underlying rules
// are package-level immutable state.
func ResetCavemanRuleCatalogCache() {
	cavemanRuleCatalogOnce = sync.Once{}
	cavemanRuleCatalogVal = CavemanRuleCatalog{}
}