import { Button } from "@/components/ui/button";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Switch } from "@/components/ui/switch";
import { TagInput } from "@/components/ui/tagInput";
import { useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { LOGGING_PLUGIN, loggingConfigSchema, pluginFragmentLabels, type Plugin } from "@/lib/types/plugins";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";

type LoggingFormValues = z.infer<typeof loggingConfigSchema>;

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const pluginConfig = (plugin.config || {}) as Record<string, any>;

	const form = useForm<LoggingFormValues>({
		resolver: zodResolver(loggingConfigSchema),
		defaultValues: {
			disable_content_logging: pluginConfig.disable_content_logging ?? false,
			retain_content_in_object_storage: pluginConfig.retain_content_in_object_storage ?? false,
			allow_per_request_content_storage_override: pluginConfig.allow_per_request_content_storage_override ?? false,
			logging_headers: pluginConfig.logging_headers ?? [],
		},
	});

	const onSubmit = async (values: LoggingFormValues) => {
		if (!hasUpdateAccess) return;
		try {
			await updatePlugin({
				name: LOGGING_PLUGIN,
				data: {
					enabled: plugin.enabled,
					config: values,
				},
			}).unwrap();
			toast.success(t("loggingConfig.savedToast"));
			form.reset(values);
		} catch {
			toast.error(t("loggingConfig.saveFailedToast"));
		}
	};

	const onError = () => {
		toast.error(t("loggingConfig.saveFailedToast"));
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit, onError)} className="space-y-6">
				<FormField
					control={form.control}
					name="disable_content_logging"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel data-testid="logging-field-disable-content-logging-label">
									{t("loggingConfig.disableContentLoggingLabel")}
								</FormLabel>
								<FormDescription>{t("loggingConfig.disableContentLoggingDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="logging-field-disable-content-logging"
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
					name="retain_content_in_object_storage"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>{t("loggingConfig.retainContentInObjectStorageLabel")}</FormLabel>
								<FormDescription>{t("loggingConfig.retainContentInObjectStorageDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="logging-field-retain-content"
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
					name="allow_per_request_content_storage_override"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>{t("loggingConfig.allowPerRequestContentStorageOverrideLabel")}</FormLabel>
								<FormDescription>{t("loggingConfig.allowPerRequestContentStorageOverrideDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="logging-field-allow-per-request-override"
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
					name="logging_headers"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("loggingConfig.loggingHeadersLabel")}</FormLabel>
							<FormDescription>{t("loggingConfig.loggingHeadersDescription")}</FormDescription>
							<FormControl>
								<TagInput
									data-testid="logging-field-logging-headers"
									value={Array.isArray(field.value) ? field.value : []}
									onValueChange={field.onChange}
									placeholder={t("loggingConfig.loggingHeadersPlaceholder")}
									disabled={!hasUpdateAccess}
								/>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				<div className="flex justify-end">
					<Button type="submit" disabled={isLoading || !form.formState.isDirty || !hasUpdateAccess}>
						{isLoading ? t("loggingConfig.saving") : t("loggingConfig.saveConfiguration")}
					</Button>
				</div>
			</form>
		</Form>
	);
}

export function LoggingFragment({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	return (
		<div data-testid="logging-fragment" className="space-y-8">
			<h3 className="text-lg font-semibold">{t(pluginFragmentLabels.logging)}</h3>
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("loggingConfig.settingsTitle")}</h4>
				<ConfigForm plugin={plugin} />
			</div>
		</div>
	);
}

export default LoggingFragment;