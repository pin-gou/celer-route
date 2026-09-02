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

// ---------------------------------------------------------------------------
// Pipeline-centric layout (V-pipeline-engine-panels)
// ---------------------------------------------------------------------------

describe("RtkFragment — pipeline checkboxes inside the enablement card", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
	});

	it("renders the enablement card with the pipeline checkboxes beneath the on/off switch", () => {
		render(<RtkFragment plugin={makePlugin()} />);
		const section = screen.getByTestId("rtk-enabled-section");
		expect(section).toBeTruthy();
		// The top-level enable switch sits in the same card.
		expect(section.contains(screen.getByTestId("rtk-enabled-switch"))).toBe(true);
		// And the two pipeline checkboxes live beneath it.
		expect(section.contains(screen.getByTestId("pipeline-rtk-checkbox"))).toBe(true);
		expect(section.contains(screen.getByTestId("pipeline-caveman-checkbox"))).toBe(true);
		// Legacy JSON textarea testid is gone so the raw editor never regresses.
		expect(screen.queryByTestId("rtk-field-pipeline")).toBeNull();
		expect(screen.queryByTestId("pipeline-section")).toBeNull();
	});

	it("keeps the two pipeline checkboxes in fixed order (rtk → caveman)", () => {
		render(<RtkFragment plugin={makePlugin()} />);
		const rtk = screen.getByTestId("pipeline-rtk-checkbox");
		const caveman = screen.getByTestId("pipeline-caveman-checkbox");
		expect(rtk.compareDocumentPosition(caveman) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
	});

	it("keeps the RTK checkbox checked-and-disabled (always-on, controlled by EnabledSwitch)", () => {
		render(<RtkFragment plugin={makePlugin()} />);
		const rtk = screen.getByTestId("pipeline-rtk-checkbox") as HTMLButtonElement;
		expect(rtk.getAttribute("data-state")).toBe("checked");
		expect(rtk.hasAttribute("disabled")).toBe(true);
	});

	it("disables both pipeline checkboxes when the plugin is off", () => {
		render(<RtkFragment plugin={makePlugin({ enabled: false })} />);
		const rtk = screen.getByTestId("pipeline-rtk-checkbox") as HTMLButtonElement;
		const caveman = screen.getByTestId("pipeline-caveman-checkbox") as HTMLButtonElement;
		expect(rtk.hasAttribute("disabled")).toBe(true);
		expect(caveman.hasAttribute("disabled")).toBe(true);
	});

	it("reflects caveman.enabled on the Caveman pipeline checkbox", () => {
		const plugin = makePlugin({ config: { caveman: { enabled: true } } as any });
		render(<RtkFragment plugin={plugin} />);
		const caveman = screen.getByTestId("pipeline-caveman-checkbox") as HTMLButtonElement;
		expect(caveman.getAttribute("data-state")).toBe("checked");

		fireEvent.click(caveman);
		expect(caveman.getAttribute("data-state")).toBe("unchecked");
	});

	it("renders the RTK and Caveman engine panels as separate first-class cards", () => {
		const plugin = makePlugin({ config: { caveman: { enabled: true } } as any });
		render(<RtkFragment plugin={plugin} />);
		// Both panels are mounted under their own config tabs; only the active
		// one is visible, but the tab list exposes three configuration entries.
		expect(screen.getByTestId("rtk-tab-shared")).toBeTruthy();
		expect(screen.getByTestId("rtk-tab-rtk")).toBeTruthy();
		expect(screen.getByTestId("rtk-tab-caveman")).toBeTruthy();
		fireEvent.mouseDown(screen.getByTestId("rtk-tab-rtk"));
		expect(screen.getByTestId("engine-panel-rtk")).toBeTruthy();
		fireEvent.mouseDown(screen.getByTestId("rtk-tab-caveman"));
		expect(screen.getByTestId("engine-panel-caveman")).toBeTruthy();
	});

	it("keeps the existing RTK field testids inside the RTK engine panel", () => {
		render(<RtkFragment plugin={makePlugin()} />);
		fireEvent.mouseDown(screen.getByTestId("rtk-tab-rtk"));
		const rtkPanel = screen.getByTestId("engine-panel-rtk");
		expect(rtkPanel.contains(screen.getByTestId("rtk-field-intensity"))).toBe(true);
		expect(rtkPanel.contains(screen.getByTestId("rtk-field-max-lines"))).toBe(true);
		expect(rtkPanel.contains(screen.getByTestId("rtk-field-preserve-cache-control"))).toBe(true);
	});

	it("hides the Caveman tab while caveman.enabled is false (default)", () => {
		render(<RtkFragment plugin={makePlugin()} />);
		expect(screen.queryByTestId("rtk-tab-caveman")).toBeNull();
		expect(screen.queryByTestId("engine-panel-caveman")).toBeNull();
	});

	it("shows the Caveman tab only when caveman.enabled is true", () => {
		const plugin = makePlugin({ config: { caveman: { enabled: true } } as any });
		render(<RtkFragment plugin={plugin} />);
		expect(screen.getByTestId("rtk-tab-caveman")).toBeTruthy();
		fireEvent.mouseDown(screen.getByTestId("rtk-tab-caveman"));
		expect(screen.getByTestId("engine-panel-caveman")).toBeTruthy();
	});

	it("keeps the existing Caveman field testids inside the Caveman engine panel (no duplicate enable toggle)", () => {
		const plugin = makePlugin({
			config: { caveman: { enabled: true, intensity: "full", language: "en", min_message_length: 80 } },
		});
		render(<RtkFragment plugin={plugin} />);
		fireEvent.mouseDown(screen.getByTestId("rtk-tab-caveman"));
		const cavemanPanel = screen.getByTestId("engine-panel-caveman");
		expect(cavemanPanel.contains(screen.getByTestId("caveman-field-intensity"))).toBe(true);
		expect(cavemanPanel.contains(screen.getByTestId("caveman-field-language"))).toBe(true);
		expect(cavemanPanel.contains(screen.getByTestId("caveman-field-roles"))).toBe(true);
		// The enable switch was removed from this panel — the pipeline checkbox
		// in the enablement card is the single toggle.
		expect(cavemanPanel.contains(screen.queryByTestId("caveman-field-enabled"))).toBe(false);
	});

	it("bounces back to the shared tab when Caveman is disabled while on its tab", () => {
		const plugin = makePlugin({ config: { caveman: { enabled: true } } as any });
		render(<RtkFragment plugin={plugin} />);
		fireEvent.mouseDown(screen.getByTestId("rtk-tab-caveman"));
		expect(screen.getByTestId("engine-panel-caveman")).toBeTruthy();
		// Uncheck Caveman in the pipeline checkbox.
		fireEvent.click(screen.getByTestId("pipeline-caveman-checkbox"));
		expect(screen.queryByTestId("rtk-tab-caveman")).toBeNull();
		expect(screen.queryByTestId("engine-panel-caveman")).toBeNull();
	});

	it("auto-jumps to the Caveman tab when Caveman is enabled from the pipeline checkbox", () => {
		render(<RtkFragment plugin={makePlugin()} />);
		// Defaults to the shared tab with no Caveman tab.
		expect(screen.queryByTestId("rtk-tab-caveman")).toBeNull();
		expect(screen.queryByTestId("engine-panel-caveman")).toBeNull();
		// Enabling Caveman reveals its tab AND activates it automatically.
		fireEvent.click(screen.getByTestId("pipeline-caveman-checkbox"));
		expect(screen.getByTestId("rtk-tab-caveman")).toBeTruthy();
		expect(screen.getByTestId("engine-panel-caveman")).toBeTruthy();
	});

	it("does not auto-jump on mount when the persisted config already has Caveman enabled", () => {
		const plugin = makePlugin({ config: { caveman: { enabled: true } } as any });
		render(<RtkFragment plugin={plugin} />);
		// The Caveman tab exists but the shared tab stays active — no jump.
		expect(screen.getByTestId("rtk-tab-caveman")).toBeTruthy();
		const sharedTrigger = screen.getByTestId("rtk-tab-shared");
		expect(sharedTrigger.getAttribute("data-state")).toBe("active");
	});

	it("exposes the caveman sub-fields when caveman.enabled is true", () => {
		const plugin = makePlugin({
			config: { caveman: { enabled: true, intensity: "full", language: "en", min_message_length: 80 } },
		});
		render(<RtkFragment plugin={plugin} />);
		fireEvent.mouseDown(screen.getByTestId("rtk-tab-caveman"));
		expect(screen.getByTestId("caveman-field-intensity")).toBeTruthy();
		expect(screen.getByTestId("caveman-field-language")).toBeTruthy();
	});

	it("writes pipeline=[{id:'rtk'}] on submit when caveman is disabled", async () => {
		render(<RtkFragment plugin={makePlugin()} />);
		// Force a save by touching a tunable then submitting. Toggling caveman
		// off when it's already off is a no-op, so we open the RTK tab and flip
		// the preserve_cache_control checkbox to make the form dirty.
		fireEvent.mouseDown(screen.getByTestId("rtk-tab-rtk"));
		fireEvent.click(screen.getByTestId("rtk-field-preserve-cache-control"));
		fireEvent.click(screen.getByTestId("rtk-save-btn"));

		await waitFor(() => expect(mocks.updatePlugin).toHaveBeenCalledTimes(1));
		const call = mocks.updatePlugin.mock.calls.find((c: any) => c[0]?.name === "rtk");
		expect(call).toBeDefined();
		const cfg = (call as any)[0].data.config as any;
		expect(cfg.pipeline).toEqual([{ id: "rtk" }]);
	});

	it("writes pipeline=[{id:'rtk'},{id:'caveman'}] on submit when caveman is enabled", async () => {
		const plugin = makePlugin({ config: { caveman: { enabled: true } } as any });
		render(<RtkFragment plugin={plugin} />);
		// Open the RTK tab and flip a checkbox so Save is enabled.
		fireEvent.mouseDown(screen.getByTestId("rtk-tab-rtk"));
		fireEvent.click(screen.getByTestId("rtk-field-preserve-cache-control"));
		fireEvent.click(screen.getByTestId("rtk-save-btn"));

		await waitFor(() => expect(mocks.updatePlugin).toHaveBeenCalledTimes(1));
		const call = mocks.updatePlugin.mock.calls.find((c: any) => c[0]?.name === "rtk");
		expect(call).toBeDefined();
		const cfg = (call as any)[0].data.config as any;
		expect(cfg.pipeline).toEqual([{ id: "rtk" }, { id: "caveman" }]);
	});
});