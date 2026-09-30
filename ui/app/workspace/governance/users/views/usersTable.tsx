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
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdownMenu";
import { Input } from "@/components/ui/input";
import { Pagination } from "@/components/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import {
	getErrorMessage,
	useDeleteUserMutation,
	useDisableUserMutation,
	useDisableUserVksMutation,
	useListUsersQuery,
	useResetUserPasswordTokenMutation,
} from "@/lib/store";
import type { AdminUserView } from "@/lib/store/apis/usersApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useDebouncedValue } from "@/hooks/useDebounce";
import { Link } from "@tanstack/react-router";
import { Copy, KeyRound, Loader2, MoreHorizontal, RefreshCw, Search, ShieldOff, Trash2, X } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const PAGE_SIZE = 25;
const STATUS_OPTIONS: AdminUserView["status"][] = ["pending", "active", "disabled"];
const ROLE_OPTIONS: AdminUserView["role"][] = ["admin", "member"];

function statusVariant(status: AdminUserView["status"]) {
	switch (status) {
		case "active":
			return "default" as const;
		case "pending":
			return "secondary" as const;
		case "disabled":
			return "destructive" as const;
	}
}

export default function UsersTable() {
	const { t } = useTranslation("governance-ui");
	const hasViewAccess = useRbac(RbacResource.Users, RbacOperation.View);
	const hasUpdateAccess = useRbac(RbacResource.Users, RbacOperation.Update);
	const hasDeleteAccess = useRbac(RbacResource.Users, RbacOperation.Delete);
	const { copy } = useCopyToClipboard();

	const [search, setSearch] = useState("");
	const [debouncedSearch] = useDebouncedValue(search, 300);
	const [statusFilter, setStatusFilter] = useState<"all" | AdminUserView["status"]>("all");
	const [roleFilter, setRoleFilter] = useState<"all" | AdminUserView["role"]>("all");
	const [offset, setOffset] = useState(0);

	const [pendingDelete, setPendingDelete] = useState<AdminUserView | null>(null);
	const [pendingDisable, setPendingDisable] = useState<AdminUserView | null>(null);
	const [pendingReset, setPendingReset] = useState<AdminUserView | null>(null);
	const [resetToken, setResetToken] = useState<string | null>(null);

	const queryParams = useMemo(
		() => ({
			limit: PAGE_SIZE,
			offset,
			search: debouncedSearch || undefined,
			status: statusFilter === "all" ? undefined : statusFilter,
			role: roleFilter === "all" ? undefined : roleFilter,
		}),
		[offset, debouncedSearch, statusFilter, roleFilter],
	);

	const { data, isLoading, isFetching, error, refetch } = useListUsersQuery(queryParams, { skip: !hasViewAccess });

	const [disableUser] = useDisableUserMutation();
	const [disableUserVks] = useDisableUserVksMutation();
	const [deleteUser] = useDeleteUserMutation();
	const [resetUserPasswordToken, { isLoading: isResetting }] = useResetUserPasswordTokenMutation();

	const users = data?.users ?? [];
	const total = data?.total ?? 0;

	const onDisable = async (user: AdminUserView) => {
		try {
			await disableUser(user.id).unwrap();
			toast.success(t("users.disableSuccess", { email: user.email }));
			setPendingDisable(null);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onDisableVks = async (user: AdminUserView) => {
		try {
			const res = await disableUserVks(user.id).unwrap();
			toast.success(t("users.disableVksSuccess", { count: res.count, email: user.email }));
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onDelete = async (user: AdminUserView) => {
		try {
			await deleteUser(user.id).unwrap();
			toast.success(t("users.deleteSuccess", { email: user.email }));
			setPendingDelete(null);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onResetPasswordToken = async (user: AdminUserView) => {
		try {
			const res = await resetUserPasswordToken(user.id).unwrap();
			setResetToken(res.token);
			setPendingReset(user);
			toast.success(t("users.resetTokenIssued"));
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const clearAllFilters = () => {
		setSearch("");
		setStatusFilter("all");
		setRoleFilter("all");
		setOffset(0);
	};

	const hasFilters = !!search || statusFilter !== "all" || roleFilter !== "all";

	if (!hasViewAccess) {
		return (
			<div className="text-muted-foreground p-8 text-center text-sm" data-testid="users-no-permission">
				{t("users.noPermission")}
			</div>
		);
	}

	return (
		<div className="flex h-full flex-col gap-4 p-6" data-testid="users-table-page">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground text-2xl font-semibold">{t("users.title")}</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("users.subtitle")}</p>
				</div>
				<div className="text-muted-foreground text-sm" data-testid="users-total-count">
					{t("users.totalCount", { count: total })}
				</div>
			</header>

			<div className="flex flex-wrap items-center gap-3">
				<div className="relative max-w-sm flex-1">
					<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
					<Input
						type="search"
						placeholder={t("users.searchPlaceholder")}
						value={search}
						onChange={(e) => {
							setSearch(e.target.value);
							setOffset(0);
						}}
						className="pl-9"
						data-testid="users-search"
					/>
				</div>
				<Select
					value={statusFilter}
					onValueChange={(v) => {
						setStatusFilter(v as typeof statusFilter);
						setOffset(0);
					}}
				>
					<SelectTrigger className="w-40" data-testid="users-status-filter">
						<SelectValue placeholder={t("users.statusFilter")} />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="all">{t("users.statusAll")}</SelectItem>
						{STATUS_OPTIONS.map((s) => (
							<SelectItem key={s} value={s}>
								{t(`status_${s}`)}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				<Select
					value={roleFilter}
					onValueChange={(v) => {
						setRoleFilter(v as typeof roleFilter);
						setOffset(0);
					}}
				>
					<SelectTrigger className="w-40" data-testid="users-role-filter">
						<SelectValue placeholder={t("users.roleFilter")} />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="all">{t("users.roleAll")}</SelectItem>
						{ROLE_OPTIONS.map((r) => (
							<SelectItem key={r} value={r}>
								{t(`role_${r}`)}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				{hasFilters && (
					<Button variant="ghost" size="sm" onClick={clearAllFilters} data-testid="users-clear-filters">
						<X className="mr-1 h-3 w-3" /> {t("users.clearFilters")}
					</Button>
				)}
				<Button variant="outline" size="sm" onClick={() => refetch()} isLoading={isFetching} data-testid="users-refresh">
					<RefreshCw className="mr-1 h-3 w-3" /> {t("users.refresh")}
				</Button>
			</div>

			<div className="border-border bg-card rounded-sm border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>{t("users.col_email")}</TableHead>
							<TableHead>{t("users.col_display_name")}</TableHead>
							<TableHead>{t("users.col_role")}</TableHead>
							<TableHead>{t("users.col_status")}</TableHead>
							<TableHead>{t("users.col_last_login")}</TableHead>
							<TableHead className="w-12 text-right">{t("users.col_actions")}</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading ? (
							<TableRow>
								<TableCell colSpan={6} className="text-muted-foreground py-8 text-center text-sm">
									<Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t("users.loading")}
								</TableCell>
							</TableRow>
						) : error ? (
							<TableRow>
								<TableCell colSpan={6} className="text-destructive py-8 text-center text-sm">
									{getErrorMessage(error)}
								</TableCell>
							</TableRow>
						) : users.length === 0 ? (
							<TableRow>
								<TableCell colSpan={6} className="text-muted-foreground py-8 text-center text-sm">
									{t("users.empty")}
								</TableCell>
							</TableRow>
						) : (
							users.map((user) => (
								<TableRow key={user.id} data-testid={`users-row-${user.id}`}>
									<TableCell className="font-mono text-xs">
										<Link
											to="/workspace/governance/users/$userId"
											params={{ userId: user.id }}
											className="hover:underline"
											data-testid={`users-row-link-${user.id}`}
										>
											{user.email}
										</Link>
									</TableCell>
									<TableCell>
										<Link to="/workspace/governance/users/$userId" params={{ userId: user.id }} className="hover:underline">
											{user.display_name || <span className="text-muted-foreground">—</span>}
										</Link>
									</TableCell>
									<TableCell>
										<Badge variant="outline" data-testid={`users-role-${user.id}`}>
											{t(`role_${user.role}`)}
										</Badge>
									</TableCell>
									<TableCell>
										<Badge variant={statusVariant(user.status)} data-testid={`users-status-${user.id}`}>
											{t(`status_${user.status}`)}
										</Badge>
									</TableCell>
									<TableCell className="text-muted-foreground text-xs">
										{user.last_login_at ? new Date(user.last_login_at).toLocaleString() : t("users.never")}
									</TableCell>
									<TableCell className="text-right">
										<DropdownMenu>
											<DropdownMenuTrigger asChild>
												<Button variant="ghost" size="icon" data-testid={`users-actions-${user.id}`}>
													<MoreHorizontal className="h-4 w-4" />
												</Button>
											</DropdownMenuTrigger>
											<DropdownMenuContent align="end">
												<DropdownMenuItem
													disabled={!hasUpdateAccess || user.status === "disabled"}
													onClick={() => setPendingDisable(user)}
													data-testid={`users-action-disable-${user.id}`}
												>
													<ShieldOff className="mr-2 h-4 w-4" /> {t("users.action_disable")}
												</DropdownMenuItem>
												<DropdownMenuItem
													disabled={!hasUpdateAccess}
													onClick={() => onDisableVks(user)}
													data-testid={`users-action-disable-vks-${user.id}`}
												>
													<KeyRound className="mr-2 h-4 w-4" /> {t("users.action_disable_vks")}
												</DropdownMenuItem>
												<DropdownMenuItem
													disabled={!hasUpdateAccess}
													onClick={() => onResetPasswordToken(user)}
													data-testid={`users-action-reset-${user.id}`}
												>
													<RefreshCw className="mr-2 h-4 w-4" /> {t("users.action_reset_token")}
												</DropdownMenuItem>
												<DropdownMenuSeparator />
												<DropdownMenuItem
													disabled={!hasDeleteAccess}
													onClick={() => setPendingDelete(user)}
													className="text-destructive focus:text-destructive"
													data-testid={`users-action-delete-${user.id}`}
												>
													<Trash2 className="mr-2 h-4 w-4" /> {t("users.action_delete")}
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

			{users.length > 0 && (
				<Pagination offset={offset} limit={PAGE_SIZE} totalCount={total} onOffsetChange={(next) => setOffset(next)} showItemsInfo />
			)}

			<AlertDialog open={!!pendingDelete} onOpenChange={(open) => !open && setPendingDelete(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("users.deleteConfirmTitle")}</AlertDialogTitle>
						<AlertDialogDescription>{t("users.deleteConfirmDesc", { email: pendingDelete?.email })}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>{t("users.cancel")}</AlertDialogCancel>
						<AlertDialogAction onClick={() => pendingDelete && onDelete(pendingDelete)} data-testid="users-delete-confirm">
							{t("users.confirmDelete")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			<AlertDialog open={!!pendingDisable} onOpenChange={(open) => !open && setPendingDisable(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("users.disableConfirmTitle")}</AlertDialogTitle>
						<AlertDialogDescription>{t("users.disableConfirmDesc", { email: pendingDisable?.email })}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>{t("users.cancel")}</AlertDialogCancel>
						<AlertDialogAction onClick={() => pendingDisable && onDisable(pendingDisable)} data-testid="users-disable-confirm">
							{t("users.confirmDisable")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			<Dialog open={!!resetToken} onOpenChange={(open) => !open && setResetToken(null)}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t("users.resetTokenTitle", { email: pendingReset?.email })}</DialogTitle>
						<DialogDescription>{t("users.resetTokenDesc")}</DialogDescription>
					</DialogHeader>
					<div className="bg-muted flex items-center gap-2 rounded-sm p-3 font-mono text-xs">
						<code className="flex-1 break-all" data-testid="users-reset-token">
							{resetToken ?? (isResetting ? t("users.loading") : "")}
						</code>
						{resetToken && (
							<Button variant="ghost" size="icon" onClick={() => copy(resetToken)} data-testid="users-reset-token-copy">
								<Copy className="h-4 w-4" />
							</Button>
						)}
					</div>
					<DialogFooter>
						<Button onClick={() => setResetToken(null)} data-testid="users-reset-token-close">
							{t("users.close")}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}