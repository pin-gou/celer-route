import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import BudgetForecastLanding from "./page";

function RouteComponent() {
	const childMatches = useChildMatches();
	if (childMatches.length === 0) return <BudgetForecastLanding />;
	return <Outlet />;
}

export const Route = createFileRoute("/workspace/reports/budget-forecast")({
	component: RouteComponent,
});