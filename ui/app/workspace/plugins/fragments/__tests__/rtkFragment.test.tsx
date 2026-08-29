// @vitest-environment jsdom
/**
 * @file RtkFragment — EnabledSwitch tests
 *
 * Mirrors the structure of providercooldownFragment.test.tsx so the RTK
 * and provider-cooldown on/off toggles share one verification pattern.
 * The regression target is the production incident where RTK silently
 * stopped compressing tool outputs because storage held a null config_json
 * — the server-side fix (plugins/rtk/config.go zero-detect) keeps RTK
 * enabled by default, and the UI here drives the operator-visible toggle
 * with the same instant-write semantics as provider-cooldown.
 */

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { RtkFragment } from "../rtkFragment";
import type { Plugin } from "@/lib/types/plugins";

// ---------------------------------------------------------------------------
// Mocks — keep the network out of the picture. We only care that the form
// dispatches a mutation with the right shape.
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

function makePlugin(overrides: Partial<Plugin> = {}): Plugin {
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
			raw_output_dir: "",
			raw_output_ttl_hours: 24,
			pipeline: [{ id: "rtk" }],
			min_tokens_to_compress: 0,
			enable_renderers: true,
			snapshot_mode: "off",
			snapshot_max_bytes: 30720,
		} as any,
		status: { name: "rtk", status: "active", logs: [], types: ["llm", "http"] },
		...overrides,
	};
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("RtkFragment — EnabledSwitch mirrors provider-cooldown UX", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
	});

	it("renders the switch reflecting the plugin's enabled state (true)", () => {
		render(<RtkFragment plugin={makePlugin({ enabled: true })} />);
		const sw = screen.getByTestId("rtk-enabled-switch") as HTMLElement;
		expect(sw.getAttribute("data-state")).toBe("checked");
	});

	it("renders the switch unchecked when the plugin is disabled", () => {
		render(<RtkFragment plugin={makePlugin({ enabled: false })} />);
		const sw = screen.getByTestId("rtk-enabled-switch") as HTMLElement;
		expect(sw.getAttribute("data-state")).toBe("unchecked");
	});

	it("dispatches an immediate enable mutation on switch click — no Save button required", async () => {
		render(<RtkFragment plugin={makePlugin({ enabled: false })} />);

		fireEvent.click(screen.getByTestId("rtk-enabled-switch"));

		await waitFor(() => {
			expect(mocks.updatePlugin).toHaveBeenCalledWith({
				name: "rtk",
				data: { enabled: true },
			});
		});
	});

	it("dispatches an immediate disable mutation on switch click", async () => {
		render(<RtkFragment plugin={makePlugin({ enabled: true })} />);

		fireEvent.click(screen.getByTestId("rtk-enabled-switch"));

		await waitFor(() => {
			expect(mocks.updatePlugin).toHaveBeenCalledWith({
				name: "rtk",
				data: { enabled: false },
			});
		});
	});
});
describe("RtkFragment — raw_output_dir / raw_output_ttl_hours fields", () => {
	// The new fields live inside the "Debug / raw output" AccordionItem, which
	// is collapsed by default. The helper expands it so getByTestId can find
	// the inputs the same way the operator would after clicking the section.
	function expandRawOutputSection() {
		fireEvent.click(screen.getByTestId("rtk-section-raw-output-trigger"));
	}

	it("renders the new fields with default empty dir and TTL=24", () => {
		render(<RtkFragment plugin={makePlugin()} />);
		expandRawOutputSection();

		const dirInput = screen.getByTestId("rtk-field-raw-output-dir") as HTMLInputElement;
		expect(dirInput.type).toBe("text");
		expect(dirInput.value).toBe("");

		const ttlInput = screen.getByTestId("rtk-field-raw-output-ttl-hours") as HTMLInputElement;
		expect(ttlInput.type).toBe("number");
		expect(ttlInput.value).toBe("24");
	});

	it("reflects the persisted values when the plugin config carries them", () => {
		render(
			<RtkFragment
				plugin={makePlugin({
					config: {
						raw_output_dir: "/var/log/celer-route-raw",
						raw_output_ttl_hours: 6,
					} as any,
				})}
			/>,
		);
		expandRawOutputSection();

		const dirInput = screen.getByTestId("rtk-field-raw-output-dir") as HTMLInputElement;
		expect(dirInput.value).toBe("/var/log/celer-route-raw");

		const ttlInput = screen.getByTestId("rtk-field-raw-output-ttl-hours") as HTMLInputElement;
		expect(ttlInput.value).toBe("6");
	});

	it("persists both values on save (PUT /api/plugins/rtk)", async () => {
		render(<RtkFragment plugin={makePlugin()} />);
		expandRawOutputSection();

		fireEvent.change(screen.getByTestId("rtk-field-raw-output-dir"), {
			target: { value: "/srv/celer-route/raw" },
		});
		fireEvent.change(screen.getByTestId("rtk-field-raw-output-ttl-hours"), {
			target: { value: "12" },
		});
		fireEvent.click(screen.getByTestId("rtk-save-btn"));

		await waitFor(() => {
			const call = mocks.updatePlugin.mock.calls.find(
				(c: any) => c[0]?.name === "rtk" && (c[0]?.data?.config as any)?.raw_output_dir === "/srv/celer-route/raw",
			);
			expect(call).toBeDefined();
			const cfg = (call as any)[0].data.config as any;
			expect(cfg.raw_output_dir).toBe("/srv/celer-route/raw");
			expect(cfg.raw_output_ttl_hours).toBe(12);
		});
	});
});