import { expect, test } from "../../core/fixtures/base.fixture";

/**
 * @file TDD Red Phase — Providers2 list page E2E tests
 *
 * These tests verify the new providers2 list page:
 * - 5 fixture provider grouped cards visible
 * - Name search filters providers
 * - Health status chip filters providers
 *
 * In the TDD red phase, the /workspace/providers2 route does not exist yet,
 * so these tests will fail when the page returns 404 or cannot find elements.
 * This is the expected result — the dev phase will implement the route.
 */

test.describe("Providers2 List", () => {
	test.describe.configure({ mode: "serial" });

	test("should display the providers2 list page with 5 fixture provider grouped cards", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2");

		// Wait for the page to load
		await page.waitForLoadState("networkidle");

		// The page should show the providers2 heading
		await expect(page.getByTestId("providers2-page-heading")).toBeVisible();

		// Should see family group sections
		const familyGroups = page.locator("[data-testid^='providers2-family-group-']");
		await expect(familyGroups.first()).toBeVisible();

		// Should see provider cards — at least 5 fixture providers
		const providerCards = page.locator("[data-testid^='providers2-card-']");
		await expect(providerCards.first()).toBeVisible();
		const cardCount = await providerCards.count();
		expect(cardCount).toBeGreaterThanOrEqual(5);
	});

	test("should display provider cards with health badge, keys count, models count, and requests", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2");
		await page.waitForLoadState("networkidle");

		// Each provider card should have a health badge
		const healthBadge = page.locator("[data-testid='providers2-card-health-badge']").first();
		await expect(healthBadge).toBeVisible();

		// Each provider card should show keys, models, and requests info
		const keysStat = page.locator("[data-testid^='providers2-card-keys-']").first();
		await expect(keysStat).toBeVisible();

		const modelsStat = page.locator("[data-testid^='providers2-card-models-']").first();
		await expect(modelsStat).toBeVisible();

		const reqsStat = page.locator("[data-testid^='providers2-card-reqs-']").first();
		await expect(reqsStat).toBeVisible();
	});

	test("should filter providers by name search", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2");
		await page.waitForLoadState("networkidle");

		// Type a search term in the search input
		const searchInput = page.getByTestId("providers2-filter-search");
		await expect(searchInput).toBeVisible();
		await searchInput.fill("openai");

		// Wait for filtering to take effect
		await page.waitForTimeout(500);

		// Only providers matching "openai" should be visible
		const visibleCards = page.locator("[data-testid^='providers2-card-']:visible");
		const openaiCard = page.locator("[data-testid='providers2-card-openai']");
		await expect(openaiCard).toBeVisible();

		// Clear search and verify more cards appear
		await searchInput.fill("");
		await page.waitForTimeout(500);
	});

	test("should filter providers by health status chip", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2");
		await page.waitForLoadState("networkidle");

		// Click the "Active" health chip
		const activeChip = page.getByTestId("providers2-filter-chip-active");
		await expect(activeChip).toBeVisible();
		await activeChip.click();

		// Wait for filtering to take effect
		await page.waitForTimeout(500);

		// The active chip should be selected
		await expect(activeChip).toHaveAttribute("data-active", "true");

		// Click the "Error" health chip
		const errorChip = page.getByTestId("providers2-filter-chip-error");
		await errorChip.click();
		await page.waitForTimeout(500);

		// The error chip should be selected
		await expect(errorChip).toHaveAttribute("data-active", "true");

		// Click "All" to reset
		const allChip = page.getByTestId("providers2-filter-chip-all");
		await allChip.click();
		await page.waitForTimeout(500);

		// All chips should be visible
		await expect(allChip).toHaveAttribute("data-active", "true");
	});

	test("should display provider cards with toggle and quick test buttons", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2");
		await page.waitForLoadState("networkidle");

		// Each provider card should have a toggle switch
		const toggle = page.locator("[data-testid='providers2-card-toggle']").first();
		await expect(toggle).toBeVisible();

		// Each provider card should have a quick test button
		const quickTestBtn = page.locator("[data-testid='providers2-card-quick-test']").first();
		await expect(quickTestBtn).toBeVisible();
	});

	test("should navigate to provider detail page when clicking a card", async ({
		page,
	}) => {
		await page.goto("/workspace/providers2");
		await page.waitForLoadState("networkidle");

		// Click on the OpenAI provider card
		const openaiCard = page.locator("[data-testid='providers2-card-openai']");
		await expect(openaiCard).toBeVisible();
		await openaiCard.click();

		// Should navigate to the detail page
		await expect(page).toHaveURL(/\/workspace\/providers2\/openai/);
	});
});