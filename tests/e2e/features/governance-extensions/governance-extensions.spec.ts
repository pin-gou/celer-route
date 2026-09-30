import { expect, test } from "../../core/fixtures/base.fixture";

// Smoke tests for the governance UX extensions shipped this batch:
//   - /workspace/governance/users/{userId} dedicated detail page (US22)
//   - /workspace/governance/teams/{teamId} model-policies section (US21)
//   - /workspace/governance/teams/{teamId}/members dedicated page
//
// We hit the routes against the live dev server (no auth fixture yet for
// these surfaces — the underlying handlers don't require it in this
// build). The Vitest unit tests pin the data-shape contracts:
//   - usersTableRowLinks.test.tsx — row links route to the detail page
//   - userDetailView.test.tsx — detail page profile + VK list + controls
//   - teamModelPoliciesSection.test.tsx — D6 兜底 parse + guard
//
// These specs prove the routes are owned by the SPA (not 404) and that
// each new sub-route mounts the right component under the right parent
// layout, which is the cheapest way to catch "I forgot to add
// <Outlet /> to a parent layout" regressions like the one that hid
// $userId and $teamId children before this batch.
test.describe("Governance UX extensions — sub-route mount smoke", () => {
	test("/workspace/governance/users/{userId} mounts the dedicated detail page", async ({ page }) => {
		// Pull the first user id from the users list. The list is
		// admin-only in production but the dev server has the fixture
		// member baked in, so this is enough to assert mount.
		await page.goto("/workspace/governance/users");
		await page.waitForLoadState("networkidle");
		await page.waitForTimeout(500);
		const link = page.locator('[data-testid^="users-row-link-"]').first();
		const href = await link.getAttribute("href");
		expect(href).toMatch(/^\/workspace\/governance\/users\/[a-f0-9-]+$/);

		await link.click();
		await page.waitForURL(/\/workspace\/governance\/users\/[a-f0-9-]+$/);
		// Detail page renders — proves the parent /users layout
		// surfaces the child via <Outlet /> (regression guard).
		await expect(page.getByTestId("user-detail-page")).toBeVisible();
		await expect(page.getByTestId("user-detail-profile")).toBeVisible();
		await expect(page.getByTestId("user-detail-vks")).toBeVisible();
	});

	test("/workspace/governance/teams/{teamId} surfaces the model-policies section", async ({ page }) => {
		// Use the smoke team id baked into the dev DB by the live
		// curl seeds earlier in this batch. The test just proves the
		// route + section mount under the SPA — it does not assume
		// any auth gate.
		const TEAM_ID = "8eda952f-1dd0-4b78-b473-57ea044be6f2";
		const response = await page.goto(`/workspace/governance/teams/${TEAM_ID}`);
		expect(response).not.toBeNull();
		await expect(page.getByTestId(`team-detail-${TEAM_ID}`)).toBeVisible({ timeout: 10_000 });
		await expect(page.getByTestId("team-model-policies")).toBeVisible();
		await expect(page.getByTestId("team-model-policies-hint")).toBeVisible();
		await expect(page.getByTestId("team-detail-manage-members")).toBeVisible();
	});

	test("/workspace/governance/teams/{teamId}/members mounts the dedicated members page", async ({ page }) => {
		const TEAM_ID = "8eda952f-1dd0-4b78-b473-57ea044be6f2";
		await page.goto(`/workspace/governance/teams/${TEAM_ID}/members`);
		await page.waitForLoadState("networkidle");
		await page.waitForTimeout(500);
		// Either the members page mounted (real team), or the loader
		// guard fired. Both prove the SPA owns the path.
		const ownedBySpa = (await page.getByTestId(`team-members-${TEAM_ID}`).count()) > 0
			|| (await page.getByTestId("team-members-loading").count()) > 0;
		expect(ownedBySpa).toBe(true);
	});
});