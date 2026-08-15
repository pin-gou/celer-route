import ProviderGovernanceTable from "@/app/workspace/providers/views/providerGovernanceTable";
import { ModelProvider } from "@/lib/types/config";

interface GovernanceTabProps {
	provider: ModelProvider;
}

export function GovernanceTab({ provider }: GovernanceTabProps) {
	return (
		<div data-testid="providers2-governance-tab">
			<ProviderGovernanceTable provider={provider} />
		</div>
	);
}