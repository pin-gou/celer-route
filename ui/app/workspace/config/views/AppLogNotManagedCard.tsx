import { useTranslation } from "react-i18next";

export default function AppLogNotManagedCard() {
	const { t } = useTranslation("config");
	return (
		<section className="space-y-4">
			<h3 className="text-muted-foreground px-1 text-xs font-semibold tracking-wide uppercase">{t("logging.storageTitle")}</h3>
			<div className="bg-muted/40 rounded-sm border px-4 py-3 text-sm" data-testid="logging-app-not-managed">
				<p className="font-medium">{t("logging.appNotManagedTitle")}</p>
				<p className="text-muted-foreground mt-1">{t("logging.appNotManagedDesc")}</p>
				<p className="text-muted-foreground mt-3 text-xs">{t("logging.appNotManagedCmds")}</p>
				<ul className="mt-2 space-y-2 text-xs">
					<li>
						<span className="font-medium">{t("logging.appNotManagedSystemd")}:</span>{" "}
						<code className="bg-muted rounded px-1 font-mono">{t("logging.appNotManagedSystemdHint")}</code>
					</li>
					<li>
						<span className="font-medium">{t("logging.appNotManagedDocker")}:</span>{" "}
						<span className="text-muted-foreground">{t("logging.appNotManagedDockerHint")}</span>
					</li>
					<li>
						<span className="font-medium">{t("logging.appNotManagedK8s")}:</span>{" "}
						<span className="text-muted-foreground">{t("logging.appNotManagedK8sHint")}</span>
					</li>
				</ul>
			</div>
		</section>
	);
}