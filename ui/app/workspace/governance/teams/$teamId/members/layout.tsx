import { createFileRoute } from "@tanstack/react-router";
import TeamMembersPage from "./page";

export const Route = createFileRoute("/workspace/governance/teams/$teamId/members")({
	component: TeamMembersPage,
});