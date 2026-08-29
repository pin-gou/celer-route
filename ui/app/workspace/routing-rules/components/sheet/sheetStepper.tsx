import { Check } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

export type SheetStepId = "basics" | "conditions" | "targets-and-fallbacks";

interface SheetStepperProps {
	current: SheetStepId;
	done: Record<SheetStepId, boolean>;
	onStepClick: (step: SheetStepId) => void;
}

interface StepDef {
	id: SheetStepId;
	iconKey: "info" | "filter" | "send";
	labelKey: string;
}

const STEPS: StepDef[] = [
	{ id: "basics", iconKey: "info", labelKey: "sheet.stepper.basics" },
	{ id: "conditions", iconKey: "filter", labelKey: "sheet.stepper.conditions" },
	{ id: "targets-and-fallbacks", iconKey: "send", labelKey: "sheet.stepper.targetsAndFallbacks" },
];

const ICONS: Record<StepDef["iconKey"], React.ReactNode> = {
	info: (
		<svg
			viewBox="0 0 24 24"
			className="h-3.5 w-3.5"
			fill="none"
			stroke="currentColor"
			strokeWidth="2"
			strokeLinecap="round"
			strokeLinejoin="round"
		>
			<circle cx="12" cy="12" r="9" />
			<path d="M12 8h.01M11 12h1v4h1" />
		</svg>
	),
	filter: (
		<svg
			viewBox="0 0 24 24"
			className="h-3.5 w-3.5"
			fill="none"
			stroke="currentColor"
			strokeWidth="2"
			strokeLinecap="round"
			strokeLinejoin="round"
		>
			<path d="M3 5h18M6 12h12M10 19h4" />
		</svg>
	),
	send: (
		<svg
			viewBox="0 0 24 24"
			className="h-3.5 w-3.5"
			fill="none"
			stroke="currentColor"
			strokeWidth="2"
			strokeLinecap="round"
			strokeLinejoin="round"
		>
			<path d="M5 12h14M13 6l6 6-6 6" />
		</svg>
	),
};

export function SheetStepper({ current, done, onStepClick }: SheetStepperProps) {
	const { t } = useTranslation("routing");
	const completedCount = STEPS.filter((s) => done[s.id]).length;
	const currentIndex = STEPS.findIndex((s) => s.id === current);

	return (
		<div className="flex flex-col gap-2" data-testid="routing-rule-sheet-stepper">
			<div role="tablist" aria-label={t("sheet.stepper.title")} className="bg-muted/30 grid grid-cols-3 gap-1 rounded-lg border p-1">
				{STEPS.map((step) => {
					const isCurrent = step.id === current;
					const isDone = done[step.id];
					return (
						<button
							key={step.id}
							type="button"
							role="tab"
							aria-selected={isCurrent}
							aria-controls={`routing-rule-tabpanel-${step.id}`}
							onClick={() => onStepClick(step.id)}
							className={cn(
								"flex cursor-pointer items-center justify-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors",
								isCurrent
									? "border-foreground/20 bg-background text-foreground font-semibold shadow-sm ring-1 ring-foreground/10"
									: "border-transparent text-muted-foreground/70 hover:bg-background/60 hover:text-foreground",
							)}
							data-testid={`routing-rule-sheet-step-${step.id}`}
						>
							<span
								className={cn(
									"flex h-5 w-5 shrink-0 items-center justify-center rounded-full border",
									isDone && "border-emerald-500 bg-emerald-500 text-white",
									!isDone && isCurrent && "border-foreground text-foreground",
									!isDone && !isCurrent && "border-muted-foreground/50 text-muted-foreground",
								)}
							>
								{isDone ? <Check className="h-3 w-3" /> : ICONS[step.iconKey]}
							</span>
							<span className="truncate text-xs font-medium">{t(step.labelKey)}</span>
						</button>
					);
				})}
			</div>
			<div className="text-muted-foreground flex items-center justify-between px-1 text-xs">
				<span data-testid="routing-rule-sheet-stepper-progress">
					{t("sheet.stepper.progress", { done: completedCount, total: STEPS.length })}
				</span>
				{currentIndex >= 0 && currentIndex < STEPS.length - 1 && (
					<span data-testid="routing-rule-sheet-stepper-next-hint">
						{t("sheet.stepper.nextHint", { next: t(STEPS[currentIndex + 1].labelKey) })}
					</span>
				)}
			</div>
		</div>
	);
}