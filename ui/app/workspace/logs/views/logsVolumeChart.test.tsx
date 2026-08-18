// @vitest-environment jsdom
/**
 * @file LogsVolumeChart — ResponsiveContainer sizing tests
 *
 * Recharts 3's ResponsiveContainer seeds its measured size at -1×-1 and emits a
 * console.warn ("The width(-1) and height(-1) of chart should be greater than 0")
 * on the first render, before the ResizeObserver effect reads the real box.
 *
 * The component passes a positive `initialDimension` so that pre-measure frame
 * never warns and never paints a mis-sized chart. These tests pin that behavior.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render } from "@testing-library/react";

import { LogsVolumeChart, fillHistogramBuckets } from "./logsVolumeChart";
import type { LogsHistogramResponse } from "@/lib/types/logs";

const SIZE_WARNING = "should be greater than 0";

// jsdom has no ResizeObserver and reports every box as 0×0. Provide one that
// immediately reports a real content size on observe(), mirroring a laid-out
// browser container, so the chart measures a positive size after mount.
class MockResizeObserver {
	callback: ResizeObserverCallback;
	constructor(callback: ResizeObserverCallback) {
		this.callback = callback;
	}
	observe(target: Element) {
		this.callback([{ contentRect: { width: 800, height: 128 } } as unknown as ResizeObserverEntry], this as unknown as ResizeObserver);
		void target;
	}
	unobserve() {}
	disconnect() {}
}

function buildData(): LogsHistogramResponse {
	// One API bucket; the component fills the rest across the 10-minute window.
	const startMs = 1_700_000_000_000;
	return {
		bucket_size_seconds: 60,
		buckets: [
			{
				timestamp: new Date(startMs).toISOString(),
				count: 5,
				success: 4,
				error: 1,
				cancelled: 0,
			},
		],
	};
}

const defaultProps = {
	data: buildData(),
	loading: false,
	onTimeRangeChange: () => {},
	onResetZoom: () => {},
	isZoomed: false,
	// 10-minute window → 10 filled buckets → hasValidData is true, so the main
	// (data) ResponsiveContainer renders rather than the EmptyChart fallback.
	startTime: 1_700_000_000,
	endTime: 1_700_000_600,
	isOpen: true,
	period: undefined as string | undefined,
	onOpenChange: () => {},
};

describe("LogsVolumeChart — ResponsiveContainer sizing", () => {
	let warnSpy: ReturnType<typeof vi.spyOn>;
	const originalResizeObserver = (globalThis as any).ResizeObserver;

	beforeEach(() => {
		(globalThis as any).ResizeObserver = MockResizeObserver;
		warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
	});

	afterEach(() => {
		warnSpy.mockRestore();
		(globalThis as any).ResizeObserver = originalResizeObserver;
	});

	const sizeWarnings = () => warnSpy.mock.calls.filter((args: unknown[]) => String(args[0] ?? "").includes(SIZE_WARNING));

	it("does not log the Recharts width/height warning for the data chart on mount", () => {
		render(<LogsVolumeChart {...defaultProps} />);
		expect(sizeWarnings()).toHaveLength(0);
	});

	it("does not log the Recharts width/height warning for the empty fallback on mount", () => {
		// No buckets + zero window → hasValidData is false → EmptyChart renders.
		render(<LogsVolumeChart {...defaultProps} data={{ bucket_size_seconds: 60, buckets: [] }} startTime={0} endTime={0} />);
		expect(sizeWarnings()).toHaveLength(0);
	});
});

// Pin the regression: in "Last X" period mode the X-axis right edge must
// advance when a polling refresh delivers newer buckets. Previously the
// effectingTimeRange useMemo captured `now` once and never recomputed, so
// after 18 minutes the latest bars would render against ticks frozen at the
// original page-load time (e.g. 21:13 even though the wall-clock was 21:31).
describe("fillHistogramBuckets — sliding window right edge", () => {
	function bucketResponses(startSec: number, count: number): LogsHistogramResponse {
		return {
			bucket_size_seconds: 60,
			buckets: Array.from({ length: count }, (_, i) => ({
				timestamp: new Date((startSec + i * 60) * 1000).toISOString(),
				count: i === count - 1 ? 7 : 0,
				success: i === count - 1 ? 6 : 0,
				error: i === count - 1 ? 1 : 0,
				cancelled: 0,
			})),
		};
	}

	it("extends the filled series to the latest bucket edge when new buckets arrive", () => {
		// Start: period anchor at 20:14, current right edge at 21:14. Data carries
		// 14 buckets, so its last bucket-end is 21:14. The fill loop emits indices
		// 0..59 spanning 20:14 .. 21:13 (the upper bound is exclusive), so the
		// final formattedTime must be "21:13".
		const startSec = Date.UTC(2024, 0, 1, 20, 14, 0) / 1000;
		const endSec = Date.UTC(2024, 0, 1, 21, 14, 0) / 1000;
		const first = fillHistogramBuckets(bucketResponses(Date.UTC(2024, 0, 1, 21, 0, 0) / 1000, 14), startSec, endSec);
		expect(first).toHaveLength(60);
		expect(first[first.length - 1].formattedTime).toBe("21:13");

		// Eighteen minutes later the polling refresh lands. Parent passes the new
		// "now" as endTime (21:32), and data now contains 32 buckets whose last
		// bucket-end is 21:32. The new fill spans 20:14 .. 21:32 (78 minutes →
		// 78 indices) and must end with the formatter's representation of 21:31
		// — proving the right edge slides forward.
		const refreshedEndSec = Date.UTC(2024, 0, 1, 21, 32, 0) / 1000;
		const second = fillHistogramBuckets(bucketResponses(Date.UTC(2024, 0, 1, 21, 0, 0) / 1000, 32), startSec, refreshedEndSec);
		expect(second).toHaveLength(78);
		expect(second[second.length - 1].formattedTime).toBe("21:31");
	});

	it("places API buckets at their correct index inside the filled window", () => {
		const startSec = Date.UTC(2024, 0, 1, 20, 0, 0) / 1000;
		const endSec = Date.UTC(2024, 0, 1, 21, 0, 0) / 1000;
		// Only one bucket at 20:30 — it must land at index 30 in the 60-bucket
		// series; the rest stay zero.
		const data: LogsHistogramResponse = {
			bucket_size_seconds: 60,
			buckets: [{ timestamp: new Date(Date.UTC(2024, 0, 1, 20, 30, 0)).toISOString(), count: 9, success: 9, error: 0, cancelled: 0 }],
		};
		const filled = fillHistogramBuckets(data, startSec, endSec);
		expect(filled).toHaveLength(60);
		expect(filled[30].count).toBe(9);
		expect(filled[29].count).toBe(0);
		expect(filled[31].count).toBe(0);
	});

	it("returns empty when the window is degenerate", () => {
		expect(fillHistogramBuckets(buildData(), 0, 0)).toEqual([]);
		expect(fillHistogramBuckets(buildData(), 100, 50)).toEqual([]);
		expect(fillHistogramBuckets(null, 0, 100)).toEqual([]);
	});
});

// Pin the integration: in period mode, a rerender with newer buckets must push
// the chartData array's rightmost element forward. The DOM-level assertions
// are unreliable under jsdom (recharts 3 doesn't render xAxis <text> nodes
// without a real font-measurer, see file-level note), so we keep the dominant
// regression pinned at the unit level (fillHistogramBuckets above) and just
// smoke-test that the period branch doesn't throw on a second polling round.
describe("LogsVolumeChart — period mode rerender honors fresher buckets", () => {
	const originalResizeObserver = (globalThis as any).ResizeObserver;

	beforeEach(() => {
		(globalThis as any).ResizeObserver = MockResizeObserver;
	});

	afterEach(() => {
		(globalThis as any).ResizeObserver = originalResizeObserver;
	});

	it("renders without throwing when a period component is rerendered with newer buckets", () => {
		const startMs = Date.UTC(2024, 0, 1, 20, 14, 0);
		const endMs = Date.UTC(2024, 0, 1, 21, 14, 0);

		const first: LogsHistogramResponse = {
			bucket_size_seconds: 60,
			buckets: Array.from({ length: 14 }, (_, i) => ({
				timestamp: new Date(startMs + i * 60 * 1000).toISOString(),
				count: 0,
				success: 0,
				error: 0,
				cancelled: 0,
			})),
		};

		const { rerender } = render(
			<LogsVolumeChart {...defaultProps} data={first} startTime={startMs / 1000} endTime={endMs / 1000} period="1h" />,
		);

		// Rerender with a fresher payload whose end edge sits past the original
		// period anchor — this would have silently no-op'd on the old code.
		const refreshedEndMs = Date.UTC(2024, 0, 1, 21, 32, 0);
		const refreshed: LogsHistogramResponse = {
			bucket_size_seconds: 60,
			buckets: Array.from({ length: 32 }, (_, i) => ({
				timestamp: new Date(startMs + i * 60 * 1000).toISOString(),
				count: 0,
				success: 0,
				error: 0,
				cancelled: 0,
			})),
		};
		expect(() =>
			rerender(
				<LogsVolumeChart {...defaultProps} data={refreshed} startTime={startMs / 1000} endTime={refreshedEndMs / 1000} period="1h" />,
			),
		).not.toThrow();
	});
});