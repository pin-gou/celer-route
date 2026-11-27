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
 * Resolves the capacity config that actually applies right now: the provider
 * override wins per field, then the plugin-level default, then the frontend
 * constant. This is both what the tri-state card summarises and what the
 * "custom" mode pre-fills.
 */
export function resolveEffectiveCapacity(
	providerSegment: CooldownCapacityConfig | undefined,
	pluginSegment: CooldownCapacityConfig | undefined,
): Required<CooldownCapacityConfig> {
	return {
		enabled: providerSegment?.enabled ?? pluginSegment?.enabled ?? false,
		token_limit: definedOr(providerSegment?.token_limit, definedOr(pluginSegment?.token_limit, 0)),
		request_limit: definedOr(providerSegment?.request_limit, definedOr(pluginSegment?.request_limit, 0)),
		window_seconds: positiveOr(providerSegment?.window_seconds, positiveOr(pluginSegment?.window_seconds, DEFAULT_CAPACITY_WINDOW_SECONDS)),
		headroom: positiveOr(providerSegment?.headroom, positiveOr(pluginSegment?.headroom, DEFAULT_CAPACITY_HEADROOM)),
		scope: providerSegment?.scope || pluginSegment?.scope || DEFAULT_CAPACITY_SCOPE,
		action: providerSegment?.action || pluginSegment?.action || DEFAULT_CAPACITY_ACTION,
	};
}

export function resolveEffectiveEscalation(
	providerSegment: CooldownEscalationConfig | undefined,
	pluginSegment: CooldownEscalationConfig | undefined,
): Required<CooldownEscalationConfig> {
	return {
		enabled: providerSegment?.enabled ?? pluginSegment?.enabled ?? false,
		max_multiplier: positiveOr(
			providerSegment?.max_multiplier,
			positiveOr(pluginSegment?.max_multiplier, DEFAULT_ESCALATION_MAX_MULTIPLIER),
		),
	};
}

export function resolveEffectiveLearning(
	providerSegment: CooldownLearningConfig | undefined,
	pluginSegment: CooldownLearningConfig | undefined,
): Required<CooldownLearningConfig> {
	return {
		enabled: providerSegment?.enabled ?? pluginSegment?.enabled ?? false,
		window_seconds: positiveOr(providerSegment?.window_seconds, positiveOr(pluginSegment?.window_seconds, DEFAULT_LEARNING_WINDOW_SECONDS)),
		max_samples: positiveOr(providerSegment?.max_samples, positiveOr(pluginSegment?.max_samples, DEFAULT_LEARNING_MAX_SAMPLES)),
		min_samples: positiveOr(providerSegment?.min_samples, positiveOr(pluginSegment?.min_samples, DEFAULT_LEARNING_MIN_SAMPLES)),
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