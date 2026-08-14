import { createFileRoute, Outlet } from "@tanstack/react-router";
import GovernancePage from "./page";

function RouteComponent() {
	return <GovernancePage />;
}

export const Route = createFileRoute("/workspace/governance")({
	component: RouteComponent,
});