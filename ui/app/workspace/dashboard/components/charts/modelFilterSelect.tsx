import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

interface ModelFilterSelectProps {
	models: string[];
	selectedModel: string;
	onModelChange: (model: string) => void;
	placeholder?: string;
	"data-testid"?: string;
}

export function ModelFilterSelect({ models, selectedModel, onModelChange, placeholder, "data-testid": testId }: ModelFilterSelectProps) {
	const { t } = useTranslation("dashboard");
	const defaultPlaceholder = t("filters.allModels");

	return (
		<Select value={selectedModel} onValueChange={onModelChange}>
			<SelectTrigger className="!h-7.5 w-[110px] text-xs sm:w-[130px]" data-testid={testId} size="sm">
				<SelectValue placeholder={placeholder || defaultPlaceholder} />
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="all">{placeholder || defaultPlaceholder}</SelectItem>
				{models.filter(Boolean).map((model) => (
					<SelectItem key={model} value={model} className="text-xs">
						{model}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	);
}