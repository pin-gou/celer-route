import { createFileRoute } from "@tanstack/react-router";
import GatewayDeltaPage from "./page";

export const Route = createFileRoute("/workspace/reports/gateway-delta")({
	component: GatewayDeltaPage,
});