import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import ProviderIcons, { type ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import type {
	ProviderLatencyHistogramResponse,
	ProviderRankingEntry,
	ProviderRankingsResponse,
	ProviderThroughputHistogramResponse,
	ProviderTokenHistogramResponse,
} from "@/lib/types/logs";
import { formatCompactNumber as formatNumber, formatTokensAdaptive } from "@/lib/utils/numbers";
import NumberFlow from "@number-flow/react";
import { memo, useCallback, useMemo, useState } from "react";
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
import { ProviderFilterSelect } from "./charts/providerFilterSelect";
import { ProviderLatencyChart } from "./charts/providerLatencyChart";
import { ProviderThroughputChart } from "./charts/providerThroughputChart";
import { ProviderTokenChart } from "./charts/providerTokenChart";
import { formatCost, SortableHeader, TrendBadge } from "./rankingsShared";

type SortField = "total_requests" | "success_rate" | "total_tokens" | "total_cost" | "avg_latency" | "throughput";
type SortOrder = "asc" | "desc";

function formatLatency(ms: number): string {
	if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
	return `${ms.toFixed(0)}ms`;
}

export interface ProviderUsageTabProps {
	// Data
	providerRankingsData: ProviderRankingsResponse | null;
	providerTokenData: ProviderTokenHistogramResponse | null;
	providerLatencyData: ProviderLatencyHistogramResponse | null;
	providerThroughputData: ProviderThroughputHistogramResponse | null;

	// Loading states
	loadingProviderRankings: boolean;
	loadingProviderTokens: boolean;
	loadingProviderLatency: boolean;
	loadingProviderThroughput: boolean;

	// Time range
	startTime: number;
	endTime: number;

	// Chart types
	providerTokenChartType: ChartType;
	providerLatencyChartType: ChartType;
	providerThroughputChartType: ChartType;

	// Provider selections
	providerTokenProvider: string;
	providerLatencyProvider: string;
	providerThroughputProvider: string;

	// Derived provider lists
	availableProviders: string[];
	providerTokenProviders: string[];
	providerLatencyProviders: string[];
	providerThroughputProviders: string[];

	// Chart type toggle callbacks
	onProviderTokenChartToggle: (type: ChartType) => void;
	onProviderLatencyChartToggle: (type: ChartType) => void;
	onProviderThroughputChartToggle: (type: ChartType) => void;

	// Filter callbacks
	onProviderTokenProviderChange: (provider: string) => void;
	onProviderLatencyProviderChange: (provider: string) => void;
	onProviderThroughputProviderChange: (provider: string) => void;
}

function ProviderRankingsTable({ data, loading }: { data: ProviderRankingsResponse | null; loading: boolean }) {
	const { t } = useTranslation("dashboard");
	const [sortField, setSortField] = useState<SortField>("total_requests");
	const [sortOrder, setSortOrder] = useState<SortOrder>("desc");

	const handleSort = useCallback(
		(field: SortField) => {
			if (sortField === field) {
				setSortOrder((prev) => (prev === "desc" ? "asc" : "desc"));
			} else {
				setSortField(field);
				setSortOrder("desc");
			}
		},
		[sortField],
	);

	const sortedRankings = useMemo(() => {
		if (!data?.rankings) return [];
		return [...data.rankings].sort((a, b) => {
			const aVal = a[sortField];
			const bVal = b[sortField];
			return sortOrder === "desc" ? (bVal as number) - (aVal as number) : (aVal as number) - (bVal as number);
		});
	}, [data, sortField, sortOrder]);

	if (loading) {
		return (
			<Card className="rounded-sm p-4 shadow-none">
				<div className="space-y-3">
					<Skeleton className="h-6 w-48" />
					<Skeleton className="h-[300px] w-full" />
				</div>
			</Card>
		);
	}

	if (!sortedRankings.length) {
		return (
			<Card className="rounded-sm p-4 shadow-none">
				<div className="text-muted-foreground flex h-[200px] items-center justify-center text-sm">{t("empty.noModelUsage")}</div>
			</Card>
		);
	}

	return (
		<Card className="rounded-sm p-2 shadow-none" data-testid="dashboard-provider-rankings-table">
			<span className="text-primary pl-2 text-sm font-medium">{t("rankings.providerRankings")}</span>
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead className="w-12">#</TableHead>
						<TableHead>{t("rankings.provider")}</TableHead>
						<TableHead className="text-right">
							<SortableHeader
								label={t("rankings.requests")}
								field="total_requests"
								currentSort={sortField}
								currentOrder={sortOrder}
								onSort={handleSort}
							/>
						</TableHead>
						<TableHead className="text-right">
							<SortableHeader
								label={t("rankings.successRate")}
								field="success_rate"
								currentSort={sortField}
								currentOrder={sortOrder}
								onSort={handleSort}
							/>
						</TableHead>
						<TableHead className="text-right">
							<SortableHeader
								label={t("rankings.tokens")}
								field="total_tokens"
								currentSort={sortField}
								currentOrder={sortOrder}
								onSort={handleSort}
							/>
						</TableHead>
						<TableHead className="text-right">
							<SortableHeader
								label={t("rankings.cost")}
								field="total_cost"
								currentSort={sortField}
								currentOrder={sortOrder}
								onSort={handleSort}
							/>
						</TableHead>
						<TableHead className="text-right">
							<SortableHeader
								label={t("rankings.avgLatency")}
								field="avg_latency"
								currentSort={sortField}
								currentOrder={sortOrder}
								onSort={handleSort}
							/>
						</TableHead>
						<TableHead className="text-right">
							<SortableHeader
								label={t("rankings.throughput")}
								field="throughput"
								currentSort={sortField}
								currentOrder={sortOrder}
								onSort={handleSort}
							/>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{sortedRankings.map((entry: ProviderRankingEntry, index: number) => (
						<TableRow key={entry.provider}>
							<TableCell className="text-muted-foreground font-mono text-xs">{index + 1}</TableCell>
							<TableCell>
								<div className="flex items-center gap-2">
									{entry.provider in ProviderIcons ? (
										<RenderProviderIcon provider={entry.provider as ProviderIconType} size="xs" className="h-4 w-4 shrink-0" />
									) : (
										<span className="text-muted-foreground shrink-0 text-xs">{entry.provider}</span>
									)}
									<span className="font-medium">{entry.provider}</span>
								</div>
							</TableCell>
							<TableCell className="text-right">
								<div className="flex items-center justify-end gap-2">
									<span>{formatNumber(entry.total_requests)}</span>
									<TrendBadge value={entry.trend.requests_trend} isNew={!entry.trend.has_previous_period} />
								</div>
							</TableCell>
							<TableCell className="text-right">
								<span
									className={
										entry.success_rate >= 99
											? "text-emerald-600 dark:text-emerald-400"
											: entry.success_rate >= 95
												? "text-yellow-600 dark:text-yellow-400"
												: "text-red-600 dark:text-red-400"
									}
								>
									{entry.success_rate.toFixed(1)}%
								</span>
							</TableCell>
							<TableCell className="text-right">
								<div className="flex items-center justify-end gap-2">
									<span>{formatTokensAdaptive(entry.total_tokens)}</span>
									<TrendBadge value={entry.trend.tokens_trend} isNew={!entry.trend.has_previous_period} />
								</div>
							</TableCell>
							<TableCell className="text-right">
								<div className="flex items-center justify-end gap-2">
									<span>{formatCost(entry.total_cost)}</span>
									<TrendBadge value={entry.trend.cost_trend} positiveIsGood={false} isNew={!entry.trend.has_previous_period} />
								</div>
							</TableCell>
							<TableCell className="text-right">
								<div className="flex items-center justify-end gap-2">
									<span>{formatLatency(entry.avg_latency)}</span>
									<TrendBadge value={entry.trend.latency_trend} positiveIsGood={false} isNew={!entry.trend.has_previous_period} />
								</div>
							</TableCell>
							<TableCell className="text-right">
								<div className="flex items-center justify-end gap-2">
									<span>{formatTokensPerSecond(entry.throughput)}</span>
									<TrendBadge value={entry.trend.throughput_trend} isNew={!entry.trend.has_previous_period} />
								</div>
							</TableCell>
						</TableRow>
					))}
				</TableBody>
			</Table>
		</Card>
	);
}

function ProviderUsageTabImpl({
	providerRankingsData,
	providerTokenData,
	providerLatencyData,
	providerThroughputData,
	loadingProviderRankings,
	loadingProviderTokens,
	loadingProviderLatency,
	loadingProviderThroughput,
	startTime,
	endTime,
	providerTokenChartType,
	providerLatencyChartType,
	providerThroughputChartType,
	providerTokenProvider,
	providerLatencyProvider,
	providerThroughputProvider,
	availableProviders,
	providerTokenProviders,
	providerLatencyProviders,
	providerThroughputProviders,
	onProviderTokenChartToggle,
	onProviderLatencyChartToggle,
	onProviderThroughputChartToggle,
	onProviderTokenProviderChange,
	onProviderLatencyProviderChange,
	onProviderThroughputProviderChange,
}: ProviderUsageTabProps) {
	const { t } = useTranslation("dashboard");

	const providerTokenTotal = useMemo(() => {
		if (!providerTokenData?.buckets) return null;
		let sum = 0;
		for (const b of providerTokenData.buckets) {
			if (!b.by_provider) continue;
			if (providerTokenProvider === "all") {
				for (const p of providerTokenData.providers) sum += b.by_provider[p]?.total_tokens ?? 0;
			} else {
				sum += b.by_provider[providerTokenProvider]?.total_tokens ?? 0;
			}
		}
		return sum;
	}, [providerTokenData, providerTokenProvider]);

	const providerLatencyAvg = useMemo(() => {
		if (!providerLatencyData?.buckets) return null;
		let weighted = 0;
		let count = 0;
		for (const b of providerLatencyData.buckets) {
			if (!b.by_provider) continue;
			const providers = providerLatencyProvider === "all" ? providerLatencyData.providers : [providerLatencyProvider];
			for (const p of providers) {
				const s = b.by_provider[p];
				if (!s || !s.total_requests) continue;
				weighted += (s.avg_latency ?? 0) * s.total_requests;
				count += s.total_requests;
			}
		}
		return count > 0 ? weighted / count : null;
	}, [providerLatencyData, providerLatencyProvider]);

	const providerThroughputAvg = useMemo(() => {
		if (!providerThroughputData?.buckets) return null;
		let weighted = 0;
		let count = 0;
		for (const b of providerThroughputData.buckets) {
			if (!b.by_provider) continue;
			const providers = providerThroughputProvider === "all" ? providerThroughputData.providers : [providerThroughputProvider];
			for (const p of providers) {
				const s = b.by_provider[p];
				if (!s || !s.total_requests) continue;
				weighted += (s.tokens_per_second ?? 0) * s.total_requests;
				count += s.total_requests;
			}
		}
		return count > 0 ? weighted / count : null;
	}, [providerThroughputData, providerThroughputProvider]);

	return (
		<div className="flex flex-col gap-4">
			<div className="grid grid-cols-1 gap-2 lg:grid-cols-2 2xl:grid-cols-3">
				{/* Provider Token Usage Chart */}
				<ChartCard
					title={t("charts.providerTokenUsage")}
					loading={loadingProviderTokens}
					testId="chart-provider-tokens"
					totalLabel={t("charts.total")}
					total={
						providerTokenTotal !== null ? (
							<span className="truncate whitespace-nowrap">{formatTokensAdaptive(providerTokenTotal)}</span>
						) : undefined
					}
					totalTooltip={providerTokenTotal !== null ? formatTokensAdaptive(providerTokenTotal) : undefined}
					legend={
						<div className={CHART_HEADER_LEGEND_CLASS}>
							{providerTokenProvider === "all" ? (
								providerTokenProviders.length > 0 && (
									<>
										<Tooltip>
											<TooltipTrigger asChild>
												<span data-testid="provider-token-legend-trigger" className="flex items-center gap-1">
													<span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: getModelColor(0) }} />
													<span className="text-muted-foreground max-w-[100px] truncate">{providerTokenProviders[0]}</span>
												</span>
											</TooltipTrigger>
											<TooltipContent>{providerTokenProviders[0]}</TooltipContent>
										</Tooltip>
										{providerTokenProviders.length > 1 && (
											<Tooltip>
												<TooltipTrigger asChild>
													<button
														type="button"
														data-testid="provider-token-legend-more-trigger"
														className="text-muted-foreground cursor-default"
													>
														{t("rankings.more", { count: providerTokenProviders.length - 1 })}
													</button>
												</TooltipTrigger>
												<TooltipContent>
													<div className="flex flex-col gap-1">
														{providerTokenProviders.slice(1).map((provider, idx) => (
															<span key={provider} className="flex items-center gap-1">
																<span
																	className="h-2 w-2 shrink-0 rounded-full"
																	style={{ backgroundColor: provider === OTHER_SERIES_KEY ? OTHER_SERIES_COLOR : getModelColor(idx + 1) }}
																/>
																{provider === OTHER_SERIES_KEY ? OTHER_SERIES_LABEL : provider}
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
										<span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: CHART_COLORS.promptTokens }} />
										<span className="text-muted-foreground">{t("charts.input")}</span>
									</span>
									<span className="flex items-center gap-1">
										<span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: CHART_COLORS.completionTokens }} />
										<span className="text-muted-foreground">{t("charts.output")}</span>
									</span>
								</>
							)}
						</div>
					}
					controls={
						<>
							<ProviderFilterSelect
								providers={availableProviders}
								selectedProvider={providerTokenProvider}
								onProviderChange={onProviderTokenProviderChange}
								data-testid="dashboard-provider-token-filter"
							/>
							<ChartTypeToggle
								chartType={providerTokenChartType}
								onToggle={onProviderTokenChartToggle}
								data-testid="dashboard-provider-token-chart-toggle"
							/>
						</>
					}
				>
					<ProviderTokenChart
						data={providerTokenData}
						chartType={providerTokenChartType}
						startTime={startTime}
						endTime={endTime}
						selectedProvider={providerTokenProvider}
					/>
				</ChartCard>

				{/* Provider Latency Chart */}
				<ChartCard
					title={t("charts.providerLatency")}
					loading={loadingProviderLatency}
					testId="chart-provider-latency"
					totalLabel={t("charts.avg")}
					total={
						providerLatencyAvg !== null ? (
							<NumberFlow value={providerLatencyAvg} format={{ minimumFractionDigits: 2, maximumFractionDigits: 2 }} suffix="ms" />
						) : undefined
					}
					totalTooltip={
						providerLatencyAvg !== null ? `${providerLatencyAvg.toLocaleString("en-US", { maximumFractionDigits: 6 })}ms` : undefined
					}
					legend={
						<div className={CHART_HEADER_LEGEND_CLASS}>
							{providerLatencyProvider === "all" ? (
								providerLatencyProviders.length > 0 && (
									<>
										<Tooltip>
											<TooltipTrigger asChild>
												<span data-testid="provider-latency-legend-trigger" className="flex items-center gap-1">
													<span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: getModelColor(0) }} />
													<span className="text-muted-foreground max-w-[100px] truncate">{providerLatencyProviders[0]}</span>
												</span>
											</TooltipTrigger>
											<TooltipContent>{providerLatencyProviders[0]}</TooltipContent>
										</Tooltip>
										{providerLatencyProviders.length > 1 && (
											<Tooltip>
												<TooltipTrigger asChild>
													<button
														type="button"
														data-testid="provider-latency-legend-more-trigger"
														className="text-muted-foreground cursor-default"
													>
														{t("rankings.more", { count: providerLatencyProviders.length - 1 })}
													</button>
												</TooltipTrigger>
												<TooltipContent>
													<div className="flex flex-col gap-1">
														{providerLatencyProviders.slice(1).map((provider, idx) => (
															<span key={provider} className="flex items-center gap-1">
																<span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: getModelColor(idx + 1) }} />
																{provider}
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
								</>
							)}
						</div>
					}
					controls={
						<>
							<ProviderFilterSelect
								providers={availableProviders}
								selectedProvider={providerLatencyProvider}
								onProviderChange={onProviderLatencyProviderChange}
								data-testid="dashboard-provider-latency-filter"
							/>
							<ChartTypeToggle
								chartType={providerLatencyChartType}
								onToggle={onProviderLatencyChartToggle}
								data-testid="dashboard-provider-latency-chart-toggle"
							/>
						</>
					}
				>
					<ProviderLatencyChart
						data={providerLatencyData}
						chartType={providerLatencyChartType}
						startTime={startTime}
						endTime={endTime}
						selectedProvider={providerLatencyProvider}
					/>
				</ChartCard>

				{/* Provider Throughput (tokens/sec) Chart */}
				<ChartCard
					title={t("charts.providerThroughput")}
					loading={loadingProviderThroughput}
					testId="chart-provider-throughput"
					totalLabel={t("charts.avg")}
					total={
						providerThroughputAvg !== null ? (
							<span className="truncate whitespace-nowrap">{formatTokensPerSecond(providerThroughputAvg)}</span>
						) : undefined
					}
					totalTooltip={
						providerThroughputAvg !== null
							? `${providerThroughputAvg.toLocaleString("en-US", { maximumFractionDigits: 2 })} tokens/sec`
							: undefined
					}
					legend={
						<div className={CHART_HEADER_LEGEND_CLASS}>
							{providerThroughputProvider === "all" ? (
								providerThroughputProviders.length > 0 && (
									<>
										<Tooltip>
											<TooltipTrigger asChild>
												<span data-testid="provider-throughput-legend-trigger" className="flex items-center gap-1">
													<span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: getModelColor(0) }} />
													<span className="text-muted-foreground max-w-[100px] truncate">{providerThroughputProviders[0]}</span>
												</span>
											</TooltipTrigger>
											<TooltipContent>{providerThroughputProviders[0]}</TooltipContent>
										</Tooltip>
										{providerThroughputProviders.length > 1 && (
											<Tooltip>
												<TooltipTrigger asChild>
													<button
														type="button"
														data-testid="provider-throughput-legend-more-trigger"
														className="text-muted-foreground cursor-default"
													>
														{t("rankings.more", { count: providerThroughputProviders.length - 1 })}
													</button>
												</TooltipTrigger>
												<TooltipContent>
													<div className="flex flex-col gap-1">
														{providerThroughputProviders.slice(1).map((provider, idx) => (
															<span key={provider} className="flex items-center gap-1">
																<span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: getModelColor(idx + 1) }} />
																{provider}
															</span>
														))}
													</div>
												</TooltipContent>
											</Tooltip>
										)}
									</>
								)
							) : (
								<Tooltip>
									<TooltipTrigger asChild>
										<span data-testid="provider-throughput-legend-single-trigger" className="flex items-center gap-1">
											<span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: THROUGHPUT_COLOR }} />
											<span className="text-muted-foreground max-w-[100px] truncate">{providerThroughputProvider}</span>
										</span>
									</TooltipTrigger>
									<TooltipContent>{providerThroughputProvider}</TooltipContent>
								</Tooltip>
							)}
						</div>
					}
					controls={
						<>
							<ProviderFilterSelect
								providers={availableProviders}
								selectedProvider={providerThroughputProvider}
								onProviderChange={onProviderThroughputProviderChange}
								data-testid="dashboard-provider-throughput-filter"
							/>
							<ChartTypeToggle
								chartType={providerThroughputChartType}
								onToggle={onProviderThroughputChartToggle}
								data-testid="dashboard-provider-throughput-chart-toggle"
							/>
						</>
					}
				>
					<ProviderThroughputChart
						data={providerThroughputData}
						chartType={providerThroughputChartType}
						startTime={startTime}
						endTime={endTime}
						selectedProvider={providerThroughputProvider}
					/>
				</ChartCard>
			</div>

			{/* Provider Rankings table — mirrors the model-rankings tab table
			    but grouped by provider, with the same columns (requests, success
			    rate, tokens, cost, latency, throughput) and trend badges. */}
			<ProviderRankingsTable data={providerRankingsData} loading={loadingProviderRankings} />
		</div>
	);
}
export const ProviderUsageTab = memo(ProviderUsageTabImpl);