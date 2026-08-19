import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { createFileRoute } from "@tanstack/react-router";
import LogDetailPage from "./page";

function RouteComponent() {
	const hasViewLogsAccess = useRbac(RbacResource.Logs, RbacOperation.View);
	if (!hasViewLogsAccess) {
		return <NoPermissionView entity="logs" entityI18nKey="logs:page.title" />;
	}
	return <LogDetailPage />;
}

export const Route = createFileRoute("/workspace/logs/$id")({
	component: RouteComponent,
	staticData: { hideSidebar: true },
});