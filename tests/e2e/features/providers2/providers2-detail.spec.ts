import { expect, test } from "../../core/fixtures/base.fixture";

/**
 * @file TDD Red Phase — Providers2 detail page E2E tests
 *
 * These tests verify the new providers2 detail page:
 * - 6 Tabs (Overview/Keys/Models/Usage/Governance/Logs) all render
 * - Each tab switch updates content area
 * - No console.error on tab switches
 *
 * In the TDD red phase, the /workspace/providers2/:id route does not exist yet,
 * so these tests will fail when the page returns 404 or cannot find elements.
 * This is the expected result — the dev phase will implement the route.
 */

test.describe("Providers2 Detail", () => {
	test.describe.configure({ mode: "serial" });

	// Collect console errors across all tests in this describe
	let consoleErrors: string[] = [];

	test.beforeEach(async ({ page }) => {
		consoleErrors = [];
		page.on("console", (msg) => {
			if (msg.type() === "error") {
				consoleErrors.push(msg.text());
			}
		});
	});

	test("should navigate to provider detail page at /workspace/providers2/openai", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// The detail page should show the provider name
		await expect(page.getByTestId("providers2-detail-heading")).toBeVisible();
		await expect(page.getByTestId("providers2-detail-heading")).toContainText("openai");
	});

	test("should display the Overview tab by default", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// Overview tab should be active by default
		const overviewTab = page.getByTestId("providers2-tab-overview");
		await expect(overviewTab).toBeVisible();
		await expect(overviewTab).toHaveAttribute("data-state", "active");

		// Overview tab content should be visible
		const overviewContent = page.getByTestId("providers2-tab-content-overview");
		await expect(overviewContent).toBeVisible();
	});

	test("should display all 6 tab buttons", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		const tabs = [
			"providers2-tab-overview",
			"providers2-tab-keys",
			"providers2-tab-models",
			"providers2-tab-usage",
			"providers2-tab-governance",
			"providers2-tab-logs",
		];

		for (const tabTestId of tabs) {
			await expect(page.getByTestId(tabTestId)).toBeVisible();
		}
	});

	test("should switch to Keys tab when clicking Keys tab", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// Click Keys tab
		await page.getByTestId("providers2-tab-keys").click();
		await page.waitForTimeout(500);

		// Keys tab content should be visible
		const keysContent = page.getByTestId("providers2-tab-content-keys");
		await expect(keysContent).toBeVisible();

		// No console errors should have occurred
		expect(consoleErrors.length).toBe(0);
	});

	test("should switch to Models tab when clicking Models tab", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// Click Models tab
		await page.getByTestId("providers2-tab-models").click();
		await page.waitForTimeout(500);

		// Models tab content should be visible
		const modelsContent = page.getByTestId("providers2-tab-content-models");
		await expect(modelsContent).toBeVisible();

		expect(consoleErrors.length).toBe(0);
	});

	test("should switch to Usage tab when clicking Usage tab", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// Click Usage tab
		await page.getByTestId("providers2-tab-usage").click();
		await page.waitForTimeout(500);

		// Usage tab content should be visible
		const usageContent = page.getByTestId("providers2-tab-content-usage");
		await expect(usageContent).toBeVisible();

		expect(consoleErrors.length).toBe(0);
	});

	test("should switch to Governance tab when clicking Governance tab", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// Click Governance tab
		await page.getByTestId("providers2-tab-governance").click();
		await page.waitForTimeout(500);

		// Governance tab content should be visible
		const govContent = page.getByTestId("providers2-tab-content-governance");
		await expect(govContent).toBeVisible();

		expect(consoleErrors.length).toBe(0);
	});

	test("should switch to Logs tab when clicking Logs tab", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// Click Logs tab
		await page.getByTestId("providers2-tab-logs").click();
		await page.waitForTimeout(500);

		// Logs tab content should be visible
		const logsContent = page.getByTestId("providers2-tab-content-logs");
		await expect(logsContent).toBeVisible();

		expect(consoleErrors.length).toBe(0);
	});

	test("should cycle through all 6 tabs without console errors", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// Cycle through all tabs
		const tabIds = [
			"providers2-tab-overview",
			"providers2-tab-keys",
			"providers2-tab-models",
			"providers2-tab-usage",
			"providers2-tab-governance",
			"providers2-tab-logs",
		];

		const contentIds = [
			"providers2-tab-content-overview",
			"providers2-tab-content-keys",
			"providers2-tab-content-models",
			"providers2-tab-content-usage",
			"providers2-tab-content-governance",
			"providers2-tab-content-logs",
		];

		for (let i = 0; i < tabIds.length; i++) {
			await page.getByTestId(tabIds[i]).click();
			await page.waitForTimeout(300);

			// Verify the content area updated
			const content = page.getByTestId(contentIds[i]);
			await expect(content).toBeVisible();
		}

		// No console errors throughout the entire cycle
		expect(consoleErrors.length).toBe(0);
	});

	test("should display breadcrumb with link back to providers list", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// Breadcrumb should contain a link back to providers2 list
		const breadcrumb = page.getByTestId("providers2-detail-breadcrumb");
		await expect(breadcrumb).toBeVisible();

		// Clicking the breadcrumb should navigate back to the list
		const listLink = page.getByTestId("providers2-breadcrumb-list-link");
		await expect(listLink).toBeVisible();
		await listLink.click();
		await page.waitForTimeout(500);

		// Should be on the list page
		await expect(page).toHaveURL(/\/workspace\/providers2$/);
	});

	test("should display the legacy view button to switch back to old providers page", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2/openai");
		await page.waitForLoadState("networkidle");

		// Legacy view button should be visible
		const legacyBtn = page.getByTestId("providers2-legacy-view-btn");
		await expect(legacyBtn).toBeVisible();

		// Clicking should navigate to the old providers page
		await legacyBtn.click();
		await page.waitForTimeout(500);

		// Should be on the old providers page with the provider param
		await expect(page).toHaveURL(/\/workspace\/providers/);
	});
});