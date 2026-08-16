import { Button } from "@/components/ui/button";
import { ComboboxSelect } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

interface PaginationProps {
	offset: number;
	limit: number;
	totalCount: number;
	onOffsetChange: (offset: number) => void;
	onLimitChange?: (limit: number) => void;
	pageSizeOptions?: number[];
	showItemsInfo?: boolean;
	className?: string;
	dataTestIdPrefix?: string;
}

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

export function Pagination({
	offset,
	limit,
	totalCount,
	onOffsetChange,
	onLimitChange,
	pageSizeOptions = DEFAULT_PAGE_SIZE_OPTIONS,
	showItemsInfo = true,
	className = "",
	dataTestIdPrefix = "pagination",
}: PaginationProps) {
	const { t } = useTranslation("common");

	const totalPages = Math.max(1, Math.ceil(totalCount / limit));
	const currentPage = Math.floor(offset / limit) + 1;
	const isFirstPage = offset === 0;
	const isLastPage = totalCount === 0 || offset + limit >= totalCount;

	const startItem = totalCount > 0 ? offset + 1 : 0;
	const endItem = Math.min(offset + limit, totalCount);

	const [pageInput, setPageInput] = useState("");
	const [isEditing, setIsEditing] = useState(false);

	const handleGoToPage = useCallback(() => {
		const page = parseInt(pageInput, 10);
		if (!isNaN(page) && page >= 1 && page <= totalPages) {
			onOffsetChange((page - 1) * limit);
		}
		setIsEditing(false);
		setPageInput("");
	}, [pageInput, totalPages, limit, onOffsetChange]);

	const handlePageKeyDown = useCallback(
		(e: React.KeyboardEvent) => {
			if (e.key === "Enter") {
				handleGoToPage();
			} else if (e.key === "Escape") {
				setIsEditing(false);
				setPageInput("");
			}
		},
		[handleGoToPage],
	);

	const handlePageSizeChange = useCallback(
		(value: string | null) => {
			if (!value) return;
			const newLimit = parseInt(value, 10);
			if (!isNaN(newLimit) && onLimitChange) {
				const currentFirstItem = offset;
				onLimitChange(newLimit);
				onOffsetChange(Math.floor(currentFirstItem / newLimit) * newLimit);
			}
		},
		[offset, onLimitChange, onOffsetChange],
	);

	const pageSizeOptionItems = useMemo(
		() => pageSizeOptions.map((size) => ({ label: String(size), value: String(size) })),
		[pageSizeOptions],
	);

	return (
		<div className={`flex shrink-0 items-center justify-between text-xs ${className}`} data-testid={dataTestIdPrefix}>
			{showItemsInfo && totalCount > 0 && (
				<div className="text-muted-foreground flex items-center gap-2">
					{startItem.toLocaleString()}-{endItem.toLocaleString()} / {totalCount.toLocaleString()} {t("pagination.entries")}
				</div>
			)}
			{showItemsInfo && totalCount === 0 && <div />}

			<div className="flex items-center gap-2">
				{onLimitChange && (
					<div className="flex items-center gap-1.5">
						<span className="text-muted-foreground">{t("pagination.rowsPerPage")}</span>
						<ComboboxSelect
							options={pageSizeOptionItems}
							value={String(limit)}
							onValueChange={handlePageSizeChange}
							disableSearch
							hideClear
							className="h-7 w-fit gap-1 text-xs"
						/>
					</div>
				)}

				<div className="flex items-center gap-2">
					<Button
						variant="ghost"
						size="sm"
						onClick={() => onOffsetChange(0)}
						disabled={isFirstPage}
						aria-label={t("pagination.firstPage")}
						data-testid={`${dataTestIdPrefix}-first-btn`}
					>
						<ChevronsLeft className="size-3" />
					</Button>
					<Button
						variant="ghost"
						size="sm"
						onClick={() => onOffsetChange(Math.max(0, offset - limit))}
						disabled={isFirstPage}
						aria-label={t("pagination.previous")}
						data-testid={`${dataTestIdPrefix}-prev-btn`}
					>
						<ChevronLeft className="size-3" />
					</Button>

					<div className="flex items-center gap-1">
						{isEditing ? (
							<Input
								className="h-6 w-12 px-1 text-center text-xs"
								value={pageInput}
								onChange={(e) => setPageInput(e.target.value)}
								onBlur={handleGoToPage}
								onKeyDown={handlePageKeyDown}
								autoFocus
								aria-label={t("pagination.goToPage")}
							/>
						) : (
							<button
								type="button"
								className="hover:bg-muted cursor-pointer rounded px-1 py-0.5"
								onClick={() => {
									setPageInput(String(currentPage));
									setIsEditing(true);
								}}
								aria-label={t("pagination.goToPage")}
								data-testid={`${dataTestIdPrefix}-page-indicator`}
							>
								<span>{t("pagination.page")}</span>
								<span>{currentPage}</span>
								<span>{t("pagination.of")}</span>
								<span>{totalPages}</span>
								<span>{t("pagination.pageSuffix")}</span>
							</button>
						)}
					</div>

					<Button
						variant="ghost"
						size="sm"
						onClick={() => onOffsetChange(offset + limit)}
						disabled={isLastPage}
						aria-label={t("pagination.next")}
						data-testid={`${dataTestIdPrefix}-next-btn`}
					>
						<ChevronRight className="size-3" />
					</Button>
					<Button
						variant="ghost"
						size="sm"
						onClick={() => onOffsetChange((totalPages - 1) * limit)}
						disabled={isLastPage}
						aria-label={t("pagination.lastPage")}
						data-testid={`${dataTestIdPrefix}-last-btn`}
					>
						<ChevronsRight className="size-3" />
					</Button>
				</div>
			</div>
		</div>
	);
}