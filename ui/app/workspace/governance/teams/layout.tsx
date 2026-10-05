import { Outlet, createFileRoute, useChildMatches } from "@tanstack/react-router";
import GovernanceTeamsPage from "./page";

// /workspace/governance/teams is the landing index — it renders the
// teams table. When a child route (such as /workspace/governance/teams/$teamId)
// is matched, render the Outlet so the child detail page can paint
// itself. Same pattern as /workspace/governance/users and
// /workspace/governance/teams/$teamId (child-detection via useChildMatches).
function RouteComponent() {
	const childMatches = useChildMatches();
	if (childMatches.length === 0) return <GovernanceTeamsPage />;
	return <Outlet />;
}

export const Route = createFileRoute("/workspace/governance/teams")({
	component: RouteComponent,
});