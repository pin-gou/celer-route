import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useOnboardingChecklist } from "@/hooks/useOnboardingChecklist";
import { CheckCircle2, Circle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "@tanstack/react-router";

export default function SetupStatusBar() {
	const { t } = useTranslation("common");
	const { steps, skippedIds, checklistReady } = useOnboardingChecklist({});
	const navigate = useNavigate();

	if (!checklistReady) return null;

	const doneCount = steps.filter((s) => s.complete || skippedIds.includes(s.id)).length;
	const allDone = doneCount === steps.length;

	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid="home-setup-status">
			<CardHeader className="flex flex-row items-center justify-between gap-2 border-b px-6 py-3">
				<CardTitle className="text-sm font-semibold">{t("home.setupStatus.title")}</CardTitle>
				<span className={["text-xs", allDone ? "font-medium text-emerald-600" : "text-muted-foreground"].join(" ")}>
					{allDone ? t("home.setupStatus.complete") : t("home.setupStatus.remaining", { n: String(steps.length - doneCount) })}
				</span>
			</CardHeader>
			<CardContent className="px-6 py-4">
				<ol className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
					{steps.map((step) => {
						const done = step.complete || skippedIds.includes(step.id);
						return (
							<li key={step.id}>
								<button
									type="button"
									onClick={() => navigate({ to: step.route })}
									className={[
										"flex w-full items-start gap-2 rounded-md border p-3 text-left transition-colors",
										done ? "border-emerald-500/30 bg-emerald-500/5" : "border-border bg-muted/20 hover:bg-muted/40",
									].join(" ")}
									data-testid={`home-setup-status-step-${step.id}`}
								>
									{done ? (
										<CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
									) : (
										<Circle className="text-muted-foreground mt-0.5 h-4 w-4 shrink-0" />
									)}
									<div className="min-w-0">
										<div className="truncate text-xs font-medium">{step.title}</div>
										<div className="text-muted-foreground text-[10px]">{t("home.setupStatus.goToStep")}</div>
									</div>
								</button>
							</li>
						);
					})}
				</ol>
			</CardContent>
		</Card>
	);
}