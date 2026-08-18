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

import { LogsVolumeChart } from "./logsVolumeChart";
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