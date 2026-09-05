import { Check } from "lucide-react";

export interface ProgressIndicatorProps {
	currentStep: number;
	totalSteps: number;
	stepIds: readonly string[];
	labels: string[];
	testIdPrefix: string;
	onStepClick?: (stepIndex: number) => void;
}

export function ProgressIndicator({ currentStep, totalSteps, stepIds, labels, testIdPrefix, onStepClick }: ProgressIndicatorProps) {
	return (
		<div className="flex items-center justify-center gap-2" data-testid={`${testIdPrefix}-progress`}>
			{Array.from({ length: totalSteps }).map((_, i) => {
				const id = stepIds[i] ?? `step-${i}`;
				const completed = i < currentStep;
				const active = i === currentStep;
				const clickable = typeof onStepClick === "function";
				const baseClass = [
					"flex h-8 w-8 items-center justify-center rounded-full text-xs font-semibold transition-all",
					completed
						? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
						: active
							? "bg-primary/15 text-primary ring-2 ring-primary/30"
							: "bg-muted text-muted-foreground",
					clickable && !active ? "cursor-pointer hover:opacity-80" : "",
				].join(" ");
				return (
					<div key={id} className="flex items-center gap-2">
						{clickable ? (
							<button
								type="button"
								className={baseClass}
								aria-current={active ? "step" : undefined}
								aria-label={labels[i] ?? id}
								data-testid={`${testIdPrefix}-step-${id}`}
								onClick={() => onStepClick?.(i)}
							>
								{completed ? <Check className="h-4 w-4" /> : i + 1}
							</button>
						) : (
							<div className={baseClass} aria-current={active ? "step" : undefined} data-testid={`${testIdPrefix}-step-${id}`}>
								{completed ? <Check className="h-4 w-4" /> : i + 1}
							</div>
						)}
						<span className={["hidden text-xs sm:inline", active ? "font-medium text-foreground" : "text-muted-foreground"].join(" ")}>
							{labels[i] ?? id}
						</span>
						{i < totalSteps - 1 && (
							<div className={["h-0.5 w-6 rounded-full transition-colors", completed ? "bg-emerald-500/40" : "bg-muted"].join(" ")} />
						)}
					</div>
				);
			})}
		</div>
	);
}