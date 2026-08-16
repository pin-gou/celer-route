import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import { cn } from "@/lib/utils";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { PlayIcon, Trash2 } from "lucide-react";

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
	keys_enabled: boolean;
	custom_provider_config?: { base_provider_type?: string } | null;
}

export interface ProviderCardProps {
	provider: ProviderCardProvider;
	onToggle: () => void;
	onQuickTest: () => void;
	onDelete: () => void;
}

export function ProviderCard({ provider, onToggle, onQuickTest, onDelete }: ProviderCardProps) {
	const navigate = useNavigate();
	const { t } = useTranslation("providers");
	const isCustom = !!provider.custom_provider_config;
	const healthStatus = provider.keys_health_status;

	const handleCardClick = () => {
		navigate({ to: "/workspace/providers/$id", params: { id: provider.name } });
	};

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
		if (diffHours < 1) return t("providers2.card.justNow");
		if (diffHours < 24) return t("providers2.card.hoursAgo", { count: diffHours });
		return t("providers2.card.daysAgo", { count: Math.floor(diffHours / 24) });
	};

	return (
		<div
			data-testid={`providers2-card-${provider.name}`}
			className={cn(
				"flex flex-col gap-3 rounded-lg border p-4 transition-colors cursor-pointer",
				provider.provider_status === "error"
					? "border-red-200 bg-red-50/30 dark:border-red-900/30 dark:bg-red-950/10"
					: "hover:bg-accent/50",
			)}
			onClick={handleCardClick}
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
								{t("providers2.card.custom")}
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
				<span data-testid="providers2-card-keys-count">{t("providers2.card.keys", { count: provider.keys_count })}</span>
				<span data-testid="providers2-card-models-count">{t("providers2.card.models", { count: provider.models_count })}</span>
				<span data-testid="providers2-card-today-requests">{t("providers2.card.requests", { count: provider.today_requests })}</span>
				<span data-testid="providers2-card-today-errors">{t("providers2.card.errors", { count: provider.today_errors })}</span>
			</div>

			{/* Actions row */}
			<div className="flex items-center gap-2">
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="outline"
							size="sm"
							data-testid="providers2-card-quick-test"
							onClick={(e) => {
								e.stopPropagation();
								onQuickTest();
							}}
							className="h-7 gap-1 text-xs"
						>
							<PlayIcon className="h-3 w-3" />
							{t("providers2.card.quickTest")}
						</Button>
					</TooltipTrigger>
					<TooltipContent>{t("providers2.card.quickTestTooltip")}</TooltipContent>
				</Tooltip>
				<div className="ml-auto flex items-center gap-1">
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="ghost"
								size="sm"
								data-testid="providers2-card-delete"
								onClick={(e) => {
									e.stopPropagation();
									onDelete();
								}}
								className="text-muted-foreground h-7 w-7 p-0 hover:text-red-500"
							>
								<Trash2 className="h-3.5 w-3.5" />
							</Button>
						</TooltipTrigger>
						<TooltipContent>{t("providers2.card.deleteTooltip")}</TooltipContent>
					</Tooltip>
					<Switch
						data-testid="providers2-card-toggle"
						checked={provider.keys_enabled}
						onCheckedChange={() => onToggle()}
						onClick={(e) => e.stopPropagation()}
						className="h-5 w-9"
					/>
				</div>
			</div>

			{/* Last error row — below actions so cards are consistent height */}
			{provider.last_error_at && (
				<div className="text-xs text-red-500">
					<span data-testid="providers2-card-last-error">
						{t("providers2.card.lastError", { time: formatLastError(provider.last_error_at) })}
					</span>
				</div>
			)}
		</div>
	);
}