import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
	getErrorMessage,
	useCreateTeamInvitationMutation,
	useGetTeamQuery,
	useListTeamInvitationsQuery,
	useListTeamMembersQuery,
	useRevokeTeamInvitationMutation,
} from "@/lib/store";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import type { TeamMember } from "@/lib/store/apis/teamsApi";
import { Link, useParams } from "@tanstack/react-router";
import { ArrowLeft, Copy as CopyGlyph, Loader2, Plus, Trash2, UserPlus, Users } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

// TeamMembersView — the dedicated /workspace/governance/teams/$teamId/members
// page. Surfaces the member roster + outstanding invitations + invite
// sheet that used to live in the right-rail of the team-detail view.
// Moving them to a dedicated route means admins can deep-link to
// "send an invite" without first loading the full team detail, and
// keeps the model-policies section (US21) the only thing that has to
// share the team-detail surface with.
export default function TeamMembersView() {
	const { teamId } = useParams({ from: "/workspace/governance/teams/$teamId/members" });
	const { t } = useTranslation("teams");
	const { copy } = useCopyToClipboard();

	const [inviteSheetOpen, setInviteSheetOpen] = useState(false);
	const [pendingRevoke, setPendingRevoke] = useState<string | null>(null);
	const [inviteLink, setInviteLink] = useState<{ token: string; link: string; email: string } | null>(null);

	const { data: teamResp, isLoading: teamLoading, error: teamError } = useGetTeamQuery(teamId);
	const team = teamResp?.team;
	const { data: membersData, isLoading: membersLoading, refetch: refetchMembers } = useListTeamMembersQuery(teamId);
	const { data: invitationsData, isLoading: invitationsLoading } = useListTeamInvitationsQuery({ teamID: teamId });

	const [createTeamInvitation, { isLoading: isCreatingInvite }] = useCreateTeamInvitationMutation();
	const [revokeTeamInvitation] = useRevokeTeamInvitationMutation();

	const members = membersData?.members ?? [];
	const invitations = invitationsData?.invitations ?? [];

	const onCreateInvite = async (email: string, role: "admin" | "member") => {
		try {
			const res = await createTeamInvitation({ teamID: teamId, body: { email, role } }).unwrap();
			setInviteLink({ token: res.token, link: res.link, email });
			toast.success(t("inviteSuccess", { email }));
			setInviteSheetOpen(false);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onRevoke = async (invitationID: string) => {
		try {
			await revokeTeamInvitation({ teamID: teamId, invitationID }).unwrap();
			toast.success(t("inviteRevoked"));
			setPendingRevoke(null);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	if (teamLoading) {
		return (
			<div className="flex min-h-[40vh] items-center justify-center" data-testid="team-members-loading">
				<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
			</div>
		);
	}

	if (teamError || !team) {
		return (
			<div className="p-8" data-testid="team-members-not-found">
				<Card>
					<CardHeader>
						<CardTitle>{t("loadFailed")}</CardTitle>
						<CardDescription>{teamError ? getErrorMessage(teamError) : t("teamNotFound")}</CardDescription>
					</CardHeader>
					<CardContent>
						<Button asChild variant="outline">
							<Link to="/workspace/governance/teams/$teamId" params={{ teamId }} data-testid="team-members-back-not-found">
								{t("back")}
							</Link>
						</Button>
					</CardContent>
				</Card>
			</div>
		);
	}

	return (
		<div className="flex h-full flex-col gap-4 p-6" data-testid={`team-members-${teamId}`}>
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div className="flex items-center gap-3">
					<Button asChild variant="ghost" size="icon">
						<Link to="/workspace/governance/teams/$teamId" params={{ teamId }} data-testid="team-members-back">
							<ArrowLeft className="h-4 w-4" />
						</Link>
					</Button>
					<div>
						<h1 className="text-foreground text-2xl font-semibold">{t("membersTitle")}</h1>
						<p className="text-muted-foreground mt-1 font-mono text-xs">{team.name}</p>
					</div>
				</div>
				<div className="flex items-center gap-2">
					<Button onClick={() => setInviteSheetOpen(true)} data-testid="team-members-invite">
						<UserPlus className="mr-1 h-4 w-4" /> {t("invite")}
					</Button>
				</div>
			</header>

			<section>
				<header className="mb-2 flex items-center gap-2">
					<Users className="h-4 w-4" />
					<h2 className="text-lg font-semibold">{t("membersTitle")}</h2>
					<span className="text-muted-foreground text-sm">({members.length})</span>
				</header>
				<div className="border-border bg-card rounded-sm border">
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>{t("col_email")}</TableHead>
								<TableHead>{t("col_display_name")}</TableHead>
								<TableHead>{t("col_role")}</TableHead>
								<TableHead>{t("col_status")}</TableHead>
								<TableHead>{t("col_user_status")}</TableHead>
								<TableHead>{t("col_joined")}</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{membersLoading ? (
								<TableRow>
									<TableCell colSpan={6} className="text-muted-foreground py-8 text-center text-sm">
										<Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t("loading")}
									</TableCell>
								</TableRow>
							) : members.length === 0 ? (
								<TableRow>
									<TableCell colSpan={6} className="text-muted-foreground py-8 text-center text-sm">
										{t("noMembers")}
									</TableCell>
								</TableRow>
							) : (
								members.map((m) => <MemberRow key={m.id} member={m} />)
							)}
						</TableBody>
					</Table>
				</div>
			</section>

			<section>
				<header className="mb-2 flex items-center gap-2">
					<UserPlus className="h-4 w-4" />
					<h2 className="text-lg font-semibold">{t("invitationsTitle")}</h2>
					<span className="text-muted-foreground text-sm">({invitations.length})</span>
				</header>
				<div className="border-border bg-card rounded-sm border">
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>{t("col_email")}</TableHead>
								<TableHead>{t("col_role")}</TableHead>
								<TableHead>{t("col_status")}</TableHead>
								<TableHead>{t("col_expires")}</TableHead>
								<TableHead className="w-12 text-right">{t("col_actions")}</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{invitationsLoading ? (
								<TableRow>
									<TableCell colSpan={5} className="text-muted-foreground py-8 text-center text-sm">
										<Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t("loading")}
									</TableCell>
								</TableRow>
							) : invitations.length === 0 ? (
								<TableRow>
									<TableCell colSpan={5} className="text-muted-foreground py-8 text-center text-sm">
										{t("noInvitations")}
									</TableCell>
								</TableRow>
							) : (
								invitations.map((inv) => (
									<TableRow key={inv.id} data-testid={`team-invitation-${inv.id}`}>
										<TableCell className="font-mono text-xs">{inv.email}</TableCell>
										<TableCell>{inv.role_in_team}</TableCell>
										<TableCell>
											<Badge variant={inv.status === "pending" ? "default" : inv.status === "accepted" ? "secondary" : "outline"}>
												{t(`inviteStatus_${inv.status}`)}
											</Badge>
										</TableCell>
										<TableCell className="text-muted-foreground text-xs">{new Date(inv.expires_at).toLocaleString()}</TableCell>
										<TableCell className="text-right">
											{inv.status === "pending" && (
												<Button
													variant="ghost"
													size="icon"
													onClick={() => setPendingRevoke(inv.id)}
													data-testid={`team-invitation-revoke-${inv.id}`}
												>
													<Trash2 className="h-4 w-4" />
												</Button>
											)}
										</TableCell>
									</TableRow>
								))
							)}
						</TableBody>
					</Table>
				</div>
			</section>

			{pendingRevoke && (
				<Card data-testid="team-members-revoke-confirm">
					<CardHeader>
						<CardTitle className="text-base">{t("revokeConfirmTitle")}</CardTitle>
						<CardDescription>{t("revokeConfirmDesc")}</CardDescription>
					</CardHeader>
					<CardContent className="flex gap-2">
						<Button onClick={() => onRevoke(pendingRevoke)} data-testid="team-members-revoke-confirm-action">
							{t("confirmRevoke")}
						</Button>
						<Button variant="outline" onClick={() => setPendingRevoke(null)}>
							{t("cancel")}
						</Button>
					</CardContent>
				</Card>
			)}

			{inviteLink && (
				<Card data-testid="team-members-invite-link">
					<CardHeader>
						<CardTitle className="text-base">{t("inviteLinkTitle", { email: inviteLink.email })}</CardTitle>
						<CardDescription>{t("inviteLinkDesc")}</CardDescription>
					</CardHeader>
					<CardContent>
						<div className="bg-muted flex items-center gap-2 rounded-sm p-3 font-mono text-xs">
							<code className="flex-1 break-all" data-testid="team-members-invite-token">
								{inviteLink.token}
							</code>
							<Button variant="ghost" size="icon" onClick={() => copy(inviteLink.token)} data-testid="team-members-invite-token-copy">
								<CopyGlyph className="h-4 w-4" />
							</Button>
						</div>
						<p className="text-muted-foreground mt-2 text-xs">{t("inviteLinkUrl", { link: inviteLink.link })}</p>
					</CardContent>
				</Card>
			)}

			<InviteSheet open={inviteSheetOpen} onOpenChange={setInviteSheetOpen} onSubmit={onCreateInvite} isSubmitting={isCreatingInvite} />
			<button hidden onClick={() => refetchMembers()} data-testid="team-members-refetch" />
		</div>
	);
}

function MemberRow({ member }: { member: TeamMember }) {
	const { t } = useTranslation("teams");
	return (
		<TableRow data-testid={`team-member-${member.id}`}>
			<TableCell className="font-mono text-xs">{member.email ?? <span className="text-muted-foreground">—</span>}</TableCell>
			<TableCell>{member.display_name ?? <span className="text-muted-foreground">—</span>}</TableCell>
			<TableCell>
				<Badge variant="outline">{member.role_in_team}</Badge>
			</TableCell>
			<TableCell>
				<Badge variant={member.status === "active" ? "default" : "secondary"}>{t(`memberStatus_${member.status}`)}</Badge>
			</TableCell>
			<TableCell className="text-muted-foreground text-xs">{member.user_status ? t(`userStatus_${member.user_status}`) : "—"}</TableCell>
			<TableCell className="text-muted-foreground text-xs">{new Date(member.joined_at).toLocaleDateString()}</TableCell>
		</TableRow>
	);
}

function InviteSheet({
	open,
	onOpenChange,
	onSubmit,
	isSubmitting,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onSubmit: (email: string, role: "admin" | "member") => Promise<void>;
	isSubmitting: boolean;
}) {
	const { t } = useTranslation("teams");
	const [email, setEmail] = useState("");
	const [role, setRole] = useState<"admin" | "member">("member");

	const [initialized, setInitialized] = useState(false);
	if (open && !initialized) {
		setEmail("");
		setRole("member");
		setInitialized(true);
	}
	if (!open && initialized) setInitialized(false);

	const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		if (!email.trim()) return;
		await onSubmit(email.trim(), role);
	};

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="w-full sm:max-w-md" data-testid="team-members-invite-sheet">
				<SheetHeader>
					<SheetTitle className="flex items-center gap-2">
						<Plus className="h-4 w-4" /> {t("invite")}
					</SheetTitle>
					<SheetDescription>{t("inviteSheetDesc")}</SheetDescription>
				</SheetHeader>
				<form onSubmit={handleSubmit} className="space-y-4 px-4 pb-4">
					<div className="space-y-2">
						<Label htmlFor="members-invite-email">{t("col_email")}</Label>
						<Input
							id="members-invite-email"
							type="email"
							value={email}
							onChange={(e) => setEmail(e.target.value)}
							required
							data-testid="team-members-invite-email"
						/>
					</div>
					<div className="space-y-2">
						<Label>{t("col_role")}</Label>
						<Select value={role} onValueChange={(v) => setRole(v as "admin" | "member")}>
							<SelectTrigger data-testid="team-members-invite-role">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="member">{t("role_member")}</SelectItem>
								<SelectItem value="admin">{t("role_admin")}</SelectItem>
							</SelectContent>
						</Select>
					</div>
					<SheetFooter className="px-0">
						<Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
							{t("cancel")}
						</Button>
						<Button type="submit" isLoading={isSubmitting} data-testid="team-members-invite-submit">
							{t("invite")}
						</Button>
					</SheetFooter>
				</form>
			</SheetContent>
		</Sheet>
	);
}