import { useState } from "react";
import { ModelProvider } from "@/lib/types/config";
import { NetworkFormFragment } from "@/app/workspace/providers/fragments/networkFormFragment";
import { ProxyFormFragment } from "@/app/workspace/providers/fragments/proxyFormFragment";
import { PerformanceFormFragment } from "@/app/workspace/providers/fragments/performanceFormFragment";
import { BetaHeadersFormFragment } from "@/app/workspace/providers/fragments/betaHeadersFormFragment";
import { OpenAIConfigFormFragment } from "@/app/workspace/providers/fragments/openaiConfigFormFragment";
import { GovernanceFormFragment } from "@/app/workspace/providers/fragments/governanceFormFragment";

export interface OverviewTabProps {
	provider: ModelProvider;
	onSave: () => void;
}

const ANTHROPIC_FAMILY_PROVIDERS = ["anthropic", "vertex", "bedrock", "bedrock_mantle", "azure"];

type EditableSection = "network" | "proxy" | "performance" | "governance" | "beta-headers" | "openai-config" | null;

function Section({
	testId,
	title,
	editTestId,
	onEdit,
	children,
}: {
	testId: string;
	title: string;
	editTestId?: string;
	onEdit?: () => void;
	children: React.ReactNode;
}) {
	return (
		<div data-testid={testId} className="rounded-lg border p-4">
			<div className="mb-3 flex items-center justify-between">
				<h3 className="text-sm font-medium">{title}</h3>
				{editTestId && onEdit && (
					<button
						data-testid={editTestId}
						className="text-primary text-xs underline"
						onClick={onEdit}
					>
						Edit
					</button>
				)}
			</div>
			{children}
		</div>
	);
}

export function OverviewTab({ provider }: OverviewTabProps) {
	const [editingSection, setEditingSection] = useState<EditableSection>(null);
	const isAnthropicFamily = ANTHROPIC_FAMILY_PROVIDERS.includes(String(provider.name).toLowerCase());
	const isOpenAI = String(provider.name) === "openai";

	const nw = provider.network_config;
	const perf = provider.concurrency_and_buffer_size;
	const proxy = provider.proxy_config;

	const handleCancelEdit = () => setEditingSection(null);

	if (editingSection === "network") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">Network</h3>
					<button
						data-testid="providers2-overview-network-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						Cancel
					</button>
				</div>
				<NetworkFormFragment provider={provider} />
			</div>
		);
	}

	if (editingSection === "proxy") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">Proxy</h3>
					<button
						data-testid="providers2-overview-proxy-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						Cancel
					</button>
				</div>
				<ProxyFormFragment provider={provider} />
			</div>
		);
	}

	if (editingSection === "performance") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">Performance</h3>
					<button
						data-testid="providers2-overview-performance-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						Cancel
					</button>
				</div>
				<PerformanceFormFragment provider={provider} />
			</div>
		);
	}

	if (editingSection === "beta-headers") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">Beta Headers</h3>
					<button
						data-testid="providers2-overview-beta-headers-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						Cancel
					</button>
				</div>
				<BetaHeadersFormFragment provider={provider} />
			</div>
		);
	}

	if (editingSection === "openai-config") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">OpenAI Config</h3>
					<button
						data-testid="providers2-overview-openai-config-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						Cancel
					</button>
				</div>
				<OpenAIConfigFormFragment provider={provider} />
			</div>
		);
	}

	if (editingSection === "governance") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">Governance</h3>
					<button
						data-testid="providers2-overview-governance-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						Cancel
					</button>
				</div>
				<GovernanceFormFragment provider={provider} />
			</div>
		);
	}

	return (
		<div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
			<Section
				testId="providers2-overview-network"
				title="Network"
				editTestId="providers2-overview-network-edit"
				onEdit={() => setEditingSection("network")}
			>
				<div className="text-muted-foreground space-y-1 text-xs">
					<div className="flex justify-between">
						<span>Base URL</span>
						<span className="font-mono">{nw?.base_url || "—"}</span>
					</div>
					<div className="flex justify-between">
						<span>Max Connections</span>
						<span className="font-mono">{nw?.max_conns_per_host ?? "—"}</span>
					</div>
					<div className="flex justify-between">
						<span>Timeout</span>
						<span className="font-mono">{nw?.default_request_timeout_in_seconds ?? "—"}s</span>
					</div>
				</div>
			</Section>

			<Section
				testId="providers2-overview-proxy"
				title="Proxy"
				editTestId="providers2-overview-proxy-edit"
				onEdit={() => setEditingSection("proxy")}
			>
				<div className="text-muted-foreground space-y-1 text-xs">
					<div className="flex justify-between">
						<span>Type</span>
						<span className="font-mono">{!proxy || proxy.type === "none" ? "none" : proxy.type}</span>
					</div>
					<div className="flex justify-between">
						<span>URL</span>
						<span className="font-mono">{proxy?.url?.value || "—"}</span>
					</div>
				</div>
			</Section>

			<Section
				testId="providers2-overview-performance"
				title="Performance"
				editTestId="providers2-overview-performance-edit"
				onEdit={() => setEditingSection("performance")}
			>
				<div className="text-muted-foreground space-y-1 text-xs">
					<div className="flex justify-between">
						<span>Concurrency</span>
						<span className="font-mono">{perf?.concurrency ?? "—"}</span>
					</div>
					<div className="flex justify-between">
						<span>Buffer Size</span>
						<span className="font-mono">{perf?.buffer_size ?? "—"}</span>
					</div>
				</div>
			</Section>

			<Section
				testId="providers2-overview-governance"
				title="Governance"
				editTestId="providers2-overview-governance-edit"
				onEdit={() => setEditingSection("governance")}
			>
				<p className="text-muted-foreground text-xs">Governance configuration for {String(provider.name)}.</p>
			</Section>

			<Section
				testId="providers2-overview-beta-headers"
				title="Beta Headers"
				editTestId="providers2-overview-beta-headers-edit"
				onEdit={() => setEditingSection("beta-headers")}
			>
				{isAnthropicFamily ? (
					<p className="text-muted-foreground text-xs">Beta Header overrides for Anthropic-family providers.</p>
				) : (
					<p className="text-muted-foreground text-xs">Beta Headers are only available for Anthropic-family providers.</p>
				)}
			</Section>

			<Section
				testId="providers2-overview-openai-config"
				title="OpenAI Config"
				editTestId="providers2-overview-openai-config-edit"
				onEdit={() => setEditingSection("openai-config")}
			>
				{isOpenAI ? (
					<p className="text-muted-foreground text-xs">OpenAI-specific configuration options.</p>
				) : (
					<p className="text-muted-foreground text-xs">OpenAI Config is only available for the openai provider.</p>
				)}
			</Section>
		</div>
	);
}