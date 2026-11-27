import { formatCapacitySummary } from "@/components/providerCooldown/capacitySummary";
import {
	capacityGateStatus,
	DEFAULT_ESCALATION_MAX_MULTIPLIER,
	DEFAULT_LEARNING_MAX_SAMPLES,
	DEFAULT_LEARNING_MIN_SAMPLES,
	DEFAULT_LEARNING_WINDOW_SECONDS,
} from "@/components/providerCooldown/defaults";
import { EscalationFields } from "@/components/providerCooldown/escalationFields";
import { LearningFields } from "@/components/providerCooldown/learningFields";
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
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { Switch } from "@/components/ui/switch";
import { CapacityUsageTable } from "./providercooldownCapacityUsageTable";
import { useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { ProviderLabels } from "@/lib/constants/logs";
import { PROVIDER_COOLDOWN_PLUGIN, type Plugin, type ProviderCooldownCapabilityDefaults } from "@/lib/types/plugins";
import { providerCooldownPluginConfigFormSchema, type ProviderCooldownPluginConfigFormSchema } from "@/lib/types/schemas";
import { formatLocalDateTime, formatRelativeDistanceToNow, getDateLocale } from "@/lib/utils/date";
import type { CooldownCapacityConfig, CooldownPolicy, CooldownPolicyRule } from "@/lib/types/config";
import type { CooldownStats, CooldownStateEntry, ProviderKindCounters } from "@/lib/types/plugins";
import type { Locale } from "date-fns";
import { Info } from "lucide-react";
import { useState } from "react";
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
// ConfigForm — the plugin-wide mechanism-level defaults
// ---------------------------------------------------------------------------
//
// The provider-cooldown plugin carries three opt-in capabilities:
//
//   - escalation — adaptive TTL upgrade (a key's cooldown grows while it keeps
//     failing)
//   - capacity   — retrospective capacity gate backed by real usage
//   - learning   — read-only learner that estimates the provider's effective
//     per-window limit from upstream rate-limit feedback
//
// Only escalation and learning are editable here. Both are mechanism-level
// policies that look the same across providers, so a fleet default is safe.
//
// capacity is deliberately NOT part of this form: its unit is a provider's
// plan quota, so a single global token limit inherited by every unconfigured
// provider is exactly the defect per-provider configuration replaced. The
// provider detail page owns the capacity gate; a `config.capacity` block left
// over from older deployments is still honoured by the backend and surfaced
// read-only here (see LegacyCapacityDefaultAlert / CapacityUsageTable).
//
// A disabled section is still submitted as `{ enabled: false, … }` — omitting
// the key would leave the previously stored value in place on the backend.

/** The stored plugin config: the capability sections plus whatever unrelated
 *  top-level keys the plugin already had (default_ttl_seconds, ttl_overrides,
 *  quota_patterns, …). */
interface ProviderCooldownStoredConfig extends ProviderCooldownCapabilityDefaults {
	default_ttl_seconds?: number;
	ttl_overrides?: Record<string, number>;
	quota_patterns?: string[];
}

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const pluginConfig = (plugin.config ?? {}) as ProviderCooldownStoredConfig;

	const form = useForm<ProviderCooldownPluginConfigFormSchema, unknown, ProviderCooldownPluginConfigFormSchema>({
		resolver: zodResolver(providerCooldownPluginConfigFormSchema) as Resolver<
			ProviderCooldownPluginConfigFormSchema,
			unknown,
			ProviderCooldownPluginConfigFormSchema
		>,
		mode: "onChange",
		reValidateMode: "onChange",
		defaultValues: {
			escalation: {
				enabled: pluginConfig.escalation?.enabled ?? false,
				max_multiplier: pluginConfig.escalation?.max_multiplier ?? DEFAULT_ESCALATION_MAX_MULTIPLIER,
			},
			learning: {
				enabled: pluginConfig.learning?.enabled ?? false,
				window_seconds: pluginConfig.learning?.window_seconds ?? DEFAULT_LEARNING_WINDOW_SECONDS,
				max_samples: pluginConfig.learning?.max_samples ?? DEFAULT_LEARNING_MAX_SAMPLES,
				min_samples: pluginConfig.learning?.min_samples ?? DEFAULT_LEARNING_MIN_SAMPLES,
			},
		},
	});

	const disabled = isLoading || !hasUpdateAccess;
	const hasFieldErrors = Object.keys(form.formState.errors).length > 0;

	const onSubmit = async (values: ProviderCooldownPluginConfigFormSchema) => {
		if (!hasUpdateAccess) return;
		// Every other top-level key of the stored config (default_ttl_seconds,
		// ttl_overrides, quota_patterns, the optional `health` section, and —
		// importantly — the legacy `capacity` block) is carried over untouched:
		// only escalation and learning are rewritten here.
		//
		// `capacity` is intentionally NOT in the payload. Spreading the current
		// config keeps whatever is stored, so saving the default policies can
		// never silently drop or overwrite a legacy fleet-wide capacity
		// default. Removing it is a separate, explicit action
		// (LegacyCapacityDefaultAlert). Covered by
		// providercooldownFragment.test.tsx.
		const currentConfig = (plugin.config ?? {}) as Record<string, unknown>;
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
				<div className="rounded-lg border p-4">
					<EscalationFields
						value={form.watch("escalation")}
						onChange={(next) =>
							form.setValue("escalation", next as ProviderCooldownPluginConfigFormSchema["escalation"], {
								shouldDirty: true,
								shouldValidate: true,
							})
						}
						disabled={disabled}
					/>
				</div>

				<div className="rounded-lg border p-4">
					<LearningFields
						value={form.watch("learning")}
						onChange={(next) =>
							form.setValue("learning", next as ProviderCooldownPluginConfigFormSchema["learning"], {
								shouldDirty: true,
								shouldValidate: true,
							})
						}
						disabled={disabled}
					/>
				</div>

				{hasFieldErrors && (
					<p role="alert" data-testid="providercooldown-config-errors" className="text-destructive text-sm">
						{t("providerCooldown.configInvalidHint")}
					</p>
				)}

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
// LegacyCapacityDefaultAlert — the deprecated plugin-level capacity default
// ---------------------------------------------------------------------------
//
// Older deployments may carry `config.capacity` in config.json. The backend
// still honours it for providers without their own `cooldown_policy.capacity`
// (removing support outright would silently un-gate them), but the plugin page
// no longer edits it. Instead it is surfaced here with its effective summary
// and an explicit, confirmed removal. Only the `capacity` key is dropped; the
// rest of the config is preserved.
export function LegacyCapacityDefaultAlert({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const { t: tc } = useTranslation("common");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();
	const [confirmOpen, setConfirmOpen] = useState(false);

	const pluginConfig = (plugin.config ?? {}) as ProviderCooldownStoredConfig;
	const legacy = pluginConfig.capacity;
	if (legacy === undefined) return null;

	const status = capacityGateStatus(undefined, legacy);
	const summary = formatCapacitySummary(t, tc, status.effective);

	const clearLegacy = async () => {
		if (!hasUpdateAccess) return;
		// Copy + delete so every other top-level key (including keys this
		// component doesn't know about) survives untouched.
		const nextConfig = { ...((plugin.config ?? {}) as Record<string, unknown>) };
		delete nextConfig.capacity;
		try {
			await updatePlugin({
				name: PROVIDER_COOLDOWN_PLUGIN,
				data: { enabled: plugin.enabled, config: nextConfig },
			}).unwrap();
			toast.success(t("providerCooldown.legacyCapacityClearedToast"));
			setConfirmOpen(false);
		} catch {
			toast.error(t("providerCooldown.legacyCapacityClearFailedToast"));
		}
	};

	return (
		<Alert variant="warning" data-testid="providercooldown-legacy-capacity-alert" className="mb-4">
			<Info />
			<AlertTitle>{t("providerCooldown.legacyCapacityTitle")}</AlertTitle>
			<AlertDescription>
				<p>{t("providerCooldown.legacyCapacityDescription")}</p>
				<p className="font-mono" data-testid="providercooldown-legacy-capacity-summary">
					{status.active ? t("providerCooldown.legacyCapacityActiveSummary", { summary }) : t("providerCooldown.legacyCapacityInactive")}
				</p>
				<Button
					type="button"
					variant="outline"
					size="sm"
					data-testid="providercooldown-legacy-capacity-clear"
					onClick={() => setConfirmOpen(true)}
					disabled={isLoading || !hasUpdateAccess}
				>
					{t("providerCooldown.legacyCapacityClear")}
				</Button>
			</AlertDescription>

			<AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("providerCooldown.legacyCapacityConfirmTitle")}</AlertDialogTitle>
						<AlertDialogDescription>{t("providerCooldown.legacyCapacityConfirmDescription")}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel data-testid="providercooldown-legacy-capacity-cancel">
							{t("providerCooldown.legacyCapacityCancel")}
						</AlertDialogCancel>
						<AlertDialogAction
							className="bg-destructive hover:bg-destructive/90"
							data-testid="providercooldown-legacy-capacity-confirm"
							onClick={() => void clearLegacy()}
							disabled={isLoading}
						>
							{t("providerCooldown.legacyCapacityConfirm")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</Alert>
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

			{/* Section 2: read-only fleet-wide capacity water level */}
			<CapacityUsageTable />

			{/* Section 3: per-provider policy overview with edit jump */}
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("providerCooldown.perProviderPoliciesTitle")}</h4>
				<PerProviderPolicyOverview plugin={plugin} />
			</div>

			{/* Section 4: enabled switch */}
			<EnabledSwitch plugin={plugin} />

			{/* Section 5: fleet-wide mechanism defaults (escalation / learning).
			    Capacity is NOT here — it is per-provider only. A legacy config.json
			    default is surfaced read-only above the form and can be cleared. */}
			<div className="rounded-lg border p-4">
				<h4 className="mb-1 text-sm font-medium">{t("providerCooldown.capabilityDefaultsTitle")}</h4>
				<p className="text-muted-foreground mb-4 text-sm">{t("providerCooldown.capabilityDefaultsDescription")}</p>
				<LegacyCapacityDefaultAlert plugin={plugin} />
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

function PerProviderPolicyOverview({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const { t: tc } = useTranslation("common");
	const navigate = useNavigate();
	const { data: providers = [], isLoading } = useGetProvidersQuery();
	const { data: statsData } = useGetCooldownStatsQuery(undefined, { pollingInterval: 5000 });

	const pluginDefaults = (plugin.config ?? {}) as ProviderCooldownCapabilityDefaults;
	// Providers that carry no capacity override inherit whatever is configured
	// in the deprecated plugin defaults (config.json) — if anything is.
	const inheritingCapacityCount = providers.filter((p) => p.cooldown_policy?.capacity === undefined).length;
	const legacyCapacityApplies = capacityGateStatus(undefined, pluginDefaults.capacity).source === "legacy-default";

	if (isLoading) {
		return <div className="text-muted-foreground py-4 text-sm">{t("providerCooldown.providersLoading")}</div>;
	}

	const withPolicy = providers.filter((p) => p.cooldown_policy !== undefined);
	const usingDefault = providers.filter((p) => p.cooldown_policy === undefined);
	const perProviderStats = statsData?.stats?.perProvider ?? {};
	const perProviderModelStats = statsData?.stats?.perProviderModel ?? {};

	const openCooldown = (providerName: string) => {
		navigate({
			to: "/workspace/providers/$id",
			params: { id: providerName },
			search: { tab: "cooldown" },
		});
	};

	return (
		<div className="space-y-3">
			<p className="text-muted-foreground text-xs">{t("providerCooldown.perProviderPoliciesHint")}</p>

			{legacyCapacityApplies && inheritingCapacityCount > 0 && (
				<p
					data-testid="providercooldown-capacity-legacy-hint"
					className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs"
				>
					{t("providerCooldown.capacityLegacyHint", { count: inheritingCapacityCount })}
				</p>
			)}

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
										onClick={() => openCooldown(p.name)}
										data-testid={`providercooldown-configure-${p.name}`}
									>
										{t("providerCooldown.configureCooldown")}
									</Button>
								</div>
							</div>
							<div className="text-muted-foreground mt-1 text-xs">
								<span className="font-medium">{t("providerCooldown.capacityGateLabel")}: </span>
								<CapacityGateStatusLine
									providerName={p.name}
									capacity={p.cooldown_policy?.capacity}
									pluginDefault={pluginDefaults.capacity}
									t={t}
									tc={tc}
								/>
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
										onClick={() => openCooldown(p.name)}
										data-testid={`providercooldown-configure-${p.name}`}
									>
										{t("providerCooldown.configureCooldown")}
									</Button>
								</div>
							</div>
							<div className="text-muted-foreground mt-1 text-xs">
								<span className="font-medium">{t("providerCooldown.capacityGateLabel")}: </span>
								<CapacityGateStatusLine
									providerName={p.name}
									capacity={p.cooldown_policy?.capacity}
									pluginDefault={pluginDefaults.capacity}
									t={t}
									tc={tc}
								/>
							</div>
						</div>
					))}
				</div>
			)}
		</div>
	);
}

