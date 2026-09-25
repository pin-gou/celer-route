import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { getErrorMessage, useGetTeamQuery } from "@/lib/store";
import { Link, useParams } from "@tanstack/react-router";
import { ArrowLeft, Loader2, UserPlus, Users } from "lucide-react";
import { useTranslation } from "react-i18next";
import TeamModelPoliciesSection from "./teamModelPoliciesSection";

// TeamDetailView — the /workspace/governance/teams/$teamId landing.
// Used to also host the members + invitations + InviteSheet; those
// moved to the dedicated /members subroute so the model-policies
// editor (US21) and the member-management surface can evolve
// independently. A "Manage members" link keeps the deep-link obvious.
export default function TeamDetailView() {
	const { teamId } = useParams({ from: "/workspace/governance/teams/$teamId" });
	const { t } = useTranslation("teams");

	const { data: teamResp, isLoading: teamLoading, error: teamError } = useGetTeamQuery(teamId);
	const team = teamResp?.team;

	if (teamLoading) {
		return (
			<div className="flex min-h-[40vh] items-center justify-center" data-testid="team-detail-loading">
				<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
			</div>
		);
	}

	if (teamError || !team) {
		return (
			<div className="p-8">
				<Card>
					<CardHeader>
						<CardTitle>{t("loadFailed")}</CardTitle>
						<CardDescription>{teamError ? getErrorMessage(teamError) : t("teamNotFound")}</CardDescription>
					</CardHeader>
					<CardContent>
						<Button asChild variant="outline">
							<Link to="/workspace/config/api-keys" data-testid="team-detail-back-redirect">
								{t("back")}
							</Link>
						</Button>
					</CardContent>
				</Card>
			</div>
		);
	}

	return (
		<div className="flex h-full flex-col gap-4 p-6" data-testid={`team-detail-${teamId}`}>
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div className="flex items-center gap-3">
					<Button asChild variant="ghost" size="icon">
						<Link to="/workspace/config/api-keys" data-testid="team-detail-back">
							<ArrowLeft className="h-4 w-4" />
						</Link>
					</Button>
					<div>
						<h1 className="text-foreground text-2xl font-semibold">{team.name}</h1>
						<p className="text-muted-foreground mt-1 font-mono text-xs">{team.id}</p>
					</div>
				</div>
				<div className="flex items-center gap-2">
					{team.customer && <Badge variant="outline">{t("customer", { name: team.customer.name })}</Badge>}
					<Button asChild variant="outline" data-testid="team-detail-manage-members">
						<Link to="/workspace/governance/teams/$teamId/members" params={{ teamId }}>
							<Users className="mr-1 h-4 w-4" /> {t("manageMembers")}
							<UserPlus className="ml-1 h-3 w-3 opacity-60" />
						</Link>
					</Button>
				</div>
			</header>

			<TeamModelPoliciesSection teamID={teamId} />
		</div>
	);
}