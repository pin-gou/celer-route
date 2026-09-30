import { Outlet, createFileRoute, useChildMatches } from "@tanstack/react-router";
import GovernanceUsersPage from "./page";

// /workspace/governance/users is the landing index — it renders the
// users table. When a child route (such as /workspace/governance/users/$userId)
// is matched, render the Outlet so the child detail page can paint
// itself. Same pattern as /workspace/governance/teams/$teamId and
// /workspace/member layouts (child-detection via useChildMatches).
function RouteComponent() {
	const childMatches = useChildMatches();
	if (childMatches.length === 0) return <GovernanceUsersPage />;
	return <Outlet />;
}

export const Route = createFileRoute("/workspace/governance/users")({
	component: RouteComponent,
});