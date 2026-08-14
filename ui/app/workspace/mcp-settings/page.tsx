import { useTranslation } from "react-i18next";
import MCPView from "../config/views/mcpView";

export default function MCPSettingsPage() {
	const { t } = useTranslation("config");
	return (
		<div className="no-padding-parent mx-auto w-full max-w-7xl p-4">
			<h1 className="sr-only">{t("mcpSettings.title")}</h1>
			<MCPView />
		</div>
	);
}