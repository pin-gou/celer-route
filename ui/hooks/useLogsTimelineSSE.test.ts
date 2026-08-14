// @vitest-environment jsdom
/**
 * @file TDD Red Phase — SSE hook for logs timeline
 *
 * These tests verify the SSE hook that subscribes to
 * GET /api/logs/active/stream for real-time active request updates.
 *
 * Behavior:
 * - On first connection, receives an "active_logs" event (full handshake).
 * - Subsequent updates arrive as "log_updated" events (incremental merge).
 * - The hook merges incoming events into the local timeline state.
 *
 * In the TDD red phase, the useLogsTimelineSSE hook does not exist yet,
 * so these tests will fail at compile time (cannot find module).
 * This is the expected result — the dev phase will implement the hook.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// The following import does not exist yet — this is TDD red phase.
// Compilation will fail with "Cannot find module" error.
import { useLogsTimelineSSE } from "./useLogsTimelineSSE";
import type { LogEntry } from "@/lib/types/logs";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const mockActiveLog: LogEntry = {
	id: "active-1",
	object: "chat.completion",
	parent_request_id: "",
	timestamp: "2026-08-15T10:00:00Z",
	provider: "openai",
	model: "gpt-4",
	status: "processing",
	latency: null as unknown as number,
	stream: true,
	number_of_retries: 0,
	fallback_index: 0,
	cost: 0,
	input_history: [],
	responses_input_history: [],
	created_at: "2026-08-15T10:00:00Z",
};

const mockCompletedLog: LogEntry = {
	id: "active-1",
	object: "chat.completion",
	parent_request_id: "",
	timestamp: "2026-08-15T10:00:00Z",
	provider: "openai",
	model: "gpt-4",
	status: "success",
	latency: 1234.56,
	stream: true,
	number_of_retries: 0,
	fallback_index: 0,
	cost: 0.002,
	token_usage: { prompt_tokens: 200, completion_tokens: 100, total_tokens: 300 },
	input_history: [],
	responses_input_history: [],
	created_at: "2026-08-15T10:00:00Z",
};

describe("useLogsTimelineSSE — SSE hook", () => {
	beforeEach(() => {
		// Mock EventSource for SSE subscription
		vi.stubGlobal("EventSource", vi.fn(() => {
			const listeners: Record<string, Set<(...args: unknown[]) => void>> = {};
			return {
				close: vi.fn(),
				addEventListener: vi.fn((event: string, handler: (...args: unknown[]) => void) => {
					if (!listeners[event]) listeners[event] = new Set();
					listeners[event].add(handler);
				}),
				removeEventListener: vi.fn((event: string, handler: (...args: unknown[]) => void) => {
					listeners[event]?.delete(handler);
				}),
				// Simulate dispatching events
				_dispatch: (event: string, data: unknown) => {
					const eventListeners = listeners[event];
					if (eventListeners) {
						eventListeners.forEach((handler) => handler({ data: JSON.stringify(data) }));
					}
				},
			};
		}));
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	// -----------------------------------------------------------------------
	// active_logs handshake (full sync)
	// -----------------------------------------------------------------------

	it("should initially populate activeLogs from the active_logs handshake event", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		// Simulate the handshake event (EventSource fires 'active_logs')
		const eventSource = (globalThis as any).EventSource.mock.results[0].value;
		eventSource._dispatch("active_logs", [mockActiveLog]);

		expect(result.current.activeLogs).toHaveLength(1);
		expect(result.current.activeLogs[0].id).toBe("active-1");
		expect(result.current.activeLogs[0].status).toBe("processing");
	});

	it("should replace the entire activeLogs array on each active_logs handshake", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		// First handshake: one active log
		eventSource._dispatch("active_logs", [mockActiveLog]);
		expect(result.current.activeLogs).toHaveLength(1);

		// Second handshake: two active logs (simulating a new request arriving)
		eventSource._dispatch("active_logs", [
			mockActiveLog,
			{ ...mockActiveLog, id: "active-2", provider: "anthropic", model: "claude-3" },
		]);
		expect(result.current.activeLogs).toHaveLength(2);
	});

	// -----------------------------------------------------------------------
	// log_updated incremental merge
	// -----------------------------------------------------------------------

	it("should merge log_updated into the existing activeLogs array", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		// Handshake: populate activeLogs
		eventSource._dispatch("active_logs", [mockActiveLog]);
		expect(result.current.activeLogs[0].status).toBe("processing");

		// Incremental update: log_updated changes status from processing to success
		eventSource._dispatch("log_updated", {
			id: "active-1",
			previous_status: "processing",
			status: "success",
			latency_ms: 1234.0,
		});

		// The active log should now be updated
		expect(result.current.activeLogs).toHaveLength(1);
		expect(result.current.activeLogs[0].status).toBe("success");
		expect(result.current.activeLogs[0].latency).toBe(1234.0);
	});

	it("should add a new log on log_updated when the id does not exist in activeLogs", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		// Handshake: no active logs
		eventSource._dispatch("active_logs", []);

		// New processing log arrives via log_updated
		eventSource._dispatch("log_updated", {
			id: "new-request-1",
			previous_status: null,
			status: "processing",
			latency_ms: null,
		});

		expect(result.current.activeLogs).toHaveLength(1);
		expect(result.current.activeLogs[0].id).toBe("new-request-1");
		expect(result.current.activeLogs[0].status).toBe("processing");
	});

	it("should remove a log from activeLogs when status is 'success' or 'error' and no previous_status", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		// Handshake: one processing log
		eventSource._dispatch("active_logs", [mockActiveLog]);
		expect(result.current.activeLogs).toHaveLength(1);

		// Completion: mark as success
		eventSource._dispatch("log_updated", {
			id: "active-1",
			previous_status: "processing",
			status: "success",
			latency_ms: 1234.0,
		});

		// After completion, the log should be removed from activeLogs (it's no longer active)
		expect(result.current.activeLogs).toHaveLength(0);
	});

	// -----------------------------------------------------------------------
	// Connection lifecycle
	// -----------------------------------------------------------------------

	it("should create an EventSource on mount with the correct URL", () => {
		renderHook(() => useLogsTimelineSSE());

		expect(globalThis.EventSource).toHaveBeenCalledTimes(1);
		// The URL should point to the active/stream SSE endpoint
		const calledUrl = (globalThis.EventSource as any).mock.calls[0][0];
		expect(calledUrl).toContain("/logs/active/stream");
	});

	it("should close the EventSource on unmount", () => {
		const { unmount } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;
		unmount();

		expect(eventSource.close).toHaveBeenCalledTimes(1);
	});

	// -----------------------------------------------------------------------
	// Error handling
	// -----------------------------------------------------------------------

	it("should set error state when EventSource fires an error event", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		// Simulate error event
		eventSource._dispatch("error", { status: 503, message: "Service Unavailable" });

		expect(result.current.error).toBeTruthy();
		expect(result.current.isConnected).toBe(false);
	});
});