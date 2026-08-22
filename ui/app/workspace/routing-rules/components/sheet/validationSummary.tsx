import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { AlertTriangle, CheckCircle2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { RuleGroupType } from "react-querybuilder";
import { validateRateLimitAndBudgetRules, validateRoutingRules } from "@/lib/utils/celConverterRouting";

export interface ValidationItem {
	id: string;
	message: string;
}

interface ValidationSummaryProps {
	name: string;
	priority: number;
	scope: string;
	scopeId: string;
	targets: Array<{ weight: number }>;
	totalWeight: number;
	query: RuleGroupType;
	conditionMode: "builder" | "cel";
}

type TFunc = (key: string, opts?: Record<string, unknown>) => string;

export function computeValidationErrors(
	t: TFunc,
	name: string,
	priority: number,
	scope: string,
	scopeId: string,
	targets: Array<{ weight: number }>,
	totalWeight: number,
	query: RuleGroupType,
	conditionMode: "builder" | "cel",
): ValidationItem[] {
	const errors: ValidationItem[] = [];

	if (!name.trim()) {
		errors.push({ id: "name", message: t("sheet.validation.nameRequired") });
	}

	if (scope !== "global" && !scopeId.trim()) {
		errors.push({ id: "scope", message: t("sheet.validation.scopeRequired") });
	}

	if (isNaN(priority) || priority < 0 || priority > 1000) {
		errors.push({ id: "priority", message: t("sheet.validation.priorityRange") });
	}

	if (targets.length === 0) {
		errors.push({ id: "targets-empty", message: t("sheet.validation.targetRequired") });
	} else {
		for (let i = 0; i < targets.length; i++) {
			if (targets[i].weight <= 0) {
				errors.push({ id: `target-weight-${i}`, message: t("sheet.validation.weightPositive") });
				break;
			}
		}
		if (Math.abs(totalWeight - 1) > 0.001) {
			errors.push({ id: "weight-sum", message: t("sheet.validation.weightSum", { total: totalWeight.toFixed(4) }) });
		}
	}

	if (conditionMode === "builder") {
		const regexErrors = validateRoutingRules(query);
		if (regexErrors.length > 0) {
			errors.push({ id: "regex", message: t("sheet.invalidRegex", { errors: regexErrors.join("\n") }) });
		}
		const rateLimitErrors = validateRateLimitAndBudgetRules(query);
		if (rateLimitErrors.length > 0) {
			errors.push({ id: "rate-limit", message: t("sheet.invalidRuleConfig", { errors: rateLimitErrors.join("\n") }) });
		}
	}

	return errors;
}

export function ValidationSummary(props: ValidationSummaryProps) {
	const { t } = useTranslation("routing");

	const errors = computeValidationErrors(
		t,
		props.name,
		props.priority,
		props.scope,
		props.scopeId,
		props.targets,
		props.totalWeight,
		props.query,
		props.conditionMode,
	);

	if (errors.length === 0) {
		return (
			<Alert variant="default" className="border-emerald-500/50 bg-emerald-500/5" data-testid="routing-rule-validation-ok">
				<CheckCircle2 className="h-4 w-4 text-emerald-600" />
				<AlertDescription className="text-emerald-700 dark:text-emerald-400">{t("sheet.validation.ready")}</AlertDescription>
			</Alert>
		);
	}

	return (
		<Alert variant="destructive" data-testid="routing-rule-validation-errors">
			<AlertTriangle className="h-4 w-4" />
			<AlertTitle>{t("sheet.validation.title")}</AlertTitle>
			<AlertDescription>
				<ul className="ml-4 list-disc space-y-0.5">
					{errors.map((err) => (
						<li key={err.id}>{err.message}</li>
					))}
				</ul>
			</AlertDescription>
		</Alert>
	);
}