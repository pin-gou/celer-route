// @vitest-environment jsdom
/**
 * ErrorSampleBrowser — the "turn errors into rules" list.
 *
 * Pins the window contract: when the embedding tab owns the window, the
 * browser renders it instead of keeping a private selector. Two independent
 * window pickers on the same page let the operator compare two error lists
 * computed over different windows, which reads as contradictory data.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
	args: [] as Array<{ provider: string; window: string; limit?: number }>,
	patterns: [] as Array<Record<string, unknown>>,
}));

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string, options?: Record<string, unknown>) => (options ? `${key} ${Object.values(options).join(" ")}` : key),
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
	initReactI18next: { type: "3rdParty", init: () => {} },
}));

vi.mock("@/lib/store/apis/errorPatternsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/errorPatternsApi")>();
	return {
		...actual,
		useGetErrorPatternsQuery: (args: { provider: string; window: string; limit?: number }) => {
			mocks.args.push(args);
			return {
				data: { provider: args.provider, window: args.window, total_errors: mocks.patterns.length, patterns: mocks.patterns },
				isLoading: false,
				error: undefined,
				fulfilledTimeStamp: 1_700_000_000_000,
			};
		},
	};
});

import { ErrorSampleBrowser } from "../errorSampleBrowser";

const pattern = {
	rank: 1,
	count: 12,
	status_code: 429,
	error_type: "rate_limit_error",
	error_code: "EndpointRPMExceeded",
	sample_message: "rpm exhausted",
};

describe("ErrorSampleBrowser", () => {
	beforeEach(() => {
		mocks.args = [];
		mocks.patterns = [];
	});

	it("queries the window the tab owns and renders no selector of its own", () => {
		render(<ErrorSampleBrowser provider="openai" onApply={() => {}} window="24h" />);

		expect(mocks.args.at(-1)).toMatchObject({ provider: "openai", window: "24h" });
		expect(screen.queryByTestId("error-sample-browser-window-trigger")).toBeNull();
	});

	it("keeps a private window selector when embedded without a tab window", () => {
		render(<ErrorSampleBrowser provider="openai" onApply={() => {}} />);

		expect(screen.getByTestId("error-sample-browser-window-trigger")).toBeTruthy();
		expect(mocks.args.at(-1)).toMatchObject({ provider: "openai", window: "1h" });
	});

	it("offers each error shape as a rule match", () => {
		mocks.patterns = [pattern];
		const onApply = vi.fn();
		render(<ErrorSampleBrowser provider="openai" onApply={onApply} window="1h" />);

		expect(screen.getByTestId("error-sample-row-1")).toBeTruthy();
		fireEvent.click(screen.getByTestId("error-sample-apply-1"));
		expect(onApply).toHaveBeenCalledWith(expect.objectContaining({ error_code: "EndpointRPMExceeded" }));
	});

	it("hides the empty state when the tab already explains it", () => {
		render(<ErrorSampleBrowser provider="openai" onApply={() => {}} window="1h" suppressEmptyState />);

		expect(screen.queryByText("fragments.cooldownPolicy.errorSample.noErrors")).toBeNull();
	});

	it("does not suggest a wider window while the widest one is already selected", () => {
		render(<ErrorSampleBrowser provider="openai" onApply={() => {}} window="24h" />);

		expect(screen.queryByText("fragments.cooldownPolicy.errorSample.tryChangingWindow")).toBeNull();
	});
});