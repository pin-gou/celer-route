/**
 * @file Timeline page — Gantt chart view of request logs
 */

import { useGetLogsQuery } from "@/lib/store";
import type { LogEntry } from "@/lib/types/logs";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useTranslation } from "react-i18next";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { LogsTimeline } from "./views/logsTimeline";
import { TimelineToolbar, type TimelineMode } from "./views/timelineToolbar";
import { TimelineLegend } from "./views/timelineLegend";
import { LogDetailSheet } from "@/app/workspace/logs/sheets/logDetailsSheet";
import { useLogsTimelineSSE, type ActiveLogEntry } from "@/hooks/useLogsTimelineSSE";
import { useQueryStates, parseAsString } from "nuqs";
import { useNavigate } from "@tanstack/react-router";
import { summarizeTimelineStats } from "./views/timelineStats";
import { Card, CardContent } from "@/components/ui/card";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import NumberFlow from "@number-flow/react";
import { Activity, BarChart, CheckCircle, Clock, Hash, Info, XCircle } from "lucide-react";

// Visible window (ms) at zoom = 1. The total rendered span is VMS / zoom.
// Matches OmniRoute's 5-minute default — tight enough to see individual bars,
// loose enough to show a meaningful throughput pattern.
const BASE_VISIBLE_WINDOW_MS = 5 * 60 * 1000;
// The fetched API window is wider so there is room to pan without refetching
// on every drag. 20 min gives a 4x buffer around the 5 min visible window.
const FETCH_RANGE_MS = 20 * 60 * 1000;
// When the visible window drifts more than 25% of the fetched range past its
// center we debounce-refetch centered on the new position.
const REFETCH_DRIFT_RATIO = 0.25;
const REFETCH_DEBOUNCE_MS = 300;
// Follow mode: NOW line pinned at 85% of the canvas, window scrolls with time.
const FOLLOW_LINE_X = 0.85;
// Live mode: NOW line snaps to 90%, window scrolls in whole slots.
const LIVE_LINE_FRACTION = 0.9;

const ZOOM_MIN = 0.25;
const ZOOM_MAX = 8;

// Stats throttling: the Gantt chart's NOW line and bars animate at up to
// ~20fps (nowMs). Recomputing visibleLogsForStats + timelineStats every frame
// is unnecessary — the stat cards only need freshness to the second. This
// decoupling keeps stats O(n) work off the hot rAF path.
const STATS_INTERVAL_MS = 300;

