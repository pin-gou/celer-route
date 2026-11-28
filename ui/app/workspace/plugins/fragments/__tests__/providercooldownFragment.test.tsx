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
import { ProvidercooldownFragment, EnabledSwitch, ConfigForm, LegacyCapacityDefaultAlert } from "../providercooldownFragment";
import { CapacityUsageTable } from "../providercooldownCapacityUsageTable";
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
	capacityUsageData: { plugin: "provider-cooldown", count: 0, entries: [] } as {
		plugin: string;
		count: number;
		entries: Array<Record<string, unknown>>;
	},
	capacityUsageError: undefined as unknown,
	capacityUsageLoading: false,
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
		useGetCapacityUsageQuery: () => ({
			data: mocks.capacityUsageError ? undefined : mocks.capacityUsageData,
			error: mocks.capacityUsageError,
			isLoading: mocks.capacityUsageLoading,
		}),
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
		mocks.capacityUsageData = { plugin: "provider-cooldown", count: 0, entries: [] };
		mocks.capacityUsageError = undefined;
		mocks.capacityUsageLoading = false;
	});

	it("renders customised policies and navigates to the provider cooldown tab", () => {
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
		// Every fleet row deep-links straight into the provider's cooldown tab.
		const configureBtn = screen.getByTestId("providercooldown-configure-sensenova");
		fireEvent.click(configureBtn);
		expect(mocks.navigate).toHaveBeenCalledWith({
			to: "/workspace/providers/$id",
			params: { id: "sensenova" },
			search: { tab: "cooldown" },
		});
		// Default-policy providers get their own row with the same jump button
		expect(screen.getByTestId("providercooldown-configure-openai")).toBeTruthy();
		// openai appears in the default-policy list; just verify the list rendered
		// at least one provider name from ProviderLabels.
		const providerLabels = screen.getAllByText("OpenAI");
		expect(providerLabels.length).toBeGreaterThan(0);

		mocks.navigate.mockReset();
		const gotoBtn = screen.getByTestId("providercooldown-configure-openai");
		fireEvent.click(gotoBtn);
		expect(mocks.navigate).toHaveBeenCalledWith({
			to: "/workspace/providers/$id",
			params: { id: "openai" },
			search: { tab: "cooldown" },
		});
	});

	it("shows the four capacity gate states on the fleet rows", () => {
		mocks.navigate.mockReset();
		mocks.providersData = [
			{
				name: "openai",
				network_config: {},
				concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 },
			} as never,
			{
				name: "anthropic",
				network_config: {},
				concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 },
				cooldown_policy: { capacity: { enabled: false } },
			} as never,
			{
				name: "gemini",
				network_config: {},
				concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 },
				cooldown_policy: { capacity: { enabled: true, token_limit: 500, window_seconds: 30 } },
			} as never,
		];
		render(
			<ProvidercooldownFragment
				plugin={{
					...mockPlugin,
					config: { ...mockPlugin.config, capacity: { enabled: true, token_limit: 1000, window_seconds: 60 } },
				}}
			/>,
		);

		// No capacity override → the deprecated plugin-level default applies.
		const inherited = screen.getByTestId("providercooldown-capacity-status-openai");
		expect(inherited.textContent).toContain("Using the legacy global default");
		expect(inherited.textContent).toContain("1,000");

		// Explicitly off.
		const disabled = screen.getByTestId("providercooldown-capacity-status-anthropic");
		expect(disabled.textContent).toContain("Off");
		expect(disabled.textContent).not.toContain("1,000");

		// Own capacity section → active with its own effective limit.
		const customised = screen.getByTestId("providercooldown-capacity-status-gemini");
		expect(customised.textContent).toContain("Active");
		expect(customised.textContent).toContain("500");
	});

	it("labels a provider with no capacity section and no legacy default as not configured", () => {
		mocks.providersData = [
			{ name: "openai", network_config: {}, concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 } } as never,
		];
		render(<ProvidercooldownFragment plugin={{ ...mockPlugin, config: {} }} />);

		const inherited = screen.getByTestId("providercooldown-capacity-status-openai");
		expect(inherited.textContent).toContain("Not configured");
		expect(inherited.textContent).not.toContain("legacy");
		// No legacy cap is in force, so the "inheriting the global default" hint is hidden.
		expect(screen.queryByTestId("providercooldown-capacity-legacy-hint")).toBeNull();
	});

	it("treats a disabled legacy default as not configured", () => {
		mocks.providersData = [
			{ name: "openai", network_config: {}, concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 } } as never,
		];
		render(
			<ProvidercooldownFragment
				plugin={{ ...mockPlugin, config: { ...mockPlugin.config, capacity: { enabled: false, token_limit: 1000 } } }}
			/>,
		);

		expect(screen.getByTestId("providercooldown-capacity-status-openai").textContent).toContain("Not configured");
		expect(screen.queryByTestId("providercooldown-capacity-legacy-hint")).toBeNull();
	});

	it("warns about providers inheriting an enabled legacy capacity default", () => {
		mocks.providersData = [
			{ name: "openai", network_config: {}, concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 } } as never,
			{
				name: "gemini",
				network_config: {},
				concurrency_and_buffer_size: { concurrency: 1, buffer_size: 1 },
				cooldown_policy: { capacity: { enabled: true, token_limit: 500 } },
			} as never,
		];
		render(
			<ProvidercooldownFragment
				plugin={{
					...mockPlugin,
					config: { ...mockPlugin.config, capacity: { enabled: true, token_limit: 1000, window_seconds: 60 } },
				}}
			/>,
		);

		// Only openai inherits the default; gemini carries its own section.
		const hint = screen.getByTestId("providercooldown-capacity-legacy-hint");
		expect(hint.textContent).toContain("1 provider");
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

// ---------------------------------------------------------------------------
// ConfigForm — fleet default policies (escalation + learning only)
//
// Contract: ConfigForm submit → useUpdatePluginMutation({
//   name: "provider-cooldown",
//   data: { enabled: <plugin.enabled>, config: { ...currentConfig, escalation, learning } },
// })
//
// `capacity` is intentionally absent from the form and from the payload: the
// spread keeps whatever is stored, so saving the default policies never
// overwrites the legacy plugin-level capacity default.
// ---------------------------------------------------------------------------

/** Build a plugin fixture whose `config` carries the pre-existing top-level keys. */
const configFormFixture = (configOverrides: Record<string, unknown> = {}): Plugin => ({
	...mockPlugin,
	config: {
		default_ttl_seconds: 600,
		ttl_overrides: { openai: 300 },
		quota_patterns: ["insufficient_quota"],
		...configOverrides,
	},
});

/** The single updatePlugin argument captured by the mocked mutation. */
const submittedPluginUpdate = () =>
	mocks.updatePlugin.mock.calls[0][0] as {
		name: string;
		data: { enabled: boolean; config: Record<string, unknown> };
	};

describe("ConfigForm — fleet default policies", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
		// Full success path: the fragment calls `.unwrap()` on the mutation promise.
		mocks.updatePlugin.mockImplementation(() => ({ unwrap: () => Promise.resolve({}) }));
		mocks.capacityUsageData = { plugin: "provider-cooldown", count: 0, entries: [] };
		mocks.capacityUsageError = undefined;
		mocks.capacityUsageLoading = false;
	});

	it("presents only the mechanism-level defaults", () => {
		mocks.providersData = [];
		render(<ProvidercooldownFragment plugin={configFormFixture()} />);

		expect(screen.getByText("Fleet default policies")).toBeTruthy();
		expect(screen.getByText(/capacity gate has no fleet default/i)).toBeTruthy();
		// The capacity form is gone from the plugin page entirely.
		expect(screen.queryByTestId("providercooldown-capacity-enabled")).toBeNull();
		expect(screen.queryByTestId("providercooldown-capacity-token-limit")).toBeNull();
	});

	it("renders both section switches and their fields", () => {
		render(<ConfigForm plugin={configFormFixture()} />);

		expect(screen.getByTestId("providercooldown-config-form")).toBeTruthy();

		// Escalation
		expect(screen.getByTestId("providercooldown-escalation-enabled")).toBeTruthy();
		expect(screen.getByTestId("providercooldown-escalation-max-multiplier")).toBeTruthy();

		// Capacity is no longer editable plugin-wide.
		expect(screen.queryByTestId("providercooldown-capacity-enabled")).toBeNull();
		expect(screen.queryByTestId("providercooldown-capacity-token-limit")).toBeNull();
		expect(screen.queryByTestId("providercooldown-capacity-headroom")).toBeNull();

		// Learning
		expect(screen.getByTestId("providercooldown-learning-enabled")).toBeTruthy();
		expect(screen.getByTestId("providercooldown-learning-window-seconds")).toBeTruthy();
		expect(screen.getByTestId("providercooldown-learning-max-samples")).toBeTruthy();
		expect(screen.getByTestId("providercooldown-learning-min-samples")).toBeTruthy();

		// Section off by default → its params are greyed out but still mounted.
		expect((screen.getByTestId("providercooldown-escalation-max-multiplier") as HTMLInputElement).disabled).toBe(true);
		expect((screen.getByTestId("providercooldown-learning-max-samples") as HTMLInputElement).disabled).toBe(true);
	});

	it("submits escalation { enabled: true, max_multiplier: 8 } and keeps the plugin enabled", async () => {
		render(<ConfigForm plugin={configFormFixture({ default_ttl_seconds: 120 })} />);

		fireEvent.click(screen.getByTestId("providercooldown-escalation-enabled"));
		fireEvent.change(screen.getByTestId("providercooldown-escalation-max-multiplier"), { target: { value: "8" } });
		fireEvent.click(screen.getByTestId("providercooldown-config-save"));

		await waitFor(() => expect(mocks.updatePlugin).toHaveBeenCalledTimes(1));

		const { name, data } = submittedPluginUpdate();
		expect(name).toBe("provider-cooldown");
		// The plugin must not be switched off by a config save.
		expect(data.enabled).toBe(true);
		expect(data.config.escalation).toEqual({ enabled: true, max_multiplier: 8 });
	});

	it("preserves the other top-level keys of the stored plugin config", async () => {
		render(<ConfigForm plugin={configFormFixture({ default_ttl_seconds: 120 })} />);

		fireEvent.click(screen.getByTestId("providercooldown-escalation-enabled"));
		fireEvent.click(screen.getByTestId("providercooldown-config-save"));

		await waitFor(() => expect(mocks.updatePlugin).toHaveBeenCalledTimes(1));

		const config = submittedPluginUpdate().data.config;
		expect(config.default_ttl_seconds).toBe(120);
		expect(config.ttl_overrides).toEqual({ openai: 300 });
		expect(config.quota_patterns).toEqual(["insufficient_quota"]);
	});

	it("does not send a capacity key and preserves a stored legacy capacity section verbatim", async () => {
		const legacyCapacity = {
			enabled: true,
			token_limit: 1000,
			request_limit: 50,
			window_seconds: 60,
			headroom: 0.9,
			scope: "provider",
			action: "fallback",
		};
		render(<ConfigForm plugin={configFormFixture({ capacity: legacyCapacity })} />);

		fireEvent.click(screen.getByTestId("providercooldown-learning-enabled"));
		fireEvent.click(screen.getByTestId("providercooldown-config-save"));

		await waitFor(() => expect(mocks.updatePlugin).toHaveBeenCalledTimes(1));

		const config = submittedPluginUpdate().data.config;
		// Untouched: the form never authored this key, the spread carried it over.
		expect(config.capacity).toEqual(legacyCapacity);
	});

	it("preserves a stored escalation.chronic block when only max_multiplier is edited", async () => {
		const chronic = {
			enabled: true,
			success_rate_threshold: 0.3,
			min_samples: 10,
			chronic_max_ttl_seconds: 1800,
			jitter_ratio: 0.2,
			recovery_successes: 3,
		};
		render(<ConfigForm plugin={configFormFixture({ escalation: { enabled: true, max_multiplier: 4, chronic } })} />);

		fireEvent.change(screen.getByTestId("providercooldown-escalation-max-multiplier"), { target: { value: "6" } });
		fireEvent.click(screen.getByTestId("providercooldown-config-save"));

		await waitFor(() => expect(mocks.updatePlugin).toHaveBeenCalledTimes(1));

		// The form has no chronic editor, but the section it submits must not
		// silently drop the block: next save would otherwise erase it.
		expect(submittedPluginUpdate().data.config.escalation).toEqual({ enabled: true, max_multiplier: 6, chronic });
	});

	it("sends { enabled: false } for a section that is switched off instead of omitting it", async () => {
		render(<ConfigForm plugin={configFormFixture()} />);

		fireEvent.click(screen.getByTestId("providercooldown-escalation-enabled"));
		fireEvent.click(screen.getByTestId("providercooldown-config-save"));

		await waitFor(() => expect(mocks.updatePlugin).toHaveBeenCalledTimes(1));

		const config = submittedPluginUpdate().data.config;
		expect((config.escalation as { enabled: boolean }).enabled).toBe(true);

		// The disabled learning section is explicitly present with `enabled: false`.
		expect("learning" in config).toBe(true);
		expect(config.learning).toEqual({
			enabled: false,
			window_seconds: 60,
			max_samples: 50,
			min_samples: 5,
		});
	});

	it("keeps the edited values of a section that is switched back off", async () => {
		render(<ConfigForm plugin={configFormFixture()} />);

		const learningSwitch = screen.getByTestId("providercooldown-learning-enabled");
		fireEvent.click(learningSwitch); // on
		fireEvent.change(screen.getByTestId("providercooldown-learning-max-samples"), { target: { value: "120" } });
		fireEvent.click(learningSwitch); // off again

		fireEvent.click(screen.getByTestId("providercooldown-config-save"));

		await waitFor(() => expect(mocks.updatePlugin).toHaveBeenCalledTimes(1));

		const learning = submittedPluginUpdate().data.config.learning as { enabled: boolean; max_samples: number };
		expect(learning.enabled).toBe(false);
		expect(learning.max_samples).toBe(120);
	});

	it("does not submit when max_multiplier is out of range", async () => {
		render(<ConfigForm plugin={configFormFixture()} />);

		fireEvent.click(screen.getByTestId("providercooldown-escalation-enabled"));
		fireEvent.change(screen.getByTestId("providercooldown-escalation-max-multiplier"), { target: { value: "99" } });

		// The save button is enabled (the form is dirty) — the block must come
		// from zod, not from a disabled button.
		const saveButton = screen.getByTestId("providercooldown-config-save") as HTMLButtonElement;
		expect(saveButton.disabled).toBe(false);
		fireEvent.click(saveButton);

		// zod rejects > 16, so the resolver marks the section invalid and RHF
		// never invokes onSubmit.
		await waitFor(() => expect(screen.getByTestId("providercooldown-config-errors")).toBeTruthy());
		expect(mocks.updatePlugin).not.toHaveBeenCalled();
	});

	it("does not submit when the learning sample bounds are out of range", async () => {
		render(<ConfigForm plugin={configFormFixture()} />);

		fireEvent.click(screen.getByTestId("providercooldown-learning-enabled"));
		fireEvent.change(screen.getByTestId("providercooldown-learning-min-samples"), { target: { value: "500" } });

		const saveButton = screen.getByTestId("providercooldown-config-save") as HTMLButtonElement;
		expect(saveButton.disabled).toBe(false);
		fireEvent.click(saveButton);

		await waitFor(() => expect(screen.getByTestId("providercooldown-config-errors")).toBeTruthy());
		expect(mocks.updatePlugin).not.toHaveBeenCalled();
	});
});
// ---------------------------------------------------------------------------
// LegacyCapacityDefaultAlert — deprecated plugin-level capacity default
// ---------------------------------------------------------------------------

