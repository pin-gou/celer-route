import { ModelProvider } from "@/lib/types/config";

interface UsageTabProps {
	provider: ModelProvider;
}

export function UsageTab({ provider }: UsageTabProps) {
	const todayRequests = (provider as any).today_requests ?? 0;
	const todayErrors = (provider as any).today_errors ?? 0;
	const uptime = (provider as any).uptime ?? 1;
	const avgLatency = (provider as any).avg_latency_ms ?? 0;

	return (
		<div data-testid="providers2-usage-tab" className="rounded-lg border p-6">
			<h3 className="mb-4 text-sm font-medium">Usage</h3>
			<div className="grid grid-cols-2 gap-4 md:grid-cols-4">
				<div className="bg-muted/50 rounded-md p-4">
					<div className="text-muted-foreground text-xs">Today Requests</div>
					<div className="mt-1 text-2xl font-semibold">{todayRequests.toLocaleString()}</div>
				</div>
				<div className="bg-muted/50 rounded-md p-4">
					<div className="text-muted-foreground text-xs">Today Errors</div>
					<div className="mt-1 text-2xl font-semibold">{todayErrors.toLocaleString()}</div>
				</div>
				<div className="bg-muted/50 rounded-md p-4">
					<div className="text-muted-foreground text-xs">Uptime (24h)</div>
					<div className="mt-1 text-2xl font-semibold">{(uptime * 100).toFixed(1)}%</div>
				</div>
				<div className="bg-muted/50 rounded-md p-4">
					<div className="text-muted-foreground text-xs">Avg Latency</div>
					<div className="mt-1 text-2xl font-semibold">{avgLatency}ms</div>
				</div>
			</div>
		</div>
	);
}