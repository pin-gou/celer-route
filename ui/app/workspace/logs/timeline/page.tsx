/**
 * @file Timeline page — Gantt chart view of request logs
 */

import { useGetLogsQuery } from "@/lib/store";
import type { LogEntry } from "@/lib/types/logs";
import { dateUtils } from "@/lib/types/logs";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useCallback, useMemo, useState } from "react";
import { LogsTimeline } from "./views/logsTimeline";
import { TimelineToolbar, type TimelineMode } from "./views/timelineToolbar";
import { TimelineLegend } from "./views/timelineLegend";
import { LogDetailSheet } from "@/app/workspace/logs/sheets/logDetailsSheet";
import { useLogsTimelineSSE } from "@/hooks/useLogsTimelineSSE";
import { useQueryStates, parseAsString } from "nuqs";

export default function TimelinePage() {
	const hasDeleteAccess = useRbac(RbacResource.Logs, RbacOperation.Delete);
	const hasRevealAccess = useRbac(RbacResource.Logs, RbacOperation.Reveal);

	// Time range — default to last 1 hour
	const defaultRange = useMemo(() => dateUtils.getDefaultTimeRange(), []);
	const [urlState, setUrlState] = useQueryStates({
		selected_log: parseAsString.withDefault(""),
	});

	const [timeRange, setTimeRange] = useState<{ start: string; end: string }>({
		start: dateUtils.toISOString(defaultRange.startTime) ?? "",
		end: dateUtils.toISOString(defaultRange.endTime) ?? "",
	});

	const [mode, setMode] = useState<TimelineMode>("follow");
	const [isLive, setIsLive] = useState(false);

	// Fetch logs
	const { data: logsData, isFetching, refetch } = useGetLogsQuery(
		{
			filters: {
				start_time: timeRange.start,
				end_time: timeRange.end,
			} as any,
			pagination: {
				limit: 500,
				offset: 0,
				sort_by: "timestamp",
				order: "asc",
			},
		},
		{ pollingInterval: isLive ? 5000 : 0, skipPollingIfUnfocused: true },
	);

	// SSE for active requests
	const { activeLogs } = useLogsTimelineSSE();

	const logs = logsData?.logs ?? [];

	const selectedLog = useMemo(
		() => (urlState.selected_log ? logs.find((l) => l.id === urlState.selected_log) ?? null : null),
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
	}, []);

	const handleLiveToggle = useCallback(
		(live: boolean) => {
			setIsLive(live);
			if (live) {
				setMode("live");
				refetch();
			}
		},
		[refetch],
	);

	return (
		<div className="dark:bg-card no-padding-parent no-border-parent flex h-[calc(100vh_-_16px)] flex-col gap-3 p-4">
			{/* Header */}
			<div className="flex items-center justify-between">
				<h1 className="text-lg font-semibold" data-testid="timeline-page-title">
					Request Timeline
				</h1>
				<TimelineLegend />
			</div>

			{/* Toolbar */}
			<TimelineToolbar
				mode={mode}
				onModeChange={handleModeChange}
				onRefresh={handleRefresh}
				isLive={isLive}
				onLiveToggle={handleLiveToggle}
			/>

			{/* Active connections indicator */}
			{isLive && activeLogs.length > 0 && (
				<div
					data-testid="timeline-active-count"
					className="flex items-center gap-2 rounded-sm border border-blue-200 bg-blue-50 px-3 py-1.5 text-xs text-blue-700 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300"
				>
					<span className="relative flex h-2 w-2">
						<span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-blue-400 opacity-75" />
						<span className="relative inline-flex h-2 w-2 rounded-full bg-blue-500" />
					</span>
					{activeLogs.length} active request{activeLogs.length !== 1 ? "s" : ""}
				</div>
			)}

			{/* Loading indicator */}
			{isFetching && (
				<div className="text-muted-foreground text-xs" data-testid="timeline-loading-indicator">
					Refreshing...
				</div>
			)}

			{/* Gantt chart */}
			<div className="min-h-0 flex-1 overflow-auto rounded-sm border bg-card p-3">
				<LogsTimeline
					logs={logs}
					timeRange={timeRange}
					onBarClick={handleBarClick}
					activeLogs={isLive ? activeLogs : undefined}
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
			/>
		</div>
	);
}