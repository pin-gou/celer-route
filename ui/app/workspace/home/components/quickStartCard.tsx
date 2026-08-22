import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Link } from "@tanstack/react-router";
import { LineChart, Network, Plug } from "lucide-react";
import { useTranslation } from "react-i18next";

const STEPS = [
	{ num: 1, href: "/workspace/providers", icon: Plug, labelKey: "step1Title", descKey: "step1Desc" },
	{ num: 2, href: "/workspace/routing-rules", icon: Network, labelKey: "step2Title", descKey: "step2Desc" },
	{ num: 3, href: "/workspace/providers/$id", icon: Plug, labelKey: "step3Title", descKey: "step3Desc" },
	{ num: 4, href: "/workspace/dashboard", icon: LineChart, labelKey: "step4Title", descKey: "step4Desc" },
];

export default function QuickStartCard() {
	const { t } = useTranslation("common");
	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid="home-quickstart">
			<CardHeader className="flex flex-row items-start justify-between gap-2 border-b px-6 py-3">
				<div className="space-y-1">
					<CardTitle className="text-sm font-semibold">{t("home.quickStart.title")}</CardTitle>
					<p className="text-muted-foreground text-xs">{t("home.quickStart.subtitle")}</p>
				</div>
			</CardHeader>
			<CardContent className="px-6 py-4">
				<ol className="grid grid-cols-1 gap-3 md:grid-cols-2">
					{STEPS.map((step) => {
						const Icon = step.icon;
						return (
							<li key={step.num}>
								<Link
									to={step.href}
									className="group bg-muted/20 hover:border-muted-foreground/40 hover:bg-muted/40 flex h-full items-start gap-3 rounded-md border p-4"
									data-testid={`home-quickstart-step-${step.num}`}
								>
									<div className="bg-primary/10 text-primary flex h-8 w-8 shrink-0 items-center justify-center rounded-md">
										<Icon className="h-4 w-4" />
									</div>
									<div className="min-w-0 space-y-1">
										<div className="text-sm font-semibold">{t(`home.quickStart.${step.labelKey}`)}</div>
										<div className="text-muted-foreground text-xs">{t(`home.quickStart.${step.descKey}`)}</div>
									</div>
								</Link>
							</li>
						);
					})}
				</ol>
			</CardContent>
		</Card>
	);
}