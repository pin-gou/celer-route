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

	// EnableRenderers enables semantic renderers (opt-in, default true on fresh install).
	// When true, structured outputs (git diff, test suites, terraform plan,
	// JSON tables) are rewritten to a more compact form after line filtering.
	// Renderers are fail-open — a panic or no-op leaves the original text unchanged.
	enable_renderers: z.boolean().default(true),

	// Renderers is an optional whitelist of detection types whose renderers
	// may run. Empty (default) enables every registered renderer. Aligned
	// with OmniRoute's RtkConfig.renderers.
	renderers: z.array(z.string()).optional(),

	// SnapshotMode controls how compression snapshots are persisted for the
	// log detail view. "off" disables snapshots entirely (default, saves log
	// storage); "split" shows per-message diffs; "merged" concatenates
	// everything into one block.
	snapshot_mode: z.enum(["split", "merged", "off"]).default("off"),

	// SnapshotMaxBytes caps the total bytes persisted per request across all
	// snapshots. Default 30 KiB; clamped at the server to [1 KiB, 256 KiB].
	snapshot_max_bytes: z
		.number()
		.int()
		.min(0)
		.default(30 * 1024),
});

export type RTKConfig = z.infer<typeof rtkConfigSchema>;

// ---------------------------------------------------------------------------
// Governance (built-in plugin) — form schema
// ---------------------------------------------------------------------------

export const OTEL_PLUGIN = "otel";

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

// ---------------------------------------------------------------------------
// Logging (built-in plugin) — form schema
// ---------------------------------------------------------------------------

export const LOGGING_PLUGIN = "logging";

// zod schema for the logging plugin's dedicated config form.
// Mirrors plugins/logging/main.go's Config struct fields.
// All 4 fields are optional — the backend applies defaults for absent fields.
export const loggingConfigSchema = z.object({
	disable_content_logging: z.boolean().optional(),
	retain_content_in_object_storage: z.boolean().optional(),
	allow_per_request_content_storage_override: z.boolean().optional(),
	logging_headers: z.array(z.string()).optional(),
});

export type LoggingConfig = z.infer<typeof loggingConfigSchema>;

// ---------------------------------------------------------------------------
// Semantic Cache (built-in plugin) — form schema
// ---------------------------------------------------------------------------

// zod schema for the semantic_cache plugin's dedicated config form.
// Mirrors plugins/semanticcache/main.go's Config struct fields.
// Uses .refine() for allOf conditional validation:
//   - when provider is set, embedding_model is required and dimension >= 2
//   - dimension is always >= 1
export const semanticCacheConfigSchema = z
	.object({
		provider: z.string().optional(),
		embedding_model: z.string().optional(),
		dimension: z.number().int().min(1).optional(),
		ttl: z.union([z.string(), z.number()]).optional(),
		threshold: z.number().min(0).max(1).optional(),
		vector_store_namespace: z.string().optional(),
		default_cache_key: z.string().optional(),
		conversation_history_threshold: z.number().int().min(0).optional(),
		cache_by_model: z.boolean().default(true),
		cache_by_provider: z.boolean().default(true),
		exclude_system_prompt: z.boolean().default(false),
	})
	.refine(
		(data) => {
			if (!data.provider || data.provider.length === 0) return true;
			// When provider is set, embedding_model must be non-empty
			if (!data.embedding_model || data.embedding_model.length === 0) return false;
			// When provider is set, dimension must be >= 2
			if (data.dimension !== undefined && data.dimension < 2) return false;
			return true;
		},
		{ message: "When provider is set, embedding_model is required and dimension must be >= 2" },
	);

export type SemanticCacheConfig = z.infer<typeof semanticCacheConfigSchema>;

// ---------------------------------------------------------------------------
// Mocker (built-in plugin) — form schema
// ---------------------------------------------------------------------------

export const MOCKER_PLUGIN = "mocker";

// Zod schema for the mocker plugin's config.
// Uses a JSON editor in the UI, so the schema validates the parsed JSON.
export const globalLatencySchema = z.object({
	min: z.string(),
	max: z.string(),
	type: z.enum(["fixed", "uniform"]).optional(),
});

export type GlobalLatency = z.infer<typeof globalLatencySchema>;

export const mockerRuleSchema = z.object({
	name: z.string().optional(),
	conditions: z.record(z.string(), z.unknown()).optional(),
	responses: z
		.array(
			z.object({
				status_code: z.number().int().min(100).max(599).optional(),
				body: z.unknown().optional(),
				headers: z.record(z.string(), z.string()).optional(),
			}),
		)
		.optional(),
	priority: z.number().int().min(-1000).max(1000).optional(),
	probability: z.number().min(0).max(1).optional(),
});

export type MockerRule = z.infer<typeof mockerRuleSchema>;

export const mockerConfigSchema = z.object({
	enabled: z.boolean().optional(),
	global_latency: globalLatencySchema.optional(),
	rules: z.array(mockerRuleSchema).optional(),
	default_behavior: z.enum(["passthrough", "error", "success"]).optional(),
});

export type MockerConfig = z.infer<typeof mockerConfigSchema>;

// ---------------------------------------------------------------------------
// Plugin fragment i18n label mapping
// ---------------------------------------------------------------------------

export const PROMPTS_PLUGIN = "prompts";
export const MODELCATALOGRESOLVER_PLUGIN = "modelcatalogresolver";
export const JSONPARSER_PLUGIN = "jsonparser";

// Maps plugin name → i18n key for the display name in fragment headers.
export const pluginFragmentLabels: Record<string, string> = {
	logging: "pluginNames.logging",
	semantic_cache: "pluginNames.semanticCache",
	mocker: "pluginNames.mocker",
	compat: "pluginNames.compat",
	prompts: "pluginNames.prompts",
	modelcatalogresolver: "pluginNames.modelcatalogresolver",
	jsonparser: "pluginNames.jsonparser",
};

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

// Process-lifetime compression counters surfaced by GET
// /api/context/rtk/stats. Token savings and compression ratio are derived
// server-side so the UI never has to guard against divide-by-zero on a
// freshly-started instance.
export interface RtkStats {
	invocations: number;
	compressedCount: number;
	originalTokens: number;
	compressedTokens: number;
	tokensSaved: number;
	compressionRatio: number;
}

export interface RtkStatsResponse {
	stats: RtkStats;
}

// Time-bucketed histogram of compression stats, returned by
// GET /api/context/rtk/stats/histogram. Used by the dashboard RTK chart.
export interface RtkHistogramBucket {
	timestamp: number;
	invocations: number;
	compressed_count: number;
	original_tokens: number;
	compressed_tokens: number;
	tokens_saved: number;
	compression_ratio: number;
}

export interface RtkStatsHistogramResponse {
	plugin: string;
	buckets: RtkHistogramBucket[];
	bucket_size_seconds: number;
	totals: RtkHistogramBucket;
	lifetime_totals: RtkStats;
}