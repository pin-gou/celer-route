import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardTitle } from "@/components/ui/card";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

export function PromptsFragment() {
	const { t } = useTranslation("plugins");
	const navigate = useNavigate();

	return (
		<Card data-testid="prompts-placeholder-card" className="w-full">
			<CardContent className="space-y-4">
				<CardTitle>{t("placeholderConfig.pluginTitle", { name: t("pluginNames.prompts") })}</CardTitle>
				<CardDescription>{t("placeholderConfig.manageViaCrudDescription")}</CardDescription>
				<Button onClick={() => navigate({ to: "/workspace/prompt-repo/prompts" })}>{t("placeholderConfig.promptsCta")}</Button>
			</CardContent>
		</Card>
	);
}

export function ModelcatalogresolverFragment() {
	const { t } = useTranslation("plugins");
	const navigate = useNavigate();

	return (
		<Card data-testid="modelcatalogresolver-placeholder-card" className="w-full">
			<CardContent className="space-y-4">
				<CardTitle>{t("placeholderConfig.pluginTitle", { name: t("pluginNames.modelcatalogresolver") })}</CardTitle>
				<CardDescription>{t("placeholderConfig.model catalog description")}</CardDescription>
				<Button onClick={() => navigate({ to: "/workspace/model-catalog" })}>{t("placeholderConfig.pluginCta")}</Button>
			</CardContent>
		</Card>
	);
}

export function JsonparserFragment() {
	const { t } = useTranslation("plugins");
	const navigate = useNavigate();

	return (
		<Card data-testid="jsonparser-placeholder-card" className="w-full">
			<CardContent className="space-y-4">
				<CardTitle>{t("placeholderConfig.pluginTitle", { name: t("pluginNames.jsonparser") })}</CardTitle>
				<CardDescription>{t("placeholderConfig.jsonparserDescription")}</CardDescription>
				<Button onClick={() => navigate({ to: "/workspace/plugins" })}>{t("placeholderConfig.pluginCta")}</Button>
			</CardContent>
		</Card>
	);
}