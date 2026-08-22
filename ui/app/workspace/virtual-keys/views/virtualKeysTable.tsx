import { BudgetDisplay } from "@/components/budgetDisplay";
import { RateLimitDisplay } from "@/components/rateLimitDisplay";
import { PIN_SHADOW_RIGHT } from "@/components/table/columnPinning";
import { useTranslation } from "react-i18next";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Pagination } from "@/components/ui/pagination";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { resetDurationLabels } from "@/lib/constants/governance";
import {
	getErrorMessage,
	useBulkRotateVirtualKeysMutation,
	useDeleteVirtualKeyMutation,
	useGetVirtualKeyQuery,
	useLazyGetVirtualKeysQuery,
	useUpdateVirtualKeyMutation,
} from "@/lib/store";
import { VirtualKey } from "@/lib/types/governance";
import { cn } from "@/lib/utils";
import { formatCurrency, getEffectiveBudgetLimit } from "@/lib/utils/governance";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { Link } from "@tanstack/react-router";
import {
	ArrowDown,
	ArrowUp,
	ArrowUpDown,
	Copy,
	Download,
	Edit,
	Eye,
	EyeOff,
	Loader2,
	Plus,
	RotateCcw,
	ScrollText,
	Search,
	ShieldCheck,
	Trash2,
	X,
} from "lucide-react";
import { useQueryState } from "nuqs";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { useVirtualKeyUsage } from "../hooks/useVirtualKeyUsage";
import VirtualKeyDetailSheet from "./virtualKeyDetailsSheet";
import { VirtualKeysEmptyState } from "./virtualKeysEmptyState";
import VirtualKeySheet from "./virtualKeySheet";

const formatResetDuration = (duration: string) => resetDurationLabels[duration] || duration;

type ExportScope = "current_page" | "all";

function virtualKeysToCSV(vks: VirtualKey[]): string {
	const headers = ["Name", "Status", "Assigned To", "Budget Limit", "Budget Spent", "Budget Reset", "Description", "Created At"];
	const rows = vks.map((vk) => {
		const isExhausted =
			vk.budgets?.some((b) => b.current_usage >= getEffectiveBudgetLimit(b)) ||
			(vk.rate_limit?.token_current_usage &&
				vk.rate_limit?.token_max_limit &&
				vk.rate_limit.token_current_usage >= vk.rate_limit.token_max_limit) ||
			(vk.rate_limit?.request_current_usage &&
				vk.rate_limit?.request_max_limit &&
				vk.rate_limit.request_current_usage >= vk.rate_limit.request_max_limit);
		const isExpired = !!vk.expires_at && Date.now() >= new Date(vk.expires_at).getTime();
		const status = !vk.is_active ? "Inactive" : isExpired ? "Expired" : isExhausted ? "Exhausted" : "Active";
		const assignedTo = vk.team ? `Team: ${vk.team.name}` : vk.customer ? `Customer: ${vk.customer.name}` : "";
		const budgetLimit = vk.budgets?.length ? vk.budgets.map((b) => formatCurrency(getEffectiveBudgetLimit(b))).join("; ") : "";
		const budgetSpent = vk.budgets?.length ? vk.budgets.map((b) => formatCurrency(b.current_usage)).join("; ") : "";
		const budgetReset = vk.budgets?.length ? vk.budgets.map((b) => formatResetDuration(b.reset_duration)).join("; ") : "";
		return [vk.name, status, assignedTo, budgetLimit, budgetSpent, budgetReset, vk.description || "", vk.created_at];
	});
	return [headers, ...rows].map((row) => row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(",")).join("\n");
}

function downloadCSV(content: string) {
	const blob = new Blob([content], { type: "text/csv;charset=utf-8;" });
	const url = URL.createObjectURL(blob);
	const link = document.createElement("a");
	link.href = url;
	link.download = `virtual-keys-${new Date().toISOString().split("T")[0]}.csv`;
	link.click();
	URL.revokeObjectURL(url);
}

