import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { getErrorMessage, useMemberMeQuery, useMemberVirtualKeysQuery } from "@/lib/store";
import type { MemberVirtualKey } from "@/lib/store/apis/sessionApi";
import { Link } from "@tanstack/react-router";
import { Calendar, ExternalLink, KeyRound, RefreshCw, Settings2, Users } from "lucide-react";
import { useTranslation } from "react-i18next";

const dateFormatter = new Intl.DateTimeFormat(undefined, {
	year: "numeric",
	month: "short",
	day: "2-digit",
	hour: "2-digit",
	minute: "2-digit",
});

function formatExpiry(value: string | null): string {
	if (!value) return "—";
	const d = new Date(value);
	if (Number.isNaN(d.getTime())) return "—";
	return dateFormatter.format(d);
}

// Member-self view — list the VKs that the current member owns. No
// provider-key metadata is rendered here; the backend strips it
// (handlers/memberportal.go §listVirtualKeys comment). When the list
// is empty we surface a hand-off to /workspace/member/setup-guide so
// the member does not get stuck without a key.
export default function MyKeysView() {
	const { t } = useTranslation("governance-ui");
	const { copy } = useCopyToClipboard({ toastOnSuccess: false });
	const { data, isLoading, isError, error, refetch, isFetching } = useMemberVirtualKeysQuery();
	const me = useMemberMeQuery();

	const vks = data?.virtual_keys ?? [];

	const onCopyID = (vk: MemberVirtualKey) => {
		copy(vk.id);
	};

	return (
		<div className="mx-auto max-w-4xl space-y-6 p-8" data-testid="member-keys">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold" data-testid="member-keys-title">
						<KeyRound className="h-6 w-6" />
						{t("users.memberKeys.title")}
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("users.memberKeys.subtitle")}</p>
				</div>
				<div className="flex items-center gap-2">
					<Button asChild variant="outline" data-testid="member-keys-setup-link">
						<Link to="/workspace/member/setup-guide">
							<Settings2 className="mr-2 h-4 w-4" />
							{t("users.memberKeys.openSetupGuide")}
						</Link>
					</Button>
					<Button variant="outline" onClick={() => refetch()} isLoading={isFetching} data-testid="member-keys-refresh">
						<RefreshCw className="mr-2 h-4 w-4" />
						{t("users.memberPortal.retry")}
					</Button>
				</div>
			</header>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">{t("users.memberKeys.yourKeys")}</CardTitle>
					<CardDescription>{t("users.memberKeys.countLabel", { count: vks.length })}</CardDescription>
				</CardHeader>
				<CardContent>
					{isLoading ? (
						<div className="flex min-h-[20vh] items-center justify-center" data-testid="member-keys-loading">
							<RefreshCw className="text-muted-foreground h-5 w-5 animate-spin" />
						</div>
					) : isError ? (
						<div className="text-muted-foreground text-sm" data-testid="member-keys-error">
							{getErrorMessage(error)}
						</div>
					) : vks.length === 0 ? (
						<div className="space-y-3 py-4 text-center" data-testid="member-keys-empty">
							<Users className="text-muted-foreground mx-auto h-8 w-8" />
							<p className="text-sm">{t("users.memberKeys.empty")}</p>
							<p className="text-muted-foreground text-xs">{t("users.memberKeys.emptyHint")}</p>
							<div className="flex justify-center gap-2 pt-2">
								<Button asChild variant="outline">
									<Link to="/workspace/member/setup-guide" data-testid="member-keys-empty-setup">
										{t("users.memberKeys.openSetupGuide")}
									</Link>
								</Button>
							</div>
						</div>
					) : (
						<ul className="divide-border divide-y" data-testid="member-keys-list">
							{vks.map((vk) => (
								<li key={vk.id} className="flex flex-wrap items-center justify-between gap-3 py-3" data-testid={`member-keys-row-${vk.id}`}>
									<div className="min-w-0 flex-1">
										<div className="flex items-center gap-2">
											<span className="font-medium">{vk.name}</span>
											{!vk.is_active && (
												<span className="text-muted-foreground text-xs uppercase">{t("users.memberPortal.status_disabled")}</span>
											)}
										</div>
										<div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-xs">
											<span data-testid={`member-keys-id-${vk.id}`}>{vk.id}</span>
											{vk.team_id && <span>· {t("users.memberKeys.teamLabel", { team: vk.team_id })}</span>}
											<span className="flex items-center gap-1">
												<Calendar className="h-3 w-3" />
												{t("users.memberKeys.expiresLabel", { when: formatExpiry(vk.expires_at) })}
											</span>
										</div>
									</div>
									<div className="flex items-center gap-2">
										<Button variant="outline" size="sm" onClick={() => onCopyID(vk)} data-testid={`member-keys-copy-${vk.id}`}>
											{t("users.memberKeys.copyId")}
										</Button>
									</div>
								</li>
							))}
						</ul>
					)}
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">{t("users.memberKeys.usageHintTitle")}</CardTitle>
					<CardDescription>{t("users.memberKeys.usageHintDesc")}</CardDescription>
				</CardHeader>
				<CardContent>
					<Button asChild variant="outline" data-testid="member-keys-usage-link">
						<Link to="/workspace/member/usage">
							<ExternalLink className="mr-2 h-4 w-4" />
							{t("users.memberKeys.openUsage")}
						</Link>
					</Button>
				</CardContent>
			</Card>

			{/* Non-blocking hint when the member has no active VK — a soft
			    pointer to /workspace/member to confirm they are still on
			    a team. We render this below the main card rather than
			    gate the page, since some members legitimately have
			    multiple VKs but no team rows. */}
			{me.data && me.data.teams.length === 0 && vks.length === 0 ? (
				<p className="text-muted-foreground text-xs">{t("users.memberKeys.noTeamHint")}</p>
			) : null}
		</div>
	);
}