import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { createFileRoute } from "@tanstack/react-router";
import WebhooksPage from "./page";

function RouteComponent() {
	const hasWebhooksAccess = useRbac(RbacResource.Governance, RbacOperation.View);
	if (!hasWebhooksAccess) {
		return <NoPermissionView entity="webhooks" entityI18nKey="webhooks:page.title" />;
	}
	return <WebhooksPage />;
}

export const Route = createFileRoute("/workspace/webhooks")({
	component: RouteComponent,
});