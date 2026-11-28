import type { TFunction } from "i18next";
import type { CooldownCapacityConfig } from "@/lib/types/config";

/**
 * Human-readable summary of an effective capacity gate, shared by the fleet
 * status line and the deprecated-plugin-default warning so both render the
 * exact same wording for the same values.
 *
 * `t` is the `plugins` namespace, `tc` the `common` one (the capacity field
 * labels live in `common.cooldownCapability.capacity`).
 */
export function formatCapacitySummary(t: TFunction, tc: TFunction, effective: Required<CooldownCapacityConfig>): string {
	const noLimit = tc("cooldownCapability.capacity.noLimit");
	return t("providerCooldown.capacitySummary", {
		tokens: effective.token_limit > 0 ? effective.token_limit.toLocaleString("en-US") : noLimit,
		requests: effective.request_limit > 0 ? effective.request_limit.toLocaleString("en-US") : noLimit,
		window: effective.window_seconds,
		headroom: effective.headroom,
		scope: tc(`cooldownCapability.capacity.scope.${effective.scope}`),
	});
}