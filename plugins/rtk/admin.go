// Package rtk — admin endpoints support.
//
// This file holds the admin-time entry points used by transports/celer-route-http/handlers/rtk.go:
//   - GetFilterCatalog:    exposes the loader's filters + diagnostics for /api/context/rtk/filters
//   - RunTest:             runs a compression trial against a payload for /api/context/rtk/test
//   - PreviewCompression:  previews compression against a payload without mutating runtime config
//
// The functions are pure read/transform helpers — they never mutate runtime state, never
// touch sync.Map (per-request state), and never produce raw-output pointers that would
// escape to disk unless RawOutputRetention is explicitly enabled and the test actually
// compressed the input. Operators can call them safely while the plugin is live.
package rtk

import (
	"sort"
)

// FilterCatalogEntry is one row of /api/context/rtk/filters. It is the JSON
// shape returned to operators inspecting the loaded filter corpus.
type FilterCatalogEntry struct {
	// ID is the canonical filter identifier. Empty when the underlying filter
	// only carries the legacy Name field.
	ID string `json:"id"`
	// Label is the human-friendly filter name (may match ID for canonical filters).
	Label string `json:"label"`
	// Description is the optional human-readable description loaded from the JSON.
	Description string `json:"description,omitempty"`
	// Category is one of: git | test | build | shell | docker | package | infra | cloud | generic.
	// Empty when the filter predates the canonical field set.
	Category string `json:"category,omitempty"`
	// Source identifies where the filter was loaded from: builtin | project | global.
	Source string `json:"source"`
	// Priority is the filter's priority (0-100, default 50).
	Priority int `json:"priority"`
	// CommandPatterns are the canonical match patterns (canonical filters only).
	CommandPatterns []string `json:"commandPatterns,omitempty"`
	// MatchPatterns are the content-match patterns (canonical filters only).
	MatchPatterns []string `json:"matchPatterns,omitempty"`
	// TestsCount is the number of inline tests attached to this filter.
	TestsCount int `json:"testsCount"`
	// HasOnEmpty is true when the filter defines an OnEmpty replacement message.
	HasOnEmpty bool `json:"hasOnEmpty"`
}

// FilterCatalog is the response shape for /api/context/rtk/filters.
type FilterCatalog struct {
	Filters     []FilterCatalogEntry    `json:"filters"`
	Diagnostics []FilterLoadDiagnostic  `json:"diagnostics"`
	Counters    map[string]int          `json:"counters"`
}

// GetFilterCatalog returns a defensive snapshot of the loader's current filter
// corpus plus the diagnostics captured during Load. Safe to call on a nil
// receiver (returns an empty catalog) and on a Plugin whose loader has not
// been initialised yet.
func (p *Plugin) GetFilterCatalog() FilterCatalog {
	if p == nil || p.loader == nil {
		return FilterCatalog{
			Filters:     []FilterCatalogEntry{},
			Diagnostics: []FilterLoadDiagnostic{},
			Counters:    map[string]int{"builtin": 0, "project": 0, "global": 0, "total": 0},
		}
	}

	l := p.loader
	l.mu.RLock()
	rawFilters := l.cachedFilters
	if len(rawFilters) == 0 {
		// Fall back to builtins so the catalog is non-empty even before Load().
		rawFilters = l.builtins
	}
	l.mu.RUnlock()

	entries := make([]FilterCatalogEntry, 0, len(rawFilters))
	counters := map[string]int{"builtin": 0, "project": 0, "global": 0, "total": 0}

	for _, f := range rawFilters {
		if f == nil {
			continue
		}
		entry := FilterCatalogEntry{
			ID:              f.ID,
			Label:           nonEmpty(f.Label, f.Name, f.ID),
			Description:     f.Description,
			Category:        f.Category,
			Source:          sourceForFilter(l, f),
			Priority:        f.Priority,
			CommandPatterns: append([]string(nil), f.CommandPatterns...),
			MatchPatterns:   append([]string(nil), f.MatchPatterns...),
			TestsCount:      len(f.Tests),
			HasOnEmpty:      f.OnEmpty != "",
		}
		if entry.Priority == 0 {
			entry.Priority = 50
		}
		entries = append(entries, entry)
		counters[entry.Source]++
		counters["total"]++
	}

	// Stable order: by source (builtin first, then global, then project), then ID.
	sort.SliceStable(entries, func(i, j int) bool {
		rankI := sourceRank(entrySourceRank(entries[i].Source))
		rankJ := sourceRank(entrySourceRank(entries[j].Source))
		if rankI != rankJ {
			return rankI < rankJ
		}
		if entries[i].ID != entries[j].ID {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].Label < entries[j].Label
	})

	return FilterCatalog{
		Filters:     entries,
		Diagnostics: l.Diagnostics(),
		Counters:    counters,
	}
}

