import { createFileRoute } from "@tanstack/react-router";
import SecurityVirtualKeysPage from "./page";

export const Route = createFileRoute("/workspace/config/security/virtual-keys")({
	component: SecurityVirtualKeysPage,
});