package rtk

import "fmt"

// Config holds the configuration for the RTK (Rule-based Tool-output Kompression) plugin.
// It controls which tool outputs are compressed and how aggressively.
type Config struct {
	// Enabled enables or disables the RTK compression plugin.
	Enabled bool `json:"enabled"`

	// Intensity controls the compression aggressiveness: minimal | standard | aggressive.
	Intensity string `json:"intensity"`

	// ApplyToToolResults controls whether tool result messages are compressed.
	ApplyToToolResults bool `json:"apply_to_tool_results"`

	// ApplyToCodeBlocks controls whether code blocks within messages are compressed.
	ApplyToCodeBlocks bool `json:"apply_to_code_blocks"`

	// MaxLinesPerResult is the maximum number of lines to keep per tool result after compression.
	MaxLinesPerResult int `json:"max_lines_per_result"`

	// MaxCharsPerResult is the maximum number of characters to keep per tool result after compression.
	MaxCharsPerResult int `json:"max_chars_per_result"`

	// DedupThreshold is the number of consecutive identical lines before deduplication kicks in.
	DedupThreshold int `json:"dedup_threshold"`

	// PreserveCacheControl preserves cache_control blocks during compression (Anthropic prompt caching).
	PreserveCacheControl bool `json:"preserve_cache_control"`

	// EnableGrouping enables fuzzy grouping of near-equivalent consecutive lines
	// (lines that differ only by timestamps/hex IDs/numbers/versions).
	EnableGrouping bool `json:"enable_grouping"`

	// GroupingThreshold is the minimum run length of near-equivalent lines before
	// grouping kicks in. Values below 2 are clamped to 2 at runtime.
	GroupingThreshold int `json:"grouping_threshold"`

	// ApplyToAssistantMessages controls whether assistant messages are compressed.
	// When true (and ApplyToCodeBlocks is false), assistant message text is fully
	// compressed. When ApplyToCodeBlocks is true and ApplyToAssistantMessages is
	// false, only the inside of code fences in assistant messages is compressed.
	ApplyToAssistantMessages bool `json:"apply_to_assistant_messages"`

	// CustomFiltersEnabled enables loading of project/global custom filters from
	// the filesystem (default true). When false, only builtin filters are used.
	CustomFiltersEnabled bool `json:"custom_filters_enabled"`

	// TrustProjectFilters bypasses the trust.json SHA256 check for project-level
	// filters (default false). When false, project filters are only loaded when
	// their filters.json SHA256 matches trust.json (or the trust env var is set).
	TrustProjectFilters bool `json:"trust_project_filters"`

	// EnabledFilters whitelists filter IDs (canonical) or names (legacy) to load.
	// Empty means all filters are enabled. Filters are matched by ID first, then Name.
	EnabledFilters []string `json:"enabled_filters"`

	// DisabledFilters blacklists filter IDs (canonical) or names (legacy) from loading.
	// Empty means no filters are disabled. Applied after EnabledFilters.
	DisabledFilters []string `json:"disabled_filters"`

	// RawOutputRetention controls when raw tool outputs are persisted to disk
	// under <appDir>/rtk/raw-output/ for debugging: "never" (default) |
	// "failures" | "always".
	RawOutputRetention string `json:"raw_output_retention"`

	// RawOutputMaxBytes caps the persisted raw output size in UTF-8 bytes
	// (default 1048576, minimum 1024).
	RawOutputMaxBytes int `json:"raw_output_max_bytes"`

	// Pipeline defines the ordered list of compression engines to run.
	// When nil or empty, applyConfigDefaults fills it with [{id:"rtk"}].
	// Each step specifies an engine ID and optional engine-specific config.
	Pipeline []PipelineStep `json:"pipeline,omitempty"`

	// MinTokensToCompress is the minimum estimated request token count
	// required to trigger compression. When > 0 and the estimated tokens
	// are below this threshold, the entire compression pipeline is skipped.
	// 0 means "no minimum threshold, always compress" (the default).
	MinTokensToCompress int `json:"min_tokens_to_compress"`

	// EnableRenderers enables semantic renderers (opt-in, default false).
	// When enabled, the pipeline looks up a renderer by detection.Type and,
	// if one is registered, applies a semantic rewrite (e.g. git-diff
	// strips context lines, test-green collapses an all-green test suite
	// into a single summary line, terraform-plan summarises a plan,
	// structured-table parses JSON arrays into TSV). Renderers are
	// fail-open: a panic or error returns the original text untouched.
	// Aligned with OmniRoute's RtkConfig.enableRenderers.
	EnableRenderers bool `json:"enable_renderers"`

	// Renderers is an optional whitelist of detection types whose renderers
	// may run. Empty (the default) enables every registered renderer. When
	// non-empty, renderers for detection types NOT in this list pass through
	// unchanged. Aligned with OmniRoute's RtkConfig.renderers.
	Renderers []string `json:"renderers,omitempty"`

// SnapshotMode controls how compression snapshots are persisted for the
		// log detail view:
		//   "off"    — disable snapshot persistence entirely (default; saves log storage)
		//   "split"  — per-message diff (recommended for tool output inspection)
		//   "merged" — single combined diff
		// Empty defaults to "off".
	SnapshotMode string `json:"snapshot_mode"`

	// SnapshotMaxBytes caps the total bytes persisted per request across all
	// snapshots. Default 30 KiB, minimum 1 KiB, maximum 256 KiB.
	SnapshotMaxBytes int `json:"snapshot_max_bytes"`
}