// entrySourceRank returns the numeric sort rank for a FilterCatalogEntry.Source string.
func entrySourceRank(source string) int {
	switch source {
	case "builtin":
		return 1
	case "global":
		return 2
	case "project":
		return 3
	default:
		return 4
	}
}

// sourceRank maps an entry source string to its sort key.
func sourceRank(r int) int { return r }

// sourceForFilter returns the source tier string for a given *Filter pointer.
// Mirrors sourceRankForFilter but produces the string label directly so we
// don't have to walk the loader tier lists twice.
func sourceForFilter(l *FilterLoader, f *Filter) string {
	if l == nil || f == nil {
		return "builtin"
	}
	for _, b := range l.builtins {
		if b == f {
			return "builtin"
		}
	}
	for _, p := range l.projects {
		if p == f {
			return "project"
		}
	}
	for _, g := range l.globals {
		if g == f {
			return "global"
		}
	}
	// Custom filters loaded via Load() may not be in the tier lists if they
	// were rejected by applyEnabledDisabled; default to project (conservative).
	if f.ID != "" {
		return "project"
	}
	return "builtin"
}

// nonEmpty returns the first non-empty string among vals. Used to coalesce
// legacy Name + canonical ID/Label into the catalog Label field.
func nonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// /api/context/rtk/test
// ---------------------------------------------------------------------------

// TestPayload is the request body for /api/context/rtk/test.
type TestPayload struct {
	// Command is an optional explicit command hint that overrides the
	// content-based detector (e.g. "git status"). When empty, the detector
	// runs against Output to determine the command.
	Command string `json:"command"`
	// Output is the text to compress (typically a simulated tool response).
	Output string `json:"output"`
	// ApplyRules when false skips the filter line-filtering step and only runs
	// the universal pipeline (ANSI strip + dedup + grouping + char limit).
	ApplyRules bool `json:"applyRules"`
}

// TestResult is the response shape for /api/context/rtk/test.
type TestResult struct {
	OriginalText     string               `json:"originalText"`
	CompressedText   string               `json:"compressedText"`
	OriginalTokens   int                  `json:"originalTokens"`
	CompressedTokens int                  `json:"compressedTokens"`
	CompressionRatio float64              `json:"compressionRatio"`
	// FilterMatched is the ID or Name of the filter selected for this payload.
	// Empty when no filter matched (generic fallback) or when ApplyRules is false.
	FilterMatched string `json:"filterMatched,omitempty"`
	// Techniques lists the compression stages that fired (e.g. "dedup",
	// "linefilter", "smarttruncate", "rtk-grouping").
	Techniques []string `json:"techniques"`
	// RawOutputPtr is set only when the active retention policy would persist
	// the raw output; useful so the UI can immediately link to the viewer.
	RawOutputPtr *RtkRawOutputPointer `json:"rawOutputPtr,omitempty"`
	// Stats mirrors the per-stage ProcessStats for diagnostic display.
	Stats *ProcessStats `json:"stats,omitempty"`
}

