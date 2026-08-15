import { Button } from "@/components/ui/button";
import { Puzzle } from "lucide-react";
import { useTranslation } from "react-i18next";

interface PluginsEmptyStateProps {
	onCreateClick: () => void;
	canCreate?: boolean;
}

export function PluginsEmptyState({ onCreateClick, canCreate = true }: PluginsEmptyStateProps) {
	const { t } = useTranslation("plugins");
	return (
		<div
			className="flex min-h-[80vh] w-full flex-col items-center justify-center gap-4 py-16 text-center"
			data-testid="plugins-empty-state"
		>
			<div className="text-muted-foreground">
				<Puzzle className="h-[5.5rem] w-[5.5rem]" strokeWidth={1} />
			</div>
			<div className="flex flex-col gap-1">
				<h1 className="text-muted-foreground text-xl font-medium">{t("emptyState.title")}</h1>
				<div className="text-muted-foreground mx-auto mt-2 max-w-[600px] text-sm font-normal">
					{t("emptyState.description")}
				</div>
				<div className="mx-auto mt-6 flex flex-row flex-wrap items-center justify-center gap-2">
					<Button
						aria-label="Create your first plugin"
						data-testid="plugins-button-install-new"
						onClick={onCreateClick}
						disabled={!canCreate}
					>
						Install New Plugin
					</Button>
				</div>
			</div>
		</div>
	);
}