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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Pagination } from "@/components/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useDebouncedValue } from "@/hooks/useDebounce";
import {
	getErrorMessage,
	useDeleteStandardPriceMutation,
	useListStandardPricesQuery,
	useSyncStandardPricesMutation,
	useUpsertStandardPriceMutation,
} from "@/lib/store";
import type { StandardPriceRow, StandardPriceUpsertRequest } from "@/lib/store/apis/reportsApi";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, Loader2, MoreHorizontal, Pencil, Plus, RefreshCw, Search, Trash2, X } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const PAGE_SIZE = 50;

export default function StandardPricesTable() {
	const { t } = useTranslation("reports");
	const [providerFilter, setProviderFilter] = useState("");
	const [debouncedProvider] = useDebouncedValue(providerFilter, 300);
	const [modelFilter, setModelFilter] = useState("");
	const [debouncedModel] = useDebouncedValue(modelFilter, 300);
	const [offset, setOffset] = useState(0);
	const [sheetOpen, setSheetOpen] = useState(false);
	const [editingRow, setEditingRow] = useState<StandardPriceRow | null>(null);
	const [pendingDelete, setPendingDelete] = useState<StandardPriceRow | null>(null);

	const params = useMemo(
		() => ({
			limit: PAGE_SIZE,
			offset,
			provider: debouncedProvider || undefined,
			model: debouncedModel || undefined,
		}),
		[offset, debouncedProvider, debouncedModel],
	);

	const { data: rows_data, isLoading, isFetching, error, refetch } = useListStandardPricesQuery(params);
	const [upsertStandardPrice, { isLoading: isUpserting }] = useUpsertStandardPriceMutation();
	const [deleteStandardPrice] = useDeleteStandardPriceMutation();
	const [syncStandardPrices, { isLoading: isSyncing }] = useSyncStandardPricesMutation();

	const rows: StandardPriceRow[] = rows_data?.rows ?? [];
	const total = rows_data?.total ?? 0;

	const clearFilters = () => {
		setProviderFilter("");
		setModelFilter("");
		setOffset(0);
	};
	const hasFilters = !!providerFilter || !!modelFilter;

	const onSubmit = async (body: StandardPriceUpsertRequest) => {
		try {
			await upsertStandardPrice(body).unwrap();
			toast.success(t("standardPrices.toast.saved"));
			setSheetOpen(false);
			setEditingRow(null);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onDelete = async (row: StandardPriceRow) => {
		try {
			await deleteStandardPrice(row.id).unwrap();
			toast.success(t("standardPrices.toast.deleted"));
			setPendingDelete(null);
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onSync = async () => {
		try {
			const res = await syncStandardPrices({ multiplier: 1.0 }).unwrap();
			toast.success(t("standardPrices.toast.synced", { count: res.created }));
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	return (
		<div className="flex h-full flex-col gap-4 p-6" data-testid="standard-prices-page">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div className="flex items-center gap-3">
					<Button asChild variant="ghost" size="icon">
						<Link to="/workspace/reports" data-testid="standard-prices-back">
							<ArrowLeft className="h-4 w-4" />
						</Link>
					</Button>
					<div>
						<h1 className="text-foreground text-2xl font-semibold">{t("standardPrices.title")}</h1>
						<p className="text-muted-foreground mt-1 text-sm">{t("standardPrices.subtitle")}</p>
					</div>
				</div>
				<div className="flex items-center gap-2">
					<Button variant="outline" size="sm" onClick={onSync} isLoading={isSyncing} data-testid="standard-prices-sync">
						<RefreshCw className="mr-1 h-3 w-3" /> {t("standardPrices.sync")}
					</Button>
					<Button variant="outline" size="sm" onClick={() => refetch()} isLoading={isFetching} data-testid="standard-prices-refresh">
						<RefreshCw className="mr-1 h-3 w-3" /> {t("refresh")}
					</Button>
					<Button
						size="sm"
						onClick={() => {
							setEditingRow(null);
							setSheetOpen(true);
						}}
						data-testid="standard-prices-create"
					>
						<Plus className="mr-1 h-3 w-3" /> {t("standardPrices.create")}
					</Button>
				</div>
			</header>

			<div className="flex flex-wrap items-center gap-3">
				<div className="relative max-w-sm flex-1">
					<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
					<Input
						type="search"
						placeholder={t("standardPrices.filterProvider")}
						value={providerFilter}
						onChange={(e) => {
							setProviderFilter(e.target.value);
							setOffset(0);
						}}
						className="pl-9"
						data-testid="standard-prices-filter-provider"
					/>
				</div>
				<div className="relative max-w-sm flex-1">
					<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
					<Input
						type="search"
						placeholder={t("standardPrices.filterModel")}
						value={modelFilter}
						onChange={(e) => {
							setModelFilter(e.target.value);
							setOffset(0);
						}}
						className="pl-9"
						data-testid="standard-prices-filter-model"
					/>
				</div>
				{hasFilters && (
					<Button variant="ghost" size="sm" onClick={clearFilters} data-testid="standard-prices-clear-filters">
						<X className="mr-1 h-3 w-3" /> {t("clearFilters")}
					</Button>
				)}
				<div className="text-muted-foreground ml-auto text-sm" data-testid="standard-prices-total-count">
					{t("totalCount", { count: total })}
				</div>
			</div>

			<div className="border-border bg-card rounded-sm border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>{t("standardPrices.col_provider")}</TableHead>
							<TableHead>{t("standardPrices.col_model")}</TableHead>
							<TableHead>{t("standardPrices.col_input")}</TableHead>
							<TableHead>{t("standardPrices.col_output")}</TableHead>
							<TableHead>{t("standardPrices.col_cache_read")}</TableHead>
							<TableHead>{t("standardPrices.col_cost_per_req")}</TableHead>
							<TableHead>{t("standardPrices.col_fx")}</TableHead>
							<TableHead>{t("standardPrices.col_effective_from")}</TableHead>
							<TableHead className="w-12 text-right">{t("col_actions")}</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading ? (
							<TableRow>
								<TableCell colSpan={9} className="text-muted-foreground py-8 text-center text-sm">
									<Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t("loading")}
								</TableCell>
							</TableRow>
						) : error ? (
							<TableRow>
								<TableCell colSpan={9} className="text-destructive py-8 text-center text-sm">
									{getErrorMessage(error)}
								</TableCell>
							</TableRow>
						) : rows.length === 0 ? (
							<TableRow>
								<TableCell colSpan={9} className="text-muted-foreground py-8 text-center text-sm">
									{t("standardPrices.empty")}
								</TableCell>
							</TableRow>
						) : (
							rows.map((row) => (
								<TableRow key={row.id} data-testid={`standard-prices-row-${row.id}`}>
									<TableCell className="font-mono text-xs">{row.provider}</TableCell>
									<TableCell className="font-mono text-xs">{row.model}</TableCell>
									<TableCell className="font-mono text-xs">{row.input_cost_per_million.toFixed(4)}</TableCell>
									<TableCell className="font-mono text-xs">{row.output_cost_per_million.toFixed(4)}</TableCell>
									<TableCell className="text-muted-foreground font-mono text-xs">
										{row.cache_read_cost_per_million?.toFixed(4) ?? "—"}
									</TableCell>
									<TableCell className="text-muted-foreground font-mono text-xs">{row.cost_per_request?.toFixed(6) ?? "—"}</TableCell>
									<TableCell className="text-muted-foreground font-mono text-xs">{row.fx_rate.toFixed(4)}</TableCell>
									<TableCell className="text-muted-foreground text-xs">{new Date(row.effective_from).toLocaleString()}</TableCell>
									<TableCell className="text-right">
										<Button
											variant="ghost"
											size="icon"
											onClick={() => {
												setEditingRow(row);
												setSheetOpen(true);
											}}
											data-testid={`standard-prices-edit-${row.id}`}
										>
											<Pencil className="h-4 w-4" />
										</Button>
										<Button
											variant="ghost"
											size="icon"
											onClick={() => setPendingDelete(row)}
											data-testid={`standard-prices-delete-${row.id}`}
										>
											<MoreHorizontal className="h-4 w-4" />
										</Button>
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			</div>

			{rows.length > 0 && (
				<Pagination offset={offset} limit={PAGE_SIZE} totalCount={total} onOffsetChange={(next) => setOffset(next)} showItemsInfo />
			)}

			<StandardPriceSheet
				open={sheetOpen}
				onOpenChange={(open) => {
					setSheetOpen(open);
					if (!open) setEditingRow(null);
				}}
				row={editingRow}
				onSubmit={onSubmit}
				isSubmitting={isUpserting}
			/>

			<AlertDialog open={!!pendingDelete} onOpenChange={(open) => !open && setPendingDelete(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("standardPrices.deleteConfirmTitle")}</AlertDialogTitle>
						<AlertDialogDescription>
							{t("standardPrices.deleteConfirmDesc", {
								provider: pendingDelete?.provider,
								model: pendingDelete?.model,
							})}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>{t("cancel")}</AlertDialogCancel>
						<AlertDialogAction onClick={() => pendingDelete && onDelete(pendingDelete)} data-testid="standard-prices-delete-confirm">
							{t("confirmDelete")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}

function StandardPriceSheet({
	open,
	onOpenChange,
	row,
	onSubmit,
	isSubmitting,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	row: StandardPriceRow | null;
	onSubmit: (body: StandardPriceUpsertRequest) => Promise<void>;
	isSubmitting: boolean;
}) {
	const { t } = useTranslation("reports");
	const [provider, setProvider] = useState("");
	const [model, setModel] = useState("");
	const [inputCost, setInputCost] = useState("0");
	const [outputCost, setOutputCost] = useState("0");
	const [cacheReadCost, setCacheReadCost] = useState("");
	const [costPerReq, setCostPerReq] = useState("");
	const [fxRate, setFxRate] = useState("1");
	const [currency, setCurrency] = useState("USD");

	// Re-init when opened
	const [initialized, setInitialized] = useState(false);
	if (open && !initialized) {
		setProvider(row?.provider ?? "");
		setModel(row?.model ?? "");
		setInputCost(row ? String(row.input_cost_per_million) : "0");
		setOutputCost(row ? String(row.output_cost_per_million) : "0");
		setCacheReadCost(row?.cache_read_cost_per_million != null ? String(row.cache_read_cost_per_million) : "");
		setCostPerReq(row?.cost_per_request != null ? String(row.cost_per_request) : "");
		setFxRate(row ? String(row.fx_rate) : "1");
		setCurrency(row?.currency ?? "USD");
		setInitialized(true);
	}
	if (!open && initialized) {
		setInitialized(false);
	}

	const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		const inputNum = Number(inputCost);
		const outputNum = Number(outputCost);
		const fxNum = Number(fxRate);
		if (!provider.trim() || !model.trim()) return;
		await onSubmit({
			id: row?.id,
			provider: provider.trim().toLowerCase(),
			model: model.trim(),
			currency: currency.trim().toUpperCase() || "USD",
			input_cost_per_million: Number.isFinite(inputNum) ? inputNum : 0,
			output_cost_per_million: Number.isFinite(outputNum) ? outputNum : 0,
			cache_read_cost_per_mil: cacheReadCost === "" ? null : Number(cacheReadCost),
			cost_per_request: costPerReq === "" ? null : Number(costPerReq),
			fx_rate: Number.isFinite(fxNum) ? fxNum : 1,
		});
	};

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="w-full sm:max-w-xl" data-testid="standard-prices-sheet">
				<SheetHeader>
					<SheetTitle>{row ? t("standardPrices.editTitle") : t("standardPrices.createTitle")}</SheetTitle>
					<SheetDescription>{t("standardPrices.sheetDesc")}</SheetDescription>
				</SheetHeader>
				<form onSubmit={handleSubmit} className="space-y-4 px-4 pb-4">
					<div className="grid grid-cols-2 gap-3">
						<div className="space-y-2">
							<Label htmlFor="sp-provider">{t("standardPrices.col_provider")}</Label>
							<Input
								id="sp-provider"
								value={provider}
								onChange={(e) => setProvider(e.target.value)}
								required
								data-testid="standard-prices-provider"
							/>
						</div>
						<div className="space-y-2">
							<Label htmlFor="sp-model">{t("standardPrices.col_model")}</Label>
							<Input id="sp-model" value={model} onChange={(e) => setModel(e.target.value)} required data-testid="standard-prices-model" />
						</div>
					</div>
					<div className="grid grid-cols-2 gap-3">
						<div className="space-y-2">
							<Label htmlFor="sp-input">{t("standardPrices.field_input")}</Label>
							<Input
								id="sp-input"
								type="number"
								step="any"
								value={inputCost}
								onChange={(e) => setInputCost(e.target.value)}
								required
								data-testid="standard-prices-input"
							/>
						</div>
						<div className="space-y-2">
							<Label htmlFor="sp-output">{t("standardPrices.field_output")}</Label>
							<Input
								id="sp-output"
								type="number"
								step="any"
								value={outputCost}
								onChange={(e) => setOutputCost(e.target.value)}
								required
								data-testid="standard-prices-output"
							/>
						</div>
					</div>
					<div className="grid grid-cols-2 gap-3">
						<div className="space-y-2">
							<Label htmlFor="sp-cache-read">{t("standardPrices.field_cache_read")}</Label>
							<Input
								id="sp-cache-read"
								type="number"
								step="any"
								value={cacheReadCost}
								onChange={(e) => setCacheReadCost(e.target.value)}
								placeholder={t("standardPrices.placeholderOptional")}
								data-testid="standard-prices-cache-read"
							/>
						</div>
						<div className="space-y-2">
							<Label htmlFor="sp-cost-per-req">{t("standardPrices.field_cost_per_req")}</Label>
							<Input
								id="sp-cost-per-req"
								type="number"
								step="any"
								value={costPerReq}
								onChange={(e) => setCostPerReq(e.target.value)}
								placeholder={t("standardPrices.placeholderOptional")}
								data-testid="standard-prices-cost-per-req"
							/>
						</div>
					</div>
					<div className="grid grid-cols-2 gap-3">
						<div className="space-y-2">
							<Label>{t("standardPrices.field_fx")}</Label>
							<Input
								type="number"
								step="any"
								value={fxRate}
								onChange={(e) => setFxRate(e.target.value)}
								required
								data-testid="standard-prices-fx"
							/>
						</div>
						<div className="space-y-2">
							<Label>{t("standardPrices.field_currency")}</Label>
							<Select value={currency} onValueChange={setCurrency}>
								<SelectTrigger data-testid="standard-prices-currency">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="USD">USD</SelectItem>
									<SelectItem value="EUR">EUR</SelectItem>
									<SelectItem value="CNY">CNY</SelectItem>
									<SelectItem value="JPY">JPY</SelectItem>
								</SelectContent>
							</Select>
						</div>
					</div>
					<SheetFooter className="px-0">
						<Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
							{t("cancel")}
						</Button>
						<Button type="submit" isLoading={isSubmitting} data-testid="standard-prices-submit">
							{row ? t("save") : t("create")}
						</Button>
					</SheetFooter>
				</form>
			</SheetContent>
		</Sheet>
	);
}

// silence unused-import lint for Select/Search if oxfmt drops them
void Search;