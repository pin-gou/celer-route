import { useOnboardingChecklist } from "@/hooks/useOnboardingChecklist";
import { Navigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import EndpointCard from "../components/endpointCard";
import FreeTierRecommendationCard from "../components/freeTierRecommendationCard";
import ProviderTopologyCard from "../components/providerTopologyCard";
import SystemHealthCard from "../components/systemHealthCard";

function resolveEndpoint(): string {
	if (typeof window === "undefined") return "http://localhost:8080/v1";
	return `${window.location.origin}/v1`;
}

export default function HomePage() {
	const { t } = useTranslation("common");
	const { isFirstTimeSetup, checklistReady } = useOnboardingChecklist({});
	const [endpoint, setEndpoint] = useState<string>("http://localhost:8080/v1");

	useEffect(() => {
		setEndpoint(resolveEndpoint());
	}, []);

	// First-run gate: send users without an admin to the onboarding wizard.
	if (checklistReady && isFirstTimeSetup) {
		return <Navigate to="/workspace/onboarding" replace />;
	}

	return (
		<div className="mx-auto w-full max-w-7xl space-y-4">
			<header className="space-y-1">
				<h1 className="text-2xl font-semibold tracking-tight">{t("home.pageTitle")}</h1>
			</header>

			<SystemHealthCard />
			<FreeTierRecommendationCard />
			<ProviderTopologyCard />
			<EndpointCard endpointUrl={endpoint} />
		</div>
	);
}