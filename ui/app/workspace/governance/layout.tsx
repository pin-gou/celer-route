import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import GovernancePage from "./page";

// /workspace/governance is the landing index. It still redirects to
// /workspace/config/api-keys (the virtual-keys surface) when no child
// route is matched, preserving the legacy entry point. Once a child
// route (e.g. /workspace/governance/users) is matched, render the
// Outlet so the child can paint itself.
//
// The pattern matches the existing /workspace/logs and /workspace/mcp-sessions
// layouts (logs/layout.tsx: childMatches.length === 0 ? <Page /> : <Outlet />).
function RouteComponent() {
	const childMatches = useChildMatches();
	if (childMatches.length === 0) return <GovernancePage />;
	return <Outlet />;
}

export const Route = createFileRoute("/workspace/governance")({
	component: RouteComponent,
});