import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useGetAllKeysQuery } from "@/lib/store/apis/providersApi";
import { useGetCoreConfigQuery } from "@/lib/store";
import { Database, KeyRound, ListChecks } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";

type HealthState = "ok" | "missing" | "partial" | "error";

interface Indicator {
	labelKey: string;
	state: HealthState;
	href?: string;
	icon: ReactNode;
}

const stateClass: Record<HealthState, string> = {
	ok: "bg-emerald-500",
	missing: "bg-zinc-400",
	partial: "bg-amber-500",
	error: "bg-red-500",
};

export default function SystemHealthCard() {
	const { t } = useTranslation("common");
	const { data: config } = useGetCoreConfigQuery({});
	const { data: allKeys } = useGetAllKeysQuery();

	const indicators: Indicator[] = [
		{
			labelKey: "home.systemHealth.db",
			state: config?.is_db_connected ? "ok" : "missing",
			href: "/workspace/config/client-settings",
			icon: <Database className="h-4 w-4" />,
		},
		{
			labelKey: "home.systemHealth.logs",
			state: config?.is_logs_connected ? "ok" : "missing",
			href: "/workspace/config/logging",
			icon: <ListChecks className="h-4 w-4" />,
		},
		{
			labelKey: "home.systemHealth.providers",
			state: (allKeys?.length ?? 0) > 0 ? "ok" : "missing",
			href: "/workspace/providers",
			icon: <KeyRound className="h-4 w-4" />,
		},
	];

	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid="home-system-health">
			<CardHeader className="border-b px-6 py-3">
				<CardTitle className="text-sm font-semibold">{t("home.systemHealth.title")}</CardTitle>
			</CardHeader>
			<CardContent className="px-6 py-4">
				<div className="flex flex-wrap items-center gap-x-6 gap-y-3">
					{indicators.map((ind) => (
						<Link
							key={ind.labelKey}
							to={ind.href ?? "#"}
							className="group flex items-center gap-2 text-xs"
							data-testid={`home-system-health-${ind.labelKey.split(".").pop()}`}
						>
							<span className={["inline-block h-2.5 w-2.5 rounded-full", stateClass[ind.state]].join(" ")} aria-hidden />
							<span className="text-muted-foreground group-hover:text-foreground">{ind.icon}</span>
							<span className="text-foreground/80 group-hover:text-foreground">{t(ind.labelKey)}</span>
							<span className="text-muted-foreground group-hover:text-foreground">
								{t(
									`home.systemHealth.${ind.state === "ok" ? "ok" : ind.state === "missing" ? "missing" : ind.state === "partial" ? "partial" : "error"}`,
								)}
							</span>
						</Link>
					))}
				</div>
			</CardContent>
		</Card>
	);
}