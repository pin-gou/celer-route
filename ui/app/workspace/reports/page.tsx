import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Link } from "@tanstack/react-router";
import { ArrowRight, ChartColumn, FileBarChart, Scale } from "lucide-react";
import { useTranslation } from "react-i18next";

const SECTIONS = [
	{
		to: "/workspace/reports/standard-prices" as const,
		icon: Scale,
		titleKey: "standardPricesTitle" as const,
		descKey: "standardPricesDesc" as const,
	},
	{
		to: "/workspace/reports/gateway-delta" as const,
		icon: ChartColumn,
		titleKey: "gatewayDeltaTitle" as const,
		descKey: "gatewayDeltaDesc" as const,
	},
];

export default function ReportsLanding() {
	const { t } = useTranslation("reports");

	return (
		<div className="mx-auto max-w-5xl space-y-6 p-8" data-testid="reports-landing">
			<header>
				<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold">
					<FileBarChart className="h-6 w-6" />
					{t("title")}
				</h1>
				<p className="text-muted-foreground mt-1 text-sm">{t("subtitle")}</p>
			</header>

			<div className="grid grid-cols-1 gap-4 md:grid-cols-2">
				{SECTIONS.map((section) => {
					const Icon = section.icon;
					return (
						<Card key={section.to}>
							<CardHeader>
								<CardTitle className="flex items-center gap-2 text-base">
									<Icon className="h-4 w-4" />
									{t(section.titleKey)}
								</CardTitle>
								<CardDescription>{t(section.descKey)}</CardDescription>
							</CardHeader>
							<CardContent>
								<Button asChild variant="outline">
									<Link to={section.to} data-testid={`reports-go-${section.to.split("/").pop()}`}>
										{t("open")} <ArrowRight className="ml-1 h-3 w-3" />
									</Link>
								</Button>
							</CardContent>
						</Card>
					);
				})}
			</div>
		</div>
	);
}