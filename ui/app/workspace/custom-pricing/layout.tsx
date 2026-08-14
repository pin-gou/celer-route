import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import CustomPricingPage from "./page";

function CustomPricingLayout({ children }: { children: React.ReactNode }) {
	const hasSettingsAccess = useRbac(RbacResource.Settings, RbacOperation.View);
	if (!hasSettingsAccess) {
		return <NoPermissionView entity="custom pricing" entityI18nKey="config:customPricing.title" />;
	}
	return <>{children}</>;
}

function RouteComponent() {
	const childMatches = useChildMatches();
	return <CustomPricingLayout>{childMatches.length === 0 ? <CustomPricingPage /> : <Outlet />}</CustomPricingLayout>;
}

export const Route = createFileRoute("/workspace/custom-pricing")({
	component: RouteComponent,
});