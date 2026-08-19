// @vitest-environment jsdom
/**
 * @file Timeline detail panel tests
 *
 * These tests verify the TimelineDetail component (rendered inside the
 * standalone log detail page at /workspace/logs/$id) that renders events
 * from the GET /api/logs/{id}/timeline API response.
 *
 * The timeline detail displays a sorted list of events with:
 * - Time offset from request start
 * - Phase name (pre_llm, upstream_call, key_attempt, post_llm, plugin_log)
 * - Source (plugin_logging, routing_engine, attempt_trail, plugin_logs)
 * - Human-readable message
 * - Duration
 * - Level badge (info/warn/error)
 *
 * In the TDD red phase, the TimelineDetail component does not exist yet,
 * so these tests will fail at compile time (cannot find module).
 * This is the expected result — the dev phase will implement the component.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// The following import does not exist yet — this is TDD red phase.
// Compilation will fail with "Cannot find module" error.
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

describe("TimelineDetail — detail panel events list", () => {
	// -----------------------------------------------------------------------
	// Basic rendering
	// -----------------------------------------------------------------------

	it("should render the log_id and total duration header", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		expect(screen.getByText(/b8f2a1c3/i)).toBeTruthy();
		// Total duration should be displayed
		expect(screen.getByText(/1,234\.56/)).toBeTruthy();
	});

	it("should render each event as a row", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		// All 4 events should be rendered
		const eventRows = screen.getAllByTestId(/timeline-event-row/);
		expect(eventRows.length).toBe(4);
	});

	it("should render events in chronological order (by time_ms_offset)", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		const eventRows = screen.getAllByTestId(/timeline-event-row/);
		// Events should be sorted by time_ms_offset ascending
		// First event: pre_llm at 0ms
		expect(eventRows[0]?.textContent).toContain("pre_llm");
		expect(eventRows[0]?.textContent).toContain("0.0");
		// Last event: post_llm at 1128ms
		expect(eventRows[eventRows.length - 1]?.textContent).toContain("post_llm");
		expect(eventRows[eventRows.length - 1]?.textContent).toContain("1128.0");
	});

	it("should show empty state when there are no events", () => {
		render(<TimelineDetail data={emptyTimelineResponse} />);

		expect(screen.getByText(/no events/i)).toBeTruthy();
	});

	// -----------------------------------------------------------------------
	// Event field rendering
	// -----------------------------------------------------------------------

	it("should display the phase name for each event", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		expect(screen.getByText("pre_llm")).toBeTruthy();
		expect(screen.getByText("upstream_call")).toBeTruthy();
		expect(screen.getByText("key_attempt")).toBeTruthy();
		expect(screen.getByText("post_llm")).toBeTruthy();
	});

	it("should display the time offset and duration for each event", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		// Some values appear in multiple events (time offset + duration across events),
		// so use getAllByText for those.
		expect(screen.getAllByText("0.0 ms").length).toBeGreaterThanOrEqual(1);
		expect(screen.getByText("8.2 ms")).toBeTruthy();
		expect(screen.getAllByText("20.1 ms").length).toBeGreaterThanOrEqual(1);
		expect(screen.getByText("1100.0 ms")).toBeTruthy();
		expect(screen.getByText("1128.0 ms")).toBeTruthy();
	});

	it("should display the event message", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		expect(screen.getByText(/pre-llm hook executed/i)).toBeTruthy();
		expect(screen.getByText(/provider=ali model=qwen-max attempt=0/i)).toBeTruthy();
		expect(screen.getByText(/key_id=xxxx status=success/i)).toBeTruthy();
		expect(screen.getByText(/post-llm hook executed/i)).toBeTruthy();
	});

	it("should display the source for each event", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		// plugin_logging appears in multiple events (pre_llm and post_llm),
		// so use getAllByText for that and getByText for unique values.
		expect(screen.getAllByText("plugin_logging").length).toBeGreaterThanOrEqual(1);
		expect(screen.getByText("routing_engine")).toBeTruthy();
		expect(screen.getByText("attempt_trail")).toBeTruthy();
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

	it("should display the plugin name when source is plugin_logging", () => {
		render(<TimelineDetail data={mockTimelineResponse} />);

		// plugin_name should be shown for plugin_logging source events
		const pluginBadges = screen.getAllByText("logging");
		expect(pluginBadges.length).toBeGreaterThanOrEqual(1);
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

	it("should show an error message when error is provided", () => {
		render(<TimelineDetail data={null} error="Failed to load timeline" />);

		expect(screen.getByText(/Failed to load timeline/i)).toBeTruthy();
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
		expect(screen.getByText(/no events recorded/i)).toBeTruthy();
	});
});