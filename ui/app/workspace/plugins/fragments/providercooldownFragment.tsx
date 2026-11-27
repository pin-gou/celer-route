import { Button } from "@/components/ui/button";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { ProviderLabels } from "@/lib/constants/logs";
import { PROVIDER_COOLDOWN_PLUGIN, type Plugin } from "@/lib/types/plugins";
import { providerCooldownPluginConfigFormSchema, type ProviderCooldownPluginConfigFormSchema } from "@/lib/types/schemas";
import { formatLocalDateTime, formatRelativeDistanceToNow, getDateLocale } from "@/lib/utils/date";
import type { CooldownPolicy, CooldownPolicyRule } from "@/lib/types/config";
import type { CooldownStats, CooldownStateEntry, ProviderKindCounters } from "@/lib/types/plugins";
import type { Locale } from "date-fns";
import { Info } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { toast } from "sonner";
import { useGetCooldownStateQuery, useGetCooldownStatsQuery, useUnfreezeCooldownMutation } from "@/lib/store/apis/pluginsApi";
import { useNavigate } from "@tanstack/react-router";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm, type Resolver } from "react-hook-form";

// ---------------------------------------------------------------------------
// EnabledSwitch — top-level enabled toggle with RBAC gating
// ---------------------------------------------------------------------------

