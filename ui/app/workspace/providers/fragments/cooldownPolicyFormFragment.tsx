import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage, setProviderFormDirtyState, useAppDispatch } from "@/lib/store";
import { useUpdateProviderMutation } from "@/lib/store/apis/providersApi";
import {
	cooldownPolicySchema,
	type CooldownPolicyFormSchema,
	type CooldownPolicyMatchFormSchema,
	type CooldownPolicyRuleFormSchema,
} from "@/lib/types/schemas";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { zodResolver } from "@hookform/resolvers/zod";
import { HelpCircle, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useFieldArray, useForm, useFormContext, useWatch, type Control, type Resolver } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { buildProviderUpdatePayload } from "../views/utils";
import type { ModelProvider } from "@/lib/types/config";
import { useGetProviderErrorCatalogQuery } from "@/lib/store/apis/providerErrorCatalogApi";
import { ErrorSampleBrowser } from "./errorSampleBrowser";
import type { ErrorPattern } from "@/lib/store/apis/errorPatternsApi";

interface CooldownPolicyFormFragmentProps {
	provider: ModelProvider;
	onCancel?: () => void;
}

const RULE_FIELDS = ["rate_limit", "quota"] as const;
type RuleField = (typeof RULE_FIELDS)[number];

const DEFAULT_RULE = (ttlSeconds: number): NonNullable<CooldownPolicyFormSchema["rate_limit"]> => ({
	match: [{ status_code: 429 }],
	match_mode: "any",
	ttl_seconds: ttlSeconds,
});

export function CooldownPolicyFormFragment({ provider, onCancel }: CooldownPolicyFormFragmentProps) {
	const { t } = useTranslation("providers");
	const dispatch = useAppDispatch();
	const hasUpdateProviderAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const [updateProvider, { isLoading: isUpdatingProvider }] = useUpdateProviderMutation();

	const buildDefaultValues = () =>
	({
		rate_limit: provider.cooldown_policy?.rate_limit ?? DEFAULT_RULE(60),
		quota: provider.cooldown_policy?.quota ?? undefined,
	}) as CooldownPolicyFormSchema;

	const form = useForm<CooldownPolicyFormSchema, any, CooldownPolicyFormSchema>({
		resolver: zodResolver(cooldownPolicySchema) as Resolver<CooldownPolicyFormSchema, any, CooldownPolicyFormSchema>,
		mode: "onChange",
		reValidateMode: "onChange",
		defaultValues: buildDefaultValues(),
	});

	const initialValuesRef = useRef<CooldownPolicyFormSchema>(buildDefaultValues());

	const watchedFormValues = useWatch({ control: form.control }) as CooldownPolicyFormSchema;

	const isFormChanged = useMemo(() => JSON.stringify(watchedFormValues) !== JSON.stringify(initialValuesRef.current), [watchedFormValues]);

	useEffect(() => {
		dispatch(setProviderFormDirtyState(isFormChanged));
	}, [isFormChanged, dispatch]);

	useEffect(() => {
		const next = buildDefaultValues();
		initialValuesRef.current = next;
		form.reset(next);
	}, [form, provider.name, provider.cooldown_policy]);

	const onSubmit = (data: CooldownPolicyFormSchema) => {
		updateProvider(
			buildProviderUpdatePayload(provider, {
				cooldown_policy: {
					rate_limit: data.rate_limit,
					quota: data.quota,
				},
			}),
		)
			.unwrap()
			.then(() => {
				toast.success(t("fragments.cooldownPolicy.toast.updated"));
				initialValuesRef.current = JSON.parse(JSON.stringify(data)) as CooldownPolicyFormSchema;
				form.reset(data);
			})
			.catch((err: unknown) => {
				toast.error(t("fragments.cooldownPolicy.toast.failedToUpdate"), {
					description: getErrorMessage(err),
				});
			});
	};

	const onClear = () => {
		updateProvider(
			buildProviderUpdatePayload(provider, {
				cooldown_policy: null,
			}),
		)
			.unwrap()
			.then(() => {
				toast.success(t("fragments.cooldownPolicy.toast.cleared"));
				const resetValues = { rate_limit: DEFAULT_RULE(60), quota: undefined } as CooldownPolicyFormSchema;
				initialValuesRef.current = JSON.parse(JSON.stringify(resetValues)) as CooldownPolicyFormSchema;
				form.reset(resetValues);
			})
			.catch((err: unknown) => {
				toast.error(t("fragments.cooldownPolicy.toast.failedToUpdate"), {
					description: getErrorMessage(err),
				});
			});
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6 px-6" data-testid="provider-config-cooldown-content">
				<div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
					{/* Left: error sample browser */}
					<div className="bg-muted/40 rounded-md border p-3" data-testid="error-sample-browser">
						<h5 className="mb-2 text-sm font-medium">{t("fragments.cooldownPolicy.errorSample.title")}</h5>
						<ErrorSampleBrowserBridge provider={provider.name} />
					</div>

					{/* Right: rule editor */}
					<div className="space-y-4">
						<InnerFields provider={String(provider.name)} />
					</div>
				</div>

				<div className="flex flex-wrap items-center justify-between gap-2 pb-6">
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={onClear}
						disabled={!hasUpdateProviderAccess || isUpdatingProvider}
						data-testid="provider-cooldown-clear-button"
					>
						{t("fragments.cooldownPolicy.clear")}
					</Button>
					<div className="flex items-center gap-2">
						{onCancel && (
							<Button type="button" variant="outline" size="sm" onClick={onCancel}>
								{t("fragments.cooldownPolicy.cancel")}
							</Button>
						)}
						<Button
							type="submit"
							disabled={!isFormChanged || !form.formState.isValid || !hasUpdateProviderAccess || isUpdatingProvider}
							isLoading={isUpdatingProvider}
							data-testid="provider-cooldown-save-button"
						>
							{t("fragments.cooldownPolicy.save")}
						</Button>
					</div>
				</div>
			</form>
		</Form>
	);
}

