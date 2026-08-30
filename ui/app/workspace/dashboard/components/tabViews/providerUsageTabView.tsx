import {
	useGetLogsProviderLatencyHistogramQuery,
	useGetLogsProviderThroughputHistogramQuery,
	useGetLogsProviderTokenHistogramQuery,
	useGetProviderRankingsQuery,
	useLazyGetLogsProviderLatencyHistogramQuery,
	useLazyGetLogsProviderThroughputHistogramQuery,
	useLazyGetLogsProviderTokenHistogramQuery,
	useLazyGetProviderRankingsQuery,
} from "@/lib/store";
import type { LogFilters, ProviderRankingsResponse } from "@/lib/types/logs";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useState } from "react";
import { computeDisplaySeries } from "../../utils/chartUtils";
import type { DashboardData } from "../../utils/exportUtils";
import { DASHBOARD_RANKINGS_LIMIT } from "../../utils/rankings";
import type { ChartType } from "../charts/chartTypeToggle";
import { ProviderUsageTab } from "../providerUsageTab";

export interface ProviderUsageTabViewHandle {
	getData: () => Partial<DashboardData>;
	loadData: () => Promise<void>;
}

const sanitizeSeriesLabels = (values?: string[]): string[] => {
	if (!values) return [];
	const trimmed = values.map((v) => v.trim()).filter((v) => v.length > 0);
	return [...new Set(trimmed)];
};

interface ProviderUsageTabViewProps {
	filters: LogFilters;
	active: boolean;
	startTime: number;
	endTime: number;
	providerTokenChartType: ChartType;
	providerLatencyChartType: ChartType;
	providerThroughputChartType: ChartType;
	providerTokenProvider: string;
	providerLatencyProvider: string;
	providerThroughputProvider: string;
	onProviderTokenChartToggle: (type: ChartType) => void;
	onProviderLatencyChartToggle: (type: ChartType) => void;
	onProviderThroughputChartToggle: (type: ChartType) => void;
	onProviderTokenProviderChange: (provider: string) => void;
	onProviderLatencyProviderChange: (provider: string) => void;
	onProviderThroughputProviderChange: (provider: string) => void;
}

