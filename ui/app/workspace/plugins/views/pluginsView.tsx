import ConfirmDeletePluginDialog from "@/app/workspace/plugins/dialogs/confirmDeletePluginDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CodeEditor } from "@/components/ui/codeEditor";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { setPluginFormDirtyState, useAppDispatch, useAppSelector, useUpdatePluginMutation } from "@/lib/store";
import {
	PluginType,
	PROVIDER_COOLDOWN_PLUGIN,
	RTK_PLUGIN,
	OTEL_PLUGIN,
	LOGGING_PLUGIN,
	SEMANTIC_CACHE_PLUGIN,
	MOCKER_PLUGIN,
	PROMPTS_PLUGIN,
	MODELCATALOGRESOLVER_PLUGIN,
	JSONPARSER_PLUGIN,
} from "@/lib/types/plugins";
import { cn } from "@/lib/utils";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, SaveIcon, Trash2Icon } from "lucide-react";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import ProvidercooldownFragment from "../fragments/providercooldownFragment";
import RtkFragment from "../fragments/rtkFragment";
import OtelView from "../../observability/views/plugins/otelView";
import LoggingFragment from "../fragments/loggingFragment";
import SemanticCacheFragment from "../fragments/semanticCacheFragment";
import MockerFragment from "../fragments/mockerFragment";
import { PromptsFragment, ModelcatalogresolverFragment, JsonparserFragment } from "../fragments/promptsFragment";
import { toast } from "sonner";
import * as z from "zod";
import { useTranslation } from "react-i18next";

interface Props {
	onDelete: () => void;
	onCreate: (pluginName: string) => void;
}

function getFormSchema(t: (key: string) => string) {
	return z.object({
		name: z.string().min(1, t("pluginForm.nameRequiredValidation")),
		enabled: z.boolean(),
		path: z.string().optional(),
		config: z.string().optional(),
		hasConfig: z.boolean(),
	});
}

type PluginFormValues = z.infer<ReturnType<typeof getFormSchema>>;

const getPluginTypeColor = (type: PluginType) => {
	switch (type) {
		case "llm":
			return "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300";
		case "mcp":
			return "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300";
		case "http":
			return "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-300";
		default:
			return "bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-300";
	}
};

