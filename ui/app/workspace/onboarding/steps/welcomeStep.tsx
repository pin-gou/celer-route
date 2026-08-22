import { Minimize2, Network, ShieldCheck, TrendingUp } from "lucide-react";
import { useTranslation } from "react-i18next";

const FEATURES = [
	{ icon: Network, key: "unified" as const },
	{ icon: Minimize2, key: "rtk" as const },
	{ icon: ShieldCheck, key: "governance" as const },
	{ icon: TrendingUp, key: "observability" as const },
];

export default function WelcomeStep() {
	const { t } = useTranslation("onboarding");
	return (
		<div className="space-y-6 text-center">
			<div>
				<h2 className="text-2xl font-semibold tracking-tight">{t("title")}</h2>
				<p className="text-muted-foreground mt-2 text-sm">{t("welcomeDesc")}</p>
			</div>
			<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
				{FEATURES.map(({ icon: Icon, key }) => (
					<div
						key={key}
						className="bg-muted/30 flex flex-col items-start gap-2 rounded-md border p-4 text-left"
						data-testid={`onboarding-feature-${key}`}
					>
						<div className="bg-primary/10 text-primary flex h-8 w-8 items-center justify-center rounded-md">
							<Icon className="h-4 w-4" />
						</div>
						<div className="text-sm font-medium">{t(`feature.${key}`)}</div>
						<div className="text-muted-foreground text-xs">{t(`feature.${key}Desc`)}</div>
					</div>
				))}
			</div>
		</div>
	);
}