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
import { ArrowUpRight, ExternalLink, Trash2 } from "lucide-react";
import { parseAsString, useQueryState } from "nuqs";
import { toast } from "sonner";
import { useEffect, useState } from "react";
import ConfirmDeleteProviderDialog from "@/app/workspace/providers/dialogs/confirmDeleteProviderDialog";
import { AdvancedTab } from "./views/AdvancedTab";
import { CooldownTab } from "./views/CooldownTab";
import { KeysTab } from "./views/KeysTab";
import { ModelsTab } from "./views/ModelsTab";
import { NetworkTab } from "./views/NetworkTab";
import { OverviewTab } from "./views/OverviewTab";

const TAB_IDS = ["overview", "keys", "models", "cooldown", "network", "advanced"] as const;
type TabId = (typeof TAB_IDS)[number];

// Map legacy `?editing=<section>` values emitted by the previous OverviewTab
// inline-edit UI onto the new top-level tabs. Editing now happens inside the
// destination tab, so we only need to point the user at the right place.
const LEGACY_EDITING_TO_TAB: Record<string, TabId> = {
	network: "network",
	proxy: "network",
	performance: "advanced",
	governance: "advanced",
	"beta-headers": "advanced",
	"openai-config": "advanced",
	debugging: "advanced",
	"cooldown-policy": "cooldown",
	"api-structure": "advanced",
};

// Map the previous top-level `governance` tab (OpenAI-only) onto the new
// Advanced tab where the GovernanceFormFragment now lives.
const LEGACY_TAB_TO_TAB: Record<string, TabId> = {
	governance: "advanced",
};

const isTabId = (value: string): value is TabId => (TAB_IDS as readonly string[]).includes(value);

export default function ProviderDetailPage() {
	const { id } = useParams({ from: "/workspace/providers/$id" });
	const navigate = useNavigate();
	const { t } = useTranslation("providers");
	const { data: provider, isLoading } = useGetProviderQuery(id);
	const [tabParam, setTab] = useQueryState("tab", parseAsString.withDefault("overview"));
	const [editingParam, setEditingParam] = useQueryState("editing", parseAsString);
	const [showDeleteDialog, setShowDeleteDialog] = useState(false);
	const hasDeleteAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Delete);
	const [deleteProvider] = useDeleteProviderMutation();

	const tabs: { id: TabId; label: string }[] = [
		{ id: "overview", label: t("providers2.tabs.overview") },
		{ id: "keys", label: t("providers2.tabs.keys") },
		{ id: "models", label: t("providers2.tabs.models") },
		{ id: "cooldown", label: t("providers2.tabs.cooldown") },
		{ id: "network", label: t("providers2.tabs.network") },
		{ id: "advanced", label: t("providers2.tabs.advanced") },
	];

	let activeTab: TabId = "overview";
	if (isTabId(tabParam)) {
		activeTab = tabParam;
	} else if (LEGACY_TAB_TO_TAB[tabParam]) {
		activeTab = LEGACY_TAB_TO_TAB[tabParam];
	}

	const redirectTarget = editingParam ? LEGACY_EDITING_TO_TAB[editingParam] : undefined;

	useEffect(() => {
		if (redirectTarget && redirectTarget !== activeTab) {
			setTab(redirectTarget);
			setEditingParam(null);
		}
	}, [redirectTarget, activeTab, setTab, setEditingParam]);

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

	const linkProvider = (isCustom ? provider.custom_provider_config?.base_provider_type : provider.name) as ProviderName | undefined;
	const websiteUrl = linkProvider ? ProviderWebsites[linkProvider] : undefined;
	const apiKeyUrl = linkProvider ? ProviderApiKeyUrls[linkProvider] : undefined;
	const showWebsiteLink = !!websiteUrl;
	const showApiKeyLink = !!apiKeyUrl && provider.is_key_less !== true && isKeyRequiredByProvider[linkProvider as ProviderName] !== false;

	const handleTabChange = (value: string) => {
		if (isTabId(value)) {
			setTab(value);
		}
	};

	const handleDelete = () => {
		deleteProvider(id)
			.unwrap()
			.then(() => {
				navigate({ to: "/workspace/providers" });
			})
			.catch((err) => {
				toast.error(t("providers2.toast.failedToDeleteProvider"), {
					description: getErrorMessage(err),
				});
			});
	};

	const navigateToTab = (tabId: string) => {
		if (isTabId(tabId)) {
			setTab(tabId);
		}
	};

	return (
		<div className="mx-auto flex h-full w-full max-w-7xl flex-col gap-6 p-6">
			<div data-testid="providers2-detail-breadcrumb" className="text-muted-foreground flex items-center gap-2 text-sm">
				<button
					data-testid="providers2-breadcrumb-list-link"
					className="hover:text-foreground underline underline-offset-2 transition-colors"
					onClick={() => navigate({ to: "/workspace/providers" })}
				>
					{t("providers2.breadcrumb")}
				</button>
				<span>/</span>
				<span className="text-foreground font-medium">{label}</span>
			</div>

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
									className="inline-flex items-center gap-1 text-lg font-semibold hover:underline"
								>
									{label}
									<ArrowUpRight className="h-3.5 w-3.5 opacity-60" />
								</a>
							) : (
								<span className="text-lg font-semibold">{label}</span>
							)}
							{provider.keys_count != null && provider.keys_count > 0 ? (
								provider.provider_status === "active" ? (
									<Badge variant="outline" className="border-green-500 text-xs text-green-600">
										● {t("providers2.detail.active")}
									</Badge>
								) : (
									<Badge variant="outline" className="border-red-500 text-xs text-red-600">
										● {provider.provider_status}
									</Badge>
								)
							) : null}
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
					<Button
						variant="outline"
						size="sm"
						data-testid="providers2-detail-logs-btn"
						onClick={() => navigate({ to: "/workspace/logs", search: { providers: [id] } })}
						className="gap-1 text-xs"
					>
						<ExternalLink className="h-3 w-3" />
						{t("providers2.detail.viewLogs")}
					</Button>
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
				</div>
			</div>

			<Tabs value={activeTab} onValueChange={handleTabChange} className="flex-1">
				<TabsList>
					{tabs.map((tab) => (
						<TabsTrigger key={tab.id} value={tab.id} data-testid={`providers2-tab-${tab.id}`}>
							{tab.label}
						</TabsTrigger>
					))}
				</TabsList>

				<TabsContent value="overview" className="mt-4" data-testid="providers2-tab-content-overview">
					<OverviewTab provider={provider} onNavigateTab={navigateToTab} />
				</TabsContent>
				<TabsContent value="keys" className="mt-4" data-testid="providers2-tab-content-keys">
					<KeysTab provider={provider} />
				</TabsContent>
				<TabsContent value="models" className="mt-4" data-testid="providers2-tab-content-models">
					<ModelsTab provider={provider} />
				</TabsContent>
				<TabsContent value="cooldown" className="mt-4" data-testid="providers2-tab-content-cooldown">
					<CooldownTab provider={provider} />
				</TabsContent>
				<TabsContent value="network" className="mt-4" data-testid="providers2-tab-content-network">
					<NetworkTab provider={provider} />
				</TabsContent>
				<TabsContent value="advanced" className="mt-4" data-testid="providers2-tab-content-advanced">
					<AdvancedTab provider={provider} />
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