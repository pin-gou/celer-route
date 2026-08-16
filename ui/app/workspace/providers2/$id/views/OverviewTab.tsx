import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ModelProvider } from "@/lib/types/config";
import { NetworkFormFragment } from "@/app/workspace/providers/fragments/networkFormFragment";
import { ProxyFormFragment } from "@/app/workspace/providers/fragments/proxyFormFragment";
import { PerformanceFormFragment } from "@/app/workspace/providers/fragments/performanceFormFragment";
import { BetaHeadersFormFragment } from "@/app/workspace/providers/fragments/betaHeadersFormFragment";
import { OpenAIConfigFormFragment } from "@/app/workspace/providers/fragments/openaiConfigFormFragment";
import { GovernanceFormFragment } from "@/app/workspace/providers/fragments/governanceFormFragment";
import { DebuggingFormFragment } from "@/app/workspace/providers/fragments/debuggingFormFragment";
import { ApiStructureFormFragment } from "@/app/workspace/providers/fragments/apiStructureFormFragment";

export interface OverviewTabProps {
	provider: ModelProvider;
	onSave: () => void;
}

const ANTHROPIC_FAMILY_PROVIDERS = ["anthropic", "vertex", "bedrock", "bedrock_mantle", "azure"];

type EditableSection = "network" | "proxy" | "performance" | "governance" | "beta-headers" | "openai-config" | "debugging" | "api-structure" | null;

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
	const { t } = useTranslation("providers");
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
						{t("providers2.overview.edit")}
					</button>
				)}
			</div>
			{children}
		</div>
	);
}

