import { Button } from "@/components/ui/button";
import { useGetModelsQuery, useRefreshProviderModelsMutation } from "@/lib/store/apis/providersApi";
import { ModelProvider } from "@/lib/types/config";
import { RefreshCwIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

interface ModelsTabProps {
	provider: ModelProvider;
}

export function ModelsTab({ provider }: ModelsTabProps) {
	const [showAll, setShowAll] = useState(false);
	const { data: modelsData, isLoading, refetch } = useGetModelsQuery({ provider: provider.name, limit: showAll ? 500 : 10 });
	const [refreshModels, { isLoading: isRefreshing }] = useRefreshProviderModelsMutation();

	const models = modelsData?.models ?? [];
	const total = modelsData?.total ?? 0;

	const handleSync = async () => {
		try {
			await refreshModels(provider.name).unwrap();
			toast.success(`Model refresh started for ${provider.name}`);
			refetch();
		} catch (err: any) {
			if (err?.status === 409) {
				toast.info(`Model refresh already running for ${provider.name}`);
			} else {
				toast.error(`Failed to refresh models for ${provider.name}`);
			}
		}
	};

	return (
		<div data-testid="providers2-models-tab" className="rounded-lg border p-6">
			<div className="mb-4 flex items-center justify-between">
				<div>
					<h3 className="text-sm font-medium">Models</h3>
					<p className="text-muted-foreground mt-1 text-xs">
						{total} model{total !== 1 ? "s" : ""} associated with {provider.name}
					</p>
				</div>
				<Button variant="outline" size="sm" data-testid="providers2-models-sync-btn" onClick={handleSync} disabled={isRefreshing} className="gap-1 text-xs">
					<RefreshCwIcon className={`h-3 w-3 ${isRefreshing ? "animate-spin" : ""}`} />
					{isRefreshing ? "Syncing..." : "Sync"}
				</Button>
			</div>

			{isLoading ? (
				<div className="text-muted-foreground flex h-20 items-center justify-center text-xs">
					Loading models...
				</div>
			) : models.length === 0 ? (
				<div className="text-muted-foreground flex h-20 items-center justify-center text-xs">
					No models found for {provider.name}. Click Sync to discover models.
				</div>
			) : (
				<div className="overflow-x-auto">
					<table className="w-full text-left text-xs">
						<thead>
							<tr className="text-muted-foreground border-b">
								<th className="pb-2 pr-4 font-medium">Model Name</th>
								<th className="pb-2 pr-4 font-medium">Provider</th>
								<th className="pb-2 font-medium">Status</th>
							</tr>
						</thead>
						<tbody>
							{models.map((model) => (
								<tr key={`${model.provider}-${model.name}`} className="border-b last:border-0" data-testid={`providers2-models-row-${model.name}`}>
									<td className="py-2 pr-4 font-mono text-xs">{model.name}</td>
									<td className="py-2 pr-4 text-xs">{model.provider}</td>
									<td className="py-2 text-xs">
										{model.is_deprecated ? (
											<span className="text-red-500">Deprecated</span>
										) : (
											<span className="text-green-600">Active</span>
										)}
									</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			)}

			{total > 10 && (
				<button
					data-testid="providers2-models-show-all"
					className="text-primary mt-3 text-xs underline"
					onClick={() => setShowAll(!showAll)}
				>
					{showAll ? "Show less" : `Show all ${total} models`}
				</button>
			)}
		</div>
	);
}