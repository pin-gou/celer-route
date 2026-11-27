import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useGetErrorPatternsQuery } from "@/lib/store/apis/errorPatternsApi";
import { useGetProviderUsageQuery } from "@/lib/store/apis/providerUsageApi";
import type { ModelProvider } from "@/lib/types/config";
import { cn } from "@/lib/utils";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

interface ProviderUsageCardProps {
	provider: ModelProvider;
}

// The usage query is comparatively expensive server-side (it groups the whole
// log window by model × key), so the card never polls faster than once a
// minute.
const POLLING_INTERVAL_MS = 60_000;

// Above this many (model × key) groups the table becomes height-capped and
// scrolls instead of pushing the cooldown form off-screen.
const SCROLL_THRESHOLD = 20;

type UsageWindow = {
	minutes: number;
	errorWindow: "1h" | "24h";
	testId: string;
	labelKey: string;
};

const WINDOW_OPTIONS: readonly UsageWindow[] = [
	{
		minutes: 60,
		errorWindow: "1h",
		testId: "providers2-usage-window-1h",
		labelKey: "fragments.providerUsage.windowOptions.1h",
	},
	{
		minutes: 1440,
		errorWindow: "24h",
		testId: "providers2-usage-window-24h",
		labelKey: "fragments.providerUsage.windowOptions.24h",
	},
];

function getErrorStatus(error: unknown): number | undefined {
	if (typeof error !== "object" || error === null || !("status" in error)) return undefined;
	const status = (error as { status?: unknown }).status;
	return typeof status === "number" ? status : undefined;
}

function formatInt(value: number | undefined): string {
	if (value === undefined || !Number.isFinite(value)) return "-";
	return value.toLocaleString("en-US");
}

function formatErrorRate(errors: number | undefined, requests: number | undefined): string {
	const safeErrors = errors ?? 0;
	const safeRequests = requests ?? 0;
	if (safeRequests <= 0) return "-";
	return `${((safeErrors / safeRequests) * 100).toFixed(1)}%`;
}

export function ProviderUsageCard({ provider }: ProviderUsageCardProps) {
	const { t } = useTranslation("providers");
	const [windowMinutes, setWindowMinutes] = useState<number>(60);
	const windowOption = WINDOW_OPTIONS.find((option) => option.minutes === windowMinutes) ?? WINDOW_OPTIONS[0];
	const providerName = String(provider.name);

	const usage = useGetProviderUsageQuery({ provider: providerName, windowMinutes }, { pollingInterval: POLLING_INTERVAL_MS });
	const patterns = useGetErrorPatternsQuery({ provider: providerName, window: windowOption.errorWindow, limit: 3 });

	const entries = usage.data?.entries;
	const rows = entries ?? [];
	const topPatterns = (patterns.data?.patterns ?? []).slice(0, 3);

	const summary = useMemo(() => {
		const totals = (entries ?? []).reduce(
			(acc, entry) => {
				acc.requests += entry.requests ?? 0;
				acc.errors += entry.errors ?? 0;
				acc.tokens += entry.total_tokens ?? 0;
				return acc;
			},
			{ requests: 0, errors: 0, tokens: 0 },
		);
		const window = usage.data?.window_minutes ?? windowMinutes;
		return {
			...totals,
			tokensPerMinute: window > 0 ? totals.tokens / window : 0,
		};
	}, [entries, usage.data?.window_minutes, windowMinutes]);

	// No logs store means usage (and the error baseline) cannot be computed at
	// all. The cooldown form works without it, so drop the whole card instead
	// of surfacing an error the operator can do nothing about.
	if (getErrorStatus(usage.error) === 503) return null;

	const baselineTitle = t("fragments.providerUsage.baseline.title");

	return (
		<div data-testid="providers2-usage-card" className="rounded-lg border p-4">
			<div className="mb-3 flex flex-wrap items-start justify-between gap-2">
				<div className="min-w-0">
					<h3 className="text-sm font-medium">{t("fragments.providerUsage.title")}</h3>
					<p className="text-muted-foreground mt-1 text-xs">{t("fragments.providerUsage.description")}</p>
				</div>
				<div className="flex items-center gap-1" role="group" aria-label={t("fragments.providerUsage.title")}>
					{WINDOW_OPTIONS.map((option) => (
						<Button
							key={option.minutes}
							type="button"
							size="sm"
							variant={option.minutes === windowMinutes ? "default" : "outline"}
							className="h-7 px-2 text-xs"
							onClick={() => setWindowMinutes(option.minutes)}
							data-testid={option.testId}
						>
							{t(option.labelKey)}
						</Button>
					))}
				</div>
			</div>

			{usage.isLoading && (
				<p data-testid="providers2-usage-loading" className="text-muted-foreground py-4 text-center text-xs">
					{t("fragments.providerUsage.loading")}
				</p>
			)}

			{!usage.isLoading && usage.error && (
				<p data-testid="providers2-usage-error" className="text-destructive py-4 text-center text-xs">
					{t("fragments.providerUsage.error")}
				</p>
			)}

			{!usage.isLoading && !usage.error && rows.length === 0 && (
				<p data-testid="providers2-usage-empty" className="text-muted-foreground py-4 text-center text-xs">
					{t("fragments.providerUsage.empty")}
				</p>
			)}

			{!usage.isLoading && !usage.error && rows.length > 0 && (
				<>
					<div data-testid="providers2-usage-summary" className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
						<SummaryMetric label={t("fragments.providerUsage.summary.requests")} value={formatInt(summary.requests)} />
						<SummaryMetric label={t("fragments.providerUsage.summary.errors")} value={formatInt(summary.errors)} />
						<SummaryMetric
							label={t("fragments.providerUsage.summary.errorRate")}
							value={formatErrorRate(summary.errors, summary.requests)}
						/>
						<SummaryMetric label={t("fragments.providerUsage.summary.totalTokens")} value={formatInt(summary.tokens)} />
						<SummaryMetric
							label={t("fragments.providerUsage.summary.tokensPerMinute")}
							value={formatInt(Math.round(summary.tokensPerMinute))}
						/>
					</div>

					<p data-testid="providers2-usage-count" className="text-muted-foreground mt-3 text-xs">
						{t("fragments.providerUsage.groupCount", { count: rows.length })}
					</p>

					<div
						data-testid="providers2-usage-table"
						className={cn("mt-1 overflow-x-auto rounded-md border", rows.length > SCROLL_THRESHOLD && "max-h-96 overflow-y-auto")}
					>
						<table className="w-full border-collapse text-xs">
							<thead className="bg-background sticky top-0 z-10">
								<tr className="text-muted-foreground border-b text-left">
									<th className="px-3 py-2 font-medium">{t("fragments.providerUsage.table.model")}</th>
									<th className="px-3 py-2 font-medium">{t("fragments.providerUsage.table.key")}</th>
									<th className="px-3 py-2 text-right font-medium">{t("fragments.providerUsage.table.requests")}</th>
									<th className="px-3 py-2 text-right font-medium">{t("fragments.providerUsage.table.errorRate")}</th>
									<th className="px-3 py-2 text-right font-medium">{t("fragments.providerUsage.table.tokensPerMinute")}</th>
									<th className="px-3 py-2 text-right font-medium">{t("fragments.providerUsage.table.avgTokens")}</th>
								</tr>
							</thead>
							<tbody>
								{rows.map((entry) => {
									const keyLabel = entry.key_name || entry.key_id || t("fragments.providerUsage.noKey");
									const hasErrors = (entry.errors ?? 0) > 0;
									return (
										<tr
											key={`${entry.model}|${entry.key_id}|${entry.key_name}`}
											data-testid="providers2-usage-row"
											className="hover:bg-muted/40 border-b last:border-0"
										>
											<td className="max-w-56 px-3 py-1.5">
												<span className="block truncate font-mono" title={entry.model}>
													{entry.model || "-"}
												</span>
											</td>
											<td className="max-w-40 px-3 py-1.5">
												<span className="text-muted-foreground block truncate" title={entry.key_id || entry.key_name}>
													{keyLabel}
												</span>
											</td>
											<td className="px-3 py-1.5 text-right font-mono">
												{formatInt(entry.requests)}
												{(entry.cancelled ?? 0) > 0 && (
													<span className="text-muted-foreground block text-[10px]">
														{t("fragments.providerUsage.table.cancelledNote", { count: entry.cancelled })}
													</span>
												)}
											</td>
											<td className={cn("px-3 py-1.5 text-right font-mono", hasErrors ? "text-destructive" : "text-muted-foreground")}>
												{formatErrorRate(entry.errors, entry.requests)}
											</td>
											<td className="px-3 py-1.5 text-right font-mono">{formatInt(Math.round(entry.tokens_per_minute ?? 0))}</td>
											<td className="px-3 py-1.5 text-right font-mono">{formatInt(Math.round(entry.avg_total_tokens ?? 0))}</td>
										</tr>
									);
								})}
							</tbody>
						</table>
					</div>
				</>
			)}

			<div data-testid="providers2-usage-baseline" className="mt-4 border-t pt-3">
				<div className="mb-2 flex flex-wrap items-center justify-between gap-2">
					<h4 className="text-xs font-medium">{baselineTitle}</h4>
					{patterns.data && (
						<span className="text-muted-foreground text-xs">
							{t("fragments.providerUsage.baseline.totalErrors", { count: patterns.data.total_errors })}
						</span>
					)}
				</div>

				{patterns.isLoading && (
					<p data-testid="providers2-usage-baseline-loading" className="text-muted-foreground py-2 text-xs">
						{t("fragments.providerUsage.baseline.loading")}
					</p>
				)}

				{!patterns.isLoading && patterns.error && (
					<p data-testid="providers2-usage-baseline-error" className="text-muted-foreground py-2 text-xs">
						{t("fragments.providerUsage.baseline.error")}
					</p>
				)}

				{!patterns.isLoading && !patterns.error && topPatterns.length === 0 && (
					<p data-testid="providers2-usage-baseline-empty" className="text-muted-foreground py-2 text-xs">
						{t("fragments.providerUsage.baseline.empty")}
					</p>
				)}

				{!patterns.isLoading && !patterns.error && topPatterns.length > 0 && (
					<div className="space-y-1">
						{topPatterns.map((pattern) => {
							const detail = pattern.error_code || pattern.error_type || pattern.sample_message;
							return (
								<div
									key={pattern.rank}
									data-testid="providers2-usage-baseline-row"
									className="bg-muted/30 flex items-center gap-2 rounded border px-2 py-1 text-xs"
								>
									<span className="text-muted-foreground font-mono">#{pattern.rank}</span>
									{pattern.status_code !== undefined && (
										<Badge variant={pattern.status_code >= 500 ? "destructive" : "outline"} className="font-mono">
											{pattern.status_code}
										</Badge>
									)}
									<span className="min-w-0 flex-1 truncate" title={detail || pattern.sample_message}>
										{detail || t("fragments.providerUsage.baseline.unknown")}
									</span>
									<span className="text-muted-foreground shrink-0 font-mono">
										{t("fragments.providerUsage.baseline.count", { count: pattern.count })}
									</span>
								</div>
							);
						})}
					</div>
				)}
			</div>
		</div>
	);
}

function SummaryMetric({ label, value }: { label: string; value: string }) {
	return (
		<div className="bg-muted/50 rounded-md px-3 py-2">
			<div className="text-muted-foreground text-[11px]">{label}</div>
			<div className="mt-0.5 font-mono text-sm font-semibold">{value}</div>
		</div>
	);
}