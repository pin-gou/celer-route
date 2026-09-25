import { createFileRoute } from "@tanstack/react-router";
import CostPage from "./page";

export const Route = createFileRoute("/workspace/reports/cost")({
	component: CostPage,
});