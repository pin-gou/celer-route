import {
	buildPinStyle,
	type ColumnConfigEntry,
	DraggableColumnHeader,
	PIN_SHADOW_LEFT,
	PIN_SHADOW_RIGHT,
	useHeaderCellRefs,
	usePinOffsets,
} from "@/components/table";
import { Button } from "@/components/ui/button";
import { Table, TableBody } from "@/components/ui/table";
import { Pagination } from "@/components/ui/pagination";
import { useTablePageSizePreference } from "@/lib/hooks/useTablePageSizePreference";
import type { DisplayLogEntry, LogEntry, Pagination as PaginationType } from "@/lib/types/logs";
import { cn } from "@/lib/utils";
import type { ColumnOrderState, ColumnPinningState, TableMeta, VisibilityState } from "@tanstack/react-table";
import { ColumnDef, flexRender, getCoreRowModel, SortingState, useReactTable } from "@tanstack/react-table";
import { Loader2, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

// Local <td>/<tr> for the logs table. Skip the global <TableCell>'s default
// `data-slot="table-cell"` and the `[role=checkbox]` hooks (this table has no
// checkbox column), and skip `min-width`/`max-width` in style since
// tanstack-table already derives the column width from `size`.
function LogTableCell({ className, style, ...props }: React.ComponentProps<"td">) {
	return (
		<td
			className={cn(
				"px-4 py-1.5 align-middle whitespace-nowrap group-hover/table-row:bg-[#f7f7f7] dark:group-hover/table-row:bg-[#232327]",
				className,
			)}
			style={style}
			{...props}
		/>
	);
}

function LogTableRow({ className, ...props }: React.ComponentProps<"tr">) {
	return <tr className={cn("hover:bg-muted/50 dark:hover:bg-muted/75 border-b transition-colors", className)} {...props} />;
}

interface DataTableProps {
	columns: ColumnDef<LogEntry>[];
	data: LogEntry[];
	totalItems: number;
	pagination: PaginationType;
	onPaginationChange: (pagination: PaginationType) => void;
	onRowClick?: (log: LogEntry, columnId: string) => void;
	polling: boolean;
	loading?: boolean;
	onRefresh: () => void;
	/** Column config — computed by the parent via useColumnConfig */
	columnEntries: ColumnConfigEntry[];
	columnOrder: ColumnOrderState;
	columnVisibility: VisibilityState;
	columnPinning: ColumnPinningState;
	onToggleColumnVisibility: (id: string) => void;
	onTogglePin: (id: string, side: "left" | "right") => void;
	onReorderColumns: (entries: ColumnConfigEntry[]) => void;
	/** Table meta consumed by the expand column in grouped view */
	tableMeta?: TableMeta<LogEntry>;
}

export function LogsDataTable({
	columns,
	data,
	totalItems,
	pagination,
	onPaginationChange,
	onRowClick,
	polling,
	loading,
	onRefresh,
	columnEntries,
	columnOrder,
	columnVisibility,
	columnPinning,
	onToggleColumnVisibility,
	onTogglePin,
	onReorderColumns,
	tableMeta,
}: DataTableProps) {
	const { t } = useTranslation("logs");
	const [sorting, setSorting] = useState<SortingState>([{ id: pagination.sort_by, desc: pagination.order === "desc" }]);
	const [pageSizePref, setPageSizePref, pageSizeHydrated] = useTablePageSizePreference("bifrost.logs.pageSize");

	const fixedColumnIds = useMemo(() => new Set<string>(["expand", "actions"]), []);

	// Measure actual header cell widths for pixel-perfect pin offsets
	const { headerCellRefs, setHeaderCellRef } = useHeaderCellRefs();
	const pinOffsets = usePinOffsets(headerCellRefs, columnPinning);

	// Shadow on the edge of pinned groups
	const lastLeftPinId = columnPinning.left?.at(-1);
	const firstRightPinId = columnPinning.right?.at(0);

	// Handle native drag-and-drop reorder
	const handleColumnDrop = useCallback(
		(draggedId: string, targetId: string) => {
			const newEntries = [...columnEntries];
			const draggedIdx = newEntries.findIndex((e) => e.id === draggedId);
			const targetIdx = newEntries.findIndex((e) => e.id === targetId);
			if (draggedIdx === -1 || targetIdx === -1) return;
			const [moved] = newEntries.splice(draggedIdx, 1);
			newEntries.splice(targetIdx, 0, moved);
			onReorderColumns(newEntries);
		},
		[columnEntries, onReorderColumns],
	);

	// Refs to avoid stale closures in the page size effect
	const paginationRef = useRef(pagination);
	const onPaginationChangeRef = useRef(onPaginationChange);
	paginationRef.current = pagination;
	onPaginationChangeRef.current = onPaginationChange;

	// Apply the page-size preference as the `limit` query param. Wait until the
	// localStorage value has hydrated — writing the pre-hydration default would
	// clobber an explicit `limit` already present in the URL (nuqs clears the
	// default from the URL), causing the param to flip-flop across refreshes.
	useEffect(() => {
		if (!pageSizeHydrated) return;
		if (paginationRef.current.limit !== pageSizePref) {
			onPaginationChangeRef.current({
				...paginationRef.current,
				limit: pageSizePref,
				offset: 0,
			});
		}
	}, [pageSizePref, pageSizeHydrated]);

	const handleSortingChange = (updaterOrValue: SortingState | ((old: SortingState) => SortingState)) => {
		const newSorting = typeof updaterOrValue === "function" ? updaterOrValue(sorting) : updaterOrValue;
		setSorting(newSorting);
		if (newSorting.length > 0) {
			const { id, desc } = newSorting[0];
			onPaginationChange({
				...pagination,
				sort_by: id as "timestamp" | "latency" | "tokens" | "cost",
				order: desc ? "desc" : "asc",
			});
		}
	};

	const table = useReactTable({
		data,
		columns,
		getCoreRowModel: getCoreRowModel(),
		manualPagination: true,
		manualSorting: true,
		manualFiltering: true,
		pageCount: Math.ceil(totalItems / pagination.limit),
		state: {
			sorting,
			columnOrder,
			columnVisibility,
			columnPinning,
		},
		onSortingChange: handleSortingChange,
		meta: tableMeta,
	});

	return (
		<div className="flex h-full flex-col gap-2">
			<div className="min-h-0 flex-1 overflow-hidden rounded-sm border">
				<Table containerClassName="h-full overflow-auto">
					<thead className={cn("[&_tr]:border-b px-2 sticky top-0 z-10 bg-[#f9f9f9] dark:bg-[#27272a]")}>
						{table.getHeaderGroups().map((headerGroup) => (
							<tr
								key={headerGroup.id}
								className="hover:bg-muted/50 dark:hover:bg-muted/75 data-[state=selected]:bg-muted border-b transition-colors"
							>
								{headerGroup.headers.map((header) => (
									<DraggableColumnHeader
										key={header.id}
										header={header}
										isConfigurable={!fixedColumnIds.has(header.column.id)}
										pinStyle={buildPinStyle(header.column, pinOffsets)}
										pinnedHeaderClassName="bg-[#f9f9f9] dark:bg-[#27272a]"
										className={cn(
											header.column.id === lastLeftPinId && PIN_SHADOW_LEFT,
											header.column.id === firstRightPinId && PIN_SHADOW_RIGHT,
										)}
										onHide={onToggleColumnVisibility}
										onPin={onTogglePin}
										onDrop={handleColumnDrop}
										cellRef={setHeaderCellRef(header.column.id)}
									/>
								))}
							</tr>
						))}
					</thead>
					<TableBody>
						<LogTableRow className="hover:bg-transparent">
							<LogTableCell colSpan={columns.length} className="h-12 text-center">
								<div className="text-muted-foreground flex items-center justify-center gap-2 text-sm">
									{loading ? (
										<>
											<RefreshCw className="h-4 w-4 animate-spin" />
											{t("table.loadingLogs")}
										</>
									) : polling ? (
										<>
											<RefreshCw className="h-4 w-4 animate-spin" />
											{t("table.waitingForLogs")}
										</>
									) : (
										<Button
											type="button"
											onClick={onRefresh}
											data-testid="logs-table-refresh-btn"
											className="hover:text-foreground inline-flex items-center gap-1.5 transition-colors"
											variant={"ghost"}
										>
											{loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
											{t("table.refresh")}
										</Button>
									)}
								</div>
							</LogTableCell>
						</LogTableRow>
						{table.getRowModel().rows.length ? (
							table.getRowModel().rows.map((row) => (
								<LogTableRow
									key={row.id}
									className={cn(
										"group/table-row min-h-[40px] cursor-pointer",
										(row.original as DisplayLogEntry).__chainChild && "bg-muted/30 border-l-2 border-l-zinc-300 dark:border-l-zinc-600",
										(row.original as DisplayLogEntry).__processing && "bg-blue-50/30 dark:bg-blue-950/20 animate-pulse",
									)}
								>
									{row.getVisibleCells().map((cell) => {
										const pinned = cell.column.getIsPinned();
										const size = cell.column.getSize();
										return (
											<LogTableCell
												onClick={() => onRowClick?.(row.original, cell.column.id)}
												key={cell.id}
												style={{
													width: size,
													...buildPinStyle(cell.column, pinOffsets),
												}}
												className={cn(
													pinned && "bg-card",
													cell.column.id === lastLeftPinId && PIN_SHADOW_LEFT,
													cell.column.id === firstRightPinId && PIN_SHADOW_RIGHT,
												)}
											>
												{flexRender(cell.column.columnDef.cell, cell.getContext())}
											</LogTableCell>
										);
									})}
								</LogTableRow>
							))
						) : loading ? null : (
							<LogTableRow>
								<LogTableCell colSpan={columns.length} className="h-24 text-center">
									{t("table.noResults")}
								</LogTableCell>
							</LogTableRow>
						)}
					</TableBody>
				</Table>
			</div>

			<Pagination
				offset={pagination.offset}
				limit={pagination.limit}
				totalCount={totalItems}
				onOffsetChange={(newOffset) => onPaginationChange({ ...pagination, offset: newOffset })}
				onLimitChange={(newLimit) => {
					setPageSizePref(newLimit);
					onPaginationChange({ ...pagination, limit: newLimit, offset: 0 });
				}}
				pageSizeOptions={[10, 25, 50, 100, 200]}
				dataTestIdPrefix="logs"
			/>
		</div>
	);
}