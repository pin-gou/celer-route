import { Button } from "@/components/ui/button";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { ProviderLabels } from "@/lib/constants/logs";
import { SEMANTIC_CACHE_PLUGIN, semanticCacheConfigSchema, pluginFragmentLabels, type Plugin } from "@/lib/types/plugins";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";

type SemanticCacheFormValues = z.input<typeof semanticCacheConfigSchema>;

// Static list of embedding providers supported by the semantic cache plugin
// (mirrors the config.schema.json allOf provider enum).
const EMBEDDING_PROVIDERS = Object.keys(ProviderLabels);

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const pluginConfig = (plugin.config || {}) as Record<string, any>;

	const form = useForm<SemanticCacheFormValues>({
		resolver: zodResolver(semanticCacheConfigSchema),
		defaultValues: {
			provider: pluginConfig.provider ?? "",
			embedding_model: pluginConfig.embedding_model ?? "",
			dimension: pluginConfig.dimension ?? 1,
			ttl: pluginConfig.ttl ?? "5m",
			threshold: pluginConfig.threshold ?? 0.8,
			vector_store_namespace: pluginConfig.vector_store_namespace ?? "BifrostSemanticCachePlugin",
			default_cache_key: pluginConfig.default_cache_key ?? "",
			conversation_history_threshold: pluginConfig.conversation_history_threshold ?? 3,
			cache_by_model: pluginConfig.cache_by_model ?? true,
			cache_by_provider: pluginConfig.cache_by_provider ?? true,
			exclude_system_prompt: pluginConfig.exclude_system_prompt ?? false,
		},
	});

	const provider = form.watch("provider");
	const hasProvider = !!provider && provider.length > 0;
	const threshold = form.watch("threshold") ?? 0.8;

	const providerLabel = (name: string) => ProviderLabels[name as keyof typeof ProviderLabels] ?? name;

	const onSubmit = async (values: SemanticCacheFormValues) => {
		if (!hasUpdateAccess) return;
		try {
			await updatePlugin({
				name: SEMANTIC_CACHE_PLUGIN,
				data: {
					enabled: plugin.enabled,
					config: values,
				},
			}).unwrap();
			toast.success(t("semanticCacheConfig.savedToast"));
			form.reset(values);
		} catch {
			toast.error(t("semanticCacheConfig.saveFailedToast"));
		}
	};

	const onError = () => {
		toast.error(t("semanticCacheConfig.saveFailedToast"));
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit, onError)} className="space-y-6">
				{/* Provider */}
				<FormField
					control={form.control}
					name="provider"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("semanticCacheConfig.providerLabel")}</FormLabel>
							<FormDescription>{t("semanticCacheConfig.providerDescription")}</FormDescription>
							<FormControl>
								<Select
									value={field.value ?? ""}
									onValueChange={(val) => {
										field.onChange(val);
										if (val && val.length > 0) {
											form.setValue("dimension", 2, { shouldValidate: true });
										} else {
											form.setValue("dimension", 1, { shouldValidate: true });
											form.setValue("embedding_model", "", { shouldValidate: false });
										}
									}}
								>
									<SelectTrigger data-testid="semantic-cache-field-provider">
										<SelectValue placeholder={t("semanticCacheConfig.providerPlaceholder")} />
									</SelectTrigger>
									<SelectContent>
										{EMBEDDING_PROVIDERS.map((name) => (
											<SelectItem key={name} value={name}>
												{providerLabel(name)}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				{/* embedding_model */}
				<FormField
					control={form.control}
					name="embedding_model"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("semanticCacheConfig.embeddingModelLabel")}</FormLabel>
							<FormDescription>{t("semanticCacheConfig.embeddingModelDescription")}</FormDescription>
							<FormControl>
								<Input
									data-testid="semantic-cache-field-embedding-model"
									placeholder={t("semanticCacheConfig.embeddingModelPlaceholder")}
									disabled={!hasProvider}
									{...field}
									value={field.value ?? ""}
								/>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				{/* dimension */}
				<FormField
					control={form.control}
					name="dimension"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("semanticCacheConfig.dimensionLabel")}</FormLabel>
							<FormDescription>{t("semanticCacheConfig.dimensionDescription")}</FormDescription>
							<FormControl>
								<Input
									data-testid="semantic-cache-field-dimension"
									type="number"
									min={hasProvider ? 2 : 1}
									disabled={!hasProvider}
									{...field}
									value={field.value ?? 1}
									onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
								/>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				{/* ttl */}
				<FormField
					control={form.control}
					name="ttl"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("semanticCacheConfig.ttlLabel")}</FormLabel>
							<FormDescription>{t("semanticCacheConfig.ttlDescription")}</FormDescription>
							<FormControl>
								<Input
									data-testid="semantic-cache-field-ttl"
									placeholder={t("semanticCacheConfig.ttlPlaceholder")}
									{...field}
									value={field.value ?? ""}
								/>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				{/* threshold */}
				<FormField
					control={form.control}
					name="threshold"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("semanticCacheConfig.thresholdLabel")}</FormLabel>
							<FormDescription>{t("semanticCacheConfig.thresholdDescription")}</FormDescription>
							<FormControl>
								<div className="space-y-2">
									<Slider
										data-testid="semantic-cache-field-threshold"
										value={[threshold]}
										min={0}
										max={1}
										step={0.01}
										onValueChange={(v) => field.onChange(v[0])}
									/>
									<span className="text-muted-foreground text-xs">{threshold.toFixed(2)}</span>
								</div>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				{/* vector_store_namespace */}
				<FormField
					control={form.control}
					name="vector_store_namespace"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("semanticCacheConfig.vectorStoreNamespaceLabel")}</FormLabel>
							<FormDescription>{t("semanticCacheConfig.vectorStoreNamespaceDescription")}</FormDescription>
							<FormControl>
								<Input data-testid="semantic-cache-field-vector-store-namespace" {...field} value={field.value ?? ""} />
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				{/* default_cache_key */}
				<FormField
					control={form.control}
					name="default_cache_key"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("semanticCacheConfig.defaultCacheKeyLabel")}</FormLabel>
							<FormDescription>{t("semanticCacheConfig.defaultCacheKeyDescription")}</FormDescription>
							<FormControl>
								<Input data-testid="semantic-cache-field-default-cache-key" {...field} value={field.value ?? ""} />
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				{/* conversation_history_threshold */}
				<FormField
					control={form.control}
					name="conversation_history_threshold"
					render={({ field }) => (
						<FormItem>
							<FormLabel>{t("semanticCacheConfig.conversationHistoryThresholdLabel")}</FormLabel>
							<FormDescription>{t("semanticCacheConfig.conversationHistoryThresholdDescription")}</FormDescription>
							<FormControl>
								<Input
									data-testid="semantic-cache-field-conversation-history-threshold"
									type="number"
									min={0}
									{...field}
									value={field.value ?? 3}
									onChange={(e) => field.onChange(e.target.valueAsNumber || e.target.value)}
								/>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>

				{/* cache_by_model */}
				<FormField
					control={form.control}
					name="cache_by_model"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>{t("semanticCacheConfig.cacheByModelLabel")}</FormLabel>
								<FormDescription>{t("semanticCacheConfig.cacheByModelDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="semantic-cache-field-cache-by-model"
									checked={Boolean(field.value)}
									onCheckedChange={field.onChange}
									disabled={!hasUpdateAccess}
								/>
							</FormControl>
						</FormItem>
					)}
				/>

				{/* cache_by_provider */}
				<FormField
					control={form.control}
					name="cache_by_provider"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>{t("semanticCacheConfig.cacheByProviderLabel")}</FormLabel>
								<FormDescription>{t("semanticCacheConfig.cacheByProviderDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="semantic-cache-field-cache-by-provider"
									checked={Boolean(field.value)}
									onCheckedChange={field.onChange}
									disabled={!hasUpdateAccess}
								/>
							</FormControl>
						</FormItem>
					)}
				/>

				{/* exclude_system_prompt */}
				<FormField
					control={form.control}
					name="exclude_system_prompt"
					render={({ field }) => (
						<FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
							<div className="space-y-0.5">
								<FormLabel>{t("semanticCacheConfig.excludeSystemPromptLabel")}</FormLabel>
								<FormDescription>{t("semanticCacheConfig.excludeSystemPromptDescription")}</FormDescription>
							</div>
							<FormControl>
								<Switch
									data-testid="semantic-cache-field-exclude-system-prompt"
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
						{isLoading ? t("semanticCacheConfig.saving") : t("semanticCacheConfig.saveConfiguration")}
					</Button>
				</div>
			</form>
		</Form>
	);
}

export function SemanticCacheFragment({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	return (
		<div data-testid="semantic-cache-fragment" className="space-y-8">
			<h3 className="text-lg font-semibold">{t(pluginFragmentLabels.semantic_cache)}</h3>
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("semanticCacheConfig.settingsTitle")}</h4>
				<ConfigForm plugin={plugin} />
			</div>
		</div>
	);
}

export default SemanticCacheFragment;