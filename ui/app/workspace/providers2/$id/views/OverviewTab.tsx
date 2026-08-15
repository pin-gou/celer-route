export interface OverviewTabProps {
	provider: any;
	onSave: () => void;
}

const ANTHROPIC_FAMILY_PROVIDERS = ["anthropic", "vertex", "bedrock", "bedrock_mantle", "azure"];

function Section({
	testId,
	title,
	editTestId,
	children,
}: {
	testId: string;
	title: string;
	editTestId?: string;
	children: React.ReactNode;
}) {
	return (
		<div data-testid={testId} className="rounded-lg border p-4">
			<div className="mb-3 flex items-center justify-between">
				<h3 className="text-sm font-medium">{title}</h3>
				{editTestId && (
					<button data-testid={editTestId} className="text-primary text-xs underline">
						Edit
					</button>
				)}
			</div>
			{children}
		</div>
	);
}

export function OverviewTab({ provider }: OverviewTabProps) {
	const isAnthropicFamily = ANTHROPIC_FAMILY_PROVIDERS.includes(String(provider.name).toLowerCase());
	const isOpenAI = String(provider.name) === "openai";

	const nw = provider.network_config || {};
	const perf = provider.concurrency_and_buffer_size || {};
	const proxy = provider.proxy_config || {};

	return (
		<div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
			<Section testId="providers2-overview-network" title="Network" editTestId="providers2-overview-network-edit">
				<div className="text-muted-foreground space-y-1 text-xs">
					<div className="flex justify-between">
						<span>Base URL</span>
						<span className="font-mono">{nw.base_url || "—"}</span>
					</div>
					<div className="flex justify-between">
						<span>Max Connections</span>
						<span className="font-mono">{nw.max_conns_per_host ?? "—"}</span>
					</div>
					<div className="flex justify-between">
						<span>Timeout</span>
						<span className="font-mono">{nw.default_request_timeout_in_seconds ?? "—"}s</span>
					</div>
				</div>
			</Section>

			<Section testId="providers2-overview-proxy" title="Proxy" editTestId="providers2-overview-proxy-edit">
				<div className="text-muted-foreground space-y-1 text-xs">
					<div className="flex justify-between">
						<span>Type</span>
						<span className="font-mono">{proxy.proxy_type || proxy.type || "none"}</span>
					</div>
					<div className="flex justify-between">
						<span>URL</span>
						<span className="font-mono">{proxy.proxy_url || proxy.url?.value || "—"}</span>
					</div>
				</div>
			</Section>

			<Section testId="providers2-overview-performance" title="Performance" editTestId="providers2-overview-performance-edit">
				<div className="text-muted-foreground space-y-1 text-xs">
					<div className="flex justify-between">
						<span>Concurrency</span>
						<span className="font-mono">{perf.concurrency ?? "—"}</span>
					</div>
					<div className="flex justify-between">
						<span>Buffer Size</span>
						<span className="font-mono">{perf.buffer_size ?? "—"}</span>
					</div>
				</div>
			</Section>

			<Section testId="providers2-overview-governance" title="Governance">
				<p className="text-muted-foreground text-xs">Governance configuration for {String(provider.name)}.</p>
			</Section>

			<Section testId="providers2-overview-beta-headers" title="Beta Headers" editTestId="providers2-overview-beta-headers-edit">
				{isAnthropicFamily ? (
					<p className="text-muted-foreground text-xs">Beta Header overrides for Anthropic-family providers.</p>
				) : (
					<p className="text-muted-foreground text-xs">Beta Headers are only available for Anthropic-family providers.</p>
				)}
			</Section>

			<Section testId="providers2-overview-openai-config" title="OpenAI Config" editTestId="providers2-overview-openai-config-edit">
				{isOpenAI ? (
					<p className="text-muted-foreground text-xs">OpenAI-specific configuration options.</p>
				) : (
					<p className="text-muted-foreground text-xs">OpenAI Config is only available for the openai provider.</p>
				)}
			</Section>
		</div>
	);
}