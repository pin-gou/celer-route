// @vitest-environment jsdom
//
// Pins the new contract for /workspace/governance/teams: the team name
// cell is a <Link> to the dedicated detail route /workspace/governance/teams/$teamId,
// and the page exposes the "New team" create entry point. The detail and
// members pages themselves are covered by teamModelPoliciesSection.test.tsx
// and the members-view flows.
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

vi.mock("@/lib/store", () => ({
	useGetTeamsQuery: () => ({
		data: {
			teams: [{ id: "t-smoke", name: "Smoke Team", virtual_key_count: 3 }],
			count: 1,
			total_count: 1,
			limit: 25,
			offset: 0,
		},
		isLoading: false,
		isFetching: false,
		error: undefined,
		refetch: () => {},
	}),
	useCreateTeamMutation: () => [vi.fn().mockResolvedValue({}), { isLoading: false }],
	useUpdateTeamMutation: () => [vi.fn().mockResolvedValue({}), { isLoading: false }],
	useDeleteTeamMutation: () => [vi.fn().mockResolvedValue({}), { isLoading: false }],
	useGetCustomersQuery: () => ({ data: { customers: [] }, isLoading: false }),
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

import TeamsTable from "@/app/workspace/governance/teams/views/teamsTable";

describe("TeamsTable — row links route to the dedicated detail page", () => {
	it("name cell points at /workspace/governance/teams/$teamId and the create button is present", () => {
		render(<TeamsTable />);
		const link = screen.getByTestId("teams-row-link-t-smoke");
		expect(link).toBeTruthy();
		expect(link.getAttribute("href")).toBe("/workspace/governance/teams/t-smoke");
		expect(link.textContent).toContain("Smoke Team");
		expect(screen.getByTestId("teams-create")).toBeTruthy();
	});
});