// ---------------------------------------------------------------------------
// Left column: error sample browser — clicking "apply" opens a popup to
// pick the target rule (rate_limit or quota).
// ---------------------------------------------------------------------------

function ErrorSampleBrowserBridge({ provider }: { provider: ModelProvider["name"] }) {
	const { t } = useTranslation("providers");
	const { control, getValues, setValue } = useFormContext<CooldownPolicyFormSchema>();

	const [pendingPattern, setPendingPattern] = useState<ErrorPattern | null>(null);
	const [dialogOpen, setDialogOpen] = useState(false);

	const handleApply = (pattern: ErrorPattern) => {
		setPendingPattern(pattern);
		setDialogOpen(true);
	};

	const handleConfirmApply = (targetRule: RuleField) => {
		const pattern = pendingPattern;
		if (!pattern) return;

		const newMatch: CooldownPolicyMatchFormSchema = {};

		if (pattern.status_code !== undefined) {
			newMatch.status_code = pattern.status_code;
		}
		if (pattern.error_type) {
			newMatch.type = [pattern.error_type];
		}
		if (pattern.error_code) {
			newMatch.code = [pattern.error_code];
		}
		if (pattern.sample_message) {
			newMatch.message_contains = [pattern.sample_message.slice(0, 80)];
		}

		const current = getValues(targetRule);
		if (!current) {
			setValue(targetRule, DEFAULT_RULE(60), { shouldDirty: true, shouldValidate: true });
		}

		const matchPath = `${targetRule}.match` as const;
		const currentMatches = getValues(matchPath as never) ?? [];
		setValue(matchPath as never, [...currentMatches, newMatch] as never, { shouldDirty: true, shouldValidate: true });

		setDialogOpen(false);
		setPendingPattern(null);
		toast.success(t("fragments.cooldownPolicy.errorSample.toast.applied"));
	};

	return (
		<div className="flex flex-col gap-3">
			<ErrorSampleBrowser provider={String(provider)} onApply={handleApply} />

			<Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
				<DialogContent data-testid="error-sample-apply-dialog">
					<DialogHeader>
						<DialogTitle>{t("fragments.cooldownPolicy.errorSample.applyDialogTitle")}</DialogTitle>
						<DialogDescription>{t("fragments.cooldownPolicy.errorSample.applyDialogDescription")}</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={() => setDialogOpen(false)}
							data-testid="error-sample-apply-dialog-cancel"
						>
							{t("fragments.cooldownPolicy.cancel")}
						</Button>
						{RULE_FIELDS.map((r) => (
							<Button key={r} type="button" size="sm" onClick={() => handleConfirmApply(r)} data-testid={`error-sample-apply-dialog-${r}`}>
								{t(`fragments.cooldownPolicy.${r}.label`)}
							</Button>
						))}
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}

// ---------------------------------------------------------------------------
// Right column: rule editor (rate_limit + quota)
// ---------------------------------------------------------------------------

function InnerFields({ provider }: { provider: string }) {
	const { t } = useTranslation("providers");
	const { control, watch, setValue, getValues } = useFormContext<CooldownPolicyFormSchema>();
	const savedRules = useRef<Partial<Record<RuleField, CooldownPolicyRuleFormSchema>>>({});
	return (
		<>
			{RULE_FIELDS.map((ruleKey) => {
				const enabled = watch(ruleKey) !== undefined;
				return (
					<div key={ruleKey} className="space-y-3 rounded-md border p-3">
						<div className="flex items-center justify-between">
							<FormLabel className="text-sm font-medium">{t(`fragments.cooldownPolicy.${ruleKey}.label`)}</FormLabel>
							<div className="flex items-center gap-2">
								<span className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.enableRule")}</span>
								<Switch
									data-testid={`provider-cooldown-${ruleKey}-enable-switch`}
									size="md"
									checked={enabled}
									onCheckedChange={(checked) => {
										if (checked) {
											const current = getValues(ruleKey);
											const saved = savedRules.current[ruleKey];
											setValue(ruleKey, current ?? saved ?? DEFAULT_RULE(60), {
												shouldDirty: true,
											});
											delete savedRules.current[ruleKey];
										} else {
											const current = getValues(ruleKey);
											if (current) {
												savedRules.current[ruleKey] = JSON.parse(JSON.stringify(current)) as CooldownPolicyRuleFormSchema;
											}
											setValue(ruleKey, undefined as never, { shouldDirty: true });
										}
									}}
								/>
							</div>
						</div>

						{enabled && <RuleFields ruleKey={ruleKey} control={control} provider={provider} />}
					</div>
				);
			})}
		</>
	);
}

function RuleFields({ ruleKey, control, provider }: { ruleKey: RuleField; control: Control<CooldownPolicyFormSchema>; provider: string }) {
	const { t } = useTranslation("providers");
	const matchName = `${ruleKey}.match` as const;
	const { fields, append, remove } = useFieldArray({ control, name: matchName });

	return (
		<>
			<div className="grid grid-cols-2 gap-3">
				<FormField
					control={control}
					name={`${ruleKey}.match_mode`}
					render={({ field }) => (
						<FormItem>
							<FormLabel className="text-xs">{t("fragments.cooldownPolicy.matchModeLabel")}</FormLabel>
							<FormControl>
								<Select value={(field.value ?? "any") as string} onValueChange={field.onChange}>
									<SelectTrigger data-testid={`provider-cooldown-${ruleKey}-match-mode-trigger`}>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="any">{t("fragments.cooldownPolicy.matchModeAny")}</SelectItem>
										<SelectItem value="all">{t("fragments.cooldownPolicy.matchModeAll")}</SelectItem>
									</SelectContent>
								</Select>
							</FormControl>
						</FormItem>
					)}
				/>

				<FormField
					control={control}
					name={`${ruleKey}.ttl_seconds`}
					render={({ field }) => (
						<FormItem>
							<FormLabel className="flex items-center gap-1.5">
								{t("fragments.cooldownPolicy.ttlLabel")}
								<HelpHint>{t("fragments.cooldownPolicy.ttlDescription")}</HelpHint>
							</FormLabel>
							<FormControl>
								<Input
									data-testid={`provider-cooldown-${ruleKey}-ttl-input`}
									type="number"
									min={1}
									max={86400}
									value={field.value ?? 0}
									onChange={(e) => field.onChange(Number(e.target.value) || 0)}
								/>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>
			</div>

			<div className="space-y-2">
				<div className="flex items-center justify-between">
					<span className="text-xs font-medium">{t("fragments.cooldownPolicy.matchesLabel")}</span>
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={() => append({ status_code: 429 })}
						data-testid={`provider-cooldown-${ruleKey}-add-match`}
					>
						<PlusIcon className="mr-1 h-3 w-3" />
						{t("fragments.cooldownPolicy.addMatch")}
					</Button>
				</div>

				{fields.length === 0 && <p className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.noMatches")}</p>}

				{fields.map((field, index) => (
					<MatchSummaryRow
						key={field.id}
						ruleKey={ruleKey}
						index={index}
						control={control}
						provider={provider}
						onRemove={() => remove(index)}
					/>
				))}
			</div>
		</>
	);
}

function MatchSummaryRow({
	ruleKey,
	index,
	control,
	provider,
	onRemove,
}: {
	ruleKey: RuleField;
	index: number;
	control: Control<CooldownPolicyFormSchema>;
	provider: string;
	onRemove: () => void;
}) {
	const { t } = useTranslation("providers");
	const baseName = `${ruleKey}.match.${index}` as const;
	const match = useWatch({ control, name: baseName as never }) as CooldownPolicyMatchFormSchema | undefined;
	const [editing, setEditing] = useState(false);
	const summary = matchSummaryText(match);

	return (
		<>
			<div
				className="bg-muted/30 flex min-h-8 items-center justify-between gap-2 rounded border px-2 py-1"
				data-testid={`provider-cooldown-${ruleKey}-match-${index}`}
			>
				<div className="text-muted-foreground min-w-0 flex-1 truncate font-mono text-xs">
					{summary || (
						<span>
							#{index + 1} {ruleKey}.match[{index}]
						</span>
					)}
				</div>
				<div className="flex shrink-0 items-center gap-0.5">
					<Button
						type="button"
						variant="ghost"
						size="sm"
						onClick={() => setEditing(true)}
						data-testid={`provider-cooldown-${ruleKey}-match-${index}-edit`}
					>
						<PencilIcon className="h-3.5 w-3.5" />
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="sm"
						onClick={onRemove}
						data-testid={`provider-cooldown-${ruleKey}-match-${index}-remove`}
					>
						<Trash2Icon className="h-4 w-4" />
					</Button>
				</div>
			</div>

			<Dialog open={editing} onOpenChange={setEditing}>
				<DialogContent data-testid={`provider-cooldown-${ruleKey}-match-${index}-dialog`}>
					<DialogHeader>
						<DialogTitle>{t("fragments.cooldownPolicy.editMatchTitle")}</DialogTitle>
						<DialogDescription className="font-mono text-xs">
							{ruleKey}.match[{index}]
						</DialogDescription>
					</DialogHeader>

					<MatchEditor ruleKey={ruleKey} index={index} control={control} provider={provider} />

					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={() => setEditing(false)}
							data-testid={`provider-cooldown-${ruleKey}-match-${index}-dialog-done`}
						>
							{t("fragments.cooldownPolicy.done")}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	);
}

function matchSummaryText(match: CooldownPolicyMatchFormSchema | undefined): string {
	if (!match) return "";
	const parts: string[] = [];
	if (match.status_code !== undefined) {
		parts.push(`[${match.status_code}]`);
	}
	if (match.type?.length) {
		parts.push(match.type.join(" · "));
	}
	if (match.code?.length) {
		parts.push(`code: ${match.code.join(" · ")}`);
	}
	if (match.message_contains?.length) {
		parts.push(match.message_contains.map((m) => `"${m}"`).join(" · "));
	}
	return parts.join("  ");
}

function MatchEditor({
	ruleKey,
	index,
	control,
	provider,
}: {
	ruleKey: RuleField;
	index: number;
	control: Control<CooldownPolicyFormSchema>;
	provider: string;
}) {
	const { t } = useTranslation("providers");
	const baseName = `${ruleKey}.match.${index}` as const;

	return (
		<div className="space-y-3">
			<FormField
				control={control}
				name={`${baseName}.status_code`}
				render={({ field }) => (
					<FormItem>
						<FormLabel className="text-xs">{t("fragments.cooldownPolicy.matchField.statusCode")}</FormLabel>
						<FormControl>
							<Input
								type="number"
								min={100}
								max={599}
								placeholder="429"
								data-testid={`provider-cooldown-${ruleKey}-match-${index}-status-code`}
								value={field.value ?? ""}
								onChange={(e) => {
									const v = e.target.value;
									const n = v === "" ? undefined : Number(v);
									field.onChange(Number.isNaN(n) ? undefined : n);
								}}
							/>
						</FormControl>
					</FormItem>
				)}
			/>

			<StringListField
				label={t("fragments.cooldownPolicy.matchField.messageContains")}
				testId={`provider-cooldown-${ruleKey}-match-${index}-message`}
				control={control}
				baseName={`${baseName}.message_contains`}
			/>

			<TypeCodeField
				label={t("fragments.cooldownPolicy.matchField.type")}
				provider={provider}
				baseName={`${baseName}.type`}
				fieldKind="type"
			/>

			<TypeCodeField
				label={t("fragments.cooldownPolicy.matchField.code")}
				provider={provider}
				baseName={`${baseName}.code`}
				fieldKind="code"
			/>
		</div>
	);
}

function StringListField({
	label,
	testId,
	control,
	baseName,
}: {
	label: string;
	testId: string;
	control: Control<CooldownPolicyFormSchema>;
	baseName: string;
}) {
	const { t } = useTranslation("providers");
	const { fields, append, remove } = useFieldArray({ control, name: baseName as never });

	return (
		<div className="space-y-1">
			<div className="flex items-center justify-between">
				<span className="text-xs font-medium">{label}</span>
				<Button type="button" variant="outline" size="sm" onClick={() => append("")} data-testid={`${testId}-add`}>
					<PlusIcon className="mr-1 h-3 w-3" />
					{t("fragments.cooldownPolicy.addItem")}
				</Button>
			</div>

			{fields.length === 0 && <p className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.noItems")}</p>}

			{fields.map((f, i) => (
				<div key={f.id} className="flex items-center gap-2">
					<FormField
						control={control}
						name={`${baseName}.${i}` as never}
						render={({ field }) => (
							<Input data-testid={`${testId}-${i}`} value={(field.value as unknown as string) ?? ""} onChange={field.onChange} />
						)}
					/>
					<Button type="button" variant="ghost" size="sm" onClick={() => remove(i)} data-testid={`${testId}-${i}-remove`}>
						<Trash2Icon className="h-4 w-4" />
					</Button>
				</div>
			))}
		</div>
	);
}

function TypeCodeField({
	label,
	provider,
	baseName,
	fieldKind,
}: {
	label: string;
	provider: string;
	baseName: string;
	fieldKind: "type" | "code";
}) {
	const { t } = useTranslation("providers");
	const { control, getValues, setValue } = useFormContext<CooldownPolicyFormSchema>();
	const { data: catalog } = useGetProviderErrorCatalogQuery(provider, { skip: !provider });
	const options = catalog?.[fieldKind === "type" ? "types" : "codes"] ?? [];
	const { fields, append, remove } = useFieldArray({
		control,
		name: baseName as never,
	});

	return (
		<div className="space-y-1">
			<div className="flex items-center justify-between">
				<span className="text-xs font-medium">{label}</span>
				<Button type="button" variant="outline" size="sm" onClick={() => append("" as never)} data-testid={`cooldown-${baseName}-add`}>
					<PlusIcon className="mr-1 h-3 w-3" />
					{t("fragments.cooldownPolicy.addItem")}
				</Button>
			</div>

			{fields.length === 0 && <p className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.noItems")}</p>}

			{fields.map((f, i) => {
				const currentValues = (getValues(baseName as never) as unknown as string[]) ?? [];
				const v = currentValues[i] ?? "";
				const isKnown = !v || options.includes(v);
				return (
					<div key={f.id} className="flex items-center gap-2">
						{isKnown ? (
							<Select
								value={v}
								onValueChange={(nv) => {
									const nvUse = nv === "__custom__" ? "" : nv;
									const next = [...currentValues];
									next[i] = nvUse;
									setValue(baseName as never, next as never, { shouldDirty: true });
								}}
							>
								<SelectTrigger className="h-8 flex-1 text-xs" data-testid={`cooldown-${fieldKind}-select-${i}`}>
									<SelectValue placeholder={t("fragments.cooldownPolicy.typeSelect.selectPlaceholder")} />
								</SelectTrigger>
								<SelectContent>
									{options.map((opt) => (
										<SelectItem key={opt} value={opt}>
											{opt}
										</SelectItem>
									))}
									<SelectItem value="__custom__">{t("fragments.cooldownPolicy.typeSelect.custom")}</SelectItem>
								</SelectContent>
							</Select>
						) : (
							<FormField
								control={control}
								name={`${baseName}.${i}` as never}
								render={({ field }) => (
									<Input
										className="h-8 flex-1 text-xs"
										value={(field.value as unknown as string) ?? ""}
										onChange={(e) => {
											field.onChange(e);
										}}
										placeholder={t("fragments.cooldownPolicy.typeSelect.customPlaceholder")}
										data-testid={`cooldown-${fieldKind}-custom-input-${i}`}
									/>
								)}
							/>
						)}
						<Button type="button" variant="ghost" size="sm" onClick={() => remove(i)} data-testid={`cooldown-${fieldKind}-remove-${i}`}>
							<Trash2Icon className="h-4 w-4" />
						</Button>
					</div>
				);
			})}
		</div>
	);
}

export type _MatchSchemaRef = CooldownPolicyMatchFormSchema;

// ---------------------------------------------------------------------------
// HelpHint — small (?) icon with a tooltip; used inline next to field labels
// to surface hints without an extra description line.
// ---------------------------------------------------------------------------

function HelpHint({ children }: { children: React.ReactNode }) {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="text-muted-foreground inline-flex cursor-help items-center" tabIndex={0}>
					<HelpCircle className="h-3.5 w-3.5" />
				</span>
			</TooltipTrigger>
			<TooltipContent side="top" className="max-w-xs text-xs">
				{children}
			</TooltipContent>
		</Tooltip>
	);
}