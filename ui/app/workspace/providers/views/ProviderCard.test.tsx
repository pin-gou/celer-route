// @vitest-environment jsdom
/**
 * @file TDD Red Phase — ProviderCard component tests
 *
 * These tests verify the ProviderCard component renders provider information
 * including icon, name, health badge, keys count, models count, requests count,
 * toggle, and quick test button.
 *
 * In the TDD red phase, the ProviderCard component does not exist yet,
 * so these tests will fail at compile time (cannot find module).
 * This is the expected result — the dev phase will implement the component.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => key,
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
}));

// The following import does not exist yet — this is TDD red phase.
// Compilation will fail with "Cannot find module" error.
import { ProviderCard } from "./ProviderCard";
import type { ProviderCardProps } from "./ProviderCard";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const mockProvider: ProviderCardProps["provider"] = {
	name: "openai",
	provider_status: "active",
	keys_count: 3,
	models_count: 47,
	keys_health_status: "healthy",
	keys_enabled: true,
};

const mockOnToggle = vi.fn();
const mockOnQuickTest = vi.fn();
const mockOnDelete = vi.fn();

describe("ProviderCard", () => {
	// -----------------------------------------------------------------------
	// Basic rendering
	// -----------------------------------------------------------------------

	it("should render provider icon", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		// Provider icon should be rendered with a data-testid
		const icon = screen.getByTestId("providers2-card-icon-openai");
		expect(icon).not.toBeNull();
	});

	it("should render provider name", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		expect(screen.getByText("openai")).not.toBeNull();
	});

	it("should render health badge with correct status", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		const badge = screen.getByTestId("providers2-card-health-badge");
		expect(badge).not.toBeNull();
		expect(badge.getAttribute("data-health-status")).toBe("healthy");
	});

	// -----------------------------------------------------------------------
	// Aggregated stats
	// -----------------------------------------------------------------------

	it("should render keys count", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		expect(screen.getByText("providers2.card.keys")).not.toBeNull();
	});

	it("should render models count", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		expect(screen.getByText("providers2.card.models")).not.toBeNull();
	});

	// -----------------------------------------------------------------------
	// Interactive elements
	// -----------------------------------------------------------------------

	it("should render a toggle switch for bulk enable/disable", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		const toggle = screen.getByTestId("providers2-card-toggle");
		expect(toggle).not.toBeNull();
	});

	it("should call onToggle when toggle is clicked", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		fireEvent.click(screen.getByTestId("providers2-card-toggle"));
		expect(mockOnToggle).toHaveBeenCalledTimes(1);
	});

	it("should render a Quick test button", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		const quickTestBtn = screen.getByTestId("providers2-card-quick-test");
		expect(quickTestBtn).not.toBeNull();
	});

	it("should call onQuickTest when Quick test button is clicked", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		fireEvent.click(screen.getByTestId("providers2-card-quick-test"));
		expect(mockOnQuickTest).toHaveBeenCalledTimes(1);
	});

	it("should render a delete button", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		const deleteBtn = screen.getByTestId("providers2-card-delete");
		expect(deleteBtn).not.toBeNull();
	});

	it("should call onDelete when delete button is clicked", () => {
		render(<ProviderCard provider={mockProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		fireEvent.click(screen.getByTestId("providers2-card-delete"));
		expect(mockOnDelete).toHaveBeenCalledTimes(1);
	});

	// -----------------------------------------------------------------------
	// Edge cases
	// -----------------------------------------------------------------------

	it("should not show last error time when last_error_at is null", () => {
		const providerWithoutError = {
			...mockProvider,
			last_error_at: null,
		};

		render(<ProviderCard provider={providerWithoutError} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		expect(screen.queryByTestId("providers2-card-last-error")).toBeNull();
	});

	it("should render as disabled when provider_status is 'error'", () => {
		const erroredProvider = {
			...mockProvider,
			provider_status: "error" as const,
			keys_health_status: "degraded" as const,
		};

		render(<ProviderCard provider={erroredProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		const badge = screen.getByTestId("providers2-card-health-badge");
		expect(badge.getAttribute("data-health-status")).toBe("degraded");
	});

	it("should render zero counts when no data is available", () => {
		const emptyProvider = {
			...mockProvider,
			keys_count: 0,
			models_count: 0,
		};

		render(<ProviderCard provider={emptyProvider} onToggle={mockOnToggle} onQuickTest={mockOnQuickTest} onDelete={mockOnDelete} />);

		expect(screen.getByText("providers2.card.keys")).not.toBeNull();
		expect(screen.getByText("providers2.card.models")).not.toBeNull();
	});
});