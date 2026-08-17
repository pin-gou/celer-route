/**
 * @file Pure aggregation helper for timeline stat cards
 *
 * Aggregates the merged log list (≤500 rows, 20-min fetch window) into the
 * summary stats the stat cards display. All values are computed client-side
 * from the LogEntry array — no backend call required.
 */

import type { LogEntry } from "@/lib/types/logs";

export interface TimelineStatsSummary {
	totalRequests: number;
	successRate: number;
	avgLatency: number;
	maxLatency: number;
	totalTokens: number;
	promptTokens: number;
	completionTokens: number;
	errorCount: number;
	cancelledCount: number;
	activeCount: number;
	/** Max number of completed requests concurrently in-flight within the window. */
	maxConcurrency: number;
}

export function summarizeTimelineStats(
	logs: LogEntry[],
	totalRequests: number,
	activeCount: number,
	windowStartMs?: number,
	windowEndMs?: number,
): TimelineStatsSummary {
	let success = 0;
	let error = 0;
	let cancelled = 0;
	let latencySum = 0;
	let latencyCount = 0;
	let maxLatency = 0;
	let totalTokens = 0;
	let promptTokens = 0;
	let completionTokens = 0;

	// Sweep-line events over the in-flight interval [timestamp, timestamp + latency]
	// of each completed request, clipped to the window. At each transition we count
	// how many requests are running simultaneously and track the running maximum.
	// +1 posted at the start, -1 at the end; ties resolve +1 first so a request
	// that starts exactly when another ends counts as overlapping.
	const events: Array<{ ms: number; delta: number }> = [];

	const hasWindow = typeof windowStartMs === "number" && typeof windowEndMs === "number";
	const winStart = hasWindow ? (windowStartMs as number) : -Infinity;
	const winEnd = hasWindow ? (windowEndMs as number) : Infinity;

	for (const log of logs) {
		const isTerminal = log.status === "success" || log.status === "error" || log.status === "cancelled";
		switch (log.status) {
			case "success":
				success++;
				break;
			case "error":
				error++;
				break;
			case "cancelled":
				cancelled++;
				break;
		}

		if (log.status === "success" && typeof log.latency === "number") {
			latencySum += log.latency;
			latencyCount++;
			if (log.latency > maxLatency) maxLatency = log.latency;
		}

		const usage = log.token_usage;
		if (usage) {
			promptTokens += usage.prompt_tokens ?? 0;
			completionTokens += usage.completion_tokens ?? 0;
			totalTokens += usage.total_tokens ?? 0;
		}

		// Concurrency only counts completed (non-processing) requests.
		if (isTerminal) {
			const start = new Date(log.timestamp).getTime();
			const startClipped = Math.max(start, winStart);
			if (startClipped <= winEnd) {
				events.push({ ms: startClipped, delta: 1 });
				const end = start + (typeof log.latency === "number" && log.latency >= 0 ? log.latency : 0);
				const endClipped = Math.min(end, winEnd);
				if (endClipped < startClipped) {
					// Interval lies fully outside the window after clipping — drop both.
					events.pop();
				} else if (endClipped < winEnd) {
					events.push({ ms: endClipped, delta: -1 });
				} else if (typeof windowEndMs === "number") {
					// End beyond window end — emit at the window boundary so the
					// window-view concurrency is measured correctly.
					events.push({ ms: winEnd, delta: -1 });
				}
			}
		}
	}

	events.sort((a, b) => a.ms - b.ms || b.delta - a.delta);
	let maxConcurrency = 0;
	let cur = 0;
	for (const e of events) {
		cur += e.delta;
		if (cur > maxConcurrency) maxConcurrency = cur;
	}

	const terminalCount = success + error + cancelled;
	return {
		totalRequests,
		successRate: terminalCount > 0 ? (success / terminalCount) * 100 : 0,
		avgLatency: latencyCount > 0 ? latencySum / latencyCount : 0,
		maxLatency,
		totalTokens,
		promptTokens,
		completionTokens,
		errorCount: error,
		cancelledCount: cancelled,
		activeCount,
		maxConcurrency,
	};
}