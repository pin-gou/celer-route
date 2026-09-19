import { useTranslation } from "react-i18next";
import MCPView from "../config/views/mcpView";

export default function MCPSettingsPage() {
	const { t } = useTranslation("config");
	return (
		<div className="mx-auto w-full max-w-7xl">
			<h1 className="sr-only">{t("mcpSettings.title")}</h1>
			<MCPView />
		</div>
	);
}