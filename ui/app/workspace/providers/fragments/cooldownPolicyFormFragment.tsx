import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage, setProviderFormDirtyState, useAppDispatch } from "@/lib/store";
import { useUpdateProviderMutation } from "@/lib/store/apis/providersApi";
import { useGetProviderKeysQuery } from "@/lib/store/apis/providersApi";
import {
	cooldownPolicySchema,
	type CooldownPolicyFormSchema,
	type CooldownPolicyMatchFormSchema,
	type CooldownPolicyRuleFormSchema,
} from "@/lib/types/schemas";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { zodResolver } from "@hookform/resolvers/zod";
import { HelpCircle, PencilIcon, PlusIcon, Trash2Icon, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useFieldArray, useForm, useFormContext, useWatch, type Control, type Resolver } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { buildProviderUpdatePayload } from "../views/utils";
import type { CooldownPolicy, ModelProvider } from "@/lib/types/config";
import type { CooldownCapacityConfig } from "@/lib/types/config";
import type { LearnedLimitEntry } from "@/lib/types/plugins";
import { useGetProviderErrorCatalogQuery } from "@/lib/store/apis/providerErrorCatalogApi";
import { ErrorSampleBrowser } from "./errorSampleBrowser";
import { LearnedLimitsPanel } from "./learnedLimitsPanel";
import type { ErrorPattern } from "@/lib/store/apis/errorPatternsApi";
import {
	cooldownCapabilityMode,
	resolveEffectiveCapabilities,
	type CooldownCapabilityKey,
	type CooldownCapabilityMode,
} from "@/components/providerCooldown/defaults";
import type { CooldownWindowKey } from "@/components/providerCooldown/cooldownWindow";
import { useCooldownCapabilityDefaults } from "@/components/providerCooldown/useCooldownCapabilityDefaults";
import { CapabilityOverrideCard } from "@/components/providerCooldown/capabilityOverrideCard";
import { EscalationFields } from "@/components/providerCooldown/escalationFields";
import { CapacityFields } from "@/components/providerCooldown/capacityFields";
import { LearningFields } from "@/components/providerCooldown/learningFields";
import { limiterDimensionOfMatch, type LimiterDimension } from "@/lib/utils/limiterDimension";

interface CooldownPolicyFormFragmentProps {
	provider: ModelProvider;
	onCancel?: () => void;
	/**
	 * Observation window owned by the embedding tab. When present, the error
	 * browser renders it instead of keeping a private selector, so the page
	 * never shows two error lists computed over different windows.
	 */
	window?: CooldownWindowKey;
}

const RULE_FIELDS = ["rate_limit", "quota"] as const;
type RuleField = (typeof RULE_FIELDS)[number];

// Every top-level key the form knows about. Anything else found on the stored
// policy (e.g. a section added by a newer backend) is carried over untouched,
// and any of THESE keys that resolves to undefined is deleted from the payload
// rather than sent — `JSON.stringify` would drop it anyway, but the spread of
// the stored policy keeps the old value, which is exactly how the previous
// implementation wiped sections it didn't know about.
//
// `health` is deliberately NOT in this list: the form has no editor for it, so
// leaving it out of both the overwrite set and the delete loop is what makes a
// stored health section survive an unrelated save.
const COOLDOWN_POLICY_KEYS = ["rate_limit", "quota", "capacity", "escalation", "learning"] as const;

type CapabilityModes = Record<CooldownCapabilityKey, CooldownCapabilityMode>;

const DEFAULT_RATE_LIMIT_RULE = (ttlSeconds: number): NonNullable<CooldownPolicyFormSchema["rate_limit"]> => ({
	match: [{ status_code: 429 }],
	match_mode: "any",
	ttl_seconds: ttlSeconds,
});

// DEFAULT_QUOTA_RULE must NOT be a bare {status_code: 429}: quota is evaluated
// before rate_limit, so a bare 429 there claims every transient rate-limit
// error and makes the rate_limit rule dead configuration. The backend rejects
// such a policy (see schemas.CooldownPolicy.Validate), so the UI seeds a
// message-scoped quota signal instead. It deliberately does NOT match on
// code=insufficient_quota: some providers (e.g. SenseNova) reuse that code for
// a short, self-healing rate window. TTL matches the backend quota default
// (600s) because genuine quota exhaustion is not self-healing.
const DEFAULT_QUOTA_RULE = (ttlSeconds: number): NonNullable<CooldownPolicyFormSchema["quota"]> => ({
	match: [{ status_code: 429, message_contains: ["insufficient_quota", "quota exceeded", "entitlement exhausted"] }],
	match_mode: "any",
	ttl_seconds: ttlSeconds,
});

