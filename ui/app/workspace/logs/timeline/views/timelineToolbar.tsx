/**
 * @file Timeline toolbar — mode switching, zoom, refresh, reset, counts
 */

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { Minus, Plus, RefreshCw, RotateCcw } from "lucide-react";

export type TimelineMode = "follow" | "live" | "pan";

export interface TimelineToolbarProps {
	mode: TimelineMode;
	onModeChange: (mode: TimelineMode) => void;
	onRefresh: () => void;
	zoom: number;
	onZoomChange: (z: number) => void;
	onReset: () => void;
	visibleCount: number;
	totalCount: number;
	className?: string;
}

const MODE_DESCRIPTIONS: Record<TimelineMode, string> = {
	follow: "Scrolling with time — NOW line stays at 75%",
	live: "Live updates — NOW line snaps close to the right edge",
	pan: "Free exploration — drag to pan through the visible window",
};

export function TimelineToolbar({
	mode,
	onModeChange,
	onRefresh,
	zoom,
	onZoomChange,
	onReset,
	visibleCount,
	totalCount,
	className,
}: TimelineToolbarProps) {
	return (
		<div className="flex flex-col gap-1">
			<div data-testid="timeline-toolbar" className={cn("flex items-center gap-2", className)}>
				{/* Mode selector */}
				<div className="bg-muted/30 flex items-center rounded-md border p-0.5" data-testid="timeline-mode-selector">
					<button
						type="button"
						data-testid="timeline-mode-follow"
						className={cn(
							"rounded-sm px-2.5 py-1 text-xs font-medium transition-colors",
							mode === "follow" ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
						)}
						onClick={() => onModeChange("follow")}
					>
						Follow
					</button>
					<button
						type="button"
						data-testid="timeline-mode-live"
						className={cn(
							"rounded-sm px-2.5 py-1 text-xs font-medium transition-colors",
							mode === "live" ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
						)}
						onClick={() => onModeChange("live")}
					>
						Live
					</button>
					<button
						type="button"
						data-testid="timeline-mode-pan"
						className={cn(
							"rounded-sm px-2.5 py-1 text-xs font-medium transition-colors",
							mode === "pan" ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
						)}
						onClick={() => onModeChange("pan")}
					>
						Pan
					</button>
				</div>

				{/* Refresh */}
				<Button variant="ghost" size="sm" data-testid="timeline-refresh-button" onClick={onRefresh} className="gap-1.5">
					<RefreshCw className="h-3.5 w-3.5" />
					<span className="text-xs">Refresh</span>
				</Button>

				{/* Zoom controls */}
				<div className="bg-muted/30 flex items-center gap-1 rounded-md border p-0.5">
					<button
						type="button"
						data-testid="timeline-zoom-out"
						onClick={() => onZoomChange(zoom / 2)}
						className="text-muted-foreground hover:text-foreground rounded-sm px-1.5 py-1 transition-colors"
					>
						<Minus className="h-3 w-3" />
					</button>
					<span className="text-muted-foreground min-w-[36px] text-center font-mono text-[10px] tabular-nums">
						{zoom >= 1 ? `${zoom.toFixed(1)}x` : `${(zoom * 100).toFixed(0)}%`}
					</span>
					<button
						type="button"
						data-testid="timeline-zoom-in"
						onClick={() => onZoomChange(zoom * 2)}
						className="text-muted-foreground hover:text-foreground rounded-sm px-1.5 py-1 transition-colors"
					>
						<Plus className="h-3 w-3" />
					</button>
				</div>

				{/* Reset */}
				<Button variant="ghost" size="sm" data-testid="timeline-reset-button" onClick={onReset} className="gap-1.5">
					<RotateCcw className="h-3.5 w-3.5" />
					<span className="text-xs">Reset</span>
				</Button>

				{/* Spacer */}
				<div className="flex-1" />

				{/* Visible / total count */}
				<span className="text-muted-foreground font-mono text-[10px] tabular-nums" data-testid="timeline-count">
					{visibleCount} visible / {totalCount} total
				</span>
			</div>

			{/* Mode description */}
			<div className="text-muted-foreground text-[10px] italic" data-testid="timeline-mode-description">
				{MODE_DESCRIPTIONS[mode]}
			</div>
		</div>
	);
}