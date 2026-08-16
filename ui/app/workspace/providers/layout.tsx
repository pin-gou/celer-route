import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import ProvidersPage from "./page";

function RouteComponent() {
	const hasProvidersAccess = useRbac(RbacResource.ModelProvider, RbacOperation.View);
	const childMatches = useChildMatches();
	if (!hasProvidersAccess) {
		return <NoPermissionView entity="model providers" />;
	}
	return childMatches.length === 0 ? <ProvidersPage /> : <Outlet />;
}

export const Route = createFileRoute("/workspace/providers")({
	component: RouteComponent,
});