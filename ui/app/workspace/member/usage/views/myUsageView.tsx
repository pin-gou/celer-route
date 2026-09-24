import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { getErrorMessage, useMemberUsageQuery, useMemberVirtualKeysQuery } from "@/lib/store";
import { Activity, BarChart3, CalendarClock, KeyRound, RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";

const dateFormatter = new Intl.DateTimeFormat(undefined, {
	year: "numeric",
	month: "short",
	day: "2-digit",
	hour: "2-digit",
	minute: "2-digit",
});

// Phase-2 surface for /api/member/usage. The backend returns the
// user row + an empty histogram map (handlers/memberportal.go §usage
// comment: "intentionally empty for Phase 2 so the wire shape stays
// stable"). The detailed per-VK histogram lands when the UI is wired
// against /api/logs/histogram — for now we render what's there and
// hand the member off to /workspace/member/keys for ownership detail.
export default function MyUsageView() {
	const { t } = useTranslation("governance-ui");
	const { data, isLoading, isError, error, refetch, isFetching } = useMemberUsageQuery();
	const vks = useMemberVirtualKeysQuery();
	const vkCount = vks.data?.count ?? 0;

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
				<div className="grid grid-cols-1 gap-4 md:grid-cols-3" data-testid="member-usage-grid">
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2 text-base">
								<KeyRound className="h-4 w-4" />
								{t("users.memberUsage.activeKeys")}
							</CardTitle>
							<CardDescription>{t("users.memberUsage.activeKeysHint")}</CardDescription>
						</CardHeader>
						<CardContent>
							<p className="text-3xl font-semibold" data-testid="member-usage-vk-count">
								{vkCount}
							</p>
						</CardContent>
					</Card>

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
				</div>
			)}

			<Card>
				<CardHeader>
					<CardTitle className="text-base">{t("users.memberUsage.detailTitle")}</CardTitle>
					<CardDescription>{t("users.memberUsage.detailDesc")}</CardDescription>
				</CardHeader>
				<CardContent className="text-muted-foreground text-sm">{t("users.memberUsage.detailFootnote")}</CardContent>
			</Card>
		</div>
	);
}