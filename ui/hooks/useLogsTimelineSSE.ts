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
import type { ActiveLogStreamEvent, LogEntry, LLMUsage } from "@/lib/types/logs";
import { getApiBaseUrl } from "@/lib/utils/port";

export interface ActiveLogEntry {
	id: string;
	status: string;
	provider?: string;
	model?: string;
	object?: string;
	stream?: boolean;
	latency?: number | null;
	timestamp?: string;
	token_usage?: LLMUsage | null;
	app?: string;
	user_agent?: string;
	cost?: number | null;
	virtual_key_name?: string;
	virtual_key_id?: string;
	routing_rule_id?: string;
	routing_rule_name?: string;
	number_of_retries?: number;
	fallback_index?: number;
	content_summary?: string;
	message?: string;
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
		object: log.object,
		stream: log.stream,
		latency: log.latency,
		timestamp: log.timestamp,
		token_usage: log.token_usage,
		app: log.app,
		user_agent: log.user_agent,
		cost: log.cost ?? null,
		virtual_key_name: log.virtual_key_name,
		virtual_key_id: log.virtual_key_id,
		routing_rule_id: log.routing_rule_id,
		routing_rule_name: log.routing_rule_name,
		number_of_retries: log.number_of_retries,
		fallback_index: log.fallback_index,
		content_summary: log.content_summary,
		message: undefined,
	};
}

function toActiveEntryFromEvent(update: ActiveLogStreamEvent): ActiveLogEntry {
	return {
		id: update.id,
		status: update.status,
		provider: update.provider,
		model: update.model,
		object: update.object,
		stream: update.stream,
		latency: update.latency_ms ?? null,
		timestamp: update.timestamp,
		token_usage: update.token_usage ?? null,
		app: update.app,
		user_agent: update.user_agent,
		cost: update.cost ?? null,
		virtual_key_name: update.virtual_key_name,
		virtual_key_id: update.virtual_key_id,
		routing_rule_id: update.routing_rule_id,
		routing_rule_name: update.routing_rule_name,
		number_of_retries: update.number_of_retries ?? 0,
		fallback_index: update.fallback_index ?? 0,
		content_summary: update.content_summary,
		message: update.message,
	};
}

// isTerminalStatus reports whether the status represents a settled (non-running)
// state. In addition to success/error, "cancelled" covers requests that were
// abandoned before reaching the provider (e.g. the synthetic 503 no_eligible_keys
// key-pool veto), which the logging backend records as cancelled.
function isTerminalStatus(status: string): boolean {
	return status === "success" || status === "error" || status === "cancelled";
}

// TTL for activeLogs entries: a processing log that hasn't received any
// log_updated event for this long is considered abandoned by the server and
// dropped locally. Without this, a backend that fails to emit the terminal
// event for an in-flight request would leak an entry forever — each subsequent
// SSE update would append a new row, the Logs table would render it, and the
// Timeline stats would keep counting it.
const ACTIVE_LOG_TTL_MS = 10 * 60 * 1000;
const ACTIVE_LOG_SWEEP_INTERVAL_MS = 30 * 1000;

