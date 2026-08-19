import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import PluginsPage from "./page";

function RouteComponent() {
	const hasPluginsAccess = useRbac(RbacResource.Plugins, RbacOperation.View);
	const childMatches = useChildMatches();
	if (!hasPluginsAccess) {
		return <NoPermissionView entity="plugins" entityI18nKey="plugins:page.title" />;
	}
	return childMatches.length === 0 ? <PluginsPage /> : <Outlet />;
}

export const Route = createFileRoute("/workspace/plugins")({
	component: RouteComponent,
});