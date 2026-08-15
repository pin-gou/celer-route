import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import TimelinePage from "./page";

function RouteComponent() {
	const hasViewLogsAccess = useRbac(RbacResource.Logs, RbacOperation.View);
	const childMatches = useChildMatches();
	if (!hasViewLogsAccess) {
		return <NoPermissionView entity="logs" entityI18nKey="logs:page.title" />;
	}
	return (
		<div className="flex h-full flex-col">
			{childMatches.length === 0 ? <TimelinePage /> : <Outlet />}
		</div>
	);
}

export const Route = createFileRoute("/workspace/logs/timeline")({
	component: RouteComponent,
});