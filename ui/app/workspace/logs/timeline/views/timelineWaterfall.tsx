/**
 * @file Timeline event row + phase group for the per-log timeline list view.
 *
 * Renders events as a plain list grouped by phase. Each row carries:
 *   [time_offset] [duration] [phase chip] [source:plugin] [message] [copy] [level]
 *
 * There is intentionally no per-row track / bar / tick — that style of
 * waterfall was tried and removed because the timeline data is composed of
 * single-point markers (routing-engine decisions, key attempts, hook
 * timestamps) whose `duration_ms` is always 0. Drawing zero-width bars per
 * event read as "all bars are empty"; the cleaner signal is to list the
 * events directly and let the header carry the total duration.
 */

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { Copy } from "lucide-react";
import { useTranslation } from "react-i18next";
import { getPhaseStyle } from "./timelinePhaseColors";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";

export interface TimelineEvent {
	time_ms_offset: number;
	duration_ms: number;
	phase: string;
	source: string;
	message: string;
	level: string;
	plugin_name: string;
	// Per-attempt upstream HTTP metadata (added by the timeline waterfall
	// feature — backend migration timeline_events_v2_provider_meta). Omitted
	// on legacy / non-upstream rows.
	provider?: string;
	model?: string;
	key_id?: string;
	status?: string;
	// Client-side correlation key: array index into the aggregated events list
	// (data.events). The backend emits no stable id per timeline event, so the
	// merged waterfall+list view links a bar/marker to its list row by this
	// index. Never sent to the API.
	_tlKey?: number;
}

function getLevelRowClass(level: string): string {
	switch (level) {
		case "warn":
			return "border-l-2 border-l-yellow-500";
		case "error":
			return "border-l-2 border-l-red-500";
		default:
			return "border-l-2 border-l-transparent";
	}
}

function getLevelBadge(level: string, t: (key: string) => string) {
	switch (level) {
		case "warn":
			return (
				<Badge
					variant="outline"
					className="border-yellow-300 bg-yellow-50 text-yellow-700 dark:border-yellow-600 dark:bg-yellow-950 dark:text-yellow-300"
					data-testid="timeline-event-badge-warn"
				>
					{t("timeline.detail.warn")}
				</Badge>
			);
		case "error":
			return (
				<Badge
					variant="outline"
					className="border-red-300 bg-red-50 text-red-700 dark:border-red-600 dark:bg-red-950 dark:text-red-300"
					data-testid="timeline-event-badge-error"
				>
					{t("timeline.detail.error")}
				</Badge>
			);
		default:
			return (
				<Badge
					variant="outline"
					className="border-gray-300 bg-gray-50 text-gray-600 dark:border-gray-600 dark:bg-gray-900 dark:text-gray-400"
					data-testid="timeline-event-badge-info"
				>
					{t("timeline.detail.info")}
				</Badge>
			);
	}
}

function formatMs(ms: number, decimals: number = 1): string {
	return ms.toLocaleString("en-US", {
		minimumFractionDigits: decimals,
		maximumFractionDigits: decimals,
		useGrouping: false,
	});
}

export interface TimelineWaterfallRowProps {
	event: TimelineEvent;
	/** True when the coordinating waterfall bar/marker for this event is hovered. */
	active?: boolean;
	/** Fired on mouse enter — uses event._tlKey to cross-highlight the other view. */
	onHover?: (key: number) => void;
	/** Fired on mouse leave — clears the cross-view highlight. */
	onHoverEnd?: () => void;
}