describe("LegacyCapacityDefaultAlert", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
		mocks.updatePlugin.mockImplementation(() => ({ unwrap: () => Promise.resolve({}) }));
	});

	it("is hidden when the stored config has no capacity section", () => {
		render(<LegacyCapacityDefaultAlert plugin={configFormFixture()} />);
		expect(screen.queryByTestId("providercooldown-legacy-capacity-alert")).toBeNull();
	});

	it("is shown with the effective summary when the legacy default gates providers", () => {
		render(
			<LegacyCapacityDefaultAlert
				plugin={configFormFixture({ capacity: { enabled: true, token_limit: 1000, request_limit: 20, window_seconds: 60 } })}
			/>,
		);

		expect(screen.getByTestId("providercooldown-legacy-capacity-alert")).toBeTruthy();
		const summary = screen.getByTestId("providercooldown-legacy-capacity-summary").textContent ?? "";
		expect(summary).toContain("1,000");
		expect(summary).toContain("20");
	});

	it("is shown even when the legacy default is disabled, and says it does not gate", () => {
		render(<LegacyCapacityDefaultAlert plugin={configFormFixture({ capacity: { enabled: false, token_limit: 1000 } })} />);

		expect(screen.getByTestId("providercooldown-legacy-capacity-alert")).toBeTruthy();
		expect(screen.getByTestId("providercooldown-legacy-capacity-summary").textContent).toContain("disabled");
	});

	it("clears only the capacity key, preserving every other config key", async () => {
		render(
			<LegacyCapacityDefaultAlert
				plugin={configFormFixture({ default_ttl_seconds: 120, capacity: { enabled: true, token_limit: 1000 } })}
			/>,
		);

		fireEvent.click(screen.getByTestId("providercooldown-legacy-capacity-clear"));
		fireEvent.click(await screen.findByTestId("providercooldown-legacy-capacity-confirm"));

		await waitFor(() => expect(mocks.updatePlugin).toHaveBeenCalledTimes(1));

		const { name, data } = submittedPluginUpdate();
		expect(name).toBe("provider-cooldown");
		expect(data.enabled).toBe(true);
		expect("capacity" in data.config).toBe(false);
		expect(data.config.default_ttl_seconds).toBe(120);
		expect(data.config.ttl_overrides).toEqual({ openai: 300 });
		expect(data.config.quota_patterns).toEqual(["insufficient_quota"]);
	});
});

