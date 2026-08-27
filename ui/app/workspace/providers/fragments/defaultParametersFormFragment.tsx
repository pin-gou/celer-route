import { Button } from "@/components/ui/button";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { getErrorMessage, setProviderFormDirtyState, useAppDispatch } from "@/lib/store";
import { useGetModelsQuery, useUpdateProviderMutation } from "@/lib/store/apis/providersApi";
import { defaultParametersFormSchema, type DefaultParametersFormSchema } from "@/lib/types/schemas";
import { useTranslation } from "react-i18next";
import { type ModelProvider } from "@/lib/types/config";
import { providerModelHasDefaultParams } from "@/lib/utils/defaultParameters";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo } from "react";
import { useFieldArray, useForm, type Resolver } from "react-hook-form";
import { toast } from "sonner";
import { buildProviderUpdatePayload } from "../views/utils";
import { PlusIcon, Trash2Icon } from "lucide-react";

interface DefaultParametersFormFragmentProps {
	provider: ModelProvider;
	onCancel?: () => void;
}

// Convert the nested map (model → param → value) from the API into the flat
// row array used by the form. Returns [] when the map is empty or absent.
function flattenDefaultParameters(
	defaults: Record<string, Record<string, string | number | boolean>> | undefined,
): DefaultParametersFormSchema["rows"] {
	if (!defaults) return [];
	const rows: DefaultParametersFormSchema["rows"] = [];
	for (const [model, params] of Object.entries(defaults)) {
		for (const [param, value] of Object.entries(params)) {
			rows.push({ model, param, value: String(value) });
		}
	}
	return rows;
}

// Convert the flat row array from the form into the nested map expected by the
// API. Duplicate (model, param) pairs are silently deduplicated (last wins).
function nestDefaultParameters(rows: DefaultParametersFormSchema["rows"]): Record<string, Record<string, string | number | boolean>> {
	const map: Record<string, Record<string, string | number | boolean>> = {};
	for (const row of rows) {
		if (!row.model || !row.param || !row.value) continue;
		if (!map[row.model]) map[row.model] = {};
		map[row.model][row.param] = row.value;
	}
	return map;
}