export function EnabledSwitch({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const handleToggle = async (checked: boolean) => {
		if (!hasUpdateAccess) return;
		try {
			await updatePlugin({
				name: PROVIDER_COOLDOWN_PLUGIN,
				data: { enabled: checked },
			}).unwrap();
			toast.success(checked ? t("providerCooldown.enabledToast") : t("providerCooldown.disabledToast"));
		} catch {
			toast.error(t("providerCooldown.updateFailedToast"));
		}
	};

	return (
		<div className="rounded-lg border p-4">
			<div className="flex flex-row items-center justify-between">
				<div className="space-y-0.5">
					<label className="text-sm font-medium">{t("providerCooldown.enableTitle")}</label>
					<p className="text-muted-foreground text-sm">{t("providerCooldown.enableDescription")}</p>
				</div>
				<Switch
					data-testid="providercooldown-enabled-switch"
					checked={plugin.enabled}
					onCheckedChange={handleToggle}
					disabled={isLoading || !hasUpdateAccess}
				/>
			</div>

			<div className="bg-muted/50 mt-4 space-y-3 rounded-md border p-4">
				<div className="flex items-center gap-2">
					<Info className="text-muted-foreground size-4 shrink-0" />
					<h4 className="text-sm font-medium">{t("providerCooldown.howItWorks.title")}</h4>
				</div>
				<div>
					<h5 className="text-muted-foreground text-sm font-medium">{t("providerCooldown.howItWorks.triggerTitle")}</h5>
					<p className="text-muted-foreground mt-1 text-sm leading-relaxed">{t("providerCooldown.howItWorks.triggerBody")}</p>
				</div>
				<div>
					<h5 className="text-muted-foreground text-sm font-medium">{t("providerCooldown.howItWorks.effectTitle")}</h5>
					<p className="text-muted-foreground mt-1 text-sm leading-relaxed">{t("providerCooldown.howItWorks.effectBody")}</p>
				</div>
				<div>
					<h5 className="text-muted-foreground text-sm font-medium">{t("providerCooldown.howItWorks.recoveryTitle")}</h5>
					<p className="text-muted-foreground mt-1 text-sm leading-relaxed">{t("providerCooldown.howItWorks.recoveryBody")}</p>
				</div>
			</div>
		</div>
	);
}

// ---------------------------------------------------------------------------
// ConfigForm — the plugin's optional capability sections
// ---------------------------------------------------------------------------
//
// The provider-cooldown plugin grew three opt-in capabilities that used to be
// reachable only through config.json / the raw plugin API:
//
//   - escalation — adaptive TTL upgrade (a key's cooldown grows while it keeps
//     failing)
//   - capacity   — retrospective capacity gate backed by real usage
//   - learning   — read-only learner that estimates the provider's effective
//     per-window limit from upstream rate-limit feedback
//
// All three are disabled by default. They live in the *plugin* config (not in
// the provider-level cooldown_policy) because the provider policy form
// rebuilds the whole cooldown_policy object and would drop them.
//
// A disabled section is still submitted as `{ enabled: false, … }` — omitting
// the key would leave the previously stored value in place on the backend.

/** Backend default for `escalation.max_multiplier` (<= 0 → 4). */
const DEFAULT_ESCALATION_MAX_MULTIPLIER = 4;
/** Backend default for `capacity.window_seconds` (<= 0 → 60). */
const DEFAULT_CAPACITY_WINDOW_SECONDS = 60;
/** Backend default for `capacity.headroom` (<= 0 → 0.9). */
const DEFAULT_CAPACITY_HEADROOM = 0.9;
/** Backend default for `capacity.scope`. */
const DEFAULT_CAPACITY_SCOPE = "provider";
/** Backend default for `capacity.action`. */
const DEFAULT_CAPACITY_ACTION = "fallback";
/** Backend default for `learning.window_seconds` (<= 0 → 60). */
const DEFAULT_LEARNING_WINDOW_SECONDS = 60;
/** Backend default for `learning.max_samples` (<= 0 → 50). */
const DEFAULT_LEARNING_MAX_SAMPLES = 50;
/** Backend default for `learning.min_samples` (<= 0 → 5). */
const DEFAULT_LEARNING_MIN_SAMPLES = 5;

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const pluginConfig = (plugin.config || {}) as Record<string, any>;

	const form = useForm<ProviderCooldownPluginConfigFormSchema, any, ProviderCooldownPluginConfigFormSchema>({
		resolver: zodResolver(providerCooldownPluginConfigFormSchema) as Resolver<
			ProviderCooldownPluginConfigFormSchema,
			any,
			ProviderCooldownPluginConfigFormSchema
		>,
		mode: "onChange",
		reValidateMode: "onChange",
		defaultValues: {
			escalation: {
				enabled: pluginConfig.escalation?.enabled ?? false,
				max_multiplier: pluginConfig.escalation?.max_multiplier ?? DEFAULT_ESCALATION_MAX_MULTIPLIER,
			},
			capacity: {
				enabled: pluginConfig.capacity?.enabled ?? false,
				token_limit: pluginConfig.capacity?.token_limit ?? 0,
				request_limit: pluginConfig.capacity?.request_limit ?? 0,
				window_seconds: pluginConfig.capacity?.window_seconds ?? DEFAULT_CAPACITY_WINDOW_SECONDS,
				headroom: pluginConfig.capacity?.headroom ?? DEFAULT_CAPACITY_HEADROOM,
				scope: pluginConfig.capacity?.scope ?? DEFAULT_CAPACITY_SCOPE,
				action: pluginConfig.capacity?.action ?? DEFAULT_CAPACITY_ACTION,
			},
			learning: {
				enabled: pluginConfig.learning?.enabled ?? false,
				window_seconds: pluginConfig.learning?.window_seconds ?? DEFAULT_LEARNING_WINDOW_SECONDS,
				max_samples: pluginConfig.learning?.max_samples ?? DEFAULT_LEARNING_MAX_SAMPLES,
				min_samples: pluginConfig.learning?.min_samples ?? DEFAULT_LEARNING_MIN_SAMPLES,
			},
		},
	});

	const escalationEnabled = form.watch("escalation.enabled");
	const capacityEnabled = form.watch("capacity.enabled");
	const learningEnabled = form.watch("learning.enabled");

	const onSubmit = async (values: ProviderCooldownPluginConfigFormSchema) => {
		if (!hasUpdateAccess) return;
		// Every other top-level key of the stored config (default_ttl_seconds,
		// ttl_overrides, quota_patterns, …) is carried over untouched — only
		// the three capability sections are rewritten here.
		const currentConfig = (plugin.config || {}) as Record<string, unknown>;
		try {
			await updatePlugin({
				name: PROVIDER_COOLDOWN_PLUGIN,
				data: {
					// `enabled` has to be sent explicitly, otherwise the update
					// would switch the plugin off.
					enabled: plugin.enabled,
					config: {
						...currentConfig,
						escalation: { ...values.escalation },
						capacity: { ...values.capacity },
						learning: { ...values.learning },
					},
				},
			}).unwrap();
			toast.success(t("providerCooldown.configSavedToast"));
			form.reset(values);
		} catch {
			toast.error(t("providerCooldown.configSaveFailedToast"));
		}
	};

	return (
		<Form {...form}>
			<form data-testid="providercooldown-config-form" onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
				{/* ---------------------------------------------------------------- */}
				{/* Section 1: escalation — adaptive TTL upgrade                    */}
				{/* ---------------------------------------------------------------- */}
				<div className="rounded-lg border p-4">
					<FormField
						control={form.control}
						name="escalation.enabled"
						render={({ field }) => (
							<FormItem className="flex flex-row items-center justify-between">
								<div className="space-y-0.5">
									<FormLabel>{t("providerCooldown.escalation.title")}</FormLabel>
									<FormDescription>{t("providerCooldown.escalation.description")}</FormDescription>
								</div>
								<FormControl>
									<Switch
										data-testid="providercooldown-escalation-enabled"
										checked={Boolean(field.value)}
										onCheckedChange={field.onChange}
										disabled={isLoading || !hasUpdateAccess}
									/>
								</FormControl>
							</FormItem>
						)}
					/>

					<div className="bg-muted/50 mt-4 space-y-3 rounded-md border p-4">
						<FormField
							control={form.control}
							name="escalation.max_multiplier"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("providerCooldown.escalation.maxMultiplierLabel")}</FormLabel>
									<FormDescription>{t("providerCooldown.escalation.maxMultiplierDescription")}</FormDescription>
									<FormControl>
										<Input
											type="number"
											min={1}
											max={16}
											step={1}
											data-testid="providercooldown-escalation-max-multiplier"
											placeholder={t("providerCooldown.escalation.maxMultiplierPlaceholder")}
											{...field}
											value={field.value ?? ""}
											disabled={!escalationEnabled || !hasUpdateAccess}
											onChange={(e) => {
												const value = e.target.value;
												if (value === "") {
													field.onChange(undefined);
													return;
												}
												const parsed = Number.parseInt(value);
												if (!Number.isNaN(parsed)) {
													field.onChange(parsed);
												}
											}}
										/>
									</FormControl>
									<FormMessage />
								</FormItem>
							)}
						/>
					</div>
				</div>

				{/* ---------------------------------------------------------------- */}
				{/* Section 2: capacity — retrospective capacity gate              */}
				{/* ---------------------------------------------------------------- */}
				<div className="rounded-lg border p-4">
					<FormField
						control={form.control}
						name="capacity.enabled"
						render={({ field }) => (
							<FormItem className="flex flex-row items-center justify-between">
								<div className="space-y-0.5">
									<FormLabel>{t("providerCooldown.capacity.title")}</FormLabel>
									<FormDescription>{t("providerCooldown.capacity.description")}</FormDescription>
								</div>
								<FormControl>
									<Switch
										data-testid="providercooldown-capacity-enabled"
										checked={Boolean(field.value)}
										onCheckedChange={field.onChange}
										disabled={isLoading || !hasUpdateAccess}
									/>
								</FormControl>
							</FormItem>
						)}
					/>

					<div className="bg-muted/50 mt-4 space-y-3 rounded-md border p-4">
						<div className="flex flex-row gap-4">
							<div className="flex-1">
								<FormField
									control={form.control}
									name="capacity.token_limit"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("providerCooldown.capacity.tokenLimitLabel")}</FormLabel>
											<FormDescription>{t("providerCooldown.capacity.tokenLimitDescription")}</FormDescription>
											<FormControl>
												<Input
													type="number"
													min={0}
													step={1}
													data-testid="providercooldown-capacity-token-limit"
													placeholder={t("providerCooldown.capacity.tokenLimitPlaceholder")}
													{...field}
													value={field.value ?? ""}
													disabled={!capacityEnabled || !hasUpdateAccess}
													onChange={(e) => {
														const value = e.target.value;
														if (value === "") {
															field.onChange(undefined);
															return;
														}
														const parsed = Number.parseInt(value);
														if (!Number.isNaN(parsed)) {
															field.onChange(parsed);
														}
													}}
												/>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
							<div className="flex-1">
								<FormField
									control={form.control}
									name="capacity.request_limit"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("providerCooldown.capacity.requestLimitLabel")}</FormLabel>
											<FormDescription>{t("providerCooldown.capacity.requestLimitDescription")}</FormDescription>
											<FormControl>
												<Input
													type="number"
													min={0}
													step={1}
													data-testid="providercooldown-capacity-request-limit"
													placeholder={t("providerCooldown.capacity.requestLimitPlaceholder")}
													{...field}
													value={field.value ?? ""}
													disabled={!capacityEnabled || !hasUpdateAccess}
													onChange={(e) => {
														const value = e.target.value;
														if (value === "") {
															field.onChange(undefined);
															return;
														}
														const parsed = Number.parseInt(value);
														if (!Number.isNaN(parsed)) {
															field.onChange(parsed);
														}
													}}
												/>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
						</div>

						<div className="flex flex-row gap-4">
							<div className="flex-1">
								<FormField
									control={form.control}
									name="capacity.window_seconds"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("providerCooldown.capacity.windowSecondsLabel")}</FormLabel>
											<FormDescription>{t("providerCooldown.capacity.windowSecondsDescription")}</FormDescription>
											<FormControl>
												<Input
													type="number"
													min={10}
													max={3600}
													step={1}
													data-testid="providercooldown-capacity-window-seconds"
													placeholder={t("providerCooldown.capacity.windowSecondsPlaceholder")}
													{...field}
													value={field.value ?? ""}
													disabled={!capacityEnabled || !hasUpdateAccess}
													onChange={(e) => {
														const value = e.target.value;
														if (value === "") {
															field.onChange(undefined);
															return;
														}
														const parsed = Number.parseInt(value);
														if (!Number.isNaN(parsed)) {
															field.onChange(parsed);
														}
													}}
												/>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
							<div className="flex-1">
								<FormField
									control={form.control}
									name="capacity.headroom"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("providerCooldown.capacity.headroomLabel")}</FormLabel>
											<FormDescription>{t("providerCooldown.capacity.headroomDescription")}</FormDescription>
											<FormControl>
												<Input
													type="number"
													min={0.1}
													max={1}
													step={0.1}
													data-testid="providercooldown-capacity-headroom"
													placeholder={t("providerCooldown.capacity.headroomPlaceholder")}
													{...field}
													value={field.value ?? ""}
													disabled={!capacityEnabled || !hasUpdateAccess}
													onChange={(e) => {
														const value = e.target.value;
														if (value === "") {
															field.onChange(undefined);
															return;
														}
														const parsed = Number.parseFloat(value);
														if (!Number.isNaN(parsed)) {
															field.onChange(parsed);
														}
													}}
												/>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
						</div>

						<div className="flex flex-row gap-4">
							<div className="flex-1">
								<FormField
									control={form.control}
									name="capacity.scope"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("providerCooldown.capacity.scopeLabel")}</FormLabel>
											<FormDescription>{t("providerCooldown.capacity.scopeDescription")}</FormDescription>
											<FormControl>
												<Select value={field.value} onValueChange={field.onChange} disabled={!capacityEnabled || !hasUpdateAccess}>
													<SelectTrigger data-testid="providercooldown-capacity-scope-trigger" className="w-full">
														<SelectValue placeholder={t("providerCooldown.capacity.scopePlaceholder")} />
													</SelectTrigger>
													<SelectContent>
														<SelectItem value="provider">{t("providerCooldown.capacity.scopeProvider")}</SelectItem>
														<SelectItem value="provider_model">{t("providerCooldown.capacity.scopeProviderModel")}</SelectItem>
													</SelectContent>
												</Select>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
							<div className="flex-1">
								<FormField
									control={form.control}
									name="capacity.action"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("providerCooldown.capacity.actionLabel")}</FormLabel>
											<FormDescription>{t("providerCooldown.capacity.actionDescription")}</FormDescription>
											<FormControl>
												<Select value={field.value} onValueChange={field.onChange} disabled={!capacityEnabled || !hasUpdateAccess}>
													<SelectTrigger data-testid="providercooldown-capacity-action-trigger" className="w-full">
														<SelectValue placeholder={t("providerCooldown.capacity.actionPlaceholder")} />
													</SelectTrigger>
													<SelectContent>
														<SelectItem value="fallback">{t("providerCooldown.capacity.actionFallback")}</SelectItem>
														<SelectItem value="reject">{t("providerCooldown.capacity.actionReject")}</SelectItem>
													</SelectContent>
												</Select>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
						</div>
					</div>
				</div>

				{/* ---------------------------------------------------------------- */}
				{/* Section 3: learning — read-only limit learner                    */}
				{/* ---------------------------------------------------------------- */}
				<div className="rounded-lg border p-4">
					<FormField
						control={form.control}
						name="learning.enabled"
						render={({ field }) => (
							<FormItem className="flex flex-row items-center justify-between">
								<div className="space-y-0.5">
									<FormLabel>{t("providerCooldown.learning.title")}</FormLabel>
									<FormDescription>{t("providerCooldown.learning.description")}</FormDescription>
								</div>
								<FormControl>
									<Switch
										data-testid="providercooldown-learning-enabled"
										checked={Boolean(field.value)}
										onCheckedChange={field.onChange}
										disabled={isLoading || !hasUpdateAccess}
									/>
								</FormControl>
							</FormItem>
						)}
					/>

					<div className="bg-muted/50 mt-4 space-y-3 rounded-md border p-4">
						<div className="flex flex-row gap-4">
							<div className="flex-1">
								<FormField
									control={form.control}
									name="learning.window_seconds"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("providerCooldown.learning.windowSecondsLabel")}</FormLabel>
											<FormDescription>{t("providerCooldown.learning.windowSecondsDescription")}</FormDescription>
											<FormControl>
												<Input
													type="number"
													min={10}
													max={3600}
													step={1}
													data-testid="providercooldown-learning-window-seconds"
													placeholder={t("providerCooldown.learning.windowSecondsPlaceholder")}
													{...field}
													value={field.value ?? ""}
													disabled={!learningEnabled || !hasUpdateAccess}
													onChange={(e) => {
														const value = e.target.value;
														if (value === "") {
															field.onChange(undefined);
															return;
														}
														const parsed = Number.parseInt(value);
														if (!Number.isNaN(parsed)) {
															field.onChange(parsed);
														}
													}}
												/>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
							<div className="flex-1">
								<FormField
									control={form.control}
									name="learning.max_samples"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("providerCooldown.learning.maxSamplesLabel")}</FormLabel>
											<FormDescription>{t("providerCooldown.learning.maxSamplesDescription")}</FormDescription>
											<FormControl>
												<Input
													type="number"
													min={5}
													max={500}
													step={1}
													data-testid="providercooldown-learning-max-samples"
													placeholder={t("providerCooldown.learning.maxSamplesPlaceholder")}
													{...field}
													value={field.value ?? ""}
													disabled={!learningEnabled || !hasUpdateAccess}
													onChange={(e) => {
														const value = e.target.value;
														if (value === "") {
															field.onChange(undefined);
															return;
														}
														const parsed = Number.parseInt(value);
														if (!Number.isNaN(parsed)) {
															field.onChange(parsed);
														}
													}}
												/>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
							<div className="flex-1">
								<FormField
									control={form.control}
									name="learning.min_samples"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("providerCooldown.learning.minSamplesLabel")}</FormLabel>
											<FormDescription>{t("providerCooldown.learning.minSamplesDescription")}</FormDescription>
											<FormControl>
												<Input
													type="number"
													min={1}
													max={100}
													step={1}
													data-testid="providercooldown-learning-min-samples"
													placeholder={t("providerCooldown.learning.minSamplesPlaceholder")}
													{...field}
													value={field.value ?? ""}
													disabled={!learningEnabled || !hasUpdateAccess}
													onChange={(e) => {
														const value = e.target.value;
														if (value === "") {
															field.onChange(undefined);
															return;
														}
														const parsed = Number.parseInt(value);
														if (!Number.isNaN(parsed)) {
															field.onChange(parsed);
														}
													}}
												/>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
						</div>
					</div>
				</div>

				<div className="flex justify-end">
					<Button
						type="submit"
						data-testid="providercooldown-config-save"
						disabled={!form.formState.isDirty || !hasUpdateAccess || isLoading}
						isLoading={isLoading}
					>
						{t("providerCooldown.configSave")}
					</Button>
				</div>
			</form>
		</Form>
	);
}

