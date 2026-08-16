import { Button } from "@/components/ui/button";
import ProviderConfigSheet from "@/app/workspace/providers/dialogs/providerConfigSheet";
import { ModelProvider } from "@/lib/types/config";
import { useTranslation } from "react-i18next";
import { SettingsIcon } from "lucide-react";
import { useState } from "react";

interface OpenLegacyConfigSheetButtonProps {
	provider: ModelProvider;
}

export default function OpenLegacyConfigSheetButton({ provider }: OpenLegacyConfigSheetButtonProps) {
	const { t } = useTranslation("providers");
	const [showConfigSheet, setShowConfigSheet] = useState(false);

	return (
		<>
			<Button
				variant="outline"
				size="sm"
				data-testid="providers2-open-legacy-config-sheet"
				onClick={() => setShowConfigSheet(true)}
				className="gap-1 text-xs"
			>
				<SettingsIcon className="h-3 w-3" />
				{t("providers2.openLegacyConfigSheet")}
			</Button>
			<ProviderConfigSheet show={showConfigSheet} onCancel={() => setShowConfigSheet(false)} provider={provider} />
		</>
	);
}