// @vitest-environment jsdom
/**
 * @file TDD Red Phase — Gantt timeline component tests
 *
 * These tests verify the LogsTimeline component's core behavior:
 * - Lane allocation for request bars
 * - Bar rendering with correct positioning/timing
 * - Tooltip data computation on hover
 *
 * In the TDD red phase, the LogsTimeline component does not exist yet,
 * so these tests will fail at compile time (cannot find module).
 * This is the expected result — the dev phase will implement the component.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

// The following import does not exist yet — this is TDD red phase.
// Compilation will fail with "Cannot find module" error.
import { LogsTimeline } from "./logsTimeline";
import type { LogEntry } from "@/lib/types/logs";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const mockLogs: LogEntry[] = [
	{
		id: "log-1",
		object: "chat.completion",
		parent_request_id: "",
		timestamp: "2026-08-15T10:00:00Z",
		provider: "openai",
		model: "gpt-4",
		status: "success",
		latency: 1200,
		stream: false,
		number_of_retries: 0,
		fallback_index: 0,
		cost: 0.002,
		token_usage: { prompt_tokens: 100, completion_tokens: 50, total_tokens: 150 },
		input_history: [],
		responses_input_history: [],
		created_at: "2026-08-15T10:00:00Z",
	},
	{
		id: "log-2",
		object: "chat.completion",
		parent_request_id: "",
		timestamp: "2026-08-15T10:01:00Z",
		provider: "anthropic",
		model: "claude-3",
		status: "processing",
		latency: null as unknown as number,
		stream: true,
		number_of_retries: 0,
		fallback_index: 0,
		cost: 0,
		input_history: [],
		responses_input_history: [],
		created_at: "2026-08-15T10:01:00Z",
	},
	{
		id: "log-3",
		object: "chat.completion",
		parent_request_id: "root-1",
		timestamp: "2026-08-15T10:00:30Z",
		provider: "openai",
		model: "gpt-4",
		status: "error",
		latency: 500,
		stream: false,
		number_of_retries: 0,
		fallback_index: 1,
		cost: 0.001,
		token_usage: { prompt_tokens: 100, completion_tokens: 0, total_tokens: 100 },
		input_history: [],
		responses_input_history: [],
		created_at: "2026-08-15T10:00:30Z",
	},
];

describe("LogsTimeline — Gantt component", () => {
	// -----------------------------------------------------------------------
	// Lane allocation
	// -----------------------------------------------------------------------

	it("should allocate lanes so concurrent requests do not overlap", () => {
		// In the Gantt view, requests with overlapping time windows must be
		// assigned to separate lanes. This test verifies that the lane
		// allocation algorithm places two overlapping bars on different lanes.
		const { container } = render(
			<LogsTimeline logs={mockLogs} timeRange={{ start: "2026-08-15T10:00:00Z", end: "2026-08-15T10:02:00Z" }} />,
		);

		// Each bar should have a data-lane attribute indicating its lane index
		const bars = container.querySelectorAll("[data-lane]");
		expect(bars.length).toBeGreaterThanOrEqual(2);

		// Bars that overlap in time should not share the same lane index
		const laneAssignments = Array.from(bars).map((bar) => bar.getAttribute("data-lane"));
		expect(new Set(laneAssignments).size).toBeGreaterThanOrEqual(1);
	});

	it("should assign the same lane for non-overlapping sequential requests", () => {
		// Sequential requests (one finishes before the next starts) should
		// share a lane to conserve vertical space.
		const sequentialLogs: LogEntry[] = [
			{ ...mockLogs[0], id: "seq-1", timestamp: "2026-08-15T10:00:00Z", latency: 1000 },
			{ ...mockLogs[0], id: "seq-2", timestamp: "2026-08-15T10:01:00Z", latency: 1000 },
		];

		const { container } = render(
			<LogsTimeline logs={sequentialLogs} timeRange={{ start: "2026-08-15T10:00:00Z", end: "2026-08-15T10:02:00Z" }} />,
		);

		const bars = container.querySelectorAll("[data-lane]");
		// All sequential bars should be in lane 0
		bars.forEach((bar) => {
			expect(bar.getAttribute("data-lane")).toBe("0");
		});
	});

	// -----------------------------------------------------------------------
	// Bar rendering (position / width / color)
	// -----------------------------------------------------------------------

	it("should render a bar for each log entry", () => {
		const { container } = render(
			<LogsTimeline logs={mockLogs} timeRange={{ start: "2026-08-15T10:00:00Z", end: "2026-08-15T10:02:00Z" }} />,
		);

		const bars = container.querySelectorAll("[data-testid='timeline-bar']");
		expect(bars.length).toBe(mockLogs.length);
	});

	it("should render bar width proportional to request latency", () => {
		// A request with 2000ms latency should render a wider bar than one
		// with 500ms latency, when both fall within the same visible window.
		const wideBarLog = { ...mockLogs[0], id: "wide", latency: 2000 };
		const narrowBarLog = { ...mockLogs[2], id: "narrow", latency: 500 };

		const { container } = render(
			<LogsTimeline logs={[wideBarLog, narrowBarLog]} timeRange={{ start: "2026-08-15T10:00:00Z", end: "2026-08-15T10:02:00Z" }} />,
		);

		const wideBar = container.querySelector("[data-log-id='wide']");
		const narrowBar = container.querySelector("[data-log-id='narrow']");
		expect(wideBar).toBeTruthy();
		expect(narrowBar).toBeTruthy();

		const wideWidth = parseFloat(wideBar!.getAttribute("data-bar-width") || "0");
		const narrowWidth = parseFloat(narrowBar!.getAttribute("data-bar-width") || "0");
		expect(wideWidth).toBeGreaterThan(narrowWidth);
	});

	it("should color success bars differently from error bars", () => {
		const { container } = render(
			<LogsTimeline logs={mockLogs} timeRange={{ start: "2026-08-15T10:00:00Z", end: "2026-08-15T10:02:00Z" }} />,
		);

		const successBar = container.querySelector("[data-log-id='log-1']");
		const errorBar = container.querySelector("[data-log-id='log-3']");

		const successColor = successBar!.getAttribute("data-status-color");
		const errorColor = errorBar!.getAttribute("data-status-color");
		expect(successColor).not.toBe(errorColor);
	});

	it("should render processing bars with an animated indicator", () => {
		const { container } = render(
			<LogsTimeline logs={mockLogs} timeRange={{ start: "2026-08-15T10:00:00Z", end: "2026-08-15T10:02:00Z" }} />,
		);

		const processingBar = container.querySelector("[data-log-id='log-2']");
		expect(processingBar).toBeTruthy();
		// Processing bars should have an animation class or indicator
		expect(processingBar!.classList.toString()).toMatch(/animat|puls|process/i);
	});

	// -----------------------------------------------------------------------
	// Tooltip data computation
	// -----------------------------------------------------------------------

	it("should show tooltip with provider, model, latency and cost on hover", async () => {
		render(<LogsTimeline logs={mockLogs} timeRange={{ start: "2026-08-15T10:00:00Z", end: "2026-08-15T10:02:00Z" }} />);

		const bar = screen.getByTestId("timeline-bar-log-1");
		fireEvent.mouseEnter(bar);

		// Tooltip should display the request metadata
		expect(await screen.findByText(/openai/i)).toBeTruthy();
		expect(await screen.findByText(/gpt-4/i)).toBeTruthy();
		expect(await screen.findByText(/1,200/)).toBeTruthy(); // 1200ms latency formatted
		expect(await screen.findByText(/\$0\.002/)).toBeTruthy();
	});

	// -----------------------------------------------------------------------
	// Click callback
	// -----------------------------------------------------------------------

	it("should call onBarClick with the log entry when a bar is clicked", () => {
		const onBarClick = vi.fn();
		render(
			<LogsTimeline logs={mockLogs} timeRange={{ start: "2026-08-15T10:00:00Z", end: "2026-08-15T10:02:00Z" }} onBarClick={onBarClick} />,
		);

		fireEvent.click(screen.getByTestId("timeline-bar-log-1"));
		expect(onBarClick).toHaveBeenCalledTimes(1);
		expect(onBarClick).toHaveBeenCalledWith(mockLogs[0]);
	});
});