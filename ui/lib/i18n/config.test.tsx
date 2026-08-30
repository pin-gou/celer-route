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