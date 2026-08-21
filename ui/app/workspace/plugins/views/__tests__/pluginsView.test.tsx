// @vitest-environment jsdom
/**
 * @file TDD Red Phase — PluginsView dispatch logic + placeholder card tests (dev.ui task 11.2)
 *
 * Contract (design.md "组件设计 / pluginsView.tsx 散转逻辑扩展"):
 *   - PluginsView dispatches to the correct fragment based on selectedPlugin.name:
 *       governance          → GovernanceFragment
 *       provider-cooldown   → ProvidercooldownFragment
 *       rtk                 → RtkFragment
 *       otel                → OtelView
 *       logging             → LoggingFragment (new)
 *       semantic_cache      → SemanticCacheFragment (new)
 *       mocker              → MockerFragment (new)
 *       compat              → CompatFragment (new)
 *       prompts             → PromptsFragment (placeholder card, new)
 *       modelcatalogresolver → ModelcatalogresolverFragment (placeholder card, new)
 *       jsonparser          → JsonparserFragment (placeholder card, new)
 *   - Placeholder cards render a Card with title, description, and a navigate button
 *
 * In the TDD red phase the new fragments are not yet created — this file is
 * expected to fail at load time ("Failed to resolve import"). This is the
 * expected TDD red-phase result.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Red phase: the component imports from paths that do not exist yet.
// The import of PluginsView itself will work (it exists), but the placeholder
// fragments imported below don't exist yet — those imports will fail.
// ---------------------------------------------------------------------------
import PluginsView from "../pluginsView";
import { PromptsFragment, ModelcatalogresolverFragment, JsonparserFragment } from "@/app/workspace/plugins/fragments/promptsFragment";
import { type Plugin } from "@/lib/types/plugins";

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

const mocks = vi.hoisted(() => ({
	updatePlugin: vi.fn(() => ({ unwrap: () => Promise.resolve({ ok: true }) })),
	selectedPlugin: undefined as Plugin | undefined,
	hasUpdateAccess: true,
	hasDeleteAccess: true,
	onDelete: vi.fn(),
	onCreate: vi.fn(),
}));

vi.mock("@/lib/store/apis/pluginsApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/pluginsApi")>();
	return {
		...actual,
		useUpdatePluginMutation: () => [mocks.updatePlugin, { isLoading: false }],
		useDeletePluginMutation: () => [vi.fn(), { isLoading: false }],
		useGetCooldownStateQuery: () => ({ data: { state: [] }, isLoading: false }),
		useGetCooldownStatsQuery: () => ({ data: { stats: { markCount: 0, suppressedCount: 0, activeCount: 0 } }, isLoading: false }),
		useUnfreezeCooldownMutation: () => [vi.fn(), { isLoading: false }],
		useGetRtkStatsQuery: () => ({
			data: {
				stats: {
					invocations: 0,
					compressedCount: 0,
					originalTokens: 0,
					compressedTokens: 0,
					tokensSaved: 0,
					compressionRatio: 0,
				},
			},
			isLoading: false,
		}),
		useGetPluginsQuery: () => ({ data: [], isLoading: false }),
		useGetLoadedPluginsQuery: () => ({ data: [], isLoading: false }),
		useGetPluginQuery: () => ({ data: undefined, isLoading: false }),
	};
});

vi.mock("@/lib/store/apis/providersApi", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store/apis/providersApi")>();
	return {
		...actual,
		useGetProvidersQuery: () => ({ data: [], isLoading: false }),
	};
});

vi.mock("@/lib/rbac", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/rbac")>();
	return {
		...actual,
		useRbac: () => mocks.hasUpdateAccess,
	};
});

vi.mock("@/lib/store", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/store")>();
	return {
		...actual,
		useAppSelector: (selector: (state: unknown) => unknown) => {
			const state = { plugin: { selectedPlugin: mocks.selectedPlugin, isDirty: false } };
			return selector(state);
		},
		useAppDispatch: () => vi.fn(),
		setPluginFormDirtyState: vi.fn(),
	};
});

// Mock translation
vi.mock("react-i18next", async (importOriginal) => {
	const actual = await importOriginal<typeof import("react-i18next")>();
	return {
		...actual,
		useTranslation: () => ({
			t: (key: string) => key,
			i18n: { language: "en" },
		}),
	};
});

// Mock the route navigation
vi.mock("@tanstack/react-router", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@tanstack/react-router")>();
	return {
		...actual,
		useNavigate: () => vi.fn(),
		Link: ({ children, ...props }: any) => (
			<a data-mock-link="true" href={props.to || "#"}>
				{children}
			</a>
		),
	};
});

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const basePlugin: Plugin = {
	name: "",
	actualName: "",
	enabled: true,
	isCustom: false,
	config: {},
	status: {
		name: "",
		status: "active",
		logs: [],
		types: ["llm"],
	},
};

// ---------------------------------------------------------------------------
// Tests — Dispatch logic
// ---------------------------------------------------------------------------

describe("PluginsView — dispatch logic (task 11.2)", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
		mocks.hasUpdateAccess = true;
		mocks.hasDeleteAccess = true;
		mocks.onDelete.mockReset();
		mocks.onCreate.mockReset();
		mocks.selectedPlugin = undefined;
	});

	it("renders the no-plugin-selected placeholder when no plugin is selected", () => {
		mocks.selectedPlugin = undefined;
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		expect(screen.getByText("pluginForm.noPluginSelected")).toBeTruthy();
	});

	it("renders GovernanceFragment when selectedPlugin.name is 'governance'", () => {
		mocks.selectedPlugin = { ...basePlugin, name: "governance", actualName: "governance" };
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		expect(screen.getByTestId("governance-fragment")).toBeTruthy();
	});

	it("renders ProvidercooldownFragment when selectedPlugin.name is 'provider-cooldown'", () => {
		mocks.selectedPlugin = { ...basePlugin, name: "provider-cooldown", actualName: "provider-cooldown" };
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		// The provider-cooldown fragment renders a default-ttl field
		expect(screen.getByTestId("providercooldown-field-default-ttl")).toBeTruthy();
	});

	it("renders RtkFragment when selectedPlugin.name is 'rtk'", () => {
		mocks.selectedPlugin = { ...basePlugin, name: "rtk", actualName: "rtk" };
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		expect(screen.getByTestId("rtk-fragment")).toBeTruthy();
	});

	it("renders OtelView when selectedPlugin.name is 'otel'", () => {
		mocks.selectedPlugin = { ...basePlugin, name: "otel", actualName: "otel" };
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		expect(screen.getByTestId("otel-fragment")).toBeTruthy();
	});

	it("renders the default form for a custom plugin (not a built-in name)", () => {
		mocks.selectedPlugin = { ...basePlugin, name: "my-custom-plugin", actualName: "my-custom-plugin", isCustom: true };
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		// Default form renders the plugin form title
		expect(screen.getByText("pluginForm.title")).toBeTruthy();
		expect(screen.getByText("pluginForm.nameLabel")).toBeTruthy();
	});

	// -----------------------------------------------------------------------
	// New fragments (to be added in dev phase)
	// -----------------------------------------------------------------------

	it("renders LoggingFragment when selectedPlugin.name is 'logging'", () => {
		mocks.selectedPlugin = { ...basePlugin, name: "logging", actualName: "logging" };
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		expect(screen.getByTestId("logging-fragment")).toBeTruthy();
	});

	it("renders SemanticCacheFragment when selectedPlugin.name is 'semantic_cache'", () => {
		mocks.selectedPlugin = { ...basePlugin, name: "semantic_cache", actualName: "semantic_cache" };
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		expect(screen.getByTestId("semantic-cache-fragment")).toBeTruthy();
	});

	it("renders MockerFragment when selectedPlugin.name is 'mocker'", () => {
		mocks.selectedPlugin = { ...basePlugin, name: "mocker", actualName: "mocker" };
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		expect(screen.getByTestId("mocker-fragment")).toBeTruthy();
	});

	it("renders CompatFragment when selectedPlugin.name is 'compat'", () => {
		mocks.selectedPlugin = { ...basePlugin, name: "compat", actualName: "compat" };
		render(<PluginsView onDelete={mocks.onDelete} onCreate={mocks.onCreate} />);

		expect(screen.getByTestId("compat-fragment")).toBeTruthy();
	});
});

// ---------------------------------------------------------------------------
// Tests — Placeholder cards
// ---------------------------------------------------------------------------

describe("Placeholder cards — prompts / modelcatalogresolver / jsonparser (task 11.2)", () => {
	beforeEach(() => {
		mocks.updatePlugin.mockReset();
	});

	it("PromptsFragment renders a card with title, description and navigate button", () => {
		render(<PromptsFragment />);

		expect(screen.getByTestId("prompts-placeholder-card")).toBeTruthy();
		expect(screen.getByText(/prompts/i)).toBeTruthy();
		expect(screen.getByRole("button", { name: /prompts/i })).toBeTruthy();
	});

	it("ModelcatalogresolverFragment renders a card with title, description and navigate button", () => {
		render(<ModelcatalogresolverFragment />);

		expect(screen.getByTestId("modelcatalogresolver-placeholder-card")).toBeTruthy();
		expect(screen.getByText(/model catalog/i)).toBeTruthy();
		expect(screen.getByRole("button")).toBeTruthy();
	});

	it("JsonparserFragment renders a card with title, description and navigate button", () => {
		render(<JsonparserFragment />);

		expect(screen.getByTestId("jsonparser-placeholder-card")).toBeTruthy();
		expect(screen.getByText(/json/i)).toBeTruthy();
		expect(screen.getByRole("button")).toBeTruthy();
	});
});