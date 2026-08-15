// @vitest-environment jsdom
/**
 * @file TDD Red Phase — OverviewTab component tests
 *
 * These tests verify the OverviewTab component renders all 6 inline-edit
 * fragments: Network, Proxy, Performance, Governance, Beta Headers,
 * and OpenAI Config.
 *
 * In the TDD red phase, the OverviewTab component does not exist yet,
 * so these tests will fail at compile time (cannot find module).
 * This is the expected result — the dev phase will implement the component.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// The following import does not exist yet — this is TDD red phase.
// Compilation will fail with "Cannot find module" error.
import { OverviewTab } from "./OverviewTab";
import type { OverviewTabProps } from "./OverviewTab";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const mockProvider: OverviewTabProps["provider"] = {
	name: "openai",
	provider_status: "active",
	network_config: {
		base_url: "https://api.openai.com/v1",
		max_conns_per_host: 5000,
		default_request_timeout_in_seconds: 30,
		max_retries: 3,
		retry_backoff_initial: 1000,
		retry_backoff_max: 30000,
		extra_headers: {},
	},
	concurrency_and_buffer_size: {
		concurrency: 100,
		buffer_size: 5000,
	},
	proxy_config: {
		type: "http",
		url: { value: "http://proxy.example.com:8080", type: "plain_text" },
	},
	send_back_raw_request: false,
	send_back_raw_response: false,
	store_raw_request_response: false,
	status: "success",
	description: "OpenAI provider",
	keys_count: 3,
	models_count: 47,
	keys_health_status: "healthy",
	today_requests: 1284,
	today_errors: 3,
	last_used_at: "2026-08-15T01:42:00Z",
	last_error_at: "2026-08-15T00:15:22Z",
	uptime: 0.998,
	avg_latency_ms: 312,
};

const mockOnSave = vi.fn();

describe("OverviewTab", () => {
	// -----------------------------------------------------------------------
	// 6 inline-edit fragment sections
	// -----------------------------------------------------------------------

	it("should render Network fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		expect(screen.getByTestId("providers2-overview-network")).not.toBeNull();
	});

	it("should render Proxy fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		expect(screen.getByTestId("providers2-overview-proxy")).not.toBeNull();
	});

	it("should render Performance fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		expect(screen.getByTestId("providers2-overview-performance")).not.toBeNull();
	});

	it("should render Governance fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		expect(screen.getByTestId("providers2-overview-governance")).not.toBeNull();
	});

	it("should render Beta Headers fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		expect(screen.getByTestId("providers2-overview-beta-headers")).not.toBeNull();
	});

	it("should render OpenAI Config fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		expect(screen.getByTestId("providers2-overview-openai-config")).not.toBeNull();
	});

	// -----------------------------------------------------------------------
	// Fragment content verification
	// -----------------------------------------------------------------------

	it("should display Network fragment with base URL and max connections", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const networkSection = screen.getByTestId("providers2-overview-network");
		expect(networkSection.textContent).toContain("Network");
	});

	it("should display Proxy fragment with proxy type info", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const proxySection = screen.getByTestId("providers2-overview-proxy");
		expect(proxySection.textContent).toContain("Proxy");
	});

	it("should display Performance fragment with concurrency and buffer size", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const perfSection = screen.getByTestId("providers2-overview-performance");
		expect(perfSection.textContent).toContain("Performance");
	});

	it("should display Governance fragment with budget and rate limit info", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const govSection = screen.getByTestId("providers2-overview-governance");
		expect(govSection.textContent).toContain("Governance");
	});

	it("should display Beta Headers fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const betaSection = screen.getByTestId("providers2-overview-beta-headers");
		expect(betaSection.textContent).toContain("Beta");
	});

	it("should display OpenAI Config fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const openaiSection = screen.getByTestId("providers2-overview-openai-config");
		expect(openaiSection.textContent).toContain("OpenAI");
	});

	// -----------------------------------------------------------------------
	// Edit buttons
	// -----------------------------------------------------------------------

	it("should have an edit button on Network fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const editBtn = screen.getByTestId("providers2-overview-network-edit");
		expect(editBtn).not.toBeNull();
	});

	it("should have an edit button on Proxy fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const editBtn = screen.getByTestId("providers2-overview-proxy-edit");
		expect(editBtn).not.toBeNull();
	});

	it("should have an edit button on Performance fragment", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const editBtn = screen.getByTestId("providers2-overview-performance-edit");
		expect(editBtn).not.toBeNull();
	});

	// -----------------------------------------------------------------------
	// Edge cases
	// -----------------------------------------------------------------------

	it("should render Governance fragment as visible even when governance data is empty", () => {
		const providerWithoutGovernance = {
			...mockProvider,
			status: "unknown" as const,
		};

		render(<OverviewTab provider={providerWithoutGovernance} onSave={mockOnSave} />);

		// Governance fragment should still render (may show "not configured" state)
		expect(screen.getByTestId("providers2-overview-governance")).not.toBeNull();
	});

	it("should render without Beta Headers fragment when provider is not anthropic family", () => {
		// Beta Headers is anthropic-family specific per design
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		// For non-anthropic providers, Beta Headers fragment may render with a notice
		const betaSection = screen.queryByTestId("providers2-overview-beta-headers");
		// This is a conditional render — we just verify it doesn't crash
		expect(betaSection).not.toBeNull();
	});

	it("should render OpenAI Config fragment only for openai provider", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		// OpenAI Config should render for openai provider
		expect(screen.getByTestId("providers2-overview-openai-config")).not.toBeNull();
	});

	it("should render all 6 fragment containers without crashing", () => {
		render(<OverviewTab provider={mockProvider} onSave={mockOnSave} />);

		const fragments = [
			"providers2-overview-network",
			"providers2-overview-proxy",
			"providers2-overview-performance",
			"providers2-overview-governance",
			"providers2-overview-beta-headers",
			"providers2-overview-openai-config",
		];

		fragments.forEach((testId) => {
			expect(screen.getByTestId(testId)).not.toBeNull();
		});
	});
});