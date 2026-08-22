import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/workspace/governance/virtual-keys")({
	beforeLoad: () => {
		throw redirect({ to: "/workspace/config/security/virtual-keys", replace: true });
	},
});