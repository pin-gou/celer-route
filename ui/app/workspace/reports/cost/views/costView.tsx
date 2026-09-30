import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { getErrorMessage, useGetCostForecastQuery, useGetCostSummaryQuery } from "@/lib/store";
import { AlertTriangle, BarChart3, Coins, RefreshCw, TrendingDown, TrendingUp } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";

function formatNumber(n: number | undefined): string {
	if (n === undefined || n === null) return "—";
	return new Intl.NumberFormat().format(n);
}

function formatCost(n: number | undefined): string {
	if (n === undefined || n === null) return "—";
	return `$${n.toFixed(2)}`;
}

// /workspace/reports/cost — admin cost-allocation dashboard. Shows the
// cost summary (totals + top dimensions), trend forecast, and a panel
// explaining what's been implemented vs. what's deferred. The trend chart
// reuses the buckets from /api/logs/histogram/cost/by-dimension when the
// log store exposes them.
// Dimension selector for the cost summary. The backend accepts
// team | customer | user | virtual_key | app | user_agent (and
// business_unit). The UI exposes the most useful three by default;
// "app" is US8's entry point — slice spend by which backend-detected
// client app produced the request (claude-cowork, codex, etc.).
const COST_DIMENSIONS: Array<{ value: string; labelKey: string }> = [
	{ value: "team_id", labelKey: "dimTeam" },
	{ value: "user_id", labelKey: "dimUser" },
	{ value: "virtual_key_id", labelKey: "dimVK" },
	{ value: "app", labelKey: "dimApp" },
	{ value: "provider", labelKey: "dimProvider" },
];

export default function CostView() {
	const { t } = useTranslation("reports");
	const [dimension, setDimension] = useState("team_id");
	const { data, isLoading, isError, error, refetch, isFetching } = useGetCostSummaryQuery({ dimension });
	const forecast = useGetCostForecastQuery({ dimension });

	const total = data?.total;
	const risk = forecast.data?.risk ?? "low";

	const riskColor = risk === "high" ? "text-red-600" : risk === "medium" ? "text-yellow-600" : "text-green-600";

	return (
		<div className="mx-auto max-w-5xl space-y-6 p-8" data-testid="cost-report">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold" data-testid="cost-report-title">
						<Coins className="h-6 w-6" />
						{t("cost.title")}
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("cost.subtitle")}</p>
				</div>
				<div className="flex items-center gap-2">
					<Select value={dimension} onValueChange={setDimension}>
						<SelectTrigger className="w-44" data-testid="cost-report-dimension">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{COST_DIMENSIONS.map((d) => (
								<SelectItem key={d.value} value={d.value}>
									{t(`cost.${d.labelKey}`)}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
					<button
						type="button"
						onClick={() => refetch()}
						className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-sm"
						data-testid="cost-report-refresh"
					>
						<RefreshCw className={isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
						{t("refresh")}
					</button>
				</div>
			</header>

			{isLoading && (
				<div className="flex min-h-[20vh] items-center justify-center" data-testid="cost-report-loading">
					<RefreshCw className="text-muted-foreground h-6 w-6 animate-spin" />
				</div>
			)}

			{isError && (
				<Card>
					<CardContent className="py-6">
						<p className="text-destructive text-sm" data-testid="cost-report-error">
							{getErrorMessage(error)}
						</p>
					</CardContent>
				</Card>
			)}

			{!isLoading && !isError && (
				<>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-3" data-testid="cost-report-grid">
						<Card>
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<Coins className="h-3.5 w-3.5" />
									{t("cost.observedTotal")}
								</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-2xl font-semibold" data-testid="cost-report-total">
									{formatCost(total?.cost)}
								</p>
								<p className="text-muted-foreground mt-1 text-xs">{data?.currency ?? "USD"}</p>
							</CardContent>
						</Card>

						<Card>
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<TrendingUp className="h-3.5 w-3.5" />
									{t("cost.projectedTotal")}
								</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-2xl font-semibold" data-testid="cost-report-forecast">
									{formatCost(forecast.data?.projected_total)}
								</p>
								<p className={`mt-1 text-xs ${riskColor}`} data-testid="cost-report-risk">
									{t(`cost.risk.${risk}`)}
								</p>
								<Link
									to="/workspace/reports/budget-forecast"
									className="mt-2 inline-flex items-center gap-1 text-xs text-blue-600 hover:underline dark:text-blue-400"
									data-testid="cost-report-forecast-link"
								>
									<TrendingDown className="h-3 w-3" />
									{t("cost.budgetForecastLink")}
								</Link>
							</CardContent>
						</Card>

						<Card>
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<BarChart3 className="h-3.5 w-3.5" />
									{t("cost.dimensions")}
								</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-2xl font-semibold" data-testid="cost-report-rows-count">
									{formatNumber(data?.rows.length)}
								</p>
								<p className="text-muted-foreground mt-1 text-xs">{t("cost.dimensionsHint")}</p>
							</CardContent>
						</Card>
					</div>

					<Tabs defaultValue="rows">
						<TabsList>
							<TabsTrigger value="rows">{t("cost.tabRows")}</TabsTrigger>
							<TabsTrigger value="notes">{t("cost.tabNotes")}</TabsTrigger>
						</TabsList>

						<TabsContent value="rows">
							<Card>
								<CardHeader>
									<CardTitle>{t("cost.rowsTitle")}</CardTitle>
									<CardDescription>{t("cost.rowsDesc")}</CardDescription>
								</CardHeader>
								<CardContent>
									{data?.rows && data.rows.length > 0 ? (
										<div className="space-y-2" data-testid="cost-report-rows">
											{data.rows.slice(0, 50).map((row, i) => (
												<div key={`${row.id}-${i}`} className="flex items-center justify-between gap-2 border-b py-2 text-sm">
													<div className="min-w-0 flex-1">
														<p className="truncate font-medium">{row.name ?? row.id}</p>
														<p className="text-muted-foreground text-xs">{row.id}</p>
													</div>
													<div className="text-right">
														<p className="font-medium tabular-nums">{formatCost(row.cost)}</p>
														<p className="text-muted-foreground text-xs">
															{formatNumber(row.requests)} req · {formatNumber(row.tokens)} tok
														</p>
													</div>
												</div>
											))}
										</div>
									) : (
										<p className="text-muted-foreground text-sm" data-testid="cost-report-rows-empty">
											{t("cost.rowsEmpty")}
										</p>
									)}
								</CardContent>
							</Card>
						</TabsContent>

						<TabsContent value="notes">
							<Card>
								<CardHeader>
									<CardTitle className="flex items-center gap-2 text-base">
										<AlertTriangle className="h-4 w-4" />
										{t("cost.notesTitle")}
									</CardTitle>
								</CardHeader>
								<CardContent className="text-muted-foreground space-y-2 text-sm">
									<p>{t("cost.note1")}</p>
									<p>{t("cost.note2")}</p>
									<p>{t("cost.note3")}</p>
								</CardContent>
							</Card>
						</TabsContent>
					</Tabs>
				</>
			)}
		</div>
	);
}