// @vitest-environment jsdom
/**
 * CooldownPolicyFormFragment — provider-level cooldown policy, including the
 * three optional capabilities (escalation / capacity / learning).
 *
 * The contract these tests pin down:
 *   - an absent capability section means "inherit": the submitted payload must
 *     NOT contain that key (otherwise inheriting becomes overriding);
 *   - `custom` submits a fully-populated object;
 *   - `off` submits exactly `{ enabled: false }`;
 *   - unknown keys already stored in cooldown_policy survive a save;
 *   - editing only rate_limit/quota must not erase an existing capacity
 *     section (the regression this rewrite fixes).
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
	updateProvider: vi.fn(),
	providerKeys: [
		{ id: "key-1-id", name: "key-1" },
		{ id: "key-2-id", name: "key-2" },
	],
	learnedLimits: [] as Array<Record<string, unknown>>,
	models: [{ name: "gpt-4o" }, { name: "gpt-4o-mini" }] as Array<Record<string, unknown>>,
}));

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string, options?: Record<string, unknown>) => (options ? `${key} ${Object.values(options).join(" ")}` : key),
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
	initReactI18next: { type: "3rdParty", init: () => {} },
}));

vi.mock("@/lib/store", () => ({
	useAppDispatch: () => vi.fn(),
	setProviderFormDirtyState: (dirty: boolean) => ({ type: "test/providerFormDirtyState", payload: dirty }),
	getErrorMessage: (error: unknown) => String(error),
}));

vi.mock("@/lib/store/apis/providersApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/providersApi")>();
	return {
		...actual,
		useUpdateProviderMutation: () => [mocks.updateProvider, { isLoading: false }],
		useGetProviderKeysQuery: () => ({ data: mocks.providerKeys, isLoading: false, error: undefined }),
		// The capacity card fetches the provider's model catalog to spell out the
		// budget count and populate the per-model override picker.
		useGetModelsQuery: () => ({
			data: { models: mocks.models, total: mocks.models.length },
			isLoading: false,
			error: undefined,
		}),
	};
});

vi.mock("@/lib/rbac", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/rbac")>();
	return { ...actual, useRbac: () => true };
});

vi.mock("@/lib/store/apis/errorPatternsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/errorPatternsApi")>();
	return {
		...actual,
		useGetErrorPatternsQuery: () => ({ data: { patterns: [], total_errors: 0 }, isLoading: false, error: undefined }),
	};
});

vi.mock("@/lib/store/apis/providerErrorCatalogApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/providerErrorCatalogApi")>();
	return {
		...actual,
		useGetProviderErrorCatalogQuery: () => ({ data: undefined }),
	};
});

vi.mock("@/lib/store/apis/pluginsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/pluginsApi")>();
	return {
		...actual,
		useGetPluginQuery: () => ({ data: undefined }),
		useGetLearnedLimitsQuery: () => ({
			data: {
				plugin: "provider-cooldown",
				count: mocks.learnedLimits.length,
				windowSeconds: 60,
				limits: mocks.learnedLimits,
			},
			error: undefined,
		}),
	};
});

import { CooldownPolicyFormFragment } from "../cooldownPolicyFormFragment";
import type { CooldownPolicy, ModelProvider } from "@/lib/types/config";

const baseProvider = (cooldownPolicy?: CooldownPolicy): ModelProvider =>
	({
		name: "openai",
		provider_status: "active",
		...(cooldownPolicy === undefined ? {} : { cooldown_policy: cooldownPolicy }),
	}) as ModelProvider;

const RATE_LIMIT_RULE: NonNullable<CooldownPolicy["rate_limit"]> = {
	match: [{ status_code: 429 }],
	match_mode: "any",
	ttl_seconds: 60,
};

const submittedPolicy = () => {
	const call = mocks.updateProvider.mock.calls.at(0);
	if (!call) throw new Error("updateProvider was not called");
	return (call[0] as { cooldown_policy: Record<string, unknown> | null }).cooldown_policy;
};

async function save() {
	const button = screen.getByTestId("provider-cooldown-save-button") as HTMLButtonElement;
	// RHF validates asynchronously after a change; the button stays disabled
	// until that first validation resolves, so wait for it before clicking.
	await waitFor(() => expect(button.disabled).toBe(false));
	fireEvent.click(button);
	await waitFor(() => expect(mocks.updateProvider).toHaveBeenCalledTimes(1));
}

function changeTtl() {
	fireEvent.change(screen.getByTestId("provider-cooldown-rate_limit-ttl-input"), { target: { value: "45" } });
}

function openSection(capability: "escalation" | "capacity" | "learning") {
	fireEvent.click(screen.getByTestId(`providercooldown-${capability}-accordion-trigger`));
}

/**
 * One aggregate estimate and one per-credential estimate for the same
 * (provider, model). The learner reports both levels because the capacity gate
 * can enforce either — and they must never be mixed.
 */
