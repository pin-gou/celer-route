import { createFileRoute } from "@tanstack/react-router";
import AlertingPage from "./page";

export const Route = createFileRoute("/workspace/alerting")({
	component: AlertingPage,
});