/**
 * @file Gantt timeline component for request timeline view
 *
 * Renders a horizontal Gantt chart with request bars, lane allocation,
 * hover tooltips, click callbacks, NOW line, zoom, pan, and touch support.
 */

import { formatCost } from "@/app/workspace/dashboard/utils/chartUtils";
import { getMessage } from "@/app/workspace/logs/views/columns";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import type { LogEntry } from "@/lib/types/logs";
import { format } from "date-fns";
import { cn } from "@/lib/utils";
import { useMemo, useState, useCallback, useRef, useEffect, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { TimelineMode } from "./timelineToolbar";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const BAR_HEIGHT = 28;
const LANE_GAP = 4;
const LANE_HEIGHT = BAR_HEIGHT + LANE_GAP;
const AXIS_HEIGHT = 32;
const MIN_BAR_WIDTH = 10;
const ZOOM_MIN = 0.25;
const ZOOM_MAX = 8;
const ZOOM_FACTOR = 1.25;
const DRAG_THRESHOLD_PX = 10;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface LogsTimelineProps {
	logs: LogEntry[];
	timeRange: { start: number; end: number };
	onBarClick?: (log: LogEntry) => void;
	activeLogs: Array<{
		id: string;
		status: string;
		provider?: string;
		model?: string;
		latency?: number | null;
		timestamp?: string;
		message?: string;
		content_summary?: string;
	}>;
	nowMs: number;
	mode: TimelineMode;
	zoom: number;
	onZoomChange: (z: number) => void;
	panOffsetMs: number;
	onPanOffsetChange: (offset: number) => void;
	onModeChange: (m: TimelineMode) => void;
	className?: string;
}

interface LaneBar {
	log: LogEntry;
	lane: number;
	leftPct: number;
	widthPct: number;
	statusColor: string;
	borderColor: string;
	isProcessing: boolean;
}

interface AxisTick {
	pct: number;
	label: string;
	kind: "minor" | "hour" | "day";
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getStatusColor(status: string): string {
	switch (status) {
		case "success":
			return "bg-green-500";
		case "error":
			return "bg-red-500";
		case "processing":
			return "bg-blue-400";
		case "cancelled":
			return "bg-gray-400";
		default:
			return "bg-gray-400";
	}
}

function getStatusBorderColor(status: string): string {
	switch (status) {
		case "success":
			return "border-green-500";
		case "error":
			return "border-red-500";
		case "processing":
			return "border-blue-400";
		case "cancelled":
			return "border-gray-400";
		default:
			return "border-gray-400";
	}
}

function getTpsBorderColor(tps: number): string {
	if (tps < 20) return "border-red-500 dark:border-red-400";
	if (tps < 50) return "border-amber-500 dark:border-amber-400";
	if (tps < 80) return "border-blue-500 dark:border-blue-400";
	return "border-green-600 dark:border-green-400";
}

function calcTps(log: LogEntry): number | null {
	if (log.status !== "success") return null;
	if (log.latency == null || log.latency <= 0) return null;
	const output = log.token_usage?.completion_tokens;
	if (!output) return null;
	return (output / log.latency) * 1000;
}

function allocateLanes(logs: LogEntry[], laneGapMs: number = 0): Map<string, number> {
	const laneMap = new Map<string, number>();
	const laneEndTimes: number[] = [];
	const sorted = [...logs].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
	for (const log of sorted) {
		const start = new Date(log.timestamp).getTime();
		// Processing logs are still running — they occupy their lane until
		// Infinity so no other log can overlap them.
		// laneGapMs accounts for the visual minWidth — bars that are close
		// together in time but visually overlapping due to MIN_BAR_WIDTH
		// get separate lanes so they don't stack on top of each other.
		const end = log.status === "processing" ? Infinity : start + (log.latency ?? 0) + laneGapMs;
		let assigned = false;
		for (let i = 0; i < laneEndTimes.length; i++) {
			if (start >= laneEndTimes[i]) {
				laneMap.set(log.id, i);
				laneEndTimes[i] = end;
				assigned = true;
				break;
			}
		}
		if (!assigned) {
			laneMap.set(log.id, laneEndTimes.length);
			laneEndTimes.push(end);
		}
	}
	return laneMap;
}

function formatAxisTime(ms: number): string {
	const d = new Date(ms);
	return `${d.getHours().toString().padStart(2, "0")}:${d.getMinutes().toString().padStart(2, "0")}:${d.getSeconds().toString().padStart(2, "0")}`;
}

function formatAxisDate(ms: number): string {
	const d = new Date(ms);
	return format(d, "yyyy-MM-dd");
}

function truncateModel(model: string | null): string {
	if (!model) return "";
	const parts = model.split("/");
	const short = parts[parts.length - 1];
	return short.length > 16 ? short.slice(0, 15) + "\u2026" : short;
}

function startOfHour(ms: number): number {
	const d = new Date(ms);
	d.setMinutes(0, 0, 0);
	return d.getTime();
}

function startOfDay(ms: number): number {
	const d = new Date(ms);
	d.setHours(0, 0, 0, 0);
	return d.getTime();
}

function computeAxisTicks(timeStart: number, timeEnd: number): AxisTick[] {
	const totalMs = timeEnd - timeStart;
	if (totalMs <= 0) return [];

	const MS_MIN = 60 * 1000;
	const MS_10MIN = 10 * MS_MIN;
	const MS_HOUR = 60 * MS_MIN;
	const MS_DAY = 24 * MS_HOUR;

	const ticks: AxisTick[] = [];

	const addTick = (ms: number, label: string, kind: AxisTick["kind"]) => {
		const pct = ((ms - timeStart) / totalMs) * 100;
		if (pct >= 0 && pct <= 100) ticks.push({ pct, label, kind });
	};

	if (totalMs <= MS_HOUR * 4) {
		const interval = totalMs <= MS_10MIN ? MS_MIN : MS_10MIN;
		const first = Math.ceil(timeStart / interval) * interval;
		for (let ms = first; ms <= timeEnd; ms += interval) {
			const isDay = startOfDay(ms) === ms;
			const isHour = startOfHour(ms) === ms;
			addTick(ms, isDay ? formatAxisDate(ms) : formatAxisTime(ms), isDay ? "day" : isHour ? "hour" : "minor");
		}
	} else if (totalMs <= MS_DAY) {
		const interval = totalMs <= MS_HOUR * 6 ? MS_10MIN : MS_HOUR;
		const first = Math.ceil(timeStart / interval) * interval;
		for (let ms = first; ms <= timeEnd; ms += interval) {
			const isDay = startOfDay(ms) === ms;
			addTick(ms, isDay ? formatAxisDate(ms) : formatAxisTime(ms), isDay ? "day" : "hour");
		}
	} else {
		const firstDay = Math.ceil(timeStart / MS_DAY) * MS_DAY;
		for (let ms = firstDay; ms <= timeEnd; ms += MS_DAY) {
			addTick(ms, formatAxisDate(ms), "day");
		}
	}

	return ticks;
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function LogsTimeline({
	logs,
	timeRange,
	onBarClick,
	activeLogs,
	nowMs,
	mode,
	zoom,
	onZoomChange,
	panOffsetMs,
	onPanOffsetChange,
	onModeChange,
	className,
}: LogsTimelineProps) {
	const { t } = useTranslation("logs");
	const [tooltipLog, setTooltipLog] = useState<LogEntry | null>(null);
	const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 });
	const [tooltipAbove, setTooltipAbove] = useState(true);
	const containerRef = useRef<HTMLDivElement>(null);
	const canvasRef = useRef<HTMLDivElement>(null);
	const [canvasWidth, setCanvasWidth] = useState(1200);

	// ResizeObserver
	useEffect(() => {
		const el = canvasRef.current;
		if (!el) return;
		const observer = new ResizeObserver((entries) => {
			for (const entry of entries) {
				setCanvasWidth(entry.contentRect.width);
			}
		});
		observer.observe(el);
		return () => observer.disconnect();
	}, []);

	const { timeStart, timeEnd, rangeDuration } = useMemo(() => {
		const start = timeRange.start;
		const end = timeRange.end;
		return { timeStart: start, timeEnd: end, rangeDuration: end - start };
	}, [timeRange]);

	// NOW line position (percentage from left). Computed from the window so it
	// always matches the page's FOLLOW_LINE_X placement (follow mode keeps the
	// window pinned such that nowMs lands at exactly that fraction).
	const nowLineX = useMemo(() => {
		const totalMs = timeEnd - timeStart;
		if (totalMs <= 0) return 50;
		return Math.max(0, Math.min(100, ((nowMs - timeStart) / totalMs) * 100));
	}, [nowMs, timeStart, timeEnd]);

	// Axis ticks
	const axisTicks = useMemo(() => computeAxisTicks(timeStart, timeEnd), [timeStart, timeEnd]);

	// Merge active logs
	const mergedLogs = useMemo(() => {
		if (activeLogs.length === 0) return logs;
		const logMap = new Map(logs.map((l) => [l.id, l]));
		for (const active of activeLogs) {
			if (!logMap.has(active.id)) {
				const startTime = active.timestamp ?? new Date().toISOString();
				logMap.set(active.id, {
					id: active.id,
					object: "chat.completion",
					parent_request_id: "",
					timestamp: startTime,
					provider: active.provider ?? "",
					model: active.model ?? "",
					status: active.status,
					latency: (active.latency ?? null) as unknown as number,
					stream: false,
					number_of_retries: 0,
					fallback_index: 0,
					cost: 0,
					input_history: [],
					responses_input_history: [],
					created_at: startTime,
					// Prefer the SSE `message` preview (last user prompt, computed
					// by the backend's activeEntryMessage): on non-hybrid log stores
					// content_summary is every message concatenated with the system
					// prompt first, so the tooltip would render the system prompt.
					content_summary: active.message || active.content_summary || "",
				} as LogEntry);
			}
		}
		return Array.from(logMap.values());
	}, [logs, activeLogs]);

	// Filter to visible window
	const visibleLogs = useMemo(
		() =>
			mergedLogs.filter((log) => {
				const start = new Date(log.timestamp).getTime();
				// Processing logs are still running — their end time is now.
				const end = log.status === "processing" ? nowMs : start + (log.latency ?? 0);
				return end >= timeStart && start <= timeEnd;
			}),
		[mergedLogs, timeStart, timeEnd, nowMs],
	);

	// Lanes
	const laneGapMs = (MIN_BAR_WIDTH / canvasWidth) * rangeDuration;
	const laneMap = useMemo(() => allocateLanes(visibleLogs, laneGapMs), [visibleLogs, laneGapMs]);
	const maxLane = useMemo(() => (laneMap.size > 0 ? Math.max(...laneMap.values()) : 0), [laneMap]);

	// Bar positions
	const bars: LaneBar[] = useMemo(
		() =>
			visibleLogs.map((log) => {
				const logStart = new Date(log.timestamp).getTime();
				const isProcessing = log.status === "processing";
				// Completed bars: normal position from start + latency.
				// Processing bars: right edge anchored to the NOW line, extending
				// leftward (into the past) by the elapsed time. MIN_BAR_WIDTH also
				// extends left, so a just-started bar appears AT the NOW line instead
				// of poking past it to the right.
				// leftPct/widthPct are intentionally NOT clamped to the canvas — bars
				// may extend past the left/right edges and are clipped by overflow-hidden.
				const elapsedPct = isProcessing
					? Math.max(0, ((nowMs - logStart) / rangeDuration) * 100)
					: rangeDuration > 0
						? ((log.latency ?? 0) / rangeDuration) * 100
						: 0;
				const widthPct = Math.max((MIN_BAR_WIDTH / canvasWidth) * 100, elapsedPct);
				const plainLeftPct = ((logStart - timeStart) / rangeDuration) * 100;
				const leftPct = isProcessing ? nowLineX - widthPct : plainLeftPct;
				const lane = laneMap.get(log.id) ?? 0;
				const tps = calcTps(log);
				const borderColor = log.status === "success" && tps != null ? getTpsBorderColor(tps) : getStatusBorderColor(log.status);
				return {
					log,
					lane,
					leftPct,
					widthPct,
					statusColor: getStatusColor(log.status),
					borderColor,
					isProcessing,
				};
			}),
		[visibleLogs, timeStart, rangeDuration, laneMap, canvasWidth, nowMs, nowLineX],
	);

	const contentHeight = Math.max((maxLane + 1) * LANE_HEIGHT + 20, 200);

	const handleMouseEnter = useCallback((log: LogEntry, event: React.MouseEvent) => {
		setTooltipLog(log);
		const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
		const containerRect = containerRef.current?.getBoundingClientRect();
		if (!containerRect) return;
		const x = rect.left - containerRect.left + rect.width / 2;
		const barTop = rect.top - containerRect.top;
		// ~60px estimated tooltip height; if tooltip above would overflow, flip below
		const canShowAbove = barTop > 60;
		setTooltipAbove(canShowAbove);
		setTooltipPos({
			x,
			y: canShowAbove ? barTop - 8 : rect.bottom - containerRect.top + 8,
		});
	}, []);

	const handleMouseLeave = useCallback(() => {
		setTooltipLog(null);
	}, []);

	const handleBarClick = useCallback(
		(log: LogEntry) => {
			onBarClick?.(log);
		},
		[onBarClick],
	);

	// --- Wheel zoom ---
	const handleWheel = useCallback(
		(e: React.WheelEvent) => {
			if (e.ctrlKey || e.metaKey) return;
			e.preventDefault();
			const factor = e.deltaY < 0 ? ZOOM_FACTOR : 1 / ZOOM_FACTOR;
			onZoomChange(zoom * factor);
		},
		[zoom, onZoomChange],
	);

	// --- Drag panning ---
	const dragStateRef = useRef<{
		startX: number;
		startOffset: number;
		startMode: TimelineMode;
		moved: boolean;
	} | null>(null);

	const handleMouseDown = useCallback(
		(e: React.MouseEvent) => {
			if (e.button !== 0) return;
			const targetMode = mode === "pan" ? mode : "pan";
			if (mode !== "pan") {
				onModeChange("pan");
			}
			dragStateRef.current = {
				startX: e.clientX,
				startOffset: panOffsetMs,
				startMode: targetMode,
				moved: false,
			};
		},
		[mode, panOffsetMs, onModeChange],
	);

	const handleMouseMove = useCallback(
		(e: React.MouseEvent) => {
			const ds = dragStateRef.current;
			if (!ds) return;
			const dx = e.clientX - ds.startX;
			if (!ds.moved && Math.abs(dx) < DRAG_THRESHOLD_PX) return;
			ds.moved = true;
			const msPerPx = rangeDuration / canvasWidth;
			onPanOffsetChange(ds.startOffset - dx * msPerPx);
		},
		[rangeDuration, canvasWidth, onPanOffsetChange],
	);

	const handleMouseUp = useCallback(() => {
		dragStateRef.current = null;
	}, []);

	// --- Touch handlers ---
	const touchStartRef = useRef<{
		x: number;
		y: number;
		dist: number;
		offset: number;
		moved: boolean;
	} | null>(null);

	const handleTouchStart = useCallback(
		(e: React.TouchEvent) => {
			if (e.touches.length === 1) {
				const touch = e.touches[0];
				touchStartRef.current = {
					x: touch.clientX,
					y: touch.clientY,
					dist: 0,
					offset: panOffsetMs,
					moved: false,
				};
			} else if (e.touches.length === 2) {
				const [a, b] = [e.touches[0], e.touches[1]];
				const dist = Math.hypot(b.clientX - a.clientX, b.clientY - a.clientY);
				touchStartRef.current = { x: 0, y: 0, dist, offset: zoom, moved: false };
			}
		},
		[panOffsetMs, zoom],
	);

	const handleTouchMove = useCallback(
		(e: React.TouchEvent) => {
			const ts = touchStartRef.current;
			if (!ts) return;
			if (e.touches.length === 1) {
				const touch = e.touches[0];
				const dx = touch.clientX - ts.x;
				const dy = touch.clientY - ts.y;
				if (!ts.moved && Math.hypot(dx, dy) < DRAG_THRESHOLD_PX) return;
				if (!ts.moved) {
					ts.moved = true;
					if (mode !== "pan") onModeChange("pan");
				}
				const msPerPx = rangeDuration / canvasWidth;
				onPanOffsetChange(ts.offset - dx * msPerPx);
			} else if (e.touches.length === 2 && ts.dist > 0) {
				e.preventDefault();
				const [a, b] = [e.touches[0], e.touches[1]];
				const dist = Math.hypot(b.clientX - a.clientX, b.clientY - a.clientY);
				const scale = dist / ts.dist;
				onZoomChange(Math.max(ZOOM_MIN, Math.min(ZOOM_MAX, ts.offset * scale)));
			}
		},
		[mode, rangeDuration, canvasWidth, onPanOffsetChange, onZoomChange],
	);

	const handleTouchEnd = useCallback(() => {
		touchStartRef.current = null;
	}, []);

	return (
		<div ref={containerRef} data-testid="logs-timeline" className={cn("relative h-full w-full", className)}>
			{/* Canvas area */}
			<div
				ref={canvasRef}
				data-testid="timeline-canvas"
				className={cn(
					"relative h-full select-none overflow-hidden",
					dragStateRef.current?.moved ? "cursor-grabbing" : mode === "pan" ? "cursor-grab" : "",
				)}
				style={{ touchAction: "none" }}
				onMouseDown={handleMouseDown}
				onMouseMove={handleMouseMove}
				onMouseUp={handleMouseUp}
				onMouseLeave={handleMouseUp}
				onWheel={handleWheel}
				onTouchStart={handleTouchStart}
				onTouchMove={handleTouchMove}
				onTouchEnd={handleTouchEnd}
				onTouchCancel={handleTouchEnd}
			>
				<div className="relative min-h-full" style={{ height: contentHeight }}>
					{/* Time axis */}
					<div className="bg-background sticky top-0 z-10 border-b" style={{ height: AXIS_HEIGHT }}>
						{axisTicks.map((tick, i) => (
							<div key={i} className="absolute" style={{ left: `${tick.pct}%`, top: 0 }}>
								<div
									className={cn(
										"w-px -translate-x-1/2",
										tick.kind === "day" ? "h-4 bg-indigo-400/60" : tick.kind === "hour" ? "h-3 bg-slate-400/60" : "h-2 bg-slate-400/30",
									)}
								/>
								<span className="text-muted-foreground mt-0.5 block -translate-x-1/2 text-center font-mono text-[9px] whitespace-nowrap">
									{tick.label}
								</span>
							</div>
						))}
					</div>

					{/* Horizontal grid lines */}
					{Array.from({ length: maxLane + 1 }, (_, i) => (
						<div
							key={`lane-${i}`}
							className="absolute right-0 left-0 border-b border-gray-100 dark:border-gray-800"
							style={{ top: AXIS_HEIGHT + i * LANE_HEIGHT + BAR_HEIGHT }}
						/>
					))}

					{/* Vertical grid lines from axis ticks */}
					{axisTicks.map((tick, i) => (
						<div
							key={`vgrid-${i}`}
							className="pointer-events-none absolute top-0 bottom-0 -translate-x-1/2"
							style={{
								left: `${tick.pct}%`,
								width: tick.kind === "day" ? 2 : 1,
								backgroundColor:
									tick.kind === "day" ? "rgba(99,102,241,0.5)" : tick.kind === "hour" ? "rgba(148,163,184,0.45)" : "rgba(148,163,184,0.15)",
								zIndex: 1,
							}}
						/>
					))}

					{/* Request bars */}
					{bars.map((bar) => (
						<div
							key={bar.log.id}
							data-testid="timeline-bar"
							data-log-id={bar.log.id}
							data-lane={bar.lane}
							data-bar-width={bar.widthPct.toFixed(2)}
							data-status-color={bar.statusColor}
							className={cn(
								"absolute cursor-pointer rounded-sm transition-opacity hover:opacity-80 flex items-center overflow-hidden px-1 border-2",
								bar.statusColor,
								bar.borderColor,
								bar.isProcessing && "ring-1 ring-blue-300/60",
							)}
							style={{
								left: `${bar.leftPct}%`,
								width: `${bar.widthPct}%`,
								transition: bar.isProcessing ? undefined : "width 0.5s ease",
								top: `${AXIS_HEIGHT + bar.lane * LANE_HEIGHT + 2}px`,
								height: `${BAR_HEIGHT}px`,
								minWidth: `${MIN_BAR_WIDTH}px`,
								zIndex: 2,
							}}
							onMouseEnter={(e) => handleMouseEnter(bar.log, e)}
							onMouseLeave={handleMouseLeave}
							onClick={() => handleBarClick(bar.log)}
						>
							{bar.widthPct > 3 && (
								<>
									<RenderProviderIcon provider={bar.log.provider as ProviderIconType} size="xs" className="mr-0.5 h-3 w-3" />
									<span className="truncate font-mono text-[11px] whitespace-nowrap text-white/90">{truncateModel(bar.log.model)}</span>
								</>
							)}
							<span data-testid={`timeline-bar-${bar.log.id}`} className="sr-only">
								bar-{bar.log.id}
							</span>
						</div>
					))}

					{/* Empty state */}
					{bars.length === 0 && (
						<div className="text-muted-foreground flex h-full items-center justify-center text-sm">{t("timeline.page.empty")}</div>
					)}
				</div>

				{/* NOW line — full canvas height */}
				<div className="pointer-events-none absolute top-0 bottom-0 z-10" style={{ left: `${nowLineX}%` }}>
					<div className="h-full w-px -translate-x-1/2 bg-red-500 shadow-[0_0_8px_rgba(239,68,68,0.6)]" />
					<div className="absolute top-[2px] -translate-x-1/2 rounded-b-sm bg-red-500 px-1.5 py-0.5 font-mono text-[8px] font-bold tracking-wider text-white">
						NOW
					</div>
				</div>
			</div>

			{/* Tooltip */}
			{tooltipLog &&
				(() => {
					const lastUserMessage = getMessage(tooltipLog);
					return (
						<div
							data-testid="timeline-tooltip"
							className="bg-popover pointer-events-none absolute z-20 max-w-[360px] rounded-md border px-3 py-2 text-xs shadow-md"
							style={{
								left: tooltipPos.x,
								top: tooltipPos.y,
								transform: tooltipAbove ? "translate(-50%, -100%)" : "translate(-50%, 0)",
							}}
						>
							<div className="flex items-center gap-2 font-medium">
								<span className="uppercase">{tooltipLog.provider}</span>
								<span className="text-muted-foreground">{tooltipLog.model}</span>
								{tooltipLog.status === "processing" && (
									<span className="rounded-sm bg-blue-100 px-1.5 py-0.5 font-mono text-[9px] text-blue-700 dark:bg-blue-950 dark:text-blue-300">
										RUNNING
									</span>
								)}
							</div>
							<div className="text-muted-foreground mt-1 flex gap-3">
								<span>
									{tooltipLog.status === "processing"
										? `~${Math.round((nowMs - new Date(tooltipLog.timestamp).getTime()) / 100) / 10}s elapsed`
										: `${(tooltipLog.latency ?? 0).toLocaleString()}ms`}
								</span>
								<span>{tooltipLog.cost != null ? formatCost(tooltipLog.cost) : "—"}</span>
							</div>
							{tooltipLog.token_usage && (
								<div className="text-muted-foreground mt-1 flex gap-3">
									<span>Input: {tooltipLog.token_usage.prompt_tokens.toLocaleString()}</span>
									<span>Output: {tooltipLog.token_usage.completion_tokens.toLocaleString()}</span>
									{tooltipLog.status !== "processing" &&
										tooltipLog.latency != null &&
										tooltipLog.latency > 0 &&
										(() => {
											const tps = tooltipLog.token_usage.completion_tokens / (tooltipLog.latency / 1000);
											const cls =
												tps < 20
													? "text-red-500 dark:text-red-400"
													: tps < 50
														? "text-amber-500 dark:text-amber-400"
														: tps < 80
															? "text-blue-500 dark:text-blue-400"
															: "text-green-600 dark:text-green-400";
											return (
												<span>
													TPS: <strong className={cls}>{tps.toFixed(1)}</strong>/s
												</span>
											);
										})()}
								</div>
							)}
							{lastUserMessage ? (
								<div
									data-testid="timeline-tooltip-last-user-message"
									className="text-foreground/90 mt-1 overflow-hidden text-ellipsis whitespace-nowrap"
									title={lastUserMessage}
								>
									{lastUserMessage}
								</div>
							) : null}
						</div>
					);
				})()}
		</div>
	);
}