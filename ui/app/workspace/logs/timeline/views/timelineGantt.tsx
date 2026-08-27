/**
 * @file Timeline Gantt waterfall view — the "where did the time go" rendering
 * of the per-log timeline.
 *
 * Unlike the list view (TimelineWaterfallGroup), this component lays events
 * on a shared time axis proportional to `total_duration_ms`:
 *
 *   - Events with `duration_ms > 0` (upstream_call spans) render as a colored
 *     horizontal bar from `time_ms_offset` to `time_ms_offset + duration_ms`.
 *     Color follows `status`: success → blue, failed → red, warn → yellow,
 *     otherwise slate. These are the *measured* spans recorded by
 *     core/upstreamspan.go — not inferred gaps.
 *   - Events with `duration_ms === 0` (routing-engine decisions, key
 *     attempts, pre/post-llm hooks) render as a zero-width triangle marker at
 *     `time_ms_offset`. They never render as a bar; a zero-width bar reads as
 *     "empty" and teaches users to distrust the chart.
 *   - A fixed time ruler on top ticks 0 / 25 / 50 / 75 / 100% of total.
 *
 * Rows are grouped by lane: one per phase, in the canonical phase order from
 * timelinePhaseColors. Multiple events in the same phase share the lane and
 * overlap if their offsets collide — acceptable because upstream_call spans
 * for distinct attempts never overlap in time, and decision markers overlap
 * only when they're deliberately colocated at a decision point.
 *
 * Legacy data (all markers, no spans) renders as: ruler + markers + a muted
 * empty-track note — never an all-zero bars strip.
 */

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { getPhaseStyle } from "./timelinePhaseColors";
import type { TimelineEvent } from "./timelineWaterfall";

export interface TimelineGanttProps {
	events: TimelineEvent[];
	totalDurationMs: number;
	t: (key: string) => string;
	/** Currently hovered _tlKey; bars/markers with this key render as active. */
	activeKey?: number | null;
	/** Fired on bar/marker mouse enter (event._tlKey). */
	onHover?: (key: number) => void;
	/** Fired on bar/marker mouse leave. */
	onHoverEnd?: () => void;
}

// Lane height + bar height in px. Bars inset by 3px top/bottom so a lane
// with both a bar and a marker stays legible.
const LANE_H = 34;
const BAR_H = 22;

function colorFor(event: TimelineEvent): string {
	if (event.status === "failed") {
		return "bg-red-500/80 dark:bg-red-500/70";
	}
	if (event.status === "success") {
		return "bg-blue-500/80 dark:bg-blue-500/80";
	}
	if (event.level === "warn") {
		return "bg-amber-500/80 dark:bg-amber-500/70";
	}
	if (event.level === "error") {
		return "bg-red-500/70 dark:bg-red-500/60";
	}
	return "bg-slate-400/70 dark:bg-slate-500/70";
}

function pct(ms: number, total: number): number {
	if (total <= 0) return 0;
	const raw = Math.max(0, Math.min(100, (ms / total) * 100));
	// Round to 4 decimals — avoids float noise like 55.00000000000001% in the
	// inline style and keeps bar edges pixel-stable across renders.
	return Math.round(raw * 10000) / 10000;
}

export function TimelineGantt({ events, totalDurationMs, t, activeKey, onHover, onHoverEnd }: TimelineGanttProps) {
	// Rewrite the ruler labels in human terms: 0 → total_duration_ms.
	const total = totalDurationMs > 0 ? totalDurationMs : 1;
	const ticks = [0, 25, 50, 75, 100].map((p) => ({ pct: p, ms: (total * p) / 100 }));

	// Lane = phase, ordered by the canonical phase order for vertical layout.
	const groupByPhase = new Map<string, TimelineEvent[]>();
	for (const ev of events) {
		const list = groupByPhase.get(ev.phase) ?? [];
		list.push(ev);
		groupByPhase.set(ev.phase, list);
	}
	const lanes = Array.from(groupByPhase.entries()).sort((a, b) => getPhaseStyle(a[0], t).order - getPhaseStyle(b[0], t).order);

	const hasAnySpan = events.some((e) => (e.duration_ms ?? 0) > 0);

	return (
		<div className="space-y-2" data-testid="timeline-gantt">
			{/* Ruler */}
			<div className="border-border relative h-5 border-b">
				{ticks.map((tick) => (
					<div
						key={tick.pct}
						className="absolute top-0 flex h-full -translate-x-1/2 flex-col justify-between"
						style={{ left: `${tick.pct}%` }}
					>
						<span className="bg-foreground/20 h-px w-px" />
						<span className="text-muted-foreground text-[10px] leading-none">{tick.pct === 0 ? "0" : `${formatMs(tick.ms)}`}</span>
					</div>
				))}
				<div className="text-muted-foreground absolute right-0 -bottom-4 text-[10px]">{formatMs(totalDurationMs)}</div>
			</div>

			{/* Track */}
			{lanes.length === 0 ? (
				<div className="text-muted-foreground py-6 text-center text-xs" data-testid="timeline-gantt-empty">
					{t("timeline.detail.gantt.empty")}
				</div>
			) : (
				<div className="space-y-0.5">
					{lanes.map(([phase, evs]) => (
						<GanttLane
							key={phase}
							phase={phase}
							events={evs}
							totalMs={total}
							hasAnySpan={hasAnySpan}
							t={t}
							activeKey={activeKey}
							onHover={onHover}
							onHoverEnd={onHoverEnd}
						/>
					))}
				</div>
			)}
		</div>
	);
}

