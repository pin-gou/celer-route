import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { COOLDOWN_OBSERVABILITY_POLLING_INTERVAL_MS } from "@/components/providerCooldown/defaults";
import { getQueryErrorStatus } from "@/components/providerCooldown/queryError";
import { useGetLearnedLimitsQuery } from "@/lib/store/apis/pluginsApi";
import type { CooldownCapacityConfig } from "@/lib/types/config";
import type { LearnedLimitConfidence, LearnedLimitEntry, LearnedLimitScope } from "@/lib/types/plugins";
import { useTranslation } from "react-i18next";

export type CapacityScope = NonNullable<CooldownCapacityConfig["scope"]>;

interface LearnedLimitsPanelProps {
	provider: string;
	/** Window the capacity gate currently uses, for the "different window" hint. */
	capacityWindowSeconds: number;
	/**
	 * Granularity the capacity gate is configured for. The learner reports every
	 * estimate at one of two levels and the two are NOT interchangeable, so only
	 * the level the gate can enforce is offered here.
	 */
	capacityScope: CapacityScope;
	onAdopt: (limit: LearnedLimitEntry) => void;
	disabled?: boolean;
}

const CONFIDENCE_TEST_IDS: Record<LearnedLimitConfidence, string> = {
	insufficient: "insufficient",
	low: "low",
	medium: "medium",
	high: "high",
};

function formatLimit(value: number | null): string {
	if (value === null) return "—";
	return value.toLocaleString("en-US");
}

/** The learning level that matches a capacity-gate scope, if any. */
function learningScopeFor(scope: CapacityScope): LearnedLimitScope | null {
	switch (scope) {
		case "key":
			return "key_model";
		case "provider_model":
			return "provider_model";
		default:
			// A provider-wide budget aggregates every model, which the learner
			// never measures: its numbers are per (provider, model), so they
			// cannot be summed into a provider-level limit without inventing a
			// model count.
			return null;
	}
}

/**
 * Read-only suggestions produced by the limit learner for one provider.
 *
 * The estimates are informational: "adopt" only pre-fills the capacity gate's
 * token / request limits in the form, it never saves anything on its own.
 *
 * Two estimates are shown per entry because the learner derives two: the token
 * budget (from TPM/unknown rejections) and the request budget (from RPM
 * rejections). Either can be missing while the other is available.
 */
