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
import { render, screen, fireEvent } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Red phase: these imports reference code that does not exist yet.
// Vitest will error with "Failed to resolve import" — this is the expected
// TDD red-phase result. The dev phase implements these modules.
// ---------------------------------------------------------------------------
import { ProvidercooldownFragment, EnabledSwitch } from "../providercooldownFragment";
import { type Plugin } from "@/lib/types/plugins";

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

const mocks = vi.hoisted(() => ({
	updatePlugin: vi.fn(),
	unfreeze: vi.fn(),
	navigate: vi.fn(),
	// Per-test mutable holders for query hook data
	stateData: [] as Array<{ provider: string; keyId: string; expireAt: string; reason: string }>,
	statsData: { markCount: 0, suppressedCount: 0, activeCount: 0 } as any,
	providersData: [] as Array<{ name: string }>,
}));

vi.mock("@tanstack/react-router", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@tanstack/react-router")>();
	return { ...actual, useNavigate: () => mocks.navigate };
});

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

vi.mock("@/lib/store/apis/providersApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/providersApi")>();
	return {
		...actual,
		useGetProvidersQuery: () => ({ data: mocks.providersData, isLoading: false }),
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

describe("PerProviderPolicyOverview — renders customised policies and edit jump (task 11.2)", () => {
	beforeEach(() => {
		mocks.providersData = [];
	});

	it("renders customised policies and navigates to provider page on edit", () => {
		mocks.navigate.mockReset();

		mocks.providersData = [
			{
				name: "sensenova",
				network_config: {},
				concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 },
				cooldown_policy: {
					rate_limit: {
						match: [{ status_code: 429 }, { message_contains: ["http error 429"] }],
						match_mode: "any",
						ttl_seconds: 30,
					},
					quota: {
						match: [{ message_contains: ["workspace allocated quota"] }],
						ttl_seconds: 600,
					},
				},
			} as never,
			{ name: "openai", network_config: {}, concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 } } as never,
		];
		render(<ProvidercooldownFragment plugin={mockPlugin} />);

		expect(screen.getByText(/per-provider cooldown policies/i)).toBeTruthy();
		// Sensational custom policy visible
		expect(screen.getByTestId("providercooldown-policy-row-sensenova")).toBeTruthy();
		// Edit button on the customised row navigates to the provider page deep-linking
		// into the cooldown policy section.
		const editBtn = screen.getByTestId("providercooldown-policy-edit-sensenova");
		fireEvent.click(editBtn);
		expect(mocks.navigate).toHaveBeenCalledWith({
			to: "/workspace/providers/$id",
			params: { id: "sensenova" },
			search: { tab: "overview", editing: "cooldown-policy" },
		});
		// Default-policy providers get their own row with a "Configure" jump link
		expect(screen.getByTestId("providercooldown-policy-goto-openai")).toBeTruthy();
		// openai appears in the default-policy list; just verify the list rendered
		// at least one provider name from ProviderLabels.
		const providerLabels = screen.getAllByText("OpenAI");
		expect(providerLabels.length).toBeGreaterThan(0);

		// The default-policy row's link navigates to the provider page too
		mocks.navigate.mockReset();
		const gotoBtn = screen.getByTestId("providercooldown-policy-goto-openai");
		fireEvent.click(gotoBtn);
		expect(mocks.navigate).toHaveBeenCalledWith({
			to: "/workspace/providers/$id",
			params: { id: "openai" },
			search: { tab: "overview", editing: "cooldown-policy" },
		});
	});

	it("shows scope label on every rule (default key scope is not hidden)", () => {
		mocks.providersData = [
			{
				name: "sensenova",
				network_config: {},
				concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 },
				cooldown_policy: {
					rate_limit: {
						match: [
							{ status_code: 429 },
							{ message_contains: ["http error 429", "rate_limit_error"] },
							{ type: ["rate_limit_error"] },
							{ code: ["insufficient_balance"] },
							{ message_contains: ["insufficient_balance"] },
						],
						match_mode: "any",
						ttl_seconds: 15,
					},
					quota: {
						match: [{ message_contains: ["workspace allocated quota"] }],
						match_mode: "any",
						ttl_seconds: 20,
						scope: "model",
					},
				},
			} as never,
		];
		render(<ProvidercooldownFragment plugin={mockPlugin} />);

		const row = screen.getByTestId("providercooldown-policy-row-sensenova");
		// rate_limit rule (scope omitted → resolves to key) must surface "per-key"
		expect(row.textContent).toContain("per-key");
		// quota rule (scope=model) must surface "per-model"
		expect(row.textContent).toContain("per-model");
		// TTL numbers from each rule must still be visible
		expect(row.textContent).toContain("15");
		expect(row.textContent).toContain("20");
	});

	it("renders per-provider kind stats from useGetCooldownStatsQuery", () => {
		mocks.providersData = [
			{ name: "sensenova", network_config: {}, concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 } } as never,
		];
		mocks.statsData = {
			markCount: 10,
			suppressedCount: 6,
			activeCount: 2,
			byKind: { rate_limit: { markCount: 3, suppressedCount: 2 }, quota: { markCount: 7, suppressedCount: 4 } },
			perProvider: {
				sensenova: {
					rate_limit: { markCount: 3, suppressedCount: 2 },
					quota: { markCount: 7, suppressedCount: 4 },
				},
			},
		};
		render(<ProvidercooldownFragment plugin={mockPlugin} />);
		const statsEl = screen.getByTestId("providercooldown-policy-stats-sensenova");
		expect(statsEl).toBeTruthy();
		expect(statsEl.textContent).toContain("3");
		expect(statsEl.textContent).toContain("2");
		expect(statsEl.textContent).toContain("7");
		expect(statsEl.textContent).toContain("4");
	});

	it("renders the key-scope remainder when per-model breakdown totals do not match the provider total", () => {
		// Reproduces the sensenova case: rate_limit stays at key scope (so
		// perProviderModel has no rate_limit row), and quota is split
		// (model=deepseek-v4-flash carries 300/1608, the rest is key scope).
		mocks.providersData = [
			{
				name: "sensenova",
				network_config: {},
				concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 },
				cooldown_policy: {
					rate_limit: { match: [{ status_code: 429 }], ttl_seconds: 30 },
					quota: {
						match: [{ message_contains: ["workspace allocated quota"] }],
						ttl_seconds: 600,
						scope: "model",
					},
				},
			} as never,
		];
		mocks.statsData = {
			markCount: 1236,
			suppressedCount: 4112,
			activeCount: 0,
			byKind: {
				rate_limit: { markCount: 932, suppressedCount: 2482 },
				quota: { markCount: 304, suppressedCount: 1630 },
			},
			perProvider: {
				sensenova: {
					rate_limit: { markCount: 932, suppressedCount: 2482 },
					quota: { markCount: 304, suppressedCount: 1630 },
				},
			},
			perProviderModel: {
				sensenova: {
					"deepseek-v4-flash": {
						rate_limit: { markCount: 0, suppressedCount: 0 },
						quota: { markCount: 300, suppressedCount: 1608 },
					},
				},
			},
			perProviderScopeKey: {
				sensenova: {
					rate_limit: { markCount: 932, suppressedCount: 2482 },
					quota: { markCount: 4, suppressedCount: 22 },
				},
			},
			perProviderScopeModel: {
				sensenova: {
					rate_limit: { markCount: 0, suppressedCount: 0 },
					quota: { markCount: 300, suppressedCount: 1608 },
				},
			},
		};
		render(<ProvidercooldownFragment plugin={mockPlugin} />);

		// The model breakdown row carries the per-model rate_limit/quota counts.
		const modelRow = screen.getByTestId("providercooldown-policy-model-stats-sensenova-deepseek-v4-flash");
		expect(modelRow.textContent).toContain("deepseek-v4-flash");
		expect(modelRow.textContent).toContain("0");

		// The key-scope remainder row explains the gap: rate_limit 全是
		// key-scope (932/2482), quota 有 4/22 走 key-scope, 300/1608 走
		// model-scope.
		const remainderRow = screen.getByTestId("providercooldown-policy-model-stats-sensenova-key-scope-remainder");
		expect(remainderRow).toBeTruthy();
		expect(remainderRow.textContent).toContain("932");
		expect(remainderRow.textContent).toContain("2482");
		expect(remainderRow.textContent).toContain("4");
		expect(remainderRow.textContent).toContain("22");
	});

	it("does not render the key-scope remainder when per-model breakdown matches the provider total", () => {
		// All-classified model-scope policy: every per-model bucket sums
		// to the perProvider total — no gap, no remainder row.
		mocks.providersData = [
			{ name: "openai", network_config: {}, concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 } } as never,
		];
		mocks.statsData = {
			markCount: 5,
			suppressedCount: 3,
			activeCount: 0,
			perProvider: {
				openai: {
					rate_limit: { markCount: 5, suppressedCount: 3 },
					quota: { markCount: 0, suppressedCount: 0 },
				},
			},
			perProviderModel: {
				openai: {
					"gpt-4o": {
						rate_limit: { markCount: 5, suppressedCount: 3 },
						quota: { markCount: 0, suppressedCount: 0 },
					},
				},
			},
		};
		render(<ProvidercooldownFragment plugin={mockPlugin} />);

		expect(screen.queryByTestId("providercooldown-policy-model-stats-openai-key-scope-remainder")).toBeNull();
	});
});