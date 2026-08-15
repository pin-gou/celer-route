/**
 * @file SSE hook for logs timeline
 *
 * Subscribes to GET /api/logs/active/stream for real-time log updates.
 * - active_logs: initial snapshot of currently processing logs
 * - recent_logs: recently completed logs (last 30s) for quick reconciliation
 * - log_updated: incremental updates for every log write/transition
 *
 * The hook merges incoming events into local state and fires callbacks so
 * the caller can add completed logs to the main list without polling.
 */

import { useEffect, useRef, useState, useCallback } from "react";
import type { ActiveLogStreamEvent, LogEntry } from "@/lib/types/logs";
import { getApiBaseUrl } from "@/lib/utils/port";

export interface ActiveLogEntry {
	id: string;
	status: string;
	provider?: string;
	model?: string;
	latency?: number | null;
	timestamp?: string;
}

export interface UseLogsTimelineSSEOptions {
	/** Called when a new completed log arrives via SSE (never seen before). */
	onNewLog?: (log: ActiveLogEntry) => void;
	/** Called when a processing log transitions to a terminal state. */
	onLogRemoved?: (id: string) => void;
}

export interface UseLogsTimelineSSEResult {
	activeLogs: ActiveLogEntry[];
	error: string | null;
	isConnected: boolean;
}

function toActiveEntry(log: LogEntry): ActiveLogEntry {
	return {
		id: log.id,
		status: log.status,
		provider: log.provider,
		model: log.model,
		latency: log.latency,
		timestamp: log.timestamp,
	};
}

function toActiveEntryFromEvent(update: ActiveLogStreamEvent): ActiveLogEntry {
	return {
		id: update.id,
		status: update.status,
		provider: update.provider,
		model: update.model,
		latency: update.latency_ms ?? null,
		timestamp: update.timestamp,
	};
}

// isTerminalStatus reports whether the status represents a settled (non-running)
// state. In addition to success/error, "cancelled" covers requests that were
// abandoned before reaching the provider (e.g. the synthetic 503 no_eligible_keys
// key-pool veto), which the logging backend records as cancelled.
function isTerminalStatus(status: string): boolean {
	return status === "success" || status === "error" || status === "cancelled";
}

export function useLogsTimelineSSE(options?: UseLogsTimelineSSEOptions): UseLogsTimelineSSEResult {
	const [activeLogs, setActiveLogs] = useState<ActiveLogEntry[]>([]);
	const [error, setError] = useState<string | null>(null);
	const [isConnected, setIsConnected] = useState(false);
	const eventSourceRef = useRef<EventSource | null>(null);
	const optionsRef = useRef(options);
	optionsRef.current = options;

	const handleActiveLogs = useCallback((data: unknown) => {
		setError(null);
		try {
			const logs = data as LogEntry[];
			setActiveLogs(logs.map(toActiveEntry));
		} catch {
			// Silently ignore malformed data
		}
	}, []);

	const handleRecentLogs = useCallback((data: unknown) => {
		try {
			const entries = data as ActiveLogStreamEvent[];
			const onNewLog = optionsRef.current?.onNewLog;
			if (!onNewLog) return;
			for (const entry of entries) {
				onNewLog(toActiveEntryFromEvent(entry));
			}
		} catch {
			// Silently ignore malformed data
		}
	}, []);

	const handleLogUpdated = useCallback((data: unknown) => {
		try {
			const update = data as ActiveLogStreamEvent;
			const { onNewLog, onLogRemoved } = optionsRef.current ?? {};

			setActiveLogs((prev) => {
				const idx = prev.findIndex((l) => l.id === update.id);

				if (idx >= 0) {
					if (isTerminalStatus(update.status)) {
						onLogRemoved?.(update.id);
						onNewLog?.(toActiveEntryFromEvent(update));
						const next = [...prev];
						next.splice(idx, 1);
						return next;
					}
					const next = [...prev];
					next[idx] = {
						...next[idx],
						status: update.status,
						latency: update.latency_ms ?? next[idx].latency,
					};
					return next;
				}

				if (isTerminalStatus(update.status)) {
					onNewLog?.(toActiveEntryFromEvent(update));
					return prev;
				}

				return [...prev, toActiveEntryFromEvent(update)];
			});
		} catch {
			// Silently ignore malformed data
		}
	}, []);

	useEffect(() => {
		const baseUrl = getApiBaseUrl();
		const url = `${baseUrl}/logs/active/stream`;
		const es = new EventSource(url);
		eventSourceRef.current = es;

		es.addEventListener("active_logs", (event: MessageEvent) => {
			try {
				const data = JSON.parse(event.data);
				handleActiveLogs(data);
			} catch {
				// Silently ignore
			}
		});

		es.addEventListener("recent_logs", (event: MessageEvent) => {
			try {
				const data = JSON.parse(event.data);
				handleRecentLogs(data);
			} catch {
				// Silently ignore
			}
		});

		es.addEventListener("log_updated", (event: MessageEvent) => {
			try {
				const data = JSON.parse(event.data);
				handleLogUpdated(data);
			} catch {
				// Silently ignore
			}
		});

		es.addEventListener("error", () => {
			setIsConnected(false);
			setError("SSE connection error");
		});

		es.onopen = () => {
			setIsConnected(true);
			setError(null);
		};

		return () => {
			es.close();
			eventSourceRef.current = null;
			setIsConnected(false);
		};
	}, [handleActiveLogs, handleRecentLogs, handleLogUpdated]);

	return { activeLogs, error, isConnected };
}