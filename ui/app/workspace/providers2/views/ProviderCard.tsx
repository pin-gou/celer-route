import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import { cn } from "@/lib/utils";
import { PlayIcon } from "lucide-react";

export interface ProviderCardProvider {
	name: string;
	provider_status: string;
	keys_count: number;
	models_count: number;
	today_requests: number;
	today_errors: number;
	last_error_at: string | null;
	uptime: number;
	avg_latency_ms: number;
	keys_health_status: string;
	custom_provider_config?: { base_provider_type?: string } | null;
}

export interface ProviderCardProps {
	provider: ProviderCardProvider;
	onToggle: () => void;
	onQuickTest: () => void;
}

export function ProviderCard({ provider, onToggle, onQuickTest }: ProviderCardProps) {
	const isCustom = !!provider.custom_provider_config;
	const healthStatus = provider.keys_health_status;

	const getHealthColor = (status: string) => {
		switch (status) {
			case "healthy":
				return "bg-green-500";
			case "degraded":
				return "bg-yellow-500";
			default:
				return "bg-gray-400";
		}
	};

	const formatLastError = (dateStr: string | null) => {
		if (!dateStr) return null;
		const date = new Date(dateStr);
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffHours = Math.floor(diffMs / 3600000);
		if (diffHours < 1) return "just now";
		if (diffHours < 24) return `${diffHours}h ago`;
		return `${Math.floor(diffHours / 24)}d ago`;
	};

	return (
		<div
			data-testid={`providers2-card-${provider.name}`}
			className={cn(
				"flex flex-col gap-3 rounded-lg border p-4 transition-colors",
				provider.provider_status === "error"
					? "border-red-200 bg-red-50/30 dark:border-red-900/30 dark:bg-red-950/10"
					: "hover:bg-accent/50",
			)}
		>
			{/* Header: Icon + Name + Health Badge + CUSTOM tag */}
			<div className="flex items-center gap-3">
				<div data-testid={`providers2-card-icon-${provider.name}`} className="flex h-8 w-8 items-center justify-center">
					<RenderProviderIcon
						provider={(isCustom ? provider.custom_provider_config?.base_provider_type : provider.name) as ProviderIconType}
						size="sm"
						className="h-6 w-6"
					/>
				</div>
				<div className="flex-1">
					<div className="flex items-center gap-2">
						<span className="text-sm font-medium">{provider.name}</span>
						{isCustom && (
							<Badge variant="secondary" className="text-muted-foreground px-1.5 py-0.5 text-[10px] font-bold">
								CUSTOM
							</Badge>
						)}
					</div>
				</div>
				<div
					data-testid="providers2-card-health-badge"
					data-health-status={healthStatus}
					className={cn("flex h-2.5 w-2.5 rounded-full", getHealthColor(healthStatus))}
					title={`Health: ${healthStatus}`}
				/>
			</div>

			{/* Stats row */}
			<div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
				<span data-testid="providers2-card-keys-count">{provider.keys_count} keys</span>
				<span data-testid="providers2-card-models-count">{provider.models_count} models</span>
				<span data-testid="providers2-card-today-requests">{provider.today_requests} reqs</span>
				<span data-testid="providers2-card-today-errors">{provider.today_errors} err</span>
				{provider.last_error_at && (
					<span data-testid="providers2-card-last-error" className="text-red-500">
						last err: {formatLastError(provider.last_error_at)}
					</span>
				)}
			</div>

			{/* Actions row */}
			<div className="flex items-center gap-2">
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="outline"
							size="sm"
							data-testid="providers2-card-quick-test"
							onClick={onQuickTest}
							className="h-7 gap-1 text-xs"
						>
							<PlayIcon className="h-3 w-3" />
							Test
						</Button>
					</TooltipTrigger>
					<TooltipContent>Quick test: POST /refresh-models</TooltipContent>
				</Tooltip>
				<div className="ml-auto">
					<Switch data-testid="providers2-card-toggle" checked={true} onCheckedChange={() => onToggle()} className="h-5 w-9" />
				</div>
			</div>
		</div>
	);
}