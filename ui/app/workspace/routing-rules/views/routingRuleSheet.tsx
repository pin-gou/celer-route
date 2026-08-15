import { VirtualKeySelector } from "@/components/entitySelectors/virtualKeySelector";
import { Button } from "@/components/ui/button";
import { ComboboxSelect } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ModelMultiselect } from "@/components/ui/modelMultiselect";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import { getProviderLabel } from "@/lib/constants/logs";
import { getErrorMessage } from "@/lib/store";
import { useGetAllKeysQuery, useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { useCreateRoutingRuleMutation, useGetRoutingRulesQuery, useUpdateRoutingRuleMutation } from "@/lib/store/apis/routingRulesApi";
import {
	DEFAULT_ROUTING_RULE_FORM_DATA,
	DEFAULT_ROUTING_TARGET,
	ROUTING_RULE_SCOPES,
	RoutingRule,
	RoutingRuleFormData,
	RoutingTargetFormData,
} from "@/lib/types/routingRules";
import { validateRateLimitAndBudgetRules, validateRoutingRules } from "@/lib/utils/celConverterRouting";
import { isValidRuleGroupType, normalizeRoutingRuleGroupQuery } from "@/lib/utils/routingRuleGroupQuery";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { Plus, Trash2, X } from "lucide-react";
import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { RuleGroupType } from "react-querybuilder";
import { Trans, useTranslation } from "react-i18next";
import { toast } from "sonner";

interface RoutingRuleDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	editingRule?: RoutingRule | null;
	onSuccess?: () => void;
}

const defaultQuery: RuleGroupType = {
	combinator: "and",
	rules: [],
};

type ConditionMode = "builder" | "cel";

function initialConditionMode(rule?: RoutingRule | null): ConditionMode {
	if (!rule) {
		return "builder";
	}
	const hasQuery = isValidRuleGroupType(rule.query) && (rule.query.rules?.length ?? 0) > 0;
	if (hasQuery) {
		return "builder";
	}
	return rule.cel_expression?.trim() ? "cel" : "builder";
}

const CELRuleBuilderLazy = lazy(() =>
	import("@/app/workspace/routing-rules/components/celBuilder/celRuleBuilder").then((mod) => ({
		default: mod.CELRuleBuilder,
	})),
);
const CELRuleBuilder = (props: React.ComponentProps<typeof CELRuleBuilderLazy>) => (
	<Suspense fallback={<div className="text-sm text-gray-500">Loading CEL builder...</div>}>
		<CELRuleBuilderLazy {...props} />
	</Suspense>
);

