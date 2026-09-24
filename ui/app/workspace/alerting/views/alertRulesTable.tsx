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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";
import { Input } from "@/components/ui/input";
import { Pagination } from "@/components/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useDebouncedValue } from "@/hooks/useDebounce";
import {
	getErrorMessage,
	useCreateAlertRuleMutation,
	useDeleteAlertRuleMutation,
	useListAlertRulesQuery,
	useTestAlertRuleMutation,
	useUpdateAlertRuleMutation,
} from "@/lib/store";
import type { AlertRule, AlertRuleUpsertRequest, AlertRuleStatus } from "@/lib/store/apis/alertingApi";
import { Link } from "@tanstack/react-router";
import { BellRing, Loader2, MoreHorizontal, Pencil, Plus, RefreshCw, Send, Search, Trash2, X } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { AlertRuleSheet } from "./alertRuleSheet";

const PAGE_SIZE = 25;
const STATUS_OPTIONS: AlertRuleStatus[] = ["enabled", "disabled", "draft"];
const SCOPE_OPTIONS: AlertRule["scope_type"][] = ["global", "team", "customer", "virtual_key"];

function statusVariant(status: AlertRuleStatus) {
	switch (status) {
		case "enabled":
			return "default" as const;
		case "disabled":
			return "secondary" as const;
		case "draft":
			return "outline" as const;
	}
}

export default function AlertRulesTable() {
	const { t } = useTranslation("alerting");
	const [search, setSearch] = useState("");
	const [debouncedSearch] = useDebouncedValue(search, 300);
	const [statusFilter, setStatusFilter] = useState<"all" | AlertRuleStatus>("all");
	const [scopeFilter, setScopeFilter] = useState<"all" | AlertRule["scope_type"]>("all");
	const [offset, setOffset] = useState(0);
	const [sheetOpen, setSheetOpen] = useState(false);
	const [editingRule, setEditingRule] = useState<AlertRule | null>(null);
	const [pendingDelete, setPendingDelete] = useState<AlertRule | null>(null);

	const params = useMemo(
		() => ({
			limit: PAGE_SIZE,
			offset,
			search: debouncedSearch || undefined,
			status: statusFilter === "all" ? undefined : statusFilter,
			scope_type: scopeFilter === "all" ? undefined : scopeFilter,
		}),
		[offset, debouncedSearch, statusFilter, scopeFilter],
	);

	const { data, isLoading, isFetching, error, refetch } = useListAlertRulesQuery(params);
	const [createAlertRule, { isLoading: isCreating }] = useCreateAlertRuleMutation();
	const [updateAlertRule, { isLoading: isUpdating }] = useUpdateAlertRuleMutation();
	const [deleteAlertRule] = useDeleteAlertRuleMutation();
	const [testAlertRule, { isLoading: isTesting }] = useTestAlertRuleMutation();

	const rules = data?.rules ?? [];
	const total = data?.total ?? 0;

	const clearFilters = () => {
		setSearch("");
		setStatusFilter("all");
		setScopeFilter("all");
		setOffset(0);
	};
	const hasFilters = !!search || statusFilter !== "all" || scopeFilter !== "all";

	const onSubmit = async (body: AlertRuleUpsertRequest) => {
		try {
			if (editingRule) {
				await updateAlertRule({ id: editingRule.id, body }).unwrap();
				toast.success(t("ruleUpdated"));
			} else {
				await createAlertRule(body).unwrap();
				toast.success(t("ruleCreated"));
			}
			setSheetOpen(false);
			setEditingRule(null);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onDelete = async (rule: AlertRule) => {
		try {
			await deleteAlertRule(rule.id).unwrap();
			toast.success(t("ruleDeleted"));
			setPendingDelete(null);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onTest = async (rule: AlertRule) => {
		try {
			await testAlertRule(rule.id).unwrap();
			toast.success(t("ruleTestSent", { name: rule.name }));
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	return (
		<div className="flex h-full flex-col gap-4 p-6" data-testid="alerting-page">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold">
						<BellRing className="h-6 w-6" />
						{t("title")}
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("subtitle")}</p>
				</div>
				<div className="flex items-center gap-2">
					<Button variant="outline" size="sm" onClick={() => refetch()} isLoading={isFetching} data-testid="alerting-refresh">
						<RefreshCw className="mr-1 h-3 w-3" /> {t("refresh")}
					</Button>
					<Button
						size="sm"
						onClick={() => {
							setEditingRule(null);
							setSheetOpen(true);
						}}
						data-testid="alerting-create"
					>
						<Plus className="mr-1 h-3 w-3" /> {t("createRule")}
					</Button>
				</div>
			</header>

			<div className="flex flex-wrap items-center gap-3">
				<div className="relative max-w-sm flex-1">
					<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
					<Input
						type="search"
						placeholder={t("searchPlaceholder")}
						value={search}
						onChange={(e) => {
							setSearch(e.target.value);
							setOffset(0);
						}}
						className="pl-9"
						data-testid="alerting-search"
					/>
				</div>
				<Select
					value={statusFilter}
					onValueChange={(v) => {
						setStatusFilter(v as typeof statusFilter);
						setOffset(0);
					}}
				>
					<SelectTrigger className="w-40" data-testid="alerting-status-filter">
						<SelectValue placeholder={t("statusFilter")} />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="all">{t("statusAll")}</SelectItem>
						{STATUS_OPTIONS.map((s) => (
							<SelectItem key={s} value={s}>
								{t(`status_${s}`)}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				<Select
					value={scopeFilter}
					onValueChange={(v) => {
						setScopeFilter(v as typeof scopeFilter);
						setOffset(0);
					}}
				>
					<SelectTrigger className="w-40" data-testid="alerting-scope-filter">
						<SelectValue placeholder={t("scopeFilter")} />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="all">{t("scopeAll")}</SelectItem>
						{SCOPE_OPTIONS.map((s) => (
							<SelectItem key={s} value={s}>
								{t(`scope_${s}`)}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				{hasFilters && (
					<Button variant="ghost" size="sm" onClick={clearFilters} data-testid="alerting-clear-filters">
						<X className="mr-1 h-3 w-3" /> {t("clearFilters")}
					</Button>
				)}
				<div className="text-muted-foreground ml-auto text-sm" data-testid="alerting-total-count">
					{t("totalCount", { count: total })}
				</div>
			</div>

			<div className="border-border bg-card rounded-sm border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>{t("col_name")}</TableHead>
							<TableHead>{t("col_scope")}</TableHead>
							<TableHead>{t("col_metric")}</TableHead>
							<TableHead>{t("col_threshold")}</TableHead>
							<TableHead>{t("col_status")}</TableHead>
							<TableHead>{t("col_channels")}</TableHead>
							<TableHead className="w-12 text-right">{t("col_actions")}</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading ? (
							<TableRow>
								<TableCell colSpan={7} className="text-muted-foreground py-8 text-center text-sm">
									<Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t("loading")}
								</TableCell>
							</TableRow>
						) : error ? (
							<TableRow>
								<TableCell colSpan={7} className="text-destructive py-8 text-center text-sm">
									{getErrorMessage(error)}
								</TableCell>
							</TableRow>
						) : rules.length === 0 ? (
							<TableRow>
								<TableCell colSpan={7} className="text-muted-foreground py-8 text-center text-sm">
									{t("empty")}
								</TableCell>
							</TableRow>
						) : (
							rules.map((rule) => (
								<TableRow key={rule.id} data-testid={`alerting-row-${rule.id}`}>
									<TableCell>
										<Link to="/workspace/alerting/$ruleId" params={{ ruleId: rule.id }} className="font-medium hover:underline">
											{rule.name}
										</Link>
									</TableCell>
									<TableCell>
										<Badge variant="outline">{t(`scope_${rule.scope_type}`)}</Badge>
										{rule.scope_id && <span className="text-muted-foreground ml-1 font-mono text-xs">{rule.scope_id.slice(0, 8)}</span>}
									</TableCell>
									<TableCell className="font-mono text-xs">{t(`metric_${rule.metric}`)}</TableCell>
									<TableCell className="font-mono text-xs">
										{rule.comparison} {rule.threshold}
									</TableCell>
									<TableCell>
										<Badge variant={statusVariant(rule.status)}>{t(`status_${rule.status}`)}</Badge>
									</TableCell>
									<TableCell className="text-muted-foreground text-xs">
										{rule.channels.length} {t("channelsCount", { count: rule.channels.length })}
									</TableCell>
									<TableCell className="text-right">
										<DropdownMenu>
											<DropdownMenuTrigger asChild>
												<Button variant="ghost" size="icon" data-testid={`alerting-actions-${rule.id}`}>
													<MoreHorizontal className="h-4 w-4" />
												</Button>
											</DropdownMenuTrigger>
											<DropdownMenuContent align="end">
												<DropdownMenuItem onClick={() => onTest(rule)} disabled={isTesting} data-testid={`alerting-test-${rule.id}`}>
													<Send className="mr-2 h-4 w-4" /> {t("action_test")}
												</DropdownMenuItem>
												<DropdownMenuItem
													onClick={() => {
														setEditingRule(rule);
														setSheetOpen(true);
													}}
													data-testid={`alerting-edit-${rule.id}`}
												>
													<Pencil className="mr-2 h-4 w-4" /> {t("action_edit")}
												</DropdownMenuItem>
												<DropdownMenuItem
													onClick={() => setPendingDelete(rule)}
													className="text-destructive focus:text-destructive"
													data-testid={`alerting-delete-${rule.id}`}
												>
													<Trash2 className="mr-2 h-4 w-4" /> {t("action_delete")}
												</DropdownMenuItem>
											</DropdownMenuContent>
										</DropdownMenu>
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			</div>

			{rules.length > 0 && (
				<Pagination offset={offset} limit={PAGE_SIZE} totalCount={total} onOffsetChange={(next) => setOffset(next)} showItemsInfo />
			)}

			{!isLoading && rules.length === 0 && !hasFilters && (
				<Card>
					<CardHeader>
						<CardTitle className="text-base">{t("emptyCardTitle")}</CardTitle>
					</CardHeader>
					<CardContent className="text-muted-foreground text-sm">{t("emptyCardDesc")}</CardContent>
				</Card>
			)}

			<AlertRuleSheet
				open={sheetOpen}
				onOpenChange={(open) => {
					setSheetOpen(open);
					if (!open) setEditingRule(null);
				}}
				rule={editingRule}
				onSubmit={onSubmit}
				isSubmitting={isCreating || isUpdating}
			/>

			<AlertDialog open={!!pendingDelete} onOpenChange={(open) => !open && setPendingDelete(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("deleteConfirmTitle")}</AlertDialogTitle>
						<AlertDialogDescription>{t("deleteConfirmDesc", { name: pendingDelete?.name })}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>{t("cancel")}</AlertDialogCancel>
						<AlertDialogAction onClick={() => pendingDelete && onDelete(pendingDelete)} data-testid="alerting-delete-confirm">
							{t("confirmDelete")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}