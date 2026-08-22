import { createFileRoute, Outlet, useChildMatches, useLocation } from "@tanstack/react-router";
import FullPageLoader from "@/components/fullPageLoader";
import { NoPermissionView } from "@/components/noPermissionView";
import { useGetCoreConfigQuery } from "@/lib/store";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import ConfigPage from "./page";

function RouteComponent() {
	const pathname = useLocation({ select: (l) => l.pathname });
	const hasSettingsAccess = useRbac(RbacResource.Settings, RbacOperation.View);
	const hasVirtualKeysAccess = useRbac(RbacResource.VirtualKeys, RbacOperation.View);
	const childMatches = useChildMatches();

	const isApiKeysRoute = pathname.startsWith("/workspace/config/api-keys");
	const requiredAccess = isApiKeysRoute ? hasVirtualKeysAccess : hasSettingsAccess;

	const { isLoading } = useGetCoreConfigQuery({ fromDB: true }, { skip: !requiredAccess });

	if (!requiredAccess) {
		return <NoPermissionView entity="configuration" entityI18nKey="config:page.title" />;
	}

	if (isLoading) {
		return <FullPageLoader />;
	}

	return childMatches.length === 0 ? <ConfigPage /> : <Outlet />;
}

export const Route = createFileRoute("/workspace/config")({
	component: RouteComponent,
});