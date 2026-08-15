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