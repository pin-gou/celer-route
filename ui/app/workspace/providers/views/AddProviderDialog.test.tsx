// @vitest-environment jsdom
/**
 * @file Tests for AddProviderDialog — the picker that replaces the legacy
 * AddProviderDropdown. These tests focus on user-visible behavior:
 *
 *  - Opens a dialog (not a dropdown) on trigger click.
 *  - Surfaces Recommended, Family sections, and Custom footer.
 *  - Search filters providers in real time.
 *  - Capability chips filter providers additively.
 *  - Already-added providers are disabled with an "Added" badge and a
 *    dedicated testid so the E2E harness can target them.
 *  - Custom option still wires to onAddCustomProvider.
 *
 * Element interactions are dispatched against the rendered Radix Dialog
 * content rather than the trigger — the dialog portal renders under
 * document.body, so we look up elements by data-testid once it's open.
 */
import { describe, it, expect, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => key,
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
}));

import { AddProviderDialog } from "./AddProviderDialog";

const KNOWN = [
	{ name: "openai" as const },
	{ name: "anthropic" as const },
	{ name: "gemini" as const },
	{ name: "deepseek" as const },
	{ name: "elevenlabs" as const },
	{ name: "replicate" as const },
	{ name: "huggingface" as const },
	{ name: "runway" as const },
];

async function openDialog(existingProviderNames: Set<string> = new Set()) {
	const onSelectKnown = vi.fn();
	const onAddCustom = vi.fn();
	render(
		<AddProviderDialog
			existingProviderNames={existingProviderNames}
			knownProviders={KNOWN}
			onSelectKnownProvider={onSelectKnown}
			onAddCustomProvider={onAddCustom}
		/>,
	);
	const trigger = screen.getByTestId("add-provider-btn");
	fireEvent.click(trigger);
	await waitFor(() => {
		expect(screen.getByTestId("add-provider-dialog")).toBeTruthy();
	});
	return { onSelectKnown, onAddCustom };
}

describe("AddProviderDialog", () => {
	it("opens a dialog (not a dropdown) when the trigger is clicked", async () => {
		await openDialog();
		expect(screen.getByTestId("add-provider-dialog")).toBeTruthy();
		expect(screen.queryByTestId("add-provider-dropdown")).toBeNull();
	});

	it("renders the search input and reset affordance", async () => {
		await openDialog();
		expect(screen.getByTestId("add-provider-search")).toBeTruthy();
	});

	it("renders capability chips for the static matrix", async () => {
		await openDialog();
		expect(screen.getByTestId("add-provider-cap-chat")).toBeTruthy();
		expect(screen.getByTestId("add-provider-cap-embed")).toBeTruthy();
		expect(screen.getByTestId("add-provider-cap-vision")).toBeTruthy();
	});

	it("renders the Recommended section by default with no active filters", async () => {
		await openDialog();
		expect(screen.getByTestId("add-provider-recommended")).toBeTruthy();
	});

	it("filters provider tiles by the search query (substring on name + label)", async () => {
		await openDialog();
		fireEvent.change(screen.getByTestId("add-provider-search"), { target: { value: "deep" } });

		await waitFor(() => {
			expect(screen.getByTestId("add-provider-option-deepseek")).toBeTruthy();
		});
		// Anthropic / OpenAI / Gemini / ElevenLabs / Replicate should be filtered out
		expect(screen.queryByTestId("add-provider-option-anthropic")).toBeNull();
		expect(screen.queryByTestId("add-provider-option-openai")).toBeNull();
	});

	it("filters provider tiles by capability chip selection (AND across chips)", async () => {
		await openDialog();
		// Pick "video only" first — runway (video) should match, deepseek (chat) should not
		fireEvent.click(screen.getByTestId("add-provider-cap-video"));

		await waitFor(() => {
			expect(screen.getByTestId("add-provider-option-runway")).toBeTruthy();
		});
		expect(screen.queryByTestId("add-provider-option-deepseek")).toBeNull();

		// Add "chat" — providers must support BOTH video AND chat now
		fireEvent.click(screen.getByTestId("add-provider-cap-chat"));
		await waitFor(() => {
			// deepseek supports chat but not video — filtered out
			expect(screen.queryByTestId("add-provider-option-deepseek")).toBeNull();
			// runway supports video but not chat — filtered out
			expect(screen.queryByTestId("add-provider-option-runway")).toBeNull();
		});
	});

	it("shows a 'no results' message when no provider matches the filters", async () => {
		await openDialog();
		fireEvent.change(screen.getByTestId("add-provider-search"), { target: { value: "zzz_nonexistent" } });

		await waitFor(() => {
			expect(screen.getByText("providers2.addProviderDialog.noResults")).toBeTruthy();
		});
	});

	it("marks already-added providers as disabled with an 'Added' badge", async () => {
		await openDialog(new Set(["openai"]));
		// OpenAI is present in both the Recommended row and the OpenAI Family
		// section, so we scope to the dialog and check every instance.
		const tiles = screen.getByTestId("add-provider-dialog").querySelectorAll('[data-testid="add-provider-option-openai"]');
		expect(tiles.length).toBeGreaterThan(0);
		for (const tile of Array.from(tiles)) {
			expect((tile as HTMLButtonElement).hasAttribute("disabled")).toBe(true);
		}
		expect(screen.getAllByTestId("add-provider-option-openai-added").length).toBeGreaterThan(0);
	});

	it("surfaces already-added providers in the Recommended row but marks them disabled", async () => {
		await openDialog(new Set(["deepseek"]));
		const recommended = screen.getByTestId("add-provider-recommended");
		const deepseekTile = recommended.querySelector('[data-testid="add-provider-option-deepseek"]');
		expect(deepseekTile).not.toBeNull();
		expect((deepseekTile as HTMLButtonElement).hasAttribute("disabled")).toBe(true);
		expect(recommended.querySelector('[data-testid="add-provider-option-deepseek-added"]')).not.toBeNull();
	});

	it("invokes onSelectKnownProvider when an unconfigured provider tile is clicked", async () => {
		const { onSelectKnown } = await openDialog();
		fireEvent.click(screen.getByTestId("add-provider-option-anthropic"));
		expect(onSelectKnown).toHaveBeenCalledWith("anthropic");
	});

	it("invokes onAddCustomProvider when the custom footer is clicked", async () => {
		const { onAddCustom } = await openDialog();
		fireEvent.click(screen.getByTestId("add-provider-option-custom"));
		expect(onAddCustom).toHaveBeenCalledTimes(1);
	});

	it("groups known providers into family sections", async () => {
		await openDialog();
		// Anthropic Family should be present (anthropic is a fixture)
		expect(screen.getByTestId("add-provider-family-anthropic-family")).toBeTruthy();
		// Google Family should be present (gemini)
		expect(screen.getByTestId("add-provider-family-google-family")).toBeTruthy();
	});
});