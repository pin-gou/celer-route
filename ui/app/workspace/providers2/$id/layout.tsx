import { createFileRoute, Outlet } from "@tanstack/react-router";

function RouteComponent() {
	return <Outlet />;
}

export const Route = createFileRoute("/workspace/providers2/$id")({
	component: RouteComponent,
});