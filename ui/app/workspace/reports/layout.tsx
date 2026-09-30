import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import ReportsLanding from "./page";

function RouteComponent() {
	const childMatches = useChildMatches();
	if (childMatches.length === 0) return <ReportsLanding />;
	return <Outlet />;
}

export const Route = createFileRoute("/workspace/reports")({
	component: RouteComponent,
});