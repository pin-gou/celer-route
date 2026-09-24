import { createFileRoute, redirect } from "@tanstack/react-router";

const MEMBER_LOGIN_PATH = "/member/login";

// The /workspace/member landing page renders only for authenticated
// members. Members land here after /member/login redirects in (the
// login form's onSuccess navigates to /workspace/member). The page
// itself re-validates the session via the loader because a member
// could land here via a stale link after their cookie expired.
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
});