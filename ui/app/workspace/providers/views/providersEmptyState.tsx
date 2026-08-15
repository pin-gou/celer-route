import { Button } from "@/components/ui/button";
import { Server } from "lucide-react";

interface ProvidersEmptyStateProps {
	/** Dropdown (or button) for adding a provider; never greyed out */
	addProviderDropdown: React.ReactNode;
}

export function ProvidersEmptyState({ addProviderDropdown }: ProvidersEmptyStateProps) {
	return (
		<div className="flex min-h-[80vh] w-full flex-col items-center justify-center gap-4 py-16 text-center">
			<div className="text-muted-foreground">
				<Server className="h-[5.5rem] w-[5.5rem]" strokeWidth={1} />
			</div>
			<div className="flex flex-col gap-1">
				<h1 className="text-muted-foreground text-xl font-medium">Add a provider to start routing requests</h1>
				<div className="text-muted-foreground mx-auto mt-2 max-w-[600px] text-sm font-normal">
					Configure API keys for OpenAI, Anthropic, Bedrock, and other supported providers. Bifrost unifies them behind a single API.
				</div>
				<div className="mx-auto mt-6 flex flex-row flex-wrap items-center justify-center gap-2">
					{addProviderDropdown}
				</div>
			</div>
		</div>
	);
}