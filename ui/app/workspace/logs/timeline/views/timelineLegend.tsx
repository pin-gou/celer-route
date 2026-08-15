/**
 * @file Timeline legend — bar color = status, tick style = time interval
 */

import { cn } from "@/lib/utils";

export interface TimelineLegendProps {
	className?: string;
}

const legendItems = [
	{ label: "Success", color: "bg-green-500" },
	{ label: "Error", color: "bg-red-500" },
	{ label: "Processing", color: "bg-blue-400" },
	{ label: "Other", color: "bg-gray-400" },
];

export function TimelineLegend({ className }: TimelineLegendProps) {
	return (
		<div data-testid="timeline-legend" className={cn("flex items-center gap-3", className)}>
			{legendItems.map((item) => (
				<div key={item.label} className="flex items-center gap-1.5" data-testid={`timeline-legend-${item.label.toLowerCase()}`}>
					<div className={cn("h-2.5 w-2.5 rounded-sm", item.color)} />
					<span className="text-muted-foreground text-[11px]">{item.label}</span>
				</div>
			))}
			<div className="text-muted-foreground flex items-center gap-1.5" data-testid="timeline-legend-ticks">
				<div className="h-px w-3 border-t border-dashed border-slate-400/60" />
				<span className="text-[11px]">minute</span>
				<div className="ml-1 h-px w-3 border-t border-slate-400/70" />
				<span className="text-[11px]">hour</span>
				<div className="ml-1 w-3 border-t-2 border-indigo-400/60" />
				<span className="text-[11px]">day</span>
			</div>
		</div>
	);
}