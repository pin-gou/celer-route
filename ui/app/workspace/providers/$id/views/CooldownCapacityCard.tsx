import { COOLDOWN_OBSERVABILITY_POLLING_INTERVAL_MS } from "@/components/providerCooldown/defaults";
import { getQueryErrorStatus } from "@/components/providerCooldown/queryError";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { useGetCapacityUsageQuery } from "@/lib/store/apis/pluginsApi";
import type { ModelProvider } from "@/lib/types/config";
import type { CapacityUsageEntry } from "@/lib/types/plugins";
import { useTranslation } from "react-i18next";

interface CooldownCapacityCardProps {
	provider: ModelProvider;
}

function formatInt(value: number): string {
	return value.toLocaleString("en-US");
}

/** used/threshold as a 0..100 percentage; threshold 0 means "no active limit". */
function usedPercent(used: number, threshold: number): number | null {
	if (threshold <= 0) return null;
	return Math.min(100, Math.round((used / threshold) * 100));
}

function CapacityMeter({ label, used, threshold, testId }: { label: string; used: number; threshold: number; testId: string }) {
	const percent = usedPercent(used, threshold);
	return (
		<div data-testid={testId} className="space-y-1">
			<div className="text-muted-foreground flex items-center justify-between gap-2 text-xs">
				<span>{label}</span>
				<span className="font-mono">{percent === null ? "—" : `${formatInt(used)} / ${formatInt(threshold)} · ${percent}%`}</span>
			</div>
			{percent !== null && <Progress value={percent} className="h-1.5" />}
		</div>
	);
}

function CapacityRow({ entry }: { entry: CapacityUsageEntry }) {
	const { t } = useTranslation("common");
	const modelKey = entry.model || "all";
	return (
		<div data-testid={`providers2-capacity-row-${modelKey}`} className="space-y-2 rounded-md border p-2.5">
			<div className="flex flex-wrap items-center justify-between gap-2">
				<span className="font-mono text-xs">{entry.model || t("cooldownCapability.capacityUsage.allModels")}</span>
				<div className="flex items-center gap-1.5">
					<Badge variant="outline" data-testid={`providers2-capacity-source-${modelKey}`}>
						{entry.source === "provider"
							? t("cooldownCapability.capacityUsage.sourceProvider")
							: t("cooldownCapability.capacityUsage.sourceDefault")}
					</Badge>
					<span className="text-muted-foreground text-xs">
						{t("cooldownCapability.capacityUsage.window", { seconds: entry.windowSeconds })}
					</span>
				</div>
			</div>
			<CapacityMeter
				label={t("cooldownCapability.capacityUsage.tokens")}
				used={entry.usedTokens}
				threshold={entry.thresholdTokens}
				testId={`providers2-capacity-tokens-${modelKey}`}
			/>
			<CapacityMeter
				label={t("cooldownCapability.capacityUsage.requests")}
				used={entry.usedRequests}
				threshold={entry.thresholdRequests}
				testId={`providers2-capacity-requests-${modelKey}`}
			/>
			{entry.retryAfterSeconds > 0 && (
				<p data-testid={`providers2-capacity-retry-${modelKey}`} className="text-xs text-amber-600 dark:text-amber-500">
					{t("cooldownCapability.capacityUsage.retryAfter", { seconds: entry.retryAfterSeconds })}
				</p>
			)}
		</div>
	);
}

/**
 * Live capacity-usage ("water level") card for one provider.
 *
 * The endpoint reports the accounting for every provider, so the rows are
 * filtered to the provider being viewed. 400 means the provider-cooldown
 * plugin isn't loaded — the whole card is hidden then, since there is nothing
 * the operator can do about it from a provider page.
 */
export function CooldownCapacityCard({ provider }: CooldownCapacityCardProps) {
	const { t } = useTranslation("common");
	const { data, error, isLoading } = useGetCapacityUsageQuery(undefined, {
		pollingInterval: COOLDOWN_OBSERVABILITY_POLLING_INTERVAL_MS,
	});

	if (getQueryErrorStatus(error) === 400) return null;

	const entries = (data?.entries ?? []).filter((entry) => entry.provider === provider.name);
	// An explicit `{ enabled: false }` on the provider means the gate will never
	// engage, no matter what the plugin default is.
	const gateExplicitlyDisabled = provider.cooldown_policy?.capacity?.enabled === false;

	return (
		<div data-testid="providers2-capacity-usage-card" className="rounded-lg border p-4">
			<div className="mb-3">
				<h3 className="text-sm font-medium">{t("cooldownCapability.capacityUsage.title")}</h3>
				<p className="text-muted-foreground mt-1 text-xs">{t("cooldownCapability.capacityUsage.description")}</p>
			</div>

			{isLoading && (
				<p data-testid="providers2-capacity-loading" className="text-muted-foreground py-4 text-center text-xs">
					{t("cooldownCapability.capacityUsage.loading")}
				</p>
			)}

			{!isLoading && error && (
				<p data-testid="providers2-capacity-error" className="text-destructive py-4 text-center text-xs">
					{t("cooldownCapability.capacityUsage.error")}
				</p>
			)}

			{!isLoading && !error && (gateExplicitlyDisabled || entries.length === 0) && (
				<p data-testid="providers2-capacity-empty" className="text-muted-foreground py-4 text-center text-xs">
					{gateExplicitlyDisabled ? t("cooldownCapability.capacityUsage.emptyDisabled") : t("cooldownCapability.capacityUsage.empty")}
				</p>
			)}

			{!isLoading && !error && !gateExplicitlyDisabled && entries.length > 0 && (
				<div className="space-y-2">
					{entries.map((entry) => (
						<CapacityRow key={`${entry.scope}|${entry.model}`} entry={entry} />
					))}
				</div>
			)}
		</div>
	);
}