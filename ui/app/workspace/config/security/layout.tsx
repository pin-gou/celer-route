import { createFileRoute, Outlet, useChildMatches, useLocation } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import SecurityPage from "./page";

function RouteComponent() {
	const pathname = useLocation({ select: (l) => l.pathname });
	const hasSettingsAccess = useRbac(RbacResource.Settings, RbacOperation.View);
	const hasVirtualKeysAccess = useRbac(RbacResource.VirtualKeys, RbacOperation.View);
	const childMatches = useChildMatches();

	const isVirtualKeysRoute = pathname.startsWith("/workspace/config/security/virtual-keys");
	const requiredAccess = isVirtualKeysRoute ? hasVirtualKeysAccess : hasSettingsAccess;

	if (!requiredAccess) {
		return <NoPermissionView entity="security" entityI18nKey="config:security.title" />;
	}

	return childMatches.length === 0 ? <SecurityPage /> : <Outlet />;
}

export const Route = createFileRoute("/workspace/config/security")({
	component: RouteComponent,
});