function VKBudgetCell({ vk }: { vk: VirtualKey }) {
	const { displayBudgets } = useVirtualKeyUsage(vk);
	return <BudgetDisplay budgets={displayBudgets} calendarAligned={vk.calendar_aligned} />;
}

function VKRateLimitCell({ vk }: { vk: VirtualKey }) {
	const { displayRateLimit } = useVirtualKeyUsage(vk);
	return <RateLimitDisplay rateLimits={displayRateLimit} calendarAligned={vk.calendar_aligned} />;
}

function VKActiveSwitch({
	vk,
	hasUpdateAccess,
	onToggle,
}: {
	vk: VirtualKey;
	hasUpdateAccess: boolean;
	onToggle: (vk: VirtualKey, checked: boolean) => Promise<void>;
}) {
	const { t } = useTranslation("governance-ui");
	const { isManagedByProfile } = useVirtualKeyUsage(vk);

	return (
		<Switch
			checked={vk.is_active}
			disabled={!hasUpdateAccess || isManagedByProfile}
			aria-label={t("table.toggleActive", { action: vk.is_active ? "Disable" : "Enable", name: vk.name })}
			data-testid={`vk-active-switch-${vk.name}`}
			title={isManagedByProfile ? t("table.managedSwitchTitle") : undefined}
			onAsyncCheckedChange={(checked) => onToggle(vk, checked)}
		/>
	);
}

function VKActionsMenu({
	vk,
	hasUpdateAccess,
	hasDeleteAccess,
	isDeleting,
	onEdit,
	onDelete,
}: {
	vk: VirtualKey;
	hasUpdateAccess: boolean;
	hasDeleteAccess: boolean;
	isDeleting: boolean;
	onEdit: (vk: VirtualKey) => void;
	onDelete: (vkId: string) => void;
}) {
	const { t } = useTranslation("governance-ui");
	const { isManagedByProfile } = useVirtualKeyUsage(vk);
	const [deleteOpen, setDeleteOpen] = useState(false);

	return (
		<>
			<div className="flex items-center justify-end gap-0.5">
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="ghost"
							size="icon-sm"
							disabled={!hasUpdateAccess}
							aria-label={t("actions.edit")}
							data-testid={`vk-edit-btn-${vk.name}`}
							onClick={() => onEdit(vk)}
						>
							<Edit className="h-3.5 w-3.5" />
						</Button>
					</TooltipTrigger>
					<TooltipContent>{t("actions.edit")}</TooltipContent>
				</Tooltip>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button variant="ghost" size="icon-sm" aria-label={t("actions.viewLogs")} data-testid={`vk-view-logs-btn-${vk.name}`} asChild>
							<Link to="/workspace/logs" search={{ virtual_key_ids: [vk.id] }}>
								<ScrollText className="h-3.5 w-3.5" />
							</Link>
						</Button>
					</TooltipTrigger>
					<TooltipContent>{t("actions.viewLogs")}</TooltipContent>
				</Tooltip>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="ghost"
							size="icon-sm"
							disabled={!hasDeleteAccess || isManagedByProfile}
							title={isManagedByProfile ? t("deleteDialog.managedDisabledTitle") : undefined}
							aria-label={t("actions.delete")}
							data-testid={`vk-delete-btn-${vk.name}`}
							onClick={() => setDeleteOpen(true)}
						>
							<Trash2 className="h-3.5 w-3.5" />
						</Button>
					</TooltipTrigger>
					<TooltipContent>{t("actions.delete")}</TooltipContent>
				</Tooltip>
			</div>
			<AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("deleteDialog.title")}</AlertDialogTitle>
						<AlertDialogDescription>
							{t("deleteDialog.confirmMessage", { name: vk.name.length > 20 ? `${vk.name.slice(0, 20)}...` : vk.name })}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel data-testid={`vk-delete-cancel-${vk.name}`}>{t("common:cancel")}</AlertDialogCancel>
						<AlertDialogAction
							onClick={() => onDelete(vk.id)}
							disabled={isDeleting}
							className="bg-destructive hover:bg-destructive/90"
							data-testid={`vk-delete-confirm-${vk.name}`}
						>
							{isDeleting ? t("deleteDialog.deleting") : t("common:delete")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	);
}

