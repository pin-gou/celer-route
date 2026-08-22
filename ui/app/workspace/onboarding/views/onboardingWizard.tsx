import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter } from "@/components/ui/card";
import { useOnboardingChecklist } from "@/hooks/useOnboardingChecklist";
import { useUpdateClientMetadataMutation } from "@/lib/store/apis/configApi";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { Check, ChevronLeft, ChevronRight } from "lucide-react";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import AdminSecurityStep from "../steps/adminSecurityStep";
import DoneStep from "../steps/doneStep";
import ProviderStep from "../steps/providerStep";
import TestStep from "../steps/testStep";
import WelcomeStep from "../steps/welcomeStep";

const STEP_IDS = ["welcome", "admin", "provider", "test", "done"] as const;
type StepId = (typeof STEP_IDS)[number];

const ONBOARDING_COMPLETE_KEY = "onboarding_complete";

export default function OnboardingWizard() {
	const { t } = useTranslation("onboarding");
	const navigate = useNavigate();
	const { data: providers } = useGetProvidersQuery();
	const { isFirstTimeSetup, bifrostConfig } = useOnboardingChecklist({});
	const [updateMetadata] = useUpdateClientMetadataMutation();

	const [step, setStep] = useState(0);
	const stepId: StepId = STEP_IDS[step];

	// Shared selections: TestStep picks provider+model, DoneStep inherits them
	// so the generated code samples reflect what the operator just tested.
	const [selectedProvider, setSelectedProvider] = useState<string | null>(null);
	const [selectedModel, setSelectedModel] = useState<string>("");
	const isLast = step === STEP_IDS.length - 1;

	// Allow the wizard to be re-run only when the operator explicitly chooses to
	// (via the "Re-run" link). On a fresh install we always show it; on existing
	// installs we still show it but flag the alreadySetUpNotice copy.
	if (!bifrostConfig) {
		return (
			<div className="text-muted-foreground flex min-h-[60vh] items-center justify-center text-sm">
				{t("firstRunGate.loading", { ns: "common" })}
			</div>
		);
	}

	const handleComplete = useCallback(async () => {
		try {
			await updateMetadata({ [ONBOARDING_COMPLETE_KEY]: true }).unwrap();
		} catch {
			// Non-critical: redirect anyway. Operator will see the wizard next session
			// only if they delete the metadata flag; that is acceptable.
		}
		navigate({ to: "/workspace/home", replace: true });
	}, [navigate, updateMetadata]);

	const handleSkipWizard = useCallback(async () => {
		try {
			await updateMetadata({ [ONBOARDING_COMPLETE_KEY]: true }).unwrap();
		} catch {
			// Non-critical
		}
		navigate({ to: "/workspace/home", replace: true });
	}, [navigate, updateMetadata]);

	const handleNext = useCallback(() => {
		if (isLast) {
			void handleComplete();
			return;
		}
		setStep((s) => Math.min(s + 1, STEP_IDS.length - 1));
	}, [handleComplete, isLast]);

	const handleBack = () => setStep((s) => Math.max(s - 1, 0));

	// The Provider step is only valid once at least one upstream provider has
	// been registered (whether or not it has keys — `opencode` is keyless). We
	// block the "Next" button until that is satisfied; the step itself also
	// shows a hint explaining why.
	const providerAdded = (providers?.length ?? 0) > 0;
	const canProceed = (() => {
		if (stepId === "admin") return true; // AdminSecurityStep does its own validation.
		if (stepId === "provider") return providerAdded;
		if (stepId === "test") return true;
		return true;
	})();

	return (
		<div className="flex items-start justify-center py-6">
			<div className="w-full max-w-2xl space-y-6">
				{!isFirstTimeSetup && (
					<div className="rounded-md border border-amber-300 bg-amber-50 px-4 py-2 text-xs text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
						{t("alreadySetUpNotice")}
					</div>
				)}

				<ProgressIndicator currentStep={step} totalSteps={STEP_IDS.length} />

				<Card className="bg-card gap-0 border py-0 shadow-sm">
					<CardContent className="space-y-6 px-8 py-8">
						{stepId === "welcome" && <WelcomeStep />}
						{stepId === "admin" && <AdminSecurityStep />}
						{stepId === "provider" && <ProviderStep />}
						{stepId === "test" && (
							<TestStep
								selectedProvider={selectedProvider}
								onProviderChange={setSelectedProvider}
								selectedModel={selectedModel}
								onModelChange={setSelectedModel}
							/>
						)}
						{stepId === "done" && (
							<DoneStep
								selectedProvider={selectedProvider}
								onProviderChange={setSelectedProvider}
								selectedModel={selectedModel}
								onModelChange={setSelectedModel}
							/>
						)}
					</CardContent>
					<CardFooter className="flex items-center justify-between border-t px-8 py-4">
						<Button variant="ghost" size="sm" disabled={step === 0} onClick={handleBack}>
							<ChevronLeft className="mr-1 h-4 w-4" />
							{t("back")}
						</Button>
						<div className="flex items-center gap-2">
							<Button
								variant="ghost"
								size="sm"
								onClick={() => {
									void handleSkipWizard();
									toast.info(t("skipWizard"));
								}}
								data-testid="onboarding-skip-wizard"
							>
								{t("skipWizard")}
							</Button>
							<Button size="sm" disabled={!canProceed} onClick={handleNext} data-testid="onboarding-next">
								{isLast ? (
									t("goToHome")
								) : stepId === "provider" && !providerAdded ? (
									t("providerRequired")
								) : (
									<>
										{t("next")}
										<ChevronRight className="ml-1 h-4 w-4" />
									</>
								)}
							</Button>
						</div>
					</CardFooter>
				</Card>

				<div className="text-muted-foreground text-center text-xs">{t("subtitle")}</div>
			</div>
		</div>
	);
}

function ProgressIndicator({ currentStep, totalSteps }: { currentStep: number; totalSteps: number }) {
	const { t } = useTranslation("onboarding");
	return (
		<div className="flex items-center justify-center gap-2">
			{Array.from({ length: totalSteps }).map((_, i) => {
				const id = STEP_IDS[i] as StepId;
				const completed = i < currentStep;
				const active = i === currentStep;
				return (
					<div key={id} className="flex items-center gap-2">
						<div
							className={[
								"flex h-8 w-8 items-center justify-center rounded-full text-xs font-semibold transition-all",
								completed
									? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
									: active
										? "bg-primary/15 text-primary ring-2 ring-primary/30"
										: "bg-muted text-muted-foreground",
							].join(" ")}
							aria-current={active ? "step" : undefined}
							data-testid={`onboarding-step-${id}`}
						>
							{completed ? <Check className="h-4 w-4" /> : i + 1}
						</div>
						<span className={["hidden text-xs sm:inline", active ? "font-medium text-foreground" : "text-muted-foreground"].join(" ")}>
							{t(`step.${id}`)}
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