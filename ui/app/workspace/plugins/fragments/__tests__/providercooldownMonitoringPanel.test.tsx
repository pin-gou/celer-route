// @vitest-environment jsdom
/**
 * @file TDD Red Phase — MonitoringPanel tests (dev.ui task 11.2)
 *
 * Contract (design.md "组件设计 / MonitoringPanel"):
 *   - MonitoringPanel is exported from ui/app/workspace/plugins/fragments/providercooldownFragment.tsx
 *   - It uses the following RTK Query hooks:
 *       useGetCooldownStateQuery()  → GET /api/plugins/provider-cooldown/state
 *       useGetCooldownStatsQuery()  → GET /api/plugins/provider-cooldown/stats
 *       useUnfreezeCooldownMutation() → DELETE /api/plugins/provider-cooldown/state/{provider}/{keyId}
 *   - Stats display: markCount, suppressedCount, activeCount
 *   - State list: entries with provider, keyId, expireAt, reason
 *   - Unfreeze button per entry triggers mutation with { provider, keyId }
 *
 * In the TDD red phase the fragment module does not exist yet — this file is
 * expected to fail at load time ("Failed to resolve import ../providercooldownFragment").
 * This is the expected TDD red-phase result.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Red phase: the fragment module does not exist yet — this import will fail.
// ---------------------------------------------------------------------------
import { MonitoringPanel } from "../providercooldownFragment";

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

const mocks = vi.hoisted(() => ({
	unfreeze: vi.fn(),
	// Per-test holders for hook data
	stateData: [
		{
			provider: "openai",
			keyId: "key-abc-123",
			expireAt: "2026-08-15T18:00:00Z",
			reason: "quota_exhausted",
		},
		{
			provider: "anthropic",
			keyId: "key-def-456",
			expireAt: "2026-08-15T18:30:00Z",
			reason: "rate_limited",
		},
	] as Array<{ provider: string; keyId: string; expireAt: string; reason: string }>,
	statsData: { markCount: 12, suppressedCount: 8, activeCount: 3 },
}));

vi.mock("@/lib/store/apis/pluginsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/pluginsApi")>();
	return {
		...actual,
		useGetCooldownStateQuery: () => ({ data: { state: mocks.stateData }, isLoading: false }),
		useGetCooldownStatsQuery: () => ({ data: { stats: mocks.statsData }, isLoading: false }),
		useUnfreezeCooldownMutation: () => [mocks.unfreeze, { isLoading: false }],
	};
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("MonitoringPanel — stats rendering (task 11.2)", () => {
	beforeEach(() => {
		mocks.unfreeze.mockReset();
		mocks.stateData = [
			{ provider: "openai", keyId: "key-abc-123", expireAt: "2026-08-15T18:00:00Z", reason: "quota_exhausted" },
			{ provider: "anthropic", keyId: "key-def-456", expireAt: "2026-08-15T18:30:00Z", reason: "rate_limited" },
		];
		mocks.statsData = { markCount: 12, suppressedCount: 8, activeCount: 3 };
	});

	it("renders markCount from useGetCooldownStatsQuery", () => {
		render(<MonitoringPanel />);

		expect(screen.getByTestId("providercooldown-stats-mark").textContent).toContain("12");
	});

	it("renders suppressedCount from useGetCooldownStatsQuery", () => {
		render(<MonitoringPanel />);

		expect(screen.getByTestId("providercooldown-stats-suppressed").textContent).toContain("8");
	});

	it("renders activeCount from useGetCooldownStatsQuery", () => {
		render(<MonitoringPanel />);

		expect(screen.getByTestId("providercooldown-stats-active").textContent).toContain("3");
	});
});

describe("MonitoringPanel — state list rendering (task 11.2)", () => {
	beforeEach(() => {
		mocks.unfreeze.mockReset();
		mocks.stateData = [
			{ provider: "openai", keyId: "key-abc-123", expireAt: "2026-08-15T18:00:00Z", reason: "quota_exhausted" },
			{ provider: "anthropic", keyId: "key-def-456", expireAt: "2026-08-15T18:30:00Z", reason: "rate_limited" },
		];
	});

	it("renders cooldown state entries from useGetCooldownStateQuery", () => {
		render(<MonitoringPanel />);

		// First entry: openai / key-abc-123
		const row1 = screen.getByTestId("providercooldown-state-row-key-abc-123");
		expect(row1).toBeTruthy();
		expect(row1.textContent).toContain("openai");
		expect(row1.textContent).toContain("key-abc-123");
		expect(row1.textContent).toContain("quota_exhausted");

		// Second entry: anthropic / key-def-456
		const row2 = screen.getByTestId("providercooldown-state-row-key-def-456");
		expect(row2).toBeTruthy();
		expect(row2.textContent).toContain("anthropic");
		expect(row2.textContent).toContain("key-def-456");
		expect(row2.textContent).toContain("rate_limited");
	});

	it("triggers DELETE via useUnfreezeCooldownMutation when unfreeze button is clicked", () => {
		render(<MonitoringPanel />);

		fireEvent.click(screen.getByTestId("providercooldown-state-row-key-abc-123-unfreeze"));
		expect(mocks.unfreeze).toHaveBeenCalledWith({ provider: "openai", keyId: "key-abc-123" });
	});

	it("unfreeze button for the second entry uses the correct provider/keyId", () => {
		render(<MonitoringPanel />);

		fireEvent.click(screen.getByTestId("providercooldown-state-row-key-def-456-unfreeze"));
		expect(mocks.unfreeze).toHaveBeenCalledWith({ provider: "anthropic", keyId: "key-def-456" });
	});
});

describe("MonitoringPanel — empty state (task 11.2)", () => {
	beforeEach(() => {
		mocks.unfreeze.mockReset();
		mocks.stateData = [];
		mocks.statsData = { markCount: 0, suppressedCount: 0, activeCount: 0 };
	});

	it("renders an empty state message when no keys are cooling down", () => {
		render(<MonitoringPanel />);

		expect(screen.getByText(/no .*cooldown/i)).toBeTruthy();
	});

	it("renders zero stats when no cooldown activity has occurred", () => {
		render(<MonitoringPanel />);

		expect(screen.getByTestId("providercooldown-stats-mark").textContent).toContain("0");
		expect(screen.getByTestId("providercooldown-stats-suppressed").textContent).toContain("0");
		expect(screen.getByTestId("providercooldown-stats-active").textContent).toContain("0");
	});
});