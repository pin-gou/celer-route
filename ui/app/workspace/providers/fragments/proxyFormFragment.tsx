import { useTranslation } from "react-i18next";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { SecretVarInput } from "@/components/ui/secretVarInput";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { getErrorMessage, setProviderFormDirtyState, useAppDispatch } from "@/lib/store";
import { useUpdateProviderMutation } from "@/lib/store/apis/providersApi";
import { ModelProvider } from "@/lib/types/config";
import { proxyOnlyFormSchema, type SecretVar, type ProxyOnlyFormSchema } from "@/lib/types/schemas";
import { cn } from "@/lib/utils";
import { toSecretVarFormValue, toOptionalSecretVarPayload } from "@/lib/utils/secretVarForm";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { zodResolver } from "@hookform/resolvers/zod";
import { Info } from "lucide-react";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { buildProviderUpdatePayload } from "../views/utils";

interface ProxyFormFragmentProps {
	provider: ModelProvider;
	onCancel?: () => void;
}

export function ProxyFormFragment({ provider, onCancel }: ProxyFormFragmentProps) {
	const { t } = useTranslation("providers");
	const dispatch = useAppDispatch();
	const hasUpdateProviderAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const [updateProvider, { isLoading: isUpdatingProvider }] = useUpdateProviderMutation();
	const form = useForm<ProxyOnlyFormSchema>({
		resolver: zodResolver(proxyOnlyFormSchema),
		mode: "onChange",
		reValidateMode: "onChange",
		defaultValues: {
			proxy_config: {
				type: provider.proxy_config?.type,
				url: toSecretVarFormValue(provider.proxy_config?.url as SecretVar | string | undefined),
				username: toSecretVarFormValue(provider.proxy_config?.username as SecretVar | string | undefined),
				password: toSecretVarFormValue(provider.proxy_config?.password as SecretVar | string | undefined),
				ca_cert_pem: toSecretVarFormValue(provider.proxy_config?.ca_cert_pem as SecretVar | string | undefined),
			},
		},
	});

	useEffect(() => {
		dispatch(setProviderFormDirtyState(form.formState.isDirty));
	}, [form.formState.isDirty, dispatch]);

	useEffect(() => {
		form.reset({
			proxy_config: {
				type: provider.proxy_config?.type,
				url: toSecretVarFormValue(provider.proxy_config?.url as SecretVar | string | undefined),
				username: toSecretVarFormValue(provider.proxy_config?.username as SecretVar | string | undefined),
				password: toSecretVarFormValue(provider.proxy_config?.password as SecretVar | string | undefined),
				ca_cert_pem: toSecretVarFormValue(provider.proxy_config?.ca_cert_pem as SecretVar | string | undefined),
			},
		});
	}, [form, provider.name, provider.proxy_config]);

	const watchedProxyType = form.watch("proxy_config.type");

	const onSubmit = (data: ProxyOnlyFormSchema) => {
		updateProvider(
			buildProviderUpdatePayload(provider, {
				proxy_config: {
					type: data.proxy_config?.type ?? "none",
					url: toOptionalSecretVarPayload(data.proxy_config?.url),
					username: toOptionalSecretVarPayload(data.proxy_config?.username),
					password: toOptionalSecretVarPayload(data.proxy_config?.password),
					ca_cert_pem: toOptionalSecretVarPayload(data.proxy_config?.ca_cert_pem),
				},
			}),
		)
			.unwrap()
			.then(() => {
				toast.success(t("fragments.proxy.toastUpdated"));
				form.reset(data);
			})
			.catch((err) => {
				toast.error(t("fragments.proxy.toastUpdateFailed"), {
					description: getErrorMessage(err),
				});
			});
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6 px-6">
				{/* Proxy Configuration */}
				<Alert>
					<Info className="h-4 w-4" />
					<AlertDescription>
						{t("fragments.proxy.alertDescription")}
					</AlertDescription>
				</Alert>
				<div className="space-y-4">
					<div className="space-y-4">
						<FormField
							control={form.control}
							name="proxy_config.type"
							render={({ field }) => (
								<FormItem>
									<FormLabel>{t("fragments.proxy.proxyType")}</FormLabel>
									<Select
										onValueChange={field.onChange}
										value={field.value === "none" ? "" : field.value}
										disabled={!hasUpdateProviderAccess}
									>
										<FormControl>
											<SelectTrigger className="w-48">
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

						<div
							className={cn(
								"block transition-all duration-200",
								(!watchedProxyType || watchedProxyType === "none" || watchedProxyType === "environment") && "hidden",
							)}
						>
							<div className="space-y-4 pt-2">
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
											<FormDescription>
												{t("fragments.proxy.caCertDescription")}
											</FormDescription>
											<FormMessage />
										</FormItem>
									)}
								/>
							</div>
						</div>
					</div>
				</div>

				{/* Form Actions */}
				<div className="mb-6 flex items-center justify-end gap-2">
					{onCancel && (
						<Button type="button" variant="outline" size="sm" onClick={onCancel}>
							{t("fragments.cancel")}
						</Button>
					)}
					<Button
						type="button"
						variant="outline"
						onClick={() => {
							onSubmit({ proxy_config: { type: "none" } });
						}}
						disabled={!hasUpdateProviderAccess || isUpdatingProvider || !provider.proxy_config || provider.proxy_config.type === "none"}
					>
						{t("fragments.proxy.removeConfiguration")}
					</Button>
					<Button
						type="submit"
						disabled={!form.formState.isDirty || !hasUpdateProviderAccess || isUpdatingProvider}
						isLoading={isUpdatingProvider}
					>
						{t("fragments.proxy.saveConfiguration")}
					</Button>
				</div>
			</form>
		</Form>
	);
}