import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { getErrorMessage, useListIdleKeysQuery } from "@/lib/store";
import { AlertCircle, CalendarClock, KeyRound, RefreshCw, Search } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

const dateFormatter = new Intl.DateTimeFormat(undefined, {
	year: "numeric",
	month: "short",
	day: "2-digit",
	hour: "2-digit",
	minute: "2-digit",
});

// /workspace/reports/idle-keys — admin-only "which VKs have not been used
// for N days" report. The threshold is exposed as a slider; backend
// caps at 365 days. NULL last_used_at ("never used") is surfaced with a
// separate badge so brand-new unshared keys show up.
export default function IdleKeysView() {
	const { t } = useTranslation("reports");
	const [idleDays, setIdleDays] = useState(30);
	const [search, setSearch] = useState("");

	const { data, isLoading, isError, error, refetch, isFetching } = useListIdleKeysQuery({ idle_days: idleDays });

	const filteredRows = data?.rows.filter((row) => {
		if (!search.trim()) return true;
		const lower = search.toLowerCase();
		return row.name.toLowerCase().includes(lower) || row.id.toLowerCase().includes(lower);
	});

	return (
		<div className="mx-auto max-w-5xl space-y-6 p-8" data-testid="idle-keys">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold" data-testid="idle-keys-title">
						<KeyRound className="h-6 w-6" />
						{t("idleKeys.title")}
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("idleKeys.subtitle")}</p>
				</div>
				<button
					type="button"
					onClick={() => refetch()}
					className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-sm"
					data-testid="idle-keys-refresh"
				>
					<RefreshCw className={isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
					{t("refresh")}
				</button>
			</header>

			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2 text-base">
						<CalendarClock className="h-4 w-4" />
						{t("idleKeys.thresholdTitle")}
					</CardTitle>
					<CardDescription>{t("idleKeys.thresholdDesc")}</CardDescription>
				</CardHeader>
				<CardContent className="space-y-3">
					<div className="flex items-center gap-3">
						<Input
							type="number"
							min={1}
							max={365}
							value={idleDays}
							onChange={(e) => setIdleDays(Math.max(1, Math.min(365, Number(e.target.value) || 30)))}
							className="w-24"
							data-testid="idle-keys-threshold"
						/>
						<span className="text-muted-foreground text-sm">{t("idleKeys.daysLabel", { days: idleDays })}</span>
					</div>
					<div className="flex items-center gap-2">
						<Search className="text-muted-foreground h-4 w-4" />
						<Input
							type="text"
							placeholder={t("idleKeys.searchPlaceholder")}
							value={search}
							onChange={(e) => setSearch(e.target.value)}
							className="max-w-xs"
							data-testid="idle-keys-search"
						/>
					</div>
				</CardContent>
			</Card>

			{isLoading && (
				<div className="flex min-h-[20vh] items-center justify-center" data-testid="idle-keys-loading">
					<RefreshCw className="text-muted-foreground h-6 w-6 animate-spin" />
				</div>
			)}

			{isError && (
				<Card>
					<CardContent className="py-6">
						<p className="text-destructive text-sm" data-testid="idle-keys-error">
							{getErrorMessage(error)}
						</p>
					</CardContent>
				</Card>
			)}

			{!isLoading && !isError && data && (
				<>
					<Card>
						<CardHeader>
							<CardTitle className="text-base">{t("idleKeys.summaryTitle")}</CardTitle>
							<CardDescription>{t("idleKeys.summaryDesc", { count: data.total, threshold: data.threshold })}</CardDescription>
						</CardHeader>
						<CardContent>
							<p className="text-3xl font-semibold" data-testid="idle-keys-total">
								{data.total}
							</p>
						</CardContent>
					</Card>

					{filteredRows && filteredRows.length > 0 ? (
						<div className="space-y-2" data-testid="idle-keys-list">
							{filteredRows.slice(0, 100).map((row) => (
								<Card key={row.id} data-testid={`idle-keys-row-${row.id}`}>
									<CardContent className="py-3">
										<div className="flex flex-wrap items-start justify-between gap-2">
											<div className="min-w-0 flex-1">
												<div className="flex items-center gap-2">
													<p className="truncate font-medium">{row.name}</p>
													{row.last_used_at === null && (
														<Badge variant="outline" className="text-xs">
															<AlertCircle className="mr-1 h-3 w-3" />
															{t("idleKeys.neverUsed")}
														</Badge>
													)}
													{!row.is_active && (
														<Badge variant="secondary" className="text-xs">
															{t("idleKeys.inactive")}
														</Badge>
													)}
												</div>
												<p className="text-muted-foreground text-xs">{row.id}</p>
												{row.description && <p className="text-muted-foreground mt-1 text-xs">{row.description}</p>}
											</div>
											<div className="text-right text-xs">
												<p className="text-muted-foreground">{t("idleKeys.lastUsed")}</p>
												<p className="font-medium">
													{row.last_used_at ? dateFormatter.format(new Date(row.last_used_at)) : t("idleKeys.never")}
												</p>
											</div>
										</div>
									</CardContent>
								</Card>
							))}
						</div>
					) : (
						<Card>
							<CardContent className="py-8 text-center">
								<p className="text-muted-foreground text-sm" data-testid="idle-keys-empty">
									{t("idleKeys.empty")}
								</p>
							</CardContent>
						</Card>
					)}
				</>
			)}
		</div>
	);
}