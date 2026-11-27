import type { CooldownCapacityConfig, CooldownEscalationConfig, CooldownLearningConfig } from "@/lib/types/config";
import type { ProviderCooldownCapabilityDefaults } from "@/lib/types/plugins";

// Frontend mirrors of the backend defaults in core/schemas/cooldownpolicy.go.
// They are only used to prefill the "custom" form with a sane, fully-populated
// object; the backend still applies the same defaults to any field left unset.
export const DEFAULT_ESCALATION_MAX_MULTIPLIER = 4;
export const DEFAULT_CAPACITY_WINDOW_SECONDS = 60;
export const DEFAULT_CAPACITY_HEADROOM = 0.9;
export const DEFAULT_CAPACITY_SCOPE = "provider" as const;
export const DEFAULT_CAPACITY_ACTION = "fallback" as const;
export const DEFAULT_LEARNING_WINDOW_SECONDS = 60;
export const DEFAULT_LEARNING_MAX_SAMPLES = 50;
export const DEFAULT_LEARNING_MIN_SAMPLES = 5;

// Both provider-cooldown observability endpoints are polled on the same
// cadence as ProviderUsageCard: the accounting is cheap but not free, and the
// operator only needs a per-minute view.
export const COOLDOWN_OBSERVABILITY_POLLING_INTERVAL_MS = 60_000;

/** The three per-provider capability overrides, in display order. */
export type CooldownCapabilityKey = "escalation" | "capacity" | "learning";

/**
 * Tri-state of a per-provider capability override:
 *   - `inherit`  → no provider-level section; the plugin default applies.
 *   - `custom`   → the provider carries its own section.
 *   - `disabled` → the provider explicitly sets `{ enabled: false }`.
 */
export type CooldownCapabilityMode = "inherit" | "custom" | "disabled";

export function cooldownCapabilityMode(segment: { enabled?: boolean } | undefined): CooldownCapabilityMode {
	if (segment === undefined) return "inherit";
	return segment.enabled === false ? "disabled" : "custom";
}

/** Treats 0/negative/absent as "unset" — the backend does the same for these fields. */
function positiveOr(value: number | undefined, fallback: number): number {
	return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : fallback;
}

