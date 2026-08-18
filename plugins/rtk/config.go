package rtk

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