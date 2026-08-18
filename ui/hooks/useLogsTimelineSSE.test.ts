// @vitest-environment jsdom
/**
 * @file SSE hook for logs timeline — tests
 *
 * These tests verify the useLogsTimelineSSE hook that subscribes to
 * GET /api/logs/active/stream for real-time log updates.
 *
 * Behavior:
 * - active_logs: initial snapshot of processing logs
 * - recent_logs: recently completed logs for reconciliation
 * - log_updated: incremental updates for every log write/transition
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

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
		vi.stubGlobal(
			"EventSource",
			vi.fn(function () {
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
					_dispatch: (event: string, data: unknown) => {
						const eventListeners = listeners[event];
						if (eventListeners) {
							eventListeners.forEach((handler) => handler({ data: JSON.stringify(data) }));
						}
					},
				};
			}),
		);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	// -----------------------------------------------------------------------
	// active_logs handshake (full sync)
	// -----------------------------------------------------------------------

	it("should initially populate activeLogs from the active_logs handshake event", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;
		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog]);
		});

		expect(result.current.activeLogs).toHaveLength(1);
		expect(result.current.activeLogs[0].id).toBe("active-1");
		expect(result.current.activeLogs[0].status).toBe("processing");
	});

	it("should replace the entire activeLogs array on each active_logs handshake", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog]);
		});
		expect(result.current.activeLogs).toHaveLength(1);

		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog, { ...mockActiveLog, id: "active-2", provider: "anthropic", model: "claude-3" }]);
		});
		expect(result.current.activeLogs).toHaveLength(2);
	});

	// -----------------------------------------------------------------------
	// recent_logs reconciliation
	// -----------------------------------------------------------------------

	it("should call onNewLog for each recent_logs entry", () => {
		const onNewLog = vi.fn();
		renderHook(() => useLogsTimelineSSE({ onNewLog }));

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;
		act(() => {
			eventSource._dispatch("recent_logs", [
				{ id: "recent-1", status: "success", provider: "openai", model: "gpt-4", timestamp: "2026-08-15T10:01:00Z", latency_ms: 500 },
			]);
		});

		expect(onNewLog).toHaveBeenCalledTimes(1);
		expect(onNewLog).toHaveBeenCalledWith(
			expect.objectContaining({ id: "recent-1", status: "success", provider: "openai", model: "gpt-4" }),
		);
	});

	// -----------------------------------------------------------------------
	// log_updated incremental merge
	// -----------------------------------------------------------------------

	it("should merge log_updated into the existing activeLogs array", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog]);
		});
		expect(result.current.activeLogs[0].status).toBe("processing");

		act(() => {
			eventSource._dispatch("log_updated", {
				id: "active-1",
				status: "processing",
				latency_ms: 500.0,
			});
		});

		expect(result.current.activeLogs).toHaveLength(1);
		expect(result.current.activeLogs[0].status).toBe("processing");
		expect(result.current.activeLogs[0].latency).toBe(500.0);
	});

	it("should add a new processing log on log_updated when the id does not exist in activeLogs", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		act(() => {
			eventSource._dispatch("active_logs", []);
		});

		act(() => {
			eventSource._dispatch("log_updated", {
				id: "new-request-1",
				status: "processing",
				provider: "anthropic",
				model: "claude-3",
				latency_ms: null,
			});
		});

		expect(result.current.activeLogs).toHaveLength(1);
		expect(result.current.activeLogs[0].id).toBe("new-request-1");
		expect(result.current.activeLogs[0].status).toBe("processing");
	});

	it("should remove a log from activeLogs when status is 'success' or 'error'", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog]);
		});
		expect(result.current.activeLogs).toHaveLength(1);

		act(() => {
			eventSource._dispatch("log_updated", {
				id: "active-1",
				status: "success",
				latency_ms: 1234.0,
			});
		});

		expect(result.current.activeLogs).toHaveLength(0);
	});

	it("should call onNewLog for a completed log not in activeLogs", () => {
		const onNewLog = vi.fn();
		const { result } = renderHook(() => useLogsTimelineSSE({ onNewLog }));

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		act(() => {
			eventSource._dispatch("active_logs", []);
		});

		act(() => {
			eventSource._dispatch("log_updated", {
				id: "fast-complete-1",
				status: "success",
				provider: "openai",
				model: "gpt-4",
				timestamp: "2026-08-15T10:01:00Z",
				latency_ms: 200,
			});
		});

		expect(onNewLog).toHaveBeenCalledTimes(1);
		expect(onNewLog).toHaveBeenCalledWith(expect.objectContaining({ id: "fast-complete-1", status: "success" }));
		expect(result.current.activeLogs).toHaveLength(0);
	});

	it("should call onLogRemoved and onNewLog when a processing log transitions to terminal", () => {
		const onLogRemoved = vi.fn();
		const onNewLog = vi.fn();
		const { result } = renderHook(() => useLogsTimelineSSE({ onLogRemoved, onNewLog }));

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog]);
		});
		expect(result.current.activeLogs).toHaveLength(1);

		act(() => {
			eventSource._dispatch("log_updated", {
				id: "active-1",
				status: "success",
				provider: "openai",
				model: "gpt-4",
				latency_ms: 1234.0,
			});
		});

		expect(onLogRemoved).toHaveBeenCalledTimes(1);
		expect(onLogRemoved).toHaveBeenCalledWith("active-1");
		expect(onNewLog).toHaveBeenCalledTimes(1);
		expect(onNewLog).toHaveBeenCalledWith(
			expect.objectContaining({ id: "active-1", status: "success", provider: "openai", model: "gpt-4" }),
		);
		expect(result.current.activeLogs).toHaveLength(0);
	});

	it("should treat a 'cancelled' status as terminal (remove from active + onNewLog)", () => {
		const onLogRemoved = vi.fn();
		const onNewLog = vi.fn();
		const { result } = renderHook(() => useLogsTimelineSSE({ onLogRemoved, onNewLog }));

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;

		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog]);
		});
		expect(result.current.activeLogs).toHaveLength(1);

		// A key-pool veto (synthetic 503 no_eligible_keys) is recorded as cancelled.
		act(() => {
			eventSource._dispatch("log_updated", {
				id: "active-1",
				status: "cancelled",
				provider: "openai",
				model: "gpt-4",
				latency_ms: 0,
			});
		});

		expect(onLogRemoved).toHaveBeenCalledTimes(1);
		expect(onLogRemoved).toHaveBeenCalledWith("active-1");
		expect(onNewLog).toHaveBeenCalledTimes(1);
		expect(onNewLog).toHaveBeenCalledWith(
			expect.objectContaining({ id: "active-1", status: "cancelled", provider: "openai", model: "gpt-4" }),
		);
		expect(result.current.activeLogs).toHaveLength(0);
	});

	// -----------------------------------------------------------------------
	// Connection lifecycle
	// -----------------------------------------------------------------------

	it("should create an EventSource on mount with the correct URL", () => {
		renderHook(() => useLogsTimelineSSE());

		expect(globalThis.EventSource).toHaveBeenCalledTimes(1);
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

		act(() => {
			eventSource._dispatch("error", { status: 503, message: "Service Unavailable" });
		});

		expect(result.current.error).toBeTruthy();
		expect(result.current.isConnected).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// TTL sweep — runs with fake timers so we can drive the 10-minute TTL and
// the 30-second sweep interval without waiting in wall-clock time. The hook
// reads Date.now() for lastSeen comparisons, so we tick the fake clock in
// lockstep with each act() that depends on time.
// ---------------------------------------------------------------------------

describe("useLogsTimelineSSE — TTL sweep", () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.stubGlobal(
			"EventSource",
			vi.fn(function () {
				const listeners: Record<string, Set<(...args: unknown[]) => void>> = {};
				return {
					close: vi.fn(),
					addEventListener: vi.fn((event: string, handler: (...args: unknown[]) => void) => {
						if (!listeners[event]) listeners[event] = new Set();
						listeners[event].add(handler);
					}),
					removeEventListener: vi.fn(),
					_dispatch: (event: string, data: unknown) => {
						const eventListeners = listeners[event];
						if (eventListeners) {
							eventListeners.forEach((handler) => handler({ data: JSON.stringify(data) }));
						}
					},
				};
			}),
		);
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.unstubAllGlobals();
	});

	it("should drop an activeLogs entry that hasn't been updated for longer than the TTL", () => {
		const onLogRemoved = vi.fn();
		const { result } = renderHook(() => useLogsTimelineSSE({ onLogRemoved }));

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;
		const t0 = Date.now();

		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog]);
		});
		expect(result.current.activeLogs).toHaveLength(1);

		// Advance past TTL (10 min) but trigger the 30s sweep before checking.
		act(() => {
			vi.setSystemTime(t0 + 11 * 60 * 1000);
			vi.advanceTimersByTime(30 * 1000);
		});

		expect(result.current.activeLogs).toHaveLength(0);
		expect(onLogRemoved).toHaveBeenCalledWith("active-1");
	});

	it("should keep an activeLogs entry that received a recent log_updated within the TTL window", () => {
		const { result } = renderHook(() => useLogsTimelineSSE());

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;
		const t0 = Date.now();

		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog]);
		});

		// Halfway through the TTL window, refresh the entry with a new
		// log_updated — this resets its lastSeen.
		act(() => {
			vi.setSystemTime(t0 + 5 * 60 * 1000);
			eventSource._dispatch("log_updated", { id: "active-1", status: "processing", latency_ms: 500 });
		});

		// Sweep fires at t0 + 5min + 30s — only 30s since the refresh, well
		// inside the 10-minute TTL.
		act(() => {
			vi.advanceTimersByTime(30 * 1000);
		});

		expect(result.current.activeLogs).toHaveLength(1);
		expect(result.current.activeLogs[0].id).toBe("active-1");
	});

	it("should only evict entries older than the TTL, leaving fresh ones intact", () => {
		const onLogRemoved = vi.fn();
		const { result } = renderHook(() => useLogsTimelineSSE({ onLogRemoved }));

		const eventSource = (globalThis as any).EventSource.mock.results[0].value;
		const t0 = Date.now();

		act(() => {
			eventSource._dispatch("active_logs", [mockActiveLog, { ...mockActiveLog, id: "active-2", provider: "anthropic", model: "claude-3" }]);
		});
		expect(result.current.activeLogs).toHaveLength(2);

		// Refresh only active-2 at t0 + 11min. active-1 stays at t0.
		act(() => {
			vi.setSystemTime(t0 + 11 * 60 * 1000);
			eventSource._dispatch("log_updated", { id: "active-2", status: "processing", latency_ms: 100 });
		});

		// Sweep at t0 + 11min: cutoff = t0 + 1min, so active-1 (lastSeen=t0)
		// expires, active-2 (lastSeen=t0+11min) is fresh.
		act(() => {
			vi.advanceTimersByTime(30 * 1000);
		});

		expect(result.current.activeLogs).toHaveLength(1);
		expect(result.current.activeLogs[0].id).toBe("active-2");
		expect(onLogRemoved).toHaveBeenCalledTimes(1);
		expect(onLogRemoved).toHaveBeenCalledWith("active-1");
	});

	it("should clear the sweep timer when the consumer unmounts", () => {
		const clearIntervalSpy = vi.spyOn(globalThis, "clearInterval");
		const { unmount } = renderHook(() => useLogsTimelineSSE());

		unmount();

		// The hook installs two timers in its main useEffect: nothing else, so
		// clearInterval should fire at least once for the sweep timer. (Exact
		// count varies if other code paths schedule intervals; assert >= 1.)
		expect(clearIntervalSpy).toHaveBeenCalled();
		clearIntervalSpy.mockRestore();
	});
});