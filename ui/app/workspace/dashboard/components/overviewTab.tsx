import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type {
	LatencyHistogramResponse,
	LogsHistogramResponse,
	ModelHistogramResponse,
	ThroughputHistogramResponse,
	TokenHistogramResponse,
} from "@/lib/types/logs";
import { COMPACT_NUMBER_FORMAT, formatRtkCompactNumber, formatRtkRatio, formatTokensAdaptive } from "@/lib/utils/numbers";
import type { RtkStatsHistogramResponse } from "@/lib/types/plugins";
import { Info } from "lucide-react";
import NumberFlow from "@number-flow/react";
import { memo, useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
	CHART_COLORS,
	CHART_HEADER_LEGEND_CLASS,
	LATENCY_COLORS,
	OTHER_SERIES_COLOR,
	OTHER_SERIES_KEY,
	OTHER_SERIES_LABEL,
	THROUGHPUT_COLOR,
	formatTokensPerSecond,
	getModelColor,
} from "../utils/chartUtils";
import { ChartCard } from "./charts/chartCard";
import { type ChartType, ChartTypeToggle } from "./charts/chartTypeToggle";
import ExternalCacheTokenMeterChart from "./charts/externalCacheTokenMeterChart";
import { LatencyChart } from "./charts/latencyChart";
import { ThroughputChart } from "./charts/throughputChart";
import { LogVolumeChart } from "./charts/logVolumeChart";
import { ModelFilterSelect } from "./charts/modelFilterSelect";
import { ModelUsageChart } from "./charts/modelUsageChart";
import { RtkCompressionChart } from "./charts/rtkCompressionChart";
import { TokenUsageChart } from "./charts/tokenUsageChart";

export interface OverviewTabProps {
	// Data
	histogramData: LogsHistogramResponse | null;
	tokenData: TokenHistogramResponse | null;
	modelData: ModelHistogramResponse | null;
	latencyData: LatencyHistogramResponse | null;
	throughputData: ThroughputHistogramResponse | null;
	rtkData: RtkStatsHistogramResponse | null;

	// Loading states
	loadingHistogram: boolean;
	loadingTokens: boolean;
	loadingModels: boolean;
	loadingLatency: boolean;
	loadingThroughput: boolean;
	loadingRtk: boolean;

	// Time range
	startTime: number;
	endTime: number;

	// Chart types
	volumeChartType: ChartType;
	tokenChartType: ChartType;
	modelChartType: ChartType;
	latencyChartType: ChartType;
	throughputChartType: ChartType;
	rtkChartType: ChartType;

	// Model selections
	usageModel: string;

	// Derived model lists
	usageModels: string[];

	// Chart type toggle callbacks
	onVolumeChartToggle: (type: ChartType) => void;
	onTokenChartToggle: (type: ChartType) => void;
	onModelChartToggle: (type: ChartType) => void;
	onLatencyChartToggle: (type: ChartType) => void;
	onThroughputChartToggle: (type: ChartType) => void;
	onRtkChartToggle: (type: ChartType) => void;

	// Filter callbacks
	onUsageModelChange: (model: string) => void;
}

