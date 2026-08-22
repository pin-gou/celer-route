import { createFileRoute } from "@tanstack/react-router";
import SecurityVirtualKeysPage from "./page";

export const Route = createFileRoute("/workspace/config/api-keys")({
	component: SecurityVirtualKeysPage,
});