export function DefaultParametersFormFragment({ provider, onCancel }: DefaultParametersFormFragmentProps) {
	const { t } = useTranslation("providers");
	const dispatch = useAppDispatch();
	const hasUpdateProviderAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const [updateProvider, { isLoading: isUpdatingProvider }] = useUpdateProviderMutation();

	// Supported default params for this provider, registered in the backend
	// schema catalog. Empty means the module is hidden entirely (handled by
	// the tab gating); here it just yields no param options.
	const definitions = provider.default_parameters_definitions ?? [];

	// Fetch all models for this provider, but only expose the ones that accept
	// at least one registered default param (e.g. sensenova reasoning_effort
	// applies only to deepseek-v4-flash / glm-5.2).
	const { data: modelsData } = useGetModelsQuery({ provider: provider.name, unfiltered: true }, { skip: !provider.name });
	const modelOptions = useMemo(() => {
		if (!modelsData?.models) return [];
		return modelsData.models
			.filter((m) => !m.is_deprecated)
			.filter((m) => providerModelHasDefaultParams(definitions, m.name))
			.map((m) => ({ value: m.name, label: m.name }));
	}, [modelsData, definitions]);

	const form = useForm<DefaultParametersFormSchema, any, DefaultParametersFormSchema>({
		resolver: zodResolver(defaultParametersFormSchema) as Resolver<DefaultParametersFormSchema, any, DefaultParametersFormSchema>,
		mode: "onChange",
		reValidateMode: "onChange",
		defaultValues: {
			rows: flattenDefaultParameters(provider.default_parameters),
		},
	});

	const { fields, append, remove } = useFieldArray({
		control: form.control,
		name: "rows",
	});

	useEffect(() => {
		dispatch(setProviderFormDirtyState(form.formState.isDirty));
	}, [form.formState.isDirty, dispatch]);

	useEffect(() => {
		form.reset({
			rows: flattenDefaultParameters(provider.default_parameters),
		});
	}, [form, provider.name, provider.default_parameters]);

	const onSubmit = (data: DefaultParametersFormSchema) => {
		updateProvider(
			buildProviderUpdatePayload(provider, {
				default_parameters: nestDefaultParameters(data.rows),
			}),
		)
			.unwrap()
			.then(() => {
				toast.success(t("fragments.defaultParameters.toast.updated"));
				form.reset(data);
			})
			.catch((err) => {
				toast.error(t("fragments.defaultParameters.toast.failedToUpdate"), {
					description: getErrorMessage(err),
				});
			});
	};

	const addRow = () => {
		append({ model: "", param: definitions[0]?.key ?? "", value: definitions[0]?.options[0] ?? "" });
		form.trigger("rows");
	};

	return (
		<Form {...form}>
			<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6 px-6" data-testid="provider-config-default-parameters-content">
				<div className="space-y-4">
					<p className="text-muted-foreground text-xs">{t("fragments.defaultParameters.description")}</p>

					{fields.length === 0 && <p className="text-muted-foreground text-xs">{t("fragments.defaultParameters.noRows")}</p>}

					{fields.map((field, index) => (
						<div key={field.id} className="flex items-start gap-2">
							{/* Model Select */}
							<FormField
								control={form.control}
								name={`rows.${index}.model`}
								render={({ field: modelField }) => (
									<FormItem className="flex-1">
										{index === 0 && <FormLabel>{t("fragments.defaultParameters.model")}</FormLabel>}
										<Select onValueChange={modelField.onChange} value={modelField.value} disabled={!hasUpdateProviderAccess}>
											<FormControl>
												<SelectTrigger className="w-full" data-testid={`provider-default-parameters-model-${index}`}>
													<SelectValue placeholder={t("fragments.defaultParameters.selectModel")} />
												</SelectTrigger>
											</FormControl>
											<SelectContent>
												{modelOptions.map((opt) => (
													<SelectItem key={opt.value} value={opt.value}>
														{opt.label}
													</SelectItem>
												))}
											</SelectContent>
										</Select>
										<FormMessage />
									</FormItem>
								)}
							/>

							{/* Param Select */}
							<FormField
								control={form.control}
								name={`rows.${index}.param`}
								render={({ field: paramField }) => (
									<FormItem className="flex-1">
										{index === 0 && <FormLabel>{t("fragments.defaultParameters.param")}</FormLabel>}
										<Select onValueChange={paramField.onChange} value={paramField.value} disabled={!hasUpdateProviderAccess}>
											<FormControl>
												<SelectTrigger className="w-full" data-testid={`provider-default-parameters-param-${index}`}>
													<SelectValue placeholder={t("fragments.defaultParameters.selectParam")} />
												</SelectTrigger>
											</FormControl>
											<SelectContent>
												{definitions.map((d) => (
													<SelectItem key={d.key} value={d.key}>
														{d.label}
													</SelectItem>
												))}
											</SelectContent>
										</Select>
										<FormMessage />
									</FormItem>
								)}
							/>

							{/* Value Select */}
							<FormField
								control={form.control}
								name={`rows.${index}.value`}
								render={({ field: valueField }) => {
									const selectedDef = definitions.find((d) => d.key === form.watch(`rows.${index}.param`));
									const options = selectedDef?.options ?? [];
									return (
										<FormItem className="flex-1">
											{index === 0 && <FormLabel>{t("fragments.defaultParameters.value")}</FormLabel>}
											<Select onValueChange={valueField.onChange} value={valueField.value} disabled={!hasUpdateProviderAccess}>
												<FormControl>
													<SelectTrigger className="w-full" data-testid={`provider-default-parameters-value-${index}`}>
														<SelectValue placeholder={t("fragments.defaultParameters.selectValue")} />
													</SelectTrigger>
												</FormControl>
												<SelectContent>
													{options.map((opt) => (
														<SelectItem key={opt} value={opt}>
															{opt}
														</SelectItem>
													))}
												</SelectContent>
											</Select>
											<FormMessage />
										</FormItem>
									);
								}}
							/>

							{/* Remove button */}
							<div className={index === 0 ? "pt-7" : "pt-0"}>
								<Button
									type="button"
									variant="ghost"
									size="sm"
									onClick={() => {
										remove(index);
										form.trigger("rows");
									}}
									disabled={!hasUpdateProviderAccess}
									data-testid={`provider-default-parameters-remove-${index}`}
								>
									<Trash2Icon className="h-4 w-4" />
								</Button>
							</div>
						</div>
					))}

					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={addRow}
						disabled={!hasUpdateProviderAccess}
						data-testid="provider-default-parameters-add"
					>
						<PlusIcon className="mr-1 h-3 w-3" />
						{t("fragments.defaultParameters.addRow")}
					</Button>
				</div>

				<div className="flex items-center justify-end gap-2 pb-6">
					{onCancel && (
						<Button type="button" variant="outline" size="sm" onClick={onCancel}>
							{t("fragments.defaultParameters.cancel")}
						</Button>
					)}
					<Button
						type="submit"
						disabled={!form.formState.isDirty || !form.formState.isValid || !hasUpdateProviderAccess || isUpdatingProvider}
						isLoading={isUpdatingProvider}
						data-testid="provider-default-parameters-save"
					>
						{t("fragments.defaultParameters.save")}
					</Button>
				</div>
			</form>
		</Form>
	);
}