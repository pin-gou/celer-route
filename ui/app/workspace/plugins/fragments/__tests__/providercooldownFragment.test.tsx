// @vitest-environment jsdom
/**
 * @file TDD Red Phase — ProvidercooldownFragment tests (dev.ui task 11.1)
 *
 * Contract (design.md "组件设计"):
 *   - ui/app/workspace/plugins/fragments/providercooldownFragment.tsx exports:
 *       ProvidercooldownFragment (default + named), EnabledSwitch, ConfigForm, MonitoringPanel
 *   - ui/lib/types/plugins.ts exports providerCooldownConfigSchema enforced by
 *     the fragment's react-hook-form + zod form:
 *       default_ttl_seconds: integer >= 1 && <= 86400
 *       ttl_overrides:       Record<string, number >= 1>
 *       quota_patterns:      array of non-empty strings, minItems >= 1
 *   - EnabledSwitch on change → useUpdatePluginMutation({ name: "provider-cooldown", data: { enabled } })
 *   - ConfigForm submit     → useUpdatePluginMutation({ name: "provider-cooldown", data: { config } })
 *
 * In the TDD red phase neither the fragment module nor the schema export exist
 * yet — this file is expected to fail at load time ("Failed to resolve import").
 * This is the expected result — the dev phase will implement the component.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Red phase: these imports reference code that does not exist yet.
// Vitest will error with "Failed to resolve import" — this is the expected
// TDD red-phase result. The dev phase implements these modules.
// ---------------------------------------------------------------------------
import { ProvidercooldownFragment, EnabledSwitch, ConfigForm } from "../providercooldownFragment";
import { providerCooldownConfigSchema, type Plugin } from "@/lib/types/plugins";

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

const mocks = vi.hoisted(() => ({
	updatePlugin: vi.fn(),
	unfreeze: vi.fn(),
	// Per-test mutable holders for query hook data
	stateData: [] as Array<{ provider: string; keyId: string; expireAt: string; reason: string }>,
	statsData: { markCount: 0, suppressedCount: 0, activeCount: 0 },
}));

vi.mock("@/lib/store/apis/pluginsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/pluginsApi")>();
	return {
		...actual,
		useUpdatePluginMutation: () => [mocks.updatePlugin, { isLoading: false }],
		useGetCooldownStateQuery: () => ({ data: { state: mocks.stateData }, isLoading: false }),
		useGetCooldownStatsQuery: () => ({ data: { stats: mocks.statsData }, isLoading: false }),
		useUnfreezeCooldownMutation: () => [mocks.unfreeze, { isLoading: false }],
		useGetPluginsQuery: () => ({ data: [], isLoading: false }),
	};
});

vi.mock("@/lib/rbac", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/rbac")>();
	return {
		...actual,
		// Allow all plugin operations in tests
		useRbac: () => true,
	};
});

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const mockPlugin: Plugin = {
	name: "provider-cooldown",
	actualName: "provider-cooldown",
	enabled: true,
	isCustom: false,
	config: {
		default_ttl_seconds: 600,
		ttl_overrides: { openai: 300 },
		quota_patterns: ["insufficient_quota", "quota exceeded"],
	},
	status: {
		name: "provider-cooldown",
		status: "active",
		logs: [],
		types: ["llm"],
	},
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ProvidercooldownFragment — 3-field form rendering (task 11.1)", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
		mocks.unfreeze.mockReset();
		mocks.stateData = [];
		mocks.statsData = { markCount: 0, suppressedCount: 0, activeCount: 0 };
	});

	// -----------------------------------------------------------------------
	// 3 fields render with plugin-configured values
	// -----------------------------------------------------------------------

	it("renders default_ttl_seconds input with the plugin's configured value", () => {
		render(<ProvidercooldownFragment plugin={mockPlugin} />);

		const input = screen.getByTestId("providercooldown-field-default-ttl") as HTMLInputElement;
		expect(input.value).toBe("600");
	});

	it("renders ttl_overrides editor with configured provider entries", () => {
		render(<ProvidercooldownFragment plugin={mockPlugin} />);

		const overrideValue = screen.getByTestId("providercooldown-field-ttl-overrides-value-openai") as HTMLInputElement;
		expect(overrideValue.value).toBe("300");
	});

	it("renders quota_patterns list editor with configured patterns", () => {
		render(<ProvidercooldownFragment plugin={mockPlugin} />);

		const patterns = screen.getAllByTestId(/^providercooldown-field-quota-patterns-/);
		expect(patterns.length).toBe(2);

		expect((patterns[0] as HTMLInputElement).value).toBe("insufficient_quota");
		expect((patterns[1] as HTMLInputElement).value).toBe("quota exceeded");
	});
});

describe("providerCooldownConfigSchema — zod validation (task 11.1)", () => {
	// -----------------------------------------------------------------------
	// Positive control
	// -----------------------------------------------------------------------

	it("accepts a valid config with all three fields", () => {
		const result = providerCooldownConfigSchema.safeParse({
			default_ttl_seconds: 600,
			ttl_overrides: { openai: 300 },
			quota_patterns: ["insufficient_quota"],
		});
		expect(result.success).toBe(true);
	});

	// -----------------------------------------------------------------------
	// default_ttl_seconds negative
	// -----------------------------------------------------------------------

	it("rejects default_ttl_seconds when negative", () => {
		const result = providerCooldownConfigSchema.safeParse({
			default_ttl_seconds: -10,
			ttl_overrides: { openai: 300 },
			quota_patterns: ["insufficient_quota"],
		});
		expect(result.success).toBe(false);
	});

	it("rejects default_ttl_seconds when zero (minimum is 1)", () => {
		const result = providerCooldownConfigSchema.safeParse({
			default_ttl_seconds: 0,
			ttl_overrides: { openai: 300 },
			quota_patterns: ["insufficient_quota"],
		});
		expect(result.success).toBe(false);
	});

	// -----------------------------------------------------------------------
	// ttl_overrides negative
	// -----------------------------------------------------------------------

	it("rejects ttl_overrides with a negative value", () => {
		const result = providerCooldownConfigSchema.safeParse({
			default_ttl_seconds: 600,
			ttl_overrides: { openai: -5 },
			quota_patterns: ["insufficient_quota"],
		});
		expect(result.success).toBe(false);
	});

	it("rejects ttl_overrides with a zero value (minimum is 1)", () => {
		const result = providerCooldownConfigSchema.safeParse({
			default_ttl_seconds: 600,
			ttl_overrides: { openai: 0 },
			quota_patterns: ["insufficient_quota"],
		});
		expect(result.success).toBe(false);
	});

	// -----------------------------------------------------------------------
	// quota_patterns empty array
	// -----------------------------------------------------------------------

	it("rejects quota_patterns when empty array", () => {
		const result = providerCooldownConfigSchema.safeParse({
			default_ttl_seconds: 600,
			ttl_overrides: { openai: 300 },
			quota_patterns: [],
		});
		expect(result.success).toBe(false);
	});

	it("rejects quota_patterns when it contains an empty string", () => {
		const result = providerCooldownConfigSchema.safeParse({
			default_ttl_seconds: 600,
			ttl_overrides: { openai: 300 },
			quota_patterns: [""],
		});
		expect(result.success).toBe(false);
	});
});

describe("EnabledSwitch — toggle triggers mutation (task 11.1)", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
	});

	it("renders the switch reflecting the plugin's enabled state", () => {
		render(<EnabledSwitch plugin={mockPlugin} />);

		const switchEl = screen.getByTestId("providercooldown-enabled-switch");
		expect(switchEl.getAttribute("data-state")).toBe("checked");
	});

	it("triggers useUpdatePluginMutation with enabled=false when switched off", () => {
		render(<EnabledSwitch plugin={mockPlugin} />);

		fireEvent.click(screen.getByTestId("providercooldown-enabled-switch"));
		expect(mocks.updatePlugin).toHaveBeenCalledWith({
			name: "provider-cooldown",
			data: { enabled: false },
		});
	});
});

describe("ConfigForm — submit triggers mutation with 3-field config (task 11.1)", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
	});

	it("submits the 3-field config through useUpdatePluginMutation", async () => {
		render(<ConfigForm plugin={mockPlugin} />);

		// Change default_ttl_seconds from 600 → 300
		const ttlInput = screen.getByTestId("providercooldown-field-default-ttl") as HTMLInputElement;
		fireEvent.change(ttlInput, { target: { value: "300" } });

		// Click Save button
		const saveBtn = screen.getByRole("button", { name: /save/i });
		fireEvent.click(saveBtn);

		// Assert mutation was called with a config containing all 3 fields
		await waitFor(() => {
			expect(mocks.updatePlugin).toHaveBeenCalledWith(
				expect.objectContaining({
					name: "provider-cooldown",
					data: expect.objectContaining({
						config: {
							default_ttl_seconds: 300,
							ttl_overrides: { openai: 300 },
							quota_patterns: ["insufficient_quota", "quota exceeded"],
						},
					}),
				}),
			);
		});
	});
});