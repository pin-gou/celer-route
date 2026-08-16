/**
 * @file Timeline detail panel — renders events from GET /api/logs/{id}/timeline
 *
 * Displays a sorted list of timeline events with:
 * - Time offset from request start
 * - Phase name
 * - Source
 * - Human-readable message
 * - Duration
 * - Level badge (info/warn/error)
 */

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface TimelineEvent {
	time_ms_offset: number;
	duration_ms: number;
	phase: string;
	source: string;
	message: string;
	level: string;
	plugin_name: string;
}

export interface TimelineResponse {
	log_id: string;
	total_duration_ms: number;
	events: TimelineEvent[];
}

export interface TimelineDetailProps {
	data: TimelineResponse | null;
	isLoading?: boolean;
	error?: string | null;
}

// ---------------------------------------------------------------------------
// Level styling
// ---------------------------------------------------------------------------

function getLevelRowClass(level: string): string {
	switch (level) {
		case "warn":
			return "border-l-2 border-l-yellow-500 bg-yellow-50/30 dark:bg-yellow-950/20";
		case "error":
			return "border-l-2 border-l-red-500 bg-red-50/30 dark:bg-red-950/20";
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
				>
					{t("timeline.detail.warn")}
				</Badge>
			);
		case "error":
			return (
				<Badge variant="outline" className="border-red-300 bg-red-50 text-red-700 dark:border-red-600 dark:bg-red-950 dark:text-red-300">
					{t("timeline.detail.error")}
				</Badge>
			);
		default:
			return (
				<Badge
					variant="outline"
					className="border-gray-300 bg-gray-50 text-gray-600 dark:border-gray-600 dark:bg-gray-900 dark:text-gray-400"
				>
					{t("timeline.detail.info")}
				</Badge>
			);
	}
}

// ---------------------------------------------------------------------------
// Format helpers
// ---------------------------------------------------------------------------

function formatMs(ms: number, t: (key: string) => string, decimals: number = 1): string {
	return `${ms.toLocaleString("en-US", { minimumFractionDigits: decimals, maximumFractionDigits: decimals, useGrouping: false })} ${t("timeline.detail.ms")}`;
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function TimelineDetail({ data, isLoading, error }: TimelineDetailProps) {
	const { t } = useTranslation("logs");

	if (isLoading) {
		return (
			<div data-testid="timeline-loading" className="flex items-center justify-center py-12">
				<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
			</div>
		);
	}

	if (error) {
		return (
			<div className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-400">
				{error}
			</div>
		);
	}

	if (!data) {
		return (
			<div data-testid="timeline-loading" className="flex items-center justify-center py-12">
				<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
			</div>
		);
	}

	const { log_id, total_duration_ms } = data;
	const events = data.events ?? [];

	return (
		<div className="space-y-3" data-testid="timeline-detail">
			{/* Header */}
			<div className="bg-muted/30 flex items-center justify-between rounded-sm border px-4 py-2">
				<div className="flex items-center gap-2 text-sm">
					<span className="text-muted-foreground">{t("timeline.detail.logId")}</span>
					<code className="font-mono text-xs">{log_id}</code>
				</div>
				<div className="text-sm font-medium tabular-nums">
					{t("timeline.detail.total")} {total_duration_ms.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}{" "}
					ms
				</div>
			</div>

			{/* Events list */}
			{events.length === 0 ? (
				<div className="text-muted-foreground rounded-sm border border-dashed p-5 text-center text-sm">{t("timeline.detail.noEvents")}</div>
			) : (
				<div className="space-y-1">
					{events.map((event, idx) => {
						const testIdLevel = event.level === "warn" ? "warn" : event.level === "error" ? "error" : "info";
						return (
							<div
								key={`${event.phase}-${idx}`}
								data-testid={`timeline-event-row-${testIdLevel}`}
								className={cn("flex items-start gap-3 rounded-sm px-3 py-2 text-xs", getLevelRowClass(event.level))}
							>
								{/* Time offset */}
								<div className="text-muted-foreground w-20 shrink-0 font-mono tabular-nums">{formatMs(event.time_ms_offset, t)}</div>

								{/* Duration */}
								<div className="text-muted-foreground w-20 shrink-0 font-mono tabular-nums">{formatMs(event.duration_ms, t)}</div>

								{/* Phase */}
								<div className="w-24 shrink-0 font-medium">{event.phase}</div>

								{/* Source */}
								<div className="text-muted-foreground w-28 shrink-0">{event.source}</div>
								{event.plugin_name && <div className="text-foreground w-20 shrink-0 font-medium">{event.plugin_name}</div>}

								{/* Message */}
								<div className="min-w-0 flex-1 truncate">{event.message}</div>

								{/* Level badge */}
								<div className="shrink-0">{getLevelBadge(event.level, t)}</div>
							</div>
						);
					})}
				</div>
			)}
		</div>
	);
}