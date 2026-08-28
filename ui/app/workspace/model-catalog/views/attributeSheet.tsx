import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { DottedSeparator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { RenderProviderIcon } from "@/lib/constants/icons";
import { ProviderLabels, ProviderName } from "@/lib/constants/logs";
import { getErrorMessage, ModelDetails, useGetCoreConfigQuery, useUpsertModelCatalogEntriesMutation } from "@/lib/store";
import { useUpdateProviderMutation } from "@/lib/store/apis/providersApi";
import { KnownProvider, ModelProvider } from "@/lib/types/config";
import { formatTokenPriceFull } from "@/lib/utils/numbers";
import { definitionsForModel, providerModelHasDefaultParams } from "@/lib/utils/defaultParameters";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { buildProviderUpdatePayload } from "@/app/workspace/providers/views/utils";
import { ExternalLink, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const DEFAULT_PRICING_SOURCE_URL = "https://pin-gou.github.io/celer-route/datasheet";

interface AttributeSheetProps {
	model: ModelDetails;
	onClose: () => void;
	// Optional full provider config. When present and the provider has registered
	// default-parameter definitions (e.g. sensenova), a "Default Parameters"
	// section is shown for this model that writes back to the provider's
	// default_parameters map. Omit (e.g. Model Catalog) to hide the section.
	provider?: ModelProvider;
}

// Local row type for the default-parameters editor. Mirrors AttributeRow but
// carries the param key + value for a single model.
interface DefaultParamRow {
	id: string;
	param: string;
	value: string;
}

// Local row type for the extra-attributes editor. We keep these outside any
// schema because empty rows are valid during editing — we filter them at
// submit time. The id is a render-stable identifier (not persisted) so React
// keeps DOM nodes pinned to the right row across add/remove.
interface AttributeRow {
	id: string;
	key: string;
	value: string;
}

let rowIdCounter = 0;
function newRowId(): string {
	if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
		return crypto.randomUUID();
	}
	rowIdCounter += 1;
	return `row-${rowIdCounter}`;
}

function rowsFromAttributes(attrs?: Record<string, string>): AttributeRow[] {
	if (!attrs) return [];
	return Object.entries(attrs)
		.filter(([k]) => k !== "description")
		.map(([key, value]) => ({ id: newRowId(), key, value }));
}

function isLinkableSource(url: string) {
	return url.startsWith("http://") || url.startsWith("https://");
}

function getPricingSourceUrl(configuredUrl: string | undefined, modelName: string) {
	if (configuredUrl) return configuredUrl;
	const url = new URL(DEFAULT_PRICING_SOURCE_URL);
	url.searchParams.set("model", modelName);
	return url.toString();
}

export default function AttributeSheet({ model, onClose, provider }: AttributeSheetProps) {
	const { t } = useTranslation("model-catalog");
	const [isOpen, setIsOpen] = useState(true);
	const hasUpdateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const { data: bifrostConfig } = useGetCoreConfigQuery({ fromDB: true });

	const [upsertEntries, { isLoading }] = useUpsertModelCatalogEntriesMutation();
	const [updateProvider, { isLoading: isUpdatingProvider }] = useUpdateProviderMutation();

	const initialDescription = model.additional_attributes?.description ?? "";
	const [description, setDescription] = useState(initialDescription);

	const initialRows = useMemo(() => rowsFromAttributes(model.additional_attributes), [model.additional_attributes]);
	const stripIds = (rows: AttributeRow[]) => rows.map(({ key, value }) => ({ key, value }));
	const [initialRowsKey] = useState(() => JSON.stringify(stripIds(initialRows)));
	const [extraRows, setExtraRows] = useState<AttributeRow[]>(initialRows);

	// Default Parameters — only relevant when the provider has registered
	// definitions, this sheet received the full provider config, AND the model
	// being edited accepts at least one of those params (e.g. sensenova
	// reasoning_effort applies only to deepseek-v4-flash / glm-5.2).
	const definitions = provider?.default_parameters_definitions ?? [];
	const modelDefinitions = useMemo(() => definitionsForModel(definitions, model.name), [definitions, model.name]);
	const hasDefaultParams = providerModelHasDefaultParams(definitions, model.name);
	const initialParamMap = useMemo(() => {
		const configured = provider?.default_parameters?.[model.name];
		if (!configured) return {} as Record<string, string>;
		const supportedKeys = new Set(modelDefinitions.map((d) => d.key));
		const map: Record<string, string> = {};
		for (const [k, v] of Object.entries(configured)) {
			if (supportedKeys.has(k)) map[k] = String(v);
		}
		return map;
	}, [provider, model.name, modelDefinitions]);
	const [paramRows, setParamRows] = useState<DefaultParamRow[]>(() =>
		Object.entries(initialParamMap).map(([param, value]) => ({ id: newRowId(), param, value })),
	);
	const currentParamMap = useMemo(() => {
		const map: Record<string, string> = {};
		for (const r of paramRows) {
			if (r.param && r.value) map[r.param] = r.value;
		}
		return map;
	}, [paramRows]);

	const rowsDirty = JSON.stringify(stripIds(extraRows)) !== initialRowsKey;
	const paramRowsDirty = JSON.stringify(currentParamMap) !== JSON.stringify(initialParamMap);
	const isDirty = description !== initialDescription || rowsDirty || paramRowsDirty;
	const pricingSourceUrl = getPricingSourceUrl(bifrostConfig?.framework_config?.pricing_url, model.name);
	const canOpenPricingSource = isLinkableSource(pricingSourceUrl);

	const handleClose = () => {
		setIsOpen(false);
		setTimeout(() => onClose(), 150);
	};

	const handleAddRow = () => setExtraRows((prev) => [...prev, { id: newRowId(), key: "", value: "" }]);
	const handleRowChange = (id: string, field: "key" | "value", val: string) =>
		setExtraRows((prev) => prev.map((row) => (row.id === id ? { ...row, [field]: val } : row)));
	const handleRemoveRow = (id: string) => setExtraRows((prev) => prev.filter((row) => row.id !== id));

	const handleAddParamRow = () =>
		setParamRows((prev) => [
			...prev,
			{ id: newRowId(), param: modelDefinitions[0]?.key ?? "", value: modelDefinitions[0]?.options?.[0] ?? "" },
		]);
	const handleParamRowChange = (id: string, field: "param" | "value", val: string) =>
		setParamRows((prev) => prev.map((row) => (row.id === id ? { ...row, [field]: val } : row)));
	const handleRemoveParamRow = (id: string) => setParamRows((prev) => prev.filter((row) => row.id !== id));

	const handleSubmit = async () => {
		if (!hasUpdateAccess) {
			toast.error(t("attributeSheet.permissionDenied"));
			return;
		}

		// Validate that extra rows have non-empty keys when they have any value.
		// Empty rows are fine — we drop them.
		const cleaned = extraRows.map((r) => ({ key: r.key.trim(), value: r.value })).filter((r) => r.key !== "" || r.value !== "");
		const missingKey = cleaned.find((r) => r.key === "");
		if (missingKey) {
			toast.error(t("attributeSheet.errors.missingKey"));
			return;
		}
		const dupKey = cleaned.find((r, i) => cleaned.findIndex((other) => other.key === r.key) !== i);
		if (dupKey) {
			toast.error(t("attributeSheet.errors.duplicateKey", { key: dupKey.key }));
			return;
		}
		// "description" is the special-cased field above — disallow it as an extra row.
		const reservedClash = cleaned.find((r) => r.key === "description");
		if (reservedClash) {
			toast.error(t("attributeSheet.errors.reservedDescription"));
			return;
		}

		const attributes: Record<string, string> = {};
		const desc = description.trim();
		if (desc !== "") attributes.description = desc;
		for (const r of cleaned) attributes[r.key] = r.value;

		// Merge this model's default parameters into the provider's map, so we
		// only touch this model's entry and preserve every other model's defaults.
		const nextDefaults: Record<string, Record<string, string | number | boolean>> = provider?.default_parameters
			? { ...provider.default_parameters }
			: {};
		if (Object.keys(currentParamMap).length > 0) {
			nextDefaults[model.name] = { ...currentParamMap };
		} else {
			delete nextDefaults[model.name];
		}

		// Decouple the two saves: only submit each mutation when its section is
		// actually dirty. The attribute upsert requires an existing pricing row
		// for (model, provider); default parameters live in the provider config
		// and must not be blocked by a missing pricing row (e.g. glm-5.2).
		const attributesDirty = description !== initialDescription || rowsDirty;
		const paramsDirty = hasDefaultParams && paramRowsDirty;

		const tasks: Promise<unknown>[] = [];
		if (attributesDirty) {
			tasks.push(
				upsertEntries([
					{
						model: model.name,
						provider: model.provider,
						additional_attributes: Object.keys(attributes).length > 0 ? attributes : undefined,
					},
				]).unwrap(),
			);
		}
		if (paramsDirty && provider) {
			tasks.push(updateProvider(buildProviderUpdatePayload(provider, { default_parameters: nextDefaults })).unwrap());
		}
		// Defensive: the Save button is disabled when nothing is dirty, so this
		// is not expected to be reachable.
		if (tasks.length === 0) {
			handleClose();
			return;
		}

		try {
			await Promise.all(tasks);
			toast.success(t("toast.attributeSaved"));
			handleClose();
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	return (
		<Sheet open={isOpen} onOpenChange={(open) => !open && handleClose()}>
			<SheetContent
				className="flex w-full flex-col overflow-x-hidden pt-4"
				onInteractOutside={(e) => {
					if (isDirty) e.preventDefault();
				}}
				onEscapeKeyDown={(e) => {
					if (isDirty) e.preventDefault();
				}}
				data-testid="model-catalog-attribute-sheet"
			>
				<SheetHeader className="flex flex-col items-start p-0 px-8 py-4" headerClassName="mb-0 sticky -top-4 bg-card z-10">
					<SheetTitle>{t("attributeSheet.title")}</SheetTitle>
					<SheetDescription>{t("attributeSheet.description")}</SheetDescription>
				</SheetHeader>

				<div className="flex h-full flex-col gap-6">
					<div className="grow space-y-4 px-8">
						{/* Read-only provider / model header */}
						<div className="grid grid-cols-2 gap-4">
							<div>
								<Label className="text-sm font-medium">{t("attributeSheet.provider")}</Label>
								<div className="bg-muted/30 mt-2 flex items-center gap-2 rounded-sm border px-3 py-2 text-sm">
									<RenderProviderIcon provider={model.provider as KnownProvider} size="sm" className="h-4 w-4" />
									<span>{ProviderLabels[model.provider as ProviderName] || model.provider}</span>
								</div>
							</div>
							<div>
								<Label className="text-sm font-medium">{t("attributeSheet.model")}</Label>
								<div className="bg-muted/30 mt-2 rounded-sm border px-3 py-2 font-mono text-sm">{model.name}</div>
							</div>
						</div>

						<DottedSeparator />

						{/* Pricing */}
						<div className="space-y-3">
							<div className="flex items-center justify-between gap-3">
								<Label className="text-sm font-medium">{t("attributeSheet.pricing")}</Label>
								{canOpenPricingSource ? (
									<a
										href={pricingSourceUrl}
										target="_blank"
										rel="noreferrer"
										className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs"
										data-testid="model-catalog-pricing-source-link"
									>
										{t("attributeSheet.source")}
										<ExternalLink className="h-3 w-3" />
									</a>
								) : (
									<span className="text-muted-foreground max-w-[260px] truncate text-right font-mono text-xs" title={pricingSourceUrl}>
										{pricingSourceUrl}
									</span>
								)}
							</div>
							<div className="grid grid-cols-2 gap-4">
								<div className="bg-muted/30 rounded-sm border px-3 py-2">
									<p className="text-muted-foreground text-xs">{t("attributeSheet.input")}</p>
									<p className="mt-1 font-mono text-sm" data-testid="model-catalog-input-cost">
										{formatTokenPriceFull(model.input_cost_per_token)}
									</p>
								</div>
								<div className="bg-muted/30 rounded-sm border px-3 py-2">
									<p className="text-muted-foreground text-xs">{t("attributeSheet.output")}</p>
									<p className="mt-1 font-mono text-sm" data-testid="model-catalog-output-cost">
										{formatTokenPriceFull(model.output_cost_per_token)}
									</p>
								</div>
								<div className="bg-muted/30 rounded-sm border px-3 py-2">
									<p className="text-muted-foreground text-xs">{t("attributeSheet.cacheWrite")}</p>
									<p className="mt-1 font-mono text-sm" data-testid="model-catalog-cache-write-cost">
										{formatTokenPriceFull(model.cache_creation_input_token_cost)}
									</p>
								</div>
								<div className="bg-muted/30 rounded-sm border px-3 py-2">
									<p className="text-muted-foreground text-xs">{t("attributeSheet.cacheRead")}</p>
									<p className="mt-1 font-mono text-sm" data-testid="model-catalog-cache-read-cost">
										{formatTokenPriceFull(model.cache_read_input_token_cost)}
									</p>
								</div>
							</div>
						</div>

						<DottedSeparator />

						{/* Description */}
						<div>
							<Label className="text-sm font-medium">{t("attributeSheet.descriptionLabel")}</Label>
							<Textarea
								className="mt-2"
								value={description}
								onChange={(e) => setDescription(e.target.value)}
								rows={4}
								placeholder={t("attributeSheet.descriptionPlaceholder")}
								data-testid="model-catalog-description-textarea"
							/>
						</div>

						<DottedSeparator />

						{/* Other attributes */}
						<div className="space-y-3">
							<div className="flex items-center justify-between">
								<Label className="text-sm font-medium">{t("attributeSheet.otherAttributes")}</Label>
								<Button type="button" variant="outline" size="sm" onClick={handleAddRow} data-testid="model-catalog-add-attribute-row">
									<Plus className="mr-1 h-3 w-3" />
									{t("attributeSheet.add")}
								</Button>
							</div>
							{extraRows.length === 0 ? (
								<p className="text-muted-foreground text-xs">{t("attributeSheet.noAttributes")}</p>
							) : (
								<div className="space-y-2">
									{extraRows.map((row, i) => (
										<div key={row.id} className="flex items-start gap-2">
											<Input
												value={row.key}
												onChange={(e) => handleRowChange(row.id, "key", e.target.value)}
												placeholder={t("attributeSheet.keyPlaceholder")}
												className="flex-1"
												data-testid={`model-catalog-attribute-key-${i}`}
											/>
											<Input
												value={row.value}
												onChange={(e) => handleRowChange(row.id, "value", e.target.value)}
												placeholder={t("attributeSheet.valuePlaceholder")}
												className="flex-1"
												data-testid={`model-catalog-attribute-value-${i}`}
											/>
											<Button
												type="button"
												variant="ghost"
												size="icon"
												onClick={() => handleRemoveRow(row.id)}
												data-testid={`model-catalog-attribute-remove-${i}`}
											>
												<Trash2 className="h-4 w-4" />
											</Button>
										</div>
									))}
								</div>
							)}
						</div>

						{hasDefaultParams && (
							<>
								<DottedSeparator />

								{/* Default Parameters */}
								<div className="space-y-3">
									<div className="flex items-center justify-between">
										<Label className="text-sm font-medium">{t("attributeSheet.defaultParameters")}</Label>
										<Button
											type="button"
											variant="outline"
											size="sm"
											onClick={handleAddParamRow}
											data-testid="model-catalog-add-default-param-row"
										>
											<Plus className="mr-1 h-3 w-3" />
											{t("attributeSheet.addDefaultParam")}
										</Button>
									</div>
									<p className="text-muted-foreground text-xs">{t("attributeSheet.defaultParametersDescription")}</p>
									{paramRows.length === 0 ? (
										<p className="text-muted-foreground text-xs">{t("attributeSheet.noDefaultParams")}</p>
									) : (
										<div className="space-y-2">
											{paramRows.map((row, i) => (
												<div key={row.id} className="flex items-start gap-2">
													<Select
														value={row.param}
														onValueChange={(v) => handleParamRowChange(row.id, "param", v)}
														disabled={!hasUpdateAccess}
													>
														<SelectTrigger className="w-full flex-1" data-testid={`model-catalog-default-param-${i}`}>
															<SelectValue placeholder={t("attributeSheet.selectParam")} />
														</SelectTrigger>
														<SelectContent>
															{modelDefinitions.map((d) => (
																<SelectItem key={d.key} value={d.key}>
																	{d.label}
																</SelectItem>
															))}
														</SelectContent>
													</Select>
													<Select
														value={row.value}
														onValueChange={(v) => handleParamRowChange(row.id, "value", v)}
														disabled={!hasUpdateAccess}
													>
														<SelectTrigger className="w-full flex-1" data-testid={`model-catalog-default-value-${i}`}>
															<SelectValue placeholder={t("attributeSheet.selectValue")} />
														</SelectTrigger>
														<SelectContent>
															{(modelDefinitions.find((d) => d.key === row.param)?.options ?? []).map((o) => (
																<SelectItem key={o} value={o}>
																	{o}
																</SelectItem>
															))}
														</SelectContent>
													</Select>
													<Button
														type="button"
														variant="ghost"
														size="icon"
														onClick={() => handleRemoveParamRow(row.id)}
														data-testid={`model-catalog-default-remove-${i}`}
													>
														<Trash2 className="h-4 w-4" />
													</Button>
												</div>
											))}
										</div>
									)}
								</div>
							</>
						)}
					</div>

					<div className="bg-card sticky bottom-0 shrink-0 border-t px-8 py-4">
						<div className="flex items-center justify-end gap-3">
							{!hasUpdateAccess && <p className="text-destructive text-sm">{t("attributeSheet.permissionDenied")}</p>}
							<Button type="button" variant="outline" onClick={handleClose} data-testid="model-catalog-attribute-cancel">
								{t("attributeSheet.cancel")}
							</Button>
							<Button
								type="button"
								onClick={handleSubmit}
								disabled={isLoading || isUpdatingProvider || !isDirty || !hasUpdateAccess}
								data-testid="model-catalog-attribute-submit"
							>
								{isLoading || isUpdatingProvider ? t("attributeSheet.saving") : t("attributeSheet.saveChanges")}
							</Button>
						</div>
					</div>
				</div>
			</SheetContent>
		</Sheet>
	);
}