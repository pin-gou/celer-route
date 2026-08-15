/**
 * @file Timeline legend — bar color = status
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
		</div>
	);
}