export function OverviewTab({ provider }: OverviewTabProps) {
	const { t } = useTranslation("providers");
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
					<h3 className="text-sm font-medium">{t("providers2.overview.network")}</h3>
					<button
						data-testid="providers2-overview-network-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						{t("providers2.overview.cancel")}
					</button>
				</div>
				<NetworkFormFragment provider={provider} onCancel={handleCancelEdit} />
			</div>
		);
	}

	if (editingSection === "proxy") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">{t("providers2.overview.proxy")}</h3>
					<button
						data-testid="providers2-overview-proxy-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						{t("providers2.overview.cancel")}
					</button>
				</div>
				<ProxyFormFragment provider={provider} onCancel={handleCancelEdit} />
			</div>
		);
	}

	if (editingSection === "performance") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">{t("providers2.overview.performance")}</h3>
					<button
						data-testid="providers2-overview-performance-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						{t("providers2.overview.cancel")}
					</button>
				</div>
				<PerformanceFormFragment provider={provider} onCancel={handleCancelEdit} />
			</div>
		);
	}

	if (editingSection === "beta-headers") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">{t("providers2.overview.betaHeaders")}</h3>
					<button
						data-testid="providers2-overview-beta-headers-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						{t("providers2.overview.cancel")}
					</button>
				</div>
				<BetaHeadersFormFragment provider={provider} onCancel={handleCancelEdit} />
			</div>
		);
	}

	if (editingSection === "openai-config") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">{t("providers2.overview.openaiConfig")}</h3>
					<button
						data-testid="providers2-overview-openai-config-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						{t("providers2.overview.cancel")}
					</button>
				</div>
				<OpenAIConfigFormFragment provider={provider} onCancel={handleCancelEdit} />
			</div>
		);
	}

	if (editingSection === "governance") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">{t("providers2.overview.governance")}</h3>
					<button
						data-testid="providers2-overview-governance-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						{t("providers2.overview.cancel")}
					</button>
				</div>
				<GovernanceFormFragment provider={provider} onCancel={handleCancelEdit} />
			</div>
		);
	}

	if (editingSection === "debugging") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">{t("providers2.overview.debugging")}</h3>
					<button
						data-testid="providers2-overview-debugging-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						{t("providers2.overview.cancel")}
					</button>
				</div>
				<DebuggingFormFragment provider={provider} onCancel={handleCancelEdit} />
			</div>
		);
	}

	if (editingSection === "api-structure") {
		return (
			<div className="rounded-lg border p-4">
				<div className="mb-3 flex items-center justify-between">
					<h3 className="text-sm font-medium">{t("providers2.overview.apiStructure")}</h3>
					<button
						data-testid="providers2-overview-api-structure-cancel"
						className="text-muted-foreground text-xs underline"
						onClick={handleCancelEdit}
					>
						{t("providers2.overview.cancel")}
					</button>
				</div>
				<ApiStructureFormFragment provider={provider} onCancel={handleCancelEdit} />
			</div>
		);
	}

	const todayRequests = provider.today_requests ?? 0;
	const todayErrors = provider.today_errors ?? 0;
	const uptime = provider.uptime ?? 1;
	const avgLatency = provider.avg_latency_ms ?? 0;

	return (
		<div className="space-y-6">
			{/* Usage stats */}
			<div data-testid="providers2-usage-section" className="rounded-lg border p-6">
				<h3 className="mb-4 text-sm font-medium">{t("providers2.usageTab.title")}</h3>
				<div className="grid grid-cols-2 gap-4 md:grid-cols-4">
					<div className="bg-muted/50 rounded-md p-4">
						<div className="text-muted-foreground text-xs">{t("providers2.usageTab.todayRequests")}</div>
						<div className="mt-1 text-2xl font-semibold">{todayRequests.toLocaleString()}</div>
					</div>
					<div className="bg-muted/50 rounded-md p-4">
						<div className="text-muted-foreground text-xs">{t("providers2.usageTab.todayErrors")}</div>
						<div className="mt-1 text-2xl font-semibold">{todayErrors.toLocaleString()}</div>
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

			<div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
				<Section
					testId="providers2-overview-network"
					title={t("providers2.overview.network")}
					editTestId="providers2-overview-network-edit"
					onEdit={() => setEditingSection("network")}
				>
					<div className="text-muted-foreground space-y-1 text-xs">
						<div className="flex justify-between">
							<span>{t("providers2.overview.baseUrl")}</span>
							<span className="font-mono">{nw?.base_url || "—"}</span>
						</div>
						<div className="flex justify-between">
							<span>{t("providers2.overview.maxConnections")}</span>
							<span className="font-mono">{nw?.max_conns_per_host ?? "—"}</span>
						</div>
						<div className="flex justify-between">
							<span>{t("providers2.overview.timeout")}</span>
							<span className="font-mono">{nw?.default_request_timeout_in_seconds ?? "—"}s</span>
						</div>
					</div>
				</Section>

				<Section
					testId="providers2-overview-proxy"
					title={t("providers2.overview.proxy")}
					editTestId="providers2-overview-proxy-edit"
					onEdit={() => setEditingSection("proxy")}
				>
					<div className="text-muted-foreground space-y-1 text-xs">
						<div className="flex justify-between">
							<span>{t("providers2.overview.type")}</span>
							<span className="font-mono">{!proxy || proxy.type === "none" ? "none" : proxy.type}</span>
						</div>
						<div className="flex justify-between">
							<span>{t("providers2.overview.url")}</span>
							<span className="font-mono">{proxy?.url?.value || "—"}</span>
						</div>
					</div>
				</Section>

				<Section
					testId="providers2-overview-performance"
					title={t("providers2.overview.performance")}
					editTestId="providers2-overview-performance-edit"
					onEdit={() => setEditingSection("performance")}
				>
					<div className="text-muted-foreground space-y-1 text-xs">
						<div className="flex justify-between">
							<span>{t("providers2.overview.concurrency")}</span>
							<span className="font-mono">{perf?.concurrency ?? "—"}</span>
						</div>
						<div className="flex justify-between">
							<span>{t("providers2.overview.bufferSize")}</span>
							<span className="font-mono">{perf?.buffer_size ?? "—"}</span>
						</div>
					</div>
				</Section>

				<Section
					testId="providers2-overview-governance"
					title={t("providers2.overview.governance")}
					editTestId="providers2-overview-governance-edit"
					onEdit={() => setEditingSection("governance")}
				>
					<p className="text-muted-foreground text-xs">{t("providers2.overview.governanceDescription", { provider: String(provider.name) })}</p>
				</Section>

				<Section
					testId="providers2-overview-beta-headers"
					title={t("providers2.overview.betaHeaders")}
					editTestId="providers2-overview-beta-headers-edit"
					onEdit={() => setEditingSection("beta-headers")}
				>
					{isAnthropicFamily ? (
						<p className="text-muted-foreground text-xs">{t("providers2.overview.betaHeadersAnthropicDescription")}</p>
					) : (
						<p className="text-muted-foreground text-xs">{t("providers2.overview.betaHeadersGenericDescription")}</p>
					)}
				</Section>

				<Section
					testId="providers2-overview-openai-config"
					title={t("providers2.overview.openaiConfig")}
					editTestId="providers2-overview-openai-config-edit"
					onEdit={() => setEditingSection("openai-config")}
				>
					{isOpenAI ? (
						<p className="text-muted-foreground text-xs">{t("providers2.overview.openaiConfigDescription")}</p>
					) : (
						<p className="text-muted-foreground text-xs">{t("providers2.overview.openaiConfigGenericDescription")}</p>
					)}
				</Section>

				<Section
					testId="providers2-overview-debugging"
					title={t("providers2.overview.debugging")}
					editTestId="providers2-overview-debugging-edit"
					onEdit={() => setEditingSection("debugging")}
				>
					<div className="text-muted-foreground space-y-1 text-xs">
						<div className="flex justify-between">
							<span>{t("providers2.overview.debuggingSendBackRawRequest")}</span>
							<span className="font-mono">{provider.send_back_raw_request ? "ON" : "OFF"}</span>
						</div>
						<div className="flex justify-between">
							<span>{t("providers2.overview.debuggingSendBackRawResponse")}</span>
							<span className="font-mono">{provider.send_back_raw_response ? "ON" : "OFF"}</span>
						</div>
						<div className="flex justify-between">
							<span>{t("providers2.overview.debuggingStoreRawRequestResponse")}</span>
							<span className="font-mono">{provider.store_raw_request_response ? "ON" : "OFF"}</span>
						</div>
					</div>
				</Section>

				{provider.custom_provider_config && (
					<Section
						testId="providers2-overview-api-structure"
						title={t("providers2.overview.apiStructure")}
						editTestId="providers2-overview-api-structure-edit"
						onEdit={() => setEditingSection("api-structure")}
					>
						<p className="text-muted-foreground text-xs">{t("providers2.overview.apiStructureDescription")}</p>
					</Section>
				)}
			</div>
		</div>
	);
}