// ---------------------------------------------------------------------------
// MonitoringPanel — cooldown state + stats + unfreeze
// ---------------------------------------------------------------------------

/**
 * Renders the cooldown `expireAt` line as a locale-aware absolute timestamp
 * plus an approximate relative-distance phrase.
 *
 * `expireAt` arrives from the API as an ISO 8601 string (UTC, e.g.
 * `2026-08-17T06:50:25.010460549Z`). Browsers render `new Date(...)` in the
 * user's local time zone, so we format that Date with date-fns — that's what
 * gives "2026-08-17 14:51:23" instead of the raw ISO string.
 *
 * The relative phrase is recomputed every render, and the RTK Query state poll
 * (5s) refreshes the parent, so it drifts naturally ("about 1 hour 3 minutes"
 * → "about 1 hour 2 minutes" → …) without a separate timer.
 *
 * Falls back to a single i18n string when the timestamp is missing/invalid.
 */
function formatExpires(t: TFunction, expireAt: string, dateLocale: Locale): string {
	const date = formatLocalDateTime(expireAt, "yyyy-MM-dd HH:mm:ss", dateLocale);
	const relative = formatRelativeDistanceToNow(expireAt, dateLocale);
	if (!date || !relative) {
		return t("providerCooldown.invalidExpireAt");
	}
	return t("providerCooldown.expires", { date, in: relative });
}