interface VirtualKeysTableProps {
	virtualKeys: VirtualKey[];
	totalCount: number;
	search: string;
	debouncedSearch: string;
	onSearchChange: (value: string) => void;
	offset: number;
	limit: number;
	onOffsetChange: (offset: number) => void;
	sortBy?: string;
	order?: string;
	onSortChange: (sortBy: string, order: string) => void;
	selectedVkId: string;
	onSelectedVkChange: (id: string, options?: { offset?: number }) => void;
}

export default function VirtualKeysTable({
	virtualKeys,
	totalCount,
	search,
	debouncedSearch,
	onSearchChange,
	offset,
	limit,
	onOffsetChange,
	sortBy,
	order,
	onSortChange,
	selectedVkId,
	onSelectedVkChange,
}: VirtualKeysTableProps) {
	const { t } = useTranslation("governance-ui");
	const [showVirtualKeySheet, setShowVirtualKeySheet] = useState(false);
	const [editingVirtualKeyId, setEditingVirtualKeyId] = useState<string | null>(null);
	const [revealedKeys, setRevealedKeys] = useState<Set<string>>(new Set());
	const [showExportDialog, setShowExportDialog] = useState(false);
	const [exportScope, setExportScope] = useState<ExportScope>("current_page");
	const [exportMaxLimit, setExportMaxLimit] = useState("");
	const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
	const [showBulkRotateDialog, setShowBulkRotateDialog] = useState(false);
	const [fetchVirtualKeys, { isFetching: isExporting }] = useLazyGetVirtualKeysQuery();

	// Derive objects from props so they stay in sync with RTK cache updates
	const editingVirtualKey = useMemo(
		() => (editingVirtualKeyId ? (virtualKeys.find((vk) => vk.id === editingVirtualKeyId) ?? null) : null),
		[editingVirtualKeyId, virtualKeys],
	);
	const selectedVkInList = useMemo(
		() => (selectedVkId ? (virtualKeys.find((vk) => vk.id === selectedVkId) ?? null) : null),
		[selectedVkId, virtualKeys],
	);
	// Deep-link support: another page (e.g. Model Limits) can open a VK via ?vk=<id>.
	// The target may not be on the current page/filter, so fetch it by id as a fallback.
	const [vkParam, setVkParam] = useQueryState("vk");
	const needsVkFetch = !!selectedVkId && !selectedVkInList;
	const { data: fetchedVkData } = useGetVirtualKeyQuery(selectedVkId ?? "", {
		skip: !needsVkFetch,
	});
	const selectedVirtualKey = selectedVkInList ?? (needsVkFetch ? (fetchedVkData?.virtual_key ?? null) : null);

	useEffect(() => {
		if (!vkParam) return;
		onSelectedVkChange(vkParam);
		setVkParam(null); // consume the param; selection is held in parent state from here
	}, [vkParam, setVkParam, onSelectedVkChange]);

	const hasCreateAccess = useRbac(RbacResource.VirtualKeys, RbacOperation.Create);
	const hasUpdateAccess = useRbac(RbacResource.VirtualKeys, RbacOperation.Update);
	const hasDeleteAccess = useRbac(RbacResource.VirtualKeys, RbacOperation.Delete);

	const [deleteVirtualKey, { isLoading: isDeleting }] = useDeleteVirtualKeyMutation();
	const [updateVirtualKey] = useUpdateVirtualKeyMutation();
	const [bulkRotateVirtualKeys, { isLoading: isBulkRotating }] = useBulkRotateVirtualKeysMutation();

	const visibleIds = useMemo(() => virtualKeys.map((vk) => vk.id), [virtualKeys]);
	const selectedVisibleIds = useMemo(() => visibleIds.filter((id) => selectedIds.has(id)), [selectedIds, visibleIds]);
	const selectedCount = selectedIds.size;
	const allVisibleSelected = visibleIds.length > 0 && selectedVisibleIds.length === visibleIds.length;
	const someVisibleSelected = selectedVisibleIds.length > 0 && selectedVisibleIds.length < visibleIds.length;

	const toggleSelectAllVisible = (checked: boolean) => {
		setSelectedIds((prev) => {
			const next = new Set(prev);
			for (const id of visibleIds) {
				if (checked) {
					next.add(id);
				} else {
					next.delete(id);
				}
			}
			return next;
		});
	};

	const toggleSelectVirtualKey = (vkId: string, checked: boolean) => {
		setSelectedIds((prev) => {
			const next = new Set(prev);
			if (checked) {
				next.add(vkId);
			} else {
				next.delete(vkId);
			}
			return next;
		});
	};

	const handleDelete = async (vkId: string) => {
		try {
			await deleteVirtualKey(vkId).unwrap();
			toast.success(t("toast.vkDeleted"));
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	};

	const handleToggleActive = async (vk: VirtualKey, checked: boolean) => {
		try {
			await updateVirtualKey({
				vkId: vk.id,
				data: { is_active: checked },
			}).unwrap();
			toast.success(t(checked ? "toast.vkEnabled" : "toast.vkDisabled"));
		} catch (error) {
			toast.error(getErrorMessage(error));
			throw error;
		}
	};

	const handleBulkRotate = async () => {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;

		try {
			const result = await bulkRotateVirtualKeys({ ids }).unwrap();
			const rotatedIds = new Set(result.virtual_keys.map((vk) => vk.id));
			setSelectedIds((prev) => {
				const next = new Set(prev);
				for (const id of rotatedIds) {
					next.delete(id);
				}
				return next;
			});
			setRevealedKeys((prev) => {
				const next = new Set(prev);
				for (const id of rotatedIds) {
					next.delete(id);
				}
				return next;
			});
			setShowBulkRotateDialog(false);

			const failureCount = result.errors ? Object.keys(result.errors).length : 0;
			if (failureCount > 0) {
				toast.warning(t("toast.rotatePartial", { count: result.virtual_keys.length, failed: failureCount }));
			} else {
				toast.success(t("toast.rotateSuccess", { count: result.virtual_keys.length }));
			}
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	};

	const handleAddVirtualKey = () => {
		setEditingVirtualKeyId(null);
		setShowVirtualKeySheet(true);
	};

	const handleEditVirtualKey = (vk: VirtualKey) => {
		setEditingVirtualKeyId(vk.id);
		setShowVirtualKeySheet(true);
	};

	const handleVirtualKeySaved = () => {
		setShowVirtualKeySheet(false);
		setEditingVirtualKeyId(null);
	};

	const handleRowClick = (vk: VirtualKey) => {
		onSelectedVkChange(vk.id);
	};

	const handleDetailSheetClose = () => {
		onSelectedVkChange("");
	};

	const selectedVirtualKeyIndex = useMemo(
		() => (selectedVkId ? virtualKeys.findIndex((vk) => vk.id === selectedVkId) : -1),
		[selectedVkId, virtualKeys],
	);

	const handleDetailNavigate = (direction: "prev" | "next") => {
		const currentVkId = selectedVkId;
		if (direction === "prev") {
			if (selectedVirtualKeyIndex > 0) {
				onSelectedVkChange(virtualKeys[selectedVirtualKeyIndex - 1].id);
			} else if (offset > 0) {
				const newOffset = Math.max(0, offset - limit);
				onSelectedVkChange("", { offset: newOffset });
				fetchVirtualKeys({
					limit,
					offset: newOffset,
					search: debouncedSearch || undefined,
					sort_by: (sortBy as "name" | "budget_spent" | "created_at" | "status") || undefined,
					order: (order as "asc" | "desc") || undefined,
				}).then((result) => {
					if (result.data?.virtual_keys?.length) {
						const lastVk = result.data.virtual_keys[result.data.virtual_keys.length - 1];
						onSelectedVkChange(lastVk.id);
					} else if (result.error) {
						onSelectedVkChange(currentVkId, { offset });
					}
				});
			}
		} else {
			if (selectedVirtualKeyIndex >= 0 && selectedVirtualKeyIndex < virtualKeys.length - 1) {
				onSelectedVkChange(virtualKeys[selectedVirtualKeyIndex + 1].id);
			} else if (offset + limit < totalCount) {
				const newOffset = offset + limit;
				onSelectedVkChange("", { offset: newOffset });
				fetchVirtualKeys({
					limit,
					offset: newOffset,
					search: debouncedSearch || undefined,
					sort_by: (sortBy as "name" | "budget_spent" | "created_at" | "status") || undefined,
					order: (order as "asc" | "desc") || undefined,
				}).then((result) => {
					if (result.data?.virtual_keys?.length) {
						const firstVk = result.data.virtual_keys[0];
						onSelectedVkChange(firstVk.id);
					} else if (result.error) {
						onSelectedVkChange(currentVkId, { offset });
					}
				});
			}
		}
	};

	const toggleKeyVisibility = (vkId: string) => {
		const newRevealed = new Set(revealedKeys);
		if (newRevealed.has(vkId)) {
			newRevealed.delete(vkId);
		} else {
			newRevealed.add(vkId);
		}
		setRevealedKeys(newRevealed);
	};

	const maskKey = (key: string, revealed: boolean) => {
		if (revealed) return key;
		return key.substring(0, 8) + "•".repeat(Math.max(0, key.length - 8));
	};

	const { copy: copyToClipboard } = useCopyToClipboard();

	const hasActiveFilters = !!debouncedSearch;

	const toggleSort = (column: string) => {
		if (sortBy === column) {
			if (order === "asc") {
				onSortChange(column, "desc");
			} else {
				// Clicking again clears sort
				onSortChange("", "");
			}
		} else {
			onSortChange(column, "asc");
		}
	};

	const handleExportCSV = async () => {
		if (exportScope === "current_page") {
			downloadCSV(virtualKeysToCSV(virtualKeys));
			toast.success(t("toast.exported", { count: virtualKeys.length }));
			setShowExportDialog(false);
			return;
		}

		// Fetch all with same filters/sort applied
		const maxLimit = exportMaxLimit ? parseInt(exportMaxLimit, 10) : undefined;
		const fetchLimit = maxLimit && maxLimit > 0 ? maxLimit : 10000;

		try {
			const result = await fetchVirtualKeys({
				limit: fetchLimit,
				offset: 0,
				search: debouncedSearch || undefined,
				sort_by: (sortBy as "name" | "budget_spent" | "created_at" | "status") || undefined,
				order: (order as "asc" | "desc") || undefined,
				export: true,
			}).unwrap();

			downloadCSV(virtualKeysToCSV(result.virtual_keys));
			toast.success(t("toast.exported", { count: result.virtual_keys.length }));
			setShowExportDialog(false);
		} catch (error) {
			toast.error(t("toast.exportFailed", { error: getErrorMessage(error) }));
		}
	};

	const openExportDialog = () => {
		setExportScope("current_page");
		setExportMaxLimit("");
		setShowExportDialog(true);
	};

	const SortableHeader = ({ column, label }: { column: string; label: string }) => {
		const isActive = sortBy === column;
		const Icon = isActive ? (order === "desc" ? ArrowDown : ArrowUp) : ArrowUpDown;
		return (
			<Button variant="ghost" onClick={() => toggleSort(column)} data-testid={`vk-sort-${column}`} className="!px-0">
				{label}
				<Icon className={cn("ml-2 h-4 w-4", isActive && "text-foreground")} />
			</Button>
		);
	};

	// True empty state: no VKs at all (not just filtered to zero)
	if (totalCount === 0 && !hasActiveFilters) {
		return (
			<>
				{showVirtualKeySheet && (
					<VirtualKeySheet virtualKey={editingVirtualKey} onSave={handleVirtualKeySaved} onCancel={() => setShowVirtualKeySheet(false)} />
				)}
				<VirtualKeysEmptyState onAddClick={handleAddVirtualKey} canCreate={hasCreateAccess} />
			</>
		);
	}

	return (
		<>
			{showVirtualKeySheet && (
				<VirtualKeySheet virtualKey={editingVirtualKey} onSave={handleVirtualKeySaved} onCancel={() => setShowVirtualKeySheet(false)} />
			)}

			{!!selectedVkId && selectedVirtualKey && (
				<VirtualKeyDetailSheet
					virtualKey={selectedVirtualKey}
					onClose={handleDetailSheetClose}
					onNavigate={handleDetailNavigate}
					hasPrev={selectedVirtualKeyIndex > 0 || (selectedVirtualKeyIndex !== -1 && offset > 0)}
					hasNext={selectedVirtualKeyIndex !== -1 && (selectedVirtualKeyIndex < virtualKeys.length - 1 || offset + limit < totalCount)}
				/>
			)}

			{/* Export Dialog */}
			<Dialog open={showExportDialog} onOpenChange={setShowExportDialog}>
				<DialogContent className="sm:max-w-[425px]">
					<DialogHeader className="pb-0">
						<DialogTitle>{t("exportDialog.title")}</DialogTitle>
						<DialogDescription>{t("exportDialog.description")}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div className="space-y-2">
							<Label className="text-sm">{t("exportDialog.exportScope")}</Label>
							<div className="grid grid-cols-2 gap-2" data-testid="vk-export-scope">
								<button
									type="button"
									onClick={() => setExportScope("current_page")}
									className={cn(
										"flex cursor-pointer flex-col items-center gap-1 rounded-md border px-3 py-3 text-sm transition-colors",
										exportScope === "current_page"
											? "border-primary bg-primary/5 text-foreground"
											: "border-border text-muted-foreground hover:border-primary/50 hover:text-foreground",
									)}
								>
									<span className="font-medium">{t("exportDialog.currentPage")}</span>
									<span className="text-muted-foreground text-xs">{t("exportDialog.entries", { count: virtualKeys.length })}</span>
								</button>
								<button
									type="button"
									onClick={() => setExportScope("all")}
									className={cn(
										"flex cursor-pointer flex-col items-center gap-1 rounded-md border px-3 py-3 text-sm transition-colors",
										exportScope === "all"
											? "border-primary bg-primary/5 text-foreground"
											: "border-border text-muted-foreground hover:border-primary/50 hover:text-foreground",
									)}
								>
									<span className="font-medium">{t("exportDialog.allEntries")}</span>
									<span className="text-muted-foreground text-xs">{t("exportDialog.total", { count: totalCount })}</span>
								</button>
							</div>
						</div>

						{exportScope === "all" && (
							<div className="space-y-2">
								<Label htmlFor="export-max-limit" className="text-sm">
									{t("exportDialog.maxEntries")} <span className="text-muted-foreground font-normal">({t("exportDialog.optional")})</span>
								</Label>
								<Input
									id="export-max-limit"
									type="number"
									min="1"
									placeholder={t("exportDialog.maxEntriesPlaceholder", { count: totalCount })}
									value={exportMaxLimit}
									onChange={(e) => setExportMaxLimit(e.target.value)}
									data-testid="vk-export-max-limit"
								/>
							</div>
						)}

						{hasActiveFilters && (
							<p className="text-muted-foreground text-xs">{t("exportDialog.filtersApplied", { query: debouncedSearch })}</p>
						)}

						<div className="text-muted-foreground flex items-center gap-2">
							<ShieldCheck className="h-3.5 w-3.5 shrink-0" />
							<p className="text-xs">{t("exportDialog.tokensExcluded")}</p>
						</div>
					</div>
					<DialogFooter className="pt-0">
						<Button variant="outline" onClick={() => setShowExportDialog(false)} disabled={isExporting}>
							{t("common:cancel")}
						</Button>
						<Button onClick={handleExportCSV} disabled={isExporting} data-testid="vk-export-confirm-btn">
							{isExporting ? (
								<>
									<Loader2 className="h-4 w-4 animate-spin" />
									{t("exportDialog.exporting")}
								</>
							) : (
								<>
									<Download className="h-4 w-4" />
									{t("exportDialog.exportCSV")}
								</>
							)}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<AlertDialog open={showBulkRotateDialog} onOpenChange={setShowBulkRotateDialog}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("bulkRotate.title")}</AlertDialogTitle>
						<AlertDialogDescription>{t("bulkRotate.description", { count: selectedCount })}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel data-testid="vk-bulk-rotate-cancel-btn">{t("common:cancel")}</AlertDialogCancel>
						<AlertDialogAction
							onClick={handleBulkRotate}
							disabled={isBulkRotating || selectedCount === 0}
							data-testid="vk-bulk-rotate-confirm-btn"
						>
							{isBulkRotating ? t("bulkRotate.rotating") : t("bulkRotate.rotateSelected")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			<div className="flex min-h-0 w-full grow flex-col overflow-hidden">
				<div className="mb-4 flex shrink-0 items-center justify-between">
					<div>
						<h2 className="text-lg font-semibold">{t("page.title")}</h2>
						<p className="text-muted-foreground text-sm">{t("page.description")}</p>
					</div>
					<div className="flex items-center gap-2">
						{selectedCount > 0 && (
							<Button
								variant="outline"
								onClick={() => setShowBulkRotateDialog(true)}
								disabled={!hasUpdateAccess || isBulkRotating}
								data-testid="vk-bulk-rotate-btn"
							>
								<RotateCcw className="h-4 w-4" />
								{t("page.rotateSelected", { count: selectedCount })}
							</Button>
						)}
						<Button variant="outline" onClick={openExportDialog} disabled={virtualKeys.length === 0} data-testid="vk-export-btn">
							<Download className="h-4 w-4" />
							{t("page.exportCSV")}
						</Button>
						<Button onClick={handleAddVirtualKey} disabled={!hasCreateAccess} data-testid="create-vk-btn">
							<Plus className="h-4 w-4" />
							{t("page.addVirtualKey")}
						</Button>
					</div>
				</div>

				{/* Toolbar: Search + Filters */}
				<div className="mb-4 flex shrink-0 items-center gap-3">
					<div className="relative max-w-sm flex-1">
						<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
						<Input
							aria-label={t("table.searchLabel")}
							placeholder={t("table.searchPlaceholder")}
							value={search}
							onChange={(e) => onSearchChange(e.target.value)}
							className="pl-9"
							data-testid="vk-search-input"
						/>
					</div>
				</div>

				<div className="mb-2 min-h-0 grow overflow-hidden rounded-sm border">
					<Table containerClassName="h-full overflow-auto" className="w-full min-w-[1528px] table-fixed" data-testid="vk-table">
						<TableHeader className="bg-muted sticky top-0 z-20">
							<TableRow>
								<TableHead className="w-[48px]">
									<Checkbox
										checked={allVisibleSelected || (someVisibleSelected ? "indeterminate" : false)}
										onCheckedChange={(checked) => toggleSelectAllVisible(checked === true)}
										aria-label={t("table.selectAll")}
										data-testid="vk-select-all-checkbox"
									/>
								</TableHead>
								<TableHead className="w-[250px]">
									<SortableHeader column="name" label={t("table.name")} />
								</TableHead>
								<TableHead className="w-[440px]">{t("table.key")}</TableHead>
								<TableHead className="w-[200px]">
									<SortableHeader column="budget_spent" label={t("table.budget")} />
								</TableHead>
								<TableHead className="w-[200px]">{t("table.rateLimits")}</TableHead>
								<TableHead className="w-[120px]">
									<SortableHeader column="status" label={t("table.status")} />
								</TableHead>
								<TableHead className={`bg-muted sticky right-0 z-30 w-[104px] text-center ${PIN_SHADOW_RIGHT}`}>
									{t("common:common.actions")}
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{virtualKeys.length === 0 ? (
								<TableRow>
									<TableCell colSpan={7} className="h-24 text-center">
										<span className="text-muted-foreground text-sm">{t("table.noMatching")}</span>
									</TableCell>
								</TableRow>
							) : (
								virtualKeys.map((vk) => {
									const isRevealed = revealedKeys.has(vk.id);
									const isExpired = !!vk.expires_at && Date.now() >= new Date(vk.expires_at).getTime();
									const showExpiredBadge = vk.is_active && isExpired;

									return (
										<TableRow
											key={vk.id}
											data-testid={`vk-row-${vk.name}`}
											className="group hover:bg-muted/50 cursor-pointer transition-colors"
											onClick={() => handleRowClick(vk)}
										>
											<TableCell onClick={(e) => e.stopPropagation()}>
												<Checkbox
													checked={selectedIds.has(vk.id)}
													onCheckedChange={(checked) => toggleSelectVirtualKey(vk.id, checked === true)}
													aria-label={t("table.selectOne", { name: vk.name })}
													data-testid={`vk-select-checkbox-${vk.name}`}
												/>
											</TableCell>
											<TableCell className="max-w-[200px]">
												<div className="truncate font-medium">{vk.name}</div>
											</TableCell>
											<TableCell onClick={(e) => e.stopPropagation()}>
												<div className="flex items-center gap-2">
													<code className="cursor-default py-1 font-mono text-sm" data-testid="vk-key-value">
														{maskKey(vk.value, isRevealed)}
													</code>
													<div className="flex items-center">
														<Button
															variant="ghost"
															size="sm"
															onClick={() => toggleKeyVisibility(vk.id)}
															data-testid={`vk-visibility-btn-${vk.name}`}
														>
															{isRevealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
														</Button>
														<Button
															variant="ghost"
															size="sm"
															onClick={() => copyToClipboard(vk.value)}
															data-testid={`vk-copy-btn-${vk.name}`}
														>
															<Copy className="h-4 w-4" />
														</Button>
													</div>
												</div>
											</TableCell>
											<TableCell>
												<VKBudgetCell vk={vk} />
											</TableCell>
											<TableCell>
												<VKRateLimitCell vk={vk} />
											</TableCell>
											<TableCell onClick={(e) => e.stopPropagation()}>
												{showExpiredBadge ? (
													<Badge variant="destructive" className="text-xs">
														Expired
													</Badge>
												) : (
													<VKActiveSwitch vk={vk} hasUpdateAccess={hasUpdateAccess} onToggle={handleToggleActive} />
												)}
											</TableCell>
											<TableCell
												className={`group-hover:bg-muted dark:bg-card dark:group-hover:bg-muted sticky right-0 z-20 bg-white text-right ${PIN_SHADOW_RIGHT}`}
												onClick={(e) => e.stopPropagation()}
											>
												<VKActionsMenu
													vk={vk}
													hasUpdateAccess={hasUpdateAccess}
													hasDeleteAccess={hasDeleteAccess}
													isDeleting={isDeleting}
													onEdit={handleEditVirtualKey}
													onDelete={handleDelete}
												/>
											</TableCell>
										</TableRow>
									);
								})
							)}
						</TableBody>
					</Table>
				</div>

				<Pagination offset={offset} limit={limit} totalCount={totalCount} onOffsetChange={onOffsetChange} dataTestIdPrefix="vk" />
			</div>
		</>
	);
}