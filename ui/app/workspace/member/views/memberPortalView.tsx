import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { getErrorMessage, useMemberLogoutMutation, useMemberMeQuery } from "@/lib/store";
import { Link, useNavigate } from "@tanstack/react-router";
import { LogOut, RefreshCw, ShieldCheck, UserPlus, Users } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { JoinTeamSheet } from "./joinTeamSheet";

// Member portal landing page — what members see after signing in.
// This is intentionally minimal in Batch C-A (sub-batch A: identity
// + admin user management). It surfaces:
//
//   - the authenticated user's display_name + email + role
//   - the teams they belong to (from /api/member/me.memberships)
//   - a sign-out button that clears the member cookie
//   - a placeholder for the upcoming portal sections (key requests,
//     usage, etc.) — the cards stay clickable so the UI does not
//     feel incomplete before Batch C-B/C-C land.
//
// Phase 1 / data-model.md §5.1: this view NEVER exposes Provider Key
// metadata — only the user's own VKs (id + name + is_active), which
// the backend trims server-side.
export default function MemberPortalView() {
	const { t } = useTranslation("governance-ui");
	const navigate = useNavigate();
	const { data, isLoading, isError, error, refetch, isFetching } = useMemberMeQuery();
	const [memberLogout, { isLoading: isLoggingOut }] = useMemberLogoutMutation();
	const [joinSheetOpen, setJoinSheetOpen] = useState(false);

	const onSignOut = async () => {
		try {
			await memberLogout().unwrap();
			toast.success(t("users.memberPortal.signedOut"));
			navigate({ to: "/member" });
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	if (isLoading) {
		return (
			<div className="flex min-h-[60vh] items-center justify-center" data-testid="member-portal-loading">
				<RefreshCw className="text-muted-foreground h-6 w-6 animate-spin" />
			</div>
		);
	}

	if (isError || !data) {
		return (
			<div className="mx-auto max-w-2xl space-y-4 p-8">
				<Card>
					<CardHeader>
						<CardTitle>{t("users.memberPortal.loadFailed")}</CardTitle>
						<CardDescription>{getErrorMessage(error)}</CardDescription>
					</CardHeader>
					<CardContent className="flex gap-2">
						<Button onClick={() => refetch()} isLoading={isFetching} data-testid="member-portal-retry">
							{t("users.memberPortal.retry")}
						</Button>
						<Button variant="outline" onClick={onSignOut} data-testid="member-portal-signout-on-error">
							{t("users.memberPortal.signOut")}
						</Button>
					</CardContent>
				</Card>
			</div>
		);
	}

	const { user, teams, is_admin } = data;

	return (
		<div className="mx-auto max-w-4xl space-y-6 p-8" data-testid="member-portal">
			<header className="flex items-start justify-between gap-4">
				<div>
					<h1 className="text-foreground text-2xl font-semibold" data-testid="member-portal-title">
						{t("users.memberPortal.welcome", { name: user.display_name || user.email })}
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">{user.email}</p>
				</div>
				<Button variant="outline" onClick={onSignOut} isLoading={isLoggingOut} data-testid="member-portal-signout">
					<LogOut className="mr-2 h-4 w-4" />
					{t("users.memberPortal.signOut")}
				</Button>
			</header>

			<div className="grid grid-cols-1 gap-4 md:grid-cols-2">
				<Card>
					<CardHeader>
						<CardTitle className="flex items-center gap-2 text-base">
							<ShieldCheck className="h-4 w-4" />
							{t("users.memberPortal.account")}
						</CardTitle>
					</CardHeader>
					<CardContent className="space-y-2 text-sm">
						<div className="flex items-center justify-between">
							<span className="text-muted-foreground">{t("users.memberPortal.status")}</span>
							<span className="font-medium capitalize" data-testid="member-status">
								{t(`status_${user.status}`)}
							</span>
						</div>
						<div className="flex items-center justify-between">
							<span className="text-muted-foreground">{t("users.memberPortal.role")}</span>
							<span className="font-medium" data-testid="member-role">
								{t(`role_${user.role}`)}
							</span>
						</div>
						<div className="flex items-center justify-between">
							<span className="text-muted-foreground">{t("users.memberPortal.lastLogin")}</span>
							<span className="font-medium">
								{user.last_login_at ? new Date(user.last_login_at).toLocaleString() : t("users.memberPortal.never")}
							</span>
						</div>
					</CardContent>
				</Card>

				<Card>
					<CardHeader>
						<CardTitle className="flex items-center gap-2 text-base">
							<Users className="h-4 w-4" />
							{t("users.memberPortal.teams")}
						</CardTitle>
						<CardDescription>{t("users.memberPortal.teamsCount", { count: teams.length })}</CardDescription>
					</CardHeader>
					<CardContent className="space-y-2 text-sm" data-testid="member-teams">
						{teams.length === 0 ? (
							<p className="text-muted-foreground">{t("users.memberPortal.noTeams")}</p>
						) : (
							<ul className="space-y-1">
								{teams.map((m) => (
									<li key={m.id} className="flex items-center justify-between" data-testid={`member-team-${m.id}`}>
										<span className="font-medium">{m.name}</span>
										<span className="text-muted-foreground">{m.role}</span>
									</li>
								))}
							</ul>
						)}
					</CardContent>
				</Card>
			</div>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">{t("users.memberPortal.quickActions")}</CardTitle>
					<CardDescription>{t("users.memberPortal.quickActionsHint")}</CardDescription>
				</CardHeader>
				<CardContent className="grid grid-cols-1 gap-3 md:grid-cols-2">
					<Button asChild variant="outline">
						<Link to="/workspace/member/usage" data-testid="member-portal-usage">
							{t("users.memberPortal.usage")}
						</Link>
					</Button>
					<Button asChild variant="outline">
						<Link to="/workspace/member/setup-guide" data-testid="member-portal-setup-guide">
							{t("users.memberPortal.setupGuide")}
						</Link>
					</Button>
					<Button asChild variant="outline">
						<Link to="/workspace/member/keys" data-testid="member-portal-keys">
							{t("users.memberPortal.keyRequests")}
						</Link>
					</Button>
					<Button variant="outline" onClick={() => setJoinSheetOpen(true)} data-testid="member-portal-join-team">
						<UserPlus className="mr-2 h-4 w-4" />
						{t("users.keyRequests.joinTitle")}
					</Button>
				</CardContent>
				{is_admin ? (
					<CardContent>
						<p className="text-muted-foreground text-xs">{t("users.memberPortal.adminNote")}</p>
					</CardContent>
				) : null}
			</Card>

			<JoinTeamSheet open={joinSheetOpen} onOpenChange={setJoinSheetOpen} onSubmitted={() => refetch()} />
		</div>
	);
}