// Format the CooldownStats response into the 4-card layout expected by the
// monitoring panel. Each kind gets one "marked" card + one "suppressed"
// card; unclassified marks do NOT contribute to byKind numbers (they
// show up only in the legacy total fields, which we don't display in the
// new layout). Returns zeroed-out cards when the backend hasn't sent
// byKind yet (older API responses).
function readKindStats(stats: CooldownStats) {
	const byKind = stats.byKind ?? {
		rate_limit: { markCount: 0, suppressedCount: 0 },
		quota: { markCount: 0, suppressedCount: 0 },
	};
	return {
		rateLimitMark: byKind.rate_limit?.markCount ?? 0,
		rateLimitSuppressed: byKind.rate_limit?.suppressedCount ?? 0,
		quotaMark: byKind.quota?.markCount ?? 0,
		quotaSuppressed: byKind.quota?.suppressedCount ?? 0,
	};
}

export function MonitoringPanel() {
	const { t, i18n } = useTranslation("plugins");
	const { data: stateData, isLoading: stateLoading } = useGetCooldownStateQuery(undefined, {
		pollingInterval: 5000,
	});
	const { data: statsData, isLoading: statsLoading } = useGetCooldownStatsQuery(undefined, {
		pollingInterval: 5000,
	});
	const [unfreeze, { isLoading: unfreezeLoading }] = useUnfreezeCooldownMutation();
	const dateLocale = getDateLocale(i18n.language);

	const entries = stateData?.state ?? [];
	const stats: CooldownStats = statsData?.stats ?? {
		markCount: 0,
		suppressedCount: 0,
		activeCount: 0,
	};
	const kindStats = readKindStats(stats);

	const handleUnfreeze = async (provider: string, keyId: string, model?: string) => {
		try {
			await unfreeze({ provider, keyId, model }).unwrap();
			toast.success(t("providerCooldown.unfreezeSuccessToast", { provider, keyId }));
		} catch {
			toast.error(t("providerCooldown.unfreezeFailedToast"));
		}
	};

	if (stateLoading || statsLoading) {
		return <div className="text-muted-foreground py-4 text-sm">{t("providerCooldown.monitoringLoading")}</div>;
	}

	return (
		<div className="space-y-6">
			{/* Stats cards — 4 kind-bucketed counters + 1 active count, separated visually */}
			<div className="grid grid-cols-2 gap-3 md:grid-cols-4">
				<div data-testid="providercooldown-stats-rate_limit-mark" className="rounded-lg border p-4">
					<div className="text-muted-foreground text-xs">{t("providerCooldown.markRateLimit")}</div>
					<div className="mt-1 text-2xl font-bold text-red-500">{kindStats.rateLimitMark}</div>
				</div>
				<div data-testid="providercooldown-stats-rate_limit-suppressed" className="rounded-lg border p-4">
					<div className="text-muted-foreground text-xs">{t("providerCooldown.suppressedRateLimit")}</div>
					<div className="mt-1 text-2xl font-bold text-blue-500">{kindStats.rateLimitSuppressed}</div>
				</div>
				<div data-testid="providercooldown-stats-quota-mark" className="rounded-lg border p-4">
					<div className="text-muted-foreground text-xs">{t("providerCooldown.markQuota")}</div>
					<div className="mt-1 text-2xl font-bold text-red-500">{kindStats.quotaMark}</div>
				</div>
				<div data-testid="providercooldown-stats-quota-suppressed" className="rounded-lg border p-4">
					<div className="text-muted-foreground text-xs">{t("providerCooldown.suppressedQuota")}</div>
					<div className="mt-1 text-2xl font-bold text-blue-500">{kindStats.quotaSuppressed}</div>
				</div>
			</div>
			{/* State entries */}
			<div>
				<h4 data-testid="providercooldown-stats-active" className="mb-2 text-sm font-medium">
					{t("providerCooldown.activeStateTitle")} <span className="text-[0.825rem] font-bold text-red-500">{stats.activeCount}</span>
				</h4>
				{entries.length === 0 ? (
					<p className="text-muted-foreground py-4 text-center text-sm">{t("providerCooldown.noActiveKeys")}</p>
				) : (
					<div className="space-y-2">
						{entries.map((entry) => (
							<ActiveStateRow
								key={entry.keyId}
								entry={entry}
								t={t}
								dateLocale={dateLocale}
								unfreezeLoading={unfreezeLoading}
								onUnfreeze={handleUnfreeze}
							/>
						))}
					</div>
				)}
			</div>
		</div>
	);
}

