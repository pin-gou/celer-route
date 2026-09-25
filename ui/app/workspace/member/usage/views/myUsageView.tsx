import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { getErrorMessage, useMemberUsageQuery, useMemberVirtualKeysQuery } from "@/lib/store";
import { Activity, BarChart3, CalendarClock, Coins, KeyRound, MessageSquare, RefreshCw, TrendingUp } from "lucide-react";
import { useTranslation } from "react-i18next";

const dateFormatter = new Intl.DateTimeFormat(undefined, {
	year: "numeric",
	month: "short",
	day: "2-digit",
	hour: "2-digit",
	minute: "2-digit",
});

function formatNumber(n: number | undefined): string {
	if (n === undefined || n === null) return "—";
	return new Intl.NumberFormat().format(n);
}

function formatCost(n: number | undefined): string {
	if (n === undefined || n === null) return "—";
	return `$${n.toFixed(4)}`;
}

// /workspace/member/usage — shows the current month's usage for the
// logged-in member. Cost + token + request histograms are queried
// server-side filtered by the member's own VK IDs; when no VKs are
// assigned or the log store isn't configured, the cards render an
// empty state instead of erroring.
export default function MyUsageView() {
	const { t } = useTranslation("governance-ui");
	const { data, isLoading, isError, error, refetch, isFetching } = useMemberUsageQuery();
	const vks = useMemberVirtualKeysQuery();
	const vkCount = vks.data?.count ?? 0;

	const usage = data?.usage;
	const hasData = usage && (usage.request_count !== undefined || usage.total_cost !== undefined);

	return (
		<div className="mx-auto max-w-4xl space-y-6 p-8" data-testid="member-usage">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold" data-testid="member-usage-title">
						<Activity className="h-6 w-6" />
						{t("users.memberUsage.title")}
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("users.memberUsage.subtitle")}</p>
				</div>
				<button
					type="button"
					onClick={() => refetch()}
					className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-sm"
					data-testid="member-usage-refresh"
					aria-label={t("users.memberPortal.retry")}
				>
					<RefreshCw className={isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
					{t("users.memberPortal.retry")}
				</button>
			</header>

			{isLoading ? (
				<div className="flex min-h-[20vh] items-center justify-center" data-testid="member-usage-loading">
					<RefreshCw className="text-muted-foreground h-5 w-5 animate-spin" />
				</div>
			) : isError || !data ? (
				<Card>
					<CardHeader>
						<CardTitle>{t("users.memberPortal.loadFailed")}</CardTitle>
						<CardDescription>{getErrorMessage(error)}</CardDescription>
					</CardHeader>
				</Card>
			) : (
				<>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4" data-testid="member-usage-grid">
						<Card>
							<CardHeader className="pb-2">
								<CardTitle className="flex items-center gap-2 text-sm">
									<KeyRound className="h-4 w-4" />
									{t("users.memberUsage.activeKeys")}
								</CardTitle>
								<CardDescription>{t("users.memberUsage.activeKeysHint")}</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-3xl font-semibold" data-testid="member-usage-vk-count">
									{formatNumber(vkCount)}
								</p>
							</CardContent>
						</Card>

						<Card>
							<CardHeader className="pb-2">
								<CardTitle className="flex items-center gap-2 text-sm">
									<Coins className="h-4 w-4" />
									{t("users.memberUsage.totalCost")}
								</CardTitle>
								<CardDescription>{t("users.memberUsage.totalCostHint")}</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-3xl font-semibold" data-testid="member-usage-total-cost">
									{hasData ? formatCost(usage.total_cost) : "—"}
								</p>
							</CardContent>
						</Card>

						<Card>
							<CardHeader className="pb-2">
								<CardTitle className="flex items-center gap-2 text-sm">
									<MessageSquare className="h-4 w-4" />
									{t("users.memberUsage.totalTokens")}
								</CardTitle>
								<CardDescription>{t("users.memberUsage.totalTokensHint")}</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-3xl font-semibold" data-testid="member-usage-total-tokens">
									{hasData ? formatNumber(usage.total_tokens) : "—"}
								</p>
							</CardContent>
						</Card>

						<Card>
							<CardHeader className="pb-2">
								<CardTitle className="flex items-center gap-2 text-sm">
									<TrendingUp className="h-4 w-4" />
									{t("users.memberUsage.requestCount")}
								</CardTitle>
								<CardDescription>{t("users.memberUsage.requestCountHint")}</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-3xl font-semibold" data-testid="member-usage-request-count">
									{hasData ? formatNumber(usage.request_count) : "—"}
								</p>
							</CardContent>
						</Card>
					</div>

					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2 text-base">
								<CalendarClock className="h-4 w-4" />
								{t("users.memberUsage.lastLogin")}
							</CardTitle>
							<CardDescription>{t("users.memberUsage.lastLoginHint")}</CardDescription>
						</CardHeader>
						<CardContent>
							<p className="text-base font-medium" data-testid="member-usage-last-login">
								{data.last_login_at ? dateFormatter.format(new Date(data.last_login_at)) : t("users.memberPortal.never")}
							</p>
						</CardContent>
					</Card>

					{hasData && usage.cost_buckets && usage.cost_buckets.length > 0 ? (
						<Card>
							<CardHeader>
								<CardTitle className="flex items-center gap-2 text-base" data-testid="member-usage-cost-chart">
									<BarChart3 className="h-4 w-4" />
									{t("users.memberUsage.costTrend")}
								</CardTitle>
								<CardDescription>
									{usage.period_start && usage.period_end && (
										<span className="flex items-center gap-1 text-xs">
											<CalendarClock className="h-3 w-3" />
											{dateFormatter.format(new Date(usage.period_start))} — {dateFormatter.format(new Date(usage.period_end))}
										</span>
									)}
								</CardDescription>
							</CardHeader>
							<CardContent>
								<div className="space-y-1" data-testid="member-usage-cost-bars">
									{usage.cost_buckets.slice(0, 30).map((bucket, i) => {
										const cost = bucket.total_cost ?? 0;
										const maxCost = Math.max(...usage.cost_buckets!.map((b) => b.total_cost ?? 0), 0.01);
										const widthPct = Math.max((cost / maxCost) * 100, 1);
										return (
											<div key={i} className="flex items-center gap-2 text-xs">
												<span className="text-muted-foreground w-24 shrink-0">{dateFormatter.format(new Date(bucket.timestamp))}</span>
												<div className="bg-primary/20 h-6 flex-1 overflow-hidden rounded" style={{ width: `${widthPct}%` }}>
													<div className="bg-primary h-full rounded" style={{ width: "100%" }} />
												</div>
												<span className="w-16 shrink-0 text-right tabular-nums">{formatCost(cost)}</span>
											</div>
										);
									})}
								</div>
							</CardContent>
						</Card>
					) : (
						<Card>
							<CardHeader>
								<CardTitle className="flex items-center gap-2 text-base">
									<BarChart3 className="h-4 w-4" />
									{t("users.memberUsage.histogram")}
								</CardTitle>
								<CardDescription>{t("users.memberUsage.histogramHint")}</CardDescription>
							</CardHeader>
							<CardContent>
								<p className="text-muted-foreground text-sm" data-testid="member-usage-histogram-empty">
									{t("users.memberUsage.histogramPlaceholder")}
								</p>
							</CardContent>
						</Card>
					)}

					<Card>
						<CardHeader>
							<CardTitle className="text-base">{t("users.memberUsage.detailTitle")}</CardTitle>
							<CardDescription>{t("users.memberUsage.detailDesc")}</CardDescription>
						</CardHeader>
						<CardContent className="text-muted-foreground text-sm">{t("users.memberUsage.detailFootnote")}</CardContent>
					</Card>
				</>
			)}
		</div>
	);
}