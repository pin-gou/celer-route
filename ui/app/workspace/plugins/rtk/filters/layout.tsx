import { createFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useGetRtkFiltersQuery } from "@/lib/store";
import type { FilterCatalogEntry, FilterLoadDiagnostic } from "@/lib/types/rtk";
import { AlertCircle, AlertTriangle, Info, Loader2 } from "lucide-react";
import { useMemo, useState } from "react";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

// /workspace/plugins/rtk/filters — filter catalog browser.
//
// Lists the loaded filters (project + global + builtin) along with their
// priority and source, plus the diagnostics captured during Load. The page
// is read-only — operators edit filters via the project/global JSON files
// and re-load the plugin to apply changes.

function RouteComponent() {
	const { t } = useTranslation();
	const { data, isLoading, isError, refetch } = useGetRtkFiltersQuery();
	const [sourceFilter, setSourceFilter] = useState<string>("all");
	const [search, setSearch] = useState("");

	const entries = useMemo(() => {
		if (!data?.filters) return [] as FilterCatalogEntry[];
		return data.filters.filter((f) => {
			if (sourceFilter !== "all" && f.source !== sourceFilter) return false;
			if (search.trim()) {
				const q = search.toLowerCase();
				return f.id.toLowerCase().includes(q) || f.label.toLowerCase().includes(q) || (f.description ?? "").toLowerCase().includes(q);
			}
			return true;
		});
	}, [data, sourceFilter, search]);

	const diagnostics = (data?.diagnostics ?? []) as FilterLoadDiagnostic[];

	return (
		<div className="flex flex-col gap-6" data-testid="rtk-filters-page">
			<Card>
				<CardHeader>
					<CardTitle>{t("plugins:rtk.filters.title")}</CardTitle>
					<CardDescription>{t("plugins:rtk.filters.subtitle")}</CardDescription>
				</CardHeader>
				<CardContent className="flex flex-col gap-4">
					<div className="flex flex-wrap items-center gap-3">
						<Input
							data-testid="rtk-filters-search"
							placeholder={t("plugins:rtk.filters.searchPlaceholder")}
							className="max-w-xs"
							value={search}
							onChange={(e) => setSearch(e.target.value)}
						/>
						<Select value={sourceFilter} onValueChange={setSourceFilter}>
							<SelectTrigger data-testid="rtk-filters-source" className="w-[160px]">
								<SelectValue placeholder={t("plugins:rtk.filters.source")} />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="all">{t("plugins:rtk.filters.sourceAll")}</SelectItem>
								<SelectItem value="builtin">{t("plugins:rtk.filters.sourceBuiltin")}</SelectItem>
								<SelectItem value="project">{t("plugins:rtk.filters.sourceProject")}</SelectItem>
								<SelectItem value="global">{t("plugins:rtk.filters.sourceGlobal")}</SelectItem>
							</SelectContent>
						</Select>
						<Button data-testid="rtk-filters-refresh" variant="outline" size="sm" onClick={() => refetch()}>
							{t("plugins:rtk.filters.refresh")}
						</Button>
						<div className="text-muted-foreground ml-auto text-xs">
							{t("plugins:rtk.filters.counters", {
								shown: entries.length,
								total: data?.counters?.total ?? 0,
							})}
						</div>
					</div>

					{isLoading && (
						<div className="text-muted-foreground flex items-center gap-2 text-sm">
							<Loader2 className="h-4 w-4 animate-spin" />
							{t("plugins:rtk.filters.loading")}
						</div>
					)}
					{isError && <div className="text-destructive text-sm">{t("plugins:rtk.filters.error")}</div>}

					{!isLoading && !isError && (
						<div className="overflow-x-auto">
							<table className="w-full text-sm">
								<thead>
									<tr className="text-muted-foreground border-b text-xs uppercase">
										<th className="px-3 py-2 text-left">{t("plugins:rtk.filters.tableId")}</th>
										<th className="px-3 py-2 text-left">{t("plugins:rtk.filters.tableCategory")}</th>
										<th className="px-3 py-2 text-left">{t("plugins:rtk.filters.tableSource")}</th>
										<th className="px-3 py-2 text-right">{t("plugins:rtk.filters.tablePriority")}</th>
										<th className="px-3 py-2 text-right">{t("plugins:rtk.filters.tableTests")}</th>
									</tr>
								</thead>
								<tbody>
									{entries.map((entry) => (
										<tr key={`${entry.source}:${entry.id}`} data-testid="rtk-filters-row" className="hover:bg-muted/40 border-b">
											<td className="px-3 py-2">
												<div className="flex flex-col">
													<span className="font-medium">{entry.id}</span>
													{entry.description ? <span className="text-muted-foreground text-xs">{entry.description}</span> : null}
												</div>
											</td>
											<td className="px-3 py-2">
												<Badge variant="secondary">{entry.category || "generic"}</Badge>
											</td>
											<td className="px-3 py-2">
												<Badge variant="outline">{entry.source}</Badge>
											</td>
											<td className="px-3 py-2 text-right tabular-nums">{entry.priority}</td>
											<td className="px-3 py-2 text-right tabular-nums">{entry.tests_count}</td>
										</tr>
									))}
									{entries.length === 0 && (
										<tr>
											<td colSpan={5} className="text-muted-foreground px-3 py-6 text-center text-sm">
												{t("plugins:rtk.filters.empty")}
											</td>
										</tr>
									)}
								</tbody>
							</table>
						</div>
					)}
				</CardContent>
			</Card>

			{diagnostics.length > 0 && (
				<Card>
					<CardHeader>
						<CardTitle>{t("plugins:rtk.filters.diagnosticsTitle")}</CardTitle>
						<CardDescription>{t("plugins:rtk.filters.diagnosticsSubtitle")}</CardDescription>
					</CardHeader>
					<CardContent className="flex flex-col gap-3">
						{diagnostics.map((diag, idx) => (
							<div
								key={`${diag.path}-${idx}`}
								className="flex items-start gap-3 rounded-md border p-3 text-sm"
								data-testid={`rtk-filters-diag-${diag.level}`}
							>
								{diag.level === "error" ? (
									<AlertCircle className="text-destructive mt-0.5 h-4 w-4" />
								) : diag.level === "warning" ? (
									<AlertTriangle className="mt-0.5 h-4 w-4 text-amber-600" />
								) : (
									<Info className="text-muted-foreground mt-0.5 h-4 w-4" />
								)}
								<div className="flex-1">
									<div className="text-muted-foreground text-xs">
										{diag.source} · {diag.format} · {diag.path}
									</div>
									<div>{diag.message}</div>
								</div>
							</div>
						))}
					</CardContent>
				</Card>
			)}
		</div>
	);
}

export const Route = createFileRoute("/workspace/plugins/rtk/filters")({
	component: RouteComponent,
});