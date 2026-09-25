import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { getErrorMessage, useGetBudgetsQuery, useGetBudgetProjectionQuery } from "@/lib/store";
import { Coins, RefreshCw, TrendingDown, TrendingUp } from "lucide-react";
import { useTranslation } from "react-i18next";

function formatCost(n: number | undefined): string {
	if (n === undefined || n === null) return "—";
	return `$${n.toFixed(2)}`;
}

function formatPercent(n: number | undefined): string {
	if (n === undefined || n === null) return "—";
	return `${n.toFixed(1)}%`;
}

const dateFormatter = new Intl.DateTimeFormat(undefined, {
	year: "numeric",
	month: "short",
	day: "2-digit",
	hour: "2-digit",
	minute: "2-digit",
});

const riskVariant: Record<string, "default" | "secondary" | "destructive" | "outline" | "success" | "warning"> = {
	low: "success",
	medium: "warning",
	high: "destructive",
};

// BudgetProjectionRow — renders a single budget's projection. The query
// is enabled only when the budget id is known, so budgets with no
// projection endpoint data still render the used/max + risk fallback.
function BudgetProjectionRow({ budgetId }: { budgetId: string }) {
	const { t } = useTranslation("reports");
	const { data, isLoading, isError, error } = useGetBudgetProjectionQuery(budgetId);

	if (isLoading) {
		return (
			<Card>
				<CardContent className="flex items-center gap-2 py-4">
					<RefreshCw className="text-muted-foreground h-4 w-4 animate-spin" />
					<span className="text-muted-foreground text-sm">{t("loading")}</span>
				</CardContent>
			</Card>
		);
	}

	if (isError) {
		return (
			<Card>
				<CardContent className="py-4">
					<p className="text-destructive text-sm" data-testid={`budget-forecast-error-${budgetId}`}>
						{getErrorMessage(error)}
					</p>
				</CardContent>
			</Card>
		);
	}

	if (!data) return null;

	const risk = data.risk_level ?? data.projection?.risk_level ?? "low";
	const usagePct = data.usage_percent;
	const barWidth = Math.min(100, Math.max(0, usagePct));
	const barColor = usagePct >= 90 ? "bg-red-500" : usagePct >= 80 ? "bg-yellow-500" : "bg-green-500";

	return (
		<Card data-testid={`budget-forecast-row-${budgetId}`}>
			<CardHeader className="pb-2">
				<div className="flex items-center justify-between gap-2">
					<div>
						<CardTitle className="flex items-center gap-2 text-sm">
							<Coins className="h-4 w-4" />
							<span className="font-mono text-xs">{budgetId}</span>
						</CardTitle>
					</div>
					<Badge variant={riskVariant[risk] ?? "secondary"} data-testid={`budget-forecast-risk-${budgetId}`}>
						{t(`cost.risk.${risk}`)}
					</Badge>
				</div>
			</CardHeader>
			<CardContent className="space-y-3">
				{/* Usage progress bar */}
				<div className="space-y-1">
					<div className="flex items-center justify-between text-xs">
						<span className="text-muted-foreground">{t("budgetForecast.usage")}</span>
						<span className="font-medium" data-testid={`budget-forecast-usage-${budgetId}`}>
							{formatPercent(usagePct)}
						</span>
					</div>
					<div className="bg-muted h-2 overflow-hidden rounded-full">
						<div className={`h-full rounded-full ${barColor}`} style={{ width: `${barWidth}%` }} />
					</div>
				</div>

				{/* Used / Max */}
				<div className="flex items-center justify-between text-sm">
					<span className="text-muted-foreground">{t("budgetForecast.used")}</span>
					<span className="font-medium">
						{formatCost(data.used_amount)} / {formatCost(data.max_amount)}
					</span>
				</div>

				{/* Projection */}
				{data.has_projection && data.projection ? (
					<div className="space-y-1 border-t pt-2">
						<div className="flex items-center justify-between text-sm">
							<span className="text-muted-foreground flex items-center gap-1">
								<TrendingUp className="h-3.5 w-3.5" />
								{t("budgetForecast.ratePerHour")}
							</span>
							<span className="font-medium" data-testid={`budget-forecast-rate-${budgetId}`}>
								{formatCost(data.projection.rate_per_hour)}/h
							</span>
						</div>
						{data.projection.hours_to_exhaustion !== undefined && (
							<div className="flex items-center justify-between text-sm">
								<span className="text-muted-foreground flex items-center gap-1">
									<TrendingDown className="h-3.5 w-3.5" />
									{t("budgetForecast.exhaustion")}
								</span>
								<span className="font-medium" data-testid={`budget-forecast-exhaustion-${budgetId}`}>
									{data.projection.hours_to_exhaustion <= 0
										? t("budgetForecast.exhausted")
										: data.projection.window_ends_at
											? dateFormatter.format(new Date(data.projection.window_ends_at))
											: `~${data.projection.hours_to_exhaustion.toFixed(0)}h`}
								</span>
							</div>
						)}
						{data.sampled_at && (
							<p className="text-muted-foreground text-xs">
								{t("budgetForecast.lastSampled")} {dateFormatter.format(new Date(data.sampled_at))}
							</p>
						)}
					</div>
				) : (
					<div className="border-t pt-2">
						<p className="text-muted-foreground text-xs" data-testid={`budget-forecast-no-projection-${budgetId}`}>
							{data.reason ?? t("budgetForecast.noProjectionReason")}
						</p>
					</div>
				)}
			</CardContent>
		</Card>
	);
}