// ---------------------------------------------------------------------------
// ActiveStateRow — single active cooldown entry. Reason chip uses
// `entry.reason` which is the CooldownKind enum string ("rate_limit" /
// "quota"); we render an i18n label instead of the raw enum.
// ---------------------------------------------------------------------------

function ActiveStateRow({
	entry,
	t,
	dateLocale,
	unfreezeLoading,
	onUnfreeze,
}: {
	entry: CooldownStateEntry;
	t: TFunction;
	dateLocale: Locale;
	unfreezeLoading: boolean;
	onUnfreeze: (provider: string, keyId: string, model?: string) => void;
}) {
	const reasonKey =
		entry.reason === "rate_limit" ? "providerCooldown.reasonRateLimit" : entry.reason === "quota" ? "providerCooldown.reasonQuota" : null;
	return (
		<div
			key={entry.keyId}
			data-testid={`providercooldown-state-row-${entry.keyId}`}
			className="flex items-center justify-between rounded-md border p-3"
		>
			<div className="min-w-0 flex-1">
				<div className="flex items-center gap-2">
					<span className="font-medium">{entry.provider}</span>
					<span className="text-muted-foreground text-xs">
						{entry.keyId === "" ? t("providerCooldown.keylessLabel") : entry.keyId}
						{entry.keyName ? ` (${entry.keyName})` : ""}
						{entry.model ? ` / ${entry.model}` : ""}
					</span>
					{reasonKey && (
						<span data-testid={`providercooldown-state-row-${entry.keyId}-kind`} className="bg-muted rounded px-1.5 py-0.5 text-xs">
							{t(reasonKey)}
						</span>
					)}
				</div>
				<div className="text-muted-foreground mt-1 text-xs">
					<span>{formatExpires(t, entry.expireAt, dateLocale)}</span>
				</div>
			</div>
			{entry.keyId !== "" && (
				<Button
					type="button"
					variant="outline"
					size="sm"
					data-testid={`providercooldown-state-row-${entry.keyId}-unfreeze`}
					onClick={() => onUnfreeze(entry.provider, entry.keyId, entry.model)}
					disabled={unfreezeLoading}
				>
					{t("providerCooldown.unfreeze")}
				</Button>
			)}
		</div>
	);
}