// Local HH:MM:SS time-of-day, matching the timeline axis labels (local time).
function formatTimeOfDay(ms: number): string {
	const d = new Date(ms);
	const pad = (n: number) => n.toString().padStart(2, "0");
	return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function computeVisibleWindow(
	mode: TimelineMode,
	nowMs: number,
	windowMs: number,
	liveBaseMs: number,
	panFrozenMs: number,
	panOffsetMs: number,
): { start: number; end: number } {
	if (mode === "follow") {
		const end = nowMs + windowMs * (1 - FOLLOW_LINE_X);
		return { start: end - windowMs, end };
	}
	if (mode === "live") {
		const base = liveBaseMs || nowMs;
		const elapsed = nowMs - base;
		const snapWindow = windowMs * LIVE_LINE_FRACTION;
		const slot = elapsed % snapWindow;
		const start = nowMs - slot - windowMs * (1 - LIVE_LINE_FRACTION);
		return { start, end: start + windowMs };
	}
	const base = panFrozenMs || nowMs;
	const start = base - windowMs * 0.5 + panOffsetMs;
	return { start, end: start + windowMs };
}

// Fields that SSE can update on an existing log entry. These are the only
// fields we overwrite during merge — rich fields like input_history,
// output_message, created_at, raw_request, raw_response are preserved from
// the authoritative API fetch.
const SSE_LIVE_FIELDS: (keyof LogEntry)[] = [
	"status",
	"provider",
	"model",
	"object",
	"stream",
	"latency",
	"timestamp",
	"token_usage",
	"cost",
	"number_of_retries",
	"fallback_index",
	"virtual_key_name",
	"app",
	"user_agent",
	"content_summary",
];

function mergeLogEntry(existing: LogEntry, live: Partial<LogEntry>): LogEntry {
	const merged = { ...existing };
	for (const field of SSE_LIVE_FIELDS) {
		const val = live[field];
		if (val !== undefined) {
			(merged as Record<string, unknown>)[field] = val;
		}
	}
	return merged;
}

export default function TimelinePage() {
	const { t } = useTranslation("logs");
	const navigate = useNavigate();
	const hasDeleteAccess = useRbac(RbacResource.Logs, RbacOperation.Delete);
	const hasRevealAccess = useRbac(RbacResource.Logs, RbacOperation.Reveal);

	// URL state — selected log for the detail sheet
	const [urlState, setUrlState] = useQueryStates({
		selected_log: parseAsString.withDefault(""),
	});

	// --- Time-window engine -------------------------------------------------
	const [mode, setMode] = useState<TimelineMode>("follow");
	const [zoom, setZoom] = useState(1);
	const [panOffsetMs, setPanOffsetMs] = useState(0);
	const [panFrozenMs, setPanFrozenMs] = useState(0);
	const [liveBaseMs, setLiveBaseMs] = useState(() => Date.now());
	const [nowMs, setNowMs] = useState(() => Date.now());

	// Throttled clock for stat cards — keeps O(n) aggregation off the hot rAF path.
	const [statsNowMs, setStatsNowMs] = useState(() => Date.now());
	useEffect(() => {
		const id = setInterval(() => setStatsNowMs(Date.now()), STATS_INTERVAL_MS);
		return () => clearInterval(id);
	}, []);

	// SSE delivers new completed logs — merge them into the main list.
	// We use a ref + callback pattern to avoid re-creating the SSE hook on
	// every log addition.
	const [extraLogs, setExtraLogs] = useState<LogEntry[]>([]);
	const extraLogsRef = useRef<LogEntry[]>(extraLogs);
	extraLogsRef.current = extraLogs;

	// Batch SSE completions so a burst of terminal events costs one
	// setExtraLogs (one re-render) instead of one per event.
	const pendingExtraLogsRef = useRef<Map<string, LogEntry>>(new Map());
	const extraLogsFlushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	const flushPendingExtraLogs = useCallback(() => {
		extraLogsFlushTimerRef.current = null;
		const pending = pendingExtraLogsRef.current;
		if (pending.size === 0) return;
		pendingExtraLogsRef.current = new Map();
		setExtraLogs((prev) => {
			let next = prev;
			for (const log of pending.values()) {
				const idx = next.findIndex((l) => l.id === log.id);
				if (idx >= 0) {
					if (next === prev) next = [...prev];
					next[idx] = { ...next[idx], ...log };
				} else {
					if (next === prev) next = [...prev];
					next.push(log);
				}
			}
			// Cap at 500 — older rows age out of the visible window as the clock
			// advances, and they are re-fetched if the user pans back. Keeping the
			// extra rows bounded prevents unbounded memory growth on the timeline.
			return next.length > 500 ? next.slice(next.length - 500) : next;
		});
	}, []);

	const onNewLog = useCallback(
		(entry: ActiveLogEntry) => {
			const log: LogEntry = {
				id: entry.id,
				object: "chat.completion",
				parent_request_id: "",
				timestamp: entry.timestamp ?? new Date().toISOString(),
				provider: entry.provider ?? "",
				model: entry.model ?? "",
				status: entry.status,
				latency: entry.latency ?? (null as unknown as number),
				stream: false,
				number_of_retries: 0,
				fallback_index: 0,
				cost: 0,
				input_history: [],
				responses_input_history: [],
				created_at: entry.timestamp ?? new Date().toISOString(),
				token_usage: entry.token_usage ?? undefined,
			};
			// Latest event per id wins inside the flush window.
			pendingExtraLogsRef.current.set(log.id, log);
			if (!extraLogsFlushTimerRef.current) {
				extraLogsFlushTimerRef.current = setTimeout(flushPendingExtraLogs, 300);
			}
		},
		[flushPendingExtraLogs],
	);

	// SSE hook — always connected, no polling needed
	const { activeLogs } = useLogsTimelineSSE({ onNewLog });

	// rAF clock — throttled while in live/pan so the NOW line animates smoothly
	// without forcing a re-render every frame; unthrottled in follow mode.
	const animRef = useRef<number>(0);
	useEffect(() => {
		let last = 0;
		const throttle = mode === "follow" ? 0 : 50;
		const onFrame = (frame: number) => {
			if (frame - last >= throttle) {
				last = frame;
				setNowMs(Date.now());
			}
			animRef.current = requestAnimationFrame(onFrame);
		};
		animRef.current = requestAnimationFrame(onFrame);
		return () => cancelAnimationFrame(animRef.current);
	}, [mode]);

	const windowMs = BASE_VISIBLE_WINDOW_MS / zoom;

	const visibleWindow = useMemo(
		() => computeVisibleWindow(mode, nowMs, windowMs, liveBaseMs, panFrozenMs, panOffsetMs),
		[mode, nowMs, windowMs, liveBaseMs, panFrozenMs, panOffsetMs],
	);

	// --- Data fetching ------------------------------------------------------
	const now = Date.now();
	const [fetchRange, setFetchRange] = useState<{ start: string; end: string }>(() => ({
		start: new Date(now - FETCH_RANGE_MS / 2).toISOString(),
		end: new Date(now + FETCH_RANGE_MS / 2).toISOString(),
	}));
	const fetchRangeRef = useRef(fetchRange);
	const refetchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	const submitFetchRange = useCallback((centerMs: number) => {
		const start = new Date(centerMs - FETCH_RANGE_MS / 2).toISOString();
		const end = new Date(centerMs + FETCH_RANGE_MS / 2).toISOString();
		fetchRangeRef.current = { start, end };
		setFetchRange({ start, end });
	}, []);

	const syncFetchRangeOnDrift = useCallback(
		(visCenter: number) => {
			if (refetchTimerRef.current) clearTimeout(refetchTimerRef.current);
			refetchTimerRef.current = setTimeout(() => {
				const cur = fetchRangeRef.current;
				const curCenter = (new Date(cur.start).getTime() + new Date(cur.end).getTime()) / 2;
				const curSpan = new Date(cur.end).getTime() - new Date(cur.start).getTime();
				if (Math.abs(visCenter - curCenter) > curSpan * REFETCH_DRIFT_RATIO) {
					submitFetchRange(visCenter);
				}
			}, REFETCH_DEBOUNCE_MS);
		},
		[submitFetchRange],
	);

	useEffect(() => {
		syncFetchRangeOnDrift((visibleWindow.start + visibleWindow.end) / 2);
		return () => {
			if (refetchTimerRef.current) clearTimeout(refetchTimerRef.current);
		};
	}, [visibleWindow, syncFetchRangeOnDrift]);

	// Clear the extraLogs flush timer on unmount and discard pending entries.
	useEffect(() => {
		return () => {
			if (extraLogsFlushTimerRef.current) {
				clearTimeout(extraLogsFlushTimerRef.current);
				extraLogsFlushTimerRef.current = null;
			}
			pendingExtraLogsRef.current.clear();
		};
	}, []);

	const {
		data: logsData,
		isFetching,
		refetch,
	} = useGetLogsQuery(
		{
			filters: {
				start_time: fetchRange.start,
				end_time: fetchRange.end,
			} as any,
			pagination: {
				limit: 500,
				offset: 0,
				sort_by: "timestamp",
				order: "asc",
			},
		},
		// No polling — SSE handles real-time updates
	);

	const logs = useMemo(() => {
		const apiLogs = logsData?.logs ?? [];
		// Merge logs from SSE, deduplicating by id. When SSE carries a newer
		// update for an id already present from the API fetch (e.g. a
		// processing → terminal transition), overwrite only the live fields so
		// the authoritative row (input_history, raw payload, etc.) is kept.
		const merged = new Map<string, LogEntry>();
		for (const l of apiLogs) merged.set(l.id, l);
		for (const l of extraLogs) {
			const existing = merged.get(l.id);
			merged.set(l.id, existing ? mergeLogEntry(existing, l) : l);
		}
		return Array.from(merged.values()).sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
	}, [logsData, extraLogs]);

	// Window as seen by the throttled stats clock — recomputed at snapshot
	// freshness (up to ~500ms stale), sufficient for the stat cards.
	const statsWindow = useMemo(
		() => computeVisibleWindow(mode, statsNowMs, windowMs, liveBaseMs, panFrozenMs, panOffsetMs),
		[mode, statsNowMs, windowMs, liveBaseMs, panFrozenMs, panOffsetMs],
	);

	// Visible-window logs for stat cards: filter the merged log list to the
	// current canvas window so all stat cards reflect the same visible range.
	// Mirrors LogsTimeline's visibleLogs filter (logsTimeline.tsx). Uses the
	// throttled statsNowMs so this O(n) pass doesn't run on every rAF frame.
	const visibleLogsForStats = useMemo(
		() =>
			logs.filter((log) => {
				const start = new Date(log.timestamp).getTime();
				const end = log.status === "processing" ? statsNowMs : start + (log.latency ?? 0);
				return end >= statsWindow.start && start <= statsWindow.end;
			}),
		[logs, statsWindow, statsNowMs],
	);

	const [firstVisibleTimeMs] = useMemo(() => {
		let firstMs: number | null = null;
		for (const log of visibleLogsForStats) {
			const start = new Date(log.timestamp).getTime();
			if (firstMs === null || start < firstMs) firstMs = start;
		}
		return [firstMs] as const;
	}, [visibleLogsForStats]);

	// Stat cards: all computed from the visible-window log list so the cards
	// match the Gantt chart exactly. totalRequests is the live count of logs
	// in the visible window, not a stale backend snapshot. Window bounds are
	// passed so maxConcurrency measures concurrency within the visible window
	// only.
	const timelineStats = useMemo(
		() => summarizeTimelineStats(visibleLogsForStats, visibleLogsForStats.length, activeLogs.length, statsWindow.start, statsWindow.end),
		[visibleLogsForStats, activeLogs.length, statsWindow],
	);

	const statCards = useMemo(
		() => [
			{
				key: "active-requests",
				title: t("timeline.statCards.activeRequests"),
				value: <NumberFlow value={timelineStats.activeCount} format={{ notation: "compact" }} />,
				icon: <Activity className="size-4" />,
				subValue: (
					<span className="text-muted-foreground">
						{t("timeline.statCards.maxConcurrency")}: <strong className="text-foreground font-bold">{timelineStats.maxConcurrency}</strong>
					</span>
				),
			},
			{
				key: "total-requests",
				title: t("timeline.statCards.totalRequests"),
				value: <NumberFlow value={timelineStats.totalRequests} format={{ notation: "compact" }} />,
				icon: <BarChart className="size-4" />,
				subValue: (
					<span className="text-muted-foreground">
						{t("timeline.toolbar.visible")}: <strong className="text-foreground font-bold">{visibleLogsForStats.length}</strong> ·{" "}
						{t("timeline.statCards.firstRequest")}:{" "}
						<strong className="text-foreground font-bold">{firstVisibleTimeMs !== null ? formatTimeOfDay(firstVisibleTimeMs) : "—"}</strong>
					</span>
				),
			},
			{
				key: "success-rate",
				title: t("timeline.statCards.successRate"),
				value: <NumberFlow value={timelineStats.successRate} format={{ minimumFractionDigits: 1, maximumFractionDigits: 1 }} suffix="%" />,
				icon: <CheckCircle className="size-4" />,
				description: t("timeline.statCards.successRateDesc"),
				subValue: (
					<span className="text-muted-foreground">
						{t("timeline.statCards.errors")}: <strong className="text-foreground font-bold">{timelineStats.errorCount}</strong> ·{" "}
						{t("timeline.statCards.cancelled")}: <strong className="text-foreground font-bold">{timelineStats.cancelledCount}</strong>
					</span>
				),
			},
			{
				key: "avg-latency",
				title: t("timeline.statCards.avgLatency"),
				value: <NumberFlow value={timelineStats.avgLatency} format={{ minimumFractionDigits: 0, maximumFractionDigits: 0 }} suffix="ms" />,
				icon: <Clock className="size-4" />,
				description: t("timeline.statCards.avgLatencyDesc"),
				subValue: (
					<span className="text-muted-foreground">
						{t("timeline.statCards.maxLatency")}:{" "}
						<strong className="text-foreground font-bold">
							<NumberFlow
								value={timelineStats.maxLatency >= 1000 ? timelineStats.maxLatency / 1000 : timelineStats.maxLatency}
								format={
									timelineStats.maxLatency >= 1000
										? { minimumFractionDigits: 2, maximumFractionDigits: 2 }
										: { minimumFractionDigits: 0, maximumFractionDigits: 0 }
								}
							/>
						</strong>
						<span className="font-normal">{timelineStats.maxLatency >= 1000 ? "s" : "ms"}</span>
					</span>
				),
			},
			{
				key: "total-tokens",
				title: t("timeline.statCards.totalTokens"),
				value: (
					<NumberFlow
						value={
							timelineStats.totalTokens >= 1_000_000
								? timelineStats.totalTokens / 1_000_000
								: timelineStats.totalTokens >= 1_000
									? timelineStats.totalTokens / 1_000
									: timelineStats.totalTokens
						}
						format={
							timelineStats.totalTokens >= 1_000
								? { minimumFractionDigits: 1, maximumFractionDigits: 1, useGrouping: true }
								: { minimumFractionDigits: 0, maximumFractionDigits: 0, useGrouping: true }
						}
						suffix={timelineStats.totalTokens >= 1_000_000 ? "M" : timelineStats.totalTokens >= 1_000 ? "K" : ""}
					/>
				),
				icon: <Hash className="size-4" />,
				description: t("timeline.statCards.totalTokensDesc"),
				subValue: (
					<div className="flex items-center gap-1">
						<span className="text-muted-foreground">{t("timeline.statCards.in")}:</span>
						<strong>
							<NumberFlow
								value={
									timelineStats.promptTokens >= 1_000_000
										? timelineStats.promptTokens / 1_000_000
										: timelineStats.promptTokens >= 1_000
											? timelineStats.promptTokens / 1_000
											: timelineStats.promptTokens
								}
								format={
									timelineStats.promptTokens >= 1_000
										? { minimumFractionDigits: 1, maximumFractionDigits: 1, useGrouping: true }
										: { minimumFractionDigits: 0, maximumFractionDigits: 0, useGrouping: true }
								}
							/>
						</strong>
						<span>{timelineStats.promptTokens >= 1_000_000 ? "M" : timelineStats.promptTokens >= 1_000 ? "K" : ""}</span>
						<span className="text-muted-foreground mx-1">·</span>
						<span className="text-muted-foreground">{t("timeline.statCards.out")}:</span>
						<strong>
							<NumberFlow
								value={
									timelineStats.completionTokens >= 1_000_000
										? timelineStats.completionTokens / 1_000_000
										: timelineStats.completionTokens >= 1_000
											? timelineStats.completionTokens / 1_000
											: timelineStats.completionTokens
								}
								format={
									timelineStats.completionTokens >= 1_000
										? { minimumFractionDigits: 1, maximumFractionDigits: 1, useGrouping: true }
										: { minimumFractionDigits: 0, maximumFractionDigits: 0, useGrouping: true }
								}
							/>
						</strong>
						<span>{timelineStats.completionTokens >= 1_000_000 ? "M" : timelineStats.completionTokens >= 1_000 ? "K" : ""}</span>
					</div>
				),
			},
		],
		[t, timelineStats, visibleLogsForStats, firstVisibleTimeMs],
	);

	const selectedLog = useMemo(
		() => (urlState.selected_log ? (logs.find((l) => l.id === urlState.selected_log) ?? null) : null),
		[urlState.selected_log, logs],
	);

	const handleBarClick = useCallback(
		(log: LogEntry) => {
			setUrlState({ selected_log: log.id }, { history: "replace" });
		},
		[setUrlState],
	);

	const handleRefresh = useCallback(() => {
		refetch();
	}, [refetch]);

	const handleModeChange = useCallback((newMode: TimelineMode) => {
		setMode(newMode);
		setPanOffsetMs(0);
		if (newMode === "pan") setPanFrozenMs(Date.now());
		setLiveBaseMs(Date.now());
	}, []);

	const handleZoomChange = useCallback((nextZoom: number) => {
		setZoom(Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, nextZoom)));
	}, []);

	const handleReset = useCallback(() => {
		setZoom(1);
		setMode("follow");
		setPanOffsetMs(0);
		setLiveBaseMs(Date.now());
		setNowMs(Date.now());
	}, []);

	const handleDragPan = useCallback((offsetMs: number) => {
		setPanOffsetMs(offsetMs);
	}, []);

	const handleFilterByParentRequestId = useCallback(
		(parentRequestId: string) => {
			navigate({ to: "/workspace/logs", search: { parent_request_id: parentRequestId } as any });
		},
		[navigate],
	);

	return (
		<div className="dark:bg-card no-padding-parent no-border-parent flex h-[calc(100vh_-_16px)] flex-col gap-3 p-4">
			{/* Header */}
			<div className="flex items-center justify-between">
				<h1 className="text-lg font-semibold" data-testid="timeline-page-title">
					{t("timeline.page.title")}
				</h1>
				<TimelineLegend />
			</div>

			{/* Toolbar */}
			<TimelineToolbar
				mode={mode}
				onModeChange={handleModeChange}
				onRefresh={handleRefresh}
				zoom={zoom}
				onZoomChange={handleZoomChange}
				onReset={handleReset}
			/>

			{/* Stat cards */}
			<div className="grid shrink-0 grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-5">
				{statCards.map((card) => (
					<Card key={card.key} className="py-3 shadow-none" data-testid={`timeline-metric-${card.key}`}>
						<CardContent className="flex items-center justify-between px-3 transition-opacity duration-200">
							<div className="w-full min-w-0">
								<div className="text-muted-foreground flex items-center gap-1 text-xs">
									<span className="truncate">{card.title}</span>
									{"description" in card && card.description && (
										<Tooltip>
											<TooltipTrigger asChild>
												<button type="button" aria-label={`${card.title} info`} className="inline-flex items-center">
													<Info className="size-3 cursor-help" />
												</button>
											</TooltipTrigger>
											<TooltipContent className="max-w-72 text-left text-xs text-wrap">{card.description}</TooltipContent>
										</Tooltip>
									)}
								</div>
								<div className="truncate font-mono text-lg font-medium sm:text-xl">{card.value}</div>
								{"subValue" in card && card.subValue && <div className="truncate font-mono text-[10px] tabular-nums">{card.subValue}</div>}
							</div>
						</CardContent>
					</Card>
				))}
			</div>

			{/* Loading indicator */}
			{isFetching && (
				<div className="text-muted-foreground text-xs" data-testid="timeline-loading-indicator">
					{t("timeline.page.refreshing")}
				</div>
			)}

			{/* Gantt chart */}
			<div className="bg-card min-h-0 flex-1 overflow-x-auto overflow-y-hidden rounded-sm border p-3">
				<LogsTimeline
					logs={logs}
					timeRange={visibleWindow}
					onBarClick={handleBarClick}
					activeLogs={activeLogs}
					nowMs={nowMs}
					mode={mode}
					zoom={zoom}
					onZoomChange={handleZoomChange}
					panOffsetMs={panOffsetMs}
					onPanOffsetChange={handleDragPan}
					onModeChange={handleModeChange}
				/>
			</div>

			{/* Log detail sheet */}
			<LogDetailSheet
				log={selectedLog}
				open={selectedLog !== null}
				onOpenChange={(open) => !open && setUrlState({ selected_log: "" })}
				handleDelete={hasDeleteAccess ? undefined : undefined}
				canReveal={hasRevealAccess}
				onNavigate={(direction) => {
					const idx = logs.findIndex((l) => l.id === urlState.selected_log);
					if (direction === "prev" && idx > 0) {
						setUrlState({ selected_log: logs[idx - 1].id }, { history: "replace" });
					} else if (direction === "next" && idx < logs.length - 1) {
						setUrlState({ selected_log: logs[idx + 1].id }, { history: "replace" });
					}
				}}
				hasPrev={logs.findIndex((l) => l.id === urlState.selected_log) > 0}
				hasNext={logs.findIndex((l) => l.id === urlState.selected_log) < logs.length - 1}
				onFilterByParentRequestId={handleFilterByParentRequestId}
			/>
		</div>
	);
}