export default function PluginsView(props: Props) {
	const { t } = useTranslation("plugins");
	const dispatch = useAppDispatch();
	const hasUpdatePluginAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const hasDeletePluginAccess = useRbac(RbacResource.Plugins, RbacOperation.Delete);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();
	const selectedPlugin = useAppSelector((state) => state.plugin.selectedPlugin);
	const [showConfig, setShowConfig] = useState(false);
	const [showDeleteDialog, setShowDeleteDialog] = useState(false);

	const form = useForm<PluginFormValues>({
		resolver: zodResolver(getFormSchema(t)),
		defaultValues: {
			name: selectedPlugin?.name || "",
			enabled: selectedPlugin?.enabled || false,
			path: selectedPlugin?.path || undefined,
			config: selectedPlugin?.config ? JSON.stringify(selectedPlugin.config, null, 2) : undefined,
			hasConfig: Boolean(selectedPlugin?.config && Object.keys(selectedPlugin.config).length > 0),
		},
	});

	// Update form when selectedPlugin changes
	useEffect(() => {
		if (selectedPlugin) {
			const hasConfig = Boolean(selectedPlugin.config && Object.keys(selectedPlugin.config).length > 0);
			setShowConfig(hasConfig);
			form.reset({
				name: selectedPlugin.name,
				enabled: selectedPlugin.enabled,
				path: selectedPlugin.path,
				config: hasConfig ? JSON.stringify(selectedPlugin.config, null, 2) : undefined,
				hasConfig,
			});
		}
	}, [selectedPlugin]);

	// Track form dirty state
	useEffect(() => {
		const isDirty = form.formState.isDirty;
		dispatch(setPluginFormDirtyState(isDirty));
	}, [form.formState.isDirty, dispatch]);

	const onSubmit = async (values: PluginFormValues) => {
		if (!selectedPlugin) return;

		try {
			let config;
			if (values.hasConfig && values.config) {
				try {
					config = JSON.parse(values.config);
				} catch {
					toast.error(t("pluginForm.invalidJsonToast"));
					return;
				}
			}

			await updatePlugin({
				name: selectedPlugin.name,
				data: {
					enabled: values.enabled,
					path: values.path ?? undefined,
					...(config !== undefined && { config }),
				},
			}).unwrap();
			toast.success(t("pluginForm.updatedToast"));
			form.reset(values);
		} catch {
			toast.error(t("pluginForm.updateErrorToast"));
		}
	};

	const onError = () => {
		toast.error(t("pluginForm.formErrorToast"));
	};

	const handleDeleteClick = () => {
		setShowDeleteDialog(true);
	};

	const handleDeleteCancel = () => {
		setShowDeleteDialog(false);
	};

	const handleDeleteSuccess = () => {
		setShowDeleteDialog(false);
		toast.success(t("pluginForm.deletedToast"));
		props.onDelete();
	};

	if (!selectedPlugin) {
		return (
			<div className="ml-4 flex w-full items-center justify-center">
				<p className="text-muted-foreground">{t("pluginForm.noPluginSelected")}</p>
			</div>
		);
	}

	// Render the dedicated fragment for built-in plugins
	if (selectedPlugin.name === PROVIDER_COOLDOWN_PLUGIN) {
		return (
			<div className="ml-4 w-full">
				<ProvidercooldownFragment plugin={selectedPlugin} />
			</div>
		);
	}
	if (selectedPlugin.name === RTK_PLUGIN) {
		return (
			<div className="ml-4 w-full">
				<RtkFragment plugin={selectedPlugin} />
			</div>
		);
	}
	if (selectedPlugin.name === OTEL_PLUGIN) {
		return (
			<div className="ml-4 w-full">
				<div data-testid="otel-fragment">
					<OtelView />
				</div>
			</div>
		);
	}
	if (selectedPlugin.name === LOGGING_PLUGIN) {
		return (
			<div className="ml-4 w-full">
				<LoggingFragment plugin={selectedPlugin} />
			</div>
		);
	}
	if (selectedPlugin.name === SEMANTIC_CACHE_PLUGIN) {
		return (
			<div className="ml-4 w-full">
				<SemanticCacheFragment plugin={selectedPlugin} />
			</div>
		);
	}
	if (selectedPlugin.name === MOCKER_PLUGIN) {
		return (
			<div className="ml-4 w-full">
				<MockerFragment plugin={selectedPlugin} />
			</div>
		);
	}
	if (selectedPlugin.name === PROMPTS_PLUGIN) {
		return (
			<div className="ml-4 w-full">
				<PromptsFragment />
			</div>
		);
	}
	if (selectedPlugin.name === MODELCATALOGRESOLVER_PLUGIN) {
		return (
			<div className="ml-4 w-full">
				<ModelcatalogresolverFragment />
			</div>
		);
	}
	if (selectedPlugin.name === JSONPARSER_PLUGIN) {
		return (
			<div className="ml-4 w-full">
				<JsonparserFragment />
			</div>
		);
	}

	const isErrorLog = (log: string) => {
		const errorKeywords = ["error", "failed", "exception", "panic", "fatal", "ERR"];
		return errorKeywords.some((keyword) => log.toLowerCase().includes(keyword.toLowerCase()));
	};

	return (
		<div className="ml-4 w-full">
			<Form {...form}>
				<form onSubmit={form.handleSubmit(onSubmit, onError)} className="space-y-6">
					<div className="">
						<h3 className="mb-4 text-lg font-semibold">{t("pluginForm.title")}</h3>
						<div className="space-y-6">
							<FormField
								control={form.control}
								name="name"
								render={({ field }) => (
									<FormItem>
										<FormLabel>{t("pluginForm.nameLabel")}</FormLabel>
										<FormControl>
											<Input placeholder={t("pluginForm.namePlaceholder")} {...field} readOnly disabled className="cursor-not-allowed" />
										</FormControl>
										<FormDescription>{t("pluginForm.nameDescription")}</FormDescription>
										<FormMessage />
									</FormItem>
								)}
							/>

							{selectedPlugin.status?.types && selectedPlugin.status.types.length > 0 && (
								<FormItem>
									<FormLabel>{t("pluginForm.typesLabel")}</FormLabel>
									<FormControl>
										<div className="flex flex-wrap gap-1">
											{selectedPlugin.status.types.map((type) => (
												<Badge
													key={type}
													variant="outline"
													className={cn("h-5 px-2 text-xs font-medium uppercase", getPluginTypeColor(type))}
												>
													{type}
												</Badge>
											))}
										</div>
									</FormControl>
								</FormItem>
							)}

							<FormField
								control={form.control}
								name="enabled"
								render={({ field }) => (
									<FormItem className="flex flex-row items-center justify-between">
										<div className="space-y-0.5">
											<FormLabel>{t("pluginForm.enabledLabel")}</FormLabel>
											<FormDescription>{t("pluginForm.enabledDescription")}</FormDescription>
										</div>
										<FormControl>
											<Switch checked={field.value} onCheckedChange={field.onChange} />
										</FormControl>
									</FormItem>
								)}
							/>

							<FormField
								control={form.control}
								name="path"
								render={({ field }) => (
									<FormItem>
										<FormLabel>{t("pluginForm.pathLabel")}</FormLabel>
										<FormControl>
											<Input placeholder={t("pluginForm.pathPlaceholder")} {...field} value={field.value || ""} />
										</FormControl>
										<FormDescription>{t("pluginForm.pathDescription")}</FormDescription>
										<FormMessage />
									</FormItem>
								)}
							/>

							{!showConfig ? (
								<Button
									type="button"
									variant="outline"
									size="sm"
									onClick={() => {
										setShowConfig(true);
										form.setValue("hasConfig", true);
										if (!form.getValues("config")) {
											form.setValue("config", "{}");
										}
									}}
									className="w-full"
								>
									<PlusIcon className="mr-2 h-4 w-4" />
									{t("pluginForm.addConfiguration")}
								</Button>
							) : (
								<FormField
									control={form.control}
									name="config"
									render={({ field }) => (
										<FormItem>
											<div className="flex items-center justify-between">
												<FormLabel>{t("pluginForm.configLabel")}</FormLabel>
												<Button
													type="button"
													variant="ghost"
													size="sm"
													onClick={() => {
														setShowConfig(false);
														form.setValue("hasConfig", false);
														form.setValue("config", undefined);
													}}
													className="h-auto p-1 text-xs"
												>
													{t("pluginForm.removeConfig")}
												</Button>
											</div>
											<FormControl>
												<div className="rounded-sm border">
													<CodeEditor
														className="z-0 w-full"
														minHeight={200}
														maxHeight={400}
														wrap={true}
														code={field.value || "{}"}
														lang="json"
														onChange={field.onChange}
														options={{
															scrollBeyondLastLine: false,
															collapsibleBlocks: true,
															lineNumbers: "on",
															alwaysConsumeMouseWheel: false,
														}}
													/>
												</div>
											</FormControl>
											<FormDescription>{t("pluginForm.configDescription")}</FormDescription>
											<FormMessage />
										</FormItem>
									)}
								/>
							)}
						</div>

						{selectedPlugin.status?.status !== "active" && (
							<div className="mt-4">
								<div className="space-y-4">
									{selectedPlugin.status?.logs && selectedPlugin.status.logs.length > 0 && (
										<div className="grid gap-2">
											<label className="text-sm font-medium">{t("pluginForm.logsLabel")}</label>
											<div className="rounded-md border px-4 py-2 font-mono text-xs">
												<div className="flex flex-row items-center gap-2">
													{selectedPlugin.status.logs.map((log, index) => (
														<div key={index} className={isErrorLog(log) ? "text-red-400" : "text-green-600"}>
															{log}
														</div>
													))}
												</div>
											</div>
										</div>
									)}
								</div>
							</div>
						)}
					</div>

					<div className="flex flex-wrap justify-end gap-2">
						<Button
							className="border-destructive text-destructive hover:bg-destructive/10 hover:text-destructive"
							type="button"
							variant="outline"
							onClick={handleDeleteClick}
							disabled={!hasDeletePluginAccess}
						>
							<Trash2Icon className="h-4 w-4" />
							{t("pluginForm.deletePlugin")}
						</Button>
						<Button
							type="button"
							variant="outline"
							onClick={() => form.reset()}
							disabled={!form.formState.isDirty || !hasUpdatePluginAccess}
						>
							{t("pluginForm.reset")}
						</Button>
						<Button type="submit" disabled={isLoading || !form.formState.isDirty || !hasUpdatePluginAccess}>
							<SaveIcon className="h-4 w-4" />
							{isLoading ? t("pluginForm.saving") : t("pluginForm.saveChanges")}
						</Button>
					</div>
				</form>
			</Form>

			{selectedPlugin && (
				<ConfirmDeletePluginDialog
					show={showDeleteDialog}
					onCancel={handleDeleteCancel}
					onDelete={handleDeleteSuccess}
					plugin={selectedPlugin}
				/>
			)}
		</div>
	);
}