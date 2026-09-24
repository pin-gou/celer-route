import { expect, test } from "../../core/fixtures/base.fixture";

// Member-portal smoke: Phase-2 P0 surfaces at /workspace/member/{keys,usage,setup-guide}.
// These are pure route-mount smoke tests. We don't log in (the
// member-side fixture is a follow-up once a member session is
// plumbed through base.fixture.ts); instead we prove the new routes
// are owned by the SPA — not the static 404 page — by asserting that
// the parent layout's auth-gate loader redirects an unauthenticated
// visitor to /login. That loader is the same one that guards
// /workspace/member today; the new sub-routes only get to render
// under it, so this also confirms TanStack Router is registering
// /workspace/member/keys|usage|setup-guide (otherwise we'd hit 404,
// not the loader redirect).
//
// Note: the canonical member login path is /member/login, but that
// route is not yet wired as a TanStack route in this build — the
// member-side login UI is a pre-existing backlog item tracked
// separately. Until then, the loader falls back to /login so the
// visitor at least sees an existing login surface instead of a
// TanStack not-found page.
//
// The Quick-Actions hrefs on the /workspace/member landing page are
// covered separately by the vitest unit test in
// ui/src/__tests__/components/memberPortal/quickActions.test.tsx —
// that test renders the portal view directly with a mocked RTK
// Query hook, so it doesn't need a member session.
test.describe("Member portal — Phase-2 P0 surfaces", () => {
	test("/workspace/member/keys is owned by the SPA (redirects unauthenticated to /login)", async ({ page }) => {
		const response = await page.goto("/workspace/member/keys");
		expect(response).not.toBeNull();
		await expect(page).toHaveURL(/\/login(?:\?.*)?$/, { timeout: 10_000 });
		const body = await page.locator("body").innerText();
		expect(body).not.toMatch(/Page not found/);
	});

	test("/workspace/member/usage is owned by the SPA (redirects unauthenticated to /login)", async ({ page }) => {
		const response = await page.goto("/workspace/member/usage");
		expect(response).not.toBeNull();
		await expect(page).toHaveURL(/\/login(?:\?.*)?$/, { timeout: 10_000 });
		const body = await page.locator("body").innerText();
		expect(body).not.toMatch(/Page not found/);
	});

	test("/workspace/member/setup-guide is owned by the SPA (redirects unauthenticated to /login)", async ({ page }) => {
		const response = await page.goto("/workspace/member/setup-guide");
		expect(response).not.toBeNull();
		await expect(page).toHaveURL(/\/login(?:\?.*)?$/, { timeout: 10_000 });
		const body = await page.locator("body").innerText();
		expect(body).not.toMatch(/Page not found/);
	});
});