/** 0 is a meaningful value for the limit fields ("no limit"), so only absent falls through. */
function definedOr(value: number | undefined, fallback: number): number {
	return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

/**
 * Resolves the config that actually applies right now, mirroring the backend's
 * whole-section rule (`capabilityDefaults.resolve*` in the plugin):
 *
 *   - provider section present → used as-is; fields it omits fall back to the
 *     built-in defaults (never to the plugin default — there is no field-level
 *     merge), then to the frontend constant which mirrors the same defaults;
 *   - provider section absent  → the plugin-level default, which is itself
 *     allowed to be partial and falls back to the built-in defaults.
 *
 * This is both what the tri-state card summarises and what the "custom" mode
 * pre-fills.
 */
export function resolveEffectiveCapacity(
	providerSegment: CooldownCapacityConfig | undefined,
	pluginSegment: CooldownCapacityConfig | undefined,
): Required<CooldownCapacityConfig> {
	const source = providerSegment ?? pluginSegment;
	return {
		enabled: source?.enabled ?? false,
		token_limit: definedOr(source?.token_limit, 0),
		request_limit: definedOr(source?.request_limit, 0),
		window_seconds: positiveOr(source?.window_seconds, DEFAULT_CAPACITY_WINDOW_SECONDS),
		headroom: positiveOr(source?.headroom, DEFAULT_CAPACITY_HEADROOM),
		scope: source?.scope || DEFAULT_CAPACITY_SCOPE,
		action: source?.action || DEFAULT_CAPACITY_ACTION,
	};
}

/**
 * Where the capacity gate that applies to a provider actually comes from:
 *   - `provider`       → the provider carries its own `cooldown_policy.capacity`
 *   - `legacy-default` → no provider section, but the plugin-level (config.json)
 *                        section exists and actually gates; it is a deprecated
 *                        fleet-wide default that the UI no longer lets anyone
 *                        edit, only observe.
 *   - `none`           → nothing is configured (or the legacy section is inert),
 *                        so the gate does not run for this provider.
 */
export type CooldownCapacitySource = "provider" | "legacy-default" | "none";

/**
 * Fleet-level summary of the capacity gate for one provider: which tri-state
 * it is in, where the effective values come from, whether the gate actually
 * blocks anything today (enabled with at least one positive limit), and the
 * values that apply after inheritance.
 */
export interface CapacityGateStatus {
	mode: CooldownCapabilityMode;
	source: CooldownCapacitySource;
	active: boolean;
	effective: Required<CooldownCapacityConfig>;
}

/** Whether the plugin-level capacity section exists and is actually gating. */
export function isLegacyCapacityActive(pluginSegment: CooldownCapacityConfig | undefined): boolean {
	if (pluginSegment === undefined) return false;
	const effective = resolveEffectiveCapacity(undefined, pluginSegment);
	return effective.enabled && (effective.token_limit > 0 || effective.request_limit > 0);
}

export function capacityGateStatus(
	providerSegment: CooldownCapacityConfig | undefined,
	pluginSegment: CooldownCapacityConfig | undefined,
): CapacityGateStatus {
	const effective = resolveEffectiveCapacity(providerSegment, pluginSegment);
	const active = effective.enabled && (effective.token_limit > 0 || effective.request_limit > 0);
	// A legacy section that is present but inert (disabled, or enabled with no
	// limit) is reported as "not configured" — it never blocks anything, so
	// calling it a default the provider "uses" would be misleading.
	const source: CooldownCapacitySource =
		providerSegment !== undefined ? "provider" : isLegacyCapacityActive(pluginSegment) ? "legacy-default" : "none";
	return {
		mode: cooldownCapabilityMode(providerSegment),
		source,
		active,
		effective,
	};
}

export function resolveEffectiveEscalation(
	providerSegment: CooldownEscalationConfig | undefined,
	pluginSegment: CooldownEscalationConfig | undefined,
): Required<CooldownEscalationConfig> {
	const source = providerSegment ?? pluginSegment;
	return {
		enabled: source?.enabled ?? false,
		max_multiplier: positiveOr(source?.max_multiplier, DEFAULT_ESCALATION_MAX_MULTIPLIER),
	};
}

export function resolveEffectiveLearning(
	providerSegment: CooldownLearningConfig | undefined,
	pluginSegment: CooldownLearningConfig | undefined,
): Required<CooldownLearningConfig> {
	const source = providerSegment ?? pluginSegment;
	return {
		enabled: source?.enabled ?? false,
		window_seconds: positiveOr(source?.window_seconds, DEFAULT_LEARNING_WINDOW_SECONDS),
		max_samples: positiveOr(source?.max_samples, DEFAULT_LEARNING_MAX_SAMPLES),
		min_samples: positiveOr(source?.min_samples, DEFAULT_LEARNING_MIN_SAMPLES),
	};
}

/** Effective values for all three capabilities, ready to prefill a "custom" section. */
export function resolveEffectiveCapabilities(
	providerPolicy:
		| { capacity?: CooldownCapacityConfig; escalation?: CooldownEscalationConfig; learning?: CooldownLearningConfig }
		| undefined,
	pluginDefaults: ProviderCooldownCapabilityDefaults,
): {
	capacity: Required<CooldownCapacityConfig>;
	escalation: Required<CooldownEscalationConfig>;
	learning: Required<CooldownLearningConfig>;
} {
	return {
		capacity: resolveEffectiveCapacity(providerPolicy?.capacity, pluginDefaults.capacity),
		escalation: resolveEffectiveEscalation(providerPolicy?.escalation, pluginDefaults.escalation),
		learning: resolveEffectiveLearning(providerPolicy?.learning, pluginDefaults.learning),
	};
}