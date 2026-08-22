import { createFileRoute } from "@tanstack/react-router";
import OnboardingWizard from "./views/onboardingWizard";

function RouteComponent() {
	return <OnboardingWizard />;
}

export const Route = createFileRoute("/workspace/onboarding")({
	component: RouteComponent,
});