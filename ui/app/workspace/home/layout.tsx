import { createFileRoute } from "@tanstack/react-router";
import HomePage from "./views/homePage";

function RouteComponent() {
	return <HomePage />;
}

export const Route = createFileRoute("/workspace/home")({
	component: RouteComponent,
});