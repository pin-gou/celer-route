import { createFileRoute } from "@tanstack/react-router";
import IdleKeysPage from "./page";

export const Route = createFileRoute("/workspace/reports/idle-keys")({
	component: IdleKeysPage,
});