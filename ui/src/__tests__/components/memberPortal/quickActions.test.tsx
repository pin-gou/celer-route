// @vitest-environment jsdom
//
// Phase-2 P0 unlock — the /workspace/member landing page's Quick
// Actions card used to render three disabled, self-pointing links
// (Batch C-A placeholder). This test pins the new contract: every
// Quick Action is an enabled Link whose href points at the matching
// Phase-2 sub-route — /workspace/member/keys, /workspace/member/usage,
// /workspace/member/setup-guide. The redirect-to-/member/login
// load-time auth gate is exercised separately by the Playwright
// route-mount smoke at tests/e2e/features/member-portal/.
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

// Mock the RTK Query hooks the portal view consumes so the
// component renders without an active store.
vi.mock("@/lib/store", () => ({
	useMemberMeQuery: () => ({
		data: {
			user: {
				id: "u1",
				email: "u1@example.com",
				display_name: "User One",
				status: "active",
				role: "member",
				last_login_at: null,
				created_at: "",
				updated_at: "",
			},
			teams: [{ id: "t1", name: "Team One", role: "member" }],
			is_admin: false,
		},
		isLoading: false,
		isError: false,
		error: undefined,
		refetch: () => {},
		isFetching: false,
	}),
	useMemberLogoutMutation: () => [vi.fn().mockResolvedValue({}), { isLoading: false }],
	getErrorMessage: () => "x",
}));

// react-i18next must return predictable labels for our assertions.
vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => {
			const lookup: Record<string, string> = {
				"users.memberPortal.keyRequests": "Key requests",
				"users.memberPortal.usage": "Usage",
				"users.memberPortal.setupGuide": "Setup guide",
				"users.memberPortal.signOut": "Sign out",
				"users.memberPortal.welcome": "Welcome, {{name}}",
				"users.memberPortal.teams": "Teams",
				"users.memberPortal.teamsCount_other": "{{count}} teams",
				"users.memberPortal.noTeams": "You are not a member of any team yet.",
				"users.memberPortal.account": "Account",
				"users.memberPortal.status": "Status",
				"users.memberPortal.role": "Role",
				"users.memberPortal.lastLogin": "Last login",
				"users.memberPortal.never": "Never",
				"users.memberPortal.quickActions": "Coming soon",
				"users.memberPortal.quickActionsHint": "…",
				status_active: "Active",
				status_pending: "Pending",
				status_disabled: "Disabled",
				role_admin: "Admin",
				role_member: "Member",
			};
			return lookup[key] ?? key;
		},
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
	Trans: ({ children }: { children: ReactNode }) => children,
}));

// TanStack Router's <Link> in this app reads the runtime router.
// We shim it as a plain anchor so the test can assert href directly.
vi.mock("@tanstack/react-router", async () => {
	const actual = await vi.importActual<typeof import("react")>("react");
	return {
		Link: ({ to, children, ...rest }: { to: string; children: ReactNode }) => (
			<a href={to} {...rest}>
				{children}
			</a>
		),
		useNavigate: () => () => {},
	};
});

import MemberPortalView from "@/app/workspace/member/views/memberPortalView";

const renderPortal = () => render(<MemberPortalView />);

describe("MemberPortalView — Quick Actions", () => {
	it("renders three Quick Action links pointing at the new sub-routes", () => {
		renderPortal();
		const keys = screen.getByTestId("member-portal-key-requests");
		const usage = screen.getByTestId("member-portal-usage");
		const billing = screen.getByTestId("member-portal-billing");
		expect(keys).toBeTruthy();
		expect(usage).toBeTruthy();
		expect(billing).toBeTruthy();
		// Anchors (the shimmed <Link>) — their href must point at
		// the Phase-2 sub-routes, not at /workspace/member itself.
		expect(keys.getAttribute("href")).toBe("/workspace/member/keys");
		expect(usage.getAttribute("href")).toBe("/workspace/member/usage");
		expect(billing.getAttribute("href")).toBe("/workspace/member/setup-guide");
	});

	it("does not render Quick Action buttons in a disabled state", () => {
		// The Batch C-A placeholder rendered <Button … disabled>. The
		// Phase-2 unlock must drop that. With shadcn/ui's <Button
		// asChild> + Radix Slot, the Quick Action renders as the
		// child element (an <a> after our router shim) — there is
		// no wrapping <button>. We assert the absence of the
		// `disabled` attribute on the rendered child.
		renderPortal();
		const keys = screen.getByTestId("member-portal-key-requests");
		const usage = screen.getByTestId("member-portal-usage");
		const billing = screen.getByTestId("member-portal-billing");
		expect(keys.hasAttribute("disabled")).toBe(false);
		expect(usage.hasAttribute("disabled")).toBe(false);
		expect(billing.hasAttribute("disabled")).toBe(false);
		// And the underlying <a> is enabled (no aria-disabled either).
		expect(keys.hasAttribute("aria-disabled")).toBe(false);
		expect(usage.hasAttribute("aria-disabled")).toBe(false);
		expect(billing.hasAttribute("aria-disabled")).toBe(false);
	});
});