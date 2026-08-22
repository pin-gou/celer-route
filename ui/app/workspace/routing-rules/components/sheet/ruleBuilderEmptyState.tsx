import { Compass } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { ExampleRuleKind } from "./insertExampleRule";

interface RuleBuilderEmptyStateProps {
	visible: boolean;
	onPick: (kind: ExampleRuleKind) => void;
}

const CHIPS: Array<{ kind: ExampleRuleKind; i18nKey: string; testId: string }> = [
	{ kind: "model", i18nKey: "sheet.ruleBuilderEmptyState.chipModel", testId: "routing-rule-example-chip-model" },
	{ kind: "provider", i18nKey: "sheet.ruleBuilderEmptyState.chipProvider", testId: "routing-rule-example-chip-provider" },
	{ kind: "requestType", i18nKey: "sheet.ruleBuilderEmptyState.chipRequestType", testId: "routing-rule-example-chip-request-type" },
];

export function RuleBuilderEmptyState({ visible, onPick }: RuleBuilderEmptyStateProps) {
	const { t } = useTranslation("routing");

	if (!visible) return null;

	return (
		<div
			className={cn("border-border bg-muted/30 flex flex-col gap-3 rounded-md border border-dashed px-4 py-5 text-sm")}
			data-testid="routing-rule-builder-empty-state"
		>
			<div className="text-muted-foreground flex items-start gap-2">
				<Compass className="mt-0.5 h-4 w-4 shrink-0" />
				<div className="space-y-1">
					<p className="text-foreground font-medium">{t("sheet.ruleBuilderEmptyState.title")}</p>
					<p className="text-xs">{t("sheet.ruleBuilderEmptyState.subtitle")}</p>
				</div>
			</div>
			<div className="flex flex-wrap gap-2" role="group" aria-label={t("sheet.ruleBuilderEmptyState.title")}>
				{CHIPS.map((chip) => (
					<button
						key={chip.kind}
						type="button"
						onClick={() => onPick(chip.kind)}
						className={cn("border-input bg-background hover:bg-muted rounded-full border px-3 py-1 text-xs font-medium transition-colors")}
						data-testid={chip.testId}
					>
						+ {t(chip.i18nKey)}
					</button>
				))}
			</div>
			<p className="text-muted-foreground text-xs">{t("sheet.ruleBuilderEmptyState.helper")}</p>
		</div>
	);
}