const defaultRuleFor = (rule: RuleField) => (rule === "quota" ? DEFAULT_QUOTA_RULE(600) : DEFAULT_RATE_LIMIT_RULE(60));

const STATUS_CODE_MIN = 100;
const STATUS_CODE_MAX = 599;

function isValidStatusCode(value: unknown): value is number {
	return typeof value === "number" && Number.isInteger(value) && value >= STATUS_CODE_MIN && value <= STATUS_CODE_MAX;
}

const modesFromValues = (values: CooldownPolicyFormSchema): CapabilityModes => ({
	escalation: cooldownCapabilityMode(values.escalation),
	capacity: cooldownCapabilityMode(values.capacity),
	learning: cooldownCapabilityMode(values.learning),
});

function formatInt(value: number): string {
	return value.toLocaleString("en-US");
}

export function CooldownPolicyFormFragment({ provider, onCancel, window }: CooldownPolicyFormFragmentProps) {
	const { t } = useTranslation("providers");
	const { t: tc } = useTranslation("common");
	const dispatch = useAppDispatch();
	const hasUpdateProviderAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const [updateProvider, { isLoading: isUpdatingProvider }] = useUpdateProviderMutation();
	// The provider payload does not embed its keys; the per-key capacity editor
	// needs their names, which is what capacity.key_limits is keyed by.
	const { data: providerKeys = [] } = useGetProviderKeysQuery(String(provider.name));
	const pluginDefaults = useCooldownCapabilityDefaults();

	const buildDefaultValues = () => {
		const policy = provider.cooldown_policy;
		// No custom policy → show the built-in default rate_limit rule (enabled).
		// Explicit policy (even empty) → reflect exactly what's stored, so a
		// stored `{}` (no cooldown rules) shows both rate_limit AND quota OFF
		// instead of re-injecting the default rate_limit rule.
		//
		// The capability sections are NEVER defaulted here: `undefined` is the
		// wire-level signal for "inherit the plugin default". Injecting values
		// would silently turn an inherit into an override on the next save.
		const hasPolicy = policy != null;
		return {
			rate_limit: hasPolicy ? policy?.rate_limit : DEFAULT_RATE_LIMIT_RULE(60),
			quota: policy?.quota ?? undefined,
			capacity: policy?.capacity,
			escalation: policy?.escalation,
			learning: policy?.learning,
		} as CooldownPolicyFormSchema;
	};

	const form = useForm<CooldownPolicyFormSchema, any, CooldownPolicyFormSchema>({
		resolver: zodResolver(cooldownPolicySchema) as Resolver<CooldownPolicyFormSchema, any, CooldownPolicyFormSchema>,
		mode: "onChange",
		reValidateMode: "onChange",
		defaultValues: buildDefaultValues(),
	});

	const initialValuesRef = useRef<CooldownPolicyFormSchema>(buildDefaultValues());
	const [modes, setModes] = useState<CapabilityModes>(() => modesFromValues(buildDefaultValues()));
	// All capability sections start collapsed — the tab already carries two
	// large blocks above (usage + water level) and these three are opt-in.
	const [openSections, setOpenSections] = useState<string[]>([]);

	const watchedFormValues = useWatch({ control: form.control }) as CooldownPolicyFormSchema;

	const isFormChanged = useMemo(() => JSON.stringify(watchedFormValues) !== JSON.stringify(initialValuesRef.current), [watchedFormValues]);

	const effective = useMemo(
		() => resolveEffectiveCapabilities(provider.cooldown_policy, pluginDefaults),
		[provider.cooldown_policy, pluginDefaults],
	);

	useEffect(() => {
		dispatch(setProviderFormDirtyState(isFormChanged));
	}, [isFormChanged, dispatch]);

	useEffect(() => {
		const next = buildDefaultValues();
		initialValuesRef.current = next;
		setModes(modesFromValues(next));
		form.reset(next);
	}, [form, provider.name, provider.cooldown_policy]);

	/**
	 * Builds the submitted policy. Starts from the stored policy so unknown
	 * keys survive, then overwrites the five keys the form owns and deletes any
	 * of them that is `undefined` (i.e. "inherit" / "not configured").
	 */
	const buildCooldownPolicy = (data: CooldownPolicyFormSchema): CooldownPolicy => {
		const policy: Record<string, unknown> = {
			...(provider.cooldown_policy ?? {}),
			rate_limit: data.rate_limit,
			quota: data.quota,
			capacity: data.capacity,
			escalation: data.escalation,
			learning: data.learning,
		};
		for (const key of COOLDOWN_POLICY_KEYS) {
			if (policy[key] === undefined) delete policy[key];
		}
		// Per-key limits only exist under the "key" scope; sending them under
		// another scope would make the backend reject the whole policy. The form
		// keeps the values so switching back restores them.
		const capacity = policy.capacity as Record<string, unknown> | undefined;
		if (capacity && typeof capacity === "object") {
			const scope = capacity.scope ?? effective.capacity.scope;
			if (scope !== "key") {
				const cleaned = { ...capacity };
				delete cleaned.key_limits;
				policy.capacity = cleaned;
			}
		}
		// The dimension TTLs (rpm_ttl_seconds / tpm_ttl_seconds) select a
		// cooldown for a RATE-LIMIT dimension, so the backend rejects them on a
		// quota rule. The form only offers them on the rate_limit rule, but a
		// value stored on the quota rule through config.json would otherwise be
		// resubmitted verbatim and make every save fail. Strip them there.
		const quota = policy.quota as Record<string, unknown> | undefined;
		if (quota && typeof quota === "object") {
			const cleaned = { ...quota };
			delete cleaned.rpm_ttl_seconds;
			delete cleaned.tpm_ttl_seconds;
			policy.quota = cleaned;
		}
		return policy as unknown as CooldownPolicy;
	};

	const onCapabilityModeChange = (key: CooldownCapabilityKey, next: CooldownCapabilityMode) => {
		setModes((prev) => ({ ...prev, [key]: next }));
		const options = { shouldDirty: true, shouldValidate: true };
		if (next === "inherit") {
			form.setValue(key, undefined as never, options);
			return;
		}
		if (next === "disabled") {
			form.setValue(key, { enabled: false } as never, options);
			return;
		}
		// Custom starts from what the provider actually gets today, so the
		// operator edits a meaningful baseline instead of empty inputs.
		form.setValue(key, { ...effective[key], enabled: true } as never, options);
	};

	const onAdoptLearnedLimit = (limit: LearnedLimitEntry) => {
		const token = limit.estimatedTokenLimit;
		const request = limit.estimatedRequestLimit;
		if (token === null && request === null) return;
		// Never mix learning levels. A key_model estimate is one credential's
		// budget; writing it into a model-wide gate (or the reverse) is the
		// "N times the budget" mistake the scope field exists to prevent.
		const expectedScope = limit.scope === "key_model" ? "key" : "provider_model";
		if (capacityScope !== expectedScope) return;
		const current = form.getValues("capacity");
		const base = current ?? { ...effective.capacity, enabled: true };
		const next = { ...base, enabled: true };
		if (token !== null) next.token_limit = token;
		if (request !== null) next.request_limit = request;
		form.setValue("capacity", next, { shouldDirty: true, shouldValidate: true });
		setModes((prev) => ({ ...prev, capacity: "custom" }));
		setOpenSections((prev) => (prev.includes("capacity") ? prev : [...prev, "capacity"]));
		void form.trigger("capacity");
	};

	const onSubmit = (data: CooldownPolicyFormSchema) => {
		updateProvider(
			buildProviderUpdatePayload(provider, {
				cooldown_policy: buildCooldownPolicy(data),
			}),
		)
			.unwrap()
			.then(() => {
				toast.success(t("fragments.cooldownPolicy.toast.updated"));
				initialValuesRef.current = JSON.parse(JSON.stringify(data)) as CooldownPolicyFormSchema;
				setModes(modesFromValues(data));
				form.reset(data);
			})
			.catch((err: unknown) => {
				toast.error(t("fragments.cooldownPolicy.toast.failedToUpdate"), {
					description: getErrorMessage(err),
				});
			});
	};

	const onRestoreDefault = () => {
		updateProvider(
			buildProviderUpdatePayload(provider, {
				cooldown_policy: null,
			}),
		)
			.unwrap()
			.then(() => {
				toast.success(t("fragments.cooldownPolicy.toast.cleared"));
				// Provider props still carry the old policy until the update
				// invalidates the tag, so reset to the "no custom policy" shape
				// explicitly instead of re-deriving it from `provider`.
				const resetValues = {
					rate_limit: DEFAULT_RATE_LIMIT_RULE(60),
					quota: undefined,
					capacity: undefined,
					escalation: undefined,
					learning: undefined,
				} as CooldownPolicyFormSchema;
				initialValuesRef.current = JSON.parse(JSON.stringify(resetValues)) as CooldownPolicyFormSchema;
				setModes(modesFromValues(resetValues));
				form.reset(resetValues);
			})
			.catch((err: unknown) => {
				toast.error(t("fragments.cooldownPolicy.toast.failedToUpdate"), {
					description: getErrorMessage(err),
				});
			});
	};

	const noLimitLabel = tc("cooldownCapability.capacity.noLimit");
	const escalationSummary = effective.escalation.enabled
		? tc("cooldownCapability.escalation.effectiveSummary", { multiplier: effective.escalation.max_multiplier })
		: tc("cooldownCapability.summaryDisabled");
	const capacitySummary = effective.capacity.enabled
		? tc("cooldownCapability.capacity.effectiveSummary", {
				tokens: effective.capacity.token_limit > 0 ? formatInt(effective.capacity.token_limit) : noLimitLabel,
				requests: effective.capacity.request_limit > 0 ? formatInt(effective.capacity.request_limit) : noLimitLabel,
				window: effective.capacity.window_seconds,
				headroom: effective.capacity.headroom,
				scope: tc(`cooldownCapability.capacity.scope.${effective.capacity.scope}`),
				action: tc(`cooldownCapability.capacity.action.${effective.capacity.action}`),
			})
		: tc("cooldownCapability.summaryDisabled");
	const learningSummary = effective.learning.enabled
		? tc("cooldownCapability.learning.effectiveSummary", {
				window: effective.learning.window_seconds,
				min: effective.learning.min_samples,
				max: effective.learning.max_samples,
			})
		: tc("cooldownCapability.summaryDisabled");
	const capabilitySummaries: Record<CooldownCapabilityKey, string> = {
		escalation: escalationSummary,
		capacity: capacitySummary,
		learning: learningSummary,
	};

	const updateDisabled = !hasUpdateProviderAccess || isUpdatingProvider;
	// Both are read from the watched form state (not getValues) so the learning
	// panel re-filters as soon as the operator changes the gate's scope/window.
	const capacityWindowSeconds = watchedFormValues.capacity?.window_seconds ?? effective.capacity.window_seconds;
	const capacityScope = (watchedFormValues.capacity?.scope ?? effective.capacity.scope ?? "provider") as NonNullable<
		CooldownCapacityConfig["scope"]
	>;

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6 px-6 py-5" data-testid="provider-config-cooldown-content">
				{/* The error shapes are the INPUT of the rule editor below, so they
				    sit directly above it instead of in a separate column: the
				    operator reads an error and writes the matching rule without
				    scrolling away from either. */}
				<div className="bg-muted/40 rounded-md border p-3" data-testid="error-sample-browser">
					<h5 className="mb-1 text-sm font-medium">{t("fragments.cooldownPolicy.errorSample.title")}</h5>
					<p className="text-muted-foreground mb-2 text-xs">{t("fragments.cooldownPolicy.errorSample.description")}</p>
					<ErrorSampleBrowserBridge provider={provider.name} window={window} />
				</div>

				<div className="space-y-4">
					<InnerFields provider={String(provider.name)} />
				</div>

				{/* Three per-provider capability overrides — inherit / custom / off. */}
				<div data-testid="providercooldown-capabilities" className="rounded-md border px-3">
					<Accordion type="multiple" value={openSections} onValueChange={setOpenSections}>
						<AccordionItem value="escalation">
							<AccordionTrigger data-testid="providercooldown-escalation-accordion-trigger">
								<span className="flex items-center gap-2 text-sm font-medium">
									{tc("cooldownCapability.escalation.title")}
									<span className="text-muted-foreground text-xs font-normal">{tc(`cooldownCapability.mode.${modes.escalation}`)}</span>
								</span>
							</AccordionTrigger>
							<AccordionContent className="pt-0">
								<CapabilityOverrideCard
									testId="providercooldown-escalation-card"
									title={tc("cooldownCapability.escalation.title")}
									description={tc("cooldownCapability.escalation.description")}
									value={modes.escalation}
									onChange={(next) => onCapabilityModeChange("escalation", next)}
									inheritedSummary={capabilitySummaries.escalation}
									disabled={updateDisabled}
								>
									<EscalationFields
										value={form.watch("escalation") ?? {}}
										onChange={(next) => form.setValue("escalation", next, { shouldDirty: true, shouldValidate: true })}
										disabled={updateDisabled}
									/>
								</CapabilityOverrideCard>
							</AccordionContent>
						</AccordionItem>

						<AccordionItem value="capacity">
							<AccordionTrigger data-testid="providercooldown-capacity-accordion-trigger">
								<span className="flex items-center gap-2 text-sm font-medium">
									{tc("cooldownCapability.capacity.title")}
									<span className="text-muted-foreground text-xs font-normal">{tc(`cooldownCapability.mode.${modes.capacity}`)}</span>
								</span>
							</AccordionTrigger>
							<AccordionContent className="pt-0">
								<CapabilityOverrideCard
									testId="providercooldown-capacity-card"
									title={tc("cooldownCapability.capacity.title")}
									description={tc("cooldownCapability.capacity.description")}
									value={modes.capacity}
									onChange={(next) => onCapabilityModeChange("capacity", next)}
									inheritedSummary={capabilitySummaries.capacity}
									disabled={updateDisabled}
								>
									<CapacityFields
										value={form.watch("capacity") ?? {}}
										onChange={(next) => form.setValue("capacity", next, { shouldDirty: true, shouldValidate: true })}
										disabled={updateDisabled}
										keys={providerKeys}
									/>
								</CapabilityOverrideCard>
							</AccordionContent>
						</AccordionItem>

						<AccordionItem value="learning">
							<AccordionTrigger data-testid="providercooldown-learning-accordion-trigger">
								<span className="flex items-center gap-2 text-sm font-medium">
									{tc("cooldownCapability.learning.title")}
									<span className="text-muted-foreground text-xs font-normal">{tc(`cooldownCapability.mode.${modes.learning}`)}</span>
								</span>
							</AccordionTrigger>
							<AccordionContent className="pt-0">
								<CapabilityOverrideCard
									testId="providercooldown-learning-card"
									title={tc("cooldownCapability.learning.title")}
									description={tc("cooldownCapability.learning.description")}
									value={modes.learning}
									onChange={(next) => onCapabilityModeChange("learning", next)}
									inheritedSummary={capabilitySummaries.learning}
									disabled={updateDisabled}
									panel={
										<LearnedLimitsPanel
											provider={String(provider.name)}
											capacityWindowSeconds={capacityWindowSeconds}
											capacityScope={capacityScope}
											onAdopt={onAdoptLearnedLimit}
											disabled={updateDisabled}
										/>
									}
								>
									<LearningFields
										value={form.watch("learning") ?? {}}
										onChange={(next) => form.setValue("learning", next, { shouldDirty: true, shouldValidate: true })}
										disabled={updateDisabled}
									/>
								</CapabilityOverrideCard>
							</AccordionContent>
						</AccordionItem>
					</Accordion>
				</div>

				<div className="flex flex-wrap items-center justify-between gap-2 pb-6">
					<div className="flex flex-wrap items-center gap-2">
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={onRestoreDefault}
							disabled={updateDisabled}
							data-testid="provider-cooldown-clear-button"
						>
							{t("fragments.cooldownPolicy.restoreDefault")}
						</Button>
						<span className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.restoreDefaultHint")}</span>
					</div>
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
// Error sample browser — clicking "apply" opens a popup to pick the target
// rule (rate_limit or quota).
// ---------------------------------------------------------------------------

function ErrorSampleBrowserBridge({ provider, window }: { provider: ModelProvider["name"]; window?: CooldownWindowKey }) {
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
			if (isValidStatusCode(pattern.status_code)) {
				newMatch.status_code = pattern.status_code;
			} else {
				toast.warning(t("fragments.cooldownPolicy.errorSample.toast.skippedInvalidStatusCode"), {
					description: String(pattern.status_code),
				});
			}
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
			setValue(targetRule, defaultRuleFor(targetRule), { shouldDirty: true, shouldValidate: true });
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
			<ErrorSampleBrowser provider={String(provider)} onApply={handleApply} window={window} />

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
				const rule = watch(ruleKey);
				const enabled = rule !== undefined && rule.enabled !== false;
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
											const base = current ?? saved ?? defaultRuleFor(ruleKey);
											setValue(ruleKey, { ...base, enabled: true } as never, {
												shouldDirty: true,
												shouldValidate: true,
											});
											delete savedRules.current[ruleKey];
										} else {
											const current = getValues(ruleKey);
											if (current) {
												savedRules.current[ruleKey] = JSON.parse(JSON.stringify(current)) as CooldownPolicyRuleFormSchema;
												setValue(ruleKey, { ...current, enabled: false } as never, { shouldDirty: true, shouldValidate: true });
											} else {
												setValue(ruleKey, undefined as never, { shouldDirty: true, shouldValidate: true });
											}
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
	const { trigger } = useFormContext<CooldownPolicyFormSchema>();
	const matchName = `${ruleKey}.match` as const;
	const { fields, append, remove } = useFieldArray({ control, name: matchName });
	// The dimension inputs fall back to the plain TTL, so the placeholder shows
	// the value that will actually be used when the field stays blank.
	const ttlSeconds = useWatch({ control, name: `${ruleKey}.ttl_seconds` });
	const ttlPlaceholder = String(ttlSeconds ?? "");

	return (
		<>
			<div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
				<FormField
					control={control}
					name={`${ruleKey}.match_mode`}
					render={({ field }) => (
						<FormItem>
							<FormLabel className="text-xs">{t("fragments.cooldownPolicy.matchModeLabel")}</FormLabel>
							<FormControl>
								<Select value={(field.value ?? "any") as string} onValueChange={field.onChange}>
									<SelectTrigger className="w-full" data-testid={`provider-cooldown-${ruleKey}-match-mode-trigger`}>
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
					name={`${ruleKey}.scope`}
					render={({ field }) => (
						<FormItem>
							<FormLabel className="text-xs">{t("fragments.cooldownPolicy.scope.label")}</FormLabel>
							<FormControl>
								<Select value={field.value ?? "key"} onValueChange={field.onChange}>
									<SelectTrigger className="w-full" data-testid={`provider-cooldown-${ruleKey}-scope-trigger`}>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="key">{t("fragments.cooldownPolicy.scope.key")}</SelectItem>
										<SelectItem value="model">{t("fragments.cooldownPolicy.scope.model")}</SelectItem>
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

				<FormField
					control={control}
					name={`${ruleKey}.retry_after_mode`}
					render={({ field }) => (
						<FormItem>
							<FormLabel className="flex items-center gap-1.5">
								{t("fragments.cooldownPolicy.retryAfterMode.label")}
								<HelpHint>{t("fragments.cooldownPolicy.retryAfterMode.description")}</HelpHint>
							</FormLabel>
							<FormControl>
								<Select value={(field.value ?? "off") as string} onValueChange={field.onChange}>
									<SelectTrigger className="w-full" data-testid={`provider-cooldown-${ruleKey}-retry-after-mode-trigger`}>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="off">{t("fragments.cooldownPolicy.retryAfterMode.off")}</SelectItem>
										<SelectItem value="cap">{t("fragments.cooldownPolicy.retryAfterMode.cap")}</SelectItem>
										<SelectItem value="exact">{t("fragments.cooldownPolicy.retryAfterMode.exact")}</SelectItem>
									</SelectContent>
								</Select>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>
			</div>

			{ruleKey === "rate_limit" && (
				<div className="grid grid-cols-2 gap-3 lg:grid-cols-4" data-testid="provider-cooldown-rate-limit-dimension-fields">
					<FormField
						control={control}
						name={`${ruleKey}.rpm_ttl_seconds`}
						render={({ field }) => (
							<FormItem>
								<FormLabel className="flex items-center gap-1.5">
									{t("fragments.cooldownPolicy.rpmTtlLabel")}
									<HelpHint>{t("fragments.cooldownPolicy.rpmTtlDescription")}</HelpHint>
								</FormLabel>
								<FormControl>
									<Input
										data-testid={`provider-cooldown-${ruleKey}-rpm-ttl-input`}
										type="number"
										min={0}
										max={86400}
										value={field.value ?? ""}
										placeholder={ttlPlaceholder}
										onChange={(e) => field.onChange(e.target.value === "" ? undefined : Number(e.target.value) || 0)}
									/>
								</FormControl>
								<FormMessage />
							</FormItem>
						)}
					/>
					<FormField
						control={control}
						name={`${ruleKey}.tpm_ttl_seconds`}
						render={({ field }) => (
							<FormItem>
								<FormLabel className="flex items-center gap-1.5">
									{t("fragments.cooldownPolicy.tpmTtlLabel")}
									<HelpHint>{t("fragments.cooldownPolicy.tpmTtlDescription")}</HelpHint>
								</FormLabel>
								<FormControl>
									<Input
										data-testid={`provider-cooldown-${ruleKey}-tpm-ttl-input`}
										type="number"
										min={0}
										max={86400}
										value={field.value ?? ""}
										placeholder={ttlPlaceholder}
										onChange={(e) => field.onChange(e.target.value === "" ? undefined : Number(e.target.value) || 0)}
									/>
								</FormControl>
								<FormMessage />
							</FormItem>
						)}
					/>
				</div>
			)}

			<div className="space-y-2">
				<div className="flex items-center justify-between">
					<span className="text-xs font-medium">{t("fragments.cooldownPolicy.matchesLabel")}</span>
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={() => {
							append({ status_code: 429 });
							trigger(matchName);
						}}
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
						onRemove={() => {
							remove(index);
							trigger(matchName);
						}}
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
				{ruleKey === "rate_limit" && match?.dimension && (
					<span
						className="text-muted-foreground shrink-0 rounded border px-1 font-mono text-[10px]"
						data-testid={`provider-cooldown-${ruleKey}-match-${index}-dimension-badge`}
					>
						{t(`fragments.cooldownPolicy.matchField.dimensionValue_${match.dimension}`)}
					</span>
				)}
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
	const { setValue, watch } = useFormContext<CooldownPolicyFormSchema>();
	const fieldName = `${baseName}.status_code` as const;
	const watchedStatusCode = watch(fieldName as never) as unknown as number | undefined;
	// Local mirror of the field value so the controlled input always reflects
	// the latest form state, even when RHF's Controller fails to propagate a
	// numeric → undefined transition through its `field` prop (observed bug).
	// The mirror is a string so we can preserve the user's in-progress typing
	// (e.g. "5" while they're still typing "500") without immediately
	// collapsing the input to empty just because an intermediate digit is
	// outside the [100, 599] band.
	const [statusCodeText, setStatusCodeText] = useState<string>(watchedStatusCode === undefined ? "" : String(watchedStatusCode));
	const inputRef = useRef<HTMLInputElement | null>(null);
	useEffect(() => {
		setStatusCodeText(watchedStatusCode === undefined ? "" : String(watchedStatusCode));
	}, [watchedStatusCode]);

	const clearStatusCode = () => {
		setStatusCodeText("");
		setValue(fieldName, undefined as never, { shouldDirty: true, shouldValidate: true });
		if (inputRef.current) inputRef.current.value = "";
	};

	return (
		<div className="space-y-3">
			<FormField
				control={control}
				name={fieldName}
				render={() => (
					<FormItem>
						<FormLabel className="text-xs">{t("fragments.cooldownPolicy.matchField.statusCode")}</FormLabel>
						<FormControl>
							<div className="flex items-center gap-1">
								<Input
									ref={inputRef}
									type="number"
									min={STATUS_CODE_MIN}
									max={STATUS_CODE_MAX}
									placeholder={t("fragments.cooldownPolicy.matchField.statusCodePlaceholder")}
									data-testid={`provider-cooldown-${ruleKey}-match-${index}-status-code`}
									value={statusCodeText}
									onChange={(e) => {
										const v = e.target.value;
										setStatusCodeText(v);
										if (v === "") {
											setValue(fieldName, undefined as never, {
												shouldDirty: true,
												shouldValidate: true,
											});
											return;
										}
										const n = Number(v);
										if (isValidStatusCode(n)) {
											setValue(fieldName, n as never, {
												shouldDirty: true,
												shouldValidate: true,
											});
										}
										// Intermediate digits outside [100, 599] (e.g. typing "5"
										// before "500") are kept in the local mirror but not
										// written to RHF, so the schema isn't violated mid-type.
									}}
								/>
								{statusCodeText !== "" && (
									<Button
										type="button"
										variant="ghost"
										size="sm"
										onClick={clearStatusCode}
										data-testid={`provider-cooldown-${ruleKey}-match-${index}-status-code-clear`}
										aria-label={t("fragments.cooldownPolicy.matchField.clearStatusCode")}
									>
										<X className="h-3.5 w-3.5" />
									</Button>
								)}
							</div>
						</FormControl>
						<FormMessage />
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

			{/* Last by design: the dimension governs which TTL the match cools
			    with, so it is filled in once the predicate is known — and 自动
			    echoes what the classifier would decide for that predicate. */}
			{ruleKey === "rate_limit" && <DimensionField ruleKey={ruleKey} index={index} control={control} />}
		</div>
	);
}

/**
 * Which upstream budget the match cools.
 *
 * "自动" (the default) leaves the decision to the backend classifier, which
 * reads the provider error body at mark time. Pinning is for matches the
 * operator KNOWS target one budget: not every provider names the exhausted
 * limit in its 429, and a pinned predicate (e.g.
 * code=EndpointRPMExceeded) must not be overridden by a generic catch-all in
 * the same rule.
 *
 * While 自动 is selected the field echoes what the classifier would decide for
 * this match's own predicates, so the operator can see the effect without
 * sending a request.
 */
function DimensionField({ ruleKey, index, control }: { ruleKey: RuleField; index: number; control: Control<CooldownPolicyFormSchema> }) {
	const { t } = useTranslation("providers");
	const { setValue, watch } = useFormContext<CooldownPolicyFormSchema>();
	const baseName = `${ruleKey}.match.${index}` as const;

	const pinned = watch(`${baseName}.dimension` as never) as unknown as "rpm" | "tpm" | undefined;
	const code = watch(`${baseName}.code` as never) as unknown as string[] | undefined;
	const type = watch(`${baseName}.type` as never) as unknown as string[] | undefined;
	const messageContains = watch(`${baseName}.message_contains` as never) as unknown as string[] | undefined;
	const rpmTTL = watch(`${ruleKey}.rpm_ttl_seconds` as never) as unknown as number | undefined;
	const tpmTTL = watch(`${ruleKey}.tpm_ttl_seconds` as never) as unknown as number | undefined;

	const selected = pinned ?? "auto";
	const autoDimension = limiterDimensionOfMatch({ code, type, message_contains: messageContains });
	// A pinned dimension without its own TTL still cools for ttl_seconds — say
	// so, otherwise the pin looks like it had no effect.
	const pinnedTTLMissing = pinned !== undefined && ((pinned === "rpm" ? rpmTTL : tpmTTL) ?? 0) <= 0;

	return (
		<div className="space-y-1">
			<div className="flex items-center gap-1.5">
				<span className="text-xs font-medium">{t("fragments.cooldownPolicy.matchField.dimension")}</span>
				<HelpHint>{t("fragments.cooldownPolicy.matchField.dimensionDescription")}</HelpHint>
			</div>

			<Select
				value={selected}
				onValueChange={(value) =>
					setValue(`${baseName}.dimension` as never, (value === "auto" ? undefined : value) as never, {
						shouldDirty: true,
						shouldValidate: true,
					})
				}
			>
				<SelectTrigger className="w-full" data-testid={`provider-cooldown-${ruleKey}-match-${index}-dimension-trigger`}>
					<SelectValue />
				</SelectTrigger>
				<SelectContent>
					<SelectItem value="auto" data-testid={`provider-cooldown-${ruleKey}-match-${index}-dimension-auto`}>
						{t("fragments.cooldownPolicy.matchField.dimensionAuto")}
					</SelectItem>
					<SelectItem value="rpm" data-testid={`provider-cooldown-${ruleKey}-match-${index}-dimension-rpm`}>
						{t("fragments.cooldownPolicy.matchField.dimensionRPM")}
					</SelectItem>
					<SelectItem value="tpm" data-testid={`provider-cooldown-${ruleKey}-match-${index}-dimension-tpm`}>
						{t("fragments.cooldownPolicy.matchField.dimensionTPM")}
					</SelectItem>
				</SelectContent>
			</Select>

			{selected === "auto" && (
				<p className="text-muted-foreground text-xs" data-testid={`provider-cooldown-${ruleKey}-match-${index}-dimension-preview`}>
					{t("fragments.cooldownPolicy.matchField.dimensionAutoPreview", {
						dimension: t(`fragments.cooldownPolicy.matchField.dimensionValue_${autoDimension}`),
					})}
				</p>
			)}

			{pinnedTTLMissing && (
				<p
					className="text-xs text-amber-600 dark:text-amber-500"
					data-testid={`provider-cooldown-${ruleKey}-match-${index}-dimension-no-ttl`}
				>
					{t("fragments.cooldownPolicy.matchField.dimensionNoTTL", {
						field: t(pinned === "rpm" ? "fragments.cooldownPolicy.rpmTtlLabel" : "fragments.cooldownPolicy.tpmTtlLabel"),
					})}
				</p>
			)}
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
	const { trigger } = useFormContext<CooldownPolicyFormSchema>();
	const { fields, append, remove } = useFieldArray({ control, name: baseName as never });

	return (
		<div className="space-y-1">
			<div className="flex items-center justify-between">
				<span className="text-xs font-medium">{label}</span>
				<Button
					type="button"
					variant="outline"
					size="sm"
					onClick={() => {
						append("");
						trigger(baseName as never);
					}}
					data-testid={`${testId}-add`}
				>
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
					<Button
						type="button"
						variant="ghost"
						size="sm"
						onClick={() => {
							remove(i);
							trigger(baseName as never);
						}}
						data-testid={`${testId}-${i}-remove`}
					>
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
	const { control, getValues, setValue, trigger } = useFormContext<CooldownPolicyFormSchema>();
	const { data: catalog } = useGetProviderErrorCatalogQuery(provider, { skip: !provider });
	const options = catalog?.[fieldKind === "type" ? "types" : "codes"] ?? [];
	const { fields, append, remove } = useFieldArray({
		control,
		name: baseName as never,
	});

	const revalidate = () => {
		trigger(baseName as never);
	};

	return (
		<div className="space-y-1">
			<div className="flex items-center justify-between">
				<span className="text-xs font-medium">{label}</span>
				<Button
					type="button"
					variant="outline"
					size="sm"
					onClick={() => {
						append("" as never);
						revalidate();
					}}
					data-testid={`cooldown-${baseName}-add`}
				>
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
									setValue(baseName as never, next as never, { shouldDirty: true, shouldValidate: true });
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
						<Button
							type="button"
							variant="ghost"
							size="sm"
							onClick={() => {
								remove(i);
								revalidate();
							}}
							data-testid={`cooldown-${fieldKind}-remove-${i}`}
						>
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