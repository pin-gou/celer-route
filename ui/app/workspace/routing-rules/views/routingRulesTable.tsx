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
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Pagination } from "@/components/ui/pagination";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { PIN_SHADOW_RIGHT } from "@/components/table/columnPinning";
import { DragDropProvider } from "@dnd-kit/react";
import type { DragEndEvent, DragOverEvent } from "@dnd-kit/react";
import { useSortable } from "@dnd-kit/react/sortable";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import { getProviderLabel } from "@/lib/constants/logs";
import { getErrorMessage } from "@/lib/store";
import {
	useDeleteRoutingRuleMutation,
	useReorderRoutingRulesMutation,
	useUpdateRoutingRuleMutation,
} from "@/lib/store/apis/routingRulesApi";
import { RoutingRule, RoutingTarget } from "@/lib/types/routingRules";
import { getScopeLabel } from "@/lib/utils/labels";
import { getPriorityBadgeClass, truncateCELExpression } from "@/lib/utils/routingRules";
import { cn } from "@/lib/utils";
import { ArrowDown, ArrowUp, Edit, Eye, GripVertical, Search, Trash2 } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

interface RoutingRulesTableProps {
	rules: RoutingRule[] | undefined;
	totalCount: number;
	isLoading: boolean;
	onEdit: (rule: RoutingRule) => void;
	onRowClick: (rule: RoutingRule) => void;
	canDelete?: boolean;
	canUpdate?: boolean;
	search: string;
	onSearchChange: (value: string) => void;
	offset: number;
	limit: number;
	onOffsetChange: (offset: number) => void;
}

