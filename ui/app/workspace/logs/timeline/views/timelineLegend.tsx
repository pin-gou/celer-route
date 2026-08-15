/**
 * @file Timeline legend — bar color = status, tick style = time interval
 */

import { cn } from "@/lib/utils";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

export interface TimelineLegendProps {
	className?: string;
}

export function TimelineLegend({ className }: TimelineLegendProps) {
	const { t } = useTranslation("logs");

	const legendItems = useMemo(() => [
		{ label: t("timeline.legend.success"), key: "success", color: "bg-green-500" },
		{ label: t("timeline.legend.error"), key: "error", color: "bg-red-500" },
		{ label: t("timeline.legend.processing"), key: "processing", color: "bg-blue-400" },
		{ label: t("timeline.legend.other"), key: "other", color: "bg-gray-400" },
	], [t]);

	return (
		<div data-testid="timeline-legend" className={cn("flex items-center gap-3", className)}>
			{legendItems.map((item) => (
				<div key={item.key} className="flex items-center gap-1.5" data-testid={`timeline-legend-${item.key}`}>
					<div className={cn("h-2.5 w-2.5 rounded-sm", item.color)} />
					<span className="text-muted-foreground text-[11px]">{item.label}</span>
				</div>
			))}
			<div className="text-muted-foreground flex items-center gap-1.5" data-testid="timeline-legend-ticks">
				<div className="h-px w-3 border-t border-dashed border-slate-400/60" />
				<span className="text-[11px]">{t("timeline.legend.minute")}</span>
				<div className="ml-1 h-px w-3 border-t border-slate-400/70" />
				<span className="text-[11px]">{t("timeline.legend.hour")}</span>
				<div className="ml-1 w-3 border-t-2 border-indigo-400/60" />
				<span className="text-[11px]">{t("timeline.legend.day")}</span>
			</div>
		</div>
	);
}