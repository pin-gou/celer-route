/**
 * @file Timeline detail panel — renders events from GET /api/logs/{id}/timeline
 *
 * Visual: a compact header summarising the request duration and the
 * last-event span (with a diff tooltip when the two diverge), followed by
 * per-phase waterfall groups. Each row shows a phase-coloured bar positioned
 * by `time_ms_offset`/`duration_ms`, the offset/duration columns, a phase
 * chip, and the full message (no truncate).
 *
 * Loading / empty / error states are explicit and distinguishable.
 */

import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { cn } from "@/lib/utils";
import { AlertCircle, Copy, LayoutGrid, Loader2, RefreshCw, Rows3 } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getPhaseStyle } from "./timelinePhaseColors";
import { TimelineGantt } from "./timelineGantt";
import { TimelineWaterfallGroup, type TimelineEvent } from "./timelineWaterfall";

// Re-export so existing consumers that import TimelineEvent / TimelineResponse
// from this module keep working without changing their import path.
export type { TimelineEvent } from "./timelineWaterfall";

export interface TimelineResponse {
	log_id: string;
	total_duration_ms: number;
	events: TimelineEvent[];
}

export interface TimelineDetailProps {
	data: TimelineResponse | null;
	isLoading?: boolean;
	error?: string | null;
	onRetry?: () => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// PgViewStorageKey is the localStorage key persisting the timeline detail
// view-mode toggle ("list" | "gantt"). Default (missing key) = "gantt" to
// surface the waterfall view for new visitors.
const PgViewStorageKey = "pg-gateway.timeline.view";

function formatMs(ms: number, decimals: number = 2): string {
	return ms.toLocaleString("en-US", {
		minimumFractionDigits: decimals,
		maximumFractionDigits: decimals,
		useGrouping: true,
	});
}

function isNetworkError(error: string | null | undefined): boolean {
	if (!error) return false;
	const lower = error.toLowerCase();
	return lower.includes("network") || lower.includes("fetch") || lower.includes("failed to load");
}

// ---------------------------------------------------------------------------
// Header
// ---------------------------------------------------------------------------

interface HeaderProps {
	logId: string;
	totalDurationMs: number;
	eventSpanMs: number;
}

function TimelineHeader({ logId, totalDurationMs, eventSpanMs }: HeaderProps) {
	const { t } = useTranslation("logs");
	const { copy } = useCopyToClipboard({ successMessage: t("timeline.detail.copied") });

	const diff = eventSpanMs - totalDurationMs;
	const absDiff = Math.abs(diff);
	const diffRatio = totalDurationMs > 0 ? absDiff / totalDurationMs : 0;
	// Show the event-span row whenever it differs from totalDurationMs by more
	// than half a millisecond, regardless of relative size. Tiny post-llm
	// overruns (a few ms over the upstream latency) are exactly the case the
	// diff line exists to explain; using a 5%/50ms threshold was hiding them.
	const showDiff = absDiff > 0.5;
	const diffSign = diff > 0 ? "+" : diff < 0 ? "" : "";
	const diffPct = totalDurationMs > 0 ? (diffRatio * 100).toFixed(1) : "0.0";

	return (
		<div className="bg-muted/30 space-y-1 rounded-sm border px-4 py-2.5 text-sm" data-testid="timeline-header">
			<div className="flex items-center justify-between gap-3">
				<div className="flex min-w-0 items-center gap-2">
					<span className="text-muted-foreground shrink-0">{t("timeline.detail.logId")}</span>
					<code className="truncate font-mono text-xs" data-testid="timeline-header-log-id">
						{logId}
					</code>
					<button
						type="button"
						onClick={() => copy(logId)}
						className="text-muted-foreground hover:text-foreground hover:border-border inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-sm border border-transparent"
						aria-label={t("timeline.detail.copyLogId")}
						data-testid="timeline-header-copy-id"
					>
						<Copy className="h-3 w-3" />
					</button>
				</div>
				<div className="flex shrink-0 items-baseline gap-2 tabular-nums" data-testid="timeline-header-total">
					<span className="text-muted-foreground text-xs">{t("timeline.detail.requestLatency")}</span>
					<span className="font-medium">{formatMs(totalDurationMs)}</span>
					<span className="text-muted-foreground text-xs">ms</span>
				</div>
			</div>
			{showDiff && (
				<div
					className={cn(
						"flex items-center justify-end gap-2 text-xs tabular-nums",
						diff > 0 ? "text-amber-600 dark:text-amber-400" : "text-sky-600 dark:text-sky-400",
					)}
					data-testid="timeline-header-event-span"
				>
					<button type="button" className="inline-flex items-center gap-1.5">
						<span className="text-muted-foreground">{t("timeline.detail.eventSpan")}</span>
						<span className="font-medium">{formatMs(eventSpanMs)}</span>
						<span>ms</span>
						<span className="font-medium" data-testid="timeline-header-diff">
							({diffSign}
							{formatMs(diff)} ms · {diffPct}%)
						</span>
					</button>
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-sm border border-current/40 text-[10px]">
								?
							</span>
						</TooltipTrigger>
						<TooltipContent side="bottom" className="max-w-xs text-left">
							<div className="space-y-1">
								<p>{t("timeline.detail.diffHint.request")}</p>
								<p>{t("timeline.detail.diffHint.span")}</p>
							</div>
						</TooltipContent>
					</Tooltip>
				</div>
			)}
		</div>
	);
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

interface EmptyStateProps {
	totalDurationMs: number;
	t: (key: string) => string;
}

function EmptyState({ totalDurationMs, t }: EmptyStateProps) {
	const isZero = totalDurationMs <= 0;
	return (
		<div className="rounded-sm border border-dashed p-5 text-center text-sm" data-testid="timeline-empty">
			<p className="text-muted-foreground font-medium">
				{isZero ? t("timeline.detail.empty.failedTitle") : t("timeline.detail.empty.title")}
			</p>
			<p className="text-muted-foreground mt-1.5 text-xs">
				{isZero ? t("timeline.detail.empty.failedHint") : t("timeline.detail.empty.hint")}
			</p>
		</div>
	);
}

// ---------------------------------------------------------------------------
// Loading skeleton
// ---------------------------------------------------------------------------

function TimelineSkeleton() {
	return (
		<div className="space-y-3" data-testid="timeline-skeleton">
			<Skeleton className="h-12 w-full" />
			<div className="space-y-2">
				<Skeleton className="h-4 w-1/3" />
				<Skeleton className="h-6 w-full" />
				<Skeleton className="h-6 w-11/12" />
				<Skeleton className="h-6 w-3/4" />
			</div>
			<div className="space-y-2">
				<Skeleton className="h-4 w-1/4" />
				<Skeleton className="h-6 w-full" />
			</div>
		</div>
	);
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function TimelineDetail({ data, isLoading, error, onRetry }: TimelineDetailProps) {
	const { t } = useTranslation("logs");

	// View mode toggle: "list" (existing grouped rows) or "gantt" (waterfall).
	// Persisted in localStorage so a user's choice survives refresh. New
	// visitors default to the gantt view to surface the waterfall feature.
	const [view, setView] = useState<"list" | "gantt">(() => {
		if (typeof window === "undefined") return "gantt";
		const stored = window.localStorage.getItem(PgViewStorageKey);
		return stored === "list" || stored === "gantt" ? stored : "gantt";
	});

	const setViewAndPersist = (next: "list" | "gantt") => {
		setView(next);
		try {
			window.localStorage.setItem(PgViewStorageKey, next);
		} catch {
			// localStorage may be unavailable (privacy mode); ignore.
		}
	};

	const grouped = useMemo(() => {
		if (!data) return [] as Array<{ phase: string; events: TimelineEvent[] }>;
		const events = (data.events ?? []) as TimelineEvent[];
		const buckets = new Map<string, TimelineEvent[]>();
		for (const ev of events) {
			const list = buckets.get(ev.phase) ?? [];
			list.push(ev);
			buckets.set(ev.phase, list);
		}
		return Array.from(buckets.entries())
			.map(([phase, evs]) => ({ phase, events: evs }))
			.sort((a, b) => {
				const ao = getPhaseStyle(a.phase, t).order;
				const bo = getPhaseStyle(b.phase, t).order;
				if (ao !== bo) return ao - bo;
				// within group, preserve ascending offset
				const aMin = a.events[0]?.time_ms_offset ?? 0;
				const bMin = b.events[0]?.time_ms_offset ?? 0;
				return aMin - bMin;
			});
	}, [data, t]);

	if (isLoading) {
		return (
			<div data-testid="timeline-loading" className="space-y-3">
				<TimelineSkeleton />
			</div>
		);
	}

	if (error) {
		return (
			<div
				className="flex items-start gap-3 rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-400"
				role="alert"
				data-testid="timeline-error"
			>
				<AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
				<div className="min-w-0 flex-1 space-y-1">
					<p className="font-medium">
						{isNetworkError(error) ? t("timeline.detail.error.networkTitle") : t("timeline.detail.error.title")}
					</p>
					<p className="text-xs break-words">{error}</p>
				</div>
				{onRetry && (
					<button
						type="button"
						onClick={onRetry}
						className="inline-flex shrink-0 items-center gap-1 rounded-sm border border-current/40 px-2 py-0.5 text-xs hover:bg-red-100/50 dark:hover:bg-red-900/40"
						data-testid="timeline-error-retry"
					>
						<RefreshCw className="h-3 w-3" />
						{t("timeline.detail.error.retry")}
					</button>
				)}
			</div>
		);
	}

	if (!data) {
		// Defer to a small inline loader rather than render the full skeleton —
		// the parent component usually transitions isLoading=true quickly.
		return (
			<div data-testid="timeline-pending" className="flex items-center justify-center py-8">
				<Loader2 className="text-muted-foreground h-5 w-5 animate-spin" />
			</div>
		);
	}

	const events = (data.events ?? []) as TimelineEvent[];
	const hasEvents = events.length > 0;

	// Last event's trailing edge = max(offset + duration) across the trace.
	const eventSpanMs = events.reduce((acc, ev) => Math.max(acc, (ev.time_ms_offset ?? 0) + (ev.duration_ms ?? 0)), 0);

	return (
		<div className="space-y-3" data-testid="timeline-detail">
			<ViewToggle view={view} onChange={setViewAndPersist} t={t} />

			<TimelineHeader logId={data.log_id} totalDurationMs={data.total_duration_ms} eventSpanMs={eventSpanMs} />

			{!hasEvents ? (
				<EmptyState totalDurationMs={data.total_duration_ms} t={t} />
			) : view === "gantt" ? (
				<TimelineGantt events={events} totalDurationMs={data.total_duration_ms} t={t} />
			) : (
				<div className="space-y-3" data-testid="timeline-groups">
					{grouped.map((group) => (
						<TimelineWaterfallGroup key={group.phase} phase={group.phase} events={group.events} t={t} />
					))}
				</div>
			)}
		</div>
	);
}

function ViewToggle({ view, onChange, t }: { view: "list" | "gantt"; onChange: (v: "list" | "gantt") => void; t: (k: string) => string }) {
	return (
		<div className="border-border flex shrink-0 items-center rounded-md border bg-transparent p-0.5" data-testid="timeline-view-toggle">
			<button
				type="button"
				onClick={() => onChange("gantt")}
				title={t("timeline.detail.view.gantt")}
				aria-pressed={view === "gantt"}
				className={cn(
					"flex items-center gap-1 rounded px-2 py-1 text-xs",
					view === "gantt" ? "bg-foreground/10 text-foreground" : "text-muted-foreground hover:text-foreground",
				)}
				data-testid="timeline-view-gantt"
			>
				<LayoutGrid className="h-3.5 w-3.5" />
				{t("timeline.detail.view.gantt")}
			</button>
			<button
				type="button"
				onClick={() => onChange("list")}
				title={t("timeline.detail.view.list")}
				aria-pressed={view === "list"}
				className={cn(
					"flex items-center gap-1 rounded px-2 py-1 text-xs",
					view === "list" ? "bg-foreground/10 text-foreground" : "text-muted-foreground hover:text-foreground",
				)}
				data-testid="timeline-view-list"
			>
				<Rows3 className="h-3.5 w-3.5" />
				{t("timeline.detail.view.list")}
			</button>
		</div>
	);
}