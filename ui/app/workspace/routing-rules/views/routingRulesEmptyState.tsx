import { Button } from "@/components/ui/button";
import { useTranslation } from "react-i18next";
import { Route } from "lucide-react";

interface RoutingRulesEmptyStateProps {
	onAddClick: () => void;
	canCreate?: boolean;
}

export function RoutingRulesEmptyState({ onAddClick, canCreate = true }: RoutingRulesEmptyStateProps) {
	const { t } = useTranslation("routing");
	return (
		<div
			className="flex min-h-[80vh] w-full flex-col items-center justify-center gap-4 py-16 text-center"
			data-testid="routing-rules-empty-state"
		>
			<div className="text-muted-foreground">
				<Route className="h-[5.5rem] w-[5.5rem]" strokeWidth={1} />
			</div>
			<div className="flex flex-col gap-1">
				<h1 className="text-muted-foreground text-xl font-medium">{t("routingRules.emptyTitle")}</h1>
				<div className="text-muted-foreground mx-auto mt-2 max-w-[600px] text-sm font-normal">
					{t("routingRules.emptyDescription")}
				</div>
				<div className="mx-auto mt-6 flex flex-row flex-wrap items-center justify-center gap-2">
					<Button
						aria-label={t("rules.createFirstAriaLabel")}
						data-testid="create-routing-rule-btn"
						onClick={onAddClick}
						disabled={!canCreate}
					>
						{t("page.newRule")}
					</Button>
				</div>
			</div>
		</div>
	);
}