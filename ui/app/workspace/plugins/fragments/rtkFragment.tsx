import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { RTK_PLUGIN, rtkConfigSchema, type Plugin } from "@/lib/types/plugins";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type RTKFormValues = z.input<typeof rtkConfigSchema>;

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
// ConfigForm — react-hook-form + zod for all RTK config fields
// ---------------------------------------------------------------------------

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const pluginConfig = (plugin.config || {}) as Record<string, any>;

	const form = useForm<RTKFormValues>({
		resolver: zodResolver(rtkConfigSchema),
		defaultValues: {
			enabled: plugin.enabled,
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
		},
	});

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
			toast.success(t("rtk.savedToast"));
		} catch {
			toast.error(t("rtk.saveFailedToast"));
		}
	};

	const onError = () => {
		toast.error(t("rtk.formErrorToast"));
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit, onError)} className="space-y-8">
				{/* Section 1: 启用与强度 */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("rtk.intensitySection")}</legend>
					<div className="mt-2 space-y-4">
						{/* intensity */}
						<FormField
							control={form.control}
							name="intensity"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("rtk.intensityLabel")}</FormLabel>
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
					</div>
				</fieldset>

				{/* Section 2: 行/字符上限 */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("rtk.limitsSection")}</legend>
					<div className="mt-2 grid grid-cols-1 gap-4 md:grid-cols-3">
						<FormField
							control={form.control}
							name="max_lines_per_result"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("rtk.maxLinesLabel")}</FormLabel>
									<FormControl>
										<Input
											data-testid="rtk-field-max-lines"
											type="number"
											min={0}
											{...field}
											onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
										/>
									</FormControl>
									<FormDescription>{t("rtk.maxLinesDescription")}</FormDescription>
									<FormMessage />
								</FormItem>
							)}
						/>
						<FormField
							control={form.control}
							name="max_chars_per_result"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("rtk.maxCharsLabel")}</FormLabel>
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
									<FormLabel>{t("rtk.dedupThresholdLabel")}</FormLabel>
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
				</fieldset>

				{/* Section 3: 作用范围 */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("rtk.scopeSection")}</legend>
					<div className="mt-2 space-y-4">
						<FormField
							control={form.control}
							name="apply_to_tool_results"
							render={({ field }) => (
								<FormItem className="flex flex-row items-start space-y-0 space-x-3">
									<FormControl>
										<Checkbox data-testid="rtk-field-apply-to-tool-results" checked={field.value} onCheckedChange={field.onChange} />
									</FormControl>
									<div className="space-y-1 leading-none">
										<FormLabel>{t("rtk.applyToToolResultsLabel")}</FormLabel>
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
										<FormLabel>{t("rtk.applyToCodeBlocksLabel")}</FormLabel>
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
										<Checkbox data-testid="rtk-field-apply-to-assistant-messages" checked={field.value} onCheckedChange={field.onChange} />
									</FormControl>
									<div className="space-y-1 leading-none">
										<FormLabel>{t("rtk.applyToAssistantMessagesLabel")}</FormLabel>
										<FormDescription>{t("rtk.applyToAssistantMessagesDescription")}</FormDescription>
									</div>
								</FormItem>
							)}
						/>
					</div>
				</fieldset>

				{/* Section 4: 分组 */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("rtk.groupingSection")}</legend>
					<div className="mt-2 space-y-4">
						<FormField
							control={form.control}
							name="enable_grouping"
							render={({ field }) => (
								<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
									<div className="space-y-0.5">
										<FormLabel>{t("rtk.enableGroupingLabel")}</FormLabel>
										<FormDescription>{t("rtk.enableGroupingDescription")}</FormDescription>
									</div>
									<FormControl>
										<Switch data-testid="rtk-field-enable-grouping" checked={field.value} onCheckedChange={field.onChange} />
									</FormControl>
								</FormItem>
							)}
						/>
						<FormField
							control={form.control}
							name="grouping_threshold"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("rtk.groupingThresholdLabel")}</FormLabel>
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
					</div>
				</fieldset>

				{/* Section 5: 过滤器 */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("rtk.filtersSection")}</legend>
					<div className="mt-2 space-y-4">
						<FormField
							control={form.control}
							name="custom_filters_enabled"
							render={({ field }) => (
								<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
									<div className="space-y-0.5">
										<FormLabel>{t("rtk.customFiltersEnabledLabel")}</FormLabel>
										<FormDescription>{t("rtk.customFiltersEnabledDescription")}</FormDescription>
									</div>
									<FormControl>
										<Switch data-testid="rtk-field-custom-filters-enabled" checked={field.value} onCheckedChange={field.onChange} />
									</FormControl>
								</FormItem>
							)}
						/>
						<FormField
							control={form.control}
							name="trust_project_filters"
							render={({ field }) => (
								<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
									<div className="space-y-0.5">
										<FormLabel>{t("rtk.trustProjectFiltersLabel")}</FormLabel>
										<FormDescription>{t("rtk.trustProjectFiltersDescription")}</FormDescription>
									</div>
									<FormControl>
										<Switch data-testid="rtk-field-trust-project-filters" checked={field.value} onCheckedChange={field.onChange} />
									</FormControl>
								</FormItem>
							)}
						/>
					</div>
				</fieldset>

				{/* Section 6b: 语义渲染器 (Semantic Renderers) */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("rtk.renderersSection")}</legend>
					<div className="mt-2 space-y-4">
						<FormField
							control={form.control}
							name="enable_renderers"
							render={({ field }) => (
								<FormItem className="flex flex-row items-start space-y-0 space-x-3">
									<FormControl>
										<Switch checked={Boolean(field.value)} onCheckedChange={field.onChange} data-testid="rtk-field-enable-renderers" />
									</FormControl>
									<div className="space-y-1 leading-none">
										<FormLabel>{t("rtk.enableRenderersLabel")}</FormLabel>
										<FormDescription>{t("rtk.enableRenderersDescription")}</FormDescription>
									</div>
								</FormItem>
							)}
						/>
						<FormField
							control={form.control}
							name="renderers"
							render={({ field }) => {
								const selected = Array.isArray(field.value) ? field.value : [];
								const options = [
									"git-diff",
									"test-pytest",
									"test-jest",
									"test-vitest",
									"test-go",
									"build-eslint",
									"terraform-plan",
									"tofu-plan",
									"aws",
									"json-output",
								];
								return (
									<FormItem>
										<FormLabel>{t("rtk.renderersLabel")}</FormLabel>
										<FormDescription>{t("rtk.renderersDescription")}</FormDescription>
										<div className="mt-2 grid grid-cols-2 gap-2">
											{options.map((opt) => {
												const checked = selected.includes(opt);
												return (
													<label key={opt} className="flex items-center gap-2 text-sm" data-testid={`rtk-field-renderer-${opt}`}>
														<Checkbox
															checked={checked}
															onCheckedChange={(v) => {
																if (v) {
																	field.onChange([...selected, opt]);
																} else {
																	field.onChange(selected.filter((s: string) => s !== opt));
																}
															}}
														/>
														<span>{opt}</span>
													</label>
												);
											})}
										</div>
										<FormMessage />
									</FormItem>
								);
							}}
						/>
					</div>
				</fieldset>

				{/* Section 7: 原始输出 */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("rtk.rawOutputSection")}</legend>
					<div className="mt-2 space-y-4">
						<FormField
							control={form.control}
							name="raw_output_retention"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("rtk.rawOutputRetentionLabel")}</FormLabel>
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
									<FormLabel>{t("rtk.rawOutputMaxBytesLabel")}</FormLabel>
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
					</div>
				</fieldset>

				{/* Section 8: 高级 */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("rtk.advancedSection")}</legend>
					<div className="mt-2 space-y-4">
						{/* pipeline as JSON textarea */}
						<FormField
							control={form.control}
							name="pipeline"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("rtk.pipelineLabel")}</FormLabel>
									<FormControl>
										<Textarea
											data-testid="rtk-field-pipeline"
											className="font-mono text-xs"
											rows={4}
											{...field}
											value={typeof field.value === "string" ? field.value : JSON.stringify(field.value, null, 2)}
											onChange={(e) => {
												// Try to parse as JSON; if it fails, store as string for editing
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
						<FormField
							control={form.control}
							name="min_tokens_to_compress"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("rtk.minTokensToCompressLabel")}</FormLabel>
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

				{/* Save button */}
				<div className="flex justify-end">
					<Button type="submit" disabled={isLoading || !form.formState.isDirty || !hasUpdateAccess}>
						{isLoading ? t("rtk.saving") : t("rtk.saveConfiguration")}
					</Button>
				</div>
			</form>
		</Form>
	);
}

// ---------------------------------------------------------------------------
// RtkFragment — full two-section fragment (EnabledSwitch + ConfigForm)
// ---------------------------------------------------------------------------

export function RtkFragment({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	return (
		<div className="space-y-8">
			<h3 className="text-lg font-semibold">{t("rtk.title")}</h3>

			{/* Section 1: enabled switch */}
			<EnabledSwitch plugin={plugin} />

			{/* Section 2: config form */}
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("rtk.settingsTitle")}</h4>
				<ConfigForm plugin={plugin} />
			</div>
		</div>
	);
}

// Default export for convenience (also used by pluginsView.tsx)
export default RtkFragment;