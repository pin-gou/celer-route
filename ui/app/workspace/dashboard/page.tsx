import { LogsFilterSidebar } from "@/components/filters/logsFilterSidebar";
import { DateTimePickerWithRange } from "@/components/ui/datePickerWithRange";
import { ScrollArea } from "@/components/ui/scrollArea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useTimezonePreference } from "@/lib/hooks/useTimezonePreference";
import { parseAsSafeArrayOf } from "@/lib/queryParamsParser";
import type { LogFilters } from "@/lib/types/logs";
import { dateUtils } from "@/lib/types/logs";
import { getRangeForPeriod, getTimePeriods } from "@/lib/utils/timeRange";
import { useLocation } from "@tanstack/react-router";
import { parseAsBoolean, parseAsInteger, parseAsString, useQueryStates } from "nuqs";
import { type RefObject, useCallback, useMemo, useRef, useState } from "react";
import { type ChartType } from "./components/charts/chartTypeToggle";
import { ExportPopover } from "./components/exportPopover";
import { type DimensionRankingsTabViewHandle, DimensionRankingsTabView } from "./components/tabViews/dimensionRankingsTabView";
import { type ModelRankingsTabViewHandle, ModelRankingsTabView } from "./components/tabViews/modelRankingsTabView";
import { type OverviewTabViewHandle, OverviewTabView } from "./components/tabViews/overviewTabView";
import { type ProviderUsageTabViewHandle, ProviderUsageTabView } from "./components/tabViews/providerUsageTabView";
import { type DashboardData, type DashboardTab, type ExportTab, DASHBOARD_EXPORT_TABS } from "./utils/exportUtils";
import { useTranslation } from "react-i18next";

const toChartType = (value: string): ChartType => (value === "line" ? "line" : "bar");

/** Wait two frames: one for React to commit, one for the browser to lay the result out. */
const nextFrames = () =>
	new Promise<void>((resolve) => {
		requestAnimationFrame(() => {
			requestAnimationFrame(() => resolve());
		});
	});

export default function DashboardPage() {
	const { t } = useTranslation("dashboard");

	const tabLabel = (tab: DashboardTab): string => {
		const key =
			tab === "overview"
				? "overview"
				: tab === "provider-usage"
					? "providerUsage"
					: tab === "rankings"
						? "modelRankings"
						: tab === "virtual-key-rankings"
							? "virtualKeyRankings"
							: "appRankings";
		return t("tabs." + key);
	};

	const defaultTimeRange = useMemo(() => dateUtils.getDefaultTimeRange(), []);

	const [timezone, setTimezone] = useTimezonePreference();

	const { search } = useLocation();
	const hasExplicitTimeRange = (search as Record<string, unknown>)?.start_time && (search as Record<string, unknown>)?.end_time;

	const [urlState, setUrlState] = useQueryStates(
		{
			period: parseAsString.withDefault(hasExplicitTimeRange ? "" : "1h").withOptions({ clearOnDefault: false }),
			start_time: parseAsInteger.withDefault(defaultTimeRange.startTime),
			end_time: parseAsInteger.withDefault(defaultTimeRange.endTime),
			tab: parseAsString.withDefault("overview"),
			virtual_key_ids: parseAsSafeArrayOf.withDefault([]),
			providers: parseAsSafeArrayOf.withDefault([]),
			models: parseAsSafeArrayOf.withDefault([]),
			selected_key_ids: parseAsSafeArrayOf.withDefault([]),
			objects: parseAsSafeArrayOf.withDefault([]),
			status: parseAsSafeArrayOf.withDefault([]),
			routing_rule_ids: parseAsSafeArrayOf.withDefault([]),
			routing_engine_used: parseAsSafeArrayOf.withDefault([]),
			stop_reasons: parseAsSafeArrayOf.withDefault([]),
			cache_hit_types: parseAsSafeArrayOf.withDefault([]),
			missing_cost_only: parseAsBoolean.withDefault(false),
			metadata_filters: parseAsString.withDefault(""),
			volume_chart: parseAsString.withDefault("bar"),
			token_chart: parseAsString.withDefault("bar"),
			model_chart: parseAsString.withDefault("bar"),
			latency_chart: parseAsString.withDefault("bar"),
			throughput_chart: parseAsString.withDefault("bar"),
			usage_model: parseAsString.withDefault("all"),
			provider_cost_chart: parseAsString.withDefault("bar"),
			provider_token_chart: parseAsString.withDefault("bar"),
			provider_latency_chart: parseAsString.withDefault("bar"),
			provider_throughput_chart: parseAsString.withDefault("bar"),
			provider_cost_provider: parseAsString.withDefault("all"),
			provider_token_provider: parseAsString.withDefault("all"),
			provider_latency_provider: parseAsString.withDefault("all"),
			provider_throughput_provider: parseAsString.withDefault("all"),
			parent_request_id: parseAsString.withDefault(""),
			user_ids: parseAsSafeArrayOf.withDefault([]),
			team_ids: parseAsSafeArrayOf.withDefault([]),
			customer_ids: parseAsSafeArrayOf.withDefault([]),
			business_unit_ids: parseAsSafeArrayOf.withDefault([]),
			aliases: parseAsSafeArrayOf.withDefault([]),
			apps: parseAsSafeArrayOf.withDefault([]),
		},
		{
			history: "push",
			shallow: false,
		},
	);

	const metadataFilters = useMemo(() => {
		if (!urlState.metadata_filters) return undefined;
		try {
			return JSON.parse(urlState.metadata_filters) as Record<string, string>;
		} catch {
			return undefined;
		}
	}, [urlState.metadata_filters]);

	const filters: LogFilters = useMemo(
		() => ({
			start_time: dateUtils.toISOString(urlState.start_time),
			...(urlState.period ? {} : { end_time: dateUtils.toISOString(urlState.end_time) }),
			...(urlState.providers.length > 0 && { providers: urlState.providers }),
			...(urlState.models.length > 0 && { models: urlState.models }),
			...(urlState.selected_key_ids.length > 0 && { selected_key_ids: urlState.selected_key_ids }),
			...(urlState.virtual_key_ids.length > 0 && {
				virtual_key_ids: urlState.virtual_key_ids,
			}),
			...(urlState.objects.length > 0 && { objects: urlState.objects }),
			...(urlState.status.length > 0 && { status: urlState.status }),
			...(urlState.routing_rule_ids.length > 0 && {
				routing_rule_ids: urlState.routing_rule_ids,
			}),
			...(urlState.routing_engine_used.length > 0 && {
				routing_engine_used: urlState.routing_engine_used,
			}),
			...(urlState.stop_reasons.length > 0 && { stop_reasons: urlState.stop_reasons }),
			...(urlState.cache_hit_types.length > 0 && { cache_hit_types: urlState.cache_hit_types }),
			...(urlState.missing_cost_only && { missing_cost_only: true }),
			...(metadataFilters &&
				Object.keys(metadataFilters).length > 0 && {
					metadata_filters: metadataFilters,
				}),
			...(urlState.parent_request_id && { parent_request_id: urlState.parent_request_id }),
			...(urlState.user_ids.length > 0 && { user_ids: urlState.user_ids }),
			...(urlState.team_ids.length > 0 && { team_ids: urlState.team_ids }),
			...(urlState.customer_ids.length > 0 && { customer_ids: urlState.customer_ids }),
			...(urlState.business_unit_ids.length > 0 && { business_unit_ids: urlState.business_unit_ids }),
			...(urlState.aliases.length > 0 && { aliases: urlState.aliases }),
			...(urlState.apps.length > 0 && { apps: urlState.apps }),
		}),
		[
			urlState.period,
			urlState.start_time,
			urlState.end_time,
			urlState.parent_request_id,
			urlState.providers,
			urlState.models,
			urlState.selected_key_ids,
			urlState.virtual_key_ids,
			urlState.objects,
			urlState.status,
			urlState.routing_rule_ids,
			urlState.routing_engine_used,
			urlState.stop_reasons,
			urlState.cache_hit_types,
			urlState.missing_cost_only,
			metadataFilters,
			urlState.user_ids,
			urlState.team_ids,
			urlState.customer_ids,
			urlState.business_unit_ids,
			urlState.aliases,
			urlState.apps,
		],
	);

	// Tab view refs for export data aggregation
	const overviewRef = useRef<OverviewTabViewHandle>(null);
	const providerRef = useRef<ProviderUsageTabViewHandle>(null);
	const modelRankingsRef = useRef<ModelRankingsTabViewHandle>(null);
	const virtualKeyRankingsRef = useRef<DimensionRankingsTabViewHandle>(null);
	const appRankingsRef = useRef<DimensionRankingsTabViewHandle>(null);

	const allRefs = [overviewRef, providerRef, modelRankingsRef, virtualKeyRankingsRef, appRankingsRef];

	const getDashboardData = useCallback((): DashboardData => {
		const merged: Partial<DashboardData> = {};
		for (const r of allRefs) {
			if (r.current) Object.assign(merged, r.current.getData());
		}
		return {
			histogramData: null,
			tokenData: null,
			modelData: null,
			latencyData: null,
			providerCostData: null,
			providerTokenData: null,
			providerLatencyData: null,
			rankingsData: null,
			virtualKeyRankingsData: null,
			appRankingsData: null,
			...merged,
		};
	}, []);

	// Which scope the in-flight export covers; null when not exporting.
	// "all" force-mounts every tab, a single tab exports only what is open.
	const [exportScope, setExportScope] = useState<ExportTab | null>(null);
	const exportingAll = exportScope === "all";
	/** True while this tab is part of the running export, so rankings render their uncapped snapshot. */
	const isExportingTab = (tab: DashboardTab) => exportingAll || exportScope === tab;

	const handlePreloadData = useCallback(async (scope: ExportTab) => {
		// For an all-tabs export, force-mount every tab before loading: Radix
		// unmounts inactive TabsContent, so an inactive tab view has no ref yet
		// and would never load its export snapshot - the report would only cover
		// the tab that happened to be open. A single-tab export deliberately
		// skips this, so only the open tab's data reaches the export.
		setExportScope(scope);
		await nextFrames();

		const refsByTab: Record<DashboardTab, RefObject<{ loadData: () => Promise<void> } | null>> = {
			overview: overviewRef,
			"provider-usage": providerRef,
			rankings: modelRankingsRef,
			"virtual-key-rankings": virtualKeyRankingsRef,
			"app-rankings": appRankingsRef,
		};

		const refs = scope === "all" ? allRefs : [refsByTab[scope]];
		await Promise.all(refs.map((r) => r.current?.loadData()));

		// Let the loaded snapshots commit before the caller reads getData() or
		// captures the DOM.
		await nextFrames();
	}, []);

	// Tab change handler
	const handleTabChange = useCallback(
		(tab: string) => {
			setUrlState({ tab });
		},
		[setUrlState],
	);

	// Chart type toggles
	const handleVolumeChartToggle = useCallback((type: ChartType) => setUrlState({ volume_chart: type }), [setUrlState]);
	const handleTokenChartToggle = useCallback((type: ChartType) => setUrlState({ token_chart: type }), [setUrlState]);
	const handleModelChartToggle = useCallback((type: ChartType) => setUrlState({ model_chart: type }), [setUrlState]);
	const handleLatencyChartToggle = useCallback((type: ChartType) => setUrlState({ latency_chart: type }), [setUrlState]);
	const handleThroughputChartToggle = useCallback((type: ChartType) => setUrlState({ throughput_chart: type }), [setUrlState]);
	const handleProviderCostChartToggle = useCallback((type: ChartType) => setUrlState({ provider_cost_chart: type }), [setUrlState]);
	const handleProviderTokenChartToggle = useCallback((type: ChartType) => setUrlState({ provider_token_chart: type }), [setUrlState]);
	const handleProviderLatencyChartToggle = useCallback((type: ChartType) => setUrlState({ provider_latency_chart: type }), [setUrlState]);
	const handleProviderThroughputChartToggle = useCallback(
		(type: ChartType) => setUrlState({ provider_throughput_chart: type }),
		[setUrlState],
	);
	// Model / provider filter changes
	const handleUsageModelChange = useCallback((model: string) => setUrlState({ usage_model: model }), [setUrlState]);
	const handleProviderCostProviderChange = useCallback(
		(provider: string) => setUrlState({ provider_cost_provider: provider }),
		[setUrlState],
	);
	const handleProviderTokenProviderChange = useCallback(
		(provider: string) => setUrlState({ provider_token_provider: provider }),
		[setUrlState],
	);
	const handleProviderLatencyProviderChange = useCallback(
		(provider: string) => setUrlState({ provider_latency_provider: provider }),
		[setUrlState],
	);
	const handleProviderThroughputProviderChange = useCallback(
		(provider: string) => setUrlState({ provider_throughput_provider: provider }),
		[setUrlState],
	);

	// Adapter: converts a full LogFilters object to dashboard URL state
	const setFilters = useCallback(
		(newFilters: LogFilters) => {
			// The sidebar/header only manage dimension filters, never the time range: in
			// period mode `newFilters` carries no start/end, so only touch time when an
			// explicit range is actually provided — otherwise we'd clear the active period.
			const hasExplicitTime = !!newFilters.start_time && !!newFilters.end_time;
			const newStartTime = hasExplicitTime ? dateUtils.toUnixTimestamp(new Date(newFilters.start_time!)) : undefined;
			const newEndTime = hasExplicitTime ? dateUtils.toUnixTimestamp(new Date(newFilters.end_time!)) : undefined;
			const timeChanged = hasExplicitTime && (newStartTime !== urlState.start_time || newEndTime !== urlState.end_time);
			setUrlState({
				...(timeChanged && { period: "", start_time: newStartTime, end_time: newEndTime }),
				providers: newFilters.providers || [],
				models: newFilters.models || [],
				selected_key_ids: newFilters.selected_key_ids || [],
				virtual_key_ids: newFilters.virtual_key_ids || [],
				objects: newFilters.objects || [],
				status: newFilters.status || [],
				routing_rule_ids: newFilters.routing_rule_ids || [],
				routing_engine_used: newFilters.routing_engine_used || [],
				stop_reasons: newFilters.stop_reasons || [],
				cache_hit_types: newFilters.cache_hit_types || [],
				missing_cost_only: newFilters.missing_cost_only ?? false,
				metadata_filters:
					newFilters.metadata_filters && Object.keys(newFilters.metadata_filters).length > 0
						? JSON.stringify(newFilters.metadata_filters)
						: "",
				parent_request_id: newFilters.parent_request_id || "",
				user_ids: newFilters.user_ids || [],
				team_ids: newFilters.team_ids || [],
				customer_ids: newFilters.customer_ids || [],
				business_unit_ids: newFilters.business_unit_ids || [],
				aliases: newFilters.aliases || [],
				apps: newFilters.apps || [],
			});
		},
		[setUrlState, urlState.start_time, urlState.end_time],
	);

	// Date range for picker
	const dateRange = useMemo(
		() => ({
			from: dateUtils.fromUnixTimestamp(urlState.start_time),
			to: dateUtils.fromUnixTimestamp(urlState.end_time),
		}),
		[urlState.start_time, urlState.end_time],
	);

	const handlePeriodChange = useCallback(
		(period: string | undefined) => {
			if (!period) return;
			const { from } = getRangeForPeriod(period);
			setUrlState({
				period,
				start_time: Math.floor(from.getTime() / 1000),
				end_time: null,
			});
		},
		[setUrlState],
	);

	const handleDateRangeChange = useCallback(
		(range: { from?: Date; to?: Date }) => {
			if (!range.from || !range.to) return;
			setUrlState({
				period: "",
				start_time: dateUtils.toUnixTimestamp(range.from),
				end_time: dateUtils.toUnixTimestamp(range.to),
			});
		},
		[setUrlState],
	);

	// PDF export mode
	const dashboardMinHeightRef = useRef<string>("");
	const hiddenTabsRef = useRef<HTMLElement[]>([]);

	const handlePdfExport = useCallback(
		async (scope: ExportTab): Promise<{ element: HTMLElement; label: string }[]> => {
			// Enters export mode, force-mounts the scoped tabs and waits for the
			// loaded snapshots to render.
			await handlePreloadData(scope);

			// Only an all-tabs export has hidden sections to reveal; a single-tab
			// export captures the section already on screen.
			if (scope === "all") {
				const hiddenTabs = document.querySelectorAll<HTMLElement>('[data-slot="tabs-content"][hidden]');
				hiddenTabsRef.current = Array.from(hiddenTabs);
				for (const tab of hiddenTabs) {
					tab.removeAttribute("hidden");
					tab.style.display = "block";
				}
			}

			const dashboardEl = document.getElementById("dashboard-root");
			if (dashboardEl) {
				dashboardMinHeightRef.current = dashboardEl.style.minHeight;
				dashboardEl.style.minHeight = "0";
			}

			window.dispatchEvent(new Event("resize"));
			await nextFrames();

			const tabs = scope === "all" ? DASHBOARD_EXPORT_TABS : DASHBOARD_EXPORT_TABS.filter((t) => t.value === scope);
			return tabs
				.map(({ sectionId, value }) => ({ element: document.getElementById(sectionId), label: tabLabel(value) }))
				.filter((s): s is { element: HTMLElement; label: string } => s.element !== null);
		},
		[handlePreloadData],
	);

	const handleExportDone = useCallback(() => {
		const dashboardEl = document.getElementById("dashboard-root");
		if (dashboardEl) {
			dashboardEl.style.minHeight = dashboardMinHeightRef.current;
		}

		for (const tab of hiddenTabsRef.current) {
			tab.setAttribute("hidden", "");
			tab.style.display = "";
		}
		hiddenTabsRef.current = [];

		setExportScope(null);
	}, []);

	const activeTab = (urlState.tab || "overview") as DashboardTab;

	return (
		<div id="dashboard-root" className="no-padding-parent no-border-parent bg-background flex h-[calc(100vh_-_16px)] w-full gap-3">
			{/* Sidebar Filters */}
			<LogsFilterSidebar filters={filters} onFiltersChange={setFilters} />

			{/* Main Content */}
			<ScrollArea className="bg-card flex min-w-0 flex-1 flex-col gap-4 rounded-l-md" viewportClassName="no-table">
				{/* Header */}
				<div className="flex items-center justify-between p-4">
					<div className="flex items-center gap-2">
						<h1 className="text-lg font-semibold">{t("title")}</h1>
					</div>
					<div className="flex items-center gap-2">
						<ExportPopover
							getData={getDashboardData}
							activeTab={activeTab}
							onPreloadData={handlePreloadData}
							onPdfExport={handlePdfExport}
							onExportDone={handleExportDone}
						/>
						<DateTimePickerWithRange
							dateTime={dateRange}
							onDateTimeUpdate={handleDateRangeChange}
							preDefinedPeriods={getTimePeriods(t)}
							predefinedPeriod={urlState.period || undefined}
							onPredefinedPeriodChange={handlePeriodChange}
							triggerTestId="dashboard-filter-daterange"
							popupAlignment="end"
							showTimezone
							timezone={timezone}
							onTimezoneChange={setTimezone}
						/>
					</div>
				</div>

				<div className="p-4">
					{/* Tabs */}
					<Tabs value={activeTab} onValueChange={handleTabChange}>
						<div className="mb-2 max-w-full overflow-x-auto">
							<TabsList className="w-max min-w-max">
								<TabsTrigger className="shrink-0" value="overview" data-testid="dashboard-tab-overview">
									{t("tabs.overview")}
								</TabsTrigger>
								<TabsTrigger className="shrink-0" value="provider-usage" data-testid="dashboard-tab-provider-usage">
									{t("tabs.providerUsage")}
								</TabsTrigger>
								<TabsTrigger className="shrink-0" value="rankings" data-testid="dashboard-tab-rankings">
									{t("tabs.modelRankings")}
								</TabsTrigger>
								<TabsTrigger className="shrink-0" value="virtual-key-rankings" data-testid="dashboard-tab-virtual-key-rankings">
									{t("tabs.virtualKeyRankings")}
								</TabsTrigger>
								<TabsTrigger value="app-rankings" data-testid="dashboard-tab-app-rankings">
									{t("tabs.appRankings")}
								</TabsTrigger>
							</TabsList>
						</div>
						{/* Overview Tab */}
						<TabsContent value="overview" {...(exportingAll && { forceMount: true })}>
							<div id="dashboard-section-overview">
								<OverviewTabView
									ref={overviewRef}
									filters={filters}
									active={activeTab === "overview" || exportingAll}
									startTime={urlState.start_time}
									endTime={urlState.end_time}
									volumeChartType={toChartType(urlState.volume_chart)}
									tokenChartType={toChartType(urlState.token_chart)}
									modelChartType={toChartType(urlState.model_chart)}
									latencyChartType={toChartType(urlState.latency_chart)}
									throughputChartType={toChartType(urlState.throughput_chart)}
									usageModel={urlState.usage_model}
									onVolumeChartToggle={handleVolumeChartToggle}
									onTokenChartToggle={handleTokenChartToggle}
									onModelChartToggle={handleModelChartToggle}
									onLatencyChartToggle={handleLatencyChartToggle}
									onThroughputChartToggle={handleThroughputChartToggle}
									onUsageModelChange={handleUsageModelChange}
								/>
							</div>
						</TabsContent>

						{/* Provider Usage Tab */}
						<TabsContent value="provider-usage" {...(exportingAll && { forceMount: true })}>
							<div id="dashboard-section-provider-usage">
								<ProviderUsageTabView
									ref={providerRef}
									filters={filters}
									active={activeTab === "provider-usage" || exportingAll}
									startTime={urlState.start_time}
									endTime={urlState.end_time}
									providerCostChartType={toChartType(urlState.provider_cost_chart)}
									providerTokenChartType={toChartType(urlState.provider_token_chart)}
									providerLatencyChartType={toChartType(urlState.provider_latency_chart)}
									providerThroughputChartType={toChartType(urlState.provider_throughput_chart)}
									providerCostProvider={urlState.provider_cost_provider}
									providerTokenProvider={urlState.provider_token_provider}
									providerLatencyProvider={urlState.provider_latency_provider}
									providerThroughputProvider={urlState.provider_throughput_provider}
									onProviderCostChartToggle={handleProviderCostChartToggle}
									onProviderTokenChartToggle={handleProviderTokenChartToggle}
									onProviderLatencyChartToggle={handleProviderLatencyChartToggle}
									onProviderThroughputChartToggle={handleProviderThroughputChartToggle}
									onProviderCostProviderChange={handleProviderCostProviderChange}
									onProviderTokenProviderChange={handleProviderTokenProviderChange}
									onProviderLatencyProviderChange={handleProviderLatencyProviderChange}
									onProviderThroughputProviderChange={handleProviderThroughputProviderChange}
								/>
							</div>
						</TabsContent>

						{/* Model Rankings Tab */}
						<TabsContent value="rankings" {...(exportingAll && { forceMount: true })}>
							<div id="dashboard-section-rankings">
								<ModelRankingsTabView
									ref={modelRankingsRef}
									filters={filters}
									active={activeTab === "rankings" || exportingAll}
									startTime={urlState.start_time}
									endTime={urlState.end_time}
									pdfMode={isExportingTab("rankings")}
								/>
							</div>
						</TabsContent>

						<TabsContent value="virtual-key-rankings" {...(exportingAll && { forceMount: true })}>
							<div id="dashboard-section-virtual-key-rankings">
								<DimensionRankingsTabView
									ref={virtualKeyRankingsRef}
									filters={filters}
									active={activeTab === "virtual-key-rankings" || exportingAll}
									dimension="virtual_key"
									dimensionLabel={t("dimensions.virtualKey")}
									testIdPrefix="dashboard-virtual-key-rankings"
									dataKey="virtualKeyRankingsData"
									pdfMode={isExportingTab("virtual-key-rankings")}
								/>
							</div>
						</TabsContent>
						{/* App Rankings Tab */}
						<TabsContent value="app-rankings" {...(isExportingTab("app-rankings") && { forceMount: true })}>
							<div id="dashboard-section-app-rankings">
								<DimensionRankingsTabView
									ref={appRankingsRef}
									filters={filters}
									active={activeTab === "app-rankings" || isExportingTab("app-rankings")}
									dimension="app"
									dimensionLabel={t("dimensions.app")}
									testIdPrefix="dashboard-app-rankings"
									dataKey="appRankingsData"
									pdfMode={isExportingTab("app-rankings")}
								/>
							</div>
						</TabsContent>
					</Tabs>
				</div>
			</ScrollArea>
		</div>
	);
}