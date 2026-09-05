// @vitest-environment jsdom
/**
 * @file OverviewTab component tests
 *
 * OverviewTab is now a read-only dashboard showing key counts, model counts,
 * cooldown summary, retry policy, and configuration suggestions. Editing
 * lives in dedicated tabs (Cooldown, Network, Advanced).
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => key,
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
	initReactI18next: { type: "3rdParty", init: () => {} },
}));

vi.mock("nuqs", () => ({
	useQueryState: () => [null, vi.fn()],
	parseAsString: {},
}));

import { OverviewTab } from "./OverviewTab";
import type { OverviewTabProps } from "./OverviewTab";

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
	custom_provider_config: {
		base_provider_type: "openai",
		is_key_less: false,
	},
	status: "success",
	description: "OpenAI provider",
	keys_count: 3,
	models_count: 47,
	keys_health_status: "healthy",
	hourly_requests: 1284,
	hourly_errors: 3,
	last_used_at: "2026-08-15T01:42:00Z",
	last_error_at: "2026-08-15T00:15:22Z",
	uptime: 0.998,
	avg_latency_ms: 312,
};

const providerWithManyKeys: OverviewTabProps["provider"] = {
	...mockProvider,
	keys_count: 5,
	models_count: 0,
	network_config: { ...(mockProvider.network_config as NonNullable<OverviewTabProps["provider"]["network_config"]>), max_retries: 0 },
};

describe("OverviewTab", () => {
	it("should render the four read-only cards", () => {
		render(<OverviewTab provider={mockProvider} />);

		expect(screen.getByTestId("providers2-overview-keys")).not.toBeNull();
		expect(screen.getByTestId("providers2-overview-models")).not.toBeNull();
		expect(screen.getByTestId("providers2-overview-cooldown-policy")).not.toBeNull();
		expect(screen.getByTestId("providers2-overview-retry")).not.toBeNull();
	});

	it("should display API Key count", () => {
		render(<OverviewTab provider={mockProvider} />);

		const keysSection = screen.getByTestId("providers2-overview-keys");
		expect(keysSection.textContent).toContain("3");
	});

	it("should display Models count", () => {
		render(<OverviewTab provider={mockProvider} />);

		const modelsSection = screen.getByTestId("providers2-overview-models");
		expect(modelsSection.textContent).toContain("47");
	});

	it("should display Retry Policy summary", () => {
		render(<OverviewTab provider={mockProvider} />);

		const retrySection = screen.getByTestId("providers2-overview-retry");
		expect(retrySection.textContent).toContain("3");
		expect(retrySection.textContent).toContain("1000ms");
		expect(retrySection.textContent).toContain("30000ms");
	});

	it("should render Cooldown Policy summary using default copy when no policy is set", () => {
		const noCooldown = { ...mockProvider, cooldown_policy: undefined };
		render(<OverviewTab provider={noCooldown} />);

		const cooldownSection = screen.getByTestId("providers2-overview-cooldown-policy");
		expect(cooldownSection.textContent).toContain("providers2.overview.cooldownPolicyUsingDefault");
	});

	it("should show noModels suggestion when models_count is 0", () => {
		render(<OverviewTab provider={providerWithManyKeys} />);

		expect(screen.getByTestId("providers2-overview-suggestion-noModels")).not.toBeNull();
	});

	it("should show multipleKeysNoRetries suggestion when many keys and zero retries", () => {
		render(<OverviewTab provider={providerWithManyKeys} />);

		expect(screen.getByTestId("providers2-overview-suggestion-multipleKeysNoRetries")).not.toBeNull();
	});

	it("should show multipleKeysNoCooldown suggestion when many keys and no cooldown", () => {
		render(<OverviewTab provider={providerWithManyKeys} />);

		expect(screen.getByTestId("providers2-overview-suggestion-multipleKeysNoCooldown")).not.toBeNull();
	});

	it("should not render any suggestions when the provider is well-configured", () => {
		const wellConfigured: OverviewTabProps["provider"] = {
			...mockProvider,
			keys_count: 3,
			models_count: 47,
			network_config: { ...(mockProvider.network_config as NonNullable<OverviewTabProps["provider"]["network_config"]>), max_retries: 3 },
			cooldown_policy: {
				rate_limit: { match: [{ status_code: 429 }], match_mode: "any", ttl_seconds: 60, enabled: true },
			},
		};
		render(<OverviewTab provider={wellConfigured} />);

		expect(screen.queryByTestId("providers2-overview-suggestions")).toBeNull();
	});

	it("should call onNavigateTab when a card's Manage button is clicked", () => {
		const navigate = vi.fn();
		render(<OverviewTab provider={mockProvider} onNavigateTab={navigate} />);

		const manageBtn = screen.getByTestId("providers2-overview-keys-manage");
		manageBtn.click();

		expect(navigate).toHaveBeenCalledWith("keys");
	});
});