function defaultLearnedLimits() {
	return [
		{
			provider: "openai",
			model: "gpt-4o",
			scope: "provider_model",
			keyName: null,
			sampleCount: 12,
			keyCount: 2,
			estimatedTokenLimit: 5000,
			windowSeconds: 60,
			lowerBound: 4800,
			upperBound: 5200,
			confidence: "high",
			requestSampleCount: 6,
			estimatedRequestLimit: 120,
			requestLowerBound: 100,
			requestUpperBound: 121,
			requestConfidence: "medium",
			limiterDimension: "tpm",
			firstSeen: "2026-01-01T00:00:00Z",
			lastSeen: "2026-01-02T00:00:00Z",
		},
		{
			provider: "openai",
			model: "gpt-4o",
			scope: "key_model",
			keyName: "key-1",
			sampleCount: 10,
			keyCount: 1,
			estimatedTokenLimit: 2000,
			windowSeconds: 60,
			lowerBound: 1900,
			upperBound: 2100,
			confidence: "medium",
			requestSampleCount: 5,
			estimatedRequestLimit: 20,
			requestLowerBound: 19,
			requestUpperBound: 21,
			requestConfidence: "low",
			limiterDimension: "tpm",
			firstSeen: "2026-01-01T00:00:00Z",
			lastSeen: "2026-01-02T00:00:00Z",
		},
	];
}

