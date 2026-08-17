import FullPageLoader from "@/components/fullPageLoader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { useDebouncedValue } from "@/hooks/useDebounce";
import { RenderProviderIcon } from "@/lib/constants/icons";
import { ProviderLabels, ProviderName } from "@/lib/constants/logs";
import { ModelDetails, useGetModelDetailsQuery, useGetProvidersQuery } from "@/lib/store";
import { KnownProvider } from "@/lib/types/config";
import { formatTokenPriceCompact } from "@/lib/utils/numbers";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { ChevronLeft, ChevronRight, Edit, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import AttributeSheet from "./attributeSheet";

const PAGE_SIZE = 25;

const toTestIdPart = (value: string) =>
	value
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-|-$/g, "");

function DescriptionCell({ description }: { description?: string }) {
	if (!description) return <span className="text-muted-foreground text-sm">—</span>;
	const truncated = description.length > 80;
	const text = truncated ? `${description.slice(0, 80)}…` : description;
	if (!truncated) return <span className="block truncate text-sm">{text}</span>;
	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<span className="block truncate text-sm">{text}</span>
				</TooltipTrigger>
				<TooltipContent className="max-w-md">{description}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
}

export function ModelsSection() {
	const { t } = useTranslation("model-catalog");
	const hasUpdateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);

	const [search, setSearch] = useState("");
	const [providerFilter, setProviderFilter] = useState<string>("");
	const [offset, setOffset] = useState(0);
	const [editing, setEditing] = useState<ModelDetails | null>(null);

	const debouncedSearch = useDebouncedValue(search, 300);

	useEffect(() => {
		setOffset(0);
	}, [debouncedSearch, providerFilter]);

	const { data: providersData } = useGetProvidersQuery();
	const { data, isLoading, error, refetch } = useGetModelDetailsQuery({
		query: debouncedSearch || undefined,
		provider: providerFilter || undefined,
		limit: PAGE_SIZE,
		offset,
		unfiltered: true,
	});

	const models = data?.models ?? [];
	const totalCount = data?.total ?? 0;

	useEffect(() => {
		if (offset < totalCount) return;
		setOffset(totalCount === 0 ? 0 : Math.floor((totalCount - 1) / PAGE_SIZE) * PAGE_SIZE);
	}, [totalCount, offset]);

	const providerOptions = useMemo(() => Array.from(new Set((providersData ?? []).map((p) => p.name))).sort(), [providersData]);

	useEffect(() => {
		if (!providerFilter || !providersData) return;
		if (!providersData.some((p) => p.name === providerFilter)) {
			setProviderFilter("");
		}
	}, [providersData, providerFilter]);

	if (isLoading && !data) return <FullPageLoader />;

	if (error) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-4 text-center">
				<p className="text-muted-foreground text-sm">{t("modelsSection.failedToLoad")}</p>
				<button type="button" onClick={refetch} className="text-sm underline" data-testid="model-catalog-retry-button">
					{t("modelsSection.retry")}
				</button>
			</div>
		);
	}

	return (
		<>
			{editing && <AttributeSheet model={editing} onClose={() => setEditing(null)} />}

			<div className="flex min-h-0 w-full grow flex-col overflow-hidden">
				<div className="mb-4 flex shrink-0 items-center justify-between">
					<div>
						<h2 className="text-lg font-semibold">{t("modelsSection.title")}</h2>
						<p className="text-muted-foreground text-sm">{t("modelsSection.subtitle")}</p>
					</div>
				</div>

				<div className="mb-4 flex shrink-0 items-center gap-3">
					<div className="relative max-w-sm flex-1">
						<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
						<Input
							aria-label={t("modelsSection.searchAriaLabel")}
							placeholder={t("modelsSection.searchPlaceholder")}
							value={search}
							onChange={(e) => setSearch(e.target.value)}
							className="pl-9"
							data-testid="model-catalog-search-input"
						/>
					</div>
					<Select value={providerFilter || "__all__"} onValueChange={(v) => setProviderFilter(v === "__all__" ? "" : v)}>
						<SelectTrigger className="w-[200px]" data-testid="model-catalog-provider-filter">
							<SelectValue placeholder={t("modelsSection.allProviders")} />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="__all__">{t("modelsSection.allProviders")}</SelectItem>
							{providerOptions.map((p) => (
								<SelectItem key={p} value={p}>
									{ProviderLabels[p as ProviderName] || p}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>

				<div className="mb-2 min-h-0 grow overflow-hidden rounded-sm border" data-testid="model-catalog-attributes-table">
					<Table containerClassName="h-full overflow-y-auto overflow-x-hidden" className="table-fixed">
						<TableHeader className="bg-muted sticky top-0 z-20">
							<TableRow className="hover:bg-transparent">
								<TableHead className="w-[116px] font-medium">{t("table.provider")}</TableHead>
								<TableHead className="font-medium">{t("table.model")}</TableHead>
								<TableHead className="w-[72px] px-2 text-right font-medium">{t("table.input")}</TableHead>
								<TableHead className="w-[76px] px-2 text-right font-medium">{t("table.output")}</TableHead>
								<TableHead className="w-[86px] px-2 text-right font-medium">{t("table.cacheWrite")}</TableHead>
								<TableHead className="w-[80px] px-2 text-right font-medium">{t("table.cacheRead")}</TableHead>
								<TableHead className="font-medium">{t("table.description")}</TableHead>
								<TableHead className="w-[68px] font-medium">{t("table.other")}</TableHead>
								<TableHead className="w-[40px] px-1"></TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{models.length === 0 ? (
								<TableRow>
									<TableCell colSpan={9} className="h-24 text-center">
										<span className="text-muted-foreground text-sm">
											{!debouncedSearch && !providerFilter ? t("modelsSection.noModels") : t("modelsSection.noMatching")}
										</span>
									</TableCell>
								</TableRow>
							) : (
								models.map((m) => {
									const attrs = m.additional_attributes ?? {};
									const extraKeys = Object.keys(attrs).filter((k) => k !== "description");
									const testKey = `${toTestIdPart(m.name)}-${toTestIdPart(m.provider)}`;
									return (
										<TableRow key={`${m.provider}|${m.name}`} data-testid={`model-catalog-row-${testKey}`}>
											<TableCell className="py-3">
												<div className="flex min-w-0 items-center gap-2">
													<RenderProviderIcon provider={m.provider as KnownProvider} size="sm" className="h-4 w-4" />
													<span className="truncate text-sm">{ProviderLabels[m.provider as ProviderName] || m.provider}</span>
												</div>
											</TableCell>
											<TableCell className="truncate py-3 font-mono text-sm" title={m.name}>
												{m.name}
											</TableCell>
											<TableCell className="px-2 py-3 text-right font-mono text-sm">
												{formatTokenPriceCompact(m.input_cost_per_token)}
											</TableCell>
											<TableCell className="px-2 py-3 text-right font-mono text-sm">
												{formatTokenPriceCompact(m.output_cost_per_token)}
											</TableCell>
											<TableCell className="px-2 py-3 text-right font-mono text-sm">
												{formatTokenPriceCompact(m.cache_creation_input_token_cost)}
											</TableCell>
											<TableCell className="px-2 py-3 text-right font-mono text-sm">
												{formatTokenPriceCompact(m.cache_read_input_token_cost)}
											</TableCell>
											<TableCell className="py-3">
												<DescriptionCell description={attrs.description} />
											</TableCell>
											<TableCell className="py-3">
												{extraKeys.length === 0 ? (
													<span className="text-muted-foreground text-sm">—</span>
												) : (
													<Badge variant="secondary">
														{extraKeys.length} {extraKeys.length === 1 ? t("modelsSection.attribute") : t("modelsSection.attributes")}
													</Badge>
												)}
											</TableCell>
											<TableCell className="px-1 py-3">
												<Button
													variant="ghost"
													size="icon"
													className="h-7 w-7"
													disabled={!hasUpdateAccess}
													onClick={() => setEditing(m)}
													aria-label={`Edit attributes for ${m.name}`}
													data-testid={`model-catalog-edit-${testKey}`}
												>
													<Edit className="h-4 w-4" />
												</Button>
											</TableCell>
										</TableRow>
									);
								})
							)}
						</TableBody>
					</Table>
				</div>

				{totalCount > 0 && (
					<div className="flex shrink-0 items-center justify-between text-xs" data-testid="model-catalog-pagination">
						<div className="text-muted-foreground">
							{(offset + 1).toLocaleString()}–{Math.min(offset + PAGE_SIZE, totalCount).toLocaleString()} of {totalCount.toLocaleString()}{" "}
							{t("modelsSection.entries")}
						</div>
						<div className="flex items-center gap-2">
							<Button
								variant="ghost"
								size="sm"
								onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
								disabled={offset === 0}
								data-testid="model-catalog-pagination-prev-btn"
								aria-label={t("modelsSection.previousPage")}
							>
								<ChevronLeft className="size-3" />
							</Button>
							<div className="flex items-center gap-1">
								<span>{t("modelsSection.page")}</span>
								<span>{Math.floor(offset / PAGE_SIZE) + 1}</span>
								<span>{t("modelsSection.pageOf", { total: Math.ceil(totalCount / PAGE_SIZE) })}</span>
							</div>
							<Button
								variant="ghost"
								size="sm"
								onClick={() => setOffset(offset + PAGE_SIZE)}
								disabled={offset + PAGE_SIZE >= totalCount}
								data-testid="model-catalog-pagination-next-btn"
								aria-label={t("modelsSection.nextPage")}
							>
								<ChevronRight className="size-3" />
							</Button>
						</div>
					</div>
				)}
			</div>
		</>
	);
}