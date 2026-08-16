import { useTranslation } from "react-i18next";
import { ModelProvider } from "@/lib/types/config";

interface UsageTabProps {
	provider: ModelProvider;
}

export function UsageTab({ provider }: UsageTabProps) {
	const { t } = useTranslation("providers");
	const p = provider as ModelProvider;
	const hourlyRequests = p.hourly_requests ?? 0;
	const hourlyErrors = p.hourly_errors ?? 0;
	const uptime = p.uptime ?? 1;
	const avgLatency = p.avg_latency_ms ?? 0;

	return (
		<div data-testid="providers2-usage-tab" className="rounded-lg border p-6">
			<h3 className="mb-4 text-sm font-medium">{t("providers2.usageTab.title")}</h3>
			<div className="grid grid-cols-2 gap-4 md:grid-cols-4">
				<div className="bg-muted/50 rounded-md p-4">
					<div className="text-muted-foreground text-xs">{t("providers2.usageTab.hourlyRequests")}</div>
					<div className="mt-1 text-2xl font-semibold">{hourlyRequests.toLocaleString()}</div>
				</div>
				<div className="bg-muted/50 rounded-md p-4">
					<div className="text-muted-foreground text-xs">{t("providers2.usageTab.hourlyErrors")}</div>
					<div className="mt-1 text-2xl font-semibold">{hourlyErrors.toLocaleString()}</div>
				</div>
				<div className="bg-muted/50 rounded-md p-4">
					<div className="text-muted-foreground text-xs">{t("providers2.usageTab.uptime")}</div>
					<div className="mt-1 text-2xl font-semibold">{(uptime * 100).toFixed(1)}%</div>
				</div>
				<div className="bg-muted/50 rounded-md p-4">
					<div className="text-muted-foreground text-xs">{t("providers2.usageTab.avgLatency")}</div>
					<div className="mt-1 text-2xl font-semibold">{avgLatency}ms</div>
				</div>
			</div>
		</div>
	);
}