describe("CooldownPolicyFormFragment — capability tri-state payload", () => {
	beforeEach(() => {
		mocks.updateProvider.mockReset();
		mocks.updateProvider.mockImplementation(() => ({ unwrap: () => Promise.resolve({}) }));
		mocks.learnedLimits = defaultLearnedLimits();
	});

	it("inherits: the payload omits the capability section entirely", async () => {
		render(<CooldownPolicyFormFragment provider={baseProvider()} />);
		changeTtl();

		await save();

		const policy = submittedPolicy();
		expect(policy).not.toHaveProperty("capacity");
		expect(policy).not.toHaveProperty("escalation");
		expect(policy).not.toHaveProperty("learning");
	});

	it("custom: submits a fully-populated capacity object", async () => {
		render(<CooldownPolicyFormFragment provider={baseProvider()} />);
		openSection("capacity");
		fireEvent.click(screen.getByTestId("providercooldown-capacity-card-mode-custom"));
		fireEvent.change(screen.getByTestId("providercooldown-capacity-token-limit"), { target: { value: "1234" } });

		await save();

		expect(submittedPolicy()?.capacity).toEqual({
			enabled: true,
			token_limit: 1234,
			request_limit: 0,
			window_seconds: 60,
			headroom: 0.9,
			scope: "provider",
			action: "fallback",
		});
	});

	it("off: submits exactly { enabled: false }", async () => {
		render(<CooldownPolicyFormFragment provider={baseProvider()} />);
		openSection("escalation");
		fireEvent.click(screen.getByTestId("providercooldown-escalation-card-mode-disabled"));

		await save();

		expect(submittedPolicy()?.escalation).toEqual({ enabled: false });
	});

	it("preserves unknown keys already stored on cooldown_policy", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: RATE_LIMIT_RULE,
					// Simulates a section written by a newer backend that this UI
					// doesn't know about yet.
					future_section: { x: 1 },
				} as CooldownPolicy)}
			/>,
		);
		changeTtl();

		await save();

		expect(submittedPolicy()?.future_section).toEqual({ x: 1 });
	});

	it("editing rate_limit keeps an existing capacity section", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: RATE_LIMIT_RULE,
					capacity: { enabled: true, token_limit: 100, window_seconds: 120 },
					learning: { enabled: false },
				})}
			/>,
		);
		changeTtl();

		await save();

		const policy = submittedPolicy();
		expect(policy?.capacity).toEqual({ enabled: true, token_limit: 100, window_seconds: 120 });
		expect(policy?.learning).toEqual({ enabled: false });
	});

	it("editing rate_limit keeps a nested escalation.chronic block (no editor, must not be dropped)", async () => {
		const chronic = {
			enabled: true,
			success_rate_threshold: 0.3,
			min_samples: 10,
			chronic_max_ttl_seconds: 1800,
			jitter_ratio: 0.2,
			recovery_successes: 3,
		};
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: RATE_LIMIT_RULE,
					escalation: { enabled: true, max_multiplier: 4, chronic },
				})}
			/>,
		);
		changeTtl();

		await save();

		// zod strips unknown keys, so `chronic` must be part of the escalation
		// schema for this to survive — that is exactly what this pins.
		expect(submittedPolicy()?.escalation).toEqual({ enabled: true, max_multiplier: 4, chronic });
	});

	it("echoes an explicit { enabled: false } section back unchanged", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: RATE_LIMIT_RULE,
					capacity: { enabled: false },
				})}
			/>,
		);
		changeTtl();

		await save();

		expect(submittedPolicy()?.capacity).toEqual({ enabled: false });
	});

	it("editing rate_limit keeps rpm/tpm dimension TTLs (zod must not strip them)", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: { ...RATE_LIMIT_RULE, rpm_ttl_seconds: 90, tpm_ttl_seconds: 40 },
				})}
			/>,
		);
		changeTtl();

		await save();

		const rateLimit = submittedPolicy()?.rate_limit as Record<string, unknown>;
		expect(rateLimit.rpm_ttl_seconds).toBe(90);
		expect(rateLimit.tpm_ttl_seconds).toBe(40);
	});

	it("strips dimension TTLs from the quota rule (the backend rejects them there)", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: RATE_LIMIT_RULE,
					quota: {
						match: [{ status_code: 429, message_contains: ["insufficient_quota"] }],
						ttl_seconds: 600,
						rpm_ttl_seconds: 90,
					},
				} as unknown as CooldownPolicy)}
			/>,
		);
		changeTtl();

		await save();

		const quota = submittedPolicy()?.quota as Record<string, unknown>;
		expect(quota).not.toHaveProperty("rpm_ttl_seconds");
		expect(quota).not.toHaveProperty("tpm_ttl_seconds");
		expect(quota.ttl_seconds).toBe(600);
	});

	it("adopting a learned limit prefills both budgets for the matching level", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({ capacity: { enabled: true, scope: "provider_model", window_seconds: 60 } } as unknown as CooldownPolicy)}
			/>,
		);
		openSection("learning");
		fireEvent.click(screen.getByTestId("providercooldown-learning-adopt-gpt-4o"));

		await waitFor(() => expect(screen.getByTestId("providercooldown-capacity-token-limit")).toBeTruthy());
		expect((screen.getByTestId("providercooldown-capacity-token-limit") as HTMLInputElement).value).toBe("5000");
		// The request budget learned from RPM rejections is adopted too.
		expect((screen.getByTestId("providercooldown-capacity-request-limit") as HTMLInputElement).value).toBe("120");

		await save();
		expect(submittedPolicy()?.capacity).toMatchObject({ enabled: true, token_limit: 5000, request_limit: 120 });
	});
});
/**
 * The dimension pin: which upstream budget a match cools.
 *
 * Default is 自动 (no field in the payload), and the editor echoes what the
 * automatic classifier would decide for the predicates already typed. Pinning
 * exists because not every provider names the exhausted budget in its 429.
 */
