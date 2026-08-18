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
}