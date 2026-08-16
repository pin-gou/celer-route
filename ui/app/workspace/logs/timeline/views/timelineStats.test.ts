/**
 * @file Unit tests for summarizeTimelineStats
 */

import { describe, it, expect } from "vitest";
import { summarizeTimelineStats } from "./timelineStats";
import type { LogEntry } from "@/lib/types/logs";

const baseLog = (overrides: Partial<LogEntry>): LogEntry => ({
	id: "test",
	object: "chat.completion",
	parent_request_id: "",
	timestamp: "2026-08-15T10:00:00Z",
	provider: "openai",
	model: "gpt-4",
	status: "success",
	latency: 1000,
	stream: false,
	number_of_retries: 0,
	fallback_index: 0,
	cost: 0.002,
	token_usage: { prompt_tokens: 100, completion_tokens: 50, total_tokens: 150 },
	input_history: [],
	responses_input_history: [],
	created_at: "2026-08-15T10:00:00Z",
	...overrides,
});

describe("summarizeTimelineStats", () => {
	it("handles an empty log list", () => {
		const result = summarizeTimelineStats([], 0, 0);
		expect(result).toEqual({
			totalRequests: 0,
			successRate: 0,
			avgLatency: 0,
			maxLatency: 0,
			totalTokens: 0,
			promptTokens: 0,
			completionTokens: 0,
			errorCount: 0,
			cancelledCount: 0,
			activeCount: 0,
		});
	});

	it("computes success rate with mixed statuses", () => {
		const logs = [
			baseLog({ id: "1", status: "success" }),
			baseLog({ id: "2", status: "success" }),
			baseLog({ id: "3", status: "error" }),
			baseLog({ id: "4", status: "cancelled" }),
		];
		const result = summarizeTimelineStats(logs, 10, 0);
		expect(result.totalRequests).toBe(10);
		expect(result.successRate).toBe(50);
		expect(result.errorCount).toBe(1);
		expect(result.cancelledCount).toBe(1);
	});

	it("computes average latency only from successful requests", () => {
		const logs = [
			baseLog({ id: "1", status: "success", latency: 500 }),
			baseLog({ id: "2", status: "success", latency: 1500 }),
			baseLog({ id: "3", status: "error", latency: 200 }),
			baseLog({ id: "4", status: "processing", latency: null as unknown as number }),
		];
		const result = summarizeTimelineStats(logs, 0, 2);
		expect(result.avgLatency).toBe(1000);
		expect(result.maxLatency).toBe(1500);
		expect(result.activeCount).toBe(2);
	});

	it("sums token usage across all logs", () => {
		const logs = [
			baseLog({ id: "1", token_usage: { prompt_tokens: 10, completion_tokens: 20, total_tokens: 30 } }),
			baseLog({ id: "2", token_usage: { prompt_tokens: 100, completion_tokens: 200, total_tokens: 300 } }),
			baseLog({ id: "3", token_usage: undefined }),
		];
		const result = summarizeTimelineStats(logs, 0, 0);
		expect(result.promptTokens).toBe(110);
		expect(result.completionTokens).toBe(220);
		expect(result.totalTokens).toBe(330);
	});

	it("handles logs without token_usage", () => {
		const logs = [baseLog({ id: "1", token_usage: undefined })];
		const result = summarizeTimelineStats(logs, 0, 0);
		expect(result.totalTokens).toBe(0);
		expect(result.promptTokens).toBe(0);
		expect(result.completionTokens).toBe(0);
	});
});