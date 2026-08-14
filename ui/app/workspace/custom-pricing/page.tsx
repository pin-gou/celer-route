import { useTranslation } from "react-i18next";
import ModelSettingsView from "@/app/workspace/config/views/modelSettingsView";

export default function CustomPricingPage() {
	const { t } = useTranslation("config");
	return (
		<div className="mx-auto w-full max-w-7xl">
			<h1 className="sr-only">{t("customPricing.title")}</h1>
			<ModelSettingsView />
		</div>
	);
}