// Per-provider capacity gate summary rendered on every fleet row. The plugin
// page no longer owns a capacity default, so the wording distinguishes where
// the gate that applies comes from:
//
//   - provider section present, not explicitly off → "active" + effective values
//   - provider section present, `enabled: false`   → "off"
//   - no provider section, legacy config.json
//     capacity actually gating                    → "using the legacy global
//                                                    default" (warning colour)
//   - anything else                                → "not configured (inert)"
function CapacityGateStatusLine({
	providerName,
	capacity,
	pluginDefault,
	t,
	tc,
}: {
	providerName: string;
	capacity: CooldownCapacityConfig | undefined;
	pluginDefault: CooldownCapacityConfig | undefined;
	t: TFunction;
	tc: TFunction;
}) {
	const status = capacityGateStatus(capacity, pluginDefault);
	const summary = formatCapacitySummary(t, tc, status.effective);

	if (status.source === "legacy-default") {
		return (
			<span data-testid={`providercooldown-capacity-status-${providerName}`}>
				<span className="font-medium text-amber-600 dark:text-amber-500">{t("providerCooldown.capacityStatusLegacy")}</span>
				<span className="font-mono"> · {summary}</span>
			</span>
		);
	}

	if (status.source === "none") {
		return (
			<span data-testid={`providercooldown-capacity-status-${providerName}`}>
				<span className="italic">{t("providerCooldown.capacityStatusUnconfigured")}</span>
			</span>
		);
	}

	if (status.mode === "disabled") {
		return (
			<span data-testid={`providercooldown-capacity-status-${providerName}`}>
				<span className="font-medium">{t("providerCooldown.capacityStatusDisabled")}</span>
			</span>
		);
	}

	return (
		<span data-testid={`providercooldown-capacity-status-${providerName}`}>
			<span className="font-medium">{t("providerCooldown.capacityStatusActive")}</span>
			<span className="font-mono"> · {summary}</span>
		</span>
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