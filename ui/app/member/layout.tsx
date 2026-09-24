import { ThemeProvider } from "@/components/themeProvider";
import { I18nProvider } from "@/lib/i18n/I18nProvider";
import { useBranding } from "@/lib/hooks/useBranding";
import { ReduxProvider } from "@/lib/store/provider";
import { useMemberAuthStatusQuery } from "@/lib/store";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { useTheme } from "next-themes";
import { NuqsAdapter } from "nuqs/adapters/tanstack-router";
import { useTranslation } from "react-i18next";
import MemberLoginPage from "./page";

const MEMBER_PORTAL_PATH = "/workspace/member";

function RouteComponent() {
	return (
		<I18nProvider>
			<ThemeProvider attribute="class" defaultTheme="system" enableSystem>
				<ReduxProvider>
					<NuqsAdapter>
						<div className="bg-background min-h-screen">
							<MemberLoginPage />
						</div>
					</NuqsAdapter>
				</ReduxProvider>
			</ThemeProvider>
		</I18nProvider>
	);
}

function PendingCard() {
	const { resolvedTheme } = useTheme();
	const { t } = useTranslation("governance-ui");
	const { logoSrc, logoAlt } = useBranding(resolvedTheme === "dark");
	return (
		<div className="flex min-h-screen items-center justify-center p-4">
			<div className="w-full max-w-md">
				<div className="border-border bg-card w-full space-y-6 rounded-sm border p-8">
					<div className="flex items-center justify-center">
						<img src={logoSrc} alt={logoAlt} width={160} height={26} className="max-h-[40px] w-auto max-w-[220px] object-contain" />
					</div>
					<div className="text-muted-foreground py-6 text-center text-sm">{t("users.memberLogin.checkingSession")}</div>
				</div>
			</div>
		</div>
	);
}

function PendingComponent() {
	return (
		<I18nProvider>
			<ThemeProvider attribute="class" defaultTheme="system" enableSystem>
				<ReduxProvider>
					<PendingCard />
				</ReduxProvider>
			</ThemeProvider>
		</I18nProvider>
	);
}

// `memberAuthStatus` returns 200 with `authenticated:false` on a
// missing cookie so the SPA can call it without surfacing a 401 in
// the network panel. We probe before painting the form so an already
// signed-in member lands directly on /workspace/member.
export const Route = createFileRoute("/member")({
	loader: async () => {
		try {
			const res = await fetch("/api/member/auth-status", { credentials: "include" });
			if (res.ok) {
				const data = (await res.json()) as { authenticated?: boolean };
				if (data.authenticated) {
					throw redirect({ href: MEMBER_PORTAL_PATH });
				}
			}
		} catch (e) {
			// Re-throw redirects; swallow any other fetch failure so the
			// login form still renders when the gateway is briefly unreachable.
			if (e instanceof Response) throw e;
		}
	},
	pendingComponent: PendingComponent,
	pendingMs: 0,
	component: RouteComponent,
});

// Silence the "unused import" lint if the auth-status hook ends up
// not directly referenced from JSX (we use fetch() in the loader to
// keep this route decoupled from the RTK Query cache).
void useMemberAuthStatusQuery;