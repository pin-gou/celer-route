import { createFileRoute } from "@tanstack/react-router";
import StandardPricesPage from "./page";

export const Route = createFileRoute("/workspace/reports/standard-prices")({
	component: StandardPricesPage,
});