export function RoutingRuleSheet({ open, onOpenChange, editingRule, onSuccess }: RoutingRuleDialogProps) {
	const { t } = useTranslation("routing");
	const { data: rulesData } = useGetRoutingRulesQuery();
	const rules = rulesData?.rules || [];
	const { data: providersData = [] } = useGetProvidersQuery();
	const { data: allKeysData = [] } = useGetAllKeysQuery();
	const [createRoutingRule, { isLoading: isCreating }] = useCreateRoutingRuleMutation();
	const [updateRoutingRule, { isLoading: isUpdating }] = useUpdateRoutingRuleMutation();

	const [targets, setTargets] = useState<RoutingTargetFormData[]>([{ ...DEFAULT_ROUTING_TARGET }]);
	const [query, setQuery] = useState<RuleGroupType>(defaultQuery);
	const [conditionMode, setConditionMode] = useState<ConditionMode>("builder");
	const [builderKey, setBuilderKey] = useState(0);
	const [celError, setCelError] = useState<string | null>(null);

	const {
		register,
		handleSubmit,
		setValue,
		watch,
		reset,
		formState: { errors },
	} = useForm<RoutingRuleFormData>({
		defaultValues: DEFAULT_ROUTING_RULE_FORM_DATA,
	});

	const isEditing = !!editingRule;
	const isLoading = isCreating || isUpdating;
	const canCreate = useRbac(RbacResource.RoutingRules, RbacOperation.Create);
	const canUpdate = useRbac(RbacResource.RoutingRules, RbacOperation.Update);
	const hasRequiredAccess = isEditing ? canUpdate : canCreate;
	const enabled = watch("enabled");
	const chainRule = watch("chain_rule");
	const scope = watch("scope");
	const scopeId = watch("scope_id");

	const fallbacks = watch("fallbacks");

	const availableProviders = Array.from(
		new Set([
			...providersData.map((p) => p.name),
			...(targets.map((t) => t.provider).filter(Boolean) as string[]),
			...(rules.flatMap((r) => r.targets?.map((t) => t.provider).filter(Boolean) ?? []) as string[]),
			...rules.flatMap((r) => (r.fallbacks ?? []).map((f) => f.split("/")[0]?.trim()).filter(Boolean)),
		]),
	);
	const providerOptions = availableProviders.map((prov) => ({
		label: getProviderLabel(prov),
		value: prov,
		icon: <RenderProviderIcon provider={prov as ProviderIconType} size="sm" className="h-4 w-4" />,
	}));

	useEffect(() => {
		if (editingRule) {
			setValue("id", editingRule.id);
			setValue("name", editingRule.name);
			setValue("description", editingRule.description);
			setValue("cel_expression", editingRule.cel_expression);
			setValue("fallbacks", editingRule.fallbacks || []);
			setValue("scope", editingRule.scope);
			setValue("scope_id", editingRule.scope_id || "");
			setValue("priority", editingRule.priority);
			setValue("enabled", editingRule.enabled);
			setValue("chain_rule", editingRule.chain_rule ?? false);
			if (editingRule.targets && editingRule.targets.length > 0) {
				setTargets(
					editingRule.targets.map((t) => ({
						...DEFAULT_ROUTING_TARGET,
						provider: t.provider || "",
						model: t.model || "",
						key_id: t.key_id || "",
						weight: t.weight,
					})),
				);
			} else {
				setTargets([{ ...DEFAULT_ROUTING_TARGET }]);
			}
			setQuery(normalizeRoutingRuleGroupQuery(editingRule.query));
			setConditionMode(initialConditionMode(editingRule));
			setBuilderKey((prev) => prev + 1);
			setCelError(null);
		} else {
			reset();
			setTargets([{ ...DEFAULT_ROUTING_TARGET }]);
			setQuery(defaultQuery);
			setConditionMode("builder");
			setBuilderKey((prev) => prev + 1);
			setCelError(null);
		}
	}, [editingRule, open, setValue, reset]);

	const handleQueryChange = useCallback(
		(expression: string, newQuery: RuleGroupType) => {
			setValue("cel_expression", expression);
			setQuery(newQuery);
			setCelError(null);
		},
		[setValue],
	);

	const handleModeChange = useCallback((mode: ConditionMode) => {
		setConditionMode(mode);
		setCelError(null);
	}, []);

	const addTarget = () => {
		const remaining = 1 - targets.reduce((sum, t) => sum + (t.weight || 0), 0);
		setTargets((prev) => [...prev, { ...DEFAULT_ROUTING_TARGET, weight: Math.max(0, parseFloat(remaining.toFixed(4))) }]);
	};

	const removeTarget = (index: number) => {
		setTargets((prev) => prev.filter((_, i) => i !== index));
	};

	const updateTarget = (index: number, field: keyof RoutingTargetFormData, value: string | number) => {
		setTargets((prev) => prev.map((t, i) => (i === index ? { ...t, [field]: value } : t)));
	};

	const totalWeight = targets.reduce((sum, t) => sum + (t.weight || 0), 0);

	const onSubmit = (data: RoutingRuleFormData) => {
		setCelError(null);

		if (data.scope !== "global" && !data.scope_id?.trim()) {
			toast.error(t("rules.virtualKeyRequired"));
			return;
		}

		if (targets.length === 0) {
			toast.error(t("rules.routingTargetRequired"));
			return;
		}
		for (const target of targets) {
			if (target.weight <= 0) {
				toast.error(t("rules.targetWeightMustBePositive"));
				return;
			}
		}
		if (Math.abs(totalWeight - 1) > 0.001) {
			toast.error(t("rules.targetWeightsMustSumToOne", { total: totalWeight.toFixed(4) }));
			return;
		}

		if (conditionMode === "builder") {
			const regexErrors = validateRoutingRules(query);
			if (regexErrors.length > 0) {
				toast.error(t("sheet.invalidRegex", { errors: regexErrors.join("\n") }));
				return;
			}

			const rateLimitErrors = validateRateLimitAndBudgetRules(query);
			if (rateLimitErrors.length > 0) {
				toast.error(t("sheet.invalidRuleConfig", { errors: rateLimitErrors.join("\n") }));
				return;
			}
		}

		const validFallbacks = (data.fallbacks || []).filter((fb) => {
			const provider = fb.split("/")[0]?.trim();
			return provider && provider.length > 0;
		});

		const payload = {
			name: data.name,
			description: data.description,
			cel_expression: data.cel_expression,
			targets: targets.map(({ provider, model, key_id, weight }) => ({
				provider: provider || undefined,
				model: model || undefined,
				key_id: key_id || undefined,
				weight,
			})),
			fallbacks: validFallbacks,
			scope: data.scope,
			scope_id: data.scope === "global" ? undefined : data.scope_id || undefined,
			priority: data.priority,
			enabled: data.enabled,
			chain_rule: data.chain_rule,
			query: query,
		};

		const submitPromise =
			isEditing && editingRule
				? updateRoutingRule({
						id: editingRule.id,
						data: payload,
					}).unwrap()
				: createRoutingRule(payload).unwrap();

		submitPromise
			.then(() => {
				toast.success(isEditing ? t("toast.ruleUpdated") : t("toast.ruleCreated"));
				reset();
				setTargets([{ ...DEFAULT_ROUTING_TARGET }]);
				setQuery(defaultQuery);
				setConditionMode("builder");
				setBuilderKey((prev) => prev + 1);
				setCelError(null);
				onOpenChange(false);
				onSuccess?.();
			})
			.catch((error: any) => {
				const message = getErrorMessage(error);
				if (conditionMode === "cel" && /cel expression/i.test(message)) {
					setCelError(message);
					return;
				}
				toast.error(message);
			});
	};

	const handleCancel = () => {
		reset();
		setTargets([{ ...DEFAULT_ROUTING_TARGET }]);
		setQuery(defaultQuery);
		setConditionMode("builder");
		setBuilderKey((prev) => prev + 1);
		setCelError(null);
		onOpenChange(false);
	};

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="flex w-full min-w-1/2 flex-col gap-4 overflow-x-hidden p-0 pt-4">
				<SheetHeader className="flex flex-col items-start px-8 py-4" headerClassName="mb-0 sticky -top-4 bg-card z-10">
					<SheetTitle>{isEditing ? t("sheet.editTitle") : t("sheet.createTitle")}</SheetTitle>
					<SheetDescription>
						{isEditing ? t("sheet.editDescription") : t("sheet.createDescription")}
					</SheetDescription>
				</SheetHeader>

				<form onSubmit={handleSubmit(onSubmit)} className="flex grow flex-col">
					<div className="flex grow flex-col gap-6 px-8 pb-6">
						<div className="space-y-3">
							<Label htmlFor="name">
								{t("rules.ruleName")} <span className="text-red-500">*</span>
							</Label>
							<Input
								id="name"
								placeholder={t("rules.ruleNamePlaceholder")}
								{...register("name", { required: t("rules.ruleNameRequired"), maxLength: 255 })}
							/>
							{errors.name && <p className="text-destructive text-sm">{errors.name.message}</p>}
						</div>

						<div className="space-y-3">
							<Label htmlFor="description">{t("rules.description")}</Label>
							<Textarea id="description" placeholder={t("rules.descriptionPlaceholder")} rows={2} {...register("description")} />
						</div>

						<div className="flex items-center justify-between rounded-lg border p-4">
							<div className="space-y-0.5">
								<Label htmlFor="enabled">{t("rules.enableRule")}</Label>
								<p className="text-muted-foreground text-sm">{t("rules.enableRuleDescription")}</p>
							</div>
							<Switch id="enabled" checked={enabled} onCheckedChange={(checked) => setValue("enabled", checked)} />
						</div>

						<div className="flex items-center justify-between rounded-lg border p-4">
							<div className="space-y-0.5">
								<Label htmlFor="chain_rule">{t("rules.chainRule")}</Label>
								<p className="text-muted-foreground text-sm">
									{t("rules.chainRuleDescription")}
								</p>
							</div>
							<Switch
								id="chain_rule"
								checked={chainRule}
								onCheckedChange={(checked) => setValue("chain_rule", checked)}
								data-testid="routing-rule-chain-rule-switch"
							/>
						</div>

						<div className="grid grid-cols-2 gap-4">
							<div className="space-y-3">
								<Label htmlFor="scope">{t("rules.scope")}</Label>
								<Select
									value={scope}
									onValueChange={(value) => {
										setValue("scope", value as any);
										setValue("scope_id", "");
									}}
								>
									<SelectTrigger className="w-full">
										<SelectValue placeholder={t("rules.scopePlaceholder")} />
									</SelectTrigger>
									<SelectContent>
										{ROUTING_RULE_SCOPES.map((scopeOption) => (
											<SelectItem key={scopeOption.value} value={scopeOption.value}>
												{scopeOption.label}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>

							<div className="space-y-3">
								<Label htmlFor="priority">
									{t("rules.priority")} <span className="text-red-500">*</span>
								</Label>
								<Input
									id="priority"
									type="number"
									min={0}
									max={1000}
									{...register("priority", {
										required: t("rules.priorityRequired"),
										min: { value: 0, message: t("rules.priorityMustBeGeZero") },
										max: { value: 1000, message: t("rules.priorityMustBeLe1000") },
										valueAsNumber: true,
									})}
								/>
								<p className="text-muted-foreground text-xs">{t("rules.priorityHint")}</p>
								{errors.priority && <p className="text-destructive text-sm">{errors.priority.message}</p>}
							</div>
						</div>

						{scope !== "global" && (
							<div className="space-y-2">
								<Label htmlFor="scope_id">
									Virtual Key <span className="text-red-500">*</span>
								</Label>
								{scope === "virtual_key" && <VirtualKeySelector value={scopeId || ""} onChange={(value) => setValue("scope_id", value)} />}
								{errors.scope_id && <p className="text-destructive text-sm">{errors.scope_id.message}</p>}
							</div>
						)}

						<Separator />

						<div className="space-y-3">
							<Label>{t("sheet.ruleBuilder")}</Label>
							<p className="text-muted-foreground text-sm">
								{t("sheet.ruleBuilderDesc")}
							</p>
							<CELRuleBuilder
								key={builderKey}
								initialQuery={query}
								onChange={handleQueryChange}
								providers={availableProviders}
								models={[]}
								allowCustomModels={true}
								allowCelMode={true}
								initialMode={conditionMode}
								initialCel={editingRule?.cel_expression ?? ""}
								onModeChange={handleModeChange}
								celError={celError}
							/>
						</div>

						<p className="text-muted-foreground text-xs">
							<Trans
								i18nKey="sheet.budgetNote"
								ns="routing"
								components={{
									1: <strong />,
									3: <strong />,
								}}
							/>
						</p>

						<Separator />

						<div className="space-y-3">
							<div className="flex items-center justify-between">
								<div>
									<Label>{t("sheet.routingTargets")}</Label>
									<p className="text-muted-foreground mt-0.5 text-xs">
										{t("sheet.routingTargetsDesc")}
									</p>
								</div>
								<Button
									type="button"
									variant="outline"
									size="sm"
									onClick={addTarget}
									className="shrink-0 gap-2"
									data-testid="routing-rule-target-add"
								>
									<Plus className="h-4 w-4" />
									{t("sheet.addTarget")}
								</Button>
							</div>

							<div className="space-y-3">
								{targets.map((target, index) => (
									<TargetRow
										key={index}
										target={target}
										index={index}
										providerOptions={providerOptions}
										allKeys={allKeysData}
										showRemove={targets.length > 1}
										onUpdate={updateTarget}
										onRemove={removeTarget}
									/>
								))}
							</div>

							<div
								className={`flex items-center justify-end gap-2 text-xs font-medium ${Math.abs(totalWeight - 1) > 0.001 ? "text-destructive" : "text-muted-foreground"}`}
							>
								{t("rules.totalWeight", { total: totalWeight.toFixed(4) })}
								{Math.abs(totalWeight - 1) > 0.001 && <span className="text-destructive">{t("rules.weightMustEqual")}</span>}
							</div>
						</div>

						<div className="space-y-3">
							<div className="flex items-center justify-between">
								<div>
									<Label>{t("sheet.fallbacks")}</Label>{" "}
									<p className="text-muted-foreground mt-0.5 text-xs">
										{t("sheet.fallbacksDesc")}
									</p>
								</div>
								<Button
									type="button"
									variant="outline"
									size="sm"
									onClick={() => setValue("fallbacks", [...(fallbacks || []), ""])}
									className="gap-2"
								>
									<Plus className="h-4 w-4" />
									{t("sheet.addFallback")}
								</Button>
							</div>
							<div className="space-y-2">
								{(fallbacks || []).length === 0 ? (
									<p className="text-muted-foreground text-sm">{t("sheet.noFallbacks")}</p>
								) : (
									(fallbacks || []).map((fallback, index) => {
										const parts = fallback.split("/");
										const fbProvider = parts[0] || "";
										const fbModel = parts.slice(1).join("/");

										const handleProviderChange = (newProvider: string) => {
											const model = fbModel || "";
											const newFallback = `${newProvider}/${model}`;
											const newFallbacks = [...fallbacks];
											newFallbacks[index] = newFallback;
											setValue("fallbacks", newFallbacks);
										};

										const handleModelChange = (newModel: string) => {
											const prov = fbProvider || "";
											const newFallback = `${prov}/${newModel}`;
											const newFallbacks = [...fallbacks];
											newFallbacks[index] = newFallback;
											setValue("fallbacks", newFallbacks);
										};

										const handleRemove = () => {
											const newFallbacks = fallbacks.filter((_: string, i: number) => i !== index);
											setValue("fallbacks", newFallbacks);
										};

										return (
											<div key={index} className="flex items-center gap-2">
												<div className="flex-1">
													<ComboboxSelect
														options={providerOptions}
														value={fbProvider || null}
														onValueChange={(value) => handleProviderChange(value ?? "")}
														placeholder={t("rules.selectProvider")}
														className="h-9"
														noPortal
													/>
												</div>
												<div className="flex-1">
													<ModelMultiselect
														provider={fbProvider || undefined}
														value={fbModel}
														onChange={handleModelChange}
														placeholder={t("rules.incomingOptional")}
														isSingleSelect
														disabled={!fbProvider}
														className="!h-9 !min-h-9 w-full"
													/>
												</div>
												<Button
													type="button"
													variant="ghost"
													size="sm"
													onClick={handleRemove}
													className="h-9 px-2"
													aria-label={t("rules.removeFallback", { index: index + 1 })}
												>
													<Trash2 className="h-4 w-4" />
												</Button>
											</div>
										);
									})
								)}
							</div>
							<p className="text-muted-foreground text-xs">{t("sheet.fallbacksOrder")}</p>
						</div>
					</div>
					<div className="bg-card sticky bottom-0 flex justify-end gap-3 border-t px-8 py-4">
						<Button type="button" variant="outline" onClick={handleCancel} disabled={isLoading}>
							{t("sheet.cancel")}
						</Button>
						<Button type="submit" disabled={isLoading || !hasRequiredAccess}>
							{isEditing ? t("sheet.updateRule") : t("sheet.saveRule")}
						</Button>
					</div>
				</form>
			</SheetContent>
		</Sheet>
	);
}

interface TargetRowProps {
	target: RoutingTargetFormData;
	index: number;
	providerOptions: Array<{ label: string; value: string; icon: React.ReactNode }>;
	allKeys: Array<{ key_id: string; name: string; provider: string }>;
	showRemove: boolean;
	onUpdate: (index: number, field: keyof RoutingTargetFormData, value: string | number) => void;
	onRemove: (index: number) => void;
}

function TargetRow({ target, index, providerOptions, allKeys, showRemove, onUpdate, onRemove }: TargetRowProps) {
	const { t } = useTranslation("routing");
	const availableKeys = target.provider
		? allKeys.filter((k) => k.provider === target.provider).map((k) => ({ id: k.key_id, name: k.name }))
		: [];

	return (
		<div className="space-y-3 rounded-lg border p-3" data-testid={`routing-target-${index}`}>
			<div className="flex items-center justify-between">
				<span className="text-muted-foreground text-sm font-medium">{t("rules.targetWeight", { index: index + 1 })}</span>
				<div className="flex items-center gap-2">
					<div className="flex items-center gap-1.5">
						<Label htmlFor={`routing-target-${index}-weight-input`} className="text-muted-foreground shrink-0 text-xs">
							{t("rules.weight")}
						</Label>
						<Input
							id={`routing-target-${index}-weight-input`}
							type="number"
							min={0.001}
							max={1}
							step={0.001}
							value={target.weight}
							onChange={(e) => onUpdate(index, "weight", parseFloat(e.target.value) || 0)}
							className="h-8 w-24 text-sm"
							data-testid={`routing-target-${index}-weight-input`}
						/>
					</div>
					{showRemove && (
						<Button
							type="button"
							variant="ghost"
							size="sm"
							onClick={() => onRemove(index)}
							className="h-8 w-8 p-0"
							aria-label={t("rules.clearProvider", { index: index + 1 })}
							data-testid={`routing-target-${index}-remove-button`}
						>
							<Trash2 className="h-3.5 w-3.5" />
						</Button>
					)}
				</div>
			</div>

			<div className="grid grid-cols-2 gap-3">
				<div className="space-y-1.5">
					<Label id={`routing-target-${index}-provider-label`} className="text-xs">
						{t("rules.provider")}
					</Label>
					<div className="flex gap-1.5">
						<ComboboxSelect
							options={providerOptions}
							value={target.provider || null}
							onValueChange={(value) => {
								onUpdate(index, "provider", value ?? "");
								onUpdate(index, "model", "");
								onUpdate(index, "key_id", "");
							}}
							placeholder={t("rules.incomingOptional")}
							className="h-9 flex-1 text-sm"
							data-testid={`routing-target-${index}-provider-select`}
							noPortal
						/>
						{target.provider && (
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={() => {
									onUpdate(index, "provider", "");
									onUpdate(index, "model", "");
									onUpdate(index, "key_id", "");
								}}
								className="h-9 w-9 p-0"
								aria-label={t("rules.clearProvider", { index: index + 1 })}
								data-testid={`routing-target-${index}-provider-clear`}
							>
								<X className="h-3.5 w-3.5" />
							</Button>
						)}
					</div>
				</div>

				<div className="space-y-1.5">
					<Label id={`routing-target-${index}-model-label`} className="text-xs">
						{t("rules.model")}
					</Label>
					<div className="flex gap-1.5">
						<div className="flex-1" data-testid={`routing-target-${index}-model-select`}>
							<ModelMultiselect
								provider={target.provider || undefined}
								value={target.model}
								onChange={(value) => onUpdate(index, "model", value)}
								placeholder={t("rules.incomingOptional")}
								isSingleSelect
								loadModelsOnEmptyProvider
								className="!h-9 !min-h-9"
								inputId={`routing-target-${index}-model-input`}
								ariaLabelledBy={`routing-target-${index}-model-label`}
							/>
						</div>
						{target.model && (
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={() => onUpdate(index, "model", "")}
								className="h-9 w-9 p-0"
								aria-label={t("rules.clearModel", { index: index + 1 })}
								data-testid={`routing-target-${index}-model-clear`}
							>
								<X className="h-3.5 w-3.5" />
							</Button>
						)}
					</div>
				</div>
			</div>

			{target.provider && (availableKeys.length > 0 || target.key_id) && (
				<div className="space-y-1.5">
					<Label id={`routing-target-${index}-apikey-label`} className="text-xs">
						{t("rules.apiKey")} <span className="text-muted-foreground">{t("rules.apiKeyOptional")}</span>
					</Label>
					<div className="flex gap-1.5">
						<Select value={target.key_id || ""} onValueChange={(value) => onUpdate(index, "key_id", value)}>
							<SelectTrigger
								id={`routing-target-${index}-apikey-select`}
								aria-labelledby={`routing-target-${index}-apikey-label`}
								className="h-9 flex-1 text-sm"
								data-testid={`routing-target-${index}-apikey-select`}
							>
								<SelectValue placeholder={t("rules.selectKey")} />
							</SelectTrigger>
							<SelectContent>
								{availableKeys.map((key) => (
									<SelectItem key={key.id} value={key.id}>
										{key.name}
									</SelectItem>
								))}
								{target.key_id && !availableKeys.some((k) => k.id === target.key_id) && (
									<SelectItem key={`pinned-${target.key_id}`} value={target.key_id}>
										{t("rules.pinned")} {target.key_id}
									</SelectItem>
								)}
							</SelectContent>
						</Select>
						{target.key_id && (
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={() => onUpdate(index, "key_id", "")}
								className="h-9 w-9 p-0"
								aria-label={t("rules.clearApiKey", { index: index + 1 })}
								data-testid={`routing-target-${index}-apikey-clear`}
							>
								<X className="h-3.5 w-3.5" />
							</Button>
						)}
					</div>
				</div>
			)}
		</div>
	);
}