/**
 * @file Timeline toolbar — mode switching, time window, refresh
 */

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { Pause, Play, RefreshCw, ZoomIn, ZoomOut } from "lucide-react";

export type TimelineMode = "follow" | "live" | "pan";

export interface TimelineToolbarProps {
	mode: TimelineMode;
	onModeChange: (mode: TimelineMode) => void;
	onRefresh: () => void;
	isLive: boolean;
	onLiveToggle: (live: boolean) => void;
	className?: string;
}

export function TimelineToolbar({
	mode,
	onModeChange,
	onRefresh,
	isLive,
	onLiveToggle,
	className,
}: TimelineToolbarProps) {
	return (
		<div
			data-testid="timeline-toolbar"
			className={cn("flex items-center gap-2", className)}
		>
			{/* Mode selector */}
			<div className="flex items-center rounded-md border bg-muted/30 p-0.5" data-testid="timeline-mode-selector">
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

			{/* Live toggle */}
			<Button
				variant="outline"
				size="sm"
				data-testid="timeline-live-toggle"
				onClick={() => onLiveToggle(!isLive)}
				className="gap-1.5"
			>
				{isLive ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
				<span className="text-xs">{isLive ? "Pause" : "Live"}</span>
			</Button>

			{/* Refresh */}
			<Button
				variant="ghost"
				size="sm"
				data-testid="timeline-refresh-button"
				onClick={onRefresh}
				className="gap-1.5"
			>
				<RefreshCw className="h-3.5 w-3.5" />
				<span className="text-xs">Refresh</span>
			</Button>

			{/* Spacer */}
			<div className="flex-1" />
		</div>
	);
}