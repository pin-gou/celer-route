import { getQueryErrorStatus } from "@/components/providerCooldown/queryError";
import { ProviderLabels } from "@/lib/constants/logs";
import { useGetCapacityUsageQuery } from "@/lib/store/apis/pluginsApi";
import type { CapacityUsageEntry } from "@/lib/types/plugins";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

const CAPACITY_USAGE_POLLING_INTERVAL_MS = 10_000;

function formatInt(value: number): string {
	return value.toLocaleString("en-US");
}

/** `used / threshold` with a percentage, or "—" when the metric has no limit. */
function formatMeter(used: number, threshold: number): string {
	if (threshold <= 0) return "—";
	const percent = Math.min(100, Math.round((used / threshold) * 100));
	return `${formatInt(used)} / ${formatInt(threshold)} · ${percent}%`;
}

function rowKey(entry: CapacityUsageEntry): string {
	return `${entry.provider}|${entry.scope}|${entry.model}`;
}

/**
 * Read-only fleet-wide "water level" of the capacity gate.
 *
 * Data comes from the plugin's live accounting endpoint, so it covers every
 * provider that has traffic in the current window — including providers that
 * inherit the deprecated plugin-level (config.json) default. There is nothing
 * to edit here: the capacity gate is configured per provider on the provider
 * detail page, and this table exists to show what those gates are doing right
 * now.
 *
 * A 400 means the provider-cooldown plugin is not loaded; the whole block is
 * hidden then, since an un-actionable error on a plugin page helps nobody.
 */
export function CapacityUsageTable() {
	const { t } = useTranslation("plugins");
	const navigate = useNavigate();
	const { data, error, isLoading } = useGetCapacityUsageQuery(undefined, {
		pollingInterval: CAPACITY_USAGE_POLLING_INTERVAL_MS,
	});

	if (getQueryErrorStatus(error) === 400) return null;

	const entries = data?.entries ?? [];

	const openProvider = (provider: string) => {
		navigate({
			to: "/workspace/providers/$id",
			params: { id: provider },
			search: { tab: "cooldown" },
		});
	};

	return (
		<div className="rounded-lg border p-4">
			<h4 className="mb-1 text-sm font-medium">{t("providerCooldown.capacityUsageTitle")}</h4>
			<p className="text-muted-foreground mb-4 text-sm">{t("providerCooldown.capacityUsageDescription")}</p>

			{isLoading && (
				<p data-testid="providercooldown-capacity-usage-loading" className="text-muted-foreground py-4 text-center text-sm">
					{t("providerCooldown.capacityUsageLoading")}
				</p>
			)}

			{!isLoading && error && (
				<p data-testid="providercooldown-capacity-usage-error" className="text-destructive py-4 text-center text-sm">
					{t("providerCooldown.capacityUsageError")}
				</p>
			)}

			{!isLoading && !error && entries.length === 0 && (
				<p data-testid="providercooldown-capacity-usage-empty" className="text-muted-foreground py-4 text-center text-sm">
					{t("providerCooldown.capacityUsageEmpty")}
				</p>
			)}

			{!isLoading && !error && entries.length > 0 && (
				<div className="overflow-x-auto">
					<table data-testid="providercooldown-capacity-usage-table" className="w-full text-xs">
						<thead>
							<tr className="text-muted-foreground border-b text-left">
								<th className="py-2 pr-3 font-medium">{t("providerCooldown.capacityUsageProvider")}</th>
								<th className="py-2 pr-3 font-medium">{t("providerCooldown.capacityUsageModel")}</th>
								<th className="py-2 pr-3 font-medium">{t("providerCooldown.capacityUsageTokens")}</th>
								<th className="py-2 pr-3 font-medium">{t("providerCooldown.capacityUsageRequests")}</th>
								<th className="py-2 pr-3 font-medium">{t("providerCooldown.capacityUsageWindow")}</th>
								<th className="py-2 pr-3 font-medium">{t("providerCooldown.capacityUsageStatus")}</th>
								<th className="py-2 font-medium">{t("providerCooldown.capacityUsageSource")}</th>
							</tr>
						</thead>
						<tbody>
							{entries.map((entry) => {
								const key = rowKey(entry);
								const modelLabel = entry.scope === "provider" ? t("providerCooldown.capacityUsageAllModels") : entry.model;
								const engaged = entry.retryAfterSeconds > 0;
								return (
									<tr
										key={key}
										data-testid={`providercooldown-capacity-usage-row-${entry.provider}-${entry.model || "all"}`}
										className="border-b last:border-b-0"
									>
										<td className="py-2 pr-3">
											<button
												type="button"
												data-testid={`providercooldown-capacity-usage-provider-${entry.provider}-${entry.model || "all"}`}
												onClick={() => openProvider(entry.provider)}
												className="font-medium underline-offset-2 hover:underline"
											>
												{ProviderLabels[entry.provider as keyof typeof ProviderLabels] ?? entry.provider}
											</button>
										</td>
										<td className="text-muted-foreground py-2 pr-3 font-mono">{modelLabel}</td>
										<td className="py-2 pr-3 font-mono">{formatMeter(entry.usedTokens, entry.thresholdTokens)}</td>
										<td className="py-2 pr-3 font-mono">{formatMeter(entry.usedRequests, entry.thresholdRequests)}</td>
										<td className="text-muted-foreground py-2 pr-3">{entry.windowSeconds}</td>
										<td className="py-2 pr-3">
											{engaged ? (
												<span className="text-amber-600 dark:text-amber-500">
													{t("providerCooldown.capacityUsageEngaged", { seconds: entry.retryAfterSeconds })}
												</span>
											) : (
												<span className="text-muted-foreground">{t("providerCooldown.capacityUsageNormal")}</span>
											)}
										</td>
										<td className="py-2">
											<span className={entry.source === "provider" ? "text-muted-foreground" : "text-amber-600 dark:text-amber-500"}>
												{entry.source === "provider"
													? t("providerCooldown.capacityUsageSourceProvider")
													: t("providerCooldown.capacityUsageSourceLegacy")}
											</span>
										</td>
									</tr>
								);
							})}
						</tbody>
					</table>
				</div>
			)}
		</div>
	);
}