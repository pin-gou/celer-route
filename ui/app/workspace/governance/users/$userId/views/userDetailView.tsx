import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alertDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { getErrorMessage, useDisableUserMutation, useDisableUserVksMutation, useGetUserQuery } from "@/lib/store";
import { Link, useParams } from "@tanstack/react-router";
import { ArrowLeft, KeyRound, Loader2, ShieldOff } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

function statusVariant(status: "pending" | "active" | "disabled") {
	switch (status) {
		case "active":
			return "default" as const;
		case "pending":
			return "secondary" as const;
		case "disabled":
			return "destructive" as const;
	}
}

// UserDetailView — the dedicated /workspace/governance/users/{userId}
// page. Surfaces the same composite shape the table-row aside used to
// inline (user + memberships + virtual_keys), but with full edit
// affordances pinned to the page: back link to the users list, the
// disable-user dialog, and the disable-user-VKs action wired to the
// dedicated button that US22 离职吊销 depends on (POST /api/governance/
// users/{user_id}/disable-vks).
//
// Provider-key metadata is intentionally absent — the admin-side
// detail page follows the same rule as the member portal: VK names
// only, never the underlying provider secret.
export default function UserDetailView() {
	const { userId } = useParams({ from: "/workspace/governance/users/$userId" });
	const { t } = useTranslation("governance-ui");

	const [pendingDisable, setPendingDisable] = useState(false);
	const [pendingDisableVks, setPendingDisableVks] = useState(false);

	const { data, isLoading, error } = useGetUserQuery(userId);

	const [disableUser] = useDisableUserMutation();
	const [disableUserVks] = useDisableUserVksMutation();

	if (isLoading) {
		return (
			<div className="flex min-h-[40vh] items-center justify-center" data-testid="user-detail-loading">
				<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
			</div>
		);
	}

	if (error || !data) {
		return (
			<div className="p-8" data-testid="user-detail-not-found">
				<Card>
					<CardHeader>
						<CardTitle>{t("users.userDetail.notFoundTitle")}</CardTitle>
						<CardDescription>{error ? getErrorMessage(error) : t("users.userDetail.notFoundDesc")}</CardDescription>
					</CardHeader>
					<CardContent>
						<Button asChild variant="outline">
							<Link to="/workspace/governance/users" data-testid="user-detail-back-not-found">
								{t("users.userDetail.backToList")}
							</Link>
						</Button>
					</CardContent>
				</Card>
			</div>
		);
	}

	const { user, memberships, virtual_keys } = data;
	const isDisabled = user.status === "disabled";

	const onConfirmDisable = async () => {
		try {
			await disableUser(user.id).unwrap();
			toast.success(t("users.disableSuccess", { email: user.email }));
			setPendingDisable(false);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onConfirmDisableVks = async () => {
		try {
			const res = await disableUserVks(user.id).unwrap();
			toast.success(t("users.disableVksSuccess", { count: res.count, email: user.email }));
			setPendingDisableVks(false);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	return (
		<div className="flex h-full flex-col gap-4 p-6" data-testid="user-detail-page">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div className="flex items-center gap-3">
					<Button asChild variant="ghost" size="icon">
						<Link to="/workspace/governance/users" data-testid="user-detail-back">
							<ArrowLeft className="h-4 w-4" />
						</Link>
					</Button>
					<div>
						<h1 className="text-foreground text-2xl font-semibold">{user.display_name || user.email}</h1>
						<p className="text-muted-foreground mt-1 font-mono text-xs">{user.id}</p>
					</div>
				</div>
				<div className="flex items-center gap-2">
					<Button
						variant="outline"
						onClick={() => setPendingDisableVks(true)}
						disabled={isDisabled || virtual_keys.length === 0}
						data-testid="user-detail-disable-vks"
					>
						<KeyRound className="mr-1 h-4 w-4" /> {t("users.userDetail.disableVks")}
					</Button>
					<Button variant="destructive" onClick={() => setPendingDisable(true)} disabled={isDisabled} data-testid="user-detail-disable">
						<ShieldOff className="mr-1 h-4 w-4" /> {t("users.userDetail.disableUser")}
					</Button>
				</div>
			</header>

			<div className="grid grid-cols-1 gap-4 md:grid-cols-2">
				<Card>
					<CardHeader>
						<CardTitle className="text-base">{t("users.userDetail.profileTitle")}</CardTitle>
					</CardHeader>
					<CardContent className="grid grid-cols-2 gap-3 text-sm" data-testid="user-detail-profile">
						<div>
							<div className="text-muted-foreground text-xs">{t("users.col_email")}</div>
							<div className="font-mono text-xs">{user.email}</div>
						</div>
						<div>
							<div className="text-muted-foreground text-xs">{t("users.col_display_name")}</div>
							<div>{user.display_name || "—"}</div>
						</div>
						<div>
							<div className="text-muted-foreground text-xs">{t("users.col_role")}</div>
							<div>
								<Badge variant="outline">{t(`role_${user.role}`)}</Badge>
							</div>
						</div>
						<div>
							<div className="text-muted-foreground text-xs">{t("users.col_status")}</div>
							<div>
								<Badge variant={statusVariant(user.status)}>{t(`status_${user.status}`)}</Badge>
							</div>
						</div>
						<div className="col-span-2">
							<div className="text-muted-foreground text-xs">{t("users.col_last_login")}</div>
							<div className="text-sm">{user.last_login_at ? new Date(user.last_login_at).toLocaleString() : t("users.never")}</div>
						</div>
					</CardContent>
				</Card>

				<Card>
					<CardHeader>
						<CardTitle className="text-base">
							{t("users.detailVirtualKeys")}
							<span className="text-muted-foreground ml-2 text-xs">({virtual_keys.length})</span>
						</CardTitle>
						<CardDescription>{t("users.userDetail.vksDesc")}</CardDescription>
					</CardHeader>
					<CardContent className="text-sm" data-testid="user-detail-vks">
						{virtual_keys.length === 0 ? (
							<p className="text-muted-foreground text-xs">{t("users.noVks")}</p>
						) : (
							<ul className="space-y-2">
								{virtual_keys.map((vk) => (
									<li
										key={vk.id}
										className="flex items-center justify-between rounded-sm border px-3 py-2"
										data-testid={`user-detail-vk-${vk.id}`}
									>
										<div>
											<div className="font-mono text-xs">{vk.name}</div>
											<div className="text-muted-foreground text-xs">
												{vk.team_id ? t("users.userDetail.teamScope", { team: vk.team_id }) : t("users.userDetail.userScope")}
											</div>
										</div>
										<Badge variant={vk.is_active ? "default" : "secondary"}>
											{vk.is_active ? t("users.vkActive") : t("users.vkInactive")}
										</Badge>
									</li>
								))}
							</ul>
						)}
					</CardContent>
				</Card>
			</div>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">
						{t("users.detailTeams")}
						<span className="text-muted-foreground ml-2 text-xs">({memberships.length})</span>
					</CardTitle>
				</CardHeader>
				<CardContent className="text-sm" data-testid="user-detail-memberships">
					{memberships.length === 0 ? (
						<p className="text-muted-foreground text-xs">{t("users.noTeams")}</p>
					) : (
						<ul className="space-y-1 font-mono text-xs">
							{memberships.map((m) => (
								<li key={m.team_id} className="flex items-center justify-between">
									<Link to="/workspace/governance/teams/$teamId" params={{ teamId: m.team_id }} className="hover:underline">
										{m.team_id}
									</Link>
									<span className="text-muted-foreground">
										{m.role_in_team} · {m.status}
									</span>
								</li>
							))}
						</ul>
					)}
				</CardContent>
			</Card>

			<AlertDialog open={pendingDisable} onOpenChange={setPendingDisable}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("users.disableConfirmTitle")}</AlertDialogTitle>
						<AlertDialogDescription>{t("users.disableConfirmDesc", { email: user.email })}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>{t("users.cancel")}</AlertDialogCancel>
						<AlertDialogAction onClick={onConfirmDisable} data-testid="user-detail-disable-confirm">
							{t("users.confirmDisable")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			<AlertDialog open={pendingDisableVks} onOpenChange={setPendingDisableVks}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("users.userDetail.disableVksTitle")}</AlertDialogTitle>
						<AlertDialogDescription>
							{t("users.userDetail.disableVksDesc", { email: user.email, count: virtual_keys.length })}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>{t("users.cancel")}</AlertDialogCancel>
						<AlertDialogAction onClick={onConfirmDisableVks} data-testid="user-detail-disable-vks-confirm">
							{t("users.userDetail.disableVksConfirm")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}