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
import { Pagination } from "@/components/ui/pagination";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { PIN_SHADOW_RIGHT } from "@/components/table/columnPinning";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import { getProviderLabel } from "@/lib/constants/logs";
import { getErrorMessage } from "@/lib/store";
import { useDeleteRoutingRuleMutation, useUpdateRoutingRuleMutation } from "@/lib/store/apis/routingRulesApi";
import { RoutingRule, RoutingTarget } from "@/lib/types/routingRules";
import { getScopeLabel } from "@/lib/utils/labels";
import { getPriorityBadgeClass, truncateCELExpression } from "@/lib/utils/routingRules";
import { Edit, Eye, Search, Trash2 } from "lucide-react";
import { useState } from "react";
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

	const sortedRules = rules ? [...rules].sort((a, b) => a.priority - b.priority) : [];
	const ruleToDelete = sortedRules.find((r) => r.id === deleteRuleId);

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
						{sortedRules.length === 0 ? (
							<TableRow>
								<TableCell colSpan={7} className="h-24 text-center">
									<span className="text-muted-foreground text-sm">{t("rules.noMatchingRules")}</span>
								</TableCell>
							</TableRow>
						) : (
							sortedRules.map((rule) => (
								<TableRow
									key={rule.id}
									className="group hover:bg-muted/50 cursor-pointer transition-colors"
									onClick={() => onRowClick(rule)}
								>
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
										<div className={`inline-block rounded px-2.5 py-1 font-mono text-xs font-medium ${getPriorityBadgeClass()}`}>
											{rule.priority}
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