// ---------------------------------------------------------------------------
// CapacityUsageTable — read-only fleet water level
// ---------------------------------------------------------------------------

const capacityEntry = (overrides: Record<string, unknown> = {}): Record<string, unknown> => ({
	provider: "openai",
	model: "",
	scope: "provider",
	windowSeconds: 60,
	usedTokens: 1200,
	usedRequests: 12,
	tokenLimit: 2000,
	requestLimit: 20,
	headroom: 0.9,
	thresholdTokens: 1800,
	thresholdRequests: 18,
	retryAfterSeconds: 0,
	source: "provider",
	...overrides,
});

describe("CapacityUsageTable", () => {
	beforeEach(() => {
		mocks.navigate.mockReset();
		mocks.capacityUsageLoading = false;
		mocks.capacityUsageError = undefined;
		mocks.capacityUsageData = { plugin: "provider-cooldown", count: 0, entries: [] };
	});

	it("renders the fleet water level rows", () => {
		mocks.capacityUsageData = {
			plugin: "provider-cooldown",
			count: 2,
			entries: [
				capacityEntry(),
				capacityEntry({
					provider: "gemini",
					model: "gemini-2.5-pro",
					scope: "provider_model",
					retryAfterSeconds: 42,
					source: "default",
				}),
			],
		};
		render(<CapacityUsageTable />);

		expect(screen.getByTestId("providercooldown-capacity-usage-table")).toBeTruthy();

		const providerRow = screen.getByTestId("providercooldown-capacity-usage-row-openai-all");
		expect(providerRow.textContent).toContain("All models");
		expect(providerRow.textContent).toContain("1,200 / 1,800");
		expect(providerRow.textContent).toContain("12 / 18");
		expect(providerRow.textContent).toContain("Normal");
		expect(providerRow.textContent).toContain("This provider");

		const modelRow = screen.getByTestId("providercooldown-capacity-usage-row-gemini-gemini-2.5-pro");
		expect(modelRow.textContent).toContain("gemini-2.5-pro");
		expect(modelRow.textContent).toContain("Engaged, releases in about 42s");
		expect(modelRow.textContent).toContain("Legacy global default");
	});

	it("renders the empty state when no usage has been recorded", () => {
		render(<CapacityUsageTable />);

		expect(screen.getByTestId("providercooldown-capacity-usage-empty")).toBeTruthy();
		expect(screen.queryByTestId("providercooldown-capacity-usage-table")).toBeNull();
	});

	it("hides the whole block when the plugin is not loaded (400)", () => {
		mocks.capacityUsageError = { status: 400, data: "plugin not loaded" };
		const { container } = render(<CapacityUsageTable />);

		expect(container.textContent).toBe("");
		expect(screen.queryByTestId("providercooldown-capacity-usage-empty")).toBeNull();
	});

	it("navigates to the provider's cooldown tab when a provider name is clicked", () => {
		mocks.capacityUsageData = { plugin: "provider-cooldown", count: 1, entries: [capacityEntry()] };
		render(<CapacityUsageTable />);

		fireEvent.click(screen.getByTestId("providercooldown-capacity-usage-provider-openai-all"));

		expect(mocks.navigate).toHaveBeenCalledWith({
			to: "/workspace/providers/$id",
			params: { id: "openai" },
			search: { tab: "cooldown" },
		});
	});
});