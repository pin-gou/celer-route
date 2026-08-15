/**
 * @file SSE hook for logs timeline
 *
 * Subscribes to GET /api/logs/active/stream for real-time active request updates.
 * - On first connection, receives an "active_logs" event (full handshake).
 * - Subsequent updates arrive as "log_updated" events (incremental merge).
 * - The hook merges incoming events into the local timeline state.
 * - EventSource is closed on unmount.
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

export interface UseLogsTimelineSSEResult {
	activeLogs: ActiveLogEntry[];
	error: string | null;
	isConnected: boolean;
}

/**
 * Build an ActiveLogEntry from a LogEntry or ActiveLogStreamEvent
 */
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

/**
 * SSE hook that subscribes to GET /api/logs/active/stream.
 * Returns activeLogs array, error state, and connection status.
 */
export function useLogsTimelineSSE(): UseLogsTimelineSSEResult {
	const [activeLogs, setActiveLogs] = useState<ActiveLogEntry[]>([]);
	const [error, setError] = useState<string | null>(null);
	const [isConnected, setIsConnected] = useState(false);
	const eventSourceRef = useRef<EventSource | null>(null);

	const handleActiveLogs = useCallback((data: unknown) => {
		setError(null);
		try {
			const logs = data as LogEntry[];
			setActiveLogs(logs.map(toActiveEntry));
		} catch {
			// Silently ignore malformed data
		}
	}, []);

	const handleLogUpdated = useCallback((data: unknown) => {
		try {
			const update = data as ActiveLogStreamEvent;
			setActiveLogs((prev) => {
				const idx = prev.findIndex((l) => l.id === update.id);
				if (idx >= 0) {
					// If the log transitioned from processing to a terminal state,
					// remove it from the active list
					if (update.previous_status === "processing" && (update.status === "success" || update.status === "error")) {
						const next = [...prev];
						next.splice(idx, 1);
						return next;
					}
					// Otherwise, update in place
					const next = [...prev];
					next[idx] = {
						...next[idx],
						status: update.status,
						latency: update.latency_ms ?? next[idx].latency,
					};
					return next;
				}
				// New log not in active list — add it
				return [
					...prev,
					{
						id: update.id,
						status: update.status,
						provider: update.provider,
						model: update.model,
						latency: update.latency_ms,
					},
				];
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
	}, [handleActiveLogs, handleLogUpdated]);

	return { activeLogs, error, isConnected };
}