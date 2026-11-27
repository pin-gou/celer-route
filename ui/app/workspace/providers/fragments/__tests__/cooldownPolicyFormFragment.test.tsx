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
				count: 1,
				windowSeconds: 60,
				limits: [
					{
						provider: "openai",
						model: "gpt-4o",
						sampleCount: 12,
						keyCount: 2,
						estimatedTokenLimit: 5000,
						windowSeconds: 60,
						lowerBound: 4800,
						upperBound: 5200,
						confidence: "high",
						firstSeen: "2026-01-01T00:00:00Z",
						lastSeen: "2026-01-02T00:00:00Z",
					},
				],
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

describe("CooldownPolicyFormFragment — capability tri-state payload", () => {
	beforeEach(() => {
		mocks.updateProvider.mockReset();
		mocks.updateProvider.mockImplementation(() => ({ unwrap: () => Promise.resolve({}) }));
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

	it("adopting a learned limit prefills the capacity token limit and switches to custom", async () => {
		render(<CooldownPolicyFormFragment provider={baseProvider()} />);
		openSection("learning");
		fireEvent.click(screen.getByTestId("providercooldown-learning-adopt-gpt-4o"));

		await waitFor(() => expect(screen.getByTestId("providercooldown-capacity-token-limit")).toBeTruthy());
		expect((screen.getByTestId("providercooldown-capacity-token-limit") as HTMLInputElement).value).toBe("5000");

		await save();
		expect(submittedPolicy()?.capacity).toMatchObject({ enabled: true, token_limit: 5000 });
	});
});