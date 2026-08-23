/**
 * Field Selector Component for CEL Rule Builder
 * Allows selection of fields for building CEL expressions
 * Fields are grouped by category (Request Properties / Metadata / Usage & Budget)
 * For keyValue fields (headers/params), also renders "has value" label and key input
 */

import { Input } from "@/components/ui/input";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectSeparator,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { FIELD_GROUP_ORDER, FieldGroup } from "@/lib/config/celFieldsRouting";
import { Info } from "lucide-react";
import { useCallback, useMemo } from "react";
import { FieldSelectorProps, RuleGroupType, RuleType } from "react-querybuilder";
import { useTranslation } from "react-i18next";

/**
 * Recursively find and update a rule's value by path in the query tree.
 */
function updateRuleValueAtPath(query: RuleGroupType, targetPath: number[], newValue: string): RuleGroupType {
	if (targetPath.length === 0) return query;

	const [currentIndex, ...restPath] = targetPath;
	const newRules = [...query.rules];

	if (restPath.length === 0) {
		// We're at the target rule
		const rule = newRules[currentIndex] as RuleType;
		newRules[currentIndex] = { ...rule, value: newValue };
	} else {
		// Recurse into nested group
		newRules[currentIndex] = updateRuleValueAtPath(newRules[currentIndex] as RuleGroupType, restPath, newValue);
	}

	return { ...query, rules: newRules };
}

interface FieldOption {
	name: string;
	label: string;
	value?: string;
	disabled?: boolean;
	group?: FieldGroup;
	description?: string;
	options?: unknown;
	[key: string]: unknown;
}

export function FieldSelector({ value, handleOnChange, options, rule, path, schema }: FieldSelectorProps) {
	const { t } = useTranslation("routing");

	// Check if this is a keyValue field (headers/params)
	const fieldData = useMemo(() => schema?.fields?.find((f) => "value" in f && f.value === value), [schema?.fields, value]);
	const isKeyValueField = fieldData && "inputType" in fieldData && fieldData.inputType === "keyValue";

	// Parse the key from the rule's value ("key:value" or just "key")
	const headerKey = useMemo(() => {
		if (!isKeyValueField || !rule?.value || typeof rule.value !== "string") return "";
		const colonIndex = rule.value.indexOf(":");
		if (colonIndex > 0) return rule.value.substring(0, colonIndex).trim();
		return rule.value.trim();
	}, [isKeyValueField, rule?.value]);

	// Localized field label for the key-name placeholder
	const keyLabel = useMemo(() => {
		if (!fieldData || !("name" in fieldData)) return "Key";
		return t(`sheet.fieldLabel.${fieldData.name}`, fieldData.label || "Key");
	}, [fieldData, t]);

	// Group the flat option list into ordered categories: request → metadata → usage
	const groupedOptions = useMemo(() => {
		const flat = (options as FieldOption[]).filter((opt) => !("options" in opt) && opt.name && opt.name !== "~" && opt.value !== "~");
		const byGroup = new Map<FieldGroup, FieldOption[]>();
		for (const opt of flat) {
			const g = opt.group || "request";
			if (!byGroup.has(g)) byGroup.set(g, []);
			byGroup.get(g)!.push(opt);
		}
		return FIELD_GROUP_ORDER.map((g) => ({ group: g, options: byGroup.get(g) || [] })).filter((grp) => grp.options.length > 0);
	}, [options]);

	const handleKeyChange = useCallback(
		(newKey: string) => {
			if (!schema || !path) return;
			// Preserve the existing value part
			const currentValue = typeof rule?.value === "string" ? rule.value : "";
			const colonIndex = currentValue.indexOf(":");
			const valuePart = colonIndex > 0 ? currentValue.substring(colonIndex + 1).trim() : "";

			let updatedValue: string;
			if (newKey && valuePart) {
				updatedValue = `${newKey}:${valuePart}`;
			} else if (newKey) {
				updatedValue = newKey;
			} else {
				updatedValue = "";
			}

			// Update the rule value via query dispatch
			const currentQuery = schema.getQuery() as RuleGroupType;
			const updatedQuery = updateRuleValueAtPath(currentQuery, path, updatedValue);
			schema.dispatchQuery(updatedQuery);
		},
		[schema, path, rule?.value],
	);

	return (
		<div className="flex items-center gap-2">
			<Select value={value || ""} onValueChange={handleOnChange}>
				<SelectTrigger className="w-[180px]" data-testid="cel-builder-field-selector-select">
					<SelectValue placeholder={t("sheet.fieldSelectPlaceholder")} />
				</SelectTrigger>
				<SelectContent>
					{groupedOptions.map((group, index) => (
						<SelectGroup key={group.group}>
							{index > 0 && <SelectSeparator />}
							<SelectLabel>{t(`sheet.fieldGroup.${group.group}`)}</SelectLabel>
							{group.options.map((option) => {
								const description = t(`sheet.fieldDescription.${option.name}`, option.description || "");
								return (
									<SelectItem key={option.name} value={option.name} disabled={option.disabled}>
										<span className="flex-1 truncate">{t(`sheet.fieldLabel.${option.name}`, option.label)}</span>
										{description && (
											<TooltipProvider delayDuration={150}>
												<Tooltip>
													<TooltipTrigger asChild>
														<span tabIndex={-1} className="text-muted-foreground ml-auto flex size-4 shrink-0 items-center justify-center">
															<Info className="size-3.5" />
														</span>
													</TooltipTrigger>
													<TooltipContent side="right" className="z-[10001] max-w-xs text-xs">
														{description}
													</TooltipContent>
												</Tooltip>
											</TooltipProvider>
										)}
									</SelectItem>
								);
							})}
						</SelectGroup>
					))}
				</SelectContent>
			</Select>
			{isKeyValueField && (
				<>
					<span className="text-muted-foreground text-sm whitespace-nowrap">{t("sheet.keyValueHasKey")}</span>
					<Input
						type="text"
						value={headerKey}
						onChange={(e) => handleKeyChange(e.target.value)}
						placeholder={t("sheet.keyValueKeyName", { label: keyLabel })}
						className="w-[180px]"
						data-testid="cel-builder-field-selector-key-input"
					/>
				</>
			)}
		</div>
	);
}