describe("CooldownPolicyFormFragment — rate-limit dimension pin", () => {
	beforeEach(() => {
		mocks.updateProvider.mockReset();
		mocks.updateProvider.mockImplementation(() => ({ unwrap: () => Promise.resolve({}) }));
	});

	async function openRateLimitMatchEditor() {
		fireEvent.click(screen.getByTestId("provider-cooldown-rate_limit-match-0-edit"));
		await waitFor(() => expect(screen.getByTestId("provider-cooldown-rate_limit-match-0-dimension-trigger")).toBeTruthy());
	}

	it("defaults to auto and echoes what the classifier decides for this match", async () => {
		render(<CooldownPolicyFormFragment provider={baseProvider()} />);
		await openRateLimitMatchEditor();

		const trigger = screen.getByTestId("provider-cooldown-rate_limit-match-0-dimension-trigger");
		expect(trigger.textContent).toContain("matchField.dimensionAuto");

		// The seeded match is a bare [429]: no budget is named, so auto cannot
		// choose a dimension-specific TTL.
		const preview = screen.getByTestId("provider-cooldown-rate_limit-match-0-dimension-preview");
		expect(preview.textContent).toContain("dimensionValue_unknown");
	});

	it("echoes the token-rate decision for a token-shaped predicate", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: {
						match: [{ status_code: 429, code: ["EndpointTPMExceeded"] }],
						match_mode: "any",
						ttl_seconds: 60,
					},
				} as unknown as CooldownPolicy)}
			/>,
		);
		await openRateLimitMatchEditor();

		const preview = screen.getByTestId("provider-cooldown-rate_limit-match-0-dimension-preview");
		expect(preview.textContent).toContain("dimensionValue_tpm");
	});

	it("keeps a stored pin through an unrelated save (zod must not strip it)", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: {
						match: [{ status_code: 429, code: ["EndpointRPMExceeded"], dimension: "rpm" }],
						match_mode: "any",
						ttl_seconds: 60,
					},
				} as unknown as CooldownPolicy)}
			/>,
		);
		changeTtl();

		await save();

		const match = (submittedPolicy()?.rate_limit as Record<string, unknown>)?.match as Array<Record<string, unknown>>;
		expect(match[0].dimension).toBe("rpm");
	});

	it("shows the pin on the collapsed match row", () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: {
						match: [{ status_code: 429, code: ["EndpointRPMExceeded"], dimension: "rpm" }],
						match_mode: "any",
						ttl_seconds: 60,
					},
				} as unknown as CooldownPolicy)}
			/>,
		);

		const badge = screen.getByTestId("provider-cooldown-rate_limit-match-0-dimension-badge");
		expect(badge.textContent).toContain("dimensionValue_rpm");
	});

	it("warns when the pin has no dedicated TTL configured", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: {
						match: [{ status_code: 429, code: ["EndpointRPMExceeded"], dimension: "rpm" }],
						match_mode: "any",
						ttl_seconds: 60,
					},
				} as unknown as CooldownPolicy)}
			/>,
		);
		await openRateLimitMatchEditor();

		expect(screen.getByTestId("provider-cooldown-rate_limit-match-0-dimension-no-ttl")).toBeTruthy();
	});

	it("does not offer a dimension on the quota rule", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: RATE_LIMIT_RULE,
					quota: { match: [{ status_code: 429, message_contains: ["insufficient_quota"] }], ttl_seconds: 600 },
				} as unknown as CooldownPolicy)}
			/>,
		);
		fireEvent.click(screen.getByTestId("provider-cooldown-quota-match-0-edit"));
		await waitFor(() => expect(screen.getByTestId("provider-cooldown-quota-match-0-status-code")).toBeTruthy());

		expect(screen.queryByTestId("provider-cooldown-quota-match-0-dimension-trigger")).toBeNull();
	});
});
/**
 * Learning levels vs the capacity gate's scope, plus the per-key limit editor.
 *
 * The learner reports two levels and the gate enforces exactly one of them, so
 * the panel must only offer the matching level: an aggregate estimate written
 * into a key-scoped gate multiplies the budget by the number of keys.
 */
