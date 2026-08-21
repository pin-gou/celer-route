import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { setSelectedPlugin, useAppDispatch, useAppSelector, useGetPluginsQuery } from "@/lib/store";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { ListOrdered, PlusIcon, Puzzle } from "lucide-react";
import { useQueryState } from "nuqs";
import { useEffect, useMemo, useState } from "react";
import type { Plugin } from "@/lib/types/plugins";
import { getPluginDisplayName } from "@/lib/utils/pluginDisplayName";
import AddNewPluginSheet from "./sheets/addNewPluginSheet";
import PluginSequenceSheet from "./sheets/pluginSequenceSheet";
import { PluginsEmptyState } from "./views/pluginsEmptyState";
import PluginsView from "./views/pluginsView";

const placementRank = (plugin: Plugin): number => {
	switch (plugin.placement) {
		case "pre_builtin":
			return 0;
		case "builtin":
			return 1;
		case "post_builtin":
			return 2;
		default:
			return plugin.isCustom ? 2 : 1;
	}
};

// Sort plugins by their runtime execution order: pre_builtin custom plugins first,
// then built-in plugins, then post_builtin custom plugins. Within each group, order
// by the plugin's order field (lower = earlier), with name as a deterministic tiebreak.
const sortPluginsByRunOrder = (plugins: Plugin[]): Plugin[] =>
	[...plugins].sort((a, b) => {
		const rankDiff = placementRank(a) - placementRank(b);
		if (rankDiff !== 0) return rankDiff;
		const orderDiff = (a.order ?? 0) - (b.order ?? 0);
		if (orderDiff !== 0) return orderDiff;
		return a.name.localeCompare(b.name);
	});

export default function PluginsPage() {
	const { t } = useTranslation("plugins");
	const dispatch = useAppDispatch();
	const hasCreatePluginAccess = useRbac(RbacResource.Plugins, RbacOperation.Create);
	const hasUpdatePluginAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const { data: plugins, isLoading } = useGetPluginsQuery();
	const selectedPlugin = useAppSelector((state) => state.plugin.selectedPlugin);
	const [selectedPluginId, setSelectedPluginId] = useQueryState("plugin");
	const allPlugins = useMemo(() => sortPluginsByRunOrder(plugins ?? []), [plugins]);

	const HIDDEN_PLUGIN_NAMES = new Set(["telemetry", "maxim", "mocker", "otel", "prompts", "semantic_cache", "logging"]);
	const visiblePlugins = useMemo(() => allPlugins.filter((p) => !HIDDEN_PLUGIN_NAMES.has(p.name)), [allPlugins]);

	const hasCustomPlugins = useMemo(() => allPlugins.some((plugin) => plugin.isCustom), [allPlugins]);
	const [isSheetOpen, setIsSheetOpen] = useState(false);
	const [isSequenceSheetOpen, setIsSequenceSheetOpen] = useState(false);

	const handleAddNew = () => {
		setIsSheetOpen(true);
	};

	const handleCloseSheet = () => {
		setIsSheetOpen(false);
	};

	useEffect(() => {
		if (!selectedPluginId) return;
		const plugin = allPlugins?.find((plugin) => plugin.name === selectedPluginId);
		if (plugin) {
			dispatch(setSelectedPlugin(plugin));
		}
	}, [selectedPluginId, allPlugins]);

	useEffect(() => {
		if (selectedPluginId) return;
		if (!selectedPlugin) {
			setSelectedPluginId(visiblePlugins?.[0]?.name ?? "");
			return;
		}
		setSelectedPluginId(selectedPlugin?.name ?? "");
	}, [visiblePlugins]);

	if (visiblePlugins?.length === 0 && !isLoading) {
		return (
			<div className="mx-auto w-full max-w-7xl">
				<PluginsEmptyState onCreateClick={handleAddNew} canCreate={hasCreatePluginAccess} />
				<AddNewPluginSheet
					open={isSheetOpen}
					onClose={handleCloseSheet}
					onCreate={(pluginName) => {
						setSelectedPluginId(pluginName);
					}}
				/>
			</div>
		);
	}

	return (
		<div className="mx-auto w-full max-w-7xl">
			<h1 className="sr-only">{t("page.title")}</h1>
			<div className="flex flex-row gap-4">
				<div className="sticky top-0 flex min-w-[250px] flex-col gap-2 self-start pb-10">
					<div className="rounded-md bg-zinc-50/50 p-4 dark:bg-zinc-800/20">
						<div className="mb-4">
							<div className="text-muted-foreground mb-2 text-xs font-medium">{t("sidebar.title")}</div>
							{visiblePlugins.map((plugin) => (
								<button
									type="button"
									key={plugin.name}
									data-testid="plugin-list-item"
									aria-current={selectedPlugin?.name === plugin.name ? "page" : undefined}
									className={cn(
										"mb-1 flex max-h-[32px] w-full items-center gap-2 rounded-sm border px-3 py-1.5 text-sm",
										selectedPlugin?.name === plugin.name
											? "bg-secondary opacity-100 hover:opacity-100"
											: "hover:bg-secondary cursor-pointer border-transparent opacity-100 hover:border",
									)}
									onClick={() => {
										setSelectedPluginId(plugin.name);
									}}
								>
									<div className="flex min-w-0 flex-row items-center gap-2">
										<Puzzle className="text-muted-foreground size-3.5 shrink-0" />
										<span className="truncate">{getPluginDisplayName(plugin, t)}</span>
										{!plugin.isCustom && (
											<Badge variant="secondary" className="text-muted-foreground h-4 px-1 text-[10px] leading-none font-normal">
												{t("sidebar.builtIn")}
											</Badge>
										)}
									</div>
									<div
										className={cn(
											"ml-auto h-2 w-2 animate-pulse rounded-full",
											plugin.status?.status === "active" ? "bg-green-800 dark:bg-green-200" : "bg-red-800 dark:bg-red-400",
										)}
									/>
								</button>
							))}
							<div className="my-4 flex flex-col gap-2">
								<Button
									data-testid="plugins-create-button"
									variant="outline"
									size="sm"
									className="w-full justify-start"
									disabled={!hasCreatePluginAccess}
									onClick={(e) => {
										e.preventDefault();
										e.stopPropagation();
										handleAddNew();
									}}
								>
									<PlusIcon className="h-4 w-4" />
									<div className="text-xs">{t("sidebar.installNewPlugin")}</div>
								</Button>
								{hasCustomPlugins && (
									<Button
										variant="outline"
										size="sm"
										className="w-full justify-start"
										disabled={!hasUpdatePluginAccess}
										onClick={() => setIsSequenceSheetOpen(true)}
										data-testid="plugins-sequence-button"
									>
										<ListOrdered className="h-4 w-4" />
										<div className="text-xs">{t("sidebar.editPluginSequence")}</div>
									</Button>
								)}
							</div>
						</div>
					</div>
				</div>
				<PluginsView
					onDelete={() => {
						setSelectedPluginId(visiblePlugins?.[0]?.name ?? "");
					}}
					onCreate={(pluginName) => {
						setSelectedPluginId(pluginName ?? "");
					}}
				/>
			</div>
			<AddNewPluginSheet
				open={isSheetOpen}
				onClose={handleCloseSheet}
				onCreate={(pluginName) => {
					setSelectedPluginId(pluginName);
				}}
			/>
			<PluginSequenceSheet open={isSequenceSheetOpen} onClose={() => setIsSequenceSheetOpen(false)} plugins={plugins ?? []} />
		</div>
	);
}