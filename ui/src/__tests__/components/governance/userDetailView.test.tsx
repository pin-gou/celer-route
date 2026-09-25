// @vitest-environment jsdom
//
// Pins the dedicated /workspace/governance/users/$userId detail page
// contract: back link, profile + VKs + memberships cards, and the two
// destructive controls (disable-user + disable-user-VKs).
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

vi.mock("@/lib/store", () => ({
	useGetUserQuery: () => ({
		data: {
			user: {
				id: "u-smoke",
				email: "smoke@example.com",
				display_name: "Smoke Member",
				status: "active",
				role: "member",
				last_login_at: null,
				created_at: "2026-01-01T00:00:00Z",
				updated_at: "2026-01-01T00:00:00Z",
			},
			memberships: [{ team_id: "t-smoke", role_in_team: "member", status: "active", joined_at: "2026-01-01T00:00:00Z" }],
			virtual_keys: [
				{
					id: "vk-smoke",
					name: "smoke-vk",
					is_active: true,
					team_id: "t-smoke",
					updated_at: "2026-01-01T00:00:00Z",
				},
			],
		},
		isLoading: false,
		error: undefined,
	}),
	useDisableUserMutation: () => [vi.fn().mockResolvedValue({}), { isLoading: false }],
	useDisableUserVksMutation: () => [
		vi.fn().mockResolvedValue({ message: "ok", count: 1, disabled_virtual_key_ids: ["vk-smoke"] }),
		{ isLoading: false },
	],
	getErrorMessage: () => "x",
}));

vi.mock("@tanstack/react-router", async () => {
	return {
		Link: ({ to, params, children, ...rest }: { to: string; params?: Record<string, string>; children: ReactNode }) => {
			const resolved = to.replace(/\$\w+/g, (k) => params?.[k.slice(1)] ?? k);
			return (
				<a href={resolved} {...rest}>
					{children}
				</a>
			);
		},
		useParams: () => ({ userId: "u-smoke" }),
	};
});

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => key,
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
	Trans: ({ children }: { children: ReactNode }) => children,
}));

import UserDetailView from "@/app/workspace/governance/users/$userId/views/userDetailView";

describe("UserDetailView — dedicated detail page", () => {
	it("renders profile + VK list + disable controls + back link", () => {
		render(<UserDetailView />);
		expect(screen.getByTestId("user-detail-page")).toBeTruthy();
		expect(screen.getByTestId("user-detail-profile")).toBeTruthy();
		expect(screen.getByTestId("user-detail-vks")).toBeTruthy();
		expect(screen.getByTestId("user-detail-memberships")).toBeTruthy();
		const back = screen.getByTestId("user-detail-back");
		expect(back.getAttribute("href")).toBe("/workspace/governance/users");
		expect(screen.getByTestId("user-detail-disable-vks")).toBeTruthy();
		expect(screen.getByTestId("user-detail-disable")).toBeTruthy();
		expect(screen.getByTestId("user-detail-vk-vk-smoke")).toBeTruthy();
	});
});