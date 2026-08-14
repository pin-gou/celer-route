import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { createFileRoute } from "@tanstack/react-router";
import ComplexityRouterPage from "./page";

function RouteComponent() {
	const hasRoutingRulesAccess = useRbac(RbacResource.RoutingRules, RbacOperation.View);
	if (!hasRoutingRulesAccess) {
		return <NoPermissionView entity="complexity router" entityI18nKey="routing:complexityRouter.title" />;
	}
	return <ComplexityRouterPage />;
}

export const Route = createFileRoute("/workspace/complexity-router")({
	component: RouteComponent,
});