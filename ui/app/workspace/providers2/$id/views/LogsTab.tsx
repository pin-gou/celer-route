import { Button } from "@/components/ui/button";
import { ModelProvider } from "@/lib/types/config";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { ExternalLink } from "lucide-react";

interface LogsTabProps {
	provider: ModelProvider;
}

export function LogsTab({ provider }: LogsTabProps) {
	const { t } = useTranslation("providers");
	const navigate = useNavigate();

	const handleViewLogs = () => {
		navigate({ to: "/workspace/logs", search: { provider: provider.name } });
	};

	return (
		<div data-testid="providers2-logs-tab" className="rounded-lg border p-6">
			<h3 className="mb-4 text-sm font-medium">{t("providers2.logsTab.title")}</h3>
			<p className="text-muted-foreground mb-4 text-sm">{t("providers2.logsTab.description", { provider: provider.name })}</p>
			<Button variant="outline" onClick={handleViewLogs} className="gap-2">
				<ExternalLink className="h-4 w-4" />
				{t("providers2.logsTab.viewLogs", { provider: provider.name })}
			</Button>
		</div>
	);
}