import { createFileRoute } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import AgentSetupPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.ModelProvider, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="agent setup" />;
	}
	return <AgentSetupPage />;
}

export const Route = createFileRoute("/workspace/agent-setup")({
	component: RouteComponent,
});