// ---------------------------------------------------------------------------
// ProvidercooldownFragment — monitoring + per-provider policy overview + toggle
// ---------------------------------------------------------------------------

export function ProvidercooldownFragment({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	return (
		<div className="space-y-8">
			<h3 className="text-lg font-semibold">{t("providerCooldown.title")}</h3>

			{/* Section 1: monitoring panel — moved to top */}
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("providerCooldown.monitoringTitle")}</h4>
				<MonitoringPanel />
			</div>

			{/* Section 2: per-provider policy overview with edit jump */}
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("providerCooldown.perProviderPoliciesTitle")}</h4>
				<PerProviderPolicyOverview />
			</div>

			{/* Section 3: enabled switch */}
			<EnabledSwitch plugin={plugin} />

			{/* Section 4: optional capability sections (escalation / capacity / learning) */}
			<div className="rounded-lg border p-4">
				<h4 className="mb-1 text-sm font-medium">{t("providerCooldown.configTitle")}</h4>
				<p className="text-muted-foreground mb-4 text-sm">{t("providerCooldown.configDescription")}</p>
				<ConfigForm plugin={plugin} />
			</div>
		</div>
	);
}

function summarizeRule(rule: CooldownPolicyRule, t: TFunction): string {
	const mode = rule.match_mode === "all" ? t("providerCooldown.matchModeAll") : t("providerCooldown.matchModeAny");
	const summary = t("providerCooldown.ruleSummary", { count: rule.match.length, mode, ttl: rule.ttl_seconds });
	const scopeLabel = rule.scope === "model" ? t("providerCooldown.scopeModel") : t("providerCooldown.scopeKey");
	return `${summary} · ${scopeLabel}`;
}

function summarizeMatch(
	m: CooldownPolicy["rate_limit"] extends infer R ? (R extends { match: infer M } ? (M extends Array<infer X> ? X : never) : never) : never,
	t: TFunction,
): string {
	const parts: string[] = [];
	if (m.status_code !== undefined) parts.push(t("providerCooldown.matchStatus", { code: m.status_code }));
	if (m.message_contains && m.message_contains.length > 0) {
		parts.push(t("providerCooldown.matchMessage", { messages: m.message_contains.map((s) => `"${s}"`).join("|") }));
	}
	if (m.type && m.type.length > 0) {
		parts.push(t("providerCooldown.matchType", { types: m.type.join(",") }));
	}
	if (m.code && m.code.length > 0) {
		parts.push(t("providerCooldown.matchCode", { codes: m.code.join(",") }));
	}
	return parts.join(", ");
}

