import FullPageLoader from "@/components/fullPageLoader";
import { ProviderNames } from "@/lib/constants/logs";
import { getErrorMessage, useCreateProviderMutation } from "@/lib/store";
import {
	useBatchUpdateProviderKeysMutation,
	useLazyGetProviderKeysQuery,
	useRefreshProviderModelsMutation,
} from "@/lib/store/apis/providersApi";
import { type ModelProvider, ModelProviderName } from "@/lib/types/config";
import { useRbac, RbacOperation, RbacResource } from "@/lib/rbac";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import AddCustomProviderSheet from "@/app/workspace/providers/dialogs/addNewCustomProviderSheet";
import { AddProviderDropdown } from "@/app/workspace/providers/views/addProviderDropdown";
import ConfirmDeleteProviderDialog from "@/app/workspace/providers/dialogs/confirmDeleteProviderDialog";
import { ProviderFamilyGroup } from "./views/ProviderFamilyGroup";
import { ProviderFilters, type FilterState } from "./views/ProviderFilters";
import { useProviders2Data } from "./views/useProviders2Data";

export default function ProvidersPage() {
	const { t } = useTranslation("providers");
	const { groupedProviders, isLoading, error, refetch } = useProviders2Data();
	const [batchUpdateKeys] = useBatchUpdateProviderKeysMutation();
	const [getProviderKeys] = useLazyGetProviderKeysQuery();
	const [refreshProviderModels] = useRefreshProviderModelsMutation();
	const [showCustomProviderSheet, setShowCustomProviderSheet] = useState(false);
	const [createProvider] = useCreateProviderMutation();
	const hasCreateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Create);

	const [keysEnabledMap, setKeysEnabledMap] = useState<Record<string, boolean>>({});

	const existingInSidebar = useMemo(() => new Set(groupedProviders.flatMap((g) => g.providers.map((p) => p.name))), [groupedProviders]);

	const knownProviders = useMemo(() => ProviderNames.map((name) => ({ name })), []);

	const providerEnabledLookup = useMemo(() => {
		const lookup: Record<string, boolean> = {};
		for (const group of groupedProviders) {
			for (const p of group.providers) {
				lookup[p.name] = p.keys_enabled;
			}
		}
		return lookup;
	}, [groupedProviders]);

	const [deleteProviderName, setDeleteProviderName] = useState<string | null>(null);
	const [showDeleteDialog, setShowDeleteDialog] = useState(false);

	const handleDeleteRequest = (providerName: string) => {
		setDeleteProviderName(providerName);
		setShowDeleteDialog(true);
	};

	const handleDeleteConfirm = () => {
		setShowDeleteDialog(false);
		setDeleteProviderName(null);
		refetch();
	};

	const [filters, setFilters] = useState<FilterState>({
		search: "",
		health: "all",
	});

	const filteredGroups = groupedProviders
		.map((group) => {
			const filtered = group.providers
				.filter((p) => {
					// Search filter
					if (filters.search && !p.name.toLowerCase().includes(filters.search.toLowerCase())) {
						return false;
					}
					// Health filter
					if (filters.health === "active" && p.provider_status !== "active") return false;
					if (filters.health === "error" && p.provider_status !== "error") return false;
					return true;
				})
				.map((p) => ({ ...p, keys_enabled: keysEnabledMap[p.name] ?? p.keys_enabled }));
			return { ...group, providers: filtered };
		})
		.filter((g) => g.providers.length > 0);

	const handleToggle = async (providerName: string) => {
		const previouslyEnabled = keysEnabledMap[providerName] ?? providerEnabledLookup[providerName] ?? true;
		const nextEnabled = !previouslyEnabled;
		setKeysEnabledMap((prev) => ({ ...prev, [providerName]: nextEnabled }));
		try {
			const keys = await getProviderKeys(providerName).unwrap();
			const keyIds = keys.map((k) => k.id);
			if (keyIds.length === 0) {
				setKeysEnabledMap((prev) => ({ ...prev, [providerName]: previouslyEnabled }));
				toast.info(t("toast.noKeysToToggle", { provider: providerName }));
				return;
			}
			const result = await batchUpdateKeys({
				provider: providerName,
				key_ids: keyIds,
				enabled: nextEnabled,
			}).unwrap();
			toast.success(
				nextEnabled
					? t("toast.keysEnabled", { count: result.updated, provider: providerName })
					: t("toast.keysDisabled", { count: result.updated, provider: providerName }),
			);
			refetch();
		} catch (err: any) {
			setKeysEnabledMap((prev) => ({ ...prev, [providerName]: previouslyEnabled }));
			toast.error(t("toast.failedToggleKeys", { provider: providerName }));
		}
	};

	const handleQuickTest = async (providerName: string) => {
		try {
			await refreshProviderModels(providerName).unwrap();
			toast.success(t("toast.modelDiscoveryTriggered", { provider: providerName }));
		} catch (err: any) {
			if (err?.status === 409) {
				toast.info(t("toast.modelDiscoveryRunning", { provider: providerName }));
			} else {
				toast.error(t("toast.quickTestFailed", { provider: providerName }));
			}
		}
	};

	const handleSelectKnownProvider = async (name: string) => {
		try {
			await createProvider({ provider: name as ModelProviderName }).unwrap();
		} catch (err: any) {
			if (err?.status === 409) return;
			toast.error(t("toast.failedToAddProvider"), {
				description: getErrorMessage(err),
			});
		}
	};

	const handleAddCustomProvider = () => {
		setShowCustomProviderSheet(true);
	};

	if (isLoading) {
		return <FullPageLoader />;
	}

	return (
		<div className="mx-auto flex h-full w-full max-w-7xl flex-col gap-6 p-6">
			{/* Page heading */}
			<div data-testid="providers2-page-heading" className="sr-only">
				{t("providers2.pageTitle")}
			</div>
			{/* Toolbar */}
			<div className="flex items-center justify-between gap-4">
				<div className="flex-1">
					<ProviderFilters filters={filters} onChange={setFilters} />
				</div>
				<div className="flex items-center gap-2">
					<AddProviderDropdown
						variant="toolbar"
						disabled={!hasCreateAccess}
						existingInSidebar={existingInSidebar}
						knownProviders={knownProviders}
						onSelectKnownProvider={handleSelectKnownProvider}
						onAddCustomProvider={handleAddCustomProvider}
					/>
				</div>
			</div>

			{/* Grouped provider cards */}
			<div className="flex-1 overflow-y-auto">
				{filteredGroups.length === 0 ? (
					<div className="text-muted-foreground flex h-40 items-center justify-center text-sm">
						{error ? t("failedToLoad") : t("noMatchFilters")}
					</div>
				) : (
					filteredGroups.map((group) => (
						<ProviderFamilyGroup
							key={group.family}
							familyName={group.family}
							providers={group.providers}
							onToggle={handleToggle}
							onQuickTest={handleQuickTest}
							onDelete={handleDeleteRequest}
						/>
					))
				)}
			</div>

			<AddCustomProviderSheet
				show={showCustomProviderSheet}
				onClose={() => setShowCustomProviderSheet(false)}
				onSave={() => {
					refetch();
					setShowCustomProviderSheet(false);
				}}
			/>

			{deleteProviderName && (
				<ConfirmDeleteProviderDialog
					show={showDeleteDialog}
					onCancel={() => {
						setShowDeleteDialog(false);
						setDeleteProviderName(null);
					}}
					onDelete={handleDeleteConfirm}
					provider={{ name: deleteProviderName } as ModelProvider}
				/>
			)}
		</div>
	);
}