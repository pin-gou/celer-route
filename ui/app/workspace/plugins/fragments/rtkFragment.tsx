import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { useGetRtkStatsQuery, useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { RTK_PLUGIN, rtkConfigSchema, type Plugin } from "@/lib/types/plugins";
import { Link } from "@tanstack/react-router";
import { zodResolver } from "@hookform/resolvers/zod";
import { Activity, Beaker, ExternalLink, FlaskConical, HelpCircle, Image as ImageIcon, Info, RotateCcw, Undo2 } from "lucide-react";
import { useEffect, useMemo, useRef } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type RTKFormValues = z.input<typeof rtkConfigSchema>;
type Intensity = "minimal" | "standard" | "aggressive";

// ---------------------------------------------------------------------------
// effectiveMaxLines — front-end mirror of plugins/rtk/compression.go:668.
// Single source of truth: max(1, round(base * factor)) by intensity factor.
//   - minimal:   ×1.5
//   - standard:  ×1.0
//   - aggressive: ×0.5
// Showing this value helps users understand how their `max_lines_per_result`
// will actually behave under the chosen intensity.
// ---------------------------------------------------------------------------

function effectiveMaxLines(base: number, intensity: string): number {
	const v = Number(base);
	if (!Number.isFinite(v) || v <= 0) return 0;
	switch (intensity) {
		case "minimal":
			return Math.max(1, Math.round(v * 1.5));
		case "aggressive":
			return Math.max(1, Math.round(v * 0.5));
		default:
			return Math.max(1, Math.round(v));
	}
}

// ---------------------------------------------------------------------------
// RTK_INTENSITY_PRESETS — bundled one-click presets. Values are tuned to match
// the existing tests' expectations and the documented defaults in config.go.
// Clicking a preset writes the relevant fields; the user can still tweak any
// field afterwards (the buttons are "jump-start", not "lock-in").
// ---------------------------------------------------------------------------

interface IntensityPreset {
	id: Intensity;
	intensity: Intensity;
	max_lines_per_result: number;
	max_chars_per_result: number;
	dedup_threshold: number;
	enable_grouping: boolean;
	grouping_threshold: number;
	min_tokens_to_compress: number;
}

const RTK_INTENSITY_PRESETS: Record<Intensity, IntensityPreset> = {
	minimal: {
		id: "minimal",
		intensity: "minimal",
		max_lines_per_result: 240,
		max_chars_per_result: 24000,
		dedup_threshold: 5,
		enable_grouping: false,
		grouping_threshold: 5,
		min_tokens_to_compress: 4000,
	},
	standard: {
		id: "standard",
		intensity: "standard",
		max_lines_per_result: 120,
		max_chars_per_result: 12000,
		dedup_threshold: 3,
		enable_grouping: false,
		grouping_threshold: 3,
		min_tokens_to_compress: 2000,
	},
	aggressive: {
		id: "aggressive",
		intensity: "aggressive",
		max_lines_per_result: 60,
		max_chars_per_result: 6000,
		dedup_threshold: 3,
		enable_grouping: true,
		grouping_threshold: 3,
		min_tokens_to_compress: 1000,
	},
};

// PRESET_FIELDS — the 7 fields that quick presets touch. Used by revertPreset
// to reset only these fields to their last-saved values without disturbing
// independent settings (scope, snapshot, renderers, etc.).
const PRESET_FIELDS: (keyof IntensityPreset & keyof RTKFormValues)[] = [
	"intensity",
	"max_lines_per_result",
	"max_chars_per_result",
	"dedup_threshold",
	"enable_grouping",
	"grouping_threshold",
	"min_tokens_to_compress",
];

// ---------------------------------------------------------------------------
// MonitoringPanel — process-lifetime compression counters. Polls the RTK
// stats endpoint every 5s so the figures stay current without a refresh.
// Mirrors the pattern used by ProvidercooldownFragment.MonitoringPanel:
// a header, three stat cards, and an empty-state copy for a freshly-started
// gateway. The stats live in process memory and reset on gateway restart
// (matching the provider-cooldown semantics) — the empty-state copy makes
// that explicit so operators don't expect historical totals.
//
// The raw-output link points at the dedicated /workspace/plugins/rtk/raw-output
// sub-page so the operator can drill from "X requests compressed" to "show
// me what got rewritten".
// ---------------------------------------------------------------------------

function formatCompactNumber(value: number): string {
	if (!Number.isFinite(value) || value <= 0) return "0";
	if (value >= 1_000_000) {
		return `${(value / 1_000_000).toFixed(value >= 10_000_000 ? 0 : 1)}M`;
	}
	if (value >= 1_000) {
		return `${(value / 1_000).toFixed(value >= 10_000 ? 0 : 1)}k`;
	}
	return String(value);
}

function formatRatio(ratio: number): string {
	if (!Number.isFinite(ratio) || ratio <= 0) return "0%";
	const pct = Math.min(100, Math.max(0, ratio * 100));
	return `${pct.toFixed(pct >= 10 ? 0 : 1)}%`;
}

export function MonitoringPanel() {
	const { t } = useTranslation("plugins");
	const { data, isLoading } = useGetRtkStatsQuery(undefined, {
		pollingInterval: 5000,
	});

	const stats = data?.stats;
	const noActivity = !stats || (stats.invocations === 0 && stats.compressedCount === 0);

	if (isLoading && !stats) {
		return <div className="text-muted-foreground py-4 text-sm">{t("rtk.monitoringLoading")}</div>;
	}

	return (
		<div className="space-y-4" data-testid="rtk-monitoring-panel">
			<div className="grid grid-cols-2 gap-3 md:grid-cols-4">
				<div data-testid="rtk-stats-invocations" className="rounded-lg border p-4 text-center">
					<div className="text-2xl font-bold">{formatCompactNumber(stats?.invocations ?? 0)}</div>
					<div className="text-muted-foreground text-xs">{t("rtk.totalInvocations")}</div>
				</div>
				<div data-testid="rtk-stats-compressed" className="rounded-lg border p-4 text-center">
					<div className="text-2xl font-bold">{formatCompactNumber(stats?.compressedCount ?? 0)}</div>
					<div className="text-muted-foreground text-xs">{t("rtk.compressedRequests")}</div>
				</div>
				<div data-testid="rtk-stats-tokens-saved" className="rounded-lg border p-4 text-center">
					<div className="text-2xl font-bold">{formatCompactNumber(stats?.tokensSaved ?? 0)}</div>
					<div className="text-muted-foreground text-xs">{t("rtk.tokensSaved")}</div>
				</div>
				<div data-testid="rtk-stats-compression-ratio" className="rounded-lg border p-4 text-center">
					<div className="text-2xl font-bold">{formatRatio(stats?.compressionRatio ?? 0)}</div>
					<div className="text-muted-foreground text-xs">{t("rtk.compressionRatio")}</div>
				</div>
			</div>

			{noActivity && (
				<p className="text-muted-foreground text-center text-xs" data-testid="rtk-stats-empty">
					{t("rtk.noDataYet")}
				</p>
			)}
		</div>
	);
}

// ---------------------------------------------------------------------------
// HelpHint — small (?) icon with a tooltip; uses the shared Tooltip primitive.
// Used inline next to field labels to surface "when to change this" hints
// without bloating the FormDescription text.
// ---------------------------------------------------------------------------

function HelpHint({ children }: { children: React.ReactNode }) {
	return (
		<TooltipProvider delayDuration={150}>
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
		</TooltipProvider>
	);
}

// ---------------------------------------------------------------------------
// IntensityPresetButtons — 3-button preset picker. Clicking writes the preset
// values into the form via setValue. The currently-selected preset is
// highlighted; if the user customizes any of the affected fields, the highlight
// disappears (computed from live form values).
// ---------------------------------------------------------------------------

function IntensityPresetButtons({
	intensity,
	onPick,
	disabled,
}: {
	intensity: string;
	onPick: (preset: IntensityPreset) => void;
	disabled?: boolean;
}) {
	const { t } = useTranslation("plugins");
	const order: Intensity[] = ["minimal", "standard", "aggressive"];
	return (
		<div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
			{order.map((id) => {
				const active = intensity === id;
				return (
					<Button
						key={id}
						type="button"
						variant={active ? "default" : "outline"}
						className="h-auto justify-start px-4 py-3 text-left"
						onClick={() => onPick(RTK_INTENSITY_PRESETS[id])}
						disabled={disabled}
						data-testid={`rtk-preset-${id}`}
					>
						<div className="flex w-full flex-col items-start gap-1">
							<div className="flex items-center gap-2">
								<span className="text-sm font-semibold">{t(`rtk.intensity${id.charAt(0).toUpperCase()}${id.slice(1)}`)}</span>
								{active && (
									<Badge variant="secondary" className="text-[10px]">
										{t("rtk.presetActive")}
									</Badge>
								)}
							</div>
							<span className={`text-xs font-normal ${active ? "text-primary-foreground/80" : "text-muted-foreground"}`}>
								{t(`rtk.preset${id.charAt(0).toUpperCase()}${id.slice(1)}Hint`)}
							</span>
						</div>
					</Button>
				);
			})}
		</div>
	);
}

// ---------------------------------------------------------------------------
// EnabledSwitch — top-level on/off toggle with RBAC gating. Mirrors the
// ProvidercooldownFragment.EnabledSwitch UX: clicking flips the boolean
// immediately and toasts success without a separate Save step. Only the
// outer plugin.enabled flag is sent; the inner Config.Enabled is recovered
// on the server by applyConfigDefaults' zero-detect (see plugins/rtk/
// config.go), so a config_json=null/{} row still boots RTK enabled.
//
// We deliberately do NOT round-trip the existing config payload here —
// doing so would force the operator's untuned toggles (intensity,
// thresholds, renderers whitelist, etc.) to be persisted as part of a
// pure enable/disable action, which is surprising. If those knobs need to
// move, the operator edits them via ConfigForm and clicks Save.
// ---------------------------------------------------------------------------

export function EnabledSwitch({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const handleToggle = async (checked: boolean) => {
		if (!hasUpdateAccess) return;
		try {
			await updatePlugin({
				name: RTK_PLUGIN,
				data: { enabled: checked },
			}).unwrap();
			toast.success(checked ? t("rtk.enabledToast") : t("rtk.disabledToast"));
		} catch {
			toast.error(t("rtk.updateFailedToast"));
		}
	};

	return (
		<div className="rounded-lg border p-4">
			<div className="flex flex-row items-center justify-between">
				<div className="space-y-0.5">
					<label className="text-sm font-medium">{t("rtk.enableTitle")}</label>
					<p className="text-muted-foreground text-sm">{t("rtk.enableDescription")}</p>
				</div>
				<Switch
					data-testid="rtk-enabled-switch"
					checked={plugin.enabled}
					onCheckedChange={handleToggle}
					disabled={isLoading || !hasUpdateAccess}
				/>
			</div>
		</div>
	);
}

// ---------------------------------------------------------------------------
// ConfigForm — react-hook-form + zod for all RTK config fields. Reorganized
// from a flat 8-section layout into a guided flow: intro card → presets →
// daily-tune sections → collapsed advanced sections → save.
// ---------------------------------------------------------------------------

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const pluginConfig = (plugin.config || {}) as Record<string, any>;

	const form = useForm<RTKFormValues>({
		resolver: zodResolver(rtkConfigSchema),
		defaultValues: {
			intensity: pluginConfig.intensity ?? "standard",
			apply_to_tool_results: pluginConfig.apply_to_tool_results ?? true,
			apply_to_code_blocks: pluginConfig.apply_to_code_blocks ?? false,
			apply_to_assistant_messages: pluginConfig.apply_to_assistant_messages ?? false,
			max_lines_per_result: pluginConfig.max_lines_per_result ?? 120,
			max_chars_per_result: pluginConfig.max_chars_per_result ?? 12000,
			dedup_threshold: pluginConfig.dedup_threshold ?? 3,
			preserve_cache_control: pluginConfig.preserve_cache_control ?? false,
			enable_grouping: pluginConfig.enable_grouping ?? false,
			grouping_threshold: pluginConfig.grouping_threshold ?? 3,
			custom_filters_enabled: pluginConfig.custom_filters_enabled ?? true,
			trust_project_filters: pluginConfig.trust_project_filters ?? false,
			enabled_filters: pluginConfig.enabled_filters ?? [],
			disabled_filters: pluginConfig.disabled_filters ?? [],
			raw_output_retention: pluginConfig.raw_output_retention ?? "never",
			raw_output_max_bytes: pluginConfig.raw_output_max_bytes ?? 1048576,
			pipeline: pluginConfig.pipeline ?? [{ id: "rtk" }],
			min_tokens_to_compress: pluginConfig.min_tokens_to_compress ?? 0,
			enable_renderers: pluginConfig.enable_renderers ?? true,
			renderers: pluginConfig.renderers ?? [],
			snapshot_mode: pluginConfig.snapshot_mode ?? "off",
			snapshot_max_bytes: pluginConfig.snapshot_max_bytes ?? 30 * 1024,
		},
	});

	// Keep the form in sync if the underlying plugin/config changes (e.g. after
	// an admin edit or reload from server). Without this, switching between
	// plugins in the sidebar leaves stale defaults visible.
	//
	// Dependency is a JSON fingerprint, not the plugin.config object
	// reference — the parent re-renders frequently with a fresh object
	// identity even when the underlying fields are unchanged, which used
	// to call form.reset() and clobber unsaved edits (including the new
	// enabled toggle). Stringifying gives a stable signal that only fires
	// when the operator-visible config actually moves.
	useEffect(() => {
		form.reset({
			intensity: pluginConfig.intensity ?? "standard",
			apply_to_tool_results: pluginConfig.apply_to_tool_results ?? true,
			apply_to_code_blocks: pluginConfig.apply_to_code_blocks ?? false,
			apply_to_assistant_messages: pluginConfig.apply_to_assistant_messages ?? false,
			max_lines_per_result: pluginConfig.max_lines_per_result ?? 120,
			max_chars_per_result: pluginConfig.max_chars_per_result ?? 12000,
			dedup_threshold: pluginConfig.dedup_threshold ?? 3,
			preserve_cache_control: pluginConfig.preserve_cache_control ?? false,
			enable_grouping: pluginConfig.enable_grouping ?? false,
			grouping_threshold: pluginConfig.grouping_threshold ?? 3,
			custom_filters_enabled: pluginConfig.custom_filters_enabled ?? true,
			trust_project_filters: pluginConfig.trust_project_filters ?? false,
			enabled_filters: pluginConfig.enabled_filters ?? [],
			disabled_filters: pluginConfig.disabled_filters ?? [],
			raw_output_retention: pluginConfig.raw_output_retention ?? "never",
			raw_output_max_bytes: pluginConfig.raw_output_max_bytes ?? 1048576,
			pipeline: pluginConfig.pipeline ?? [{ id: "rtk" }],
			min_tokens_to_compress: pluginConfig.min_tokens_to_compress ?? 0,
			enable_renderers: pluginConfig.enable_renderers ?? true,
			renderers: pluginConfig.renderers ?? [],
			snapshot_mode: pluginConfig.snapshot_mode ?? "off",
			snapshot_max_bytes: pluginConfig.snapshot_max_bytes ?? 30 * 1024,
		});
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [JSON.stringify(pluginConfig)]);

	// lastSavedRef tracks the most recently saved values so revertPreset can
	// reset only the 7 preset-managed fields without touching independent ones.
	const lastSavedRef = useRef<Partial<RTKFormValues>>({});
	useEffect(() => {
		lastSavedRef.current = form.formState.defaultValues as Partial<RTKFormValues>;
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [JSON.stringify(pluginConfig)]);

	const onSubmit = async (values: RTKFormValues) => {
		if (!hasUpdateAccess) return;
		try {
			await updatePlugin({
				name: RTK_PLUGIN,
				data: {
					enabled: plugin.enabled,
					config: values,
				},
			}).unwrap();
			lastSavedRef.current = values;
			toast.success(t("rtk.savedToast"));
		} catch {
			toast.error(t("rtk.saveFailedToast"));
		}
	};

	const onError = () => {
		toast.error(t("rtk.formErrorToast"));
	};

	// Watch intensity + max_lines to display the live "effective value" hint.
	const intensity = form.watch("intensity") ?? "standard";
	const maxLines = form.watch("max_lines_per_result");
	const effectiveLines = useMemo(() => effectiveMaxLines(Number(maxLines) || 0, String(intensity)), [maxLines, intensity]);

	const applyPreset = (preset: IntensityPreset) => {
		form.setValue("intensity", preset.intensity, { shouldDirty: true });
		form.setValue("max_lines_per_result", preset.max_lines_per_result, { shouldDirty: true });
		form.setValue("max_chars_per_result", preset.max_chars_per_result, { shouldDirty: true });
		form.setValue("dedup_threshold", preset.dedup_threshold, { shouldDirty: true });
		form.setValue("enable_grouping", preset.enable_grouping, { shouldDirty: true });
		form.setValue("grouping_threshold", preset.grouping_threshold, { shouldDirty: true });
		form.setValue("min_tokens_to_compress", preset.min_tokens_to_compress, { shouldDirty: true });
		toast.info(t("rtk.presetAppliedToast", { name: t(`rtk.intensity${preset.id.charAt(0).toUpperCase()}${preset.id.slice(1)}`) }));
	};

	const revertPreset = () => {
		const saved = lastSavedRef.current;
		if (!saved) return;
		PRESET_FIELDS.forEach((field) => {
			form.setValue(field, saved[field] as any, { shouldDirty: true });
		});
		toast.info(t("rtk.revertPresetToast"));
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit, onError)}>
				<div className="space-y-6">
					{/* ── Intro card: "What is RTK?" + quick presets + revert button ───── */}
					<Card data-testid="rtk-intro-card">
						<CardHeader className="pb-3">
							<div className="flex items-center gap-2">
								<Info className="text-muted-foreground h-4 w-4" />
								<CardTitle className="text-base">{t("rtk.introTitle")}</CardTitle>
							</div>
							<CardDescription>{t("rtk.introDescription")}</CardDescription>
						</CardHeader>
						<CardContent className="space-y-3">
							<ul className="text-muted-foreground list-disc space-y-1 pl-5 text-sm">
								<li>{t("rtk.introBullet1")}</li>
								<li>{t("rtk.introBullet2")}</li>
								<li>{t("rtk.introBullet3")}</li>
							</ul>
							<Separator />
							<div className="space-y-2">
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-2">
										<span className="text-sm font-medium">{t("rtk.presetSectionLabel")}</span>
										<HelpHint>{t("rtk.presetHelp")}</HelpHint>
									</div>
								</div>
								<div className="flex items-end gap-2">
									<div className="min-w-0 flex-1">
										<IntensityPresetButtons intensity={intensity} onPick={applyPreset} disabled={!hasUpdateAccess} />
									</div>
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={revertPreset}
										disabled={!hasUpdateAccess}
										data-testid="rtk-revert-preset-btn"
										className="shrink-0"
									>
										<Undo2 className="h-4 w-4" />
										{t("rtk.revertPreset")}
									</Button>
								</div>
							</div>
						</CardContent>
					</Card>

					{/* ── ① Managed by Quick Presets (7 fields) ───────────────────────── */}
					<fieldset className="rounded-lg border p-4" data-testid="rtk-preset-managed-section">
						<legend className="bg-background px-2 text-sm font-semibold">{t("rtk.presetManagedSection")}</legend>
						<div className="mt-2 space-y-4">
							<FormField
								control={form.control}
								name="intensity"
								render={({ field }) => (
									<FormItem>
										<div className="flex items-center gap-1.5">
											<FormLabel>{t("rtk.intensityLabel")}</FormLabel>
											<HelpHint>{t("rtk.intensityWhen")}</HelpHint>
										</div>
										<Select value={field.value} onValueChange={field.onChange}>
											<FormControl>
												<SelectTrigger data-testid="rtk-field-intensity">
													<SelectValue placeholder={t("rtk.intensityPlaceholder")} />
												</SelectTrigger>
											</FormControl>
											<SelectContent>
												<SelectItem value="minimal">{t("rtk.intensityMinimal")}</SelectItem>
												<SelectItem value="standard">{t("rtk.intensityStandard")}</SelectItem>
												<SelectItem value="aggressive">{t("rtk.intensityAggressive")}</SelectItem>
											</SelectContent>
										</Select>
										<FormDescription>{t("rtk.intensityDescription")}</FormDescription>
										<FormMessage />
									</FormItem>
								)}
							/>
							<div className="grid grid-cols-1 gap-4 md:grid-cols-3">
								<FormField
									control={form.control}
									name="max_lines_per_result"
									render={({ field }) => (
										<FormItem>
											<div className="flex items-center gap-1.5">
												<FormLabel>{t("rtk.maxLinesLabel")}</FormLabel>
												<HelpHint>{t("rtk.maxLinesWhen")}</HelpHint>
											</div>
											<FormControl>
												<Input
													data-testid="rtk-field-max-lines"
													type="number"
													min={0}
													{...field}
													onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
												/>
											</FormControl>
											<FormDescription>
												{t("rtk.maxLinesDescription")}
												{Number(maxLines) > 0 && (
													<>
														{" · "}
														<span className="font-medium">{t("rtk.effectiveValue", { value: effectiveLines })}</span>
													</>
												)}
											</FormDescription>
											<FormMessage />
										</FormItem>
									)}
								/>
								<FormField
									control={form.control}
									name="max_chars_per_result"
									render={({ field }) => (
										<FormItem>
											<div className="flex items-center gap-1.5">
												<FormLabel>{t("rtk.maxCharsLabel")}</FormLabel>
												<HelpHint>{t("rtk.maxCharsWhen")}</HelpHint>
											</div>
											<FormControl>
												<Input
													data-testid="rtk-field-max-chars"
													type="number"
													min={0}
													{...field}
													onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
												/>
											</FormControl>
											<FormDescription>{t("rtk.maxCharsDescription")}</FormDescription>
											<FormMessage />
										</FormItem>
									)}
								/>
								<FormField
									control={form.control}
									name="dedup_threshold"
									render={({ field }) => (
										<FormItem>
											<div className="flex items-center gap-1.5">
												<FormLabel>{t("rtk.dedupThresholdLabel")}</FormLabel>
												<HelpHint>{t("rtk.dedupThresholdWhen")}</HelpHint>
											</div>
											<FormControl>
												<Input
													data-testid="rtk-field-dedup-threshold"
													type="number"
													min={0}
													{...field}
													onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
												/>
											</FormControl>
											<FormDescription>{t("rtk.dedupThresholdDescription")}</FormDescription>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
							<FormField
								control={form.control}
								name="enable_grouping"
								render={({ field }) => (
									<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
										<div className="space-y-0.5">
											<div className="flex items-center gap-1.5">
												<FormLabel>{t("rtk.enableGroupingLabel")}</FormLabel>
												<HelpHint>{t("rtk.enableGroupingWhen")}</HelpHint>
											</div>
											<FormDescription>{t("rtk.enableGroupingDescription")}</FormDescription>
										</div>
										<FormControl>
											<Switch data-testid="rtk-field-enable-grouping" checked={field.value} onCheckedChange={field.onChange} />
										</FormControl>
									</FormItem>
								)}
							/>
							{form.watch("enable_grouping") && (
								<FormField
									control={form.control}
									name="grouping_threshold"
									render={({ field }) => (
										<FormItem>
											<div className="flex items-center gap-1.5">
												<FormLabel>{t("rtk.groupingThresholdLabel")}</FormLabel>
												<HelpHint>{t("rtk.groupingThresholdWhen")}</HelpHint>
											</div>
											<FormControl>
												<Input
													data-testid="rtk-field-grouping-threshold"
													type="number"
													min={0}
													{...field}
													onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
												/>
											</FormControl>
											<FormDescription>{t("rtk.groupingThresholdDescription")}</FormDescription>
											<FormMessage />
										</FormItem>
									)}
								/>
							)}
							<FormField
								control={form.control}
								name="min_tokens_to_compress"
								render={({ field }) => (
									<FormItem>
										<div className="flex items-center gap-1.5">
											<FormLabel>{t("rtk.minTokensToCompressLabel")}</FormLabel>
											<HelpHint>{t("rtk.minTokensToCompressWhen")}</HelpHint>
										</div>
										<FormControl>
											<Input
												data-testid="rtk-field-min-tokens"
												type="number"
												min={0}
												{...field}
												onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
											/>
										</FormControl>
										<FormDescription>{t("rtk.minTokensToCompressDescription")}</FormDescription>
										<FormMessage />
									</FormItem>
								)}
							/>
						</div>
					</fieldset>

					{/* ── ② Independent Advanced Settings ────────────────────────────── */}
					<fieldset className="rounded-lg border p-4" data-testid="rtk-independent-section">
						<legend className="bg-background px-2 text-sm font-semibold">{t("rtk.independentAdvancedSection")}</legend>
						<div className="mt-2 space-y-4">
							{/* Scope: 3 checkboxes */}
							<fieldset className="rounded-lg border p-4">
								<legend className="bg-background px-2 text-xs font-semibold">{t("rtk.scopeSection")}</legend>
								<div className="mt-2 space-y-3">
									<FormField
										control={form.control}
										name="apply_to_tool_results"
										render={({ field }) => (
											<FormItem className="flex flex-row items-start space-y-0 space-x-3">
												<FormControl>
													<Checkbox data-testid="rtk-field-apply-to-tool-results" checked={field.value} onCheckedChange={field.onChange} />
												</FormControl>
												<div className="space-y-1 leading-none">
													<div className="flex items-center gap-1.5">
														<FormLabel>{t("rtk.applyToToolResultsLabel")}</FormLabel>
														<HelpHint>{t("rtk.applyToToolResultsWhen")}</HelpHint>
													</div>
													<FormDescription>{t("rtk.applyToToolResultsDescription")}</FormDescription>
												</div>
											</FormItem>
										)}
									/>
									<FormField
										control={form.control}
										name="apply_to_code_blocks"
										render={({ field }) => (
											<FormItem className="flex flex-row items-start space-y-0 space-x-3">
												<FormControl>
													<Checkbox data-testid="rtk-field-apply-to-code-blocks" checked={field.value} onCheckedChange={field.onChange} />
												</FormControl>
												<div className="space-y-1 leading-none">
													<div className="flex items-center gap-1.5">
														<FormLabel>{t("rtk.applyToCodeBlocksLabel")}</FormLabel>
														<HelpHint>{t("rtk.applyToCodeBlocksWhen")}</HelpHint>
													</div>
													<FormDescription>{t("rtk.applyToCodeBlocksDescription")}</FormDescription>
												</div>
											</FormItem>
										)}
									/>
									<FormField
										control={form.control}
										name="apply_to_assistant_messages"
										render={({ field }) => (
											<FormItem className="flex flex-row items-start space-y-0 space-x-3">
												<FormControl>
													<Checkbox
														data-testid="rtk-field-apply-to-assistant-messages"
														checked={field.value}
														onCheckedChange={field.onChange}
													/>
												</FormControl>
												<div className="space-y-1 leading-none">
													<div className="flex items-center gap-1.5">
														<FormLabel>{t("rtk.applyToAssistantMessagesLabel")}</FormLabel>
														<HelpHint>{t("rtk.applyToAssistantMessagesWhen")}</HelpHint>
													</div>
													<FormDescription>{t("rtk.applyToAssistantMessagesDescription")}</FormDescription>
												</div>
											</FormItem>
										)}
									/>
								</div>
							</fieldset>

							{/* Snapshot: mode + max bytes */}
							<fieldset className="rounded-lg border p-4">
								<legend className="bg-background px-2 text-xs font-semibold">{t("rtk.snapshotSection")}</legend>
								<div className="mt-2 grid grid-cols-1 gap-4 md:grid-cols-2">
									<FormField
										control={form.control}
										name="snapshot_mode"
										render={({ field }) => (
											<FormItem>
												<div className="flex items-center gap-1.5">
													<FormLabel>{t("rtk.snapshotModeLabel")}</FormLabel>
													<HelpHint>{t("rtk.snapshotModeWhen")}</HelpHint>
												</div>
												<Select value={field.value} onValueChange={field.onChange}>
													<FormControl>
														<SelectTrigger data-testid="rtk-field-snapshot-mode">
															<SelectValue />
														</SelectTrigger>
													</FormControl>
													<SelectContent>
														<SelectItem value="off">{t("rtk.snapshotModeOff")}</SelectItem>
														<SelectItem value="split">{t("rtk.snapshotModeSplit")}</SelectItem>
														<SelectItem value="merged">{t("rtk.snapshotModeMerged")}</SelectItem>
													</SelectContent>
												</Select>
												<FormDescription>{t("rtk.snapshotModeDescription")}</FormDescription>
												<FormMessage />
											</FormItem>
										)}
									/>
									<FormField
										control={form.control}
										name="snapshot_max_bytes"
										render={({ field }) => (
											<FormItem>
												<div className="flex items-center gap-1.5">
													<FormLabel>{t("rtk.snapshotMaxBytesLabel")}</FormLabel>
													<HelpHint>{t("rtk.snapshotMaxBytesWhen")}</HelpHint>
												</div>
												<FormControl>
													<Input
														data-testid="rtk-field-snapshot-max-bytes"
														type="number"
														min={0}
														step={1024}
														{...field}
														onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
													/>
												</FormControl>
												<FormDescription>{t("rtk.snapshotMaxBytesHelp")}</FormDescription>
												<FormMessage />
											</FormItem>
										)}
									/>
								</div>
							</fieldset>

							{/* Collapsible advanced */}
							<Accordion type="multiple" className="rounded-lg border">
								{/* Semantic renderers */}
								<AccordionItem value="renderers" className="px-4">
									<AccordionTrigger data-testid="rtk-section-renderers-trigger">{t("rtk.renderersSection")}</AccordionTrigger>
									<AccordionContent className="space-y-3">
										<FormField
											control={form.control}
											name="enable_renderers"
											render={({ field }) => (
												<FormItem className="flex flex-row items-start space-y-0 space-x-3">
													<FormControl>
														<Switch
															checked={Boolean(field.value)}
															onCheckedChange={field.onChange}
															data-testid="rtk-field-enable-renderers"
														/>
													</FormControl>
													<div className="space-y-1 leading-none">
														<div className="flex items-center gap-1.5">
															<FormLabel>{t("rtk.enableRenderersLabel")}</FormLabel>
															<HelpHint>{t("rtk.enableRenderersWhen")}</HelpHint>
														</div>
														<FormDescription>{t("rtk.enableRenderersDescription")}</FormDescription>
													</div>
												</FormItem>
											)}
										/>
										{form.watch("enable_renderers") && (
											<FormField
												control={form.control}
												name="renderers"
												render={({ field }) => (
													<FormItem>
														<div className="flex items-center gap-1.5">
															<FormLabel>{t("rtk.renderersLabel")}</FormLabel>
															<HelpHint>{t("rtk.renderersWhen")}</HelpHint>
														</div>
														<FormControl>
															<Input
																data-testid="rtk-field-renderers"
																placeholder="e.g. git-diff, test-green, terraform-plan"
																{...field}
																value={Array.isArray(field.value) ? field.value.join(", ") : ""}
																onChange={(e) => field.onChange(e.target.value ? e.target.value.split(/\s*,\s*/).filter(Boolean) : [])}
															/>
														</FormControl>
														<FormDescription>{t("rtk.renderersDescription")}</FormDescription>
														<FormMessage />
													</FormItem>
												)}
											/>
										)}
									</AccordionContent>
								</AccordionItem>

								{/* Debug / raw output */}
								<AccordionItem value="rawOutput" className="px-4">
									<AccordionTrigger data-testid="rtk-section-raw-output-trigger">{t("rtk.rawOutputSection")}</AccordionTrigger>
									<AccordionContent className="space-y-3">
										<FormField
											control={form.control}
											name="raw_output_retention"
											render={({ field }) => (
												<FormItem>
													<div className="flex items-center gap-1.5">
														<FormLabel>{t("rtk.rawOutputRetentionLabel")}</FormLabel>
														<HelpHint>{t("rtk.rawOutputRetentionWhen")}</HelpHint>
													</div>
													<Select value={field.value} onValueChange={field.onChange}>
														<FormControl>
															<SelectTrigger data-testid="rtk-field-raw-output-retention">
																<SelectValue placeholder={t("rtk.rawOutputRetentionPlaceholder")} />
															</SelectTrigger>
														</FormControl>
														<SelectContent>
															<SelectItem value="never">{t("rtk.rawOutputRetentionNever")}</SelectItem>
															<SelectItem value="failures">{t("rtk.rawOutputRetentionFailures")}</SelectItem>
															<SelectItem value="always">{t("rtk.rawOutputRetentionAlways")}</SelectItem>
														</SelectContent>
													</Select>
													<FormDescription>{t("rtk.rawOutputRetentionDescription")}</FormDescription>
													<FormMessage />
												</FormItem>
											)}
										/>
										<FormField
											control={form.control}
											name="raw_output_max_bytes"
											render={({ field }) => (
												<FormItem>
													<div className="flex items-center gap-1.5">
														<FormLabel>{t("rtk.rawOutputMaxBytesLabel")}</FormLabel>
														<HelpHint>{t("rtk.rawOutputMaxBytesWhen")}</HelpHint>
													</div>
													<FormControl>
														<Input
															data-testid="rtk-field-raw-output-max-bytes"
															type="number"
															min={0}
															{...field}
															onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
														/>
													</FormControl>
													<FormDescription>{t("rtk.rawOutputMaxBytesDescription")}</FormDescription>
													<FormMessage />
												</FormItem>
											)}
										/>
									</AccordionContent>
								</AccordionItem>

								{/* Filters & custom */}
								<AccordionItem value="filters" className="px-4">
									<AccordionTrigger data-testid="rtk-section-filters-trigger">{t("rtk.filtersSection")}</AccordionTrigger>
									<AccordionContent className="space-y-3">
										<FormField
											control={form.control}
											name="custom_filters_enabled"
											render={({ field }) => (
												<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
													<div className="space-y-0.5">
														<div className="flex items-center gap-1.5">
															<FormLabel>{t("rtk.customFiltersEnabledLabel")}</FormLabel>
															<HelpHint>{t("rtk.customFiltersEnabledWhen")}</HelpHint>
														</div>
														<FormDescription>{t("rtk.customFiltersEnabledDescription")}</FormDescription>
													</div>
													<FormControl>
														<Switch data-testid="rtk-field-custom-filters-enabled" checked={field.value} onCheckedChange={field.onChange} />
													</FormControl>
												</FormItem>
											)}
										/>
										{form.watch("custom_filters_enabled") && (
											<FormField
												control={form.control}
												name="trust_project_filters"
												render={({ field }) => (
													<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
														<div className="space-y-0.5">
															<div className="flex items-center gap-1.5">
																<FormLabel>{t("rtk.trustProjectFiltersLabel")}</FormLabel>
																<HelpHint>{t("rtk.trustProjectFiltersWhen")}</HelpHint>
															</div>
															<FormDescription>{t("rtk.trustProjectFiltersDescription")}</FormDescription>
														</div>
														<FormControl>
															<Switch
																data-testid="rtk-field-trust-project-filters"
																checked={field.value}
																onCheckedChange={field.onChange}
															/>
														</FormControl>
													</FormItem>
												)}
											/>
										)}
										<div className="text-muted-foreground text-xs">
											{t("rtk.filterCatalogHint")}{" "}
											<Link
												to="/workspace/plugins/rtk/filters"
												className="text-blue-600 underline-offset-2 hover:underline dark:text-blue-400"
											>
												{t("rtk.admin.tabs.filters")}
												<ExternalLink className="ml-0.5 inline h-3 w-3" />
											</Link>
										</div>
									</AccordionContent>
								</AccordionItem>

								{/* Advanced: pipeline + cache_control (min_tokens moved to presets) */}
								<AccordionItem value="advanced" className="px-4">
									<AccordionTrigger data-testid="rtk-section-advanced-trigger">{t("rtk.advancedSection")}</AccordionTrigger>
									<AccordionContent className="space-y-3">
										<FormField
											control={form.control}
											name="preserve_cache_control"
											render={({ field }) => (
												<FormItem className="flex flex-row items-start space-y-0 space-x-3">
													<FormControl>
														<Checkbox
															data-testid="rtk-field-preserve-cache-control"
															checked={field.value}
															onCheckedChange={field.onChange}
														/>
													</FormControl>
													<div className="space-y-1 leading-none">
														<div className="flex items-center gap-1.5">
															<FormLabel>{t("rtk.preserveCacheControlLabel")}</FormLabel>
															<HelpHint>{t("rtk.preserveCacheControlWhen")}</HelpHint>
														</div>
														<FormDescription>{t("rtk.preserveCacheControlDescription")}</FormDescription>
													</div>
												</FormItem>
											)}
										/>
										<FormField
											control={form.control}
											name="pipeline"
											render={({ field }) => (
												<FormItem>
													<div className="flex items-center gap-1.5">
														<FormLabel>{t("rtk.pipelineLabel")}</FormLabel>
														<HelpHint>{t("rtk.pipelineWhen")}</HelpHint>
													</div>
													<FormControl>
														<Textarea
															data-testid="rtk-field-pipeline"
															className="font-mono text-xs"
															rows={4}
															{...field}
															value={typeof field.value === "string" ? field.value : JSON.stringify(field.value, null, 2)}
															onChange={(e) => {
																try {
																	const parsed = JSON.parse(e.target.value);
																	field.onChange(parsed);
																} catch {
																	field.onChange(e.target.value);
																}
															}}
														/>
													</FormControl>
													<FormDescription>{t("rtk.pipelineDescription")}</FormDescription>
													<FormMessage />
												</FormItem>
											)}
										/>
									</AccordionContent>
								</AccordionItem>
							</Accordion>
						</div>
					</fieldset>

					{/* ── 效果验证侧边提示 ───────────────────────────────────────── */}
					<div className="bg-muted/30 text-muted-foreground rounded-lg border border-dashed p-3 text-xs">
						<div className="flex items-start gap-2">
							<Beaker className="mt-0.5 h-4 w-4 shrink-0" />
							<div className="space-y-1.5">
								<div className="text-foreground text-sm font-medium">{t("rtk.afterSaveTitle")}</div>
								<div>{t("rtk.afterSaveBody")}</div>
								<div className="flex flex-wrap gap-2 pt-1">
									<Button variant="outline" size="sm" asChild>
										<Link to="/workspace/plugins/rtk/preview" data-testid="rtk-after-save-preview">
											<ImageIcon className="h-3.5 w-3.5" />
											{t("rtk.admin.tabs.preview")}
										</Link>
									</Button>
									<Button variant="outline" size="sm" asChild>
										<Link to="/workspace/plugins/rtk/test" data-testid="rtk-after-save-test">
											<Beaker className="h-3.5 w-3.5" />
											{t("rtk.admin.tabs.test")}
										</Link>
									</Button>
									<Button variant="outline" size="sm" asChild>
										<Link to="/workspace/plugins/rtk/raw-output" data-testid="rtk-after-save-raw">
											<FileSearchIcon />
											{t("rtk.admin.tabs.rawOutput")}
										</Link>
									</Button>
								</div>
							</div>
						</div>
					</div>
				</div>

				{/* Save bar */}
				<div className="bg-background sticky bottom-0 z-10 -mx-4 mt-6 flex flex-wrap items-center justify-between gap-2 border-t px-4 pt-4 pb-2">
					<Button
						type="button"
						variant="ghost"
						size="sm"
						onClick={() => form.reset()}
						disabled={!form.formState.isDirty || !hasUpdateAccess}
						data-testid="rtk-reset-btn"
					>
						<RotateCcw className="h-4 w-4" />
						{t("rtk.reset")}
					</Button>
					<div className="flex flex-wrap items-center gap-2">
						<Button variant="outline" size="sm" asChild data-testid="rtk-admin-link-filters">
							<Link to="/workspace/plugins/rtk/filters">
								<FlaskConical className="h-4 w-4" />
								{t("rtk.admin.tabs.filters")}
							</Link>
						</Button>
						<Button variant="outline" size="sm" asChild data-testid="rtk-admin-link-test">
							<Link to="/workspace/plugins/rtk/test">
								<Beaker className="h-4 w-4" />
								{t("rtk.admin.tabs.test")}
							</Link>
						</Button>
						<Button type="submit" disabled={isLoading || !form.formState.isDirty || !hasUpdateAccess} data-testid="rtk-save-btn">
							{isLoading ? t("rtk.saving") : t("rtk.saveConfiguration")}
						</Button>
					</div>
				</div>
			</form>
		</Form>
	);
}

// Tiny inline icon component for the "Raw Output" link in the after-save card.
// Kept local to avoid adding another lucide import beyond what's already used.
function FileSearchIcon() {
	return (
		<svg
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			strokeWidth="2"
			strokeLinecap="round"
			strokeLinejoin="round"
			className="h-3.5 w-3.5"
		>
			<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
			<path d="M14 2v6h6" />
			<circle cx="11.5" cy="14.5" r="2.5" />
			<path d="M13.5 16.5 15 18" />
		</svg>
	);
}

// ---------------------------------------------------------------------------
// RtkFragment — MonitoringPanel + EnabledSwitch + ConfigForm. The on/off
// toggle is a dedicated switch (mirrors ProvidercooldownFragment.EnabledSwitch)
// so flipping it takes effect immediately without a separate Save click.
// Server-side, plugins/rtk/config.go's applyConfigDefaults zero-detect
// safeguard keeps RTK enabled even when the persisted config_json is null
// or all-zero — the inner Config.Enabled flag never gets a chance to silently
// turn the plugin into a no-op.
// ---------------------------------------------------------------------------

export function RtkFragment({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	return (
		<div data-testid="rtk-fragment" className="space-y-6">
			<div className="space-y-1">
				<h3 className="text-lg font-semibold">{t("rtk.title")}</h3>
				<p className="text-muted-foreground text-sm">{t("rtk.subtitle")}</p>
			</div>

			<div className="rounded-lg border p-4">
				<div className="mb-4 flex items-center gap-2">
					<Activity className="text-muted-foreground h-4 w-4" />
					<h4 className="text-sm font-medium">{t("rtk.monitoringTitle")}</h4>
				</div>
				<MonitoringPanel />
			</div>

			<EnabledSwitch plugin={plugin} />

			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("rtk.settingsTitle")}</h4>
				<ConfigForm plugin={plugin} />
			</div>
		</div>
	);
}

// Default export for convenience (also used by pluginsView.tsx)
export default RtkFragment;