// RunTest runs the compression pipeline against payload.Output without
// mutating any per-request or runtime state. It is safe to call from a
// request handler — it does not write to sync.Map, does not write to disk
// unless retention explicitly requests it, and never returns the input
// longer than what the pipeline produced.
func (p *Plugin) RunTest(payload TestPayload) TestResult {
	res := TestResult{
		OriginalText: payload.Output,
		Techniques:   []string{},
	}

	if p == nil || p.config == nil {
		res.OriginalTokens = estimateTokens(payload.Output)
		res.CompressedTokens = res.OriginalTokens
		res.CompressedText = payload.Output
		res.CompressionRatio = 0
		return res
	}

	if payload.Output == "" {
		res.CompressedText = ""
		res.OriginalTokens = 0
		res.CompressedTokens = 0
		return res
	}

	// Build a shallow copy of the config so we can disable ApplyRules
	// without mutating the live plugin config. We restore it via defer.
	cfgCopy := *p.config
	savedApplyToolResults := cfgCopy.ApplyToToolResults
	savedApplyCode := cfgCopy.ApplyToCodeBlocks
	savedApplyAssistant := cfgCopy.ApplyToAssistantMessages
	if !payload.ApplyRules {
		// ApplyRules=false means "no line filters"; we still run the rest of
		// the pipeline (dedup/grouping/smart-truncate/char limit) by
		// clearing any filter-driven config fields that would otherwise
		// cause the loader to skip the filter step. We do this by routing
		// through processRtkTextWithCommand with a nil loader, which makes
		// it fall back to "no filter" and apply only the safe steps.
		defer func() {
			cfgCopy.ApplyToToolResults = savedApplyToolResults
			cfgCopy.ApplyToCodeBlocks = savedApplyCode
			cfgCopy.ApplyToAssistantMessages = savedApplyAssistant
		}()
	}

	res.OriginalTokens = estimateTokens(payload.Output)
	compressed, stats := processRtkTextWithCommand(nil, payload.Output, &cfgCopy, p.loader, payload.Command, "")

	if stats != nil {
		res.CompressedTokens = stats.CompressedTokens
		res.Techniques = append(res.Techniques, stats.Techniques...)
		// Stats are returned for diagnostic visibility but the pointer itself
		// is intentionally not persisted to disk here — see MaybePersistRtkRawOutput
		// inside the pipeline for the only path that writes files.
		statsCopy := *stats
		res.Stats = &statsCopy
	}
	if res.CompressedTokens == 0 {
		res.CompressedTokens = estimateTokens(compressed)
	}
	res.CompressedText = compressed
	if res.OriginalTokens > 0 {
		res.CompressionRatio = 1.0 - float64(res.CompressedTokens)/float64(res.OriginalTokens)
		if res.CompressionRatio < 0 {
			res.CompressionRatio = 0
		}
	}

	// FilterMatched is set only when the test actually applied a filter —
	// either an explicit one or the generic fallback.
	if payload.ApplyRules && p.loader != nil {
		det := defaultDetector.detect(stripANSI(payload.Output), payload.Command)
		if det.Command == "" && payload.Command != "" {
			det.Command = payload.Command
		}
		if matched := p.loader.Match(det.Type, det.Command); matched != nil {
			res.FilterMatched = nonEmpty(matched.ID, matched.Name)
		}
	}

	// Populate the raw-output pointer when retention is enabled. This
	// mirrors the production PreLLMHook path so the test accurately
	// represents what an actual request would do.
	if p.config.RawOutputRetention != "" && p.config.RawOutputRetention != string(RawOutputRetentionNever) &&
		stats != nil && len(stats.RawOutputPointers) > 0 {
		res.RawOutputPtr = stats.RawOutputPointers[0]
	}

	return res
}

// ---------------------------------------------------------------------------
// /api/compression/preview
// ---------------------------------------------------------------------------

// CompressionMode selects the preview strategy.
type CompressionMode string

const (
	// CompressionModeRTK runs a single RTK pass.
	CompressionModeRTK CompressionMode = "rtk"
	// CompressionModeStacked runs the configured Pipeline as a stacked
	// pipeline. Today the only registered engine is "rtk"; future caveman
	// engines will appear as additional stages.
	CompressionModeStacked CompressionMode = "stacked"
	// CompressionModeOff returns the payload unchanged (baseline).
	CompressionModeOff CompressionMode = "off"
)

// PreviewRequest is the request body for /api/compression/preview.
type PreviewRequest struct {
	// Mode selects which pipeline to preview. Empty defaults to "rtk".
	Mode CompressionMode `json:"mode"`
	// Payload is the same shape as the /test endpoint.
	Payload TestPayload `json:"payload"`
	// Intensity optionally overrides the plugin's configured intensity
	// ("minimal" | "standard" | "aggressive"). Empty leaves the runtime
	// value untouched.
	Intensity string `json:"intensity,omitempty"`
}

// PreviewResponse is the response shape for /api/compression/preview.
type PreviewResponse struct {
	Mode            CompressionMode    `json:"mode"`
	Result          TestResult         `json:"result"`
	EngineStats     []EngineBreakdown  `json:"engineStats,omitempty"`
	OriginalConfig  *Config            `json:"originalConfig,omitempty"`
	EffectiveConfig *Config            `json:"effectiveConfig,omitempty"`
	// EnginesPlanned lists the engine IDs the configured pipeline intends to
	// run. Today only ["rtk"] is registered; future stages will append here
	// so callers can distinguish "planned but not yet implemented" from
	// "actually executed".
	EnginesPlanned []string `json:"enginesPlanned,omitempty"`
}

