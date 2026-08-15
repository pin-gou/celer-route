import { ModelProvider } from "@/lib/types/config";

interface ModelsTabProps {
	provider: ModelProvider;
}

export function ModelsTab({ provider }: ModelsTabProps) {
	return (
		<div data-testid="providers2-models-tab" className="rounded-lg border p-6">
			<h3 className="mb-4 text-sm font-medium">Models</h3>
			<p className="text-muted-foreground text-sm">
				Model catalog for {provider.name}. View and manage models associated with this provider.
			</p>
		</div>
	);
}