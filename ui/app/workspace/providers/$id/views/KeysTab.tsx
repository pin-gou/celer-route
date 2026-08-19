import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage } from "@/lib/store";
import {
	useBatchUpdateProviderKeysMutation,
	useDeleteProviderKeyMutation,
	useGetProviderKeysQuery,
	useRefreshProviderKeyModelsMutation,
	useRefreshProviderModelsMutation,
	useUpdateProviderKeyMutation,
} from "@/lib/store/apis/providersApi";
import { ModelProvider } from "@/lib/types/config";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { AlertCircle, CheckCircle2, PencilIcon, PlusIcon, RefreshCwIcon, SearchIcon, TrashIcon } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import AddNewKeySheet from "@/app/workspace/providers/dialogs/addNewKeySheet";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alertDialog";

interface KeysTabProps {
	provider: ModelProvider;
}

const PAGE_SIZE = 10;

type HealthFilter = "all" | "active" | "error";

export function KeysTab({ provider }: KeysTabProps) {
	const { t } = useTranslation("providers");
	const hasUpdateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const hasDeleteAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Delete);
	const { data: keys = [], isLoading, refetch } = useGetProviderKeysQuery(provider.name);
	const [updateProviderKey] = useUpdateProviderKeyMutation();
	const [deleteProviderKey] = useDeleteProviderKeyMutation();
	const [refreshProviderModels, { isLoading: isRefreshingAll }] = useRefreshProviderModelsMutation();
	const [refreshProviderKeyModels] = useRefreshProviderKeyModelsMutation();
	const [batchUpdateKeys] = useBatchUpdateProviderKeysMutation();

	const [search, setSearch] = useState("");
	const [healthFilter, setHealthFilter] = useState<HealthFilter>("all");
	const [page, setPage] = useState(0);
	const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
	const [togglingId, setTogglingId] = useState<string | null>(null);
	const [testingKeyId, setTestingKeyId] = useState<string | null>(null);
	const [testingAll, setTestingAll] = useState(false);
	const [batchUpdating, setBatchUpdating] = useState<string | null>(null);
	const [batchDeleting, setBatchDeleting] = useState(false);
	const [batchRetesting, setBatchRetesting] = useState(false);
	const [showAddKeySheet, setShowAddKeySheet] = useState(false);
	const [editKeyId, setEditKeyId] = useState<string | null>(null);
	const [deleteConfirmKeyId, setDeleteConfirmKeyId] = useState<string | null>(null);

	const isKeyHealthy = (key: (typeof keys)[number]) => {
		if (key.enabled === false) return false;
		return key.status === "success" || !key.status;
	};

	const filtered = useMemo(() => {
		let result = keys;
		if (search.trim()) {
			const q = search.toLowerCase();
			result = result.filter((k) => k.name.toLowerCase().includes(q));
		}
		if (healthFilter === "active") {
			result = result.filter((k) => isKeyHealthy(k));
		} else if (healthFilter === "error") {
			result = result.filter((k) => !isKeyHealthy(k));
		}
		return result;
	}, [keys, search, healthFilter]);

	const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
	const clampedPage = Math.min(page, totalPages - 1);
	const pageKeys = filtered.slice(clampedPage * PAGE_SIZE, (clampedPage + 1) * PAGE_SIZE);

	const allSelected = pageKeys.length > 0 && pageKeys.every((k) => selectedIds.has(k.id));
	const someSelected = pageKeys.some((k) => selectedIds.has(k.id));

	const handleToggleSelect = (id: string) => {
		setSelectedIds((prev) => {
			const next = new Set(prev);
			if (next.has(id)) next.delete(id);
			else next.add(id);
			return next;
		});
	};

	const handleToggleSelectAll = () => {
		if (allSelected) {
			setSelectedIds((prev) => {
				const next = new Set(prev);
				for (const k of pageKeys) next.delete(k.id);
				return next;
			});
		} else {
			setSelectedIds((prev) => {
				const next = new Set(prev);
				for (const k of pageKeys) next.add(k.id);
				return next;
			});
		}
	};

	const handleToggleEnabled = async (keyId: string, key: (typeof keys)[number], enabled: boolean) => {
		setTogglingId(keyId);
		try {
			await updateProviderKey({ provider: provider.name, keyId, key: { ...key, enabled } }).unwrap();
			toast.success(enabled ? t("providers2.keysTab.toast.keyEnabled") : t("providers2.keysTab.toast.keyDisabled"));
		} catch (err) {
			toast.error(t("providers2.keysTab.toast.failedToUpdateKey"), { description: getErrorMessage(err) });
		} finally {
			setTogglingId(null);
		}
	};

	const handleRefreshAll = async () => {
		try {
			await refreshProviderModels(provider.name).unwrap();
			toast.success(t("providers2.keysTab.toast.modelRefreshStarted"));
		} catch (err: any) {
			if (err?.status === 409) {
				toast.info(t("providers2.keysTab.toast.modelRefreshRunning"));
			} else {
				toast.error(t("providers2.keysTab.toast.failedToRefreshModels"), { description: getErrorMessage(err) });
			}
		}
	};

	const handleTestKey = async (keyId: string) => {
		setTestingKeyId(keyId);
		try {
			await refreshProviderKeyModels({ provider: provider.name, keyId }).unwrap();
			await refetch();
			const key = keys.find((k) => k.id === keyId);
			if (key?.status === "success" || !key?.status) {
				toast.success(t("providers2.keysTab.toast.keyTestPassed"));
			} else {
				toast.error(t("providers2.keysTab.toast.keyTestFailed"), { description: key?.description || "Unknown error" });
			}
		} catch (err) {
			toast.error(t("providers2.keysTab.toast.keyTestFailed"), { description: getErrorMessage(err) });
		} finally {
			setTestingKeyId(null);
		}
	};

	const handleTestAll = async () => {
		setTestingAll(true);
		const enabledKeys = keys.filter((k) => k.enabled !== false);
		let passed = 0;
		let failed = 0;
		for (const key of enabledKeys) {
			try {
				await refreshProviderKeyModels({ provider: provider.name, keyId: key.id }).unwrap();
				passed++;
			} catch {
				failed++;
			}
		}
		await refetch();
		toast.success(t("providers2.keysTab.toast.keysTested", { count: enabledKeys.length, passed, failed }));
		setTestingAll(false);
	};

	const handleBatchToggle = async (enabled: boolean) => {
		if (selectedIds.size === 0) return;
		setBatchUpdating(enabled ? "activate" : "deactivate");
		try {
			const result = await batchUpdateKeys({
				provider: provider.name,
				key_ids: Array.from(selectedIds),
				enabled,
			}).unwrap();
			toast.success(t("providers2.keysTab.toast.keysUpdated", { count: result.updated }));
			setSelectedIds(new Set());
		} catch (err) {
			toast.error(t("providers2.keysTab.toast.batchUpdateFailed"), { description: getErrorMessage(err) });
		} finally {
			setBatchUpdating(null);
		}
	};

	const handleBatchDelete = async () => {
		if (selectedIds.size === 0) return;
		setBatchDeleting(true);
		let count = 0;
		for (const keyId of selectedIds) {
			try {
				await deleteProviderKey({ provider: provider.name, keyId }).unwrap();
				count++;
			} catch {
				// skip individual failures
			}
		}
		toast.success(t("providers2.keysTab.toast.keysDeleted", { count }));
		setSelectedIds(new Set());
		setBatchDeleting(false);
	};

	const handleBatchRetest = async () => {
		if (selectedIds.size === 0) return;
		setBatchRetesting(true);
		for (const keyId of selectedIds) {
			try {
				await refreshProviderKeyModels({ provider: provider.name, keyId }).unwrap();
			} catch {
				// skip
			}
		}
		await refetch();
		toast.success(t("providers2.keysTab.toast.keysRetested", { count: selectedIds.size }));
		setSelectedIds(new Set());
		setBatchRetesting(false);
	};

	const handleDeleteKey = async (keyId: string) => {
		setDeleteConfirmKeyId(null);
		try {
			await deleteProviderKey({ provider: provider.name, keyId }).unwrap();
			toast.success(t("providers2.keysTab.toast.keyDeleted"));
		} catch (err) {
			toast.error(t("providers2.keysTab.toast.failedToDeleteKey"), { description: getErrorMessage(err) });
		}
	};

	const isBulkBusy = batchUpdating !== null || batchDeleting || batchRetesting;

	const healthOptions = [
		{ value: "all" as const, label: t("providers2.keysTab.healthFilter.all") },
		{ value: "active" as const, label: t("providers2.keysTab.healthFilter.active") },
		{ value: "error" as const, label: t("providers2.keysTab.healthFilter.error") },
	];

	// Keyless providers (e.g. bare opencode) never carry API keys — show a
	// static notice instead of the key-management surface. The tab stays so
	// operators see the provider's Keys entry point, but there is nothing to
	// manage underneath.
	if (provider.is_key_less) {
		return (
			<div data-testid="providers2-keys-tab" className="rounded-lg border">
				<div data-testid="providers2-keys-keyless-notice" className="text-muted-foreground px-4 py-12 text-center text-xs">
					{t("providers2.keysTab.keylessNotice")}
				</div>
			</div>
		);
	}

	return (
		<div data-testid="providers2-keys-tab" className="rounded-lg border">
			{/* Toolbar */}
			<div className="flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
				<div className="flex items-center gap-2">
					{/* Search */}
					<div className="relative">
						<SearchIcon className="text-muted-foreground absolute top-1/2 left-2 h-3.5 w-3.5 -translate-y-1/2" />
						<input
							type="text"
							value={search}
							onChange={(e) => {
								setSearch(e.target.value);
								setPage(0);
							}}
							placeholder={t("providers2.keysTab.searchPlaceholder")}
							className="border-input w-48 rounded-md border py-1.5 pr-3 pl-7 text-xs outline-none focus:ring-1 focus:ring-blue-500"
						/>
					</div>
					{/* Health filter pills */}
					<div className="flex items-center gap-1">
						{healthOptions.map((opt) => (
							<button
								key={opt.value}
								onClick={() => {
									setHealthFilter(opt.value);
									setPage(0);
								}}
								className={`rounded-full px-2.5 py-1 text-xs font-medium transition-colors ${
									healthFilter === opt.value ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:bg-muted/80"
								}`}
							>
								{opt.label}
							</button>
						))}
					</div>
				</div>
				<div className="flex items-center gap-2">
					{selectedIds.size > 0 && (
						<>
							<span className="text-muted-foreground text-xs">{t("providers2.keysTab.bulk.selected", { count: selectedIds.size })}</span>
							<Button
								variant="outline"
								size="sm"
								className="text-xs"
								disabled={isBulkBusy || !hasUpdateAccess}
								onClick={() => handleBatchToggle(true)}
							>
								{t("providers2.keysTab.bulk.enable")}
							</Button>
							<Button
								variant="outline"
								size="sm"
								className="text-xs"
								disabled={isBulkBusy || !hasUpdateAccess}
								onClick={() => handleBatchToggle(false)}
							>
								{t("providers2.keysTab.bulk.disable")}
							</Button>
							<Button variant="outline" size="sm" className="text-xs" disabled={isBulkBusy || !hasUpdateAccess} onClick={handleBatchRetest}>
								{t("providers2.keysTab.bulk.retest")}
							</Button>
							<Button
								variant="outline"
								size="sm"
								className="text-xs text-red-500 hover:text-red-600"
								disabled={isBulkBusy || !hasDeleteAccess}
								onClick={handleBatchDelete}
							>
								{t("providers2.keysTab.bulk.delete")}
							</Button>
						</>
					)}
					{keys.length > 0 && (
						<Button variant="outline" size="sm" className="gap-1 text-xs" onClick={handleTestAll} disabled={testingAll}>
							<RefreshCwIcon className={`h-3 w-3 ${testingAll ? "animate-spin" : ""}`} />
							{testingAll ? t("providers2.keysTab.testing") : t("providers2.keysTab.testAll")}
						</Button>
					)}
					<Button variant="outline" size="sm" className="gap-1 text-xs" onClick={handleRefreshAll} disabled={isRefreshingAll}>
						<RefreshCwIcon className={`h-3 w-3 ${isRefreshingAll ? "animate-spin" : ""}`} />
						{isRefreshingAll ? t("providers2.keysTab.syncing") : t("providers2.keysTab.syncModels")}
					</Button>
					<Button size="sm" className="gap-1 text-xs" onClick={() => setShowAddKeySheet(true)}>
						<PlusIcon className="h-3 w-3" />
						{t("providers2.keysTab.addKey")}
					</Button>
				</div>
			</div>

			{/* Keys table */}
			{isLoading ? (
				<div className="text-muted-foreground flex h-24 items-center justify-center text-xs">{t("providers2.keysTab.loading")}</div>
			) : keys.length === 0 ? (
				<div className="text-muted-foreground flex h-24 items-center justify-center text-xs">{t("providers2.keysTab.noKeys")}</div>
			) : (
				<>
					<div className="overflow-x-auto">
						<table className="w-full text-left text-xs">
							<thead>
								<tr className="text-muted-foreground border-b">
									<th className="w-8 px-3 py-2">
										<input
											type="checkbox"
											checked={allSelected}
											ref={(el) => {
												if (el) el.indeterminate = someSelected && !allSelected;
											}}
											onChange={handleToggleSelectAll}
											className="h-4 w-4 rounded border-gray-300"
										/>
									</th>
									<th className="px-2 py-2 font-medium">{t("providers2.keysTab.table.name")}</th>
									<th className="w-20 px-2 py-2 font-medium">{t("providers2.keysTab.table.weight")}</th>
									<th className="w-24 px-2 py-2 font-medium">{t("providers2.keysTab.table.status")}</th>
									<th className="w-20 px-2 py-2 font-medium">{t("providers2.keysTab.table.enabled")}</th>
									<th className="w-28 px-2 py-2 font-medium">{t("providers2.keysTab.table.test")}</th>
									<th className="w-20 px-2 py-2 font-medium"></th>
								</tr>
							</thead>
							<tbody>
								{pageKeys.map((key) => {
									const healthy = isKeyHealthy(key);
									const isTesting = testingKeyId === key.id;
									return (
										<tr key={key.id} className="hover:bg-muted/30 border-b last:border-0">
											<td className="px-3 py-2">
												<input
													type="checkbox"
													checked={selectedIds.has(key.id)}
													onChange={() => handleToggleSelect(key.id)}
													className="h-4 w-4 rounded border-gray-300"
												/>
											</td>
											<td className="max-w-[200px] truncate px-2 py-2 font-mono text-xs" title={key.name}>
												<span className="inline-flex items-center gap-1">
													{healthy ? (
														<CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-green-500" />
													) : (
														<AlertCircle className="h-3.5 w-3.5 shrink-0 text-red-500" />
													)}
													<span className="truncate">{key.name}</span>
												</span>
											</td>
											<td className="px-2 py-2 font-mono">{key.weight}</td>
											<td className="px-2 py-2">
												{healthy ? (
													<span className="text-green-600">{t("providers2.keysTab.status.active")}</span>
												) : key.status === "list_models_failed" ? (
													<Tooltip>
														<TooltipTrigger asChild>
															<span className="cursor-help text-red-600 underline decoration-dotted">
																{t("providers2.keysTab.status.listModelsFailed")}
															</span>
														</TooltipTrigger>
														<TooltipContent className="max-w-xs">
															{key.description || t("providers2.keysTab.status.listModelsFailed")}
														</TooltipContent>
													</Tooltip>
												) : (
													<span className="text-red-500">{t("providers2.keysTab.status.error")}</span>
												)}
											</td>
											<td className="px-2 py-2">
												<Switch
													checked={key.enabled ?? true}
													size="md"
													disabled={!hasUpdateAccess || togglingId === key.id}
													onAsyncCheckedChange={async (checked) => {
														await handleToggleEnabled(key.id, key, checked);
													}}
												/>
											</td>
											<td className="px-2 py-2">
												<Button
													variant="outline"
													size="sm"
													className="text-xs"
													disabled={isTesting || key.enabled === false}
													onClick={() => handleTestKey(key.id)}
												>
													<RefreshCwIcon className={`mr-1 h-3 w-3 ${isTesting ? "animate-spin" : ""}`} />
													{isTesting ? t("providers2.keysTab.testing") : t("providers2.keysTab.test")}
												</Button>
											</td>
											<td className="px-2 py-2">
												<div className="flex items-center gap-1">
													{hasUpdateAccess && (
														<Tooltip>
															<TooltipTrigger asChild>
																<button
																	onClick={() => setEditKeyId(key.id)}
																	className="text-muted-foreground hover:text-foreground rounded p-1 transition-colors"
																>
																	<PencilIcon className="h-3.5 w-3.5" />
																</button>
															</TooltipTrigger>
															<TooltipContent>{t("providers2.keysTab.tooltip.edit")}</TooltipContent>
														</Tooltip>
													)}
													{hasDeleteAccess && (
														<Tooltip>
															<TooltipTrigger asChild>
																<button
																	onClick={() => setDeleteConfirmKeyId(key.id)}
																	className="text-muted-foreground rounded p-1 transition-colors hover:text-red-500"
																>
																	<TrashIcon className="h-3.5 w-3.5" />
																</button>
															</TooltipTrigger>
															<TooltipContent>{t("providers2.keysTab.tooltip.delete")}</TooltipContent>
														</Tooltip>
													)}
												</div>
											</td>
										</tr>
									);
								})}
							</tbody>
						</table>
					</div>

					{/* Pagination */}
					{totalPages > 1 && (
						<div className="text-muted-foreground flex items-center justify-between border-t px-4 py-2 text-xs">
							<span>
								{clampedPage * PAGE_SIZE + 1}–{Math.min((clampedPage + 1) * PAGE_SIZE, filtered.length)} / {filtered.length}
							</span>
							<div className="flex items-center gap-1">
								<button
									disabled={clampedPage === 0}
									onClick={() => setPage((p) => Math.max(0, p - 1))}
									className="disabled:text-muted-foreground/30 hover:bg-muted rounded px-2 py-1"
								>
									{t("providers2.keysTab.pagination.prev")}
								</button>
								<span className="min-w-[4rem] text-center">
									{clampedPage + 1} / {totalPages}
								</span>
								<button
									disabled={clampedPage >= totalPages - 1}
									onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
									className="disabled:text-muted-foreground/30 hover:bg-muted rounded px-2 py-1"
								>
									{t("providers2.keysTab.pagination.next")}
								</button>
							</div>
						</div>
					)}
				</>
			)}

			{/* Add / Edit key sheet */}
			{showAddKeySheet && (
				<AddNewKeySheet show={showAddKeySheet} onCancel={() => setShowAddKeySheet(false)} provider={provider} keyId={null} />
			)}
			{editKeyId && <AddNewKeySheet show={!!editKeyId} onCancel={() => setEditKeyId(null)} provider={provider} keyId={editKeyId} />}

			{/* Delete confirmation */}
			{deleteConfirmKeyId && (
				<AlertDialog open={!!deleteConfirmKeyId}>
					<AlertDialogContent onClick={(e) => e.stopPropagation()}>
						<AlertDialogHeader>
							<AlertDialogTitle>{t("providers2.keysTab.deleteConfirm.title")}</AlertDialogTitle>
							<AlertDialogDescription>{t("providers2.keysTab.deleteConfirm.description")}</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter className="pt-4">
							<AlertDialogCancel onClick={() => setDeleteConfirmKeyId(null)}>
								{t("providers2.keysTab.deleteConfirm.cancel")}
							</AlertDialogCancel>
							<AlertDialogAction className="bg-red-600 text-white hover:bg-red-700" onClick={() => handleDeleteKey(deleteConfirmKeyId)}>
								{t("providers2.keysTab.deleteConfirm.confirm")}
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			)}
		</div>
	);
}