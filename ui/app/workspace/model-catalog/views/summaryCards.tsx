import { Card, CardContent } from "@/components/ui/card";
import { useTranslation } from "react-i18next";

export interface SummaryCardsProps {
	totalProviders: number;
	totalModels: number;
	totalRequests1h: number;
}

export function SummaryCards({ totalProviders, totalModels, totalRequests1h }: SummaryCardsProps) {
	const { t } = useTranslation("model-catalog");
	const cards = [
		{ label: t("summary.totalProviders"), value: totalProviders.toLocaleString() },
		{ label: t("summary.totalModels"), value: totalModels.toLocaleString() },
		{ label: t("summary.totalRequests1h"), value: totalRequests1h.toLocaleString() },
	];

	return (
		<div className="grid grid-cols-3 gap-4">
			{cards.map((card) => (
				<Card key={card.label} className="py-4 shadow-none">
					<CardContent className="px-4">
						<p className="text-muted-foreground text-xs">{card.label}</p>
						<p className="mt-1 text-xl font-semibold">{card.value}</p>
					</CardContent>
				</Card>
			))}
		</div>
	);
}