function OverviewTabImpl({
	histogramData,
	tokenData,
	modelData,
	latencyData,
	throughputData,
	rtkData,
	loadingHistogram,
	loadingTokens,
	loadingModels,
	loadingLatency,
	loadingThroughput,
	loadingRtk,
	startTime,
	endTime,
	volumeChartType,
	tokenChartType,
	modelChartType,
	latencyChartType,
	throughputChartType,
	rtkChartType,
	usageModel,
	usageModels,
	onVolumeChartToggle,
	onTokenChartToggle,
	onModelChartToggle,
	onLatencyChartToggle,
	onThroughputChartToggle,
	onRtkChartToggle,
	onUsageModelChange,
}: OverviewTabProps) {
	const { t } = useTranslation("dashboard");

	const volumeTotal = useMemo(() => {
		if (!histogramData?.buckets) return null;
		return histogramData.buckets.reduce((sum, b) => sum + (b.count ?? 0), 0);
	}, [histogramData]);

	const tokenInputTotal = useMemo(() => {
		if (!tokenData?.buckets) return null;
		return tokenData.buckets.reduce((sum, b) => sum + (b.prompt_tokens ?? 0), 0);
	}, [tokenData]);

	const tokenOutputTotal = useMemo(() => {
		if (!tokenData?.buckets) return null;
		return tokenData.buckets.reduce((sum, b) => sum + (b.completion_tokens ?? 0), 0);
	}, [tokenData]);

	const tokenTotal = useMemo(() => {
		if (!tokenData?.buckets) return null;
		return tokenData.buckets.reduce((sum, b) => sum + (b.total_tokens ?? 0), 0);
	}, [tokenData]);

	const rtkLifetimeSaved = useMemo(() => {
		return rtkData?.lifetime_totals?.tokensSaved ?? null;
	}, [rtkData]);

	const rtkLifetimeRatio = useMemo(() => {
		return rtkData?.lifetime_totals?.compressionRatio ?? null;
	}, [rtkData]);

	const rtkLifetimeInvocations = useMemo(() => {
		return rtkData?.lifetime_totals?.invocations ?? null;
	}, [rtkData]);

	const modelUsageTotal = useMemo(() => {
		if (!modelData?.buckets) return null;
		if (usageModel === "all") {
			let sum = 0;
			for (const b of modelData.buckets) {
				if (!b.by_model) continue;
				for (const m of modelData.models) sum += b.by_model[m]?.total ?? 0;
			}
			return sum;
		}
		return modelData.buckets.reduce((sum, b) => sum + (b.by_model?.[usageModel]?.total ?? 0), 0);
	}, [modelData, usageModel]);

	const latencyAvg = useMemo(() => {
		if (!latencyData?.buckets || latencyData.buckets.length === 0) return null;
		let weighted = 0;
		let count = 0;
		for (const b of latencyData.buckets) {
			const reqs = b.total_requests ?? 0;
			if (reqs === 0) continue;
			weighted += (b.avg_latency ?? 0) * reqs;
			count += reqs;
		}
		return count > 0 ? weighted / count : null;
	}, [latencyData]);

	// Weighted-average throughput across buckets (weighted by request count) so
	// the header figure matches how the aggregate rate is computed per bucket.
	const throughputAvg = useMemo(() => {
		if (!throughputData?.buckets || throughputData.buckets.length === 0) return null;
		let weighted = 0;
		let count = 0;
		for (const b of throughputData.buckets) {
			const reqs = b.total_requests ?? 0;
			if (reqs === 0) continue;
			weighted += (b.tokens_per_second ?? 0) * reqs;
			count += reqs;
		}
		return count > 0 ? weighted / count : null;
	}, [throughputData]);

	return (
		<>
			{/* Charts Grid */}
			<div className="grid grid-cols-1 gap-2 lg:grid-cols-2 2xl:grid-cols-3">
				{/* Log Volume Chart */}
				<ChartCard
					title={t("charts.requestVolume")}
					loading={loadingHistogram}
					testId="chart-log-volume"
					totalLabel={t("charts.total")}
					total={volumeTotal !== null ? <NumberFlow value={volumeTotal} format={COMPACT_NUMBER_FORMAT} /> : undefined}
					totalTooltip={volumeTotal !== null ? volumeTotal.toLocaleString("en-US") : undefined}
					legend={
						<div className={CHART_HEADER_LEGEND_CLASS}>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: CHART_COLORS.success }} />
								<span className="text-muted-foreground">{t("charts.success")}</span>
							</span>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: CHART_COLORS.error }} />
								<span className="text-muted-foreground">{t("charts.error")}</span>
							</span>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: CHART_COLORS.cancelled }} />
								<span className="text-muted-foreground">{t("charts.cancelled")}</span>
							</span>
						</div>
					}
					controls={
						<ChartTypeToggle chartType={volumeChartType} onToggle={onVolumeChartToggle} data-testid="dashboard-volume-chart-toggle" />
					}
				>
					<LogVolumeChart data={histogramData} chartType={volumeChartType} startTime={startTime} endTime={endTime} />
				</ChartCard>

				{/* Token Usage Chart */}
				<ChartCard
					title={t("charts.tokenUsage")}
					loading={loadingTokens}
					testId="chart-token-usage"
					totalLabel={t("charts.input")}
					total={
						tokenInputTotal !== null ? (
							<span className="truncate whitespace-nowrap">{formatTokensAdaptive(tokenInputTotal)}</span>
						) : undefined
					}
					totalTooltip={tokenInputTotal !== null ? formatTokensAdaptive(tokenInputTotal) : undefined}
					secondaryTotalLabel={t("charts.output")}
					secondaryTotal={
						tokenOutputTotal !== null ? (
							<span className="truncate whitespace-nowrap">{formatTokensAdaptive(tokenOutputTotal)}</span>
						) : undefined
					}
					secondaryTotalTooltip={tokenOutputTotal !== null ? formatTokensAdaptive(tokenOutputTotal) : undefined}
					tertiaryTotalLabel={t("charts.total")}
					tertiaryTotal={
						tokenTotal !== null ? <span className="truncate whitespace-nowrap">{formatTokensAdaptive(tokenTotal)}</span> : undefined
					}
					tertiaryTotalTooltip={tokenTotal !== null ? formatTokensAdaptive(tokenTotal) : undefined}
					legend={
						<div className={CHART_HEADER_LEGEND_CLASS}>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: CHART_COLORS.promptTokens }} />
								<span className="text-muted-foreground">{t("charts.input")}</span>
							</span>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: CHART_COLORS.completionTokens }} />
								<span className="text-muted-foreground">{t("charts.output")}</span>
							</span>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: CHART_COLORS.cachedReadTokens }} />
								<span className="text-muted-foreground">{t("charts.cached")}</span>
							</span>
						</div>
					}
					controls={<ChartTypeToggle chartType={tokenChartType} onToggle={onTokenChartToggle} data-testid="dashboard-token-chart-toggle" />}
				>
					<TokenUsageChart data={tokenData} chartType={tokenChartType} startTime={startTime} endTime={endTime} />
				</ChartCard>

				{/* RTK Compression Chart */}
				<ChartCard
					title={t("charts.rtkCompression")}
					loading={loadingRtk}
					testId="chart-rtk-compression"
					totalLabel={t("charts.rtkSaved")}
					total={
						rtkLifetimeSaved !== null ? (
							<span className="truncate whitespace-nowrap">{formatRtkCompactNumber(rtkLifetimeSaved)}</span>
						) : undefined
					}
					totalTooltip={rtkLifetimeSaved !== null ? formatRtkCompactNumber(rtkLifetimeSaved) : undefined}
					secondaryTotalLabel={t("charts.rtkRatio")}
					secondaryTotal={
						rtkLifetimeRatio !== null ? <span className="truncate whitespace-nowrap">{formatRtkRatio(rtkLifetimeRatio)}</span> : undefined
					}
					secondaryTotalTooltip={rtkLifetimeRatio !== null ? formatRtkRatio(rtkLifetimeRatio) : undefined}
					tertiaryTotalLabel={t("charts.rtkInvocations")}
					tertiaryTotal={
						rtkLifetimeInvocations !== null ? (
							<span className="truncate whitespace-nowrap">{formatRtkCompactNumber(rtkLifetimeInvocations)}</span>
						) : undefined
					}
					tertiaryTotalTooltip={rtkLifetimeInvocations !== null ? formatRtkCompactNumber(rtkLifetimeInvocations) : undefined}
					legend={
						<div className={CHART_HEADER_LEGEND_CLASS}>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: "#3b82f6" }} />
								<span className="text-muted-foreground">{t("charts.rtkCompressed")}</span>
							</span>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: "#22c55e" }} />
								<span className="text-muted-foreground">{t("charts.rtkSaved")}</span>
							</span>
							<span className="flex items-center gap-1.5">
								<Tooltip>
									<TooltipTrigger asChild>
										<button
											type="button"
											data-testid="rtk-compression-info-btn"
											className="text-zinc-500 transition-colors hover:text-zinc-300"
											aria-label={t("charts.rtkTooltip")}
										>
											<Info className="h-3 w-3" />
										</button>
									</TooltipTrigger>
									<TooltipContent side="top">{t("charts.rtkTooltip")}</TooltipContent>
								</Tooltip>
							</span>
						</div>
					}
					controls={<ChartTypeToggle chartType={rtkChartType} onToggle={onRtkChartToggle} data-testid="dashboard-rtk-chart-toggle" />}
				>
					<RtkCompressionChart data={rtkData} chartType={rtkChartType} startTime={startTime} endTime={endTime} />
				</ChartCard>

				{/* External Cache Hit Rate Meter */}
				<ChartCard title={t("charts.externalCacheHitRate")} loading={loadingTokens} testId="chart-cache-external">
					<ExternalCacheTokenMeterChart data={tokenData} />
				</ChartCard>

				{/* Model Usage Chart */}
				<ChartCard
					title={t("charts.modelUsage")}
					loading={loadingModels}
					testId="chart-model-usage"
					totalLabel={t("charts.total")}
					total={modelUsageTotal !== null ? <NumberFlow value={modelUsageTotal} format={COMPACT_NUMBER_FORMAT} /> : undefined}
					totalTooltip={modelUsageTotal !== null ? modelUsageTotal.toLocaleString("en-US") : undefined}
					legend={
						<div className={CHART_HEADER_LEGEND_CLASS}>
							{usageModel === "all" ? (
								usageModels.length > 0 && (
									<>
										<Tooltip>
											<TooltipTrigger asChild>
												<span tabIndex={0} data-testid="usage-legend-trigger" className="flex items-center gap-1">
													<span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: getModelColor(0) }} />
													<span className="text-muted-foreground max-w-[100px] truncate">{usageModels[0]}</span>
												</span>
											</TooltipTrigger>
											<TooltipContent>{usageModels[0]}</TooltipContent>
										</Tooltip>
										{usageModels.length > 1 && (
											<Tooltip>
												<TooltipTrigger asChild>
													<span tabIndex={0} data-testid="usage-legend-more-trigger" className="text-muted-foreground cursor-default">
														{t("rankings.more", { count: usageModels.length - 1 })}
													</span>
												</TooltipTrigger>
												<TooltipContent>
													<div className="flex flex-col gap-1">
														{usageModels.slice(1).map((model, idx) => (
															<span key={model} className="flex items-center gap-1">
																<span
																	className="h-2 w-2 shrink-0 rounded-full"
																	style={{
																		backgroundColor: model === OTHER_SERIES_KEY ? OTHER_SERIES_COLOR : getModelColor(idx + 1),
																	}}
																/>
																{model === OTHER_SERIES_KEY ? OTHER_SERIES_LABEL : model}
															</span>
														))}
													</div>
												</TooltipContent>
											</Tooltip>
										)}
									</>
								)
							) : (
								<>
									<span className="flex items-center gap-1">
										<span className="h-2 w-2 rounded-full" style={{ backgroundColor: CHART_COLORS.success }} />
										<span className="text-muted-foreground">{t("charts.success")}</span>
									</span>
									<span className="flex items-center gap-1">
										<span className="h-2 w-2 rounded-full" style={{ backgroundColor: CHART_COLORS.error }} />
										<span className="text-muted-foreground">{t("charts.error")}</span>
									</span>
									<span className="flex items-center gap-1">
										<span className="h-2 w-2 rounded-full" style={{ backgroundColor: CHART_COLORS.cancelled }} />
										<span className="text-muted-foreground">{t("charts.cancelled")}</span>
									</span>
								</>
							)}
						</div>
					}
					controls={
						<>
							<ModelFilterSelect
								models={modelData?.models ?? []}
								selectedModel={usageModel}
								onModelChange={onUsageModelChange}
								data-testid="dashboard-usage-model-filter"
							/>
							<ChartTypeToggle chartType={modelChartType} onToggle={onModelChartToggle} data-testid="dashboard-usage-chart-toggle" />
						</>
					}
				>
					<ModelUsageChart data={modelData} chartType={modelChartType} startTime={startTime} endTime={endTime} selectedModel={usageModel} />
				</ChartCard>

				{/* Latency Chart */}
				<ChartCard
					title={t("charts.latency")}
					loading={loadingLatency}
					testId="chart-latency"
					totalLabel={t("charts.avg")}
					total={
						latencyAvg !== null ? (
							<NumberFlow value={latencyAvg} format={{ minimumFractionDigits: 2, maximumFractionDigits: 2 }} suffix="ms" />
						) : undefined
					}
					totalTooltip={latencyAvg !== null ? `${latencyAvg.toLocaleString("en-US", { maximumFractionDigits: 6 })}ms` : undefined}
					legend={
						<div className={CHART_HEADER_LEGEND_CLASS}>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: LATENCY_COLORS.avg }} />
								<span className="text-muted-foreground">{t("charts.avg")}</span>
							</span>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: LATENCY_COLORS.p90 }} />
								<span className="text-muted-foreground">{t("charts.p90")}</span>
							</span>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: LATENCY_COLORS.p95 }} />
								<span className="text-muted-foreground">{t("charts.p95")}</span>
							</span>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: LATENCY_COLORS.p99 }} />
								<span className="text-muted-foreground">{t("charts.p99")}</span>
							</span>
						</div>
					}
					controls={
						<ChartTypeToggle chartType={latencyChartType} onToggle={onLatencyChartToggle} data-testid="dashboard-latency-chart-toggle" />
					}
				>
					<LatencyChart data={latencyData} chartType={latencyChartType} startTime={startTime} endTime={endTime} />
				</ChartCard>

				{/* Throughput (tokens/sec) Chart */}
				<ChartCard
					title={t("charts.throughput")}
					loading={loadingThroughput}
					testId="chart-throughput"
					totalLabel={t("charts.avg")}
					total={
						throughputAvg !== null ? <span className="truncate whitespace-nowrap">{formatTokensPerSecond(throughputAvg)}</span> : undefined
					}
					totalTooltip={
						throughputAvg !== null ? `${throughputAvg.toLocaleString("en-US", { maximumFractionDigits: 2 })} tokens/sec` : undefined
					}
					legend={
						<div className={CHART_HEADER_LEGEND_CLASS}>
							<span className="flex items-center gap-1">
								<span className="h-2 w-2 rounded-full" style={{ backgroundColor: THROUGHPUT_COLOR }} />
								<span className="text-muted-foreground">{t("charts.tokensPerSec")}</span>
							</span>
						</div>
					}
					controls={
						<ChartTypeToggle
							chartType={throughputChartType}
							onToggle={onThroughputChartToggle}
							data-testid="dashboard-throughput-chart-toggle"
						/>
					}
				>
					<ThroughputChart data={throughputData} chartType={throughputChartType} startTime={startTime} endTime={endTime} />
				</ChartCard>
			</div>
		</>
	);
}
export const OverviewTab = memo(OverviewTabImpl);