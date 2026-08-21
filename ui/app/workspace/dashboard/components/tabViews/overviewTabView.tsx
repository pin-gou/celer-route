import {
	useGetLogsHistogramQuery,
	useGetLogsLatencyHistogramQuery,
	useGetLogsModelHistogramQuery,
	useGetLogsThroughputHistogramQuery,
	useGetLogsTokenHistogramQuery,
	useLazyGetLogsHistogramQuery,
	useLazyGetLogsLatencyHistogramQuery,
	useLazyGetLogsModelHistogramQuery,
	useLazyGetLogsThroughputHistogramQuery,
	useLazyGetLogsTokenHistogramQuery,
} from "@/lib/store";
import { useGetRtkStatsHistogramQuery } from "@/lib/store/apis/pluginsApi";
import type { LogFilters } from "@/lib/types/logs";
import { forwardRef, useCallback, useImperativeHandle, useMemo } from "react";
import { computeDisplaySeries } from "../../utils/chartUtils";
import type { DashboardData } from "../../utils/exportUtils";
import type { ChartType } from "../charts/chartTypeToggle";
import { OverviewTab } from "../overviewTab";

export interface OverviewTabViewHandle {
	getData: () => Partial<DashboardData>;
	loadData: () => Promise<void>;
}

interface OverviewTabViewProps {
	filters: LogFilters;
	active: boolean;
	startTime: number;
	endTime: number;
	volumeChartType: ChartType;
	tokenChartType: ChartType;
	modelChartType: ChartType;
	latencyChartType: ChartType;
	throughputChartType: ChartType;
	rtkChartType: ChartType;
	usageModel: string;
	onVolumeChartToggle: (type: ChartType) => void;
	onTokenChartToggle: (type: ChartType) => void;
	onModelChartToggle: (type: ChartType) => void;
	onLatencyChartToggle: (type: ChartType) => void;
	onThroughputChartToggle: (type: ChartType) => void;
	onRtkChartToggle: (type: ChartType) => void;
	onUsageModelChange: (model: string) => void;
}

export const OverviewTabView = forwardRef<OverviewTabViewHandle, OverviewTabViewProps>(function OverviewTabView(
	{
		filters,
		active,
		startTime,
		endTime,
		volumeChartType,
		tokenChartType,
		modelChartType,
		latencyChartType,
		throughputChartType,
		rtkChartType,
		usageModel,
		onVolumeChartToggle,
		onTokenChartToggle,
		onModelChartToggle,
		onLatencyChartToggle,
		onThroughputChartToggle,
		onRtkChartToggle,
		onUsageModelChange,
	},
	ref,
) {
	const fetchArg = useMemo(() => ({ filters }), [filters]);
	const skipOpts = useMemo(() => ({ skip: !active }), [active]);

	const { data: histogramData, isLoading: loadingHistogram } = useGetLogsHistogramQuery(fetchArg, skipOpts);
	const { data: tokenData, isLoading: loadingTokens } = useGetLogsTokenHistogramQuery(fetchArg, skipOpts);
	const { data: modelData, isLoading: loadingModels } = useGetLogsModelHistogramQuery(fetchArg, skipOpts);
	const { data: latencyData, isLoading: loadingLatency } = useGetLogsLatencyHistogramQuery(fetchArg, skipOpts);
	const { data: throughputData, isLoading: loadingThroughput } = useGetLogsThroughputHistogramQuery(fetchArg, skipOpts);

	// RTK histogram — uses the same time range filters as the other charts.
	const rtkFetchArg = useMemo(() => {
		const p: { start_time?: string; end_time?: string; period?: string } = {};
		if (filters.period) {
			p.period = filters.period;
		} else {
			if (filters.start_time) p.start_time = filters.start_time;
			if (filters.end_time) p.end_time = filters.end_time;
		}
		return { filters: p };
	}, [filters]);
	const { data: rtkData, isLoading: loadingRtk } = useGetRtkStatsHistogramQuery(rtkFetchArg, skipOpts);

	const [triggerHistogram] = useLazyGetLogsHistogramQuery();
	const [triggerTokens] = useLazyGetLogsTokenHistogramQuery();
	const [triggerModels] = useLazyGetLogsModelHistogramQuery();
	const [triggerLatency] = useLazyGetLogsLatencyHistogramQuery();
	const [triggerThroughput] = useLazyGetLogsThroughputHistogramQuery();

	const loadData = useCallback(async () => {
		await Promise.all([
			triggerHistogram(fetchArg, true),
			triggerTokens(fetchArg, true),
			triggerModels(fetchArg, true),
			triggerLatency(fetchArg, true),
			triggerThroughput(fetchArg, true),
		]);
	}, [fetchArg, triggerHistogram, triggerTokens, triggerModels, triggerLatency, triggerThroughput]);

	useImperativeHandle(
		ref,
		() => ({
			getData: () => ({
				histogramData: histogramData ?? null,
				tokenData: tokenData ?? null,
				modelData: modelData ?? null,
				latencyData: latencyData ?? null,
				rtkHistogramData: rtkData ?? null,
			}),
			loadData,
		}),
		[histogramData, tokenData, modelData, latencyData, rtkData, loadData],
	);

	// Legend lists mirror the charts' display order (top-N by volume + "Other"),
	// not the API's alphabetical order — index-based colors must match the bars.
	const usageModels = useMemo(
		() => computeDisplaySeries(modelData?.buckets, modelData?.models, (b, m) => b.by_model?.[m]?.total ?? 0),
		[modelData],
	);

	return (
		<OverviewTab
			histogramData={histogramData ?? null}
			tokenData={tokenData ?? null}
			modelData={modelData ?? null}
			latencyData={latencyData ?? null}
			throughputData={throughputData ?? null}
			rtkData={rtkData ?? null}
			loadingHistogram={loadingHistogram}
			loadingTokens={loadingTokens}
			loadingModels={loadingModels}
			loadingLatency={loadingLatency}
			loadingThroughput={loadingThroughput}
			loadingRtk={loadingRtk}
			startTime={startTime}
			endTime={endTime}
			volumeChartType={volumeChartType}
			tokenChartType={tokenChartType}
			modelChartType={modelChartType}
			latencyChartType={latencyChartType}
			throughputChartType={throughputChartType}
			rtkChartType={rtkChartType}
			usageModel={usageModel}
			usageModels={usageModels}
			onVolumeChartToggle={onVolumeChartToggle}
			onTokenChartToggle={onTokenChartToggle}
			onModelChartToggle={onModelChartToggle}
			onLatencyChartToggle={onLatencyChartToggle}
			onThroughputChartToggle={onThroughputChartToggle}
			onRtkChartToggle={onRtkChartToggle}
			onUsageModelChange={onUsageModelChange}
		/>
	);
});