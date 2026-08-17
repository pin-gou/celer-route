/**
 * @file Timeline legend — bar fill color = status, border color = tokens/s speed
 */

import { cn } from "@/lib/utils";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

export interface TimelineLegendProps {
	className?: string;
}

export function TimelineLegend({ className }: TimelineLegendProps) {
	const { t } = useTranslation("logs");

	const legendItems = useMemo(
		() => [
			{ label: t("timeline.legend.success"), key: "success", color: "bg-green-500" },
			{ label: t("timeline.legend.error"), key: "error", color: "bg-red-500" },
			{ label: t("timeline.legend.processing"), key: "processing", color: "bg-blue-400" },
			{ label: t("timeline.legend.other"), key: "other", color: "bg-gray-400" },
		],
		[t],
	);

	const borderItems = useMemo(
		() => [
			{ label: t("timeline.legend.tpsVerySlow"), key: "tps-very-slow", border: "border-red-500" },
			{ label: t("timeline.legend.tpsSlow"), key: "tps-slow", border: "border-amber-500" },
			{ label: t("timeline.legend.tpsMedium"), key: "tps-medium", border: "border-blue-500" },
			{ label: t("timeline.legend.tpsFast"), key: "tps-fast", border: "border-green-600" },
		],
		[t],
	);

	return (
		<div data-testid="timeline-legend" className={cn("flex items-center gap-4", className)}>
			<div className="flex items-center gap-3">
				<span className="text-muted-foreground text-[11px]">{t("timeline.legend.fillStatus")}:</span>
				{legendItems.map((item) => (
					<div key={item.key} className="flex items-center gap-1.5" data-testid={`timeline-legend-${item.key}`}>
						<div className={cn("h-2.5 w-2.5 rounded-sm", item.color)} />
						<span className="text-muted-foreground text-[11px]">{item.label}</span>
					</div>
				))}
			</div>
			<div className="flex items-center gap-3">
				<span className="text-muted-foreground text-[11px]">{t("timeline.legend.borderTps")}:</span>
				{borderItems.map((item) => (
					<div key={item.key} className="flex items-center gap-1.5" data-testid={`timeline-legend-border-${item.key}`}>
						<div className={cn("h-2.5 w-2.5 rounded-sm border bg-transparent", item.border)} />
						<span className="text-muted-foreground text-[11px]">{item.label}</span>
					</div>
				))}
			</div>
		</div>
	);
}