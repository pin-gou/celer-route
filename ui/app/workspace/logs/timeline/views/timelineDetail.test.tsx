// @vitest-environment jsdom
/**
 * @file Timeline detail panel tests
 *
 * These tests verify the TimelineDetail component (rendered inside the
 * /workspace/logs/{id} "时间线" tab) after the per-phase waterfall
 * redesign. Assertions cover:
 *   - Header (log id + request latency + event-span diff hint)
 *   - Phase grouping (groups sorted by phase order, events within a group)
 *   - Per-row data (offset, duration, phase chip, source/plugin chip, message)
 *   - Level styling (info / warn / error)
 *   - Loading / error / empty states
 *
 * The legacy `timeline-event-row-{level}` testids are preserved so any
 * existing E2E selector keeps working — the component still emits exactly
 * one row per event under the matching phase group.
 */

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TimelineDetail } from "./timelineDetail";

// ---------------------------------------------------------------------------
// Types based on design doc: GET /api/logs/{id}/timeline response
// ---------------------------------------------------------------------------

interface TimelineEvent {
	time_ms_offset: number;
	duration_ms: number;
	phase: string;
	source: string;
	message: string;
	level: string;
	plugin_name: string;
}

interface TimelineResponse {
	log_id: string;
	total_duration_ms: number;
	events: TimelineEvent[];
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const mockTimelineResponse: TimelineResponse = {
	log_id: "b8f2a1c3-1234-5678-9abc-def012345678",
	total_duration_ms: 1234.56,
	events: [
		{
			time_ms_offset: 0.0,
			duration_ms: 8.2,
			phase: "pre_llm",
			source: "plugin_logging",
			message: "pre-llm hook executed",
			level: "info",
			plugin_name: "logging",
		},
		{
			time_ms_offset: 20.1,
			duration_ms: 1100.0,
			phase: "upstream_call",
			source: "routing_engine",
			message: "provider=ali model=qwen-max attempt=0",
			level: "info",
			plugin_name: "",
		},
		{
			time_ms_offset: 20.1,
			duration_ms: 0.0,
			phase: "key_attempt",
			source: "attempt_trail",
			message: "key_id=xxxx status=success",
			level: "info",
			plugin_name: "",
		},
		{
			time_ms_offset: 1128.0,
			duration_ms: 6.5,
			phase: "post_llm",
			source: "plugin_logging",
			message: "post-llm hook executed",
			level: "info",
			plugin_name: "logging",
		},
	],
};

const mockTimelineResponseWithWarnings: TimelineResponse = {
	log_id: "log-warn-1",
	total_duration_ms: 2500.0,
	events: [
		{
			time_ms_offset: 0.0,
			duration_ms: 5.0,
			phase: "pre_llm",
			source: "plugin_logging",
			message: "pre-llm hook executed",
			level: "info",
			plugin_name: "logging",
		},
		{
			time_ms_offset: 10.0,
			duration_ms: 0.0,
			phase: "plugin_log",
			source: "plugin_logs",
			message: "rate limit approaching: 80/100 rpm",
			level: "warn",
			plugin_name: "governance",
		},
		{
			time_ms_offset: 1500.0,
			duration_ms: 5.0,
			phase: "post_llm",
			source: "plugin_logging",
			message: "post-llm hook executed",
			level: "error",
			plugin_name: "logging",
		},
	],
};

const emptyTimelineResponse: TimelineResponse = {
	log_id: "empty-log-1",
	total_duration_ms: 0,
	events: [],
};

const populatedZeroDurationResponse: TimelineResponse = {
	log_id: "zero-log-1",
	total_duration_ms: 0,
	events: [
		{
			time_ms_offset: 0,
			duration_ms: 0,
			phase: "pre_llm",
			source: "plugin_logging",
			message: "pre-llm hook executed",
			level: "info",
			plugin_name: "logging",
		},
	],
};

describe("TimelineDetail — detail panel events list", () => {
	// -----------------------------------------------------------------------
	// Header
	// -----------------------------------------------------------------------

	it("should render the log_id and total duration header", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		expect(screen.getByText(/b8f2a1c3/i)).toBeTruthy();
		// Total duration should be displayed
		expect(screen.getByText(/1,234\.56/)).toBeTruthy();
	});

	it("should expose a copy-log-id affordance in the header", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);
		expect(screen.getByTestId("timeline-header-copy-id")).toBeTruthy();
	});

	it("should show the event-span diff when it diverges from request latency", () => {
		// eventSpan = 1128 + 6.5 = 1134.5; totalDurationMs = 1234.56 → diff = -100.06ms (~ -8.1%)
		render(<TimelineDetail data={mockTimelineResponse} />);
		const span = screen.getByTestId("timeline-header-event-span");
		expect(span).toBeTruthy();
		expect(span.textContent ?? "").toMatch(/1,134\.50|1134\.5/);
	});

	it("should NOT show the diff hint when the two values are within tolerance", () => {
		// totalDurationMs = 1100.5, eventSpan = 1100.0 (within 0.5ms) → no diff row
		const resp: TimelineResponse = {
			log_id: "close-log",
			total_duration_ms: 1100.5,
			events: [
				{
					time_ms_offset: 0,
					duration_ms: 1100,
					phase: "upstream_call",
					source: "routing_engine",
					message: "ok",
					level: "info",
					plugin_name: "",
				},
			],
		};
		render(<TimelineDetail data={resp} />);
		expect(screen.queryByTestId("timeline-header-event-span")).toBeNull();
	});

	it("should show the diff hint when post_llm overruns log.Latency by a few ms", () => {
		// Reproduces the bab31b69 case where the upstream provider reports
		// 8992ms but the post_llm hook fires at 8994ms (PostLLMHook itself
		// runs after the provider timer is stamped). The diff row must be
		// visible so the user understands why the last event sits past the
		// advertised latency.
		const resp: TimelineResponse = {
			log_id: "bab31b69-...",
			total_duration_ms: 8992,
			events: [
				{
					time_ms_offset: 0,
					duration_ms: 0,
					phase: "pre_llm",
					source: "plugin_logging",
					message: "pre-llm hook executed",
					level: "info",
					plugin_name: "logging",
				},
				{
					time_ms_offset: 8994,
					duration_ms: 0,
					phase: "post_llm",
					source: "plugin_logging",
					message: "post-llm hook executed",
					level: "info",
					plugin_name: "logging",
				},
			],
		};
		render(<TimelineDetail data={resp} />);
		// The span row must appear even when the diff is small; the user
		// benefits from seeing the post-llm overrun explained.
		const span = screen.getByTestId("timeline-header-event-span");
		expect(span).toBeTruthy();
	});

	// -----------------------------------------------------------------------
	// Phase grouping
	// -----------------------------------------------------------------------

	it("should render each event as a row", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		// All 4 events should be rendered
		const eventRows = screen.getAllByTestId(/timeline-event-row/);
		expect(eventRows.length).toBe(4);
	});

	it("should expose phase group containers", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		const groups = screen.getAllByTestId("timeline-phase-group");
		expect(groups.length).toBeGreaterThanOrEqual(3); // pre_llm + upstream_call + key_attempt + post_llm
		const phases = groups.map((g) => g.getAttribute("data-phase"));
		expect(phases.slice(0, 3)).toEqual(["pre_llm", "upstream_call", "key_attempt"]);
	});

	it("should mark each event row with its raw phase via data-phase", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		const rows = screen.getAllByTestId(/timeline-event-row/);
		const phases = rows.map((r) => r.getAttribute("data-phase"));
		expect(phases).toContain("pre_llm");
		expect(phases).toContain("upstream_call");
		expect(phases).toContain("key_attempt");
		expect(phases).toContain("post_llm");
	});

	it("should render events in chronological order (by time_ms_offset)", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		const eventRows = screen.getAllByTestId(/timeline-event-row/);
		// pre_llm at 0ms comes first
		expect(eventRows[0]?.getAttribute("data-phase")).toBe("pre_llm");
		// post_llm at 1128ms comes last
		expect(eventRows[eventRows.length - 1]?.getAttribute("data-phase")).toBe("post_llm");
	});

	// -----------------------------------------------------------------------
	// Empty state
	// -----------------------------------------------------------------------

	it("should show empty state when there are no events", () => {
		render(<TimelineDetail data={emptyTimelineResponse} />);

		expect(screen.getByTestId("timeline-empty")).toBeTruthy();
	});

	it("should use the 'failed/aborted' empty hint when totalDurationMs is 0", () => {
		render(<TimelineDetail data={emptyTimelineResponse} />);
		// 'failedHint' is distinct from the generic hint — rendered via i18n key fallback
		// when the logs namespace isn't loaded by the test setup.
		const hint = screen.getByTestId("timeline-empty");
		expect(hint.textContent ?? "").toMatch(/timeline\.detail\.empty\.failedHint|short-circuit/i);
	});

	// -----------------------------------------------------------------------
	// Per-row content
	// -----------------------------------------------------------------------

	it("should display the offset and duration for each event", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		// pre_llm row's offset cell renders 0.0
		const offsets = screen.getAllByTestId("timeline-event-offset");
		expect(offsets.map((n) => n.textContent).join("|")).toContain("0.0");

		// upstream_call row's duration cell renders 1100.0
		const durations = screen.getAllByTestId("timeline-event-duration");
		const durText = durations.map((n) => n.textContent).join("|");
		expect(durText).toContain("1100.0");
		// key_attempt with duration_ms=0 should render an em-dash placeholder
		expect(durText).toContain("—");
	});

	it("should display the event message verbatim (no truncation)", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		expect(screen.getByText(/pre-llm hook executed/i)).toBeTruthy();
		expect(screen.getByText(/provider=ali model=qwen-max attempt=0/i)).toBeTruthy();
		expect(screen.getByText(/key_id=xxxx status=success/i)).toBeTruthy();
		expect(screen.getByText(/post-llm hook executed/i)).toBeTruthy();
	});

	it("should expose the source string on every row", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		const sources = screen.getAllByTestId("timeline-event-source").map((n) => n.textContent);
		expect(sources.some((t) => t && t.includes("plugin_logging"))).toBe(true);
		expect(sources.some((t) => t && t.includes("routing_engine"))).toBe(true);
		expect(sources.some((t) => t && t.includes("attempt_trail"))).toBe(true);
	});

	// -----------------------------------------------------------------------
	// Level indicators
	// -----------------------------------------------------------------------

	it("should style warn level events with a warning indicator", () => {
		render(<TimelineDetail data={mockTimelineResponseWithWarnings} />);

		const warnRow = screen.getByTestId("timeline-event-row-warn");
		expect(warnRow).toBeTruthy();
		expect(warnRow.className).toMatch(/warn|yellow|amber/i);
	});

	it("should style error level events with an error indicator", () => {
		render(<TimelineDetail data={mockTimelineResponseWithWarnings} />);

		const errorRow = screen.getByTestId("timeline-event-row-error");
		expect(errorRow).toBeTruthy();
		expect(errorRow.className).toMatch(/error|red|destruct/i);
	});

	it("should style info level events with a neutral indicator", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		const infoRows = screen.getAllByTestId("timeline-event-row-info");
		expect(infoRows.length).toBeGreaterThanOrEqual(1);
		// All info rows should have a neutral style (no warn/error indicator)
		infoRows.forEach((row) => {
			expect(row.className).not.toMatch(/warn|yellow|amber|error|red|destruct/i);
		});
	});

	// -----------------------------------------------------------------------
	// Plugin name display
	// -----------------------------------------------------------------------

	it("should expose a copy button per row that targets the event message", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);
		const copyButtons = screen.getAllByTestId("timeline-event-copy");
		expect(copyButtons.length).toBe(4);
	});

	// -----------------------------------------------------------------------
	// Loading state
	// -----------------------------------------------------------------------

	it("should show a loading indicator when isLoading is true", () => {
		render(<TimelineDetail data={null} isLoading={true} />);

		expect(screen.getByTestId("timeline-loading")).toBeTruthy();
	});

	// -----------------------------------------------------------------------
	// Error state
	// -----------------------------------------------------------------------

	it("should show a generic error message when error is provided", () => {
		render(<TimelineDetail data={null} error="Failed to load timeline" />);

		expect(screen.getByText(/Failed to load timeline/i)).toBeTruthy();
	});

	it("should flag network errors with a dedicated title and render a retry control when onRetry is supplied", () => {
		render(<TimelineDetail data={null} error="network request failed" onRetry={() => {}} />);
		expect(screen.getByTestId("timeline-error")).toBeTruthy();
		// i18n fallback returns the key when the logs namespace isn't loaded by
		// the test setup; verify the dedicated network-error branch ran.
		const errorEl = screen.getByTestId("timeline-error");
		expect(errorEl.textContent ?? "").toMatch(/timeline\.detail\.error\.networkTitle|network error/i);
		expect(screen.getByTestId("timeline-error-retry")).toBeTruthy();
	});

	// -----------------------------------------------------------------------
	// Malformed API payload — backend may serialize a nil events slice as null
	// -----------------------------------------------------------------------

	it("should not throw when the API returns events: null (e.g. a request with no events recorded)", () => {
		// Regression: GET /api/logs/{id}/timeline returned `"events": null` when a
		// log had no timeline events recorded, and `events.length` crashed with
		// "Cannot read properties of null (reading 'length')".
		const dataWithNullEvents: TimelineResponse = {
			log_id: "error-log-null-events",
			total_duration_ms: 136024,
			events: null as unknown as TimelineEvent[],
		};

		render(<TimelineDetail data={dataWithNullEvents} />);

		expect(screen.getByTestId("timeline-detail")).toBeTruthy();
		expect(screen.getByTestId("timeline-empty")).toBeTruthy();
	});

	// -----------------------------------------------------------------------
	// Edge case — populated response with zero total_duration_ms
	// -----------------------------------------------------------------------

	it("should still render events when total_duration_ms is 0 but events are present", () => {
		render(<TimelineDetail data={populatedZeroDurationResponse} />);
		const rows = screen.getAllByTestId(/timeline-event-row/);
		expect(rows.length).toBe(1);
		// The diff row should be suppressed because totalDurationMs=0
		expect(screen.queryByTestId("timeline-header-event-span")).toBeNull();
	});
});