export function TimelineWaterfallRow({ event, active = false, onHover, onHoverEnd }: TimelineWaterfallRowProps) {
	const { t } = useTranslation("logs");
	const phaseStyle = getPhaseStyle(event.phase, t);
	const { copy } = useCopyToClipboard({ successMessage: t("timeline.detail.copied") });

	const hasDuration = event.duration_ms > 0;
	const testIdLevel = event.level === "warn" ? "warn" : event.level === "error" ? "error" : "info";

	return (
		<div
			data-testid={`timeline-event-row-${testIdLevel}`}
			data-phase={event.phase}
			data-active={active || undefined}
			onMouseEnter={event._tlKey != null ? () => onHover?.(event._tlKey as number) : undefined}
			onMouseLeave={onHoverEnd}
			className={cn(
				"hover:bg-muted/30 group flex cursor-default items-center gap-3 rounded-sm px-3 py-1.5 text-xs transition-colors",
				getLevelRowClass(event.level),
				active && "bg-accent/70 ring-1 ring-inset ring-accent-foreground/50 dark:bg-accent/60",
			)}
		>
			{/* Time offset */}
			<div className="text-muted-foreground w-16 shrink-0 text-right font-mono tabular-nums" data-testid="timeline-event-offset">
				{formatMs(event.time_ms_offset)}
			</div>

			{/* Duration — em dash when not measurable */}
			<div className="text-muted-foreground w-16 shrink-0 text-right font-mono tabular-nums" data-testid="timeline-event-duration">
				{hasDuration ? formatMs(event.duration_ms) : "—"}
			</div>

			{/* Phase + source/plugin chip */}
			<div className="flex w-44 shrink-0 items-center gap-1.5">
				<span
					className={cn(
						"inline-flex items-center rounded-sm px-1.5 py-0.5 text-[10.5px] font-medium tracking-wide uppercase",
						phaseStyle.chip,
					)}
					data-testid="timeline-event-phase"
				>
					{phaseStyle.label}
				</span>
				{event.source && (
					<span className="text-muted-foreground truncate font-mono text-[10.5px]" data-testid="timeline-event-source">
						{event.source}
						{event.plugin_name ? `:${event.plugin_name}` : ""}
					</span>
				)}
			</div>

			{/* Message — full text, no truncate; wraps on whitespace */}
			<div className="text-foreground/90 min-w-0 flex-1 break-words whitespace-pre-wrap" data-testid="timeline-event-message">
				{event.message}
			</div>

			{/* Copy + level badge */}
			<div className="flex shrink-0 items-center gap-2">
				<button
					type="button"
					onClick={() => copy(event.message)}
					className="text-muted-foreground hover:text-foreground hover:border-border invisible inline-flex h-5 w-5 items-center justify-center rounded-sm border border-transparent transition group-hover:visible"
					aria-label={t("timeline.detail.copyMessage")}
					data-testid="timeline-event-copy"
				>
					<Copy className="h-3 w-3" />
				</button>
				{getLevelBadge(event.level, t)}
			</div>
		</div>
	);
}

export interface TimelineWaterfallGroupProps {
	phase: string;
	events: TimelineEvent[];
	t: (key: string) => string;
	/** Currently hovered _tlKey; rows with this key render as active. */
	activeKey?: number | null;
	/** Fired on row mouse enter (event._tlKey). */
	onHover?: (key: number) => void;
	/** Fired on row mouse leave. */
	onHoverEnd?: () => void;
}

export function TimelineWaterfallGroup({ phase, events, t, activeKey, onHover, onHoverEnd }: TimelineWaterfallGroupProps) {
	const phaseStyle = getPhaseStyle(phase, t);
	return (
		<section className="space-y-1" data-testid="timeline-phase-group" data-phase={phase}>
			<header className="text-muted-foreground bg-background/95 sticky top-0 z-10 flex items-center gap-2 px-3 py-1 text-[10.5px] font-medium tracking-wide uppercase backdrop-blur">
				<span className={cn("inline-flex items-center rounded-sm px-1.5 py-0.5 text-[10px]", phaseStyle.chip)}>{phaseStyle.label}</span>
				<span className="text-muted-foreground/70 font-mono normal-case">
					{events.length} {events.length === 1 ? t("timeline.detail.eventCount.one") : t("timeline.detail.eventCount.other")}
				</span>
			</header>
			<div className="space-y-0.5">
				{events.map((event, idx) => (
					<TimelineWaterfallRow
						key={event._tlKey ?? idx}
						event={event}
						active={event._tlKey != null && activeKey === event._tlKey}
						onHover={onHover}
						onHoverEnd={onHoverEnd}
					/>
				))}
			</div>
		</section>
	);
}

export type { PhaseStyle } from "./timelinePhaseColors";