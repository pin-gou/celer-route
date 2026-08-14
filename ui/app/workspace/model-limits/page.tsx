import { useTranslation } from "react-i18next";
import ModelLimitsView from "./views/modelLimitsView";

export default function ModelLimitsPage() {
	const { t } = useTranslation("governance");
	return (
		<div className="no-padding-parent mx-auto flex h-[calc(100dvh-1rem)] min-h-0 w-full flex-col overflow-hidden p-4">
			<h1 className="sr-only">{t("modelLimits.title")}</h1>
			<ModelLimitsView />
		</div>
	);
}