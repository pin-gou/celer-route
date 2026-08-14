import { useTranslation } from "react-i18next";
import ObservabilityView from "./views/observabilityView";

export default function ObservabilityPage() {
	const { t } = useTranslation("observability");
	return (
		<div className="no-padding-parent mx-auto w-full max-w-7xl">
			<h1 className="sr-only">{t("page.title")}</h1>
			<ObservabilityView />
		</div>
	);
}