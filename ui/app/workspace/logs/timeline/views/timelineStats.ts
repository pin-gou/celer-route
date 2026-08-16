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
}

export function summarizeTimelineStats(logs: LogEntry[], totalRequests: number, activeCount: number): TimelineStatsSummary {
	let success = 0;
	let error = 0;
	let cancelled = 0;
	let latencySum = 0;
	let latencyCount = 0;
	let maxLatency = 0;
	let totalTokens = 0;
	let promptTokens = 0;
	let completionTokens = 0;

	for (const log of logs) {
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
	};
}