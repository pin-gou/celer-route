import { Outlet, createFileRoute, useChildMatches } from "@tanstack/react-router";
import TeamDetailPage from "./page";

// /workspace/governance/teams/$teamId is the landing index — it shows
// the team summary + model-policies editor (US21). When a child route
// (such as /members) is matched, render the Outlet so the child can
// paint itself. The pattern mirrors the existing /workspace/governance
// and /workspace/member layouts: child-detection via useChildMatches.
function RouteComponent() {
	const childMatches = useChildMatches();
	if (childMatches.length === 0) return <TeamDetailPage />;
	return <Outlet />;
}

export const Route = createFileRoute("/workspace/governance/teams/$teamId")({
	component: RouteComponent,
});