export function RoutingRulesTable({
	rules,
	totalCount,
	isLoading,
	onEdit,
	onRowClick,
	canDelete = false,
	canUpdate = false,
	search,
	onSearchChange,
	offset,
	limit,
	onOffsetChange,
}: RoutingRulesTableProps) {
	const { t } = useTranslation("routing");
	const [deleteRuleId, setDeleteRuleId] = useState<string | null>(null);
	const [deleteRoutingRule, { isLoading: isDeleting }] = useDeleteRoutingRuleMutation();
	const [updateRoutingRule] = useUpdateRoutingRuleMutation();
	const [reorderRoutingRules] = useReorderRoutingRulesMutation();

	const handleDelete = async () => {
		if (!canDelete || !deleteRuleId) return;

		try {
			await deleteRoutingRule(deleteRuleId).unwrap();
			toast.success(t("toast.ruleDeleted"));
			setDeleteRuleId(null);
		} catch (error: unknown) {
			toast.error(getErrorMessage(error));
		}
	};

	const sortedRules = useMemo(() => (rules ? [...rules].sort((a, b) => a.priority - b.priority) : []), [rules]);
	const ruleToDelete = sortedRules.find((r) => r.id === deleteRuleId);

	const [orderedRules, setOrderedRules] = useState<RoutingRule[]>(sortedRules);
	const orderedRef = useRef(orderedRules);
	const isDraggingRef = useRef(false);
	const originalRef = useRef<RoutingRule[]>([]);

	// Re-sync local display order from the server-sorted rules whenever the
	// underlying data changes, unless a drag is in progress (live reorder owns
	// the list until the drop persists).
	useEffect(() => {
		if (isDraggingRef.current) return;
		orderedRef.current = sortedRules;
		setOrderedRules(sortedRules);
	}, [sortedRules]);

	const handleDragOver = useCallback((event: Parameters<DragOverEvent>[0]) => {
		const { source, target } = event.operation;
		if (!source || !target || source.id === target.id) return;

		const current = orderedRef.current;
		const sourceIndex = current.findIndex((rule) => rule.id === String(source.id));
		const targetIndex = current.findIndex((rule) => rule.id === String(target.id));
		if (sourceIndex === -1 || targetIndex === -1 || sourceIndex === targetIndex) return;

		const next = [...current];
		const [moved] = next.splice(sourceIndex, 1);
		next.splice(targetIndex, 0, moved);
		orderedRef.current = next;
		setOrderedRules(next);
	}, []);

	const handleDragStart = useCallback(() => {
		isDraggingRef.current = true;
		originalRef.current = [...orderedRef.current];
	}, []);

	const applyPriorityShift = useCallback(
		(original: RoutingRule[], final: RoutingRule[], movedIdStr: string) => {
			const from = original.findIndex((rule) => rule.id === movedIdStr);
			const to = final.findIndex((rule) => rule.id === movedIdStr);
			if (from === -1 || to === -1 || from === to) return;

			const start = Math.min(from, to);
			const end = Math.max(from, to);
			// Original priorities of the block are already ascending (server-sorted).
			const blockPriorities = original.slice(start, end + 1).map((rule) => rule.priority);
			const updates: { id: string; priority: number }[] = [];

			const next = [...final];
			for (let i = start; i <= end; i++) {
				const newPriority = blockPriorities[i - start];
				if (final[i].priority !== newPriority) {
					next[i] = { ...final[i], priority: newPriority };
					updates.push({ id: final[i].id, priority: newPriority });
				}
			}

			setOrderedRules(next);
			orderedRef.current = next;

			if (updates.length === 0) return;

			reorderRoutingRules({ rules: updates })
				.unwrap()
				.then(() => {
					toast.success(t("rules.priorityReordered"));
				})
				.catch((err) => {
					toast.error(t("rules.failedToReorder"), { description: getErrorMessage(err) });
					// Roll back to the server order on failure.
					setOrderedRules(sortedRules);
					orderedRef.current = sortedRules;
				});
		},
		[reorderRoutingRules, sortedRules, t],
	);

	const handleDragEnd = useCallback(
		(event: Parameters<DragEndEvent>[0]) => {
			isDraggingRef.current = false;
			const original = originalRef.current;
			const final = orderedRef.current;
			if (original.length === 0 || final.length === 0) return;

			const movedId = event.operation.source?.id;
			if (!movedId) return;
			const movedIdStr = String(movedId);

			// A canceled drag (e.g. Escape) must not persist; restore the original order.
			if (event.canceled) {
				setOrderedRules(original);
				orderedRef.current = original;
				return;
			}

			applyPriorityShift(original, final, movedIdStr);
		},
		[applyPriorityShift],
	);

	const handleMoveRule = useCallback(
		(ruleId: string, direction: "up" | "down") => {
			const original = orderedRef.current;
			const from = original.findIndex((rule) => rule.id === ruleId);
			if (from === -1) return;
			const to = direction === "up" ? from - 1 : from + 1;
			if (to < 0 || to >= original.length) return;

			const final = [...original];
			const [moved] = final.splice(from, 1);
			final.splice(to, 0, moved);
			applyPriorityShift(original, final, ruleId);
		},
		[applyPriorityShift],
	);

	const renderRuleCells = (rule: RoutingRule, index: number, total: number) => (
		<>
			<TableCell className="font-medium">
				<div className="flex flex-col gap-1">
					<span className="max-w-xs truncate">{rule.name}</span>
					{rule.description && (
						<span data-testid="routing-rule-description" className="text-muted-foreground max-w-xs truncate text-xs">
							{rule.description}
						</span>
					)}
				</div>
			</TableCell>
			<TableCell>
				<TargetsSummary targets={rule.targets || []} />
			</TableCell>
			<TableCell>
				<Badge variant="secondary">{getScopeLabel(rule.scope)}</Badge>
			</TableCell>
			<TableCell className="text-right">
				<div className="inline-flex items-center justify-end gap-1.5">
					{canUpdate && (
						<>
							<RuleMoveButton ruleId={rule.id} direction="up" disabled={index === 0} onMove={handleMoveRule} />
							<RuleDragHandle ruleId={rule.id} />
							<RuleMoveButton ruleId={rule.id} direction="down" disabled={index === total - 1} onMove={handleMoveRule} />
						</>
					)}
					<div className={`inline-block rounded px-2.5 py-1 font-mono text-xs font-medium ${getPriorityBadgeClass()}`}>{rule.priority}</div>
				</div>
			</TableCell>
			<TableCell>
				<span className="text-muted-foreground block max-w-xs truncate font-mono text-xs" title={rule.cel_expression}>
					{truncateCELExpression(rule.cel_expression)}
				</span>
			</TableCell>
			<TableCell onClick={(e) => e.stopPropagation()}>
				<Switch
					data-testid={`routing-rule-enabled-${rule.id}-switch`}
					checked={rule.enabled ?? true}
					size="md"
					disabled={!canUpdate}
					onAsyncCheckedChange={async (checked) => {
						await updateRoutingRule({
							id: rule.id,
							data: { enabled: checked },
						})
							.unwrap()
							.then(() => {
								toast.success(checked ? t("rules.ruleEnabled") : t("rules.ruleDisabled"));
							})
							.catch((err) => {
								toast.error(t("rules.failedToUpdateRule"), {
									description: getErrorMessage(err),
								});
							});
					}}
				/>
			</TableCell>
			<TableCell
				className={`group-hover:bg-muted dark:bg-card dark:group-hover:bg-muted sticky right-0 z-20 bg-white text-right ${PIN_SHADOW_RIGHT}`}
				onClick={(e) => e.stopPropagation()}
			>
				<div className="flex items-center justify-end gap-1">
					<Button
						variant="ghost"
						size="icon"
						className="h-8 w-8"
						onClick={() => onRowClick(rule)}
						data-testid={`routing-rule-view-${rule.id}-btn`}
						aria-label={t("rules.viewRule")}
					>
						<Eye className="h-4 w-4" />
					</Button>
					<Button
						variant="ghost"
						size="icon"
						className="h-8 w-8"
						disabled={!canUpdate}
						onClick={() => onEdit(rule)}
						data-testid={`routing-rule-edit-${rule.id}-btn`}
						aria-label={t("rules.editRule")}
					>
						<Edit className="h-4 w-4" />
					</Button>
					<Button
						variant="ghost"
						size="icon"
						className="h-8 w-8"
						disabled={!canDelete}
						onClick={() => setDeleteRuleId(rule.id)}
						data-testid={`routing-rule-delete-${rule.id}-btn`}
						aria-label={t("rules.deleteRule")}
					>
						<Trash2 className="h-4 w-4" />
					</Button>
				</div>
			</TableCell>
		</>
	);

	if (isLoading) {
		return (
			<div className="rounded-sm border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>{t("rules.name")}</TableHead>
							<TableHead>{t("rules.targets")}</TableHead>
							<TableHead>{t("rules.scope")}</TableHead>
							<TableHead className="text-right">{t("rules.priority")}</TableHead>
							<TableHead>{t("rules.expression")}</TableHead>
							<TableHead>{t("rules.status")}</TableHead>
							<TableHead className="text-right">{t("rules.actions")}</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{[...Array(5)].map((_, i) => (
							<TableRow key={i}>
								<TableCell colSpan={7} className="h-10">
									<div className="bg-muted h-2 w-32 animate-pulse rounded" />
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			</div>
		);
	}

	return (
		<>
			<div className="mb-4 flex items-center gap-3">
				<div className="relative max-w-sm flex-1">
					<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
					<Input
						aria-label={t("rules.searchAriaLabel")}
						placeholder={t("rules.searchByPlaceholder")}
						value={search}
						onChange={(e) => onSearchChange(e.target.value)}
						className="pl-9"
						data-testid="routing-rules-search-input"
					/>
				</div>
			</div>

			<div className="mb-2 overflow-hidden rounded-sm border">
				<Table containerClassName="h-full overflow-auto">
					<TableHeader className="bg-muted sticky top-0 z-10">
						<TableRow className="bg-muted/50">
							<TableHead className="font-semibold">{t("rules.name")}</TableHead>
							<TableHead className="font-semibold">{t("rules.targets")}</TableHead>
							<TableHead className="font-semibold">{t("rules.scope")}</TableHead>
							<TableHead className="text-right font-semibold">{t("rules.priority")}</TableHead>
							<TableHead className="font-semibold">{t("rules.expression")}</TableHead>
							<TableHead className="font-semibold">{t("rules.status")}</TableHead>
							<TableHead className={`bg-muted sticky right-0 z-30 w-[130px] text-center font-semibold ${PIN_SHADOW_RIGHT}`}>
								{t("rules.actions")}
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{orderedRules.length === 0 ? (
							<TableRow>
								<TableCell colSpan={7} className="h-24 text-center">
									<span className="text-muted-foreground text-sm">{t("rules.noMatchingRules")}</span>
								</TableCell>
							</TableRow>
						) : canUpdate ? (
							<DragDropProvider onDragStart={handleDragStart} onDragOver={handleDragOver} onDragEnd={handleDragEnd}>
								{orderedRules.map((rule, index) => (
									<SortableRuleRow key={rule.id} rule={rule} index={index} onRowClick={onRowClick}>
										{renderRuleCells(rule, index, orderedRules.length)}
									</SortableRuleRow>
								))}
							</DragDropProvider>
						) : (
							orderedRules.map((rule, index) => (
								<TableRow
									key={rule.id}
									className="group hover:bg-muted/50 cursor-pointer transition-colors"
									onClick={() => onRowClick(rule)}
								>
									{renderRuleCells(rule, index, orderedRules.length)}
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			</div>

			<Pagination offset={offset} limit={limit} totalCount={totalCount} onOffsetChange={onOffsetChange} dataTestIdPrefix="routing-rules" />

			<AlertDialog open={!!deleteRuleId} onOpenChange={(open) => !open && setDeleteRuleId(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("rules.deleteRuleTitle")}</AlertDialogTitle>
						<AlertDialogDescription>{t("rules.deleteRuleConfirm", { name: ruleToDelete?.name })}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={isDeleting}>{t("rules.cancel")}</AlertDialogCancel>
						<AlertDialogAction onClick={handleDelete} disabled={isDeleting} className="bg-destructive hover:bg-destructive/90">
							{isDeleting ? t("rules.deleting") : t("rules.delete")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	);
}

const RuleRowDragHandleContext = createContext<{ handleRef: ((node: HTMLElement | null) => void) | null }>({ handleRef: null });

function SortableRuleRow({
	rule,
	index,
	onRowClick,
	children,
}: {
	rule: RoutingRule;
	index: number;
	onRowClick: (rule: RoutingRule) => void;
	children: React.ReactNode;
}) {
	const { ref, isDragging, handleRef } = useSortable({ id: rule.id, index });

	return (
		<RuleRowDragHandleContext.Provider value={{ handleRef }}>
			<TableRow
				ref={ref}
				className={cn("group hover:bg-muted/50 cursor-pointer transition-colors", isDragging && "opacity-50")}
				onClick={() => onRowClick(rule)}
			>
				{children}
			</TableRow>
		</RuleRowDragHandleContext.Provider>
	);
}

function RuleDragHandle({ ruleId }: { ruleId: string }) {
	const { t } = useTranslation("routing");
	const { handleRef } = useContext(RuleRowDragHandleContext);

	return (
		<TooltipProvider delayDuration={150}>
			<Tooltip>
				<TooltipTrigger asChild>
					<div
						ref={handleRef}
						data-testid={`routing-rule-drag-${ruleId}-handle`}
						aria-label={t("rules.dragHandle")}
						className="text-muted-foreground hover:text-foreground cursor-ns-resize"
						onClick={(e) => e.stopPropagation()}
					>
						<GripVertical className="h-4 w-4" />
					</div>
				</TooltipTrigger>
				<TooltipContent side="top">{t("rules.dragHandle")}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
}

function RuleMoveButton({
	ruleId,
	direction,
	disabled,
	onMove,
}: {
	ruleId: string;
	direction: "up" | "down";
	disabled?: boolean;
	onMove: (ruleId: string, direction: "up" | "down") => void;
}) {
	const { t } = useTranslation("routing");
	const isUp = direction === "up";
	const label = isUp ? t("rules.moveUp") : t("rules.moveDown");
	const Icon = isUp ? ArrowUp : ArrowDown;

	// Reflect the action direction in the pointer: up arrow / down arrow.
	const cursorClass = isUp ? "cursor-n-resize" : "cursor-s-resize";

	return (
		<TooltipProvider delayDuration={150}>
			<Tooltip>
				<TooltipTrigger asChild>
					<button
						type="button"
						data-testid={`routing-rule-${direction}-${ruleId}-button`}
						aria-label={label}
						disabled={disabled}
						onClick={(e) => {
							e.stopPropagation();
							onMove(ruleId, direction);
						}}
						className={`text-muted-foreground hover:text-foreground disabled:text-muted-foreground/30 inline-flex disabled:cursor-not-allowed ${cursorClass}`}
					>
						<Icon className="h-4 w-4" />
					</button>
				</TooltipTrigger>
				<TooltipContent side="top">{label}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
}

function TargetsSummary({ targets }: { targets: RoutingTarget[] }) {
	const { t } = useTranslation("routing");

	if (!targets || targets.length === 0) {
		return <span className="text-muted-foreground text-sm">-</span>;
	}

	const first = targets[0];
	const label = [first.provider ? getProviderLabel(first.provider) : t("rules.anyProvider"), first.model || t("rules.anyModel")].join(
		" / ",
	);

	return (
		<div className="flex flex-col gap-1">
			<div className="flex items-center gap-1.5">
				{first.provider && <RenderProviderIcon provider={first.provider as ProviderIconType} size="sm" className="h-4 w-4 shrink-0" />}
				<span className="max-w-[160px] truncate text-sm">{label}</span>
			</div>
			{targets.length > 1 && <span className="text-muted-foreground text-xs">{t("rules.moreTargets", { count: targets.length - 1 })}</span>}
		</div>
	);
}