// Validate checks the config for valid values and returns an error if any field
// is out of range or invalid. This is called during Init to fail fast on
// misconfiguration, protecting against malicious or accidental bad config.
func (c *Config) Validate() error {
	validIntensities := map[string]bool{"minimal": true, "standard": true, "aggressive": true}
	if c.Intensity != "" && !validIntensities[c.Intensity] {
		return fmt.Errorf("rtk: invalid intensity %q: must be one of minimal, standard, aggressive", c.Intensity)
	}
	if c.MaxLinesPerResult < 0 {
		return fmt.Errorf("rtk: max_lines_per_result must be >= 0, got %d", c.MaxLinesPerResult)
	}
	if c.MaxCharsPerResult < 0 {
		return fmt.Errorf("rtk: max_chars_per_result must be >= 0, got %d", c.MaxCharsPerResult)
	}
	if c.DedupThreshold < 0 {
		return fmt.Errorf("rtk: dedup_threshold must be >= 0, got %d", c.DedupThreshold)
	}
	// The 4 new fields (CustomFiltersEnabled, TrustProjectFilters, EnabledFilters,
	// DisabledFilters) are validated for type only — they can be nil/empty/false.
	// CustomFiltersEnabled and TrustProjectFilters are booleans (no invalid states).
	// EnabledFilters/DisabledFilters are string slices — empty is valid.

	// RawOutputRetention validation: must be one of never, failures, always.
	if c.RawOutputRetention != "" {
		switch c.RawOutputRetention {
		case "never", "failures", "always":
		default:
			return fmt.Errorf("rtk: invalid raw_output_retention %q: must be one of never, failures, always", c.RawOutputRetention)
		}
	}
	// RawOutputMaxBytes validation: negative is invalid, positive but < 1024 is invalid.
	if c.RawOutputMaxBytes < 0 {
		return fmt.Errorf("rtk: raw_output_max_bytes must be >= 0, got %d", c.RawOutputMaxBytes)
	}
	if c.RawOutputMaxBytes > 0 && c.RawOutputMaxBytes < 1024 {
		return fmt.Errorf("rtk: raw_output_max_bytes must be >= 1024 when set, got %d", c.RawOutputMaxBytes)
	}
	// MinTokensToCompress validation: negative is invalid.
	if c.MinTokensToCompress < 0 {
		return fmt.Errorf("rtk: min_tokens_to_compress must be >= 0, got %d", c.MinTokensToCompress)
	}
	// SnapshotMode validation: must be one of split, merged, off, or empty (defaults to off).
	if c.SnapshotMode != "" {
		switch c.SnapshotMode {
		case "split", "merged", "off":
		default:
			return fmt.Errorf("rtk: invalid snapshot_mode %q: must be one of split, merged, off", c.SnapshotMode)
		}
	}
	// SnapshotMaxBytes validation: clamp at apply time, here we only reject negative.
	if c.SnapshotMaxBytes < 0 {
		return fmt.Errorf("rtk: snapshot_max_bytes must be >= 0, got %d", c.SnapshotMaxBytes)
	}
	return nil
}

// applyConfigDefaults fills in zero-value fields with sensible defaults.
func applyConfigDefaults(c *Config) {
	if c.Intensity == "" {
		c.Intensity = "standard"
	}
	if c.MaxLinesPerResult == 0 {
		c.MaxLinesPerResult = 120
	}
	if c.MaxCharsPerResult == 0 {
		c.MaxCharsPerResult = 12000
	}
	if c.DedupThreshold == 0 {
		c.DedupThreshold = 3
	}
	if !c.ApplyToToolResults && !c.ApplyToCodeBlocks {
		c.ApplyToToolResults = true
	}
	// Grouping defaults: zero value → 3 (default off for EnableGrouping).
	if c.GroupingThreshold == 0 {
		c.GroupingThreshold = 3
	} else if c.GroupingThreshold < 2 {
		// Clamp values below 2 to 2, logging a WARN with the original value.
		fmt.Printf("WARN: rtk: grouping_threshold %d is below minimum 2, clamping to 2\n", c.GroupingThreshold)
		c.GroupingThreshold = 2
	}
	// CustomFiltersEnabled defaults to true (design.md). The plain-bool zero
	// value cannot distinguish "explicit false" from "unset", so the defaulting
	// happens in FilterLoader.customFiltersEnabled() at load time: a config that
	// leaves all four custom-filter fields at zero is treated as "defaults
	// enabled". We deliberately do NOT force the field here so an explicit
	// custom_filters_enabled=false in config.json survives to the loader.

	// RawOutputRetention defaults to "never".
	if c.RawOutputRetention == "" {
		c.RawOutputRetention = "never"
	}

	// RawOutputMaxBytes defaults to 1048576.
	if c.RawOutputMaxBytes == 0 {
		c.RawOutputMaxBytes = 1048576
	}

	// Pipeline defaults: nil or empty → [{id:"rtk"}]
	if len(c.Pipeline) == 0 {
		c.Pipeline = []PipelineStep{{ID: "rtk"}}
	}

	// MinTokensToCompress stays at 0 (zero value = no threshold).
	// Do not overwrite an explicit positive value.

	// EnableRenderers stays at false (opt-in). Renderers whitelist stays
	// empty (== all registered renderers enabled when EnableRenderers=true).

	// SnapshotMode defaults to "off" (no snapshot persistence). Opt-in to
	// per-message ("split") or combined ("merged") diffs in the log detail view.
	if c.SnapshotMode == "" {
		c.SnapshotMode = "off"
	}
	// SnapshotMaxBytes default 30 KiB, clamp to [1 KiB, 256 KiB].
	if c.SnapshotMaxBytes == 0 {
		c.SnapshotMaxBytes = 30 * 1024
	} else if c.SnapshotMaxBytes < 1024 {
		c.SnapshotMaxBytes = 1024
	} else if c.SnapshotMaxBytes > 256*1024 {
		c.SnapshotMaxBytes = 256 * 1024
	}
}
