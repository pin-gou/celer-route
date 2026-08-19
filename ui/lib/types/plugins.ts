// Plugins types that match the Go backend structures

import { z } from "zod";

export const SEMANTIC_CACHE_PLUGIN = "semantic_cache";
export const MAXIM_PLUGIN = "maxim";

export type PluginType = "llm" | "mcp" | "http";

export interface PluginStatus {
	name: string;
	status: string;
	logs: string[];
	types: PluginType[];
}

export interface Plugin {
	name: string;
	actualName?: string;
	enabled: boolean;
	config: any;
	isCustom: boolean;
	path?: string;
	status?: PluginStatus;
	placement?: string;
	order?: number;
}

export interface PluginsResponse {
	plugins: Plugin[];
	count: number;
}

export interface CreatePluginRequest {
	name: string;
	path: string;
	enabled: boolean;
	config: any;
	placement?: string;
	order?: number;
}

export interface UpdatePluginRequest {
	enabled: boolean;
	path?: string;
	config?: any;
	placement?: string;
	order?: number;
}

// ---------------------------------------------------------------------------
// RTK (built-in plugin) — form schema
// ---------------------------------------------------------------------------

export const RTK_PLUGIN = "rtk";

// zod schema for the RTK plugin's dedicated config form.
// Mirrors plugins/rtk/config.go's Config struct fields.
export const rtkConfigSchema = z.object({
	// Enabled enables or disables the RTK compression plugin.
	enabled: z.boolean().optional(),

	// Intensity controls the compression aggressiveness.
	intensity: z.enum(["minimal", "standard", "aggressive"]).optional(),

	// ApplyToToolResults controls whether tool result messages are compressed.
	apply_to_tool_results: z.boolean().optional(),

	// ApplyToCodeBlocks controls whether code blocks within messages are compressed.
	apply_to_code_blocks: z.boolean().optional(),

	// MaxLinesPerResult is the maximum number of lines to keep per tool result.
	max_lines_per_result: z.number().int().min(0).optional(),

	// MaxCharsPerResult is the maximum number of characters to keep per tool result.
	max_chars_per_result: z.number().int().min(0).optional(),

	// DedupThreshold is the number of consecutive identical lines before deduplication.
	dedup_threshold: z.number().int().min(0).optional(),

	// PreserveCacheControl preserves cache_control blocks during compression.
	preserve_cache_control: z.boolean().optional(),

	// EnableGrouping enables fuzzy grouping of near-equivalent consecutive lines.
	enable_grouping: z.boolean().optional(),

	// GroupingThreshold is the minimum run length of near-equivalent lines before grouping.
	grouping_threshold: z.number().int().min(0).optional(),

	// ApplyToAssistantMessages controls whether assistant messages are compressed.
	apply_to_assistant_messages: z.boolean().optional(),

	// CustomFiltersEnabled enables loading of project/global custom filters.
	custom_filters_enabled: z.boolean().optional(),

	// TrustProjectFilters bypasses the trust.json SHA256 check for project-level filters.
	trust_project_filters: z.boolean().optional(),

	// EnabledFilters whitelists filter IDs to load.
	enabled_filters: z.array(z.string()).optional(),

	// DisabledFilters blacklists filter IDs from loading.
	disabled_filters: z.array(z.string()).optional(),

	// RawOutputRetention controls when raw tool outputs are persisted to disk.
	raw_output_retention: z.enum(["never", "failures", "always", "all"]).optional(),

	// RawOutputMaxBytes caps the persisted raw output size.
	raw_output_max_bytes: z.number().int().min(0).optional(),

	// Pipeline defines the ordered list of compression engines to run.
	pipeline: z
		.array(
			z.object({
				id: z.string(),
				config: z.unknown().optional(),
			}),
		)
		.default([{ id: "rtk" }]),

	// MinTokensToCompress is the minimum estimated request token count required to trigger compression.
	min_tokens_to_compress: z.number().int().min(0).default(0),

	// EnableRenderers enables semantic renderers (opt-in, default false).
	// When true, structured outputs (git diff, test suites, terraform plan,
	// JSON tables) are rewritten to a more compact form after line filtering.
	// Renderers are fail-open — a panic or no-op leaves the original text unchanged.
	enable_renderers: z.boolean().default(false),

	// Renderers is an optional whitelist of detection types whose renderers
	// may run. Empty (default) enables every registered renderer. Aligned
	// with OmniRoute's RtkConfig.renderers.
	renderers: z.array(z.string()).optional(),
});

export type RTKConfig = z.infer<typeof rtkConfigSchema>;

// ---------------------------------------------------------------------------
// Provider cooldown (built-in plugin) — form schema + monitoring types
// ---------------------------------------------------------------------------

export const PROVIDER_COOLDOWN_PLUGIN = "provider-cooldown";

// zod schema for the provider-cooldown plugin's dedicated config form.
// Mirrors config.schema.json's allOf/if/then rules for name=provider-cooldown:
//   - default_ttl_seconds: integer >= 1 && <= 86400 (1 day)
//   - ttl_overrides:       Record<provider, number >= 1>
//   - quota_patterns:      array of non-empty strings, at least 1
export const providerCooldownConfigSchema = z.object({
	default_ttl_seconds: z.number().int().min(1).max(86400),
	ttl_overrides: z.record(z.string(), z.number().int().min(1)),
	quota_patterns: z.array(z.string().min(1)).min(1),
});

export type ProviderCooldownConfig = z.infer<typeof providerCooldownConfigSchema>;

// One entry in the cooldown state list surfaced by GET
// /api/plugins/provider-cooldown/state.
export interface CooldownStateEntry {
	provider: string;
	keyId: string;
	keyName?: string;
	expireAt: string;
	reason: string;
}

// Lifetime counters + point-in-time active count surfaced by GET
// /api/plugins/provider-cooldown/stats.
export interface CooldownStats {
	markCount: number;
	suppressedCount: number;
	activeCount: number;
}

export interface CooldownStateResponse {
	state: CooldownStateEntry[];
}

export interface CooldownStatsResponse {
	stats: CooldownStats;
}

export interface UnfreezeCooldownResponse {
	message: string;
	provider: string;
	keyId: string;
}