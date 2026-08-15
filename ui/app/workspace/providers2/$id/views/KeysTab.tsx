import ModelProviderKeysTableView from "@/app/workspace/providers/views/modelProviderKeysTableView";
import { ModelProvider } from "@/lib/types/config";
import { useBatchUpdateProviderKeysMutation } from "@/lib/store/apis/providersApi";
import { toast } from "sonner";

interface KeysTabProps {
	provider: ModelProvider;
}

export function KeysTab({ provider }: KeysTabProps) {
	const [batchUpdateKeys] = useBatchUpdateProviderKeysMutation();

	const handleBatchToggle = async (enabled: boolean) => {
		try {
			const result = await batchUpdateKeys({
				provider: provider.name,
				key_ids: [],
				enabled,
			}).unwrap();
			toast.success(`Updated ${result.updated} keys`);
		} catch (err) {
			toast.error("Failed to batch update keys");
		}
	};

	return (
		<div data-testid="providers2-keys-tab">
			<ModelProviderKeysTableView provider={provider} />
		</div>
	);
}