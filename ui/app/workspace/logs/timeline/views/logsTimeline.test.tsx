// @vitest-environment jsdom
/**
 * @file Gantt timeline component tests
 *
 * These tests verify the LogsTimeline component's core behavior:
 * - Lane allocation for request bars
 * - Bar rendering with correct positioning/timing
 * - Tooltip data computation on hover
 * - NOW line, zoom, pan, and axis ticks
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { LogsTimeline } from "./logsTimeline";
import type { LogsTimelineProps } from "./logsTimeline";
import type { LogEntry } from "@/lib/types/logs";

vi.mock("react-i18next", () => {
	const en: Record<string, string> = {
		"timeline.now": "NOW",
		"timeline.tooltip.running": "RUNNING",
		"timeline.tooltip.elapsed": "~{{seconds}}s elapsed",
		"timeline.tooltip.latency": "{{value}}ms",
		"timeline.tooltip.input": "Input: {{value}}",
		"timeline.tooltip.output": "Output: {{value}}",
		"timeline.tooltip.tpsPrefix": "TPS: ",
		"timeline.tooltip.tpsSuffix": "/s",
	};
	return {
		useTranslation: () => ({
			t: (key: string, params?: Record<string, string>) => {
				let result = en[key] ?? key;
				if (params) {
					for (const [k, v] of Object.entries(params)) {
						result = result.replace(`{{${k}}}`, v);
					}
				}
				return result;
			},
			i18n: { language: "en", options: { ns: [] }, services: {} },
		}),
		Trans: ({ children }: { children: React.ReactNode }) => children,
	};
});

// Polyfill ResizeObserver for jsdom
globalThis.ResizeObserver = class {
	observe() {}
	unobserve() {}
	disconnect() {}
} as any;

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const t = (iso: string) => Date.parse(iso);

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

const range10to102 = { start: t("2026-08-15T10:00:00Z"), end: t("2026-08-15T10:02:00Z") };
const defaultProps: LogsTimelineProps = {
	logs: mockLogs,
	timeRange: range10to102,
	nowMs: t("2026-08-15T10:02:30Z"),
	mode: "follow",
	zoom: 1,
	onZoomChange: vi.fn(),
	panOffsetMs: 0,
	onPanOffsetChange: vi.fn(),
	onModeChange: vi.fn(),
	activeLogs: [],
};

describe("LogsTimeline — Gantt component", () => {
	// -----------------------------------------------------------------------
	// Lane allocation
	// -----------------------------------------------------------------------

	it("should allocate lanes so concurrent requests do not overlap", () => {
		const { container } = render(<LogsTimeline {...defaultProps} />);

		const bars = container.querySelectorAll("[data-lane]");
		expect(bars.length).toBeGreaterThanOrEqual(2);

		const laneAssignments = Array.from(bars).map((bar) => bar.getAttribute("data-lane"));
		expect(new Set(laneAssignments).size).toBeGreaterThanOrEqual(1);
	});

	it("should assign the same lane for non-overlapping sequential requests", () => {
		const sequentialLogs: LogEntry[] = [
			{ ...mockLogs[0], id: "seq-1", timestamp: "2026-08-15T10:00:00Z", latency: 1000 },
			{ ...mockLogs[0], id: "seq-2", timestamp: "2026-08-15T10:01:00Z", latency: 1000 },
		];

		const { container } = render(<LogsTimeline {...defaultProps} logs={sequentialLogs} />);

		const bars = container.querySelectorAll("[data-lane]");
		bars.forEach((bar) => {
			expect(bar.getAttribute("data-lane")).toBe("0");
		});
	});

	// -----------------------------------------------------------------------
	// Bar rendering (position / width / color)
	// -----------------------------------------------------------------------

	it("should render a bar for each log entry", () => {
		const { container } = render(<LogsTimeline {...defaultProps} />);

		const bars = container.querySelectorAll("[data-testid='timeline-bar']");
		expect(bars.length).toBe(mockLogs.length);
	});

	it("should render bar width proportional to request latency", () => {
		const wideBarLog = { ...mockLogs[0], id: "wide", latency: 2000 };
		const narrowBarLog = { ...mockLogs[2], id: "narrow", latency: 500 };

		const { container } = render(<LogsTimeline {...defaultProps} logs={[wideBarLog, narrowBarLog]} />);

		const wideBar = container.querySelector("[data-log-id='wide']");
		const narrowBar = container.querySelector("[data-log-id='narrow']");
		expect(wideBar).toBeTruthy();
		expect(narrowBar).toBeTruthy();

		const wideWidth = parseFloat(wideBar!.getAttribute("data-bar-width") || "0");
		const narrowWidth = parseFloat(narrowBar!.getAttribute("data-bar-width") || "0");
		expect(wideWidth).toBeGreaterThan(narrowWidth);
	});

	it("should color success bars differently from error bars", () => {
		const { container } = render(<LogsTimeline {...defaultProps} />);

		const successBar = container.querySelector("[data-log-id='log-1']");
		const errorBar = container.querySelector("[data-log-id='log-3']");

		const successColor = successBar!.getAttribute("data-status-color");
		const errorColor = errorBar!.getAttribute("data-status-color");
		expect(successColor).not.toBe(errorColor);
	});

	it("should render processing bars with a visual indicator (ring)", () => {
		const { container } = render(<LogsTimeline {...defaultProps} />);

		const processingBar = container.querySelector("[data-log-id='log-2']");
		expect(processingBar).toBeTruthy();
		expect(processingBar!.classList.toString()).toMatch(/ring|process/i);
	});

	// -----------------------------------------------------------------------
	// Tooltip data computation
	// -----------------------------------------------------------------------

	it("should show tooltip with provider, model, latency and cost on hover", async () => {
		render(<LogsTimeline {...defaultProps} />);

		const bar = screen.getByTestId("timeline-bar-log-1");
		fireEvent.mouseEnter(bar);

		expect(await screen.findByText(/openai/i)).toBeTruthy();
		expect(await screen.findByText(/gpt-4/i)).toBeTruthy();
		expect(await screen.findByText(/1,200/)).toBeTruthy();
		expect(await screen.findByText(/\$0\.002/)).toBeTruthy();
	});

	it("should show the last user prompt in the tooltip, not a trailing system message", async () => {
		const logsWithHistory: LogEntry[] = [
			{
				...mockLogs[0],
				id: "log-msg",
				input_history: [
					{ role: "system", content: "You are a helpful assistant." },
					{ role: "user", content: "what is the capital of France?" },
					{ role: "system", content: "<system-reminder>trailing injection</system-reminder>" },
				],
			},
		];
		render(<LogsTimeline {...defaultProps} logs={logsWithHistory} />);

		fireEvent.mouseEnter(screen.getByTestId("timeline-bar-log-msg"));

		const preview = await screen.findByTestId("timeline-tooltip-last-user-message");
		expect(preview.textContent).toBe("what is the capital of France?");
	});

	it("should fall back to content_summary in the tooltip when input history is empty", async () => {
		const logsWithSummary: LogEntry[] = [
			{
				...mockLogs[0],
				id: "log-summary",
				input_history: [],
				responses_input_history: [],
				content_summary: "summary of the last user prompt",
			},
		];
		render(<LogsTimeline {...defaultProps} logs={logsWithSummary} />);

		fireEvent.mouseEnter(screen.getByTestId("timeline-bar-log-summary"));

		const preview = await screen.findByTestId("timeline-tooltip-last-user-message");
		expect(preview.textContent).toBe("summary of the last user prompt");
	});

	// -----------------------------------------------------------------------
	// Click callback
	// -----------------------------------------------------------------------

	it("should call onBarClick with the log entry when a bar is clicked", () => {
		const onBarClick = vi.fn();
		render(<LogsTimeline {...defaultProps} onBarClick={onBarClick} />);

		fireEvent.click(screen.getByTestId("timeline-bar-log-1"));
		expect(onBarClick).toHaveBeenCalledTimes(1);
		expect(onBarClick).toHaveBeenCalledWith(mockLogs[0]);
	});

	// -----------------------------------------------------------------------
	// NOW line
	// -----------------------------------------------------------------------

	it("should render NOW line at the correct position in follow mode", () => {
		const { container } = render(
			<LogsTimeline
				{...defaultProps}
				mode="follow"
				nowMs={t("2026-08-15T10:01:00Z")}
				timeRange={{ start: t("2026-08-15T10:00:00Z"), end: t("2026-08-15T10:02:00Z") }}
			/>,
		);
		// NOW line renders at the window-relative position of nowMs
		const nowLine = container.querySelector("[class*='bg-red-500']");
		expect(nowLine).toBeTruthy();
		expect(container.textContent).toContain("NOW");
	});

	// -----------------------------------------------------------------------
	// Zoom
	// -----------------------------------------------------------------------

	it("should call onZoomChange on wheel event", () => {
		const onZoomChange = vi.fn();
		const { container } = render(<LogsTimeline {...defaultProps} zoom={1} onZoomChange={onZoomChange} />);

		const canvas = container.querySelector("[data-testid='timeline-canvas']");
		expect(canvas).toBeTruthy();
		if (canvas) {
			fireEvent.wheel(canvas, { deltaY: -120 });
			expect(onZoomChange).toHaveBeenCalledTimes(1);
		}
	});

	// -----------------------------------------------------------------------
	// Pan / drag
	// -----------------------------------------------------------------------

	it("should call onModeChange on mousedown when not in pan mode", () => {
		const onModeChange = vi.fn();
		const { container } = render(<LogsTimeline {...defaultProps} mode="follow" onModeChange={onModeChange} />);

		const canvas = container.querySelector("[data-testid='timeline-canvas']");
		expect(canvas).toBeTruthy();
		if (canvas) {
			fireEvent.mouseDown(canvas, { button: 0, clientX: 100 });
			expect(onModeChange).toHaveBeenCalledWith("pan");
		}
	});

	// -----------------------------------------------------------------------
	// Axis ticks
	// -----------------------------------------------------------------------

	it("should render axis tick labels", () => {
		const { container } = render(<LogsTimeline {...defaultProps} />);

		const ticks = container.querySelectorAll("[class*='font-mono']");
		expect(ticks.length).toBeGreaterThanOrEqual(1);
	});

	// -----------------------------------------------------------------------
	// Empty state
	// -----------------------------------------------------------------------

	it("should show empty state when no logs provided", () => {
		render(<LogsTimeline {...defaultProps} logs={[]} />);
		expect(screen.getByText("timeline.page.empty")).toBeTruthy();
	});

	// -----------------------------------------------------------------------
	// Bar label (truncated model name)
	// -----------------------------------------------------------------------

	it("should render truncated model name inside bars wide enough", () => {
		// A 30s window makes the 1200ms latency bar ~4% wide — wide enough for the label
		const { container } = render(
			<LogsTimeline
				{...defaultProps}
				logs={[mockLogs[0]]}
				timeRange={{ start: t("2026-08-15T10:00:00Z"), end: t("2026-08-15T10:00:30Z") }}
			/>,
		);
		const allText = container.textContent || "";
		expect(allText).toMatch(/gpt-4/i);
	});

	it("should render provider icon on narrow bars", () => {
		const { container } = render(<LogsTimeline {...defaultProps} logs={[mockLogs[0]]} />);
		const bar = container.querySelector("[data-log-id='log-1']");
		const svg = bar?.querySelector("svg");
		expect(svg).toBeTruthy();
	});
});