import FullPageLoader from "@/components/fullPageLoader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import { ProviderLabels, ProviderName } from "@/lib/constants/logs";
import { ProviderApiKeyUrls, ProviderWebsites } from "@/lib/constants/config";
import { isKeyRequiredByProvider } from "@/lib/constants/config";
import { getErrorMessage, useDeleteProviderMutation } from "@/lib/store";
import { useGetProviderQuery } from "@/lib/store/apis/providersApi";
import { useRbac, RbacOperation, RbacResource } from "@/lib/rbac";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { ArrowUpRight, Trash2 } from "lucide-react";
import { parseAsString, useQueryState } from "nuqs";
import { toast } from "sonner";
import { useState } from "react";
import OpenLegacyConfigSheetButton from "./dialogs/OpenLegacyConfigSheetButton";
import ConfirmDeleteProviderDialog from "@/app/workspace/providers/dialogs/confirmDeleteProviderDialog";
import { GovernanceTab } from "./views/GovernanceTab";
import { KeysTab } from "./views/KeysTab";
import { LogsTab } from "./views/LogsTab";
import { ModelsTab } from "./views/ModelsTab";
import { OverviewTab } from "./views/OverviewTab";
import { UsageTab } from "./views/UsageTab";

export default function ProviderDetailPage() {
	const { id } = useParams({ from: "/workspace/providers2/$id" });
	const navigate = useNavigate();
	const { t } = useTranslation("providers");
	const { data: provider, isLoading } = useGetProviderQuery(id);
	const [tabParam, setTab] = useQueryState("tab", parseAsString.withDefault("overview"));
	const [showDeleteDialog, setShowDeleteDialog] = useState(false);
	const hasDeleteAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Delete);
	const [deleteProvider] = useDeleteProviderMutation();

	if (isLoading) {
		return <FullPageLoader />;
	}

	if (!provider) {
		return (
			<div className="flex h-full items-center justify-center">
				<div className="text-muted-foreground text-sm">{t("providers2.detail.providerNotFound")}</div>
			</div>
		);
	}

	const isCustom = !provider.name || !Object.keys(ProviderLabels).includes(provider.name);
	const label = isCustom ? provider.name : (ProviderLabels[provider.name as keyof typeof ProviderLabels] ?? provider.name);

	// Custom providers inherit the base provider type for the header links so a
	// sensova→openai-style custom provider still surfaces its provider's
	// website / API-key page. Keyless providers never get an API-key link.
	const linkProvider = (isCustom
		? provider.custom_provider_config?.base_provider_type
		: provider.name) as ProviderName | undefined;
	const websiteUrl = linkProvider ? ProviderWebsites[linkProvider] : undefined;
	const apiKeyUrl = linkProvider ? ProviderApiKeyUrls[linkProvider] : undefined;
	const keyRequired = linkProvider ? isKeyRequiredByProvider[linkProvider] : undefined;
	const showWebsiteLink = !!websiteUrl;
	const showApiKeyLink = !!apiKeyUrl && keyRequired !== false;

	const tabs = [
		{ id: "overview", label: t("providers2.tabs.overview") },
		{ id: "keys", label: t("providers2.tabs.keys") },
		{ id: "models", label: t("providers2.tabs.models") },
		{ id: "usage", label: t("providers2.tabs.usage") },
		...(provider.name === "openai" ? [{ id: "governance", label: t("providers2.tabs.governance") }] : []),
		{ id: "logs", label: t("providers2.tabs.logs") },
	];

	// The tab is persisted in the URL query param so a refresh keeps the user
	// on the selected tab. Fall back to overview if the URL holds an inactive
	// tab (e.g. "governance" for a non-OpenAI provider).
	const tabIds = tabs.map((t) => t.id);
	const activeTab = tabIds.includes(tabParam) ? tabParam : "overview";

	const handleTabChange = (value: string) => {
		setTab(value);
	};

	const handleLegacyView = () => {
		navigate({ to: "/workspace/providers", search: { provider: id } });
	};

	const handleDelete = () => {
		deleteProvider(id)
			.unwrap()
			.then(() => {
				navigate({ to: "/workspace/providers2" });
			})
			.catch((err) => {
				toast.error(t("providers2.toast.failedToDeleteProvider"), {
					description: getErrorMessage(err),
				});
			});
	};

	return (
		<div className="mx-auto flex h-full w-full max-w-7xl flex-col gap-6 p-6">
			{/* Breadcrumb */}
			<div data-testid="providers2-detail-breadcrumb" className="flex items-center gap-2 text-sm text-muted-foreground">
				<button
					data-testid="providers2-breadcrumb-list-link"
					className="hover:text-foreground underline underline-offset-2 transition-colors"
					onClick={() => navigate({ to: "/workspace/providers2" })}
				>
					{t("providers2.breadcrumb")}
				</button>
				<span>/</span>
				<span className="text-foreground font-medium">{label}</span>
			</div>

			{/* Header */}
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-3" data-testid="providers2-detail-heading">
					<div className="flex h-10 w-10 items-center justify-center">
						<RenderProviderIcon
							provider={(isCustom ? provider.custom_provider_config?.base_provider_type : provider.name) as ProviderIconType}
							size="md"
							className="h-8 w-8"
						/>
					</div>
					<div>
						<div className="flex items-center gap-2">
							{showWebsiteLink ? (
								<a
									href={websiteUrl}
									target="_blank"
									rel="noopener noreferrer"
									className="text-lg font-semibold hover:underline inline-flex items-center gap-1"
								>
									{label}
									<ArrowUpRight className="h-3.5 w-3.5 opacity-60" />
								</a>
							) : (
								<span className="text-lg font-semibold">{label}</span>
							)}
							{provider.provider_status === "active" ? (
								<Badge variant="outline" className="border-green-500 text-xs text-green-600">
									● {t("providers2.detail.active")}
								</Badge>
							) : (
								<Badge variant="outline" className="border-red-500 text-xs text-red-600">
									● {provider.provider_status}
								</Badge>
							)}
						</div>
						{showApiKeyLink && (
							<a
								href={apiKeyUrl}
								target="_blank"
								rel="noopener noreferrer"
								data-testid="providers2-detail-api-key-link"
								className="text-muted-foreground hover:text-primary mt-0.5 inline-flex items-center gap-1 text-xs underline underline-offset-2"
							>
								{t("providers2.detail.getApiKey")}
								<ArrowUpRight className="h-3 w-3" />
							</a>
						)}
					</div>
				</div>
				<div className="flex items-center gap-2">
					{hasDeleteAccess && (
						<Button
							variant="outline"
							size="sm"
							data-testid="providers2-detail-delete-btn"
							onClick={() => setShowDeleteDialog(true)}
							className="gap-1 text-xs text-red-500 hover:text-red-600"
						>
							<Trash2 className="h-3 w-3" />
							{t("providers2.detail.delete")}
						</Button>
					)}
					<Button
						variant="outline"
						size="sm"
						data-testid="providers2-legacy-view-btn"
						onClick={handleLegacyView}
						className="gap-1 text-xs"
					>
						{t("providers2.detail.openLegacyView")}
						<ArrowUpRight className="h-3 w-3" />
					</Button>
					<OpenLegacyConfigSheetButton provider={provider} />
				</div>
			</div>

			{/* Tabs */}
			<Tabs value={activeTab} onValueChange={handleTabChange} className="flex-1">
				<TabsList>
					{tabs.map((tab) => (
						<TabsTrigger key={tab.id} value={tab.id} data-testid={`providers2-tab-${tab.id}`}>
							{tab.label}
						</TabsTrigger>
					))}
				</TabsList>

				<TabsContent value="overview" className="mt-4" data-testid="providers2-tab-content-overview">
					<OverviewTab provider={provider} onSave={() => {}} />
				</TabsContent>
				<TabsContent value="keys" className="mt-4" data-testid="providers2-tab-content-keys">
					<KeysTab provider={provider} />
				</TabsContent>
				<TabsContent value="models" className="mt-4" data-testid="providers2-tab-content-models">
					<ModelsTab provider={provider} />
				</TabsContent>
				<TabsContent value="usage" className="mt-4" data-testid="providers2-tab-content-usage">
					<UsageTab provider={provider} />
				</TabsContent>
				<TabsContent value="governance" className="mt-4" data-testid="providers2-tab-content-governance">
					<GovernanceTab provider={provider} />
				</TabsContent>
				<TabsContent value="logs" className="mt-4" data-testid="providers2-tab-content-logs">
					<LogsTab provider={provider} />
				</TabsContent>
			</Tabs>

			<ConfirmDeleteProviderDialog
				show={showDeleteDialog}
				onCancel={() => setShowDeleteDialog(false)}
				onDelete={handleDelete}
				provider={provider}
			/>
		</div>
	);
}