function PerProviderPolicyOverview() {
	const { t } = useTranslation("plugins");
	const navigate = useNavigate();
	const { data: providers = [], isLoading } = useGetProvidersQuery();
	const { data: statsData } = useGetCooldownStatsQuery(undefined, { pollingInterval: 5000 });

	if (isLoading) {
		return <div className="text-muted-foreground py-4 text-sm">{t("providerCooldown.providersLoading")}</div>;
	}

	const withPolicy = providers.filter((p) => p.cooldown_policy !== undefined);
	const usingDefault = providers.filter((p) => p.cooldown_policy === undefined);
	const perProviderStats = statsData?.stats?.perProvider ?? {};
	const perProviderModelStats = statsData?.stats?.perProviderModel ?? {};

	const editPolicy = (providerName: string) => {
		navigate({
			to: "/workspace/providers/$id",
			params: { id: providerName },
			search: { tab: "overview", editing: "cooldown-policy" },
		});
	};

	return (
		<div className="space-y-3">
			<p className="text-muted-foreground text-xs">{t("providerCooldown.perProviderPoliciesHint")}</p>

			{withPolicy.length > 0 && (
				<div className="space-y-2">
					{withPolicy.map((p) => (
						<div key={p.name} data-testid={`providercooldown-policy-row-${p.name}`} className="rounded-md border p-3">
							<div className="flex items-center justify-between">
								<span className="font-medium">{ProviderLabels[p.name as keyof typeof ProviderLabels] ?? p.name}</span>
								<div className="flex items-center gap-3">
									<ProviderKindStats providerName={p.name} stats={perProviderStats[p.name]} t={t} />
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={() => editPolicy(p.name)}
										data-testid={`providercooldown-policy-edit-${p.name}`}
									>
										{t("providerCooldown.editPolicy")}
									</Button>
								</div>
							</div>
							{p.cooldown_policy?.rate_limit && (
								<div className="text-muted-foreground mt-1 text-xs">
									<span className="font-medium">{t("providerCooldown.rateLimitLabel")}: </span>
									{p.cooldown_policy.rate_limit.enabled === false ? (
										<span className="italic">{t("providerCooldown.disabled")}</span>
									) : (
										<>
											{summarizeRule(p.cooldown_policy.rate_limit, t)}
											<ul className="mt-1 ml-4 list-disc">
												{p.cooldown_policy.rate_limit.match.map((m, i) => (
													<li key={i} className="font-mono">
														{summarizeMatch(m, t)}
													</li>
												))}
											</ul>
										</>
									)}
								</div>
							)}
							{p.cooldown_policy?.quota && (
								<div className="text-muted-foreground mt-2 text-xs">
									<span className="font-medium">{t("providerCooldown.quotaLabel")}: </span>
									{p.cooldown_policy.quota.enabled === false ? (
										<span className="italic">{t("providerCooldown.disabled")}</span>
									) : (
										<>
											{summarizeRule(p.cooldown_policy.quota, t)}
											<ul className="mt-1 ml-4 list-disc">
												{p.cooldown_policy.quota.match.map((m, i) => (
													<li key={i} className="font-mono">
														{summarizeMatch(m, t)}
													</li>
												))}
											</ul>
										</>
									)}
								</div>
							)}
							{(() => {
								const modelStats = perProviderModelStats[p.name];
								if (!modelStats || Object.keys(modelStats).length === 0) return null;
								const total = perProviderStats[p.name];
								const scopeKey = statsData?.stats?.perProviderScopeKey?.[p.name];
								const scopeModel = statsData?.stats?.perProviderScopeModel?.[p.name];
								return (
									<div className="mt-2 border-t pt-2" data-testid={`providercooldown-policy-model-stats-${p.name}`}>
										<div className="text-muted-foreground mb-1 text-xs font-medium">{t("providerCooldown.modelBreakdownTitle")}</div>
										{Object.entries(modelStats).map(([model, ms]) => (
											<div
												key={model}
												className="text-muted-foreground font-mono text-xs"
												data-testid={`providercooldown-policy-model-stats-${p.name}-${model}`}
											>
												<span className="font-medium">{model}:</span> <span className="text-red-500">{ms.rate_limit?.markCount ?? 0}</span>/
												<span className="text-blue-500">{ms.rate_limit?.suppressedCount ?? 0}</span>
												<span className="text-border mx-1">·</span>
												<span className="text-red-500">{ms.quota?.markCount ?? 0}</span>/
												<span className="text-blue-500">{ms.quota?.suppressedCount ?? 0}</span>
												<span className="ml-0.5">
													（<span className="text-red-500">{t("providerCooldown.marked")}</span>/
													<span className="text-blue-500">{t("providerCooldown.suppressed")}</span>）
												</span>
											</div>
										))}
										{(() => {
											if (!total) return null;
											const modelRlMarkSum = Object.values(modelStats).reduce((acc, ms) => acc + (ms.rate_limit?.markCount ?? 0), 0);
											const modelRlSupSum = Object.values(modelStats).reduce((acc, ms) => acc + (ms.rate_limit?.suppressedCount ?? 0), 0);
											const modelQMarkSum = Object.values(modelStats).reduce((acc, ms) => acc + (ms.quota?.markCount ?? 0), 0);
											const modelQSupSum = Object.values(modelStats).reduce((acc, ms) => acc + (ms.quota?.suppressedCount ?? 0), 0);
											const rlMarkRemain = (total.rate_limit?.markCount ?? 0) - modelRlMarkSum;
											const rlSupRemain = (total.rate_limit?.suppressedCount ?? 0) - modelRlSupSum;
											const qMarkRemain = (total.quota?.markCount ?? 0) - modelQMarkSum;
											const qSupRemain = (total.quota?.suppressedCount ?? 0) - modelQSupSum;
											if (rlMarkRemain === 0 && rlSupRemain === 0 && qMarkRemain === 0 && qSupRemain === 0) return null;
											return (
												<div
													className="text-muted-foreground mt-1 font-mono text-xs italic"
													data-testid={`providercooldown-policy-model-stats-${p.name}-key-scope-remainder`}
												>
													<span className="font-medium">{t("providerCooldown.modelBreakdownKeyScopeRemainder")}:</span>{" "}
													<span className="text-red-500">{rlMarkRemain}</span>/<span className="text-blue-500">{rlSupRemain}</span>
													<span className="text-border mx-1">·</span>
													<span className="text-red-500">{qMarkRemain}</span>/<span className="text-blue-500">{qSupRemain}</span>
													{scopeKey && (
														<span className="text-muted-foreground/70 ml-1 text-[0.7rem]">
															(scope=key 标记 {scopeKey.rate_limit?.markCount ?? 0 + (scopeKey.quota?.markCount ?? 0)} / 抑制{" "}
															{scopeKey.rate_limit?.suppressedCount ?? 0 + (scopeKey.quota?.suppressedCount ?? 0)})
														</span>
													)}
													{scopeModel && (
														<span className="text-muted-foreground/70 ml-1 text-[0.7rem]">
															(scope=model 标记 {scopeModel.rate_limit?.markCount ?? 0 + (scopeModel.quota?.markCount ?? 0)} / 抑制{" "}
															{scopeModel.rate_limit?.suppressedCount ?? 0 + (scopeModel.quota?.suppressedCount ?? 0)})
														</span>
													)}
												</div>
											);
										})()}
									</div>
								);
							})()}
						</div>
					))}
				</div>
			)}

			{usingDefault.length > 0 && (
				<div className="space-y-2">
					{usingDefault.map((p) => (
						<div key={p.name} data-testid={`providercooldown-policy-row-${p.name}`} className="rounded-md border border-dashed p-3">
							<div className="flex items-center justify-between">
								<span className="font-medium">{ProviderLabels[p.name as keyof typeof ProviderLabels] ?? p.name}</span>
								<div className="flex items-center gap-3">
									<ProviderKindStats providerName={p.name} stats={perProviderStats[p.name]} t={t} />
									<Button
										type="button"
										variant="ghost"
										size="sm"
										onClick={() => editPolicy(p.name)}
										data-testid={`providercooldown-policy-goto-${p.name}`}
									>
										{t("providerCooldown.gotoConfig")}
									</Button>
								</div>
							</div>
						</div>
					))}
				</div>
			)}
		</div>
	);
}

