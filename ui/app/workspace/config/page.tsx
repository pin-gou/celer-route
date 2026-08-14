import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

export default function ConfigPage() {
	const navigate = useNavigate();
	// Check permission
	const hasConfigAccess = useRbac(RbacResource.Settings, RbacOperation.View);

	useEffect(() => {
		if (hasConfigAccess) {
			navigate({ to: "/workspace/config/client-settings", replace: true });
		}
	}, [hasConfigAccess, navigate]);

	if (!hasConfigAccess) {
		return <NoPermissionView entity="configuration" entityI18nKey="config:page.title" />;
	}
	return null;
}