export function LearnedLimitsPanel({ provider, capacityWindowSeconds, capacityScope, onAdopt, disabled }: LearnedLimitsPanelProps) {
	const { t } = useTranslation("common");
	const { data, error } = useGetLearnedLimitsQuery(undefined, {
		pollingInterval: COOLDOWN_OBSERVABILITY_POLLING_INTERVAL_MS,
	});

	// 400 = the provider-cooldown plugin isn't loaded. Hide the whole block
	// instead of repeating an error the operator can't act on.
	if (getQueryErrorStatus(error) === 400) return null;

	const wantedScope = learningScopeFor(capacityScope);
	const limits = (data?.limits ?? []).filter((limit) => limit.provider === provider && wantedScope !== null && limit.scope === wantedScope);
	// The token estimate is the primary number in the UI; when it is missing the
	// request estimate may still be actionable.
	const hasAdoptableEstimate = (limit: LearnedLimitEntry) => limit.estimatedTokenLimit !== null || limit.estimatedRequestLimit !== null;

	return (
		<div data-testid="providercooldown-learning-suggestions" className="border-t pt-3">
			<div className="mb-2 flex items-center justify-between gap-2">
				<h6 className="text-xs font-medium">{t("cooldownCapability.learning.suggestionsTitle")}</h6>
				{data && (
					<span className="text-muted-foreground text-xs">
						{t("cooldownCapability.learning.suggestionsCount", { count: limits.length })}
					</span>
				)}
			</div>

			{wantedScope === null ? (
				<p data-testid="providercooldown-learning-scope-provider-hint" className="text-muted-foreground text-xs italic">
					{t("cooldownCapability.learning.scopeProviderHint")}
				</p>
			) : limits.length === 0 ? (
				<p data-testid="providercooldown-learning-suggestions-empty" className="text-muted-foreground text-xs">
					{t("cooldownCapability.learning.suggestionsEmpty")}
				</p>
			) : (
				<ul className="space-y-2">
					{limits.map((limit) => {
						const windowDiffers = limit.windowSeconds > 0 && limit.windowSeconds !== capacityWindowSeconds;
						// A per-key entry is identified by its credential; an
						// aggregate one by its model alone.
						const rowId = limit.scope === "key_model" ? `${limit.keyName ?? ""}|${limit.model || "all"}` : limit.model || "all";
						const tokenMissing = limit.estimatedTokenLimit === null;
						const requestMissing = limit.estimatedRequestLimit === null;
						return (
							<li
								key={rowId}
								data-testid={`providercooldown-learning-suggestion-${rowId}`}
								className="bg-muted/30 space-y-1 rounded border px-2 py-1.5 text-xs"
							>
								<div className="flex flex-wrap items-center justify-between gap-2">
									<span className="flex flex-wrap items-center gap-1.5 font-mono">
										<span>{limit.model || t("cooldownCapability.learning.allModels")}</span>
										{limit.scope === "key_model" && (
											<span data-testid={`providercooldown-learning-key-${rowId}`} className="text-muted-foreground font-sans break-all">
												{t("cooldownCapability.learning.keyLabel", { name: limit.keyName ?? "" })}
											</span>
										)}
									</span>
									<span className="flex items-center gap-1">
										<Badge variant="outline" data-testid={`providercooldown-learning-confidence-${CONFIDENCE_TEST_IDS[limit.confidence]}`}>
											{t(`cooldownCapability.learning.confidence.${limit.confidence}`)}
										</Badge>
										{limit.limiterDimension && (
											<Badge variant="outline" data-testid={`providercooldown-learning-dimension-${limit.limiterDimension}`}>
												{t(`cooldownCapability.learning.dimension.${limit.limiterDimension}`)}
											</Badge>
										)}
										<span className="text-muted-foreground">
											{t("cooldownCapability.learning.sampleCount", { count: limit.sampleCount })}
										</span>
									</span>
								</div>
								<div className="text-muted-foreground" data-testid={`providercooldown-learning-token-${rowId}`}>
									{tokenMissing
										? t("cooldownCapability.learning.noTokenEstimate")
										: t("cooldownCapability.learning.estimate", {
												tokens: formatLimit(limit.estimatedTokenLimit),
												window: limit.windowSeconds,
											})}
								</div>
								<div className="text-muted-foreground" data-testid={`providercooldown-learning-request-${rowId}`}>
									{requestMissing
										? t("cooldownCapability.learning.noRequestEstimate")
										: t("cooldownCapability.learning.requestEstimate", {
												requests: formatLimit(limit.estimatedRequestLimit),
												window: limit.windowSeconds,
											})}
								</div>
								{limit.limiterDimension === "rpm" && (
									<p data-testid={`providercooldown-learning-rpm-only-hint-${rowId}`} className="text-muted-foreground text-xs italic">
										{t("cooldownCapability.learning.rpmOnlyHint")}
									</p>
								)}
								{windowDiffers && (
									<p data-testid={`providercooldown-learning-window-hint-${rowId}`} className="text-muted-foreground text-xs italic">
										{t("cooldownCapability.learning.windowMismatch", {
											learned: limit.windowSeconds,
											capacity: capacityWindowSeconds,
										})}
									</p>
								)}
								<Button
									type="button"
									variant="outline"
									size="sm"
									className="h-7 px-2 text-xs"
									data-testid={`providercooldown-learning-adopt-${rowId}`}
									onClick={() => onAdopt(limit)}
									disabled={disabled || !hasAdoptableEstimate(limit)}
								>
									{t("cooldownCapability.learning.adopt")}
								</Button>
							</li>
						);
					})}
				</ul>
			)}
		</div>
	);
}