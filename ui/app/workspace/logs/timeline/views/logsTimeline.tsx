/**
 * @file Gantt timeline component for request timeline view
 *
 * Renders a horizontal Gantt chart with request bars, lane allocation,
 * hover tooltips, and click callbacks.
 */

import { formatCost } from "@/app/workspace/dashboard/utils/chartUtils";
import type { LogEntry } from "@/lib/types/logs";
import { cn } from "@/lib/utils";
import { useMemo, useState, useCallback, useRef, type ReactNode } from "react";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface LogsTimelineProps {
	logs: LogEntry[];
	timeRange: { start: string; end: string };
	onBarClick?: (log: LogEntry) => void;
	activeLogs?: Array<{ id: string; status: string; provider?: string; model?: string; latency?: number | null }>;
	className?: string;
}

interface LaneBar {
	log: LogEntry;
	lane: number;
	leftPct: number;
	widthPct: number;
	statusColor: string;
	isProcessing: boolean;
}

// ---------------------------------------------------------------------------
// Status color mapping
// ---------------------------------------------------------------------------

function getStatusColor(status: string): string {
	switch (status) {
		case "success":
			return "bg-green-500";
		case "error":
			return "bg-red-500";
		case "processing":
			return "bg-blue-400";
		default:
			return "bg-gray-400";
	}
}

// ---------------------------------------------------------------------------
// Lane allocation
// ---------------------------------------------------------------------------

