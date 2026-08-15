import { NoPermissionView } from "@/components/noPermissionView";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useTranslation } from "react-i18next";
import AttributesTab from "./attributesTab";
import OverviewTab from "./overviewTab";

export default function ModelCatalogView() {
	const { t } = useTranslation("model-catalog");
	const hasAccess = useRbac(RbacResource.ModelProvider, RbacOperation.View);

	if (!hasAccess) {
		return <NoPermissionView entity="model catalog" entityI18nKey="model-catalog:page.title" />;
	}

	return (
		<div className="no-padding-parent mx-auto flex h-[calc(100dvh-1rem)] min-h-0 w-full max-w-7xl flex-col overflow-hidden p-4">
			<h1 className="sr-only">{t("page.title")}</h1>
			<Tabs defaultValue="overview" className="flex min-h-0 grow flex-col gap-4">
				<TabsList className="shrink-0">
					<TabsTrigger value="overview" data-testid="model-catalog-tab-overview">
						{t("tabs.overview")}
					</TabsTrigger>
					<TabsTrigger value="attributes" data-testid="model-catalog-tab-attributes">
						{t("tabs.models")}
					</TabsTrigger>
				</TabsList>
				<TabsContent value="overview" className="min-h-0 overflow-auto">
					<OverviewTab hasAccess={hasAccess} />
				</TabsContent>
				<TabsContent value="attributes" className="flex min-h-0 flex-col">
					<AttributesTab hasAccess={hasAccess} />
				</TabsContent>
			</Tabs>
		</div>
	);
}