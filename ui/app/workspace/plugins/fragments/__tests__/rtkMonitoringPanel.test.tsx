// @vitest-environment jsdom
/**
 * @file RTK MonitoringPanel tests.
 *
 * Contract:
 *   - MonitoringPanel is exported from ui/app/workspace/plugins/fragments/rtkFragment.tsx
 *   - It uses useGetRtkStatsQuery() → GET /api/context/rtk/stats
 *   - Stats cards render invocations / compressedCount / tokensSaved / compressionRatio
 *   - Empty state copy appears when the gateway has not compressed anything yet
 *   - Raw-output link points at /workspace/plugins/rtk/raw-output
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// Module under test
import { MonitoringPanel } from "../rtkFragment";

// Stub <Link> with a plain anchor — the test only cares about the rendered
// href/structure, not router-internal behaviour. Mirrors the pattern used
// by pluginsView.test.tsx, with data-testid forwarded so the assertions on
// the MonitoringPanel's raw-output link can find the element.
vi.mock("@tanstack/react-router", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@tanstack/react-router")>();
	return {
		...actual,
		useNavigate: () => vi.fn(),
		Link: ({ children, ...props }: any) => (
			<a data-mock-link="true" data-testid={props["data-testid"]} href={props.to || "#"}>
				{children}
			</a>
		),
	};
});

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mocks = vi.hoisted(() => ({
	statsData: {
		invocations: 42,
		compressedCount: 17,
		originalTokens: 8000,
		compressedTokens: 2000,
		tokensSaved: 6000,
		compressionRatio: 0.75,
	} as {
		invocations: number;
		compressedCount: number;
		originalTokens: number;
		compressedTokens: number;
		tokensSaved: number;
		compressionRatio: number;
		engineBreakdown?: import("@/lib/types/plugins").RtkEngineStat[];
	},
	loading: false,
}));

vi.mock("@/lib/store/apis/pluginsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/pluginsApi")>();
	return {
		...actual,
		useGetRtkStatsQuery: vi.fn(() => ({ data: { stats: mocks.statsData }, isLoading: mocks.loading })),
	};
});

// Re-import the mocked hook so individual tests can override its return
// value with mockReturnValueOnce when they need a specific shape (e.g. the
// loading-only state on first render).
import { useGetRtkStatsQuery } from "@/lib/store/apis/pluginsApi";

// ---------------------------------------------------------------------------
// Tests — populated state
// ---------------------------------------------------------------------------

describe("RTK MonitoringPanel — populated state", () => {
	beforeEach(() => {
		mocks.statsData = {
			invocations: 42,
			compressedCount: 17,
			originalTokens: 8000,
			compressedTokens: 2000,
			tokensSaved: 6000,
			compressionRatio: 0.75,
		};
		mocks.loading = false;
	});

	it("renders all four stat cards", () => {
		render(<MonitoringPanel />);

		expect(screen.getByTestId("rtk-stats-invocations")).toBeTruthy();
		expect(screen.getByTestId("rtk-stats-compressed")).toBeTruthy();
		expect(screen.getByTestId("rtk-stats-tokens-saved")).toBeTruthy();
		expect(screen.getByTestId("rtk-stats-compression-ratio")).toBeTruthy();
	});

	it("renders invocationCount from useGetRtkStatsQuery", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("rtk-stats-invocations").textContent).toContain("42");
	});

	it("renders compressedCount from useGetRtkStatsQuery", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("rtk-stats-compressed").textContent).toContain("17");
	});

	it("renders tokensSaved from useGetRtkStatsQuery", () => {
		render(<MonitoringPanel />);
		// formatCompactNumber(6000) → "6.0k" (k-range, sub-10k → 1 decimal)
		expect(screen.getByTestId("rtk-stats-tokens-saved").textContent).toContain("6.0k");
	});

	it("renders the compression ratio as a percentage", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("rtk-stats-compression-ratio").textContent).toContain("75%");
	});
});

// ---------------------------------------------------------------------------
// Tests — empty state
// ---------------------------------------------------------------------------

describe("RTK MonitoringPanel — empty state", () => {
	beforeEach(() => {
		mocks.statsData = {
			invocations: 0,
			compressedCount: 0,
			originalTokens: 0,
			compressedTokens: 0,
			tokensSaved: 0,
			compressionRatio: 0,
		};
		mocks.loading = false;
	});

	it("renders zero counters without crashing", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("rtk-stats-invocations").textContent).toContain("0");
		expect(screen.getByTestId("rtk-stats-compression-ratio").textContent).toContain("0%");
	});

	it("renders the empty-state copy when nothing has been compressed yet", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("rtk-stats-empty")).toBeTruthy();
		expect(screen.getByTestId("rtk-stats-empty").textContent).toMatch(/no compression activity/i);
	});
});

// ---------------------------------------------------------------------------
// Tests — loading state
// ---------------------------------------------------------------------------

describe("RTK MonitoringPanel — loading state", () => {
	beforeEach(() => {
		mocks.loading = true;
		mocks.statsData = {
			invocations: 0,
			compressedCount: 0,
			originalTokens: 0,
			compressedTokens: 0,
			tokensSaved: 0,
			compressionRatio: 0,
		};
	});

	it("renders a loading indicator instead of the stat cards when no cached data exists", () => {
		// Override the mock for this single test so useGetRtkStatsQuery
		// returns no data on first render — the only state where the
		// panel shows the loading placeholder.
		vi.mocked(useGetRtkStatsQuery).mockReturnValueOnce({ data: undefined, isLoading: true } as any);
		render(<MonitoringPanel />);
		expect(screen.getByText(/loading monitoring/i)).toBeTruthy();
		expect(screen.queryByTestId("rtk-stats-invocations")).toBeNull();
	});
});

// ---------------------------------------------------------------------------
// Tests — per-engine breakdown (V-pipeline-engine-stats)
// ---------------------------------------------------------------------------

describe("RTK MonitoringPanel — per-engine breakdown", () => {
	beforeEach(() => {
		mocks.loading = false;
		mocks.statsData = {
			invocations: 25,
			compressedCount: 22,
			originalTokens: 8000,
			compressedTokens: 2000,
			tokensSaved: 6000,
			compressionRatio: 0.75,
			engineBreakdown: [
				{ id: "caveman", invocations: 5, inputBytes: 800, outputBytes: 300, compressedBy: 0.625 },
				{ id: "rtk", invocations: 20, inputBytes: 4000, outputBytes: 1200, compressedBy: 0.7 },
			],
		};
	});

	it("renders the per-engine stats group when the server reports engine activity", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("rtk-engine-stats-group")).toBeTruthy();
	});

	it("renders one row per engine id with rtk-engine-stats-<id> testids", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("rtk-engine-stats-rtk")).toBeTruthy();
		expect(screen.getByTestId("rtk-engine-stats-caveman")).toBeTruthy();
	});

	it("renders the engine invocations and bytes inside each row", () => {
		render(<MonitoringPanel />);
		// rtk row: 20 invocations, 4 kB in, 1.2 kB out, 70% ratio.
		const rtkRow = screen.getByTestId("rtk-engine-stats-rtk");
		expect(rtkRow.textContent).toContain("20");
		expect(rtkRow.textContent).toContain("4.0 kB");
		expect(rtkRow.textContent).toContain("70%");
		// caveman row: 5 invocations, 800 B in, 300 B out, 62.5% ratio.
		const cavemanRow = screen.getByTestId("rtk-engine-stats-caveman");
		expect(cavemanRow.textContent).toContain("5");
		expect(cavemanRow.textContent).toContain("800");
		expect(cavemanRow.textContent).toContain("300");
	});

	it("hides the per-engine group when the server reports no engine activity", () => {
		mocks.statsData = {
			invocations: 0,
			compressedCount: 0,
			originalTokens: 0,
			compressedTokens: 0,
			tokensSaved: 0,
			compressionRatio: 0,
			// engineBreakdown absent (server omits the key when nothing has run).
		};
		render(<MonitoringPanel />);
		expect(screen.queryByTestId("rtk-engine-stats-group")).toBeNull();
		expect(screen.queryByTestId("rtk-engine-stats-rtk")).toBeNull();
	});
});