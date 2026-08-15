import { Button } from "@/components/ui/button";
import { Webhook } from "lucide-react";
import { useTranslation } from "react-i18next";

interface WebhooksEmptyStateProps {
	onAddClick: () => void;
	canCreate: boolean;
}

export function WebhooksEmptyState({ onAddClick, canCreate }: WebhooksEmptyStateProps) {
	const { t } = useTranslation("webhooks");
	return (
		<div
			className="flex min-h-[80vh] w-full flex-col items-center justify-center gap-4 py-16 text-center"
			data-testid="webhooks-empty-state"
		>
			<div className="text-muted-foreground">
				<Webhook className="h-[5.5rem] w-[5.5rem]" strokeWidth={1} />
			</div>
			<div className="flex flex-col gap-1">
				<h1 className="text-muted-foreground text-xl font-medium">{t("emptyState.title")}</h1>
				<div className="text-muted-foreground mx-auto mt-2 max-w-[600px] text-sm font-normal">
					{t("emptyState.description")}
				</div>
				<div className="mx-auto mt-6 flex flex-row flex-wrap items-center justify-center gap-2">
					<Button onClick={onAddClick} disabled={!canCreate} data-testid="create-webhook-btn">
						Add Webhook Endpoint
					</Button>
				</div>
			</div>
		</div>
	);
}