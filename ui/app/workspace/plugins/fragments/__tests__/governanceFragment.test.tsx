// @vitest-environment jsdom
/**
 * @file TDD Red Phase — GovernanceFragment tests (dev.ui task 1.1–1.4)
 *
 * Contract (design.md "组件设计" + "数据模型"):
 *   - ui/app/workspace/plugins/fragments/governanceFragment.tsx exports:
 *       GovernanceFragment (default + named), EnabledSwitch, ConfigForm
 *     GovernanceFragment composes EnabledSwitch + ConfigForm and renders a root
 *     div with data-testid="governance-fragment".
 *   - ui/lib/types/plugins.ts exports:
 *       GOVERNANCE_PLUGIN = "governance"
 *       governanceConfigSchema — all 4 fields optional:
 *         is_vk_mandatory:           boolean
 *         required_headers:          string[]
 *         disable_auto_tool_inject:  boolean
 *         routing_chain_max_depth:   int ≥ 1 && ≤ 100
 *   - EnabledSwitch on change →
 *       useUpdatePluginMutation({ name: GOVERNANCE_PLUGIN, data: { enabled } })
 *   - ConfigForm defaultValues (fallbacks for Go pointer fields):
 *       is_vk_mandatory:          config.is_vk_mandatory          ?? false      (*bool)
 *       required_headers:         config.required_headers         ?? []         (*[]string)
 *       disable_auto_tool_inject: config.disable_auto_tool_inject ?? false      (*bool)
 *       routing_chain_max_depth:  config.routing_chain_max_depth  ?? 5          (*int)
 *   - ConfigForm submit →
 *       useUpdatePluginMutation({ name: GOVERNANCE_PLUGIN, data: { enabled: plugin.enabled, config: values } })
 *   - EnabledSwitch + ConfigForm are gated by useRbac(RbacResource.Plugins,
 *     RbacOperation.Update): the switch is disabled and no mutation fires when
 *     RBAC denies update access.
 *
 * In the TDD red phase neither the fragment module nor the schema/constant
 * exports exist yet — this file is expected to fail at load time
 * ("Failed to resolve import"). This is the expected red-phase result; the
 * dev phase implements the contract.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Red phase: these imports reference code that does not exist yet.
// Vitest will error with "Failed to resolve import" — this is the expected
// TDD red-phase result. The dev phase implements these modules/exports.
// ---------------------------------------------------------------------------
import { GovernanceFragment, EnabledSwitch, ConfigForm } from "../governanceFragment";
import { governanceConfigSchema, GOVERNANCE_PLUGIN, type Plugin } from "@/lib/types/plugins";

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

const mocks = vi.hoisted(() => ({
	// useUpdatePluginMutation trigger — returns a thenable with unwrap() so the
	// component's `await trigger(...).unwrap()` success path executes cleanly.
	updatePlugin: vi.fn(() => ({ unwrap: () => Promise.resolve({ ok: true }) })),
	// useRbac result — per-test mutable so RBAC interception can be exercised.
	hasUpdateAccess: true,
}));

vi.mock("@/lib/store/apis/pluginsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/pluginsApi")>();
	return {
		...actual,
		useUpdatePluginMutation: () => [mocks.updatePlugin, { isLoading: false }],
	};
});

vi.mock("@/lib/rbac", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/rbac")>();
	return {
		...actual,
		useRbac: () => mocks.hasUpdateAccess,
	};
});

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const fullConfigPlugin: Plugin = {
	name: GOVERNANCE_PLUGIN,
	actualName: GOVERNANCE_PLUGIN,
	enabled: true,
	isCustom: false,
	config: {
		is_vk_mandatory: true,
		required_headers: ["x-org-id", "x-team-id"],
		disable_auto_tool_inject: true,
		routing_chain_max_depth: 8,
	},
	status: {
		name: GOVERNANCE_PLUGIN,
		status: "active",
		logs: [],
		types: ["llm", "http"],
	},
};

// Same plugin but with an empty config — exercises the Go pointer-field
// (nil) fallbacks: *bool → false, *[]string → [], *int → 5.
const emptyConfigPlugin: Plugin = {
	...fullConfigPlugin,
	config: {},
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("GovernanceFragment — composes EnabledSwitch + ConfigForm (task 1.1)", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
		mocks.hasUpdateAccess = true;
	});

	it("renders the fragment root, the enabled switch, the is-vk-mandatory label and the save button", () => {
		render(<GovernanceFragment plugin={fullConfigPlugin} />);

		expect(screen.getByTestId("governance-fragment")).toBeTruthy();
		expect(screen.getByTestId("governance-enabled-switch")).toBeTruthy();
		expect(screen.getByTestId("governance-field-is-vk-mandatory-label")).toBeTruthy();
		expect(screen.getByTestId("governance-save-button")).toBeTruthy();
	});
});

describe("EnabledSwitch — toggle triggers updatePlugin + RBAC interception (task 1.1)", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
		mocks.hasUpdateAccess = true;
	});

	it("renders the switch reflecting the plugin's enabled state", () => {
		render(<EnabledSwitch plugin={fullConfigPlugin} />);

		const switchEl = screen.getByTestId("governance-enabled-switch");
		expect(switchEl.getAttribute("data-state")).toBe("checked");
	});

	it("triggers useUpdatePluginMutation with { name: GOVERNANCE_PLUGIN, data: { enabled: false } } when switched off", () => {
		render(<EnabledSwitch plugin={fullConfigPlugin} />);

		fireEvent.click(screen.getByTestId("governance-enabled-switch"));
		expect(mocks.updatePlugin).toHaveBeenCalledWith({
			name: GOVERNANCE_PLUGIN,
			data: { enabled: false },
		});
	});

	it("does not trigger the mutation when RBAC denies Plugins:Update access", () => {
		mocks.hasUpdateAccess = false;
		render(<EnabledSwitch plugin={fullConfigPlugin} />);

		const switchEl = screen.getByTestId("governance-enabled-switch");
		expect((switchEl as HTMLButtonElement).disabled).toBe(true);

		fireEvent.click(switchEl);
		expect(mocks.updatePlugin).not.toHaveBeenCalled();
	});
});

describe("ConfigForm — initial values from plugin.config with pointer-field fallbacks (task 1.2)", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
		mocks.hasUpdateAccess = true;
	});

	it("fills the 4 field values from plugin.config", () => {
		render(<ConfigForm plugin={fullConfigPlugin} />);

		// is_vk_mandatory: *bool true → switch checked
		expect(screen.getByTestId("governance-field-is-vk-mandatory").getAttribute("data-state")).toBe("checked");

		// required_headers: *[]string ["x-org-id", "x-team-id"] → both tags rendered
		expect(screen.getByText("x-org-id")).toBeTruthy();
		expect(screen.getByText("x-team-id")).toBeTruthy();

		// disable_auto_tool_inject: *bool true → switch checked
		expect(screen.getByTestId("governance-field-disable-auto-tool-inject").getAttribute("data-state")).toBe("checked");

		// routing_chain_max_depth: *int 8 → input shows 8
		expect((screen.getByTestId("governance-field-routing-chain-max-depth") as HTMLInputElement).value).toBe("8");
	});

	it("falls back to false / [] / 5 when config fields are unset (Go nil-pointer semantics)", () => {
		render(<ConfigForm plugin={emptyConfigPlugin} />);

		// *bool unset → false → switches unchecked
		expect(screen.getByTestId("governance-field-is-vk-mandatory").getAttribute("data-state")).toBe("unchecked");
		expect(screen.getByTestId("governance-field-disable-auto-tool-inject").getAttribute("data-state")).toBe("unchecked");

		// *[]string unset → [] → no tags rendered
		expect(screen.queryByText("x-org-id")).toBeNull();
		expect(screen.queryByText("x-team-id")).toBeNull();

		// *int unset → 5 → input shows 5
		expect((screen.getByTestId("governance-field-routing-chain-max-depth") as HTMLInputElement).value).toBe("5");
	});

	it("removes a required_headers tag when its close button is clicked", () => {
		render(<ConfigForm plugin={fullConfigPlugin} />);

		// Start with 2 configured header tags
		expect(screen.getByText("x-org-id")).toBeTruthy();
		expect(screen.getByText("x-team-id")).toBeTruthy();

		fireEvent.click(screen.getByRole("button", { name: "Remove x-org-id" }));

		expect(screen.queryByText("x-org-id")).toBeNull();
		expect(screen.getByText("x-team-id")).toBeTruthy();
	});
});

describe("ConfigForm — submit triggers updatePlugin with name + { enabled, config } (task 1.3)", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
		mocks.hasUpdateAccess = true;
	});

	it("submits the 4-field config through useUpdatePluginMutation", async () => {
		// Start from a plugin with only is_vk_mandatory set (true); every other
		// pointer field is nil so the fallbacks ([] / false / 5) are in play.
		const plugin: Plugin = {
			...fullConfigPlugin,
			enabled: true,
			config: { is_vk_mandatory: true },
		};
		render(<ConfigForm plugin={plugin} />);

		// 1. is_vk_mandatory: true → click the switch → false
		fireEvent.click(screen.getByTestId("governance-field-is-vk-mandatory"));

		// 2. required_headers: [] → add "x-org-id" via the TagInput (Enter commits)
		const headerInput = screen.getByTestId("governance-field-required-headers-input");
		fireEvent.change(headerInput, { target: { value: "x-org-id" } });
		fireEvent.keyDown(headerInput, { key: "Enter", code: "Enter", keyCode: 13 });

		// 3. routing_chain_max_depth: 5 (fallback) → 10
		const depthInput = screen.getByTestId("governance-field-routing-chain-max-depth") as HTMLInputElement;
		fireEvent.change(depthInput, { target: { value: "10" } });

		// 4. disable_auto_tool_inject stays at its fallback false

		// Save
		fireEvent.click(screen.getByTestId("governance-save-button"));

		await waitFor(() => {
			expect(mocks.updatePlugin).toHaveBeenCalledWith({
				name: GOVERNANCE_PLUGIN,
				data: {
					enabled: true, // plugin.enabled is preserved unchanged
					config: {
						is_vk_mandatory: false,
						required_headers: ["x-org-id"],
						disable_auto_tool_inject: false,
						routing_chain_max_depth: 10,
					},
				},
			});
		});
	});
});

describe("governanceConfigSchema — zod validation (task 1.4)", () => {
	// -----------------------------------------------------------------------
	// Missing fields may not error — all four fields are optional so a partial
	// PUT can only merge the fields the form knows about.
	// -----------------------------------------------------------------------

	it("accepts an empty object (missing fields do not error)", () => {
		const result = governanceConfigSchema.safeParse({});
		expect(result.success).toBe(true);
	});

	it("accepts a single-field object", () => {
		const result = governanceConfigSchema.safeParse({ routing_chain_max_depth: 10 });
		expect(result.success).toBe(true);
	});

	it("accepts a valid config with all 4 fields", () => {
		const result = governanceConfigSchema.safeParse({
			is_vk_mandatory: true,
			required_headers: ["x-org-id", "x-team-id"],
			disable_auto_tool_inject: false,
			routing_chain_max_depth: 5,
		});
		expect(result.success).toBe(true);
	});

	// -----------------------------------------------------------------------
	// Type mismatches must error
	// -----------------------------------------------------------------------

	it("rejects is_vk_mandatory when its type mismatches", () => {
		const result = governanceConfigSchema.safeParse({ is_vk_mandatory: "yes" });
		expect(result.success).toBe(false);
	});

	it("rejects required_headers when it is not an array", () => {
		const result = governanceConfigSchema.safeParse({ required_headers: "x-org-id" });
		expect(result.success).toBe(false);
	});

	it("rejects disable_auto_tool_inject when its type mismatches", () => {
		const result = governanceConfigSchema.safeParse({ disable_auto_tool_inject: 1 });
		expect(result.success).toBe(false);
	});

	it("rejects routing_chain_max_depth when its type mismatches", () => {
		const result = governanceConfigSchema.safeParse({ routing_chain_max_depth: "5" });
		expect(result.success).toBe(false);
	});

	// -----------------------------------------------------------------------
	// routing_chain_max_depth bounds (int ≥ 1 && ≤ 100)
	// -----------------------------------------------------------------------

	it("rejects routing_chain_max_depth greater than 100", () => {
		const result = governanceConfigSchema.safeParse({ routing_chain_max_depth: 101 });
		expect(result.success).toBe(false);
	});

	it("accepts routing_chain_max_depth exactly 100", () => {
		const result = governanceConfigSchema.safeParse({ routing_chain_max_depth: 100 });
		expect(result.success).toBe(true);
	});

	it("rejects routing_chain_max_depth below 1", () => {
		const result = governanceConfigSchema.safeParse({ routing_chain_max_depth: 0 });
		expect(result.success).toBe(false);
	});

	it("rejects routing_chain_max_depth when it is not an integer", () => {
		const result = governanceConfigSchema.safeParse({ routing_chain_max_depth: 5.5 });
		expect(result.success).toBe(false);
	});
});