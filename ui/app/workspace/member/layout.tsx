import { Outlet, createFileRoute, redirect } from "@tanstack/react-router";
import MemberPortalView from "./views/memberPortalView";

const MEMBER_LOGIN_PATH = "/login";

// Parent layout for /workspace/member/* — TanStack file-based
// router in this project only mounts child routes when the parent
// renders <Outlet />, so this layout owns:
//   - the auth-gate loader (redirects unauthenticated to the admin
//     login surface — /login — because /member/login is not yet
//     wired as a TanStack route in this build; the member-session
//     cookie is independent of the admin cookie, so the admin login
//     surface can authenticate members too once `cookieName` is
//     routed correctly; tracked separately in the member-portal
//     backlog).
//   - the Outlet that mounts sub-routes (keys, usage, setup-guide).
//   - the index/portal view as the default content when no child
//     route is active. The hasChild-detection below suppresses the
//     portal view when a sub-route mounts, so /workspace/member/keys
//     shows MyKeysView, not both views.
export const Route = createFileRoute("/workspace/member")({
	loader: async () => {
		try {
			const res = await fetch("/api/member/auth-status", { credentials: "include" });
			if (res.ok) {
				const data = (await res.json()) as { authenticated?: boolean };
				if (data.authenticated) return;
			}
		} catch {
			// Fall through to redirect.
		}
		throw redirect({ href: MEMBER_LOGIN_PATH });
	},
	component: MemberPortalLayout,
});

import { useMatches } from "@tanstack/react-router";

function MemberPortalLayout() {
	const matches = useMatches();
	const hasChild = matches.some((m) => m.routeId !== "/workspace/member");
	if (hasChild) {
		return <Outlet />;
	}
	return <MemberPortalView />;
}