// PreviewCompression evaluates the configured compression strategy against
// a payload without modifying the live plugin. The OriginalConfig is a
// snapshot of the runtime config; EffectiveConfig reflects any in-request
// overrides (e.g. intensity). The Result field is always populated, even
// when Mode is "off" (in which case compressedText == originalText).
func (p *Plugin) PreviewCompression(req PreviewRequest) PreviewResponse {
	mode := req.Mode
	if mode == "" {
		mode = CompressionModeRTK
	}

	resp := PreviewResponse{
		Mode:           mode,
		Result:         TestResult{Techniques: []string{}},
		EnginesPlanned: []string{"rtk"},
	}

	if p == nil || p.config == nil {
		// Plugin not loaded — behave like "off".
		resp.Result.OriginalText = req.Payload.Output
		resp.Result.CompressedText = req.Payload.Output
		resp.Result.OriginalTokens = estimateTokens(req.Payload.Output)
		resp.Result.CompressedTokens = resp.Result.OriginalTokens
		resp.Mode = CompressionModeOff
		return resp
	}

	// Snapshot the runtime config so the caller can see what would have
	// applied without mutating it.
	orig := *p.config
	resp.OriginalConfig = &orig

	// Apply intensity override on a local copy.
	effective := orig
	if req.Intensity != "" {
		effective.Intensity = req.Intensity
	}
	resp.EffectiveConfig = &effective

	// "off" mode short-circuits without touching the pipeline.
	if mode == CompressionModeOff {
		resp.Result.OriginalText = req.Payload.Output
		resp.Result.CompressedText = req.Payload.Output
		resp.Result.OriginalTokens = estimateTokens(req.Payload.Output)
		resp.Result.CompressedTokens = resp.Result.OriginalTokens
		return resp
	}

	// Run through the pipeline runner so the preview reflects the same
	// engine chain that PreLLMHook would invoke.
	loader := p.loader
	if loader == nil {
		loader = NewFilterLoader(&effective)
	}

	runner := NewPipelineRunner(globalCatalog)
	pipeline := &Pipeline{Engines: []string{"rtk"}}
	// Pass the payload's command hint through to the engine so the preview
	// routes identically to the real PreLLMHook path (which forwards the
	// extracted command from the tool call). Without this, preview would
	// fall back to pure content detection and diverge from production for
	// commands whose output carries no recognisable content signature.
	resultText, breakdown, _, _, _, _ := runner.Run(nil, pipeline, req.Payload.Output, EngineConfig{
		Enabled:     true,
		CommandHint: req.Payload.Command,
	})

	resp.EngineStats = breakdown
	resp.Result.OriginalText = req.Payload.Output
	resp.Result.CompressedText = resultText
	resp.Result.OriginalTokens = estimateTokens(req.Payload.Output)
	resp.Result.CompressedTokens = estimateTokens(resultText)
	if resp.Result.OriginalTokens > 0 {
		ratio := 1.0 - float64(resp.Result.CompressedTokens)/float64(resp.Result.OriginalTokens)
		if ratio < 0 {
			ratio = 0
		}
		resp.Result.CompressionRatio = ratio
	}

	// If the mode is "stacked", populate EnginesPlanned with whatever the
	// configured pipeline declares. Today only "rtk" exists; the field is
	// forward-compatible with future caveman / llmlingua engines.
	if mode == CompressionModeStacked && len(orig.Pipeline) > 0 {
		ids := make([]string, 0, len(orig.Pipeline))
		for _, step := range orig.Pipeline {
			if step.ID != "" {
				ids = append(ids, step.ID)
			}
		}
		if len(ids) > 0 {
			resp.EnginesPlanned = ids
		}
	}

	// Match the filter that would have been selected so the UI can label
	// the preview result.
	if loader != nil {
		det := defaultDetector.detect(stripANSI(req.Payload.Output), req.Payload.Command)
		if det.Command == "" && req.Payload.Command != "" {
			det.Command = req.Payload.Command
		}
		if matched := loader.Match(det.Type, det.Command); matched != nil {
			resp.Result.FilterMatched = nonEmpty(matched.ID, matched.Name)
		}
	}

	return resp
}