describe("CooldownPolicyFormFragment — learning scopes and per-key limits", () => {
	beforeEach(() => {
		mocks.updateProvider.mockReset();
		mocks.updateProvider.mockImplementation(() => ({ unwrap: () => Promise.resolve({}) }));
		mocks.learnedLimits = defaultLearnedLimits();
	});

	it("provider scope explains that there is no adoptable estimate", () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({ capacity: { enabled: true, scope: "provider", token_limit: 1000 } } as unknown as CooldownPolicy)}
			/>,
		);
		openSection("learning");

		expect(screen.getByTestId("providercooldown-learning-scope-provider-hint")).toBeTruthy();
		expect(screen.queryByTestId("providercooldown-learning-adopt-gpt-4o")).toBeNull();
	});

	it("key scope adopts a per-credential estimate into that key, not the shared default", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({ capacity: { enabled: true, scope: "key", window_seconds: 60 } } as unknown as CooldownPolicy)}
			/>,
		);
		openSection("learning");

		// The aggregate row is not offered under a key-scoped gate.
		expect(screen.queryByTestId("providercooldown-learning-suggestion-gpt-4o")).toBeNull();
		expect(screen.getByTestId("providercooldown-learning-suggestion-key-1|gpt-4o")).toBeTruthy();

		fireEvent.click(screen.getByTestId("providercooldown-learning-adopt-key-1|gpt-4o"));

		await waitFor(() => expect(screen.getByTestId("providercooldown-capacity-key-limit-token-key-1")).toBeTruthy());
		// The estimate belongs to key-1, so it lands on key-1's override.
		expect((screen.getByTestId("providercooldown-capacity-key-limit-token-key-1") as HTMLInputElement).value).toBe("2000");
		expect((screen.getByTestId("providercooldown-capacity-key-limit-request-key-1") as HTMLInputElement).value).toBe("20");
		// The shared default is untouched: adopting one key's budget must not
		// grant every other key the same number.
		expect((screen.getByTestId("providercooldown-capacity-token-limit") as HTMLInputElement).value).toBe("");
		expect((screen.getByTestId("providercooldown-capacity-key-limit-token-key-2") as HTMLInputElement).value).toBe("");

		await save();
		expect(submittedPolicy()?.capacity).toMatchObject({
			enabled: true,
			scope: "key",
			key_limits: { "key-1": { token_limit: 2000, request_limit: 20 } },
		});
		expect((submittedPolicy()?.capacity as Record<string, unknown>).token_limit).toBeUndefined();
	});

	it("per-key overrides are editable and an emptied field removes the entry", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					capacity: {
						enabled: true,
						scope: "key",
						token_limit: 1000,
						key_limits: { "key-1": { token_limit: 5 } },
					},
				} as unknown as CooldownPolicy)}
			/>,
		);
		openSection("capacity");

		await waitFor(() => expect(screen.getByTestId("providercooldown-capacity-key-limit-token-key-1")).toBeTruthy());
		expect((screen.getByTestId("providercooldown-capacity-key-limit-token-key-1") as HTMLInputElement).value).toBe("5");
		// A key without an override starts blank (= inherit).
		expect((screen.getByTestId("providercooldown-capacity-key-limit-token-key-2") as HTMLInputElement).value).toBe("");

		fireEvent.change(screen.getByTestId("providercooldown-capacity-key-limit-token-key-2"), { target: { value: "50" } });
		fireEvent.change(screen.getByTestId("providercooldown-capacity-key-limit-token-key-1"), { target: { value: "" } });

		await save();
		expect((submittedPolicy()?.capacity as Record<string, unknown>).key_limits).toEqual({ "key-2": { token_limit: 50 } });
	});

	it("shows the request estimate when the token estimate is still missing", async () => {
		mocks.learnedLimits = [
			{
				provider: "openai",
				model: "gpt-4o",
				scope: "provider_model",
				keyName: null,
				sampleCount: 0,
				keyCount: 2,
				estimatedTokenLimit: null,
				windowSeconds: 60,
				lowerBound: null,
				upperBound: null,
				confidence: "insufficient",
				requestSampleCount: 8,
				estimatedRequestLimit: 120,
				requestLowerBound: 100,
				requestUpperBound: 121,
				requestConfidence: "high",
				limiterDimension: "rpm",
				firstSeen: "2026-01-01T00:00:00Z",
				lastSeen: "2026-01-02T00:00:00Z",
			},
		];
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({ capacity: { enabled: true, scope: "provider_model", window_seconds: 60 } } as unknown as CooldownPolicy)}
			/>,
		);
		openSection("learning");

		expect(screen.getByTestId("providercooldown-learning-token-gpt-4o").textContent).toContain(
			"cooldownCapability.learning.noTokenEstimate",
		);
		expect(screen.getByTestId("providercooldown-learning-request-gpt-4o").textContent).toContain(
			"cooldownCapability.learning.requestEstimate",
		);
		// The request budget alone is still adoptable.
		const adopt = screen.getByTestId("providercooldown-learning-adopt-gpt-4o") as HTMLButtonElement;
		expect(adopt.disabled).toBe(false);
	});

	it("disables adopt when neither budget has an estimate", () => {
		mocks.learnedLimits = [
			{
				provider: "openai",
				model: "gpt-4o",
				scope: "provider_model",
				keyName: null,
				sampleCount: 1,
				keyCount: 1,
				estimatedTokenLimit: null,
				windowSeconds: 60,
				lowerBound: null,
				upperBound: null,
				confidence: "insufficient",
				requestSampleCount: 1,
				estimatedRequestLimit: null,
				requestLowerBound: null,
				requestUpperBound: null,
				requestConfidence: "insufficient",
				limiterDimension: "unknown",
				firstSeen: "2026-01-01T00:00:00Z",
				lastSeen: "2026-01-02T00:00:00Z",
			},
		];
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({ capacity: { enabled: true, scope: "provider_model", window_seconds: 60 } } as unknown as CooldownPolicy)}
			/>,
		);
		openSection("learning");

		const adopt = screen.getByTestId("providercooldown-learning-adopt-gpt-4o") as HTMLButtonElement;
		expect(adopt.disabled).toBe(true);
	});

	it("strips per-key overrides when the scope is not key", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					rate_limit: RATE_LIMIT_RULE,
					capacity: {
						enabled: true,
						scope: "provider_model",
						token_limit: 1000,
						key_limits: { "key-1": { token_limit: 5 } },
					},
				} as unknown as CooldownPolicy)}
			/>,
		);
		changeTtl();

		await save();
		// The backend rejects key_limits under any other scope, so the payload
		// must not carry them even when an older config stored some.
		expect(submittedPolicy()?.capacity).not.toHaveProperty("key_limits");
	});
});
/**
 * The capacity gate's budget unit multiplies the limit, and the per-model
 * override is the narrowest of three layers. Both are easy to misread, so the
 * UI has to state the consequence and keep the override on the credential it
 * belongs to.
 */