// /workspace/reports/budget-forecast — US17. Lists every budget and shows
// the predicted exhaustion time + risk level from
// GET /api/governance/budgets/{id}/projection (hourly budget_snapshots
// feed the linear-fit projection). Budgets with no snapshots degrade to
// used/max + risk only (has_projection=false).
export default function BudgetForecastView() {
	const { t } = useTranslation("reports");
	const { data, isLoading, isError, error, refetch, isFetching } = useGetBudgetsQuery();

	const budgets = data?.budgets ?? [];

	return (
		<div className="mx-auto max-w-5xl space-y-6 p-8" data-testid="budget-forecast">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold" data-testid="budget-forecast-title">
						<TrendingDown className="h-6 w-6" />
						{t("budgetForecast.title")}
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("budgetForecast.subtitle")}</p>
				</div>
				<button
					type="button"
					onClick={() => refetch()}
					className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-sm"
					data-testid="budget-forecast-refresh"
				>
					<RefreshCw className={isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
					{t("refresh")}
				</button>
			</header>

			{isLoading && (
				<div className="flex min-h-[20vh] items-center justify-center" data-testid="budget-forecast-loading">
					<RefreshCw className="text-muted-foreground h-6 w-6 animate-spin" />
				</div>
			)}

			{isError && (
				<Card>
					<CardContent className="py-6">
						<p className="text-destructive text-sm" data-testid="budget-forecast-load-error">
							{getErrorMessage(error)}
						</p>
					</CardContent>
				</Card>
			)}

			{!isLoading && !isError && budgets.length === 0 && (
				<Card>
					<CardContent className="py-6">
						<p className="text-muted-foreground text-sm" data-testid="budget-forecast-empty">
							{t("budgetForecast.empty")}
						</p>
					</CardContent>
				</Card>
			)}

			{!isLoading && !isError && budgets.length > 0 && (
				<div className="grid grid-cols-1 gap-4 md:grid-cols-2" data-testid="budget-forecast-grid">
					{budgets.map((b) => (
						<BudgetProjectionRow key={b.id} budgetId={b.id} />
					))}
				</div>
			)}

			<Card>
				<CardHeader>
					<CardTitle className="text-sm">{t("budgetForecast.notesTitle")}</CardTitle>
				</CardHeader>
				<CardContent>
					<p className="text-muted-foreground text-xs">{t("budgetForecast.note1")}</p>
				</CardContent>
			</Card>
		</div>
	);
}