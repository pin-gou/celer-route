// @vitest-environment jsdom
/**
 * CooldownTab — the provider cooldown tab's information architecture.
 *
 * The regressions this pins down:
 *   - the observation window is owned by the TAB. Usage, error shapes and the
 *     error→rule flow must all describe the SAME window, otherwise the page
 *     shows two error lists computed over different windows and they read as
 *     contradictory data;
 *   - the error shapes are rendered ONCE. The old "baseline: top 3" block was
 *     a second rendering of the very same endpoint, which cost a duplicate
 *     request and a misleading "baseline" name for a single-period list;
 *   - one empty state for "no requests" (the tab notice), while each block
 *     still states its own result ("no errors", "no traffic in the gate") —
 *     those are different facts and must not be swallowed;
 *   - the configuration zone (errors → rules) comes before the supporting
 *     observation detail.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
	usageArgs: [] as Array<{ provider: string; windowMinutes: number }>,
	usageEntries: [] as Array<Record<string, unknown>>,
	usageError: undefined as unknown,
	usageLoading: false,
	errorPatternArgs: [] as Array<{ provider: string; window: string; limit?: number }>,
	errorPatterns: [] as Array<Record<string, unknown>>,
	capacityEntries: [] as Array<Record<string, unknown>>,
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

vi.mock("@/lib/store/apis/providerUsageApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/providerUsageApi")>();
	return {
		...actual,
		useGetProviderUsageQuery: (args: { provider: string; windowMinutes: number }) => {
			mocks.usageArgs.push(args);
			return {
				data: mocks.usageError
					? undefined
					: { provider: args.provider, window_minutes: args.windowMinutes, start_time: "", end_time: "", entries: mocks.usageEntries },
				error: mocks.usageError,
				isLoading: mocks.usageLoading,
				fulfilledTimeStamp: 1_700_000_000_000,
			};
		},
	};
});

vi.mock("@/lib/store/apis/errorPatternsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/errorPatternsApi")>();
	return {
		...actual,
		useGetErrorPatternsQuery: (args: { provider: string; window: string; limit?: number }) => {
			mocks.errorPatternArgs.push(args);
			return {
				data: { provider: args.provider, window: args.window, total_errors: mocks.errorPatterns.length, patterns: mocks.errorPatterns },
				isLoading: false,
				error: undefined,
				fulfilledTimeStamp: 1_700_000_000_000,
			};
		},
	};
});

vi.mock("@/lib/store/apis/pluginsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/pluginsApi")>();
	return {
		...actual,
		useGetCapacityUsageQuery: () => ({
			data: { plugin: "provider-cooldown", count: mocks.capacityEntries.length, entries: mocks.capacityEntries },
			error: undefined,
			isLoading: false,
			fulfilledTimeStamp: 1_700_000_000_000,
		}),
		useGetPluginQuery: () => ({ data: undefined }),
		useGetLearnedLimitsQuery: () => ({ data: undefined, error: undefined }),
	};
});

vi.mock("@/lib/store/apis/providersApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/providersApi")>();
	return {
		...actual,
		useUpdateProviderMutation: () => [vi.fn(), { isLoading: false }],
		// The cooldown form's per-key capacity editor reads the provider's keys.
		useGetProviderKeysQuery: () => ({ data: [], isLoading: false, error: undefined }),
	};
});

vi.mock("@/lib/store/apis/providerErrorCatalogApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/providerErrorCatalogApi")>();
	return {
		...actual,
		useGetProviderErrorCatalogQuery: () => ({ data: undefined }),
	};
});

vi.mock("@/lib/rbac", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/rbac")>();
	return { ...actual, useRbac: () => true };
});

import { CooldownTab } from "./CooldownTab";
import type { ModelProvider } from "@/lib/types/config";

const provider = {
	name: "openai",
	provider_status: "active",
} as ModelProvider;

const usageEntry = (overrides: Record<string, unknown> = {}) => ({
	model: "gpt-4o",
	key_id: "key-1",
	key_name: "key-1",
	requests: 10,
	successes: 9,
	errors: 1,
	cancelled: 0,
	prompt_tokens: 0,
	completion_tokens: 0,
	cached_read_tokens: 0,
	total_tokens: 1000,
	avg_total_tokens: 100,
	requests_per_minute: 1,
	tokens_per_minute: 10,
	...overrides,
});

describe("CooldownTab", () => {
	beforeEach(() => {
		mocks.usageArgs = [];
		mocks.usageEntries = [];
		mocks.usageError = undefined;
		mocks.usageLoading = false;
		mocks.errorPatternArgs = [];
		mocks.errorPatterns = [];
		mocks.capacityEntries = [];
	});

	it("drives usage and error-shape queries from the SAME tab-level window", () => {
		mocks.usageEntries = [usageEntry()];
		render(<CooldownTab provider={provider} />);

		expect(mocks.usageArgs.at(-1)).toEqual({ provider: "openai", windowMinutes: 60 });
		expect(mocks.errorPatternArgs.at(-1)).toMatchObject({ provider: "openai", window: "1h" });

		fireEvent.click(screen.getByTestId("providers2-cooldown-window-24h"));

		expect(mocks.usageArgs.at(-1)).toEqual({ provider: "openai", windowMinutes: 1440 });
		expect(mocks.errorPatternArgs.at(-1)).toMatchObject({ provider: "openai", window: "24h" });
	});

	it("renders the error shapes exactly once — no second 'baseline' list", () => {
		mocks.usageEntries = [usageEntry()];
		render(<CooldownTab provider={provider} />);

		expect(document.querySelector("[data-testid='providers2-usage-baseline']")).toBeNull();
		expect(screen.getAllByTestId("error-sample-browser")).toHaveLength(1);
		// One endpoint, one query shape — the baseline used to ask for the same
		// data a second time with a smaller limit, which meant two renderings
		// of the same list and two independent windows.
		const distinct = new Set(mocks.errorPatternArgs.map((args) => JSON.stringify(args)));
		expect([...distinct]).toEqual([JSON.stringify({ provider: "openai", window: "1h", limit: 20 })]);
	});

	it("shows the error shapes above the rule editor they feed", () => {
		mocks.usageEntries = [usageEntry()];
		render(<CooldownTab provider={provider} />);

		const browser = screen.getByTestId("error-sample-browser");
		const rule = screen.getByTestId("provider-cooldown-rate_limit-enable-switch");
		expect(browser.compareDocumentPosition(rule) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
	});

	it("puts the configuration zone before the observation detail", () => {
		mocks.usageEntries = [usageEntry()];
		render(<CooldownTab provider={provider} />);

		const policyCard = screen.getByTestId("providers2-cooldown-policy-card");
		const usageCard = screen.getByTestId("providers2-usage-card");
		expect(policyCard.compareDocumentPosition(usageCard) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
	});

	it("keeps the SAME page structure in both windows", () => {
		// The window is a FILTER, not a layout switch. The summary strip and the
		// usage card must exist in every window; an empty window changes their
		// numbers, not whether they are on the page. This is the regression the
		// "1h shows no numbers at all" report was about.
		const structure = () =>
			({
				summary: !!screen.queryByTestId("providers2-cooldown-summary"),
				usageCard: !!screen.queryByTestId("providers2-usage-card"),
			}) as Record<string, boolean>;

		mocks.usageEntries = [];
		render(<CooldownTab provider={provider} />);
		const emptyWindow = structure();
		expect(emptyWindow.summary).toBe(true);
		expect(emptyWindow.usageCard).toBe(true);

		fireEvent.click(screen.getByTestId("providers2-cooldown-window-24h"));
		expect(structure()).toEqual(emptyWindow);
	});

	it("shows zeros (and an undefined error rate) for an empty window", () => {
		mocks.usageEntries = [];
		render(<CooldownTab provider={provider} />);

		const strip = screen.getByTestId("providers2-cooldown-summary");
		expect(strip.textContent).toContain("0");
		expect(strip.textContent).toContain("—");
		// The usage card explains the empty window inside itself rather than
		// disappearing.
		expect(screen.getByTestId("providers2-usage-empty")).toBeTruthy();
	});

	it("keeps the error shapes' own 'no errors' message in BOTH windows", () => {
		// "no errors" is a different fact from "no requests": a window can hold
		// traffic and still have nothing to turn into a rule. The error block
		// must therefore explain itself whatever the window, otherwise it looks
		// blank on one window and populated on the other.
		mocks.usageEntries = [];
		render(<CooldownTab provider={provider} />);
		expect(screen.getByText("fragments.cooldownPolicy.errorSample.noErrors")).toBeTruthy();

		fireEvent.click(screen.getByTestId("providers2-cooldown-window-24h"));
		expect(screen.getByText("fragments.cooldownPolicy.errorSample.noErrors")).toBeTruthy();
	});

	it("keeps the summary numbers in the tab strip and the breakdown in the table", () => {
		mocks.usageEntries = [usageEntry({ requests: 10, errors: 1, total_tokens: 1000 })];
		render(<CooldownTab provider={provider} />);

		const strip = screen.getByTestId("providers2-cooldown-summary");
		expect(strip.textContent).toContain("10");
		expect(strip.textContent).toContain("10.0%");
		expect(screen.getAllByTestId("providers2-usage-row")).toHaveLength(1);
	});

	it("shows when the observation data was fetched", () => {
		mocks.usageEntries = [usageEntry()];
		render(<CooldownTab provider={provider} />);

		expect(screen.getByTestId("providers2-cooldown-updated-at")).toBeTruthy();
		expect(screen.getByTestId("providers2-capacity-updated-at")).toBeTruthy();
	});
});