export function useLogsTimelineSSE(options?: UseLogsTimelineSSEOptions): UseLogsTimelineSSEResult {
	const [activeLogs, setActiveLogs] = useState<ActiveLogEntry[]>([]);
	const [error, setError] = useState<string | null>(null);
	const [isConnected, setIsConnected] = useState(false);
	const eventSourceRef = useRef<EventSource | null>(null);
	const optionsRef = useRef(options);
	optionsRef.current = options;
	const lastSeenRef = useRef<Map<string, number>>(new Map());
	// Mirror of activeLogs readable from the sweep timer without relying on
	// setState-updater timing (updaters run during render, not at call time).
	const activeLogsRef = useRef<ActiveLogEntry[]>([]);
	activeLogsRef.current = activeLogs;

	const handleActiveLogs = useCallback((data: unknown) => {
		setError(null);
		try {
			const logs = data as LogEntry[];
			const now = Date.now();
			const seen = lastSeenRef.current;
			seen.clear();
			const entries = logs.map((log) => {
				seen.set(log.id, now);
				return toActiveEntry(log);
			});
			setActiveLogs(entries);
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
			const seen = lastSeenRef.current;
			const now = Date.now();

			setActiveLogs((prev) => {
				const idx = prev.findIndex((l) => l.id === update.id);

				if (idx >= 0) {
					if (isTerminalStatus(update.status)) {
						onLogRemoved?.(update.id);
						onNewLog?.(toActiveEntryFromEvent(update));
						seen.delete(update.id);
						const next = [...prev];
						next.splice(idx, 1);
						return next;
					}
					const next = [...prev];
					const fresh = toActiveEntryFromEvent(update);
					next[idx] = {
						...next[idx],
						...fresh,
						// Only overwrite a field when the update actually carries it;
						// otherwise keep the already-known value (initial "processing"
						// snapshot is authoritative for fields the update omits).
						latency: update.latency_ms ?? next[idx].latency,
						provider: fresh.provider ?? next[idx].provider,
						model: fresh.model ?? next[idx].model,
						object: fresh.object ?? next[idx].object,
						stream: fresh.stream ?? next[idx].stream,
						token_usage: fresh.token_usage ?? next[idx].token_usage,
						app: fresh.app ?? next[idx].app,
						user_agent: fresh.user_agent ?? next[idx].user_agent,
						cost: fresh.cost ?? next[idx].cost,
						virtual_key_name: fresh.virtual_key_name ?? next[idx].virtual_key_name,
						virtual_key_id: fresh.virtual_key_id ?? next[idx].virtual_key_id,
						routing_rule_id: fresh.routing_rule_id ?? next[idx].routing_rule_id,
						routing_rule_name: fresh.routing_rule_name ?? next[idx].routing_rule_name,
						number_of_retries: fresh.number_of_retries ?? next[idx].number_of_retries,
						fallback_index: fresh.fallback_index ?? next[idx].fallback_index,
						content_summary: fresh.content_summary ?? next[idx].content_summary,
						message: fresh.message ?? next[idx].message,
					};
					seen.set(update.id, now);
					return next;
				}

				if (isTerminalStatus(update.status)) {
					onNewLog?.(toActiveEntryFromEvent(update));
					return prev;
				}

				seen.set(update.id, now);
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

		// TTL sweep: drop activeLogs entries the server stopped updating. Without
		// this, a backend that fails to emit the terminal log_updated for an
		// in-flight request leaks an entry forever, growing activeLogs unbounded
		// and bloating every consumer re-render (Logs table rows, Timeline stats).
		const sweepTimer = setInterval(() => {
			const seen = lastSeenRef.current;
			const active = activeLogsRef.current;
			if (active.length === 0) return;
			const cutoff = Date.now() - ACTIVE_LOG_TTL_MS;
			const expiredIds: string[] = [];
			const next: ActiveLogEntry[] = [];
			for (const entry of active) {
				const lastSeen = seen.get(entry.id) ?? 0;
				if (lastSeen < cutoff) {
					expiredIds.push(entry.id);
				} else {
					next.push(entry);
				}
			}
			if (expiredIds.length === 0) return;
			setActiveLogs(next);
			const onLogRemoved = optionsRef.current?.onLogRemoved;
			for (const id of expiredIds) {
				seen.delete(id);
				onLogRemoved?.(id);
			}
		}, ACTIVE_LOG_SWEEP_INTERVAL_MS);

		return () => {
			clearInterval(sweepTimer);
			es.close();
			eventSourceRef.current = null;
			setIsConnected(false);
			lastSeenRef.current.clear();
			activeLogsRef.current = [];
		};
	}, [handleActiveLogs, handleRecentLogs, handleLogUpdated]);

	return { activeLogs, error, isConnected };
}