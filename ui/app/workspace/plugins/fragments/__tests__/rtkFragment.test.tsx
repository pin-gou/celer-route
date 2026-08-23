// @vitest-environment jsdom
/**
 * @file RtkFragment — ConfigForm enabled switch tests
 *
 * Post-mortem of the production incident where RTK silently stopped
 * compressing tool outputs because storage held a null config_json that
 * deserialised to Config{Enabled: false}. Two safeguards back the fix:
 *
 *   1. plugins/rtk/config.go zero-detect — applies a default on Init.
 *   2. UI ConfigForm must surface `enabled` and write it through on save.
 *
 * The tests below pin (2). They check the in-form switch exists, mirrors
 * the stored value, and is the sole source of truth (no second top-level
 * toggle). Submit-time mutation shape is covered by the shared plugin
 * API contract — see providercooldownFragment.test.tsx for the analogous
 * 3-field pattern.
 */

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { RtkFragment } from "../rtkFragment";
import type { Plugin } from "@/lib/types/plugins";

// ---------------------------------------------------------------------------
// Mocks — keep the network out of the picture.
// ---------------------------------------------------------------------------

const mocks = vi.hoisted(() => ({
	updatePlugin: vi.fn(() => ({ unwrap: () => Promise.resolve({}) })),
	rtkStatsData: {
		plugin: "rtk",
		invocations: 0,
		compressed_count: 0,
		original_tokens: 0,
		compressed_tokens: 0,
		tokens_saved: 0,
		compression_ratio: 0,
	},
}));

vi.mock("@/lib/store/apis/pluginsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/pluginsApi")>();
	return {
		...actual,
		useUpdatePluginMutation: () => [mocks.updatePlugin, { isLoading: false }],
		useGetRtkStatsQuery: () => ({ data: mocks.rtkStatsData, isLoading: false }),
	};
});

// RBAC — keep the form submittable in tests.
vi.mock("@/lib/rbac", () => ({
	RbacOperation: { Update: "update" },
	RbacResource: { Plugins: "plugins" },
	useRbac: () => true,
}));

// @tanstack/react-router — the fragment uses Link in its after-save card.
// We replace Link with an <a> so jsdom doesn't need a RouterProvider.
vi.mock("@tanstack/react-router", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@tanstack/react-router")>();
	return {
		...actual,
		useNavigate: () => vi.fn(),
		Link: ({ children, ...props }: any) => (
			<a data-mock-link="true" data-testid={props["data-testid"]} href={props.to || "#"}>
				{children}
			</a>
		),
	};
});

// sonner — silence toast side effects so tests don't pollute jsdom output.
vi.mock("sonner", () => ({
	toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makePlugin(overrides: Partial<Plugin["config"]> = {}): Plugin {
	return {
		name: "rtk",
		actualName: "rtk",
		enabled: true,
		isCustom: false,
		config: {
			intensity: "standard",
			apply_to_tool_results: true,
			apply_to_code_blocks: false,
			apply_to_assistant_messages: false,
			max_lines_per_result: 120,
			max_chars_per_result: 12000,
			dedup_threshold: 3,
			enable_grouping: false,
			grouping_threshold: 3,
			preserve_cache_control: false,
			custom_filters_enabled: true,
			trust_project_filters: false,
			enabled_filters: [],
			disabled_filters: [],
			raw_output_retention: "never",
			raw_output_max_bytes: 1048576,
			pipeline: [{ id: "rtk" }],
			min_tokens_to_compress: 0,
			enable_renderers: true,
			snapshot_mode: "off",
			snapshot_max_bytes: 30720,
			...overrides,
		} as any,
		status: { name: "rtk", status: "active", logs: [], types: ["llm", "http"] },
	};
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("RtkFragment — enabled switch in ConfigForm", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
	});

	it("renders the enabled switch reflecting config.enabled (true by default)", () => {
		render(<RtkFragment plugin={makePlugin()} />);

		const sw = screen.getByTestId("rtk-field-enabled") as HTMLElement;
		// Radix Switch encodes checked via data-state="checked".
		expect(sw.getAttribute("data-state")).toBe("checked");
	});

	it("renders the enabled switch unchecked when storage carried enabled=false", () => {
		render(<RtkFragment plugin={makePlugin({ enabled: false })} />);

		const sw = screen.getByTestId("rtk-field-enabled") as HTMLElement;
		expect(sw.getAttribute("data-state")).toBe("unchecked");
	});

	it("does NOT render the legacy top-level rtk-enabled-switch test id (two-toggle guard)", () => {
		render(<RtkFragment plugin={makePlugin()} />);

		// The old EnabledSwitch used data-testid="rtk-enabled-switch". With
		// the unified design, only the in-form "rtk-field-enabled" remains.
		expect(screen.queryByTestId("rtk-enabled-switch")).toBeNull();
	});

	it("does NOT render the top-level enableDescription card (old EnabledSwitch body)", () => {
		render(<RtkFragment plugin={makePlugin()} />);

		// The old layout had a card with "Compress tool outputs to reduce
		// token usage and latency" as standalone body copy. The new layout
		// reuses the same copy as a help hint next to the in-form label,
		// so it appears once — not inside a separate card.
		const helpHints = screen.queryAllByText(/Compress tool outputs to reduce token usage and latency/i);
		expect(helpHints.length).toBeLessThanOrEqual(1);
	});
});