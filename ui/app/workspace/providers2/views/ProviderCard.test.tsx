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
	today_requests: 1284,
	today_errors: 3,
	last_error_at: "2026-08-15T00:15:22Z",
	uptime: 0.998,
	avg_latency_ms: 312,
	keys_health_status: "healthy",
};

const mockOnToggle = vi.fn();
const mockOnQuickTest = vi.fn();

describe("ProviderCard", () => {
	// -----------------------------------------------------------------------
	// Basic rendering
	// -----------------------------------------------------------------------

	it("should render provider icon", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		// Provider icon should be rendered with a data-testid
		const icon = screen.getByTestId("providers2-card-icon-openai");
		expect(icon).toBeTruthy();
	});

	it("should render provider name", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		expect(screen.getByText("openai")).toBeTruthy();
	});

	it("should render health badge with correct status", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		const badge = screen.getByTestId("providers2-card-health-badge");
		expect(badge).toBeTruthy();
		expect(badge.getAttribute("data-health-status")).toBe("healthy");
	});

	// -----------------------------------------------------------------------
	// Aggregated stats
	// -----------------------------------------------------------------------

	it("should render keys count as '3 keys'", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		expect(screen.getByText(/3 keys/i)).toBeTruthy();
	});

	it("should render models count as '47 models'", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		expect(screen.getByText(/47 models/i)).toBeTruthy();
	});

	it("should render today requests count as '1284 reqs'", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		expect(screen.getByText(/1284/i)).toBeTruthy();
	});

	it("should render today errors count", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		expect(screen.getByText(/3 err/i)).toBeTruthy();
	});

	it("should render last error time when last_error_at is present", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		expect(screen.getByTestId("providers2-card-last-error")).toBeTruthy();
	});

	// -----------------------------------------------------------------------
	// Interactive elements
	// -----------------------------------------------------------------------

	it("should render a toggle switch for bulk enable/disable", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		const toggle = screen.getByTestId("providers2-card-toggle");
		expect(toggle).toBeTruthy();
	});

	it("should call onToggle when toggle is clicked", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		fireEvent.click(screen.getByTestId("providers2-card-toggle"));
		expect(mockOnToggle).toHaveBeenCalledTimes(1);
	});

	it("should render a Quick test button", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		const quickTestBtn = screen.getByTestId("providers2-card-quick-test");
		expect(quickTestBtn).toBeTruthy();
	});

	it("should call onQuickTest when Quick test button is clicked", () => {
		render(
			<ProviderCard
				provider={mockProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		fireEvent.click(screen.getByTestId("providers2-card-quick-test"));
		expect(mockOnQuickTest).toHaveBeenCalledTimes(1);
	});

	// -----------------------------------------------------------------------
	// Edge cases
	// -----------------------------------------------------------------------

	it("should not show last error time when last_error_at is null", () => {
		const providerWithoutError = {
			...mockProvider,
			last_error_at: null,
		};

		render(
			<ProviderCard
				provider={providerWithoutError}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		expect(screen.queryByTestId("providers2-card-last-error")).toBeNull();
	});

	it("should render as disabled when provider_status is 'error'", () => {
		const erroredProvider = {
			...mockProvider,
			provider_status: "error" as const,
			keys_health_status: "degraded" as const,
		};

		render(
			<ProviderCard
				provider={erroredProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		const badge = screen.getByTestId("providers2-card-health-badge");
		expect(badge.getAttribute("data-health-status")).toBe("degraded");
	});

	it("should render zero counts when no data is available", () => {
		const emptyProvider = {
			...mockProvider,
			keys_count: 0,
			models_count: 0,
			today_requests: 0,
			today_errors: 0,
			last_error_at: null,
		};

		render(
			<ProviderCard
				provider={emptyProvider}
				onToggle={mockOnToggle}
				onQuickTest={mockOnQuickTest}
			/>,
		);

		expect(screen.getByText(/0 keys/i)).toBeTruthy();
		expect(screen.getByText(/0 models/i)).toBeTruthy();
		expect(screen.getByText(/0 reqs/i)).toBeTruthy();
	});
});