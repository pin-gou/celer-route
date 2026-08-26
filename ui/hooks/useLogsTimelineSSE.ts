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
import type { ActiveLogStreamEvent, LLMUsage } from "@/lib/types/logs";
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
	selected_key_name?: string;
	selected_key_id?: string;
	routing_rule_id?: string;
	routing_rule_name?: string;
	routing_decision_count?: number;
	number_of_retries?: number;
	fallback_index?: number;
	content_summary?: string;
	message?: string;
	metadata?: Record<string, unknown>;
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
		selected_key_name: update.selected_key_name,
		selected_key_id: update.selected_key_id,
		routing_rule_id: update.routing_rule_id,
		routing_rule_name: update.routing_rule_name,
		routing_decision_count: update.routing_decision_count,
		number_of_retries: update.number_of_retries ?? 0,
		fallback_index: update.fallback_index ?? 0,
		content_summary: update.content_summary,
		message: update.message,
		metadata: update.metadata,
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

// log_updated state changes are coalesced over this window so a burst of
// progress events costs consumers a single re-render instead of one per event.
// Callbacks (onNewLog/onLogRemoved) and lastSeen bookkeeping still run
// immediately at event time — only the activeLogs array update is deferred.
const LOG_UPDATED_FLUSH_INTERVAL_MS = 500;

interface PendingLogUpdate {
	entry: ActiveLogEntry;
	terminal: boolean;
}

// Merge a fresh log_updated snapshot into an existing active entry. Only
// overwrite a field when the update actually carries it; otherwise keep the
// already-known value (the initial "processing" snapshot is authoritative for
// fields the update omits).
function mergeActiveEntry(existing: ActiveLogEntry, fresh: ActiveLogEntry): ActiveLogEntry {
	return {
		...existing,
		...fresh,
		latency: fresh.latency ?? existing.latency,
		provider: fresh.provider ?? existing.provider,
		model: fresh.model ?? existing.model,
		object: fresh.object ?? existing.object,
		stream: fresh.stream ?? existing.stream,
		token_usage: fresh.token_usage ?? existing.token_usage,
		app: fresh.app ?? existing.app,
		user_agent: fresh.user_agent ?? existing.user_agent,
		cost: fresh.cost ?? existing.cost,
		virtual_key_name: fresh.virtual_key_name ?? existing.virtual_key_name,
		virtual_key_id: fresh.virtual_key_id ?? existing.virtual_key_id,
		selected_key_name: fresh.selected_key_name ?? existing.selected_key_name,
		selected_key_id: fresh.selected_key_id ?? existing.selected_key_id,
		routing_rule_id: fresh.routing_rule_id ?? existing.routing_rule_id,
		routing_rule_name: fresh.routing_rule_name ?? existing.routing_rule_name,
		routing_decision_count: fresh.routing_decision_count ?? existing.routing_decision_count,
		number_of_retries: fresh.number_of_retries ?? existing.number_of_retries,
		fallback_index: fresh.fallback_index ?? existing.fallback_index,
		content_summary: fresh.content_summary ?? existing.content_summary,
		message: fresh.message ?? existing.message,
		metadata: fresh.metadata ?? existing.metadata,
	};
}

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
	// Batched log_updated state changes, keyed by log id (latest event wins).
	// Flushed on a short timer so a burst of events triggers one re-render.
	const pendingUpdatesRef = useRef<Map<string, PendingLogUpdate>>(new Map());
	const flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	const flushPendingLogUpdates = useCallback(() => {
		flushTimerRef.current = null;
		const pending = pendingUpdatesRef.current;
		if (pending.size === 0) return;
		pendingUpdatesRef.current = new Map();
		setActiveLogs((prev) => {
			let next = prev;
			for (const [id, upd] of pending) {
				const idx = next.findIndex((l) => l.id === id);
				if (upd.terminal) {
					if (idx >= 0) {
						if (next === prev) next = [...prev];
						next.splice(idx, 1);
					}
				} else if (idx >= 0) {
					if (next === prev) next = [...prev];
					next[idx] = mergeActiveEntry(next[idx], upd.entry);
				} else {
					if (next === prev) next = [...prev];
					next.push(upd.entry);
				}
			}
			return next;
		});
	}, []);

	const handleActiveLogs = useCallback((data: unknown) => {
		setError(null);
		try {
			// The active_logs handshake carries the same activeLogEntry wire shape as
			// log_updated events (latency_ms, message, content_summary), so reuse
			// toActiveEntryFromEvent — mapping it as LogEntry previously dropped the
			// `message` (last-user-prompt) preview and mis-read latency.
			const logs = data as ActiveLogStreamEvent[];
			const now = Date.now();
			const seen = lastSeenRef.current;
			seen.clear();
			const entries = logs.map((log) => {
				seen.set(log.id, now);
				return toActiveEntryFromEvent(log);
			});
			// A handshake is a full resync — drop any batched incremental
			// updates, which are now stale relative to this snapshot.
			pendingUpdatesRef.current.clear();
			if (flushTimerRef.current) {
				clearTimeout(flushTimerRef.current);
				flushTimerRef.current = null;
			}
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

	const handleLogUpdated = useCallback(
		(data: unknown) => {
			try {
				const update = data as ActiveLogStreamEvent;
				const { onNewLog, onLogRemoved } = optionsRef.current ?? {};
				const seen = lastSeenRef.current;
				const now = Date.now();
				const terminal = isTerminalStatus(update.status);
				const entry = toActiveEntryFromEvent(update);

				// Callbacks and lastSeen bookkeeping run at event time (exactly
				// once per event). Firing them inside a setState updater would
				// double-invoke under StrictMode and defer them until render.
				if (terminal) {
					const exists = activeLogsRef.current.some((l) => l.id === update.id) || pendingUpdatesRef.current.has(update.id);
					if (exists) onLogRemoved?.(update.id);
					onNewLog?.(entry);
					seen.delete(update.id);
				} else {
					seen.set(update.id, now);
				}

				// Buffer the state change; the flush window coalesces a burst of
				// log_updated events into a single setActiveLogs call. Latest
				// event per id wins.
				pendingUpdatesRef.current.set(update.id, { entry, terminal });
				if (!flushTimerRef.current) {
					flushTimerRef.current = setTimeout(flushPendingLogUpdates, LOG_UPDATED_FLUSH_INTERVAL_MS);
				}
			} catch {
				// Silently ignore malformed data
			}
		},
		[flushPendingLogUpdates],
	);

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
			if (flushTimerRef.current) {
				clearTimeout(flushTimerRef.current);
				flushTimerRef.current = null;
			}
			pendingUpdatesRef.current.clear();
			es.close();
			eventSourceRef.current = null;
			setIsConnected(false);
			lastSeenRef.current.clear();
			activeLogsRef.current = [];
		};
	}, [handleActiveLogs, handleRecentLogs, handleLogUpdated]);

	return { activeLogs, error, isConnected };
}