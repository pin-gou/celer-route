import { Outlet, createFileRoute, redirect } from "@tanstack/react-router";
import MemberPortalView from "./views/memberPortalView";

const MEMBER_LOGIN_PATH = "/member";

// Parent layout for /workspace/member/* — TanStack file-based
// router in this project only mounts child routes when the parent
// renders <Outlet />, so this layout owns:
//   - the auth-gate loader (redirects unauthenticated to the member
//     login surface at /member — the same path Batch C-A registered
//     as a top-level route, not /member/login; the route file is
//     ui/app/member/{layout,page}.tsx and it renders MemberLoginView).
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
	// Detect whether the active route has a child under /workspace/member
	// (such as /workspace/member/keys). The matches array always
	// includes the parent chain (/workspace, /workspace/member), so
	// we compare against a strict descendant routeId prefix.
	const hasChild = matches.some((m) => m.routeId.startsWith("/workspace/member/"));
	if (hasChild) {
		return <Outlet />;
	}
	return <MemberPortalView />;
}