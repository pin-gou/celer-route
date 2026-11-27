import { CooldownPolicyFormFragment } from "@/app/workspace/providers/fragments/cooldownPolicyFormFragment";
import { COOLDOWN_OBSERVABILITY_POLLING_INTERVAL_MS } from "@/components/providerCooldown/defaults";
import {
	COOLDOWN_WINDOW_OPTIONS,
	DEFAULT_COOLDOWN_WINDOW,
	cooldownWindowLabelKey,
	cooldownWindowMinutes,
	formatUpdatedAt,
	type CooldownWindowKey,
} from "@/components/providerCooldown/cooldownWindow";
import { getQueryErrorStatus } from "@/components/providerCooldown/queryError";
import { Button } from "@/components/ui/button";
import { useGetProviderUsageQuery } from "@/lib/store/apis/providerUsageApi";
import type { ModelProvider } from "@/lib/types/config";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { CooldownCapacityCard } from "./CooldownCapacityCard";
import { ProviderUsageCard } from "./ProviderUsageCard";

interface CooldownTabProps {
	provider: ModelProvider;
}

/**
 * The provider cooldown tab.
 *
 * Information architecture: the tab owns ONE observation window and one
 * observation snapshot, and every block below renders that same window. The
 * configuration zone comes first because it is what the tab is for — the
 * "from errors to rules" flow sits directly above the rule editor, so the
 * operator reads an error shape and writes the matching rule without leaving
 * the block. The observation cards follow as the supporting detail (which key
 * is misbehaving, how close the capacity gate is to its budget).
 */
export function CooldownTab({ provider }: CooldownTabProps) {
	const { t: tc } = useTranslation("common");
	const { t: tp } = useTranslation("providers");
	const [window, setWindow] = useState<CooldownWindowKey>(DEFAULT_COOLDOWN_WINDOW);
	const windowMinutes = cooldownWindowMinutes(window);
	const providerName = String(provider.name);

	const usage = useGetProviderUsageQuery(
		{ provider: providerName, windowMinutes },
		{ pollingInterval: COOLDOWN_OBSERVABILITY_POLLING_INTERVAL_MS },
	);

	// No logs store means usage (and the error shapes) cannot be computed at
	// all. The cooldown form works without them, so the whole observation
	// layer is dropped instead of surfacing an error the operator can do
	// nothing about.
	const logsUnavailable = getQueryErrorStatus(usage.error) === 503;
	const rows = usage.data?.entries ?? [];
	const hasTraffic = rows.length > 0;

	const summary = useMemo(() => {
		const totals = rows.reduce(
			(acc, entry) => {
				acc.requests += entry.requests ?? 0;
				acc.errors += entry.errors ?? 0;
				acc.tokens += entry.total_tokens ?? 0;
				return acc;
			},
			{ requests: 0, errors: 0, tokens: 0 },
		);
		const windowMinutesUsed = usage.data?.window_minutes ?? windowMinutes;
		return {
			...totals,
			errorRate: totals.requests > 0 ? `${((totals.errors / totals.requests) * 100).toFixed(1)}%` : "-",
			tokensPerMinute: windowMinutesUsed > 0 ? Math.round(totals.tokens / windowMinutesUsed) : 0,
		};
	}, [rows, usage.data?.window_minutes, windowMinutes]);

	const updatedAt = formatUpdatedAt(usage.fulfilledTimeStamp);
	// One empty state for the whole tab: when the window holds no traffic
	// there is nothing to observe, and the per-block "no data" messages used
	// to repeat the same fact three times.
	const showEmptyNotice = !logsUnavailable && !usage.isLoading && !usage.error && !hasTraffic;

	return (
		<div className="space-y-4">
			{!logsUnavailable && (
				<div
					data-testid="providers2-cooldown-observability-bar"
					className="flex flex-wrap items-center gap-x-5 gap-y-2 rounded-lg border px-4 py-2.5"
				>
					<div className="flex items-center gap-2">
						<span className="text-muted-foreground text-xs">{tc("cooldownCapability.window.label")}</span>
						<div className="flex items-center gap-1" role="group" aria-label={tc("cooldownCapability.window.label")}>
							{COOLDOWN_WINDOW_OPTIONS.map((option) => (
								<Button
									key={option.key}
									type="button"
									size="sm"
									variant={option.key === window ? "default" : "outline"}
									className="h-7 px-2 text-xs"
									onClick={() => setWindow(option.key)}
									data-testid={option.testId}
								>
									{tc(cooldownWindowLabelKey(option.key))}
								</Button>
							))}
						</div>
					</div>

					{hasTraffic && (
						<div data-testid="providers2-cooldown-summary" className="flex flex-wrap items-center gap-x-4 gap-y-1">
							<SummaryMetric label={tp("fragments.providerUsage.summary.requests")} value={formatInt(summary.requests)} />
							<SummaryMetric label={tp("fragments.providerUsage.summary.errors")} value={formatInt(summary.errors)} />
							<SummaryMetric label={tp("fragments.providerUsage.summary.errorRate")} value={summary.errorRate} />
							<SummaryMetric label={tp("fragments.providerUsage.summary.totalTokens")} value={formatInt(summary.tokens)} />
							<SummaryMetric label={tp("fragments.providerUsage.summary.tokensPerMinute")} value={formatInt(summary.tokensPerMinute)} />
						</div>
					)}

					{updatedAt && (
						<span data-testid="providers2-cooldown-updated-at" className="text-muted-foreground ml-auto text-xs">
							{tp("fragments.providerUsage.updatedAt", { time: updatedAt })}
						</span>
					)}
				</div>
			)}

			{showEmptyNotice && (
				<div
					data-testid="providers2-cooldown-empty-notice"
					className="text-muted-foreground rounded-lg border border-dashed px-4 py-3 text-xs"
				>
					{tp("fragments.providerUsage.emptyTabNotice")}
				</div>
			)}

			{/* Primary zone: the policy editor, with the error shapes that feed
			    it directly above the rules. */}
			<div data-testid="providers2-cooldown-policy-card" className="rounded-lg border">
				<CooldownPolicyFormFragment provider={provider} window={window} />
			</div>

			{/* Supporting detail: the per-key breakdown and the gate's live
			    water level. */}
			{!showEmptyNotice && (
				<ProviderUsageCard
					usage={{
						entries: rows,
						isLoading: usage.isLoading,
						hasError: Boolean(usage.error),
						updatedAt: usage.fulfilledTimeStamp,
					}}
				/>
			)}
			<CooldownCapacityCard provider={provider} />
		</div>
	);
}

function formatInt(value: number): string {
	return value.toLocaleString("en-US");
}

function SummaryMetric({ label, value }: { label: string; value: string }) {
	return (
		<span className="text-xs">
			<span className="text-muted-foreground">{label}</span> <span className="font-mono font-medium">{value}</span>
		</span>
	);
}