function GanttLane({
	phase,
	events,
	totalMs,
	hasAnySpan,
	t,
	activeKey,
	onHover,
	onHoverEnd,
}: {
	phase: string;
	events: TimelineEvent[];
	totalMs: number;
	hasAnySpan: boolean;
	t: (key: string) => string;
	activeKey?: number | null;
	onHover?: (key: number) => void;
	onHoverEnd?: () => void;
}) {
	const style = getPhaseStyle(phase, t);
	const sorted = [...events].sort((a, b) => a.time_ms_offset - b.time_ms_offset);

	return (
		<div className="flex items-center gap-2" data-testid="timeline-gantt-lane">
			<div className="text-muted-foreground w-28 shrink-0 pr-2 text-right text-[11px]" title={style.label}>
				<span className={cn("inline-block rounded px-1.5 py-0.5", style.chip)}>{style.label}</span>
			</div>
			<div className="relative h-px flex-1" style={{ height: LANE_H }}>
				{/* track line (only meaningful when spans exist) */}
				{!hasAnySpan && <div className="bg-border/40 absolute inset-x-0 top-1/2 h-px" data-testid="timeline-marker-empty-track" />}
				{sorted.map((ev, i) =>
					(ev.duration_ms ?? 0) > 0 ? (
						<GanttBar
							key={ev._tlKey ?? `${ev.message}-${i}`}
							ev={ev}
							totalMs={totalMs}
							t={t}
							active={ev._tlKey != null && activeKey === ev._tlKey}
							onHover={onHover}
							onHoverEnd={onHoverEnd}
						/>
					) : (
						<GanttMarker
							key={ev._tlKey ?? `${ev.message}-${i}`}
							ev={ev}
							totalMs={totalMs}
							t={t}
							active={ev._tlKey != null && activeKey === ev._tlKey}
							onHover={onHover}
							onHoverEnd={onHoverEnd}
						/>
					),
				)}
			</div>
		</div>
	);
}

function GanttBar({
	ev,
	totalMs,
	t,
	active,
	onHover,
	onHoverEnd,
}: {
	ev: TimelineEvent;
	totalMs: number;
	t: (k: string) => string;
	active?: boolean;
	onHover?: (key: number) => void;
	onHoverEnd?: () => void;
}) {
	const left = pct(ev.time_ms_offset ?? 0, totalMs);
	const width = pct(ev.duration_ms ?? 0, totalMs);
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<div
					className={cn(
						"absolute z-10 cursor-default rounded-sm transition-shadow",
						colorFor(ev),
						active && "z-20 ring-2 ring-foreground/70 ring-offset-1 ring-offset-background",
					)}
					style={{ left: `${left}%`, top: (LANE_H - BAR_H) / 2, width: `${Math.max(width, active ? 1.2 : 0.4)}%`, height: BAR_H }}
					data-testid="timeline-gantt-bar"
					data-active={active || undefined}
					onMouseEnter={ev._tlKey != null ? () => onHover?.(ev._tlKey as number) : undefined}
					onMouseLeave={onHoverEnd}
				/>
			</TooltipTrigger>
			<TooltipContent side="top">
				<GanttTooltip ev={ev} t={t} />
			</TooltipContent>
		</Tooltip>
	);
}

function GanttMarker({
	ev,
	totalMs,
	t,
	active,
	onHover,
	onHoverEnd,
}: {
	ev: TimelineEvent;
	totalMs: number;
	t: (k: string) => string;
	active?: boolean;
	onHover?: (key: number) => void;
	onHoverEnd?: () => void;
}) {
	const left = pct(ev.time_ms_offset ?? 0, totalMs);
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<div
					className={cn("absolute z-10 -translate-x-1/2 cursor-default transition-transform", active && "z-20 scale-125")}
					style={{ left: `${left}%`, top: 4 }}
					data-testid="timeline-gantt-marker"
					data-active={active || undefined}
					onMouseEnter={ev._tlKey != null ? () => onHover?.(ev._tlKey as number) : undefined}
					onMouseLeave={onHoverEnd}
				>
					<svg width="10" height="12" viewBox="0 0 10 12" aria-hidden="true">
						<path d="M5 0 L10 12 L0 12 Z" fill="currentColor" className={cn(active ? "text-foreground" : "text-foreground/60")} />
					</svg>
				</div>
			</TooltipTrigger>
			<TooltipContent side="top">
				<GanttTooltip ev={ev} t={t} />
			</TooltipContent>
		</Tooltip>
	);
}

function GanttTooltip({ ev, t }: { ev: TimelineEvent; t: (k: string) => string }) {
	const lines: Array<[string, string]> = [];
	if (ev.provider) lines.push([t("timeline.detail.gantt.provider"), ev.provider]);
	if (ev.model) lines.push([t("timeline.detail.gantt.model"), ev.model]);
	if (ev.key_id) lines.push([t("timeline.detail.gantt.key"), ev.key_id]);
	if (ev.status) lines.push([t("timeline.detail.gantt.status"), ev.status]);
	if (ev.duration_ms > 0) lines.push([t("timeline.detail.gantt.duration"), `${ev.duration_ms.toFixed(1)}ms`]);
	lines.push([t("timeline.detail.gantt.offset"), `${(ev.time_ms_offset ?? 0).toFixed(1)}ms`]);
	lines.push([t("timeline.detail.gantt.source"), ev.source]);

	return (
		<div className="max-w-xs space-y-0.5 text-xs" data-testid="timeline-gantt-tooltip">
			<p className="line-clamp-2 font-medium break-words">{ev.message}</p>
			{lines.map(([k, v]) => (
				<div key={k} className="flex justify-between gap-3">
					<span className="text-muted-foreground">{k}</span>
					<span className="truncate">{v}</span>
				</div>
			))}
		</div>
	);
}

function formatMs(ms: number): string {
	if (ms < 1000) return `${ms.toFixed(0)}ms`;
	return `${(ms / 1000).toFixed(1)}s`;
}