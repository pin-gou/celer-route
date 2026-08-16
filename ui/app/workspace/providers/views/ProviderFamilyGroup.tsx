import { ChevronDown, ChevronRight } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ProviderCard, type ProviderCardProvider } from "./ProviderCard";

export interface ProviderFamilyGroupProps {
	familyName: string;
	providers: ProviderCardProvider[];
	onToggle: (providerName: string) => void;
	onQuickTest: (providerName: string) => void;
	onDelete: (providerName: string) => void;
}

export function ProviderFamilyGroup({ familyName, providers, onToggle, onQuickTest, onDelete }: ProviderFamilyGroupProps) {
	const { t } = useTranslation("providers");
	const [isExpanded, setIsExpanded] = useState(true);

	const familyLabel = t(
		{
			Custom: "providers2.family.custom",
			Other: "providers2.family.other",
			"OpenAI Family": "providers2.family.openai",
			"Anthropic Family": "providers2.family.anthropic",
			"Google Family": "providers2.family.google",
			"Meta-Llama Family": "providers2.family.metaLlama",
			"AWS Family": "providers2.family.aws",
		}[familyName] ?? familyName,
	);

	return (
		<div data-testid={`providers2-family-group-${familyName.toLowerCase().replace(/\s+/g, "-")}`} className="mb-6">
			<button
				data-testid="providers2-family-toggle"
				onClick={() => setIsExpanded(!isExpanded)}
				className="mb-3 flex w-full items-center gap-2 py-1 text-left"
			>
				{isExpanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
				<h2 className="text-sm font-semibold">{familyLabel}</h2>
				<span className="text-muted-foreground text-xs">({providers.length})</span>
			</button>
			{isExpanded && (
				<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
					{providers.map((provider) => (
						<ProviderCard
							key={provider.name}
							provider={provider}
							onToggle={() => onToggle(provider.name)}
							onQuickTest={() => onQuickTest(provider.name)}
							onDelete={() => onDelete(provider.name)}
						/>
					))}
				</div>
			)}
		</div>
	);
}