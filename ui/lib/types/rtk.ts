// RTK admin API types.
//
// These types mirror the Go handler at transports/celer-route-http/handlers/rtk.go
// and the plugin admin methods at plugins/rtk/admin.go. They are kept in a
// dedicated file so the RTK config form (plugins.ts) and the RTK admin UI
// (ui/app/workspace/plugins/rtk/) can both reference them without creating
// a circular import.

export type CompressionMode = "rtk" | "stacked" | "off";

export type CompressionTechnique =
	| "linefilter"
	| "dedup"
	| "smarttruncate"
	| "charlimit"
	| "rtk-grouping"
	| "rtk-render"
	| "codeFence"
	| "pipeline-runner";

export type FilterSource = "builtin" | "project" | "global";
export type FilterCategory = "git" | "test" | "build" | "shell" | "docker" | "package" | "infra" | "cloud" | "generic";

export interface RtkRawOutputPointer {
	id: string;
	path: string;
	bytes: number;
	sha256: string;
	redacted: boolean;
}

export interface RtkProcessStats {
	originalTokens: number;
	compressedTokens: number;
	techniques: string[];
	rawOutputPointers: RtkRawOutputPointer[];
}

export interface FilterCatalogEntry {
	id: string;
	label: string;
	description?: string;
	category?: string;
	source: FilterSource;
	priority: number;
	command_patterns?: string[];
	match_patterns?: string[];
	tests_count: number;
	has_on_empty: boolean;
}

export interface FilterLoadDiagnostic {
	source: string;
	format: string;
	path: string;
	level: "info" | "warning" | "error";
	message: string;
}

export interface FilterCatalog {
	filters: FilterCatalogEntry[];
	diagnostics: FilterLoadDiagnostic[];
	counters: {
		builtin?: number;
		project?: number;
		global?: number;
		total?: number;
	};
}

// ---------------------------------------------------------------------------
// /api/context/rtk/config
// ---------------------------------------------------------------------------

export interface RtkConfigResponse {
	enabled: boolean;
	config: import("./plugins").RTKConfig;
}

export interface PutRtkConfigRequest {
	enabled?: boolean;
	config: import("./plugins").RTKConfig;
}

// ---------------------------------------------------------------------------
// /api/context/rtk/test
// ---------------------------------------------------------------------------

export interface TestPayload {
	command?: string;
	output: string;
	applyRules?: boolean;
}

export interface TestResult {
	originalText: string;
	compressedText: string;
	originalTokens: number;
	compressedTokens: number;
	compressionRatio: number;
	filterMatched?: string;
	techniques: string[];
	rawOutputPtr?: RtkRawOutputPointer;
	stats?: RtkProcessStats;
}

// ---------------------------------------------------------------------------
// /api/compression/preview
// ---------------------------------------------------------------------------

export interface EngineBreakdown {
	id: string;
	input_bytes: number;
	output_bytes: number;
	compressed_by: number;
}

export interface PreviewRequest {
	mode?: CompressionMode;
	payload: TestPayload;
	intensity?: string;
}

export interface PreviewResponse {
	mode: CompressionMode;
	result: TestResult;
	engineStats?: EngineBreakdown[];
	originalConfig?: import("./plugins").RTKConfig;
	effectiveConfig?: import("./plugins").RTKConfig;
	enginesPlanned?: string[];
}

// ---------------------------------------------------------------------------
// Engine breakdown subset (used by the test endpoint internally)
// ---------------------------------------------------------------------------

export interface EngineStats {
	engines: EngineBreakdown[];
}

// Validation helpers shared between the form schema and the admin UI tests.

export const RTK_RAW_OUTPUT_ID_PATTERN = /^[0-9a-fA-F]{24}$/;

export function isValidRawOutputID(id: string): boolean {
	return RTK_RAW_OUTPUT_ID_PATTERN.test(id);
}

// ---------------------------------------------------------------------------
// /api/context/rtk/caveman/rules
// ---------------------------------------------------------------------------

// CavemanRuleCatalogEntry mirrors the Go struct at
// plugins/rtk/cavemancatalog.go. The shape is consumed by the skip_rules
// multi-select and the preserve_patterns hint list in rtkFragment.tsx.
export interface CavemanRuleCatalogEntry {
	name: string;
	label: string;
	category?: string;
	context: "all" | "user" | "system" | "assistant";
	language: "en" | "zh";
	minIntensity: "lite" | "full" | "ultra";
}

export interface CavemanRuleCatalog {
	rules: CavemanRuleCatalogEntry[];
	builtInPreservePatterns: string[];
}