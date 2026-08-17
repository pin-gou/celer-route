import { Button } from "@/components/ui/button";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { ProviderLabels } from "@/lib/constants/logs";
import { PROVIDER_COOLDOWN_PLUGIN, providerCooldownConfigSchema, type Plugin } from "@/lib/types/plugins";
import { formatLocalDateTime, formatRelativeDistanceToNow, getDateLocale } from "@/lib/utils/date";
import { zodResolver } from "@hookform/resolvers/zod";
import type { Locale } from "date-fns";
import { Info, PlusIcon, Trash2Icon } from "lucide-react";
import { useFieldArray, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { toast } from "sonner";
import { z } from "zod";
import { useGetCooldownStateQuery, useGetCooldownStatsQuery, useUnfreezeCooldownMutation } from "@/lib/store/apis/pluginsApi";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type ProviderCooldownFormValues = z.infer<typeof providerCooldownConfigSchema>;

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
// ConfigForm — react-hook-form + zod for the 3 config fields
// ---------------------------------------------------------------------------

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();
	const { data: providers = [] } = useGetProvidersQuery();

	const pluginConfig = (plugin.config || {}) as Record<string, any>;

	const form = useForm<ProviderCooldownFormValues>({
		resolver: zodResolver(providerCooldownConfigSchema),
		defaultValues: {
			default_ttl_seconds: pluginConfig.default_ttl_seconds ?? 300,
			ttl_overrides: pluginConfig.ttl_overrides ?? {},
			quota_patterns: pluginConfig.quota_patterns ?? ["用量上限", "token plan", "token-plan"],
		},
	});

	const {
		fields: quotaFields,
		append: appendQuota,
		remove: removeQuota,
	} = useFieldArray<any>({
		control: form.control,
		name: "quota_patterns",
	});

	const ttlOverrides = form.watch("ttl_overrides") || {};
	const ttlOverrideKeys = Object.keys(ttlOverrides);
	const usedProviders = new Set(ttlOverrideKeys);
	const availableProviders = providers.filter((p) => !usedProviders.has(p.name));
	const firstAvailableProvider = availableProviders[0]?.name;
	const allProvidersConsumed = ttlOverrideKeys.length > 0 && availableProviders.length === 0;

	const renameOverride = (from: string, to: string) => {
		const next = { ...ttlOverrides };
		const ttl = next[from];
		delete next[from];
		next[to] = ttl;
		form.setValue("ttl_overrides", next, { shouldValidate: true, shouldDirty: true });
	};

	const providerLabel = (name: string) => ProviderLabels[name as keyof typeof ProviderLabels] ?? name;

	const onSubmit = async (values: ProviderCooldownFormValues) => {
		if (!hasUpdateAccess) return;
		try {
			await updatePlugin({
				name: PROVIDER_COOLDOWN_PLUGIN,
				data: {
					enabled: plugin.enabled,
					config: {
						default_ttl_seconds: values.default_ttl_seconds,
						ttl_overrides: values.ttl_overrides,
						quota_patterns: values.quota_patterns,
					},
				},
			}).unwrap();
			toast.success(t("providerCooldown.savedToast"));
		} catch {
			toast.error(t("providerCooldown.saveFailedToast"));
		}
	};

	const onError = () => {
		toast.error(t("providerCooldown.formErrorToast"));
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit, onError)} className="space-y-6">
				{/* default_ttl_seconds */}
				<FormField
					control={form.control}
					name="default_ttl_seconds"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("providerCooldown.defaultTTLLabel")}</FormLabel>
							<FormControl>
								<Input
									data-testid="providercooldown-field-default-ttl"
									type="number"
									min={1}
									max={86400}
									placeholder={t("providerCooldown.defaultTTLPlaceholder")}
									{...field}
									onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
								/>
							</FormControl>
							<p className="text-muted-foreground text-xs">{t("providerCooldown.defaultTTLDescription")}</p>
							<FormMessage />
						</FormItem>
					)}
				/>

				{/* ttl_overrides */}
				<FormItem>
					<FormLabel>{t("providerCooldown.ttlOverridesLabel")}</FormLabel>
					<p className="text-muted-foreground mb-2 text-xs">{t("providerCooldown.ttlOverridesDescription")}</p>
					<FormControl>
						<div className="space-y-2">
							{ttlOverrideKeys.length === 0 && <p className="text-muted-foreground text-sm">{t("providerCooldown.noOverrides")}</p>}
							{ttlOverrideKeys.map((providerKey) => {
								const otherKeys = ttlOverrideKeys.filter((k) => k !== providerKey);
								const otherSet = new Set<string>(otherKeys);
								const rowOptionNames: string[] = providers.map((p) => p.name).filter((name) => !otherSet.has(name));
								if (!rowOptionNames.includes(providerKey)) {
									rowOptionNames.push(providerKey);
								}
								return (
									<div key={providerKey} className="flex items-center gap-2">
										<Select value={providerKey} onValueChange={(next) => renameOverride(providerKey, next)}>
											<SelectTrigger className="w-1/3" data-testid={`providercooldown-field-ttl-overrides-provider-${providerKey}`}>
												<SelectValue placeholder={t("providerCooldown.selectProviderPlaceholder")} />
											</SelectTrigger>
											<SelectContent>
												{rowOptionNames.map((name) => (
													<SelectItem key={name} value={name}>
														{providerLabel(name)}
													</SelectItem>
												))}
											</SelectContent>
										</Select>
										<Input
											data-testid={`providercooldown-field-ttl-overrides-value-${providerKey}`}
											type="number"
											min={1}
											placeholder={t("providerCooldown.ttlSecondsPlaceholder")}
											value={ttlOverrides[providerKey]}
											onChange={(e) => {
												const num = e.target.valueAsNumber;
												if (Number.isNaN(num)) return;
												form.setValue(
													"ttl_overrides",
													{ ...ttlOverrides, [providerKey]: num },
													{ shouldValidate: true, shouldDirty: true },
												);
											}}
										/>
										<Button
											type="button"
											variant="ghost"
											size="sm"
											onClick={() => {
												form.setValue(
													"ttl_overrides",
													Object.fromEntries(Object.entries(ttlOverrides).filter(([k]) => k !== providerKey)) as Record<string, number>,
													{ shouldValidate: true, shouldDirty: true },
												);
											}}
										>
											<Trash2Icon className="h-4 w-4" />
										</Button>
									</div>
								);
							})}
							{allProvidersConsumed && providers.length > 0 && (
								<p className="text-muted-foreground text-xs">{t("providerCooldown.allProvidersConsumed")}</p>
							)}
							{providers.length === 0 && <p className="text-muted-foreground text-xs">{t("providerCooldown.noProviders")}</p>}
							<Button
								type="button"
								variant="outline"
								size="sm"
								disabled={!firstAvailableProvider}
								onClick={() => {
									if (firstAvailableProvider) {
										form.setValue(
											"ttl_overrides",
											{ ...ttlOverrides, [firstAvailableProvider]: 300 },
											{ shouldValidate: true, shouldDirty: true },
										);
									}
								}}
							>
								<PlusIcon className="mr-1 h-3 w-3" />
								{t("providerCooldown.addOverride")}
							</Button>
						</div>
					</FormControl>
					<FormMessage />
				</FormItem>

				{/* quota_patterns */}
				<FormItem>
					<FormLabel>{t("providerCooldown.quotaPatternsLabel")}</FormLabel>
					<p className="text-muted-foreground mb-2 text-xs">{t("providerCooldown.quotaPatternsDescription")}</p>
					<FormControl>
						<div className="space-y-2">
							{quotaFields.map((field, index) => (
								<div key={field.id} className="flex items-center gap-2">
									<FormField
										control={form.control}
										name={`quota_patterns.${index}`}
										render={({ field: innerField }) => (
											<>
												<Input
													data-testid={`providercooldown-field-quota-patterns-${index}`}
													placeholder={t("providerCooldown.quotaPatternPlaceholder")}
													{...innerField}
												/>
												<Button type="button" variant="ghost" size="sm" onClick={() => removeQuota(index)}>
													<Trash2Icon className="h-4 w-4" />
												</Button>
											</>
										)}
									/>
								</div>
							))}
							<Button type="button" variant="outline" size="sm" onClick={() => appendQuota("")}>
								<PlusIcon className="mr-1 h-3 w-3" />
								{t("providerCooldown.addPattern")}
							</Button>
						</div>
					</FormControl>
					<FormMessage />
				</FormItem>

				{/* Save button */}
				<div className="flex justify-end">
					<Button type="submit" disabled={isLoading || !form.formState.isDirty || !hasUpdateAccess}>
						{isLoading ? t("providerCooldown.saving") : t("providerCooldown.saveConfiguration")}
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
	const stats = statsData?.stats ?? { markCount: 0, suppressedCount: 0, activeCount: 0 };

	const handleUnfreeze = async (provider: string, keyId: string) => {
		try {
			await unfreeze({ provider, keyId }).unwrap();
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
			{/* Stats cards */}
			<div className="grid grid-cols-3 gap-4">
				<div data-testid="providercooldown-stats-mark" className="rounded-lg border p-4 text-center">
					<div className="text-2xl font-bold">{stats.markCount}</div>
					<div className="text-muted-foreground text-xs">{t("providerCooldown.totalMarked")}</div>
				</div>
				<div data-testid="providercooldown-stats-suppressed" className="rounded-lg border p-4 text-center">
					<div className="text-2xl font-bold">{stats.suppressedCount}</div>
					<div className="text-muted-foreground text-xs">{t("providerCooldown.suppressedRequests")}</div>
				</div>
				<div data-testid="providercooldown-stats-active" className="rounded-lg border p-4 text-center">
					<div className="text-2xl font-bold">{stats.activeCount}</div>
					<div className="text-muted-foreground text-xs">{t("providerCooldown.currentlyActive")}</div>
				</div>
			</div>

			{/* State entries */}
			<div>
				<h4 className="mb-2 text-sm font-medium">{t("providerCooldown.activeStateTitle")}</h4>
				{entries.length === 0 ? (
					<p className="text-muted-foreground py-4 text-center text-sm">{t("providerCooldown.noActiveKeys")}</p>
				) : (
					<div className="space-y-2">
						{entries.map((entry) => (
							<div
								key={entry.keyId}
								data-testid={`providercooldown-state-row-${entry.keyId}`}
								className="flex items-center justify-between rounded-md border p-3"
							>
								<div className="min-w-0 flex-1">
									<div className="flex items-center gap-2">
										<span className="font-medium">{entry.provider}</span>
										<span className="text-muted-foreground text-xs">
											{entry.keyId}
											{entry.keyName ? ` (${entry.keyName})` : ""}
										</span>
									</div>
									<div className="text-muted-foreground mt-1 text-xs">
										<span>{t("providerCooldown.reason", { reason: entry.reason })}</span>
										<span className="ml-3">{formatExpires(t, entry.expireAt, dateLocale)}</span>
									</div>
								</div>
								<Button
									type="button"
									variant="outline"
									size="sm"
									data-testid={`providercooldown-state-row-${entry.keyId}-unfreeze`}
									onClick={() => handleUnfreeze(entry.provider, entry.keyId)}
									disabled={unfreezeLoading}
								>
									{t("providerCooldown.unfreeze")}
								</Button>
							</div>
						))}
					</div>
				)}
			</div>
		</div>
	);
}

// ---------------------------------------------------------------------------
// ProvidercooldownFragment — full three-section fragment
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

			{/* Section 2: enabled switch */}
			<EnabledSwitch plugin={plugin} />

			{/* Section 3: config form */}
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("providerCooldown.settingsTitle")}</h4>
				<ConfigForm plugin={plugin} />
			</div>
		</div>
	);
}

// Default export for convenience (also used by pluginsView.tsx)
export default ProvidercooldownFragment;