import { createFileRoute } from "@tanstack/react-router";
import CacheSavingsPage from "./page";

export const Route = createFileRoute("/workspace/reports/cache-savings")({
	component: CacheSavingsPage,
});