// Per-provider kind counter group rendered next to the Edit/Configure
// button on each row. Format: "速率限制 4/3 配额耗尽 1/1（标记/抑制）"
// — two compact `marked/suppressed` pairs with bold numbers, monospace
// so columns line up across rows. Mark counts and 标记 label are red;
// suppressed counts and 抑制 label are blue. When the provider has no
// classified traffic yet (no entry in perProviderStats) we render "—"
// to make the empty state visually distinct from explicit "0/0" counts.
function ProviderKindStats({ providerName, stats, t }: { providerName: string; stats: ProviderKindCounters | undefined; t: TFunction }) {
	if (!stats) {
		return (
			<span data-testid={`providercooldown-policy-stats-${providerName}`} className="text-muted-foreground font-mono text-xs">
				—
			</span>
		);
	}
	return (
		<span data-testid={`providercooldown-policy-stats-${providerName}`} className="text-muted-foreground font-mono text-xs">
			<span data-testid={`providercooldown-policy-stats-${providerName}-rate_limit`}>
				{t("providerCooldown.rateLimitLabel")} <span className="font-semibold text-red-500">{stats.rate_limit?.markCount ?? 0}</span>/
				<span className="font-semibold text-blue-500">{stats.rate_limit?.suppressedCount ?? 0}</span>
			</span>
			<span className="text-border mx-2">·</span>
			<span data-testid={`providercooldown-policy-stats-${providerName}-quota`}>
				{t("providerCooldown.quotaLabel")} <span className="font-semibold text-red-500">{stats.quota?.markCount ?? 0}</span>/
				<span className="font-semibold text-blue-500">{stats.quota?.suppressedCount ?? 0}</span>
			</span>
			<span className="ml-1">
				（<span className="text-red-500">{t("providerCooldown.marked")}</span>/
				<span className="text-blue-500">{t("providerCooldown.suppressed")}</span>）
			</span>
		</span>
	);
}

// Default export for convenience (also used by pluginsView.tsx)
export default ProvidercooldownFragment;