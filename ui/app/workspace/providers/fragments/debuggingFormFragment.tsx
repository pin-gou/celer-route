import { Button } from "@/components/ui/button";
import { useTranslation } from "react-i18next";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage, setProviderFormDirtyState, useAppDispatch } from "@/lib/store";
import { useUpdateProviderMutation } from "@/lib/store/apis/providersApi";
import { ModelProvider } from "@/lib/types/config";
import { debuggingFormSchema, type DebuggingFormSchema } from "@/lib/types/schemas";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { zodResolver } from "@hookform/resolvers/zod";
import { Info } from "lucide-react";
import { useEffect } from "react";
import { useForm, type Resolver } from "react-hook-form";
import { toast } from "sonner";
import { buildProviderUpdatePayload } from "../views/utils";

interface DebuggingFormFragmentProps {
	provider: ModelProvider;
	onCancel?: () => void;
}

export function DebuggingFormFragment({ provider, onCancel }: DebuggingFormFragmentProps) {
	const { t } = useTranslation("providers");
	const dispatch = useAppDispatch();
	const hasUpdateProviderAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const [updateProvider, { isLoading: isUpdatingProvider }] = useUpdateProviderMutation();
	const form = useForm<DebuggingFormSchema, any, DebuggingFormSchema>({
		resolver: zodResolver(debuggingFormSchema) as Resolver<DebuggingFormSchema, any, DebuggingFormSchema>,
		mode: "onChange",
		reValidateMode: "onChange",
		defaultValues: {
			send_back_raw_request: provider.send_back_raw_request ?? false,
			send_back_raw_response: provider.send_back_raw_response ?? false,
			store_raw_request_response: provider.store_raw_request_response ?? false,
		},
	});
	const sendBackRawRequest = form.watch("send_back_raw_request");
	const sendBackRawResponse = form.watch("send_back_raw_response");
	const storeRawRequestResponse = form.watch("store_raw_request_response");

	useEffect(() => {
		dispatch(setProviderFormDirtyState(form.formState.isDirty));
	}, [form.formState.isDirty, dispatch]);

	useEffect(() => {
		form.reset({
			send_back_raw_request: provider.send_back_raw_request ?? false,
			send_back_raw_response: provider.send_back_raw_response ?? false,
			store_raw_request_response: provider.store_raw_request_response ?? false,
		});
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [provider.name, provider.send_back_raw_request, provider.send_back_raw_response, provider.store_raw_request_response]);

	const onSubmit = (data: DebuggingFormSchema) => {
		const updatedProvider = buildProviderUpdatePayload(provider, {
			send_back_raw_request: data.send_back_raw_request,
			send_back_raw_response: data.send_back_raw_response,
			store_raw_request_response: data.store_raw_request_response,
		});
		updateProvider(updatedProvider)
			.unwrap()
			.then(() => {
				toast.success(t("fragments.debugging.toastSuccess"));
				form.reset(data);
			})
			.catch((err) => {
				toast.error(t("fragments.debugging.toastError"), {
					description: getErrorMessage(err),
				});
			});
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6 p-6" data-testid="provider-config-debugging-content">
				<div className="space-y-4">
					{/* Send Back Raw Request */}
					<FormField
						control={form.control}
						name="send_back_raw_request"
						render={({ field }) => (
							<FormItem>
								<div className="flex items-center justify-between space-x-2">
									<div className="space-y-0.5">
										<div className="flex items-center gap-1.5">
											<FormLabel>{t("fragments.debugging.sendBackRawRequest")}</FormLabel>
											<TooltipProvider>
												<Tooltip>
													<TooltipTrigger asChild data-testid="provider-debugging-send-back-raw-request-tooltip-trigger">
														<Info className="text-muted-foreground h-3 w-3 cursor-pointer" />
													</TooltipTrigger>
													<TooltipContent>
														{t("fragments.debugging.sendBackRawRequestTooltip", { value: String(!sendBackRawRequest) })}
													</TooltipContent>
												</Tooltip>
											</TooltipProvider>
										</div>
										<p className="text-muted-foreground text-xs">
											{t("fragments.debugging.sendBackRawRequestDescription")}
										</p>
									</div>
									<FormControl>
										<Switch
											size="md"
											checked={field.value}
											disabled={!hasUpdateProviderAccess}
											onCheckedChange={(checked) => {
												field.onChange(checked);
												form.trigger("send_back_raw_request");
											}}
										/>
									</FormControl>
								</div>
								<FormMessage />
							</FormItem>
						)}
					/>

					{/* Send Back Raw Response */}
					<FormField
						control={form.control}
						name="send_back_raw_response"
						render={({ field }) => (
							<FormItem>
								<div className="flex items-center justify-between space-x-2">
									<div className="space-y-0.5">
										<div className="flex items-center gap-1.5">
											<FormLabel>{t("fragments.debugging.sendBackRawResponse")}</FormLabel>
											<TooltipProvider>
												<Tooltip>
													<TooltipTrigger asChild data-testid="provider-debugging-send-back-raw-response-tooltip-trigger">
														<Info className="text-muted-foreground h-3 w-3 cursor-pointer" />
													</TooltipTrigger>
													<TooltipContent>
														{t("fragments.debugging.sendBackRawResponseTooltip", { value: String(!sendBackRawResponse) })}
													</TooltipContent>
												</Tooltip>
											</TooltipProvider>
										</div>
										<p className="text-muted-foreground text-xs">
											{t("fragments.debugging.sendBackRawResponseDescription")}
										</p>
									</div>
									<FormControl>
										<Switch
											size="md"
											checked={field.value}
											disabled={!hasUpdateProviderAccess}
											onCheckedChange={(checked) => {
												field.onChange(checked);
												form.trigger("send_back_raw_response");
											}}
										/>
									</FormControl>
								</div>
								<FormMessage />
							</FormItem>
						)}
					/>

					{/* Store Raw Request/Response */}
					<FormField
						control={form.control}
						name="store_raw_request_response"
						render={({ field }) => (
							<FormItem>
								<div className="flex items-center justify-between space-x-2">
									<div className="space-y-0.5">
										<div className="flex items-center gap-1.5">
											<FormLabel>{t("fragments.debugging.storeRawRequestResponse")}</FormLabel>
											<TooltipProvider>
												<Tooltip>
													<TooltipTrigger asChild data-testid="provider-debugging-store-raw-request-response-tooltip-trigger">
														<Info className="text-muted-foreground h-3 w-3 cursor-pointer" />
													</TooltipTrigger>
													<TooltipContent>
														{t("fragments.debugging.storeRawRequestResponseTooltip", { value: String(!storeRawRequestResponse) })}
													</TooltipContent>
												</Tooltip>
											</TooltipProvider>
										</div>
										<p className="text-muted-foreground text-xs">
											{t("fragments.debugging.storeRawRequestResponseDescription")}
										</p>
									</div>
									<FormControl>
										<Switch
											data-testid="provider-debugging-store-raw-request-response-switch"
											size="md"
											checked={field.value}
											disabled={!hasUpdateProviderAccess}
											onCheckedChange={(checked) => {
												field.onChange(checked);
												form.trigger("store_raw_request_response");
											}}
										/>
									</FormControl>
								</div>
								<FormMessage />
							</FormItem>
						)}
					/>
				</div>

				<div className="flex items-center justify-end gap-2">
					{onCancel && (
						<Button type="button" variant="outline" size="sm" onClick={onCancel}>
{t("fragments.debugging.cancel")}
						</Button>
					)}
					<Button
						type="submit"
						disabled={!form.formState.isDirty || !hasUpdateProviderAccess || isUpdatingProvider}
						isLoading={isUpdatingProvider}
					>
						{t("fragments.debugging.save")}
					</Button>
				</div>
			</form>
		</Form>
	);
}