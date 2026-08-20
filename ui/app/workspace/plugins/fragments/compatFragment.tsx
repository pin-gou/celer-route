import { Button } from "@/components/ui/button";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel } from "@/components/ui/form";
import { Switch } from "@/components/ui/switch";
import { useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { COMPAT_PLUGIN, compatConfigSchema, pluginFragmentLabels, type Plugin } from "@/lib/types/plugins";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";

type CompatFormValues = z.input<typeof compatConfigSchema>;

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const pluginConfig = (plugin.config || {}) as Record<string, any>;

	const form = useForm<CompatFormValues>({
		resolver: zodResolver(compatConfigSchema),
		defaultValues: {
			convert_text_to_chat: pluginConfig.convert_text_to_chat ?? true,
			convert_chat_to_responses: pluginConfig.convert_chat_to_responses ?? true,
			should_drop_params: pluginConfig.should_drop_params ?? true,
			should_convert_params: pluginConfig.should_convert_params ?? false,
		},
	});

	const onSubmit = async (values: CompatFormValues) => {
		if (!hasUpdateAccess) return;
		try {
			await updatePlugin({
				name: COMPAT_PLUGIN,
				data: {
					enabled: plugin.enabled,
					config: values,
				},
			}).unwrap();
			toast.success(t("compatConfig.savedToast"));
			form.reset(values);
		} catch {
			toast.error(t("compatConfig.saveFailedToast"));
		}
	};

	const onError = () => {
		toast.error(t("compatConfig.saveFailedToast"));
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit, onError)} className="space-y-6">
				<FormField
					control={form.control}
					name="convert_text_to_chat"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel data-testid="compat-field-convert-text-to-chat-label">{t("compatConfig.convertTextToChatLabel")}</FormLabel>
								<FormDescription>{t("compatConfig.convertTextToChatDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="compat-field-convert-text-to-chat"
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
					name="convert_chat_to_responses"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>{t("compatConfig.convertChatToResponsesLabel")}</FormLabel>
								<FormDescription>{t("compatConfig.convertChatToResponsesDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="compat-field-convert-chat-to-responses"
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
					name="should_drop_params"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>{t("compatConfig.shouldDropParamsLabel")}</FormLabel>
								<FormDescription>{t("compatConfig.shouldDropParamsDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="compat-field-should-drop-params"
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
					name="should_convert_params"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>{t("compatConfig.shouldConvertParamsLabel")}</FormLabel>
								<FormDescription>{t("compatConfig.shouldConvertParamsDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="compat-field-should-convert-params"
									checked={Boolean(field.value)}
									onCheckedChange={field.onChange}
									disabled={!hasUpdateAccess}
								/>
							</FormControl>
						</FormItem>
					)}
				/>

				<div className="flex justify-end">
					<Button type="submit" disabled={isLoading || !form.formState.isDirty || !hasUpdateAccess}>
						{isLoading ? t("compatConfig.saving") : t("compatConfig.saveConfiguration")}
					</Button>
				</div>
			</form>
		</Form>
	);
}

export function CompatFragment({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	return (
		<div data-testid="compat-fragment" className="space-y-8">
			<h3 className="text-lg font-semibold">{t(pluginFragmentLabels.compat)}</h3>
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("compatConfig.settingsTitle")}</h4>
				<ConfigForm plugin={plugin} />
			</div>
		</div>
	);
}

export default CompatFragment;