import FullPageLoader from "@/components/fullPageLoader";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import { ProviderLabels } from "@/lib/constants/logs";
import { useGetProviderQuery } from "@/lib/store/apis/providersApi";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useState } from "react";
import OpenLegacyConfigSheetButton from "./dialogs/OpenLegacyConfigSheetButton";
import { GovernanceTab } from "./views/GovernanceTab";
import { KeysTab } from "./views/KeysTab";
import { LogsTab } from "./views/LogsTab";
import { ModelsTab } from "./views/ModelsTab";
import { OverviewTab } from "./views/OverviewTab";
import { UsageTab } from "./views/UsageTab";

export default function ProviderDetailPage() {
	const { id } = useParams({ from: "/workspace/providers2/$id" });
	const navigate = useNavigate();
	const { data: provider, isLoading } = useGetProviderQuery(id);
	const [activeTab, setActiveTab] = useState("overview");

	if (isLoading) {
		return <FullPageLoader />;
	}

	if (!provider) {
		return (
			<div className="flex h-full items-center justify-center">
				<div className="text-muted-foreground text-sm">Provider not found</div>
			</div>
		);
	}

	const isCustom = !provider.name || !Object.keys(ProviderLabels).includes(provider.name);
	const label = isCustom ? provider.name : (ProviderLabels[provider.name as keyof typeof ProviderLabels] ?? provider.name);

	const tabs = [
		{ id: "overview", label: "Overview" },
		{ id: "keys", label: "Keys" },
		{ id: "models", label: "Models" },
		{ id: "usage", label: "Usage" },
		...(provider.name === "openai" ? [{ id: "governance", label: "Governance" }] : []),
		{ id: "logs", label: "Logs" },
	];

	return (
		<div className="mx-auto flex h-full w-full max-w-7xl flex-col gap-6 p-6">
			{/* Breadcrumb + Header */}
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-3">
					<div className="flex h-10 w-10 items-center justify-center">
						<RenderProviderIcon
							provider={(isCustom ? provider.custom_provider_config?.base_provider_type : provider.name) as ProviderIconType}
							size="md"
							className="h-8 w-8"
						/>
					</div>
					<div>
						<div className="flex items-center gap-2">
							<span className="text-lg font-semibold">{label}</span>
							{provider.provider_status === "active" ? (
								<Badge variant="outline" className="border-green-500 text-xs text-green-600">
									● active
								</Badge>
							) : (
								<Badge variant="outline" className="border-red-500 text-xs text-red-600">
									● {provider.provider_status}
								</Badge>
							)}
						</div>
					</div>
				</div>
				<div className="flex items-center gap-2">
					<OpenLegacyConfigSheetButton provider={provider} />
				</div>
			</div>

			{/* Tabs */}
			<Tabs value={activeTab} onValueChange={setActiveTab} className="flex-1">
				<TabsList>
					{tabs.map((tab) => (
						<TabsTrigger key={tab.id} value={tab.id} data-testid={`providers2-tab-${tab.id}`}>
							{tab.label}
						</TabsTrigger>
					))}
				</TabsList>

				<TabsContent value="overview" className="mt-4">
					<OverviewTab provider={provider} onSave={() => {}} />
				</TabsContent>
				<TabsContent value="keys" className="mt-4">
					<KeysTab provider={provider} />
				</TabsContent>
				<TabsContent value="models" className="mt-4">
					<ModelsTab provider={provider} />
				</TabsContent>
				<TabsContent value="usage" className="mt-4">
					<UsageTab provider={provider} />
				</TabsContent>
				<TabsContent value="governance" className="mt-4">
					<GovernanceTab provider={provider} />
				</TabsContent>
				<TabsContent value="logs" className="mt-4">
					<LogsTab provider={provider} />
				</TabsContent>
			</Tabs>
		</div>
	);
}