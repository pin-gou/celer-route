import { Button } from "@/components/ui/button";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { TagInput } from "@/components/ui/tagInput";
import { useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { GOVERNANCE_PLUGIN, governanceConfigSchema, type Plugin } from "@/lib/types/plugins";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type GovernanceFormValues = z.infer<typeof governanceConfigSchema>;

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
				name: GOVERNANCE_PLUGIN,
				data: { enabled: checked },
			}).unwrap();
			toast.success(checked ? t("governance.enabledToast") : t("governance.disabledToast"));
		} catch {
			toast.error(t("governance.updateFailedToast"));
		}
	};

	return (
		<div className="rounded-lg border p-4">
			<div className="flex flex-row items-center justify-between">
				<div className="space-y-0.5">
					<label className="text-sm font-medium">{t("governance.enableTitle")}</label>
					<p className="text-muted-foreground text-sm">{t("governance.enableDescription")}</p>
				</div>
				<Switch
					data-testid="governance-enabled-switch"
					checked={plugin.enabled}
					onCheckedChange={handleToggle}
					disabled={isLoading || !hasUpdateAccess}
				/>
			</div>
		</div>
	);
}

// ---------------------------------------------------------------------------
// ConfigForm — react-hook-form + zod for the 4 governance config fields
// ---------------------------------------------------------------------------

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const pluginConfig = (plugin.config || {}) as Record<string, any>;

	const form = useForm<GovernanceFormValues>({
		resolver: zodResolver(governanceConfigSchema),
		defaultValues: {
			is_vk_mandatory: pluginConfig.is_vk_mandatory ?? false,
			required_headers: pluginConfig.required_headers ?? [],
			disable_auto_tool_inject: pluginConfig.disable_auto_tool_inject ?? false,
			routing_chain_max_depth: pluginConfig.routing_chain_max_depth ?? 5,
		},
	});

	const onSubmit = async (values: GovernanceFormValues) => {
		if (!hasUpdateAccess) return;
		try {
			await updatePlugin({
				name: GOVERNANCE_PLUGIN,
				data: {
					enabled: plugin.enabled,
					config: values,
				},
			}).unwrap();
			toast.success(t("governance.savedToast"));
			form.reset(values);
		} catch {
			toast.error(t("governance.updateFailedToast"));
		}
	};

	const onError = () => {
		toast.error(t("governance.formErrorToast"));
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit, onError)} className="space-y-6">
				{/* Fieldset 1: 访问控制 (Access Control) */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("governance.accessControlSection")}</legend>
					<div className="mt-2 space-y-4">
						<FormField
							control={form.control}
							name="is_vk_mandatory"
							render={({ field }) => (
								<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
									<div className="space-y-0.5">
										<FormLabel data-testid="governance-field-is-vk-mandatory-label">{t("governance.isVkMandatoryLabel")}</FormLabel>
										<FormDescription>{t("governance.isVkMandatoryDescription")}</FormDescription>
									</div>
									<FormControl>
										<Switch
											data-testid="governance-field-is-vk-mandatory"
											checked={Boolean(field.value)}
											onCheckedChange={field.onChange}
											disabled={!hasUpdateAccess}
										/>
									</FormControl>
								</FormItem>
							)}
						/>
						<FormField
							control={form.control}
							name="required_headers"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("governance.requiredHeadersLabel")}</FormLabel>
									<FormDescription>{t("governance.requiredHeadersDescription")}</FormDescription>
									<FormControl>
										<TagInput
											data-testid="governance-field-required-headers-input"
											value={Array.isArray(field.value) ? field.value : []}
											onValueChange={field.onChange}
											placeholder={t("governance.requiredHeadersPlaceholder")}
											disabled={!hasUpdateAccess}
										/>
									</FormControl>
									<FormMessage />
								</FormItem>
							)}
						/>
					</div>
				</fieldset>

				{/* Fieldset 2: 行为 (Behavior) */}
				<fieldset className="rounded-lg border p-4">
					<legend className="text-sm font-semibold">{t("governance.behaviorSection")}</legend>
					<div className="mt-2 space-y-4">
						<FormField
							control={form.control}
							name="disable_auto_tool_inject"
							render={({ field }) => (
								<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
									<div className="space-y-0.5">
										<FormLabel>{t("governance.disableAutoToolInjectLabel")}</FormLabel>
										<FormDescription>{t("governance.disableAutoToolInjectDescription")}</FormDescription>
									</div>
									<FormControl>
										<Switch
											data-testid="governance-field-disable-auto-tool-inject"
											checked={Boolean(field.value)}
											onCheckedChange={field.onChange}
											disabled={!hasUpdateAccess}
										/>
									</FormControl>
								</FormItem>
							)}
						/>
						<FormField
							control={form.control}
							name="routing_chain_max_depth"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("governance.routingChainMaxDepthLabel")}</FormLabel>
									<FormControl>
										<Input
											data-testid="governance-field-routing-chain-max-depth"
											type="number"
											min={1}
											max={100}
											disabled={!hasUpdateAccess}
											{...field}
											onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
										/>
									</FormControl>
									<FormDescription>{t("governance.routingChainMaxDepthDescription")}</FormDescription>
									<FormMessage />
								</FormItem>
							)}
						/>
					</div>
				</fieldset>

				{/* Save button */}
				<div className="flex justify-end">
					<Button type="submit" data-testid="governance-save-button" disabled={isLoading || !form.formState.isDirty || !hasUpdateAccess}>
						{isLoading ? t("governance.saving") : t("governance.saveConfiguration")}
					</Button>
				</div>
			</form>
		</Form>
	);
}

// ---------------------------------------------------------------------------
// GovernanceFragment — full two-section fragment (EnabledSwitch + ConfigForm)
// ---------------------------------------------------------------------------

export function GovernanceFragment({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	return (
		<div data-testid="governance-fragment" className="space-y-8">
			<h3 className="text-lg font-semibold">{t("governance.title")}</h3>

			{/* Section 1: enabled switch */}
			<EnabledSwitch plugin={plugin} />

			{/* Section 2: config form */}
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("governance.settingsTitle")}</h4>
				<ConfigForm plugin={plugin} />
			</div>
		</div>
	);
}

// Default export for convenience (also used by pluginsView.tsx)
export default GovernanceFragment;