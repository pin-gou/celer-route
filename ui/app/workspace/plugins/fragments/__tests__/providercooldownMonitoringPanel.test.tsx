// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { MonitoringPanel } from "../providercooldownFragment";

const mocks = vi.hoisted(() => ({
	unfreeze: vi.fn(),
	stateData: [
		{ provider: "openai", keyId: "key-abc-123", keyName: "prod-openai-key", expireAt: "2026-08-15T18:00:00Z", reason: "rate_limit" },
		{ provider: "anthropic", keyId: "key-def-456", keyName: "prod-anthropic-key", expireAt: "2026-08-15T18:30:00Z", reason: "quota" },
	] as Array<{ provider: string; keyId: string; keyName?: string; expireAt: string; reason: string }>,
	statsData: {
		markCount: 12,
		suppressedCount: 8,
		activeCount: 3,
		byKind: {
			rate_limit: { markCount: 7, suppressedCount: 5 },
			quota: { markCount: 5, suppressedCount: 3 },
		},
	} as any,
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

describe("MonitoringPanel — stats cards (4-kind layout)", () => {
	beforeEach(() => {
		mocks.unfreeze.mockReset();
		mocks.stateData = [
			{ provider: "openai", keyId: "key-abc-123", keyName: "prod-openai-key", expireAt: "2026-08-15T18:00:00Z", reason: "rate_limit" },
			{ provider: "anthropic", keyId: "key-def-456", keyName: "prod-anthropic-key", expireAt: "2026-08-15T18:30:00Z", reason: "quota" },
		];
		mocks.statsData = {
			markCount: 12,
			suppressedCount: 8,
			activeCount: 3,
			byKind: { rate_limit: { markCount: 7, suppressedCount: 5 }, quota: { markCount: 5, suppressedCount: 3 } },
		};
	});

	it("renders rate_limit mark count", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("providercooldown-stats-rate_limit-mark").textContent).toContain("7");
	});

	it("renders quota mark count", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("providercooldown-stats-quota-mark").textContent).toContain("5");
	});

	it("renders rate_limit suppressed count", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("providercooldown-stats-rate_limit-suppressed").textContent).toContain("5");
	});

	it("renders quota suppressed count", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("providercooldown-stats-quota-suppressed").textContent).toContain("3");
	});

	it("renders active count", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("providercooldown-stats-active").textContent).toContain("3");
	});
});

describe("MonitoringPanel — state list rendering", () => {
	beforeEach(() => {
		mocks.unfreeze.mockReset();
		mocks.stateData = [
			{ provider: "openai", keyId: "key-abc-123", keyName: "prod-openai-key", expireAt: "2026-08-15T18:00:00Z", reason: "rate_limit" },
			{ provider: "anthropic", keyId: "key-def-456", keyName: "prod-anthropic-key", expireAt: "2026-08-15T18:30:00Z", reason: "quota" },
		];
	});

	it("renders cooldown state entries", () => {
		render(<MonitoringPanel />);

		const row1 = screen.getByTestId("providercooldown-state-row-key-abc-123");
		expect(row1).toBeTruthy();
		expect(row1.textContent).toContain("openai");
		expect(row1.textContent).toContain("key-abc-123 (prod-openai-key)");

		const row2 = screen.getByTestId("providercooldown-state-row-key-def-456");
		expect(row2).toBeTruthy();
		expect(row2.textContent).toContain("anthropic");
		expect(row2.textContent).toContain("key-def-456 (prod-anthropic-key)");
	});

	it("renders per-entry reason badges", () => {
		render(<MonitoringPanel />);

		expect(screen.getByTestId("providercooldown-state-row-key-abc-123-kind")).toBeTruthy();
		expect(screen.getByTestId("providercooldown-state-row-key-def-456-kind")).toBeTruthy();
	});

	it("triggers unfreeze mutation when unfreeze button is clicked", () => {
		render(<MonitoringPanel />);
		fireEvent.click(screen.getByTestId("providercooldown-state-row-key-abc-123-unfreeze"));
		expect(mocks.unfreeze).toHaveBeenCalledWith({ provider: "openai", keyId: "key-abc-123" });
	});

	it("unfreeze button for second entry uses correct provider/keyId", () => {
		render(<MonitoringPanel />);
		fireEvent.click(screen.getByTestId("providercooldown-state-row-key-def-456-unfreeze"));
		expect(mocks.unfreeze).toHaveBeenCalledWith({ provider: "anthropic", keyId: "key-def-456" });
	});
});

describe("MonitoringPanel — empty state", () => {
	beforeEach(() => {
		mocks.unfreeze.mockReset();
		mocks.stateData = [];
		mocks.statsData = { markCount: 0, suppressedCount: 0, activeCount: 0 } as any;
	});

	it("renders an empty state message when no keys are cooling down", () => {
		render(<MonitoringPanel />);
		expect(screen.getByText(/no .*cooldown/i)).toBeTruthy();
	});

	it("renders zero stats cards when no cooldown activity has occurred", () => {
		render(<MonitoringPanel />);
		expect(screen.getByTestId("providercooldown-stats-rate_limit-mark").textContent).toContain("0");
		expect(screen.getByTestId("providercooldown-stats-rate_limit-suppressed").textContent).toContain("0");
		expect(screen.getByTestId("providercooldown-stats-quota-mark").textContent).toContain("0");
		expect(screen.getByTestId("providercooldown-stats-quota-suppressed").textContent).toContain("0");
		expect(screen.getByTestId("providercooldown-stats-active").textContent).toContain("0");
	});
});