import { Button } from "@/components/ui/button";
import { Wallet } from "lucide-react";

interface ModelLimitsEmptyStateProps {
	onAddClick: () => void;
	canCreate?: boolean;
}

export function ModelLimitsEmptyState({ onAddClick, canCreate = true }: ModelLimitsEmptyStateProps) {
	return (
		<div className="flex min-h-[80vh] w-full flex-col items-center justify-center gap-4 py-16 text-center">
			<div className="text-muted-foreground">
				<Wallet className="h-[5.5rem] w-[5.5rem]" strokeWidth={1} />
			</div>
			<div className="flex flex-col gap-1">
				<h1 className="text-muted-foreground text-xl font-medium">Budgets and rate limits</h1>
				<div className="text-muted-foreground mx-auto mt-2 max-w-[600px] text-sm font-normal">
					Set spending caps and rate limits at any scope: virtual keys, users, providers, or specific models.
				</div>
				<div className="mx-auto mt-6 flex flex-row flex-wrap items-center justify-center gap-2">
					<Button aria-label="Add your first limit" onClick={onAddClick} disabled={!canCreate} data-testid="model-limits-button-create">
						Add Limit
					</Button>
				</div>
			</div>
		</div>
	);
}