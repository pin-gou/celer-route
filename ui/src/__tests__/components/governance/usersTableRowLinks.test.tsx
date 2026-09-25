// @vitest-environment jsdom
//
// Pins the new contract for /workspace/governance/users: email +
// display-name cells are <Link>s to the new dedicated detail route
// /workspace/governance/users/$userId. The detail page itself is
// covered by userDetailView.test.tsx.
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

vi.mock("@/lib/store", () => ({
	useListUsersQuery: () => ({
		data: {
			users: [
				{
					id: "u-smoke",
					email: "smoke@example.com",
					display_name: "Smoke Member",
					status: "active",
					role: "member",
					last_login_at: null,
					created_at: "2026-01-01T00:00:00Z",
					updated_at: "2026-01-01T00:00:00Z",
				},
			],
			total: 1,
			limit: 25,
			offset: 0,
		},
		isLoading: false,
		isFetching: false,
		error: undefined,
		refetch: () => {},
	}),
	useDeleteUserMutation: () => [vi.fn().mockResolvedValue({}), { isLoading: false }],
	useDisableUserMutation: () => [vi.fn().mockResolvedValue({}), { isLoading: false }],
	useDisableUserVksMutation: () => [
		vi.fn().mockResolvedValue({ message: "ok", count: 0, disabled_virtual_key_ids: [] }),
		{ isLoading: false },
	],
	useResetUserPasswordTokenMutation: () => [vi.fn().mockResolvedValue({ message: "ok", token: "tok" }), { isLoading: false }],
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
		useNavigate: () => () => {},
	};
});

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => key,
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
	Trans: ({ children }: { children: ReactNode }) => children,
}));

import UsersTable from "@/app/workspace/governance/users/views/usersTable";

describe("UsersTable — row links route to the dedicated detail page", () => {
	it("email cell points at /workspace/governance/users/$userId", () => {
		render(<UsersTable />);
		const link = screen.getByTestId("users-row-link-u-smoke");
		expect(link).toBeTruthy();
		expect(link.getAttribute("href")).toBe("/workspace/governance/users/u-smoke");
		expect(link.textContent).toContain("smoke@example.com");
	});
});