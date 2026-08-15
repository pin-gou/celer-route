// @vitest-environment jsdom
/**
 * @file TDD Red Phase — ProviderFilters component tests
 *
 * These tests verify the ProviderFilters component:
 * - Search input triggers onChange with correct search term
 * - Health status chips toggle correctly
 * - onChange callback is called with combined filter parameters
 *
 * In the TDD red phase, the ProviderFilters component does not exist yet,
 * so these tests will fail at compile time (cannot find module).
 * This is the expected result — the dev phase will implement the component.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

// The following import does not exist yet — this is TDD red phase.
// Compilation will fail with "Cannot find module" error.
import { ProviderFilters } from "./ProviderFilters";
import type { ProviderFiltersProps, FilterState } from "./ProviderFilters";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const defaultFilters: FilterState = {
	search: "",
	health: "all",
};

describe("ProviderFilters", () => {
	// -----------------------------------------------------------------------
	// Search input
	// -----------------------------------------------------------------------

	it("should render a search input", () => {
		const onChange = vi.fn();
		render(<ProviderFilters filters={defaultFilters} onChange={onChange} />);

		const searchInput = screen.getByTestId("providers2-filter-search");
		expect(searchInput).toBeTruthy();
	});

	it("should call onChange with search term when typing in search input", () => {
		const onChange = vi.fn();
		render(<ProviderFilters filters={defaultFilters} onChange={onChange} />);

		const searchInput = screen.getByTestId("providers2-filter-search");
		fireEvent.change(searchInput, { target: { value: "openai" } });

		expect(onChange).toHaveBeenCalledTimes(1);
		expect(onChange).toHaveBeenCalledWith({
			...defaultFilters,
			search: "openai",
		});
	});

	it("should render the placeholder text in the search input", () => {
		const onChange = vi.fn();
		render(<ProviderFilters filters={defaultFilters} onChange={onChange} />);

		const searchInput = screen.getByTestId("providers2-filter-search");
		const placeholder = searchInput.getAttribute("placeholder");
		expect(placeholder?.toLowerCase()).toContain("search");
	});

	// -----------------------------------------------------------------------
	// Health status chips
	// -----------------------------------------------------------------------

	it("should render health status chips", () => {
		const onChange = vi.fn();
		render(<ProviderFilters filters={defaultFilters} onChange={onChange} />);

		const allChip = screen.getByTestId("providers2-filter-chip-all");
		const activeChip = screen.getByTestId("providers2-filter-chip-active");
		const errorChip = screen.getByTestId("providers2-filter-chip-error");

		expect(allChip).toBeTruthy();
		expect(activeChip).toBeTruthy();
		expect(errorChip).toBeTruthy();
	});

	it("should mark the 'all' chip as active by default", () => {
		const onChange = vi.fn();
		render(<ProviderFilters filters={defaultFilters} onChange={onChange} />);

		const allChip = screen.getByTestId("providers2-filter-chip-all");
		expect(allChip.getAttribute("data-active")).toBe("true");
	});

	it("should call onChange with health='active' when clicking active chip", () => {
		const onChange = vi.fn();
		render(<ProviderFilters filters={defaultFilters} onChange={onChange} />);

		fireEvent.click(screen.getByTestId("providers2-filter-chip-active"));
		expect(onChange).toHaveBeenCalledWith({
			...defaultFilters,
			health: "active",
		});
	});

	it("should call onChange with health='error' when clicking error chip", () => {
		const onChange = vi.fn();
		render(<ProviderFilters filters={defaultFilters} onChange={onChange} />);

		fireEvent.click(screen.getByTestId("providers2-filter-chip-error"));
		expect(onChange).toHaveBeenCalledWith({
			...defaultFilters,
			health: "error",
		});
	});

	it("should mark the active chip as selected and others as unselected", () => {
		const onChange = vi.fn();
		const activeFilters: FilterState = { search: "", health: "active" };

		render(<ProviderFilters filters={activeFilters} onChange={onChange} />);

		const allChip = screen.getByTestId("providers2-filter-chip-all");
		const activeChip = screen.getByTestId("providers2-filter-chip-active");
		const errorChip = screen.getByTestId("providers2-filter-chip-error");

		expect(allChip.getAttribute("data-active")).toBe("false");
		expect(activeChip.getAttribute("data-active")).toBe("true");
		expect(errorChip.getAttribute("data-active")).toBe("false");
	});

	// -----------------------------------------------------------------------
	// Combined filter state
	// -----------------------------------------------------------------------

	it("should combine search term and health chip selection when both are active", () => {
		const onChange = vi.fn();
		const { rerender } = render(
			<ProviderFilters filters={defaultFilters} onChange={onChange} />,
		);

		// First change search term
		fireEvent.change(screen.getByTestId("providers2-filter-search"), {
			target: { value: "openai" },
		});
		expect(onChange).toHaveBeenCalledWith({
			search: "openai",
			health: "all",
		});

		// Rerender with updated filters
		const updatedFilters: FilterState = { search: "openai", health: "all" };
		rerender(<ProviderFilters filters={updatedFilters} onChange={onChange} />);

		// Then click active chip
		fireEvent.click(screen.getByTestId("providers2-filter-chip-active"));
		expect(onChange).toHaveBeenCalledWith({
			search: "openai",
			health: "active",
		});
	});

	it("should clear search when input is emptied", () => {
		const onChange = vi.fn();
		const filtersWithSearch: FilterState = { search: "openai", health: "all" };

		const { rerender } = render(
			<ProviderFilters filters={filtersWithSearch} onChange={onChange} />,
		);

		// Clear the search input
		fireEvent.change(screen.getByTestId("providers2-filter-search"), {
			target: { value: "" },
		});
		expect(onChange).toHaveBeenCalledWith({
			search: "",
			health: "all",
		});
	});

	// -----------------------------------------------------------------------
	// Accessibility
	// -----------------------------------------------------------------------

	it("should have accessible labels for search input and chips", () => {
		const onChange = vi.fn();
		render(<ProviderFilters filters={defaultFilters} onChange={onChange} />);

		const searchInput = screen.getByTestId("providers2-filter-search");
		expect(searchInput.getAttribute("aria-label")).toBeTruthy();

		const allChip = screen.getByTestId("providers2-filter-chip-all");
		expect(allChip.getAttribute("aria-label")).toBeTruthy();
	});
});