describe("CooldownPolicyFormFragment — budget unit preview and per-model overrides", () => {
	beforeEach(() => {
		mocks.updateProvider.mockReset();
		mocks.updateProvider.mockImplementation(() => ({ unwrap: () => Promise.resolve({}) }));
		mocks.learnedLimits = [];
		mocks.models = [{ name: "gpt-4o" }, { name: "gpt-4o-mini" }];
	});

	it("spells out the budget count and the aggregate limit", () => {
		const { unmount } = render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					capacity: { enabled: true, scope: "key", token_limit: 1000, request_limit: 10, window_seconds: 60 },
				} as unknown as CooldownPolicy)}
			/>,
		);
		openSection("capacity");

		// 2 keys × 2 models = 4 budgets; 1,000 tokens each = 4,000 aggregate.
		const preview = screen.getByTestId("providercooldown-capacity-scope-preview");
		expect(preview.textContent).toContain("4");
		const total = screen.getByTestId("providercooldown-capacity-scope-total");
		expect(total.textContent).toContain("4,000");
		expect(total.textContent).toContain("40");
		unmount();

		// Under the whole-provider unit the multiplier disappears.
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					capacity: { enabled: true, scope: "provider", token_limit: 1000, request_limit: 10, window_seconds: 60 },
				} as unknown as CooldownPolicy)}
			/>,
		);
		openSection("capacity");
		expect(screen.getByTestId("providercooldown-capacity-scope-preview").textContent).toContain("1");
		const singleTotal = screen.getByTestId("providercooldown-capacity-scope-total");
		expect(singleTotal.textContent).toContain("1,000");
		expect(singleTotal.textContent).not.toContain("4,000");
	});

	it("edits a per-model override inside a credential and submits it nested", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					capacity: {
						enabled: true,
						scope: "key",
						token_limit: 1000,
						key_limits: { "key-1": { token_limit: 500 } },
					},
				} as unknown as CooldownPolicy)}
			/>,
		);
		openSection("capacity");

		await waitFor(() => expect(screen.getByTestId("providercooldown-capacity-key-model-add-key-1")).toBeTruthy());
		fireEvent.click(screen.getByTestId("providercooldown-capacity-key-model-add-key-1"));

		const tokenInput = screen.getByTestId("providercooldown-capacity-key-model-token-key-1-gpt-4o") as HTMLInputElement;
		// Seeded from the credential-level value, so the operator edits a real
		// baseline instead of an empty entry.
		expect(tokenInput.value).toBe("500");
		fireEvent.change(tokenInput, { target: { value: "50" } });

		await save();
		expect((submittedPolicy()?.capacity as Record<string, unknown>).key_limits).toEqual({
			"key-1": { token_limit: 500, models: { "gpt-4o": { token_limit: 50 } } },
		});
	});

	it("removing a per-model override keeps the credential-level limits", async () => {
		render(
			<CooldownPolicyFormFragment
				provider={baseProvider({
					capacity: {
						enabled: true,
						scope: "key",
						token_limit: 1000,
						key_limits: {
							"key-1": { token_limit: 500, models: { "gpt-4o": { token_limit: 50 } } },
						},
					},
				} as unknown as CooldownPolicy)}
			/>,
		);
		openSection("capacity");

		await waitFor(() => expect(screen.getByTestId("providercooldown-capacity-key-model-remove-key-1-gpt-4o")).toBeTruthy());
		fireEvent.click(screen.getByTestId("providercooldown-capacity-key-model-remove-key-1-gpt-4o"));

		await save();
		expect((submittedPolicy()?.capacity as Record<string, unknown>).key_limits).toEqual({
			"key-1": { token_limit: 500 },
		});
	});
});