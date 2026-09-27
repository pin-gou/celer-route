import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useTranslation } from "react-i18next";
import "@/lib/i18n/config";

function RoutingTabs() {
	const { t } = useTranslation(["routing", "common"]);
	return (
		<div>
			<span data-testid="curl">{t("codeTabs.curl")}</span>
			<span data-testid="go">{t("codeTabs.go")}</span>
			<span data-testid="routing-key">{t("infoSheet.testCommandPanel.copyTab", { language: "cURL" })}</span>
		</div>
	);
}

function OnboardingTabs() {
	const { t } = useTranslation(["onboarding", "common"]);
	return (
		<div>
			<span data-testid="python">{t("codeTabs.python")}</span>
			<span data-testid="onboard-key">{t("step.done")}</span>
		</div>
	);
}

function AlertingPage() {
	const { t } = useTranslation("alerting");
	return (
		<div>
			<span data-testid="alerting-title">{t("title")}</span>
			<span data-testid="alerting-metric">{t("metric_tokens_per_minute")}</span>
			<span data-testid="alerting-refresh">{t("refresh")}</span>
			<span data-testid="alerting-shared">{t("cancel")}</span>
		</div>
	);
}

function ReportsPage() {
	const { t } = useTranslation("reports");
	return (
		<div>
			<span data-testid="reports-title">{t("title")}</span>
			<span data-testid="reports-sp-title">{t("standardPrices.title")}</span>
			<span data-testid="reports-gd-metric">{t("gatewayDelta.metric_tokens")}</span>
			<span data-testid="reports-export">{t("export")}</span>
			<span data-testid="reports-shared">{t("loadFailed")}</span>
		</div>
	);
}

describe("array namespace fallback (react.nsMode)", () => {
	it("resolves shared codeTabs keys from common when they are absent from the leading ns", () => {
		render(<RoutingTabs />);
		expect(screen.getByTestId("curl").textContent).toBe("cURL");
		expect(screen.getByTestId("go").textContent).toBe("Go");
	});

	it("still resolves namespaced keys from the leading ns", () => {
		render(<RoutingTabs />);
		expect(screen.getByTestId("routing-key").textContent).toContain("cURL");
	});

	it("works for onboarding + common", () => {
		render(<OnboardingTabs />);
		expect(screen.getByTestId("python").textContent).toBe("Python");
		expect(screen.getByTestId("onboard-key").textContent).toBeTruthy();
	});
});

describe("alerting namespace", () => {
	it("resolves own keys without rendering raw key names", () => {
		render(<AlertingPage />);
		expect(screen.getByTestId("alerting-title").textContent).toBe("Alerting");
		expect(screen.getByTestId("alerting-metric").textContent).toBe("Tokens per minute");
	});

	it("resolves shared keys included in the namespace", () => {
		render(<AlertingPage />);
		expect(screen.getByTestId("alerting-refresh").textContent).toBe("Refresh");
		expect(screen.getByTestId("alerting-shared").textContent).toBe("Cancel");
	});
});

describe("reports namespace", () => {
	it("resolves nested standardPrices / gatewayDelta keys", () => {
		render(<ReportsPage />);
		expect(screen.getByTestId("reports-title").textContent).toBe("Reports");
		expect(screen.getByTestId("reports-sp-title").textContent).toBe("Standard prices");
		expect(screen.getByTestId("reports-gd-metric").textContent).toBe("Tokens");
	});

	it("resolves shared keys included in the namespace", () => {
		render(<ReportsPage />);
		expect(screen.getByTestId("reports-export").textContent).toBe("Export CSV");
		expect(screen.getByTestId("reports-shared").textContent).toBe("Failed to load");
	});
});