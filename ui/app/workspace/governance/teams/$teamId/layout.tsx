import { createFileRoute } from "@tanstack/react-router";
import TeamDetailPage from "./page";

export const Route = createFileRoute("/workspace/governance/teams/$teamId")({
	component: TeamDetailPage,
});