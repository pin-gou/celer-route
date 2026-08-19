import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { Beaker, FileSearch, FlaskConical, Image as ImageIcon, Settings } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";

// /workspace/plugins/rtk — RTK admin sub-router.
//
// Four sub-pages live under this layout:
//   /workspace/plugins/rtk/filters          — filter catalog browser
//   /workspace/plugins/rtk/test             — compression test runner
//   /workspace/plugins/rtk/preview          — preview playground (rtk/stacked/off)
//   /workspace/plugins/rtk/raw-output       — raw-output viewer (id-driven)
//
// When the parent path is matched directly (no child route active), the
// layout renders an overview card grid so the URL is never empty.

function RouteComponent() {
	const hasPluginsAccess = useRbac(RbacResource.Plugins, RbacOperation.View);
	if (!hasPluginsAccess) {
		return <NoPermissionView entity="plugins" entityI18nKey="plugins:page.title" />;
	}
	return <RtkAdminLayout />;
}

function RtkAdminLayout() {
	const { t } = useTranslation();
	const location = useLocation();

	const tabs = [
		{ to: "/workspace/plugins/rtk", label: t("plugins:rtk.admin.tabs.overview", "Overview"), icon: Settings, exact: true },
		{ to: "/workspace/plugins/rtk/filters", label: t("plugins:rtk.admin.tabs.filters"), icon: FlaskConical },
		{ to: "/workspace/plugins/rtk/test", label: t("plugins:rtk.admin.tabs.test"), icon: Beaker },
		{ to: "/workspace/plugins/rtk/preview", label: t("plugins:rtk.admin.tabs.preview"), icon: ImageIcon },
		{ to: "/workspace/plugins/rtk/raw-output", label: t("plugins:rtk.admin.tabs.rawOutput", "Raw Output"), icon: FileSearch },
	];

	const isActive = (path: string, exact?: boolean) => {
		if (exact) return location.pathname === path;
		return location.pathname.startsWith(path);
	};

	const isOverview = location.pathname === "/workspace/plugins/rtk";

	const overviewCards = [
		{
			to: "/workspace/plugins/rtk/filters",
			title: t("plugins:rtk.admin.tabs.filters"),
			description: t("plugins:rtk.admin.filtersDescription"),
			icon: FlaskConical,
		},
		{
			to: "/workspace/plugins/rtk/test",
			title: t("plugins:rtk.admin.tabs.test"),
			description: t("plugins:rtk.admin.testDescription"),
			icon: Beaker,
		},
		{
			to: "/workspace/plugins/rtk/preview",
			title: t("plugins:rtk.admin.tabs.preview"),
			description: t("plugins:rtk.admin.previewDescription"),
			icon: ImageIcon,
		},
		{
			to: "/workspace/plugins/rtk/raw-output",
			title: t("plugins:rtk.admin.tabs.rawOutput", "Raw Output"),
			description: t("plugins:rtk.admin.rawOutputDescription", "Open a persisted raw-output file by id."),
			icon: FileSearch,
		},
	];

	return (
		<div className="mx-auto flex w-full max-w-7xl flex-col gap-6">
			<header className="flex flex-col gap-1">
				<h1 className="text-2xl font-semibold">{t("plugins:rtk.admin.title")}</h1>
				<p className="text-muted-foreground text-sm">{t("plugins:rtk.admin.subtitle")}</p>
			</header>
			<nav className="border-b" data-testid="rtk-admin-nav">
				<ul className="-mb-px flex flex-wrap gap-x-4 gap-y-2 text-sm">
					{tabs.map((tab) => {
						const Icon = tab.icon;
						const active = isActive(tab.to, tab.exact);
						return (
							<li key={tab.to}>
								<Link
									to={tab.to}
									data-testid={`rtk-admin-tab-${tab.to.split("/").pop() || "overview"}`}
									className={cn(
										"inline-flex items-center gap-2 border-b-2 px-3 py-2 transition-colors",
										active ? "border-primary text-primary" : "text-muted-foreground hover:text-foreground border-transparent",
									)}
								>
									<Icon className="h-4 w-4" />
									{tab.label}
								</Link>
							</li>
						);
					})}
				</ul>
			</nav>
			<main>
				{isOverview ? (
					<div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4" data-testid="rtk-admin-overview">
						{overviewCards.map((card) => {
							const Icon = card.icon;
							return (
								<Link key={card.to} to={card.to} data-testid={`rtk-admin-card-${card.to.split("/").pop()}`}>
									<Card className="hover:border-primary h-full cursor-pointer transition-colors">
										<CardHeader className="flex flex-row items-start gap-3">
											<Icon className="text-muted-foreground mt-1 h-5 w-5" />
											<div className="flex flex-col">
												<CardTitle className="text-base">{card.title}</CardTitle>
												<CardDescription>{card.description}</CardDescription>
											</div>
										</CardHeader>
										<CardContent className="text-muted-foreground text-sm">{t("plugins:rtk.admin.openCta")} →</CardContent>
									</Card>
								</Link>
							);
						})}
					</div>
				) : (
					<Outlet />
				)}
			</main>
		</div>
	);
}

export const Route = createFileRoute("/workspace/plugins/rtk")({
	component: RouteComponent,
});