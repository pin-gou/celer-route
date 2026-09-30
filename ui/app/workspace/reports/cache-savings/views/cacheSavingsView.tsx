import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { getErrorMessage, useGetCacheSavingsQuery } from "@/lib/store";
import { CalendarClock, Coins, Database, Percent, RefreshCw, TrendingUp } from "lucide-react";
import { useTranslation } from "react-i18next";

function formatNumber(n: number | undefined): string {
	if (n === undefined || n === null) return "—";
	return new Intl.NumberFormat().format(n);
}

function formatCost(n: number | undefined): string {
	if (n === undefined || n === null) return "—";
	return `$${n.toFixed(2)}`;
}

function formatPercent(n: number | undefined): string {
	if (n === undefined || n === null) return "—";
	return `${(n * 100).toFixed(1)}%`;
}

const dateFormatter = new Intl.DateTimeFormat(undefined, {
	year: "numeric",
	month: "short",
	day: "2-digit",
	hour: "2-digit",
	minute: "2-digit",
});

// /workspace/reports/cache-savings — answers "how much did the semantic
// cache save us in the window?" by combining the log-store window view
// (cache hits + estimated savings) with the in-process tracker (since-boot
// hit rate). When the semantic cache plugin isn't loaded, the process view
// is null and only the window view renders.
export default function CacheSavingsView() {
	const { t } = useTranslation("reports");
	const { data, isLoading, isError, error, refetch, isFetching } = useGetCacheSavingsQuery();

	return (
		<div className="mx-auto max-w-5xl space-y-6 p-8" data-testid="cache-savings">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold" data-testid="cache-savings-title">
						<Database className="h-6 w-6" />
						{t("cacheSavings.title")}
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("cacheSavings.subtitle")}</p>
				</div>
				<button
					type="button"
					onClick={() => refetch()}
					className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-sm"
					data-testid="cache-savings-refresh"
				>
					<RefreshCw className={isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
					{t("refresh")}
				</button>
			</header>

			{isLoading && (
				<div className="flex min-h-[20vh] items-center justify-center" data-testid="cache-savings-loading">
					<RefreshCw className="text-muted-foreground h-6 w-6 animate-spin" />
				</div>
			)}

			{isError && (
				<Card>
					<CardContent className="py-6">
						<p className="text-destructive text-sm" data-testid="cache-savings-error">
							{getErrorMessage(error)}
						</p>
					</CardContent>
				</Card>
			)}

			{!isLoading && !isError && data && (
				<>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-3" data-testid="cache-savings-grid">
						<Card>
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<TrendingUp className="h-3.5 w-3.5" />
									{t("cacheSavings.cacheHitRequests")}
								</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-2xl font-semibold" data-testid="cache-savings-hit-requests">
									{formatNumber(data.window.cache_hit_requests)}
								</p>
							</CardContent>
						</Card>

						<Card>
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<CalendarClock className="h-3.5 w-3.5" />
									{t("cacheSavings.cacheHitTokens")}
								</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-2xl font-semibold" data-testid="cache-savings-hit-tokens">
									{formatNumber(data.window.cache_hit_input_tokens)}
								</p>
							</CardContent>
						</Card>

						<Card>
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<Coins className="h-3.5 w-3.5" />
									{t("cacheSavings.estimatedSavings")}
								</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-2xl font-semibold text-green-600" data-testid="cache-savings-amount">
									{formatCost(data.window.estimated_savings)}
								</p>
								<p className="text-muted-foreground mt-1 text-xs">{data.currency}</p>
							</CardContent>
						</Card>
					</div>

					{data.process && (
						<Card>
							<CardHeader>
								<CardTitle className="flex items-center gap-2 text-base">
									<Percent className="h-4 w-4" />
									{t("cacheSavings.processTitle")}
								</CardTitle>
								<CardDescription>{t("cacheSavings.processDesc")}</CardDescription>
							</CardHeader>
							<CardContent>
								<div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
									<div>
										<p className="text-muted-foreground text-xs">{t("cacheSavings.hits")}</p>
										<p className="text-xl font-semibold" data-testid="cache-savings-process-hits">
											{formatNumber(data.process.cache_hits)}
										</p>
									</div>
									<div>
										<p className="text-muted-foreground text-xs">{t("cacheSavings.misses")}</p>
										<p className="text-xl font-semibold" data-testid="cache-savings-process-misses">
											{formatNumber(data.process.cache_misses)}
										</p>
									</div>
									<div>
										<p className="text-muted-foreground text-xs">{t("cacheSavings.hitRate")}</p>
										<p className="text-xl font-semibold" data-testid="cache-savings-process-rate">
											{formatPercent(data.process.hit_rate)}
										</p>
									</div>
								</div>
							</CardContent>
						</Card>
					)}

					<Card>
						<CardContent className="py-4">
							<p className="text-muted-foreground text-xs">
								{dateFormatter.format(new Date(data.period.start))} — {dateFormatter.format(new Date(data.period.end))}
							</p>
						</CardContent>
					</Card>
				</>
			)}
		</div>
	);
}