function allocateLanes(logs: LogEntry[]): Map<string, number> {
	const laneMap = new Map<string, number>();
	const laneEndTimes: number[] = [];

	// Sort by timestamp ascending
	const sorted = [...logs].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());

	for (const log of sorted) {
		const start = new Date(log.timestamp).getTime();
		const end = start + (log.latency ?? 0);

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

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function LogsTimeline({ logs, timeRange, onBarClick, activeLogs, className }: LogsTimelineProps) {
	const [tooltipLog, setTooltipLog] = useState<LogEntry | null>(null);
	const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 });
	const containerRef = useRef<HTMLDivElement>(null);

	const timeRangeStart = useMemo(() => new Date(timeRange.start).getTime(), [timeRange.start]);
	const timeRangeEnd = useMemo(() => new Date(timeRange.end).getTime(), [timeRange.end]);
	const rangeDuration = timeRangeEnd - timeRangeStart;

	// Merge active logs into the logs array for display
	const mergedLogs = useMemo(() => {
		if (!activeLogs || activeLogs.length === 0) return logs;
		const logMap = new Map(logs.map((l) => [l.id, l]));
		for (const active of activeLogs) {
			if (!logMap.has(active.id)) {
				// Create a synthetic LogEntry for active logs not in the main list
				logMap.set(active.id, {
					id: active.id,
					object: "chat.completion",
					parent_request_id: "",
					timestamp: new Date().toISOString(),
					provider: active.provider ?? "",
					model: active.model ?? "",
					status: active.status,
					latency: active.latency ?? (null as unknown as number),
					stream: false,
					number_of_retries: 0,
					fallback_index: 0,
					cost: 0,
					input_history: [],
					responses_input_history: [],
					created_at: new Date().toISOString(),
				} as LogEntry);
			}
		}
		return Array.from(logMap.values());
	}, [logs, activeLogs]);

	// Allocate lanes
	const laneMap = useMemo(() => allocateLanes(mergedLogs), [mergedLogs]);

	// Compute bar positions
	const bars: LaneBar[] = useMemo(
		() =>
			mergedLogs.map((log) => {
				const logStart = new Date(log.timestamp).getTime();
				const logEnd = logStart + (log.latency ?? 0);
				const leftPct = Math.max(0, ((logStart - timeRangeStart) / rangeDuration) * 100);
				const widthPct = Math.max(0.5, rangeDuration > 0 ? ((log.latency ?? 0) / rangeDuration) * 100 : 0.5);
				const lane = laneMap.get(log.id) ?? 0;
				return {
					log,
					lane,
					leftPct: Math.min(leftPct, 100),
					widthPct: Math.min(widthPct, 100 - leftPct),
					statusColor: getStatusColor(log.status),
					isProcessing: log.status === "processing" || log.status === "processing",
				};
			}),
		[mergedLogs, timeRangeStart, rangeDuration, laneMap],
	);

	const maxLane = useMemo(() => Math.max(0, ...bars.map((b) => b.lane)) + 1, [bars]);

	const laneHeight = 40;
	const totalHeight = maxLane * laneHeight + 20;

	const handleMouseEnter = useCallback((log: LogEntry, event: React.MouseEvent) => {
		setTooltipLog(log);
		const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
		const containerRect = containerRef.current?.getBoundingClientRect();
		setTooltipPos({
			x: rect.left - (containerRect?.left ?? 0) + rect.width / 2,
			y: rect.top - (containerRect?.top ?? 0) - 8,
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

	return (
		<div ref={containerRef} data-testid="logs-timeline" className={cn("relative w-full overflow-x-auto", className)}>
			{/* Time axis header */}
			<div className="bg-background text-muted-foreground sticky top-0 z-10 flex h-6 border-b text-[11px] tabular-nums">
				<span className="pl-1">{timeRange.start}</span>
				<span className="ml-auto pr-1">{timeRange.end}</span>
			</div>

			{/* Gantt area */}
			<div className="relative" style={{ height: totalHeight }} data-testid="timeline-gantt-area">
				{/* Grid lines */}
				{bars.length > 0 && (
					<>
						{/* Vertical grid lines every 25% */}
						{[25, 50, 75].map((pct) => (
							<div
								key={`grid-${pct}`}
								className="absolute top-0 h-full border-l border-dashed border-gray-200 dark:border-gray-700"
								style={{ left: `${pct}%` }}
							/>
						))}
						{/* Horizontal grid lines per lane */}
						{Array.from({ length: maxLane }, (_, i) => (
							<div
								key={`lane-line-${i}`}
								className="absolute right-0 left-0 border-b border-gray-100 dark:border-gray-800"
								style={{ top: `${i * laneHeight + laneHeight}px` }}
							/>
						))}
					</>
				)}

				{/* Bars */}
				{bars.map((bar) => (
					<div
						key={bar.log.id}
						data-testid="timeline-bar"
						data-log-id={bar.log.id}
						data-lane={bar.lane}
						data-bar-width={bar.widthPct.toFixed(2)}
						data-status-color={bar.statusColor}
						className={cn(
							"absolute cursor-pointer rounded-sm transition-opacity hover:opacity-80",
							bar.statusColor,
							bar.isProcessing && "animate-pulse",
						)}
						style={{
							left: `${bar.leftPct}%`,
							width: `${bar.widthPct}%`,
							top: `${bar.lane * laneHeight + 4}px`,
							height: `${laneHeight - 8}px`,
						}}
						onMouseEnter={(e) => handleMouseEnter(bar.log, e)}
						onMouseLeave={handleMouseLeave}
						onClick={() => handleBarClick(bar.log)}
					>
						<span data-testid={`timeline-bar-${bar.log.id}`} className="sr-only">
							bar-{bar.log.id}
						</span>
					</div>
				))}

				{/* Empty state */}
				{bars.length === 0 && (
					<div className="text-muted-foreground flex h-full items-center justify-center text-sm">No requests in this time range.</div>
				)}
			</div>

			{/* Tooltip */}
			{tooltipLog && (
				<div
					data-testid="timeline-tooltip"
					className="bg-popover pointer-events-none absolute z-20 rounded-md border px-3 py-2 text-xs shadow-md"
					style={{
						left: tooltipPos.x,
						top: tooltipPos.y,
						transform: "translate(-50%, -100%)",
					}}
				>
					<div className="flex items-center gap-2 font-medium">
						<span className="uppercase">{tooltipLog.provider}</span>
						<span className="text-muted-foreground">{tooltipLog.model}</span>
					</div>
					<div className="text-muted-foreground mt-1 flex gap-3">
						<span>{(tooltipLog.latency ?? 0).toLocaleString()}ms</span>
						<span>{tooltipLog.cost != null ? formatCost(tooltipLog.cost) : "—"}</span>
					</div>
				</div>
			)}
		</div>
	);
}