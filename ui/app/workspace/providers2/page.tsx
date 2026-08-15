import FullPageLoader from "@/components/fullPageLoader";
import { useRefreshProviderModelsMutation } from "@/lib/store/apis/providersApi";
import { useRbac, RbacOperation, RbacResource } from "@/lib/rbac";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import TryLegacyViewButton from "./dialogs/TryLegacyViewButton";
import { ProviderFamilyGroup } from "./views/ProviderFamilyGroup";
import { ProviderFilters, type FilterState } from "./views/ProviderFilters";
import { useProviders2Data } from "./views/useProviders2Data";

export default function Providers2Page() {
	const navigate = useNavigate();
	const { groupedProviders, isLoading, error, refetch } = useProviders2Data();
	const [refreshProviderModels] = useRefreshProviderModelsMutation();
	const hasUpdateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);

	const [filters, setFilters] = useState<FilterState>({
		search: "",
		health: "all",
	});

	const filteredGroups = groupedProviders
		.map((group) => {
			const filtered = group.providers.filter((p) => {
				// Search filter
				if (filters.search && !p.name.toLowerCase().includes(filters.search.toLowerCase())) {
					return false;
				}
				// Health filter
				if (filters.health === "active" && p.provider_status !== "active") return false;
				if (filters.health === "error" && p.provider_status !== "error") return false;
				return true;
			});
			return { ...group, providers: filtered };
		})
		.filter((g) => g.providers.length > 0);

	const handleToggle = async (providerName: string) => {
		// Toggle all keys for the provider — uses batch endpoint
		try {
			await refreshProviderModels(providerName).unwrap();
			toast.success(`Keys refreshed for ${providerName}`);
		} catch (err: any) {
			toast.error(`Failed to toggle keys for ${providerName}`);
		}
	};

	const handleQuickTest = async (providerName: string) => {
		try {
			await refreshProviderModels(providerName).unwrap();
			toast.success(`Model discovery triggered for ${providerName}`);
		} catch (err: any) {
			if (err?.status === 409) {
				toast.info(`Model discovery already running for ${providerName}`);
			} else {
				toast.error(`Quick test failed for ${providerName}`);
			}
		}
	};

	if (isLoading) {
		return <FullPageLoader />;
	}

	return (
		<div className="mx-auto flex h-full w-full max-w-7xl flex-col gap-6 p-6">
			{/* Toolbar */}
			<div className="flex items-center justify-between gap-4">
				<div className="flex-1">
					<ProviderFilters filters={filters} onChange={setFilters} />
				</div>
				<div className="flex items-center gap-2">
					<TryLegacyViewButton currentProvider={filteredGroups[0]?.providers[0]?.name} />
				</div>
			</div>

			{/* Grouped provider cards */}
			<div className="flex-1 overflow-y-auto">
				{filteredGroups.length === 0 ? (
					<div className="text-muted-foreground flex h-40 items-center justify-center text-sm">
						{error ? "Failed to load providers" : "No providers match your filters"}
					</div>
				) : (
					filteredGroups.map((group) => (
						<ProviderFamilyGroup
							key={group.family}
							familyName={group.family}
							providers={group.providers}
							onToggle={handleToggle}
							onQuickTest={handleQuickTest}
						/>
					))
				)}
			</div>
		</div>
	);
}