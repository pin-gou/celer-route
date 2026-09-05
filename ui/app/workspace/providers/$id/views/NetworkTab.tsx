import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { SecretVarInput } from "@/components/ui/secretVarInput";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { HeadersTable } from "@/components/ui/headersTable";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { DefaultNetworkConfig } from "@/lib/constants/config";
import { cn } from "@/lib/utils";
import { getErrorMessage, setProviderFormDirtyState, useAppDispatch } from "@/lib/store";
import { useUpdateProviderMutation } from "@/lib/store/apis/providersApi";
import { ModelProvider, isKnownProvider } from "@/lib/types/config";
import { networkAndProxyFormSchema, type NetworkAndProxyFormSchema, type SecretVar } from "@/lib/types/schemas";
import { toOptionalSecretVarPayload, toSecretVarFormValue } from "@/lib/utils/secretVarForm";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { zodResolver } from "@hookform/resolvers/zod";
import { Info } from "lucide-react";
import { useEffect } from "react";
import { useForm, type FieldValues, type Path, type Resolver, type UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { buildProviderUpdatePayload } from "@/app/workspace/providers/views/utils";

const secondsToHumanReadable = (seconds: number | undefined, t: (key: string) => string) => {
	if (!seconds || seconds < 0 || Number.isNaN(seconds)) {
		return t("fragments.network.timeUnits.zero");
	}
	const s = Math.floor(seconds);
	if (s < 60) return `${s} ${s === 1 ? t("fragments.network.timeUnits.second") : t("fragments.network.timeUnits.seconds")}`;
	if (s < 3600) {
		const m = Math.floor(s / 60);
		const r = s % 60;
		const parts = [`${m} ${m === 1 ? t("fragments.network.timeUnits.minute") : t("fragments.network.timeUnits.minutes")}`];
		if (r > 0) parts.push(`${r} ${r === 1 ? t("fragments.network.timeUnits.second") : t("fragments.network.timeUnits.seconds")}`);
		return parts.join(" ");
	}
	if (s < 86400) {
		const h = Math.floor(s / 3600);
		const m = Math.floor((s % 3600) / 60);
		const r = s % 60;
		const parts = [`${h} ${h === 1 ? t("fragments.network.timeUnits.hour") : t("fragments.network.timeUnits.hours")}`];
		if (m > 0) parts.push(`${m} ${m === 1 ? t("fragments.network.timeUnits.minute") : t("fragments.network.timeUnits.minutes")}`);
		if (r > 0) parts.push(`${r} ${r === 1 ? t("fragments.network.timeUnits.second") : t("fragments.network.timeUnits.seconds")}`);
		return parts.join(" ");
	}
	const d = Math.floor(s / 86400);
	const h = Math.floor((s % 86400) / 3600);
	const m = Math.floor((s % 3600) / 60);
	const r = s % 60;
	const parts = [`${d} ${d === 1 ? t("fragments.network.timeUnits.day") : t("fragments.network.timeUnits.days")}`];
	if (h > 0) parts.push(`${h} ${h === 1 ? t("fragments.network.timeUnits.hour") : t("fragments.network.timeUnits.hours")}`);
	if (m > 0) parts.push(`${m} ${m === 1 ? t("fragments.network.timeUnits.minute") : t("fragments.network.timeUnits.minutes")}`);
	if (r > 0) parts.push(`${r} ${r === 1 ? t("fragments.network.timeUnits.second") : t("fragments.network.timeUnits.seconds")}`);
	return parts.join(" ");
};

const numericFieldOnChange =
	<T extends FieldValues>(field: { onChange: (v: unknown) => void }, form: UseFormReturn<T>, name: Path<T>) =>
	(e: React.ChangeEvent<HTMLInputElement>) => {
		const value = e.target.value;
		if (value === "") {
			field.onChange(undefined);
			return;
		}
		const parsed = Number(value);
		if (!Number.isNaN(parsed)) {
			field.onChange(parsed);
		}
		void form.trigger(name);
	};

interface NetworkTabProps {
	provider: ModelProvider;
}

export function NetworkTab({ provider }: NetworkTabProps) {
	const { t } = useTranslation("providers");
	const dispatch = useAppDispatch();
	const hasUpdateProviderAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const [updateProvider, { isLoading: isUpdatingProvider }] = useUpdateProviderMutation();
	const isCustomProvider = !isKnownProvider(provider.name as string);

	const buildDefaults = () => {
		const nw = provider.network_config;
		const px = provider.proxy_config;
		return {
			network_config: {
				base_url: nw?.base_url || undefined,
				extra_headers: nw?.extra_headers,
				default_request_timeout_in_seconds:
					nw?.default_request_timeout_in_seconds ?? DefaultNetworkConfig.default_request_timeout_in_seconds,
				max_retries: nw?.max_retries ?? DefaultNetworkConfig.max_retries,
				retry_backoff_initial: nw?.retry_backoff_initial ?? DefaultNetworkConfig.retry_backoff_initial,
				retry_backoff_max: nw?.retry_backoff_max ?? DefaultNetworkConfig.retry_backoff_max,
				insecure_skip_verify: nw?.insecure_skip_verify ?? DefaultNetworkConfig.insecure_skip_verify,
				ca_cert_pem: toSecretVarFormValue(nw?.ca_cert_pem as SecretVar | string | undefined),
				stream_idle_timeout_in_seconds: nw?.stream_idle_timeout_in_seconds ?? DefaultNetworkConfig.stream_idle_timeout_in_seconds,
				keep_alive_timeout_in_seconds: nw?.keep_alive_timeout_in_seconds ?? DefaultNetworkConfig.keep_alive_timeout_in_seconds,
				max_conns_per_host: nw?.max_conns_per_host ?? DefaultNetworkConfig.max_conns_per_host,
				enforce_http2: nw?.enforce_http2 ?? DefaultNetworkConfig.enforce_http2,
				allow_private_network: nw?.allow_private_network ?? DefaultNetworkConfig.allow_private_network,
			},
			proxy_config: {
				type: (px?.type ?? "none") as NonNullable<NetworkAndProxyFormSchema["proxy_config"]>["type"],
				url: toSecretVarFormValue(px?.url as SecretVar | string | undefined),
				username: toSecretVarFormValue(px?.username as SecretVar | string | undefined),
				password: toSecretVarFormValue(px?.password as SecretVar | string | undefined),
				ca_cert_pem: toSecretVarFormValue(px?.ca_cert_pem as SecretVar | string | undefined),
			},
		};
	};

	const form = useForm<NetworkAndProxyFormSchema, any, NetworkAndProxyFormSchema>({
		resolver: zodResolver(networkAndProxyFormSchema) as Resolver<NetworkAndProxyFormSchema, any, NetworkAndProxyFormSchema>,
		mode: "onChange",
		reValidateMode: "onChange",
		defaultValues: buildDefaults(),
	});

	useEffect(() => {
		dispatch(setProviderFormDirtyState(form.formState.isDirty));
	}, [form.formState.isDirty, dispatch]);

	useEffect(() => {
		form.reset(buildDefaults());
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [provider.name, provider.network_config, provider.proxy_config]);

	const watchedProxyType = form.watch("proxy_config.type");
	const proxyRequiresUrl = watchedProxyType === "http" || watchedProxyType === "socks5";

	const baseURLRequired = isCustomProvider;
	const baseFormat = (provider.custom_provider_config?.base_provider_type as string) || "default";
	const hideBaseURL = provider.name === "vllm" || provider.name === "ollama" || provider.name === "sgl";

	const onSubmit = (data: NetworkAndProxyFormSchema) => {
		if (baseURLRequired && (data.network_config?.base_url ?? "").trim() === "") {
			if ((provider.network_config?.base_url ?? "").trim() !== "") {
				toast.error(t("fragments.network.toast.cannotRemoveNetworkConfig"));
			} else {
				toast.error(t("fragments.network.toast.baseUrlRequired"));
			}
			return;
		}
		const updatedProvider = buildProviderUpdatePayload(provider, {
			network_config: {
				...provider.network_config,
				base_url: data.network_config?.base_url || undefined,
				extra_headers: data.network_config?.extra_headers || undefined,
				default_request_timeout_in_seconds:
					data.network_config?.default_request_timeout_in_seconds ?? DefaultNetworkConfig.default_request_timeout_in_seconds,
				max_retries: data.network_config?.max_retries ?? 0,
				retry_backoff_initial: data.network_config?.retry_backoff_initial ?? 500,
				retry_backoff_max: data.network_config?.retry_backoff_max ?? 10000,
				insecure_skip_verify: data.network_config?.insecure_skip_verify ?? false,
				ca_cert_pem: toOptionalSecretVarPayload(data.network_config?.ca_cert_pem),
				stream_idle_timeout_in_seconds:
					data.network_config?.stream_idle_timeout_in_seconds ?? DefaultNetworkConfig.stream_idle_timeout_in_seconds,
				keep_alive_timeout_in_seconds:
					data.network_config?.keep_alive_timeout_in_seconds ?? DefaultNetworkConfig.keep_alive_timeout_in_seconds,
				max_conns_per_host: data.network_config?.max_conns_per_host ?? DefaultNetworkConfig.max_conns_per_host,
				enforce_http2: data.network_config?.enforce_http2 ?? DefaultNetworkConfig.enforce_http2,
				allow_private_network: data.network_config?.allow_private_network ?? DefaultNetworkConfig.allow_private_network,
			},
			proxy_config: {
				type: data.proxy_config?.type ?? "none",
				url: toOptionalSecretVarPayload(data.proxy_config?.url),
				username: toOptionalSecretVarPayload(data.proxy_config?.username),
				password: toOptionalSecretVarPayload(data.proxy_config?.password),
				ca_cert_pem: toOptionalSecretVarPayload(data.proxy_config?.ca_cert_pem),
			},
		});
		updateProvider(updatedProvider)
			.unwrap()
			.then(() => {
				toast.success(t("fragments.network.toast.providerUpdated"));
				form.reset(data);
			})
			.catch((err) => {
				toast.error(t("fragments.network.toast.failedToUpdate"), {
					description: getErrorMessage(err),
				});
			});
	};

	return (
		<div data-testid="providers2-network-tab" className="rounded-lg border p-4">
			<Form {...form}>
				<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
					<Accordion type="multiple" defaultValue={["basic"]}>
						<AccordionItem value="basic">
							<AccordionTrigger data-testid="providers2-network-basic-trigger">
								<span className="text-sm font-medium">{t("providers2.networkTab.basic")}</span>
							</AccordionTrigger>
							<AccordionContent className="space-y-4 pt-4">
								{!hideBaseURL && (
									<FormField
										control={form.control}
										name="network_config.base_url"
										render={({ field }) => (
											<FormItem>
												<FormLabel>
													{baseURLRequired ? t("fragments.network.baseUrlRequired") : t("fragments.network.baseUrlOptional")}
												</FormLabel>
												{baseURLRequired && <FormDescription>{t(`fragments.network.baseUrlDesc.${baseFormat}`)}</FormDescription>}
												<FormControl>
													<Input
														placeholder={
															isCustomProvider ? t("fragments.network.baseUrlPlaceholderCustom") : t("fragments.network.baseUrlPlaceholder")
														}
														{...field}
														value={field.value || ""}
														disabled={!hasUpdateProviderAccess}
													/>
												</FormControl>
												<FormMessage />
											</FormItem>
										)}
									/>
								)}

								<div className="flex w-full flex-row items-start gap-4">
									<FormField
										control={form.control}
										name="network_config.default_request_timeout_in_seconds"
										render={({ field }) => (
											<FormItem className="flex-1">
												<FormLabel>{t("fragments.network.timeout")}</FormLabel>
												<FormControl>
													<Input
														placeholder="30"
														{...field}
														value={field.value === undefined || Number.isNaN(field.value) ? "" : field.value}
														disabled={!hasUpdateProviderAccess}
														onChange={numericFieldOnChange(field, form, "network_config")}
													/>
												</FormControl>
												<FormDescription>{secondsToHumanReadable(field.value, t)}</FormDescription>
												<FormMessage />
											</FormItem>
										)}
									/>
									<FormField
										control={form.control}
										name="network_config.stream_idle_timeout_in_seconds"
										render={({ field }) => (
											<FormItem className="flex-1">
												<FormLabel>{t("fragments.network.streamIdleTimeout")}</FormLabel>
												<FormControl>
													<Input
														placeholder="60"
														data-testid="network-config-stream-idle-timeout-input"
														{...field}
														value={field.value === undefined || Number.isNaN(field.value) ? "" : field.value}
														disabled={!hasUpdateProviderAccess}
														onChange={numericFieldOnChange(field, form, "network_config")}
													/>
												</FormControl>
												<FormDescription>
													{field.value ? secondsToHumanReadable(field.value, t) : ""} {t("fragments.network.streamIdleTimeoutDescription")}
												</FormDescription>
												<FormMessage />
											</FormItem>
										)}
									/>
									<FormField
										control={form.control}
										name="network_config.max_retries"
										render={({ field }) => (
											<FormItem className="flex-1">
												<FormLabel>{t("fragments.network.maxRetries")}</FormLabel>
												<FormControl>
													<Input
														placeholder="0"
														{...field}
														value={field.value === undefined || Number.isNaN(field.value) ? "" : field.value}
														disabled={!hasUpdateProviderAccess}
														onChange={numericFieldOnChange(field, form, "network_config")}
													/>
												</FormControl>
												<FormMessage />
											</FormItem>
										)}
									/>
								</div>

								<FormField
									control={form.control}
									name="proxy_config.type"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("fragments.proxy.proxyType")}</FormLabel>
											<Select
												onValueChange={field.onChange}
												value={field.value === "none" ? "" : (field.value as string)}
												disabled={!hasUpdateProviderAccess}
											>
												<FormControl>
													<SelectTrigger className="w-48" data-testid="env-var-proxy-type-trigger">
														<SelectValue placeholder={t("fragments.proxy.selectType")} />
													</SelectTrigger>
												</FormControl>
												<SelectContent>
													<SelectItem value="http">{t("fragments.proxy.http")}</SelectItem>
													<SelectItem value="socks5">{t("fragments.proxy.socks5")}</SelectItem>
													<SelectItem value="environment">{t("fragments.proxy.environment")}</SelectItem>
												</SelectContent>
											</Select>
											<FormMessage />
										</FormItem>
									)}
								/>

								<div className={cn("block transition-all duration-200", !proxyRequiresUrl && "hidden")}>
									<Alert>
										<Info className="h-4 w-4" />
										<AlertDescription>{t("fragments.proxy.alertDescription")}</AlertDescription>
									</Alert>
									<FormField
										control={form.control}
										name="proxy_config.url"
										render={({ field }) => (
											<FormItem>
												<FormLabel>{t("fragments.proxy.proxyUrl")}</FormLabel>
												<FormControl>
													<SecretVarInput
														placeholder={t("fragments.proxy.proxyUrlPlaceholder")}
														{...field}
														value={field.value}
														disabled={!hasUpdateProviderAccess}
														data-testid="env-var-proxy-url"
													/>
												</FormControl>
												<FormMessage />
											</FormItem>
										)}
									/>
								</div>
							</AccordionContent>
						</AccordionItem>

						<AccordionItem value="advanced">
							<AccordionTrigger data-testid="providers2-network-advanced-trigger">
								<span className="text-sm font-medium">{t("providers2.networkTab.advanced")}</span>
							</AccordionTrigger>
							<AccordionContent className="space-y-4 pt-4">
								<div className="flex w-full flex-row items-start gap-4">
									<FormField
										control={form.control}
										name="network_config.retry_backoff_initial"
										render={({ field }) => (
											<FormItem className="flex-1">
												<FormLabel>{t("fragments.network.initialBackoff")}</FormLabel>
												<FormControl>
													<Input
														placeholder={t("fragments.network.initialBackoffPlaceholder")}
														{...field}
														value={field.value === undefined || Number.isNaN(field.value) ? "" : field.value}
														disabled={!hasUpdateProviderAccess}
														onChange={numericFieldOnChange(field, form, "network_config")}
													/>
												</FormControl>
												<FormMessage />
											</FormItem>
										)}
									/>
									<FormField
										control={form.control}
										name="network_config.retry_backoff_max"
										render={({ field }) => (
											<FormItem className="flex-1">
												<FormLabel>{t("fragments.network.maxBackoff")}</FormLabel>
												<FormControl>
													<Input
														placeholder={t("fragments.network.maxBackoffPlaceholder")}
														{...field}
														value={field.value === undefined || Number.isNaN(field.value) ? "" : field.value}
														disabled={!hasUpdateProviderAccess}
														onChange={numericFieldOnChange(field, form, "network_config")}
													/>
												</FormControl>
												<FormMessage />
											</FormItem>
										)}
									/>
								</div>

								<div className="flex w-full flex-row items-start gap-4">
									<FormField
										control={form.control}
										name="network_config.max_conns_per_host"
										render={({ field }) => (
											<FormItem className="flex-1">
												<FormLabel>{t("fragments.network.maxConnectionsPerHost")}</FormLabel>
												<FormControl>
													<Input
														data-testid="network-config-max-conns-per-host-input"
														placeholder="5000"
														{...field}
														value={field.value === undefined || Number.isNaN(field.value) ? "" : field.value}
														disabled={!hasUpdateProviderAccess}
														onChange={numericFieldOnChange(field, form, "network_config")}
													/>
												</FormControl>
												<FormDescription>{t("fragments.network.maxConnectionsPerHostDescription")}</FormDescription>
												<FormMessage />
											</FormItem>
										)}
									/>
									<FormField
										control={form.control}
										name="network_config.keep_alive_timeout_in_seconds"
										render={({ field }) => (
											<FormItem className="flex-1">
												<FormLabel>{t("fragments.network.keepAliveTimeout")}</FormLabel>
												<FormControl>
													<Input
														data-testid="network-config-keep-alive-timeout-input"
														placeholder="30"
														{...field}
														value={field.value === undefined || Number.isNaN(field.value) ? "" : field.value}
														disabled={!hasUpdateProviderAccess}
														onChange={numericFieldOnChange(field, form, "network_config")}
													/>
												</FormControl>
												<FormDescription>
													{field.value ? secondsToHumanReadable(field.value, t) : ""} {t("fragments.network.keepAliveTimeoutDescription")}
												</FormDescription>
												<FormMessage />
											</FormItem>
										)}
									/>
								</div>

								<FormField
									control={form.control}
									name="network_config.enforce_http2"
									render={({ field }) => (
										<FormItem className="flex flex-row items-center justify-between rounded border p-4">
											<div className="space-y-0.5">
												<FormLabel>{t("fragments.network.enforceHttp2")}</FormLabel>
												<FormDescription>{t("fragments.network.enforceHttp2Description")}</FormDescription>
											</div>
											<FormControl>
												<Switch
													checked={field.value ?? false}
													onCheckedChange={field.onChange}
													disabled={!hasUpdateProviderAccess}
													data-testid="network-config-enforce-http2"
												/>
											</FormControl>
										</FormItem>
									)}
								/>
								<FormField
									control={form.control}
									name="network_config.allow_private_network"
									render={({ field }) => (
										<FormItem className="flex flex-row items-center justify-between rounded border p-4">
											<div className="space-y-0.5">
												<FormLabel>{t("fragments.network.allowPrivateNetwork")}</FormLabel>
												<FormDescription>{t("fragments.network.allowPrivateNetworkDescription")}</FormDescription>
											</div>
											<FormControl>
												<Switch
													checked={field.value ?? false}
													onCheckedChange={field.onChange}
													disabled={!hasUpdateProviderAccess}
													data-testid="network-config-allow-private-network"
												/>
											</FormControl>
										</FormItem>
									)}
								/>

								<FormField
									control={form.control}
									name="network_config.extra_headers"
									render={({ field }) => (
										<FormItem>
											<FormControl>
												<HeadersTable
													value={field.value || {}}
													onChange={field.onChange}
													keyPlaceholder={t("fragments.network.headerNamePlaceholder")}
													valuePlaceholder={t("fragments.network.headerValuePlaceholder")}
													label={t("fragments.network.extraHeaders")}
													disabled={!hasUpdateProviderAccess}
												/>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>

								<FormField
									control={form.control}
									name="network_config.insecure_skip_verify"
									render={({ field }) => (
										<FormItem className="flex flex-row items-center justify-between rounded border p-4">
											<div className="space-y-0.5">
												<FormLabel>{t("fragments.network.skipTlsVerification")}</FormLabel>
												<FormDescription>{t("fragments.network.skipTlsVerificationDescription")}</FormDescription>
											</div>
											<FormControl>
												<Switch
													checked={field.value ?? false}
													onCheckedChange={field.onChange}
													disabled={!hasUpdateProviderAccess}
													data-testid="network-config-insecure-skip-verify"
												/>
											</FormControl>
										</FormItem>
									)}
								/>
								<FormField
									control={form.control}
									name="network_config.ca_cert_pem"
									render={({ field }) => (
										<FormItem>
											<FormLabel>{t("fragments.network.caCertificate")}</FormLabel>
											<FormControl>
												<SecretVarInput
													variant="textarea"
													placeholder={t("fragments.network.caCertificatePlaceholder")}
													className="font-mono text-xs"
													rows={6}
													hideValueWhenEnv
													redactNonEnvValue
													{...field}
													value={field.value}
													disabled={!hasUpdateProviderAccess}
													data-testid="network-config-ca-cert-pem"
												/>
											</FormControl>
											<FormDescription>{t("fragments.network.caCertificateDescription")}</FormDescription>
											<FormMessage />
										</FormItem>
									)}
								/>

								<div className={cn("block transition-all duration-200", !proxyRequiresUrl && "hidden")}>
									<div className="grid grid-cols-2 gap-4">
										<FormField
											control={form.control}
											name="proxy_config.username"
											render={({ field }) => (
												<FormItem>
													<FormLabel>{t("fragments.proxy.username")}</FormLabel>
													<FormControl>
														<SecretVarInput
															placeholder={t("fragments.proxy.usernamePlaceholder")}
															{...field}
															value={field.value}
															disabled={!hasUpdateProviderAccess}
															data-testid="env-var-proxy-username"
														/>
													</FormControl>
													<FormMessage />
												</FormItem>
											)}
										/>
										<FormField
											control={form.control}
											name="proxy_config.password"
											render={({ field }) => (
												<FormItem>
													<FormLabel>{t("fragments.proxy.password")}</FormLabel>
													<FormControl>
														<SecretVarInput
															type="password"
															placeholder={t("fragments.proxy.passwordPlaceholder")}
															hideValueWhenEnv
															redactNonEnvValue
															{...field}
															value={field.value}
															disabled={!hasUpdateProviderAccess}
															data-testid="env-var-proxy-password"
														/>
													</FormControl>
													<FormMessage />
												</FormItem>
											)}
										/>
									</div>
									<FormField
										control={form.control}
										name="proxy_config.ca_cert_pem"
										render={({ field }) => (
											<FormItem>
												<FormLabel>{t("fragments.proxy.caCertPem")}</FormLabel>
												<FormControl>
													<SecretVarInput
														variant="textarea"
														placeholder={t("fragments.proxy.caCertPemPlaceholder")}
														className="font-mono text-xs"
														rows={6}
														hideValueWhenEnv
														redactNonEnvValue
														{...field}
														value={field.value}
														disabled={!hasUpdateProviderAccess}
														data-testid="env-var-proxy-ca-cert-pem"
													/>
												</FormControl>
												<FormDescription>{t("fragments.proxy.caCertDescription")}</FormDescription>
												<FormMessage />
											</FormItem>
										)}
									/>
								</div>
							</AccordionContent>
						</AccordionItem>
					</Accordion>

					<div className="flex items-center justify-end gap-2 pt-2">
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<Button type="submit" disabled={!form.formState.isDirty || !hasUpdateProviderAccess} isLoading={isUpdatingProvider}>
										{t("fragments.network.saveConfiguration")}
									</Button>
								</TooltipTrigger>
								{(!form.formState.isDirty || !form.formState.isValid) && (
									<TooltipContent>
										<p>
											{!form.formState.isDirty && !form.formState.isValid
												? t("fragments.network.tooltip.noChangesAndErrors")
												: !form.formState.isDirty
													? t("fragments.network.tooltip.noChanges")
													: t("fragments.network.tooltip.fixErrors")}
										</p>
									</TooltipContent>
								)}
							</Tooltip>
						</TooltipProvider>
					</div>
				</form>
			</Form>
		</div>
	);
}