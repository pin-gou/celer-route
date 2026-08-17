import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useGetModelsQuery, useGetProvidersQuery, useLazyGetLogsStatsQuery } from "@/lib/store";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { ModelsSection } from "./modelsSection";
import { SummaryCards } from "./summaryCards";

export default function ModelCatalogView() {
	const { t } = useTranslation("model-catalog");
	const hasAccess = useRbac(RbacResource.ModelProvider, RbacOperation.View);

	const { data: providers } = useGetProvidersQuery(undefined, { skip: !hasAccess });
	const { data: modelsData } = useGetModelsQuery({ unfiltered: true }, { skip: !hasAccess });
	const [triggerGlobalStats, { data: globalStats }] = useLazyGetLogsStatsQuery();

	useEffect(() => {
		if (!hasAccess) return;
		const now = new Date().toISOString();
		const hourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString();
		triggerGlobalStats({ filters: { start_time: hourAgo, end_time: now } });
	}, [hasAccess, triggerGlobalStats]);

	if (!hasAccess) {
		return <NoPermissionView entity="model catalog" entityI18nKey="model-catalog:page.title" />;
	}

	return (
		<div className="no-padding-parent mx-auto flex h-[calc(100dvh-1rem)] min-h-0 w-full max-w-7xl flex-col gap-4 overflow-hidden p-4">
			<h1 className="sr-only">{t("page.title")}</h1>
			<SummaryCards
				totalProviders={(providers ?? []).length}
				totalModels={modelsData?.total ?? 0}
				totalRequests1h={globalStats?.total_requests ?? 0}
			/>
			<div className="flex min-h-0 grow flex-col overflow-hidden">
				<ModelsSection />
			</div>
		</div>
	);
}