export const ProviderUsageTabView = forwardRef<ProviderUsageTabViewHandle, ProviderUsageTabViewProps>(function ProviderUsageTabView(
	{
		filters,
		active,
		startTime,
		endTime,
		providerTokenChartType,
		providerLatencyChartType,
		providerThroughputChartType,
		providerTokenProvider,
		providerLatencyProvider,
		providerThroughputProvider,
		onProviderTokenChartToggle,
		onProviderLatencyChartToggle,
		onProviderThroughputChartToggle,
		onProviderTokenProviderChange,
		onProviderLatencyProviderChange,
		onProviderThroughputProviderChange,
	},
	ref,
) {
	const fetchArg = useMemo(() => ({ filters }), [filters]);
	const rankingsArg = useMemo(() => ({ filters, limit: DASHBOARD_RANKINGS_LIMIT }), [filters]);
	// Exports are never truncated: they ask for every ranked provider.
	const rankingsExportArg = useMemo(() => ({ filters, all: true }), [filters]);
	const skipOpts = useMemo(() => ({ skip: !active }), [active]);

	const { data: providerTokenData, isLoading: loadingProviderTokens } = useGetLogsProviderTokenHistogramQuery(fetchArg, skipOpts);
	const { data: providerLatencyData, isLoading: loadingProviderLatency } = useGetLogsProviderLatencyHistogramQuery(fetchArg, skipOpts);
	const { data: providerThroughputData, isLoading: loadingProviderThroughput } = useGetLogsProviderThroughputHistogramQuery(
		fetchArg,
		skipOpts,
	);
	const { data: providerRankingsData, isLoading: loadingProviderRankings } = useGetProviderRankingsQuery(rankingsArg, skipOpts);

	const [triggerProviderTokens] = useLazyGetLogsProviderTokenHistogramQuery();
	const [triggerProviderLatency] = useLazyGetLogsProviderLatencyHistogramQuery();
	const [triggerProviderThroughput] = useLazyGetLogsProviderThroughputHistogramQuery();
	const [triggerProviderRankings] = useLazyGetProviderRankingsQuery();
	const [rankingsExportData, setRankingsExportData] = useState<ProviderRankingsResponse | null>(null);

	// A snapshot belongs to the filters it was fetched with - drop it when those change.
	useEffect(() => setRankingsExportData(null), [rankingsExportArg]);

	const loadData = useCallback(async () => {
		const [rankings] = await Promise.all([
			// Fall back to the capped view rather than failing the whole export.
			triggerProviderRankings(rankingsExportArg, true)
				.unwrap()
				.catch(() => null),
			triggerProviderTokens(fetchArg, true),
			triggerProviderLatency(fetchArg, true),
			triggerProviderThroughput(fetchArg, true),
		]);
		setRankingsExportData(rankings);
	}, [fetchArg, rankingsExportArg, triggerProviderRankings, triggerProviderTokens, triggerProviderLatency, triggerProviderThroughput]);

	useImperativeHandle(
		ref,
		() => ({
			getData: () => ({
				providerRankingsData: rankingsExportData ?? providerRankingsData ?? null,
				providerTokenData: providerTokenData ?? null,
				providerLatencyData: providerLatencyData ?? null,
			}),
			loadData,
		}),
		[rankingsExportData, providerRankingsData, providerTokenData, providerLatencyData, loadData],
	);

	const availableProviders = useMemo(
		() =>
			sanitizeSeriesLabels([
				...(providerRankingsData?.rankings.map((r) => r.provider) ?? []),
				...(providerTokenData?.providers ?? []),
				...(providerLatencyData?.providers ?? []),
				...(providerThroughputData?.providers ?? []),
			]),
		[providerRankingsData, providerTokenData?.providers, providerLatencyData?.providers, providerThroughputData?.providers],
	);
	// Legend lists mirror each chart's display order (top-N by its own ranking
	// metric, plus "Other" where the chart rolls up the tail) — index-based
	// colors must match the drawn series, not the API's alphabetical order.
	const providerTokenProviders = useMemo(
		() => computeDisplaySeries(providerTokenData?.buckets, providerTokenData?.providers, (b, p) => b.by_provider?.[p]?.total_tokens ?? 0),
		[providerTokenData],
	);
	const providerLatencyProviders = useMemo(
		() =>
			computeDisplaySeries(
				providerLatencyData?.buckets,
				providerLatencyData?.providers,
				(b, p) => b.by_provider?.[p]?.total_requests ?? 0,
				false,
			),
		[providerLatencyData],
	);
	const providerThroughputProviders = useMemo(
		() =>
			computeDisplaySeries(
				providerThroughputData?.buckets,
				providerThroughputData?.providers,
				(b, p) => b.by_provider?.[p]?.total_requests ?? 0,
				false,
			),
		[providerThroughputData],
	);

	return (
		<ProviderUsageTab
			providerRankingsData={providerRankingsData ?? null}
			loadingProviderRankings={loadingProviderRankings}
			providerTokenData={providerTokenData ?? null}
			providerLatencyData={providerLatencyData ?? null}
			providerThroughputData={providerThroughputData ?? null}
			loadingProviderTokens={loadingProviderTokens}
			loadingProviderLatency={loadingProviderLatency}
			loadingProviderThroughput={loadingProviderThroughput}
			startTime={startTime}
			endTime={endTime}
			providerTokenChartType={providerTokenChartType}
			providerLatencyChartType={providerLatencyChartType}
			providerThroughputChartType={providerThroughputChartType}
			providerTokenProvider={providerTokenProvider}
			providerLatencyProvider={providerLatencyProvider}
			providerThroughputProvider={providerThroughputProvider}
			availableProviders={availableProviders}
			providerTokenProviders={providerTokenProviders}
			providerLatencyProviders={providerLatencyProviders}
			providerThroughputProviders={providerThroughputProviders}
			onProviderTokenChartToggle={onProviderTokenChartToggle}
			onProviderLatencyChartToggle={onProviderLatencyChartToggle}
			onProviderThroughputChartToggle={onProviderThroughputChartToggle}
			onProviderTokenProviderChange={onProviderTokenProviderChange}
			onProviderLatencyProviderChange={onProviderLatencyProviderChange}
			onProviderThroughputProviderChange={onProviderThroughputProviderChange}
		/>
	);
});