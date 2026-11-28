// @vitest-environment jsdom
/**
 * CooldownCapacityCard — "water level" of the capacity gate for one provider.
 *
 * Pins: used/threshold rendering (with retry-after when the gate is engaged),
 * provider filtering, and the explicit empty states — never a misleading
 * 0/0 progress bar.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
	entries: [] as Array<Record<string, unknown>>,
	error: undefined as unknown,
	isLoading: false,
}));

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string, options?: Record<string, unknown>) => (options ? `${key} ${Object.values(options).join(" ")}` : key),
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
	initReactI18next: { type: "3rdParty", init: () => {} },
}));

vi.mock("@/lib/store/apis/pluginsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/pluginsApi")>();
	return {
		...actual,
		useGetCapacityUsageQuery: () => ({
			data: { plugin: "provider-cooldown", count: mocks.entries.length, entries: mocks.entries },
			error: mocks.error,
			isLoading: mocks.isLoading,
		}),
	};
});

import { CooldownCapacityCard } from "./CooldownCapacityCard";
import type { ModelProvider } from "@/lib/types/config";

const provider = (cooldownPolicy?: ModelProvider["cooldown_policy"]): ModelProvider =>
	({
		name: "openai",
		provider_status: "active",
		...(cooldownPolicy === undefined ? {} : { cooldown_policy: cooldownPolicy }),
	}) as ModelProvider;

const capacityEntry = (overrides: Record<string, unknown> = {}) => ({
	provider: "openai",
	model: "gpt-4o",
	scope: "provider_model",
	windowSeconds: 60,
	usedTokens: 450,
	usedRequests: 45,
	tokenLimit: 1000,
	requestLimit: 100,
	headroom: 0.9,
	thresholdTokens: 900,
	thresholdRequests: 90,
	retryAfterSeconds: 0,
	source: "provider",
	...overrides,
});

describe("CooldownCapacityCard", () => {
	beforeEach(() => {
		mocks.entries = [];
		mocks.error = undefined;
		mocks.isLoading = false;
	});

	it("renders the used/threshold percentages for the provider", () => {
		mocks.entries = [capacityEntry()];
		render(<CooldownCapacityCard provider={provider()} />);

		const tokens = screen.getByTestId("providers2-capacity-tokens-gpt-4o");
		expect(tokens.textContent).toContain("450");
		expect(tokens.textContent).toContain("900");
		expect(tokens.textContent).toContain("50%");

		const requests = screen.getByTestId("providers2-capacity-requests-gpt-4o");
		expect(requests.textContent).toContain("45");
		expect(requests.textContent).toContain("90");
		expect(requests.textContent).toContain("50%");
	});

	it("renders the retry-after hint only when the gate is engaged", () => {
		mocks.entries = [capacityEntry({ retryAfterSeconds: 45 })];
		const { unmount } = render(<CooldownCapacityCard provider={provider()} />);
		expect(screen.getByTestId("providers2-capacity-retry-gpt-4o").textContent).toContain("45");
		unmount();

		mocks.entries = [capacityEntry()];
		render(<CooldownCapacityCard provider={provider()} />);
		expect(screen.queryByTestId("providers2-capacity-retry-gpt-4o")).toBeNull();
	});

	it("ignores entries belonging to other providers", () => {
		mocks.entries = [capacityEntry({ provider: "anthropic" })];
		render(<CooldownCapacityCard provider={provider()} />);

		expect(screen.queryByTestId("providers2-capacity-row-gpt-4o")).toBeNull();
		expect(screen.getByTestId("providers2-capacity-empty")).toBeTruthy();
	});

	it("renders an explicit empty state when the window has no traffic", () => {
		mocks.entries = [];
		render(<CooldownCapacityCard provider={provider()} />);

		expect(screen.getByTestId("providers2-capacity-empty").textContent).toContain("capacityUsage.empty");
	});

	it("renders the disabled empty state when the provider turned the gate off", () => {
		mocks.entries = [];
		render(<CooldownCapacityCard provider={provider({ capacity: { enabled: false } })} />);

		expect(screen.getByTestId("providers2-capacity-empty").textContent).toContain("capacityUsage.emptyDisabled");
	});

	it("groups key-scoped rows by credential and summarizes how many are engaged", () => {
		mocks.entries = [
			capacityEntry({
				scope: "key",
				keyId: "key-1-id",
				keyName: "key-1",
				model: "gpt-4o",
				limitSource: "key_model",
				retryAfterSeconds: 12,
			}),
			capacityEntry({ scope: "key", keyId: "key-1-id", keyName: "key-1", model: "gpt-4o-mini", limitSource: "key" }),
			capacityEntry({ scope: "key", keyId: "key-2-id", keyName: "key-2", model: "gpt-4o" }),
		];
		render(<CooldownCapacityCard provider={provider()} />);

		const summary = screen.getByTestId("providers2-capacity-summary");
		expect(summary.textContent).toContain("3");
		expect(summary.textContent).toContain("1");

		expect(screen.getByTestId("providers2-capacity-group-key-1-id").textContent).toContain("key-1");
		expect(screen.getByTestId("providers2-capacity-group-key-2-id").textContent).toContain("key-2");
		// The most specific layer is called out; the shared default is not.
		expect(screen.getByTestId("providers2-capacity-limit-source-gpt-4o|key-1-id").textContent).toContain(
			"cooldownCapability.capacityUsage.limitSource.key_model",
		);
		expect(screen.queryByTestId("providers2-capacity-limit-source-gpt-4o|key-2-id")).toBeNull();
	});

	it("hides the whole card when the plugin is not loaded (HTTP 400)", () => {
		mocks.error = { status: 400 };
		render(<CooldownCapacityCard provider={provider()} />);

		expect(screen.queryByTestId("providers2-capacity-usage-card")).toBeNull();
	});
});