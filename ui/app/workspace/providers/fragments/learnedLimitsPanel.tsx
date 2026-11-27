import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { COOLDOWN_OBSERVABILITY_POLLING_INTERVAL_MS } from "@/components/providerCooldown/defaults";
import { getQueryErrorStatus } from "@/components/providerCooldown/queryError";
import { useGetLearnedLimitsQuery } from "@/lib/store/apis/pluginsApi";
import type { LearnedLimitConfidence, LearnedLimitEntry } from "@/lib/types/plugins";
import { useTranslation } from "react-i18next";

interface LearnedLimitsPanelProps {
	provider: string;
	/** Window the capacity gate currently uses, for the "different window" hint. */
	capacityWindowSeconds: number;
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

/**
 * Read-only suggestions produced by the limit learner for one provider.
 *
 * The estimates are informational: "adopt" only pre-fills the capacity gate's
 * token limit in the form, it never saves anything on its own.
 */
export function LearnedLimitsPanel({ provider, capacityWindowSeconds, onAdopt, disabled }: LearnedLimitsPanelProps) {
	const { t } = useTranslation("common");
	const { data, error } = useGetLearnedLimitsQuery(undefined, {
		pollingInterval: COOLDOWN_OBSERVABILITY_POLLING_INTERVAL_MS,
	});

	// 400 = the provider-cooldown plugin isn't loaded. Hide the whole block
	// instead of repeating an error the operator can't act on.
	if (getQueryErrorStatus(error) === 400) return null;

	const limits = (data?.limits ?? []).filter((limit) => limit.provider === provider);

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

			{limits.length === 0 ? (
				<p data-testid="providercooldown-learning-suggestions-empty" className="text-muted-foreground text-xs">
					{t("cooldownCapability.learning.suggestionsEmpty")}
				</p>
			) : (
				<ul className="space-y-2">
					{limits.map((limit) => {
						const windowDiffers = limit.windowSeconds > 0 && limit.windowSeconds !== capacityWindowSeconds;
						return (
							<li
								key={`${limit.provider}|${limit.model}`}
								data-testid={`providercooldown-learning-suggestion-${limit.model || "all"}`}
								className="bg-muted/30 space-y-1 rounded border px-2 py-1.5 text-xs"
							>
								<div className="flex flex-wrap items-center justify-between gap-2">
									<span className="font-mono">{limit.model || t("cooldownCapability.learning.allModels")}</span>
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
								{limit.limiterDimension === "rpm" && (
									<p
										data-testid={`providercooldown-learning-rpm-only-hint-${limit.model || "all"}`}
										className="text-muted-foreground text-xs italic"
									>
										{t("cooldownCapability.learning.rpmOnlyHint")}
									</p>
								)}
								<div className="text-muted-foreground">
									{t("cooldownCapability.learning.estimate", {
										tokens: formatLimit(limit.estimatedTokenLimit),
										window: limit.windowSeconds,
									})}
								</div>
								{windowDiffers && (
									<p
										data-testid={`providercooldown-learning-window-hint-${limit.model || "all"}`}
										className="text-muted-foreground text-xs italic"
									>
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
									data-testid={`providercooldown-learning-adopt-${limit.model || "all"}`}
									onClick={() => onAdopt(limit)}
									disabled={disabled || limit.estimatedTokenLimit === null}
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