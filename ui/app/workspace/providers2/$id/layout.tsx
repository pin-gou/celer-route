import { createFileRoute } from "@tanstack/react-router";
import ProviderDetailPage from "./page";

function RouteComponent() {
	return <ProviderDetailPage />;
}

export const Route = createFileRoute("/workspace/providers2/$id")({
	component: RouteComponent,
});