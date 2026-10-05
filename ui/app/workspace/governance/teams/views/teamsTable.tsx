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
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdownMenu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Pagination } from "@/components/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useDebouncedValue } from "@/hooks/useDebounce";
import {
	getErrorMessage,
	useCreateTeamMutation,
	useDeleteTeamMutation,
	useGetCustomersQuery,
	useGetTeamsQuery,
	useUpdateTeamMutation,
} from "@/lib/store";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import type { Team } from "@/lib/types/governance";
import { Link } from "@tanstack/react-router";
import { Loader2, MoreHorizontal, Pencil, Plus, RefreshCw, Search, Trash2, Users } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const PAGE_SIZE = 25;

function customerName(team: Team): string {
	return team.customer?.name ?? team.customer_id ?? "—";
}

// Shared create/edit form. Customer assignment is optional: the backend
// treats customer_id "" on update as "clear the customer" (governance.go
// updateTeam), so the empty Select value maps to omitting it on create
// and clearing it on edit.
function TeamFormDialog({
	open,
	onOpenChange,
	team,
	onSubmit,
	isSubmitting,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	team: Team | null;
	onSubmit: (data: { name: string; customer_id: string }) => void;
	isSubmitting: boolean;
}) {
	const { t } = useTranslation("governance-ui");
	const [name, setName] = useState("");
	const [customerId, setCustomerId] = useState("");

	useEffect(() => {
		if (open) {
			setName(team?.name ?? "");
			setCustomerId(team?.customer_id ?? "");
		}
	}, [open, team]);

	const { data: customersData } = useGetCustomersQuery({ limit: 100 }, { skip: !open });
	const customers = customersData?.customers ?? [];

	const canSubmit = name.trim().length > 0 && !isSubmitting;

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent data-testid="teams-form-dialog">
				<DialogHeader>
					<DialogTitle>{team ? t("teams.editTitle") : t("teams.createTitle")}</DialogTitle>
					<DialogDescription>{team ? t("teams.editDesc") : t("teams.createDesc")}</DialogDescription>
				</DialogHeader>
				<div className="space-y-4">
					<div className="space-y-2">
						<Label htmlFor="teams-form-name">{t("teams.nameLabel")}</Label>
						<Input
							id="teams-form-name"
							value={name}
							onChange={(e) => setName(e.target.value)}
							placeholder={t("teams.namePlaceholder")}
							data-testid="teams-form-name"
							autoFocus
						/>
					</div>
					<div className="space-y-2">
						<Label htmlFor="teams-form-customer">{t("teams.customerLabel")}</Label>
						<Select value={customerId} onValueChange={setCustomerId}>
							<SelectTrigger className="w-full" data-testid="teams-form-customer">
								<SelectValue placeholder={t("teams.noCustomer")} />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="">{t("teams.noCustomer")}</SelectItem>
								{customers.map((c) => (
									<SelectItem key={c.id} value={c.id}>
										{c.name}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
				</div>
				<DialogFooter>
					<Button type="button" variant="outline" onClick={() => onOpenChange(false)} data-testid="teams-form-cancel">
						{t("teams.cancel")}
					</Button>
					<Button
						type="button"
						disabled={!canSubmit}
						isLoading={isSubmitting}
						onClick={() => onSubmit({ name: name.trim(), customer_id: customerId })}
						data-testid="teams-form-submit"
					>
						{t("teams.save")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

export default function TeamsTable() {
	const { t } = useTranslation("governance-ui");
	const hasViewAccess = useRbac(RbacResource.Governance, RbacOperation.View);
	const hasCreateAccess = useRbac(RbacResource.Governance, RbacOperation.Create);
	const hasUpdateAccess = useRbac(RbacResource.Governance, RbacOperation.Update);
	const hasDeleteAccess = useRbac(RbacResource.Governance, RbacOperation.Delete);

	const [search, setSearch] = useState("");
	const [debouncedSearch] = useDebouncedValue(search, 300);
	const [offset, setOffset] = useState(0);

	const [formOpen, setFormOpen] = useState(false);
	const [editingTeam, setEditingTeam] = useState<Team | null>(null);
	const [pendingDelete, setPendingDelete] = useState<Team | null>(null);

	const queryParams = useMemo(
		() => ({
			limit: PAGE_SIZE,
			offset,
			search: debouncedSearch || undefined,
		}),
		[offset, debouncedSearch],
	);

	const { data, isLoading, isFetching, error, refetch } = useGetTeamsQuery(queryParams, { skip: !hasViewAccess });

	const [createTeam, { isLoading: isCreating }] = useCreateTeamMutation();
	const [updateTeam, { isLoading: isUpdating }] = useUpdateTeamMutation();
	const [deleteTeam] = useDeleteTeamMutation();

	const teams = data?.teams ?? [];
	const total = data?.total_count ?? 0;

	const openCreate = () => {
		setEditingTeam(null);
		setFormOpen(true);
	};

	const openEdit = (team: Team) => {
		setEditingTeam(team);
		setFormOpen(true);
	};

	const onSubmit = async (form: { name: string; customer_id: string }) => {
		try {
			if (editingTeam) {
				await updateTeam({ teamId: editingTeam.id, data: { name: form.name, customer_id: form.customer_id } }).unwrap();
				toast.success(t("teams.updateSuccess"));
			} else {
				await createTeam({ name: form.name, customer_id: form.customer_id || undefined }).unwrap();
				toast.success(t("teams.createSuccess"));
			}
			setFormOpen(false);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onDelete = async (team: Team) => {
		try {
			await deleteTeam(team.id).unwrap();
			toast.success(t("teams.deleteSuccess"));
			setPendingDelete(null);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	if (!hasViewAccess) {
		return (
			<div className="text-muted-foreground p-8 text-center text-sm" data-testid="teams-no-permission">
				{t("users.noPermission")}
			</div>
		);
	}

	return (
		<div className="flex h-full flex-col gap-4 p-6" data-testid="teams-table-page">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground text-2xl font-semibold">{t("teams.title")}</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("teams.subtitle")}</p>
				</div>
				<div className="text-muted-foreground text-sm" data-testid="teams-total-count">
					{t("teams.totalCount", { count: total })}
				</div>
			</header>

			<div className="flex flex-wrap items-center gap-3">
				<div className="relative max-w-sm flex-1">
					<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
					<Input
						type="search"
						placeholder={t("teams.searchPlaceholder")}
						value={search}
						onChange={(e) => {
							setSearch(e.target.value);
							setOffset(0);
						}}
						className="pl-9"
						data-testid="teams-search"
					/>
				</div>
				<Button variant="outline" size="sm" onClick={() => refetch()} isLoading={isFetching} data-testid="teams-refresh">
					<RefreshCw className="mr-1 h-3 w-3" /> {t("teams.refresh")}
				</Button>
				<Button size="sm" disabled={!hasCreateAccess} onClick={openCreate} data-testid="teams-create">
					<Plus className="mr-1 h-4 w-4" /> {t("teams.create")}
				</Button>
			</div>

			<div className="border-border bg-card rounded-sm border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>{t("teams.col_name")}</TableHead>
							<TableHead>{t("teams.col_id")}</TableHead>
							<TableHead>{t("teams.col_customer")}</TableHead>
							<TableHead>{t("teams.col_vks")}</TableHead>
							<TableHead className="w-12 text-right">{t("teams.col_actions")}</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading ? (
							<TableRow>
								<TableCell colSpan={5} className="text-muted-foreground py-8 text-center text-sm">
									<Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t("teams.loading")}
								</TableCell>
							</TableRow>
						) : error ? (
							<TableRow>
								<TableCell colSpan={5} className="text-destructive py-8 text-center text-sm">
									{getErrorMessage(error)}
								</TableCell>
							</TableRow>
						) : teams.length === 0 ? (
							<TableRow>
								<TableCell colSpan={5} className="text-muted-foreground py-8 text-center text-sm">
									{t("teams.empty")}
								</TableCell>
							</TableRow>
						) : (
							teams.map((team) => (
								<TableRow key={team.id} data-testid={`teams-row-${team.id}`}>
									<TableCell>
										<Link
											to="/workspace/governance/teams/$teamId"
											params={{ teamId: team.id }}
											className="hover:underline"
											data-testid={`teams-row-link-${team.id}`}
										>
											{team.name}
										</Link>
									</TableCell>
									<TableCell className="text-muted-foreground font-mono text-xs">{team.id}</TableCell>
									<TableCell>{customerName(team)}</TableCell>
									<TableCell data-testid={`teams-vk-count-${team.id}`}>{team.virtual_key_count ?? 0}</TableCell>
									<TableCell className="text-right">
										<DropdownMenu>
											<DropdownMenuTrigger asChild>
												<Button variant="ghost" size="icon" data-testid={`teams-actions-${team.id}`}>
													<MoreHorizontal className="h-4 w-4" />
												</Button>
											</DropdownMenuTrigger>
											<DropdownMenuContent align="end">
												<DropdownMenuItem asChild>
													<Link
														to="/workspace/governance/teams/$teamId/members"
														params={{ teamId: team.id }}
														data-testid={`teams-action-members-${team.id}`}
													>
														<Users className="mr-2 h-4 w-4" /> {t("teams.manageMembers")}
													</Link>
												</DropdownMenuItem>
												<DropdownMenuItem
													disabled={!hasUpdateAccess}
													onClick={() => openEdit(team)}
													data-testid={`teams-action-edit-${team.id}`}
												>
													<Pencil className="mr-2 h-4 w-4" /> {t("teams.action_edit")}
												</DropdownMenuItem>
												<DropdownMenuSeparator />
												<DropdownMenuItem
													disabled={!hasDeleteAccess}
													onClick={() => setPendingDelete(team)}
													className="text-destructive focus:text-destructive"
													data-testid={`teams-action-delete-${team.id}`}
												>
													<Trash2 className="mr-2 h-4 w-4" /> {t("teams.action_delete")}
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

			{teams.length > 0 && (
				<Pagination offset={offset} limit={PAGE_SIZE} totalCount={total} onOffsetChange={(next) => setOffset(next)} showItemsInfo />
			)}

			<TeamFormDialog
				open={formOpen}
				onOpenChange={setFormOpen}
				team={editingTeam}
				onSubmit={onSubmit}
				isSubmitting={isCreating || isUpdating}
			/>

			<AlertDialog open={!!pendingDelete} onOpenChange={(open) => !open && setPendingDelete(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("teams.deleteConfirmTitle")}</AlertDialogTitle>
						<AlertDialogDescription>{t("teams.deleteConfirmDesc", { name: pendingDelete?.name })}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>{t("teams.cancel")}</AlertDialogCancel>
						<AlertDialogAction onClick={() => pendingDelete && onDelete(pendingDelete)} data-testid="teams-delete-confirm">
							{t("teams.confirmDelete")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}