import { useTranslation } from "react-i18next";
import type { ProviderUsageEntry } from "@/lib/store/apis/providerUsageApi";
import { cn } from "@/lib/utils";

/**
 * The usage snapshot the cooldown tab collected for the selected window.
 *
 * The query itself lives in CooldownTab: the window is a tab-level control
 * and the same snapshot feeds the summary strip, the empty state and this
 * table, so fetching it once at the tab level avoids three components
 * disagreeing about which window they are describing.
 */
export interface ProviderUsageSnapshot {
	entries: ProviderUsageEntry[];
	isLoading: boolean;
	/** Whether the usage query failed (the card only needs the fact). */
	hasError: boolean;
	/** Epoch milliseconds of the last successful fetch ("updated at"). */
	updatedAt?: number;
}

// Above this many (model × key) groups the table becomes height-capped and
// scrolls instead of pushing the rest of the page off-screen.
const SCROLL_THRESHOLD = 20;

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

/**
 * Per-model × key consumption detail for one provider.
 *
 * The headline numbers (requests, error rate, tokens) live in the tab's
 * summary strip — they are the same row of data as this table, so rendering
 * them in both places would be the same duplication this page was cleaned up
 * for. What is left here is the breakdown that actually says WHICH key is
 * misbehaving.
 */
export function ProviderUsageCard({ usage }: { usage: ProviderUsageSnapshot }) {
	const { t } = useTranslation("providers");
	const rows = usage.entries;

	return (
		<div data-testid="providers2-usage-card" className="rounded-lg border p-4">
			<div className="mb-3">
				<h3 className="text-sm font-medium">{t("fragments.providerUsage.title")}</h3>
				<p className="text-muted-foreground mt-1 text-xs">{t("fragments.providerUsage.description")}</p>
			</div>

			{usage.isLoading && (
				<p data-testid="providers2-usage-loading" className="text-muted-foreground py-4 text-center text-xs">
					{t("fragments.providerUsage.loading")}
				</p>
			)}

			{!usage.isLoading && usage.hasError && (
				<p data-testid="providers2-usage-error" className="text-destructive py-4 text-center text-xs">
					{t("fragments.providerUsage.error")}
				</p>
			)}

			{!usage.isLoading && !usage.hasError && rows.length === 0 && (
				<p data-testid="providers2-usage-empty" className="text-muted-foreground py-4 text-center text-xs">
					{t("fragments.providerUsage.empty")}
				</p>
			)}

			{!usage.isLoading && !usage.hasError && rows.length > 0 && (
				<>
					<p data-testid="providers2-usage-count" className="text-muted-foreground mb-1 text-xs">
						{t("fragments.providerUsage.groupCount", { count: rows.length })}
					</p>

					<div
						data-testid="providers2-usage-table"
						className={cn("overflow-x-auto rounded-md border", rows.length > SCROLL_THRESHOLD && "max-h-96 overflow-y-auto")}
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
		</div>
	);
}