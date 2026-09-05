import { CooldownPolicyFormFragment } from "@/app/workspace/providers/fragments/cooldownPolicyFormFragment";
import { ModelProvider } from "@/lib/types/config";

interface CooldownTabProps {
	provider: ModelProvider;
}

export function CooldownTab({ provider }: CooldownTabProps) {
	return (
		<div data-testid="providers2-cooldown-tab" className="rounded-lg border p-4">
			<CooldownPolicyFormFragment provider={provider} />
		</div>
	);
}