import { createFileRoute } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import ObservabilityPage from "./page";

function RouteComponent() {
	const hasObservabilityAccess = useRbac(RbacResource.Observability, RbacOperation.View);
	if (!hasObservabilityAccess) {
		return <NoPermissionView entity="observability settings" entityI18nKey="observability:page.title" />;
	}
	return <ObservabilityPage />;
}

export const Route = createFileRoute("/workspace/observability")({
	component: RouteComponent,
});