import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { AlertTriangle, Info, Lightbulb } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ModelProvider } from "@/lib/types/config";

const ANTHROPIC_FAMILY_PROVIDERS = ["anthropic", "vertex", "bedrock", "bedrock_mantle", "azure"];

interface ConfigSuggestionsProps {
	provider: ModelProvider;
}

type SuggestionSeverity = "info" | "warning";
type SuggestionId =
	| "multipleKeysNoRetries"
	| "multipleKeysNoCooldown"
	| "noModels"
	| "anthropicNoBetaHeaders"
	| "customBaseUrlDefaultTimeout";

interface Suggestion {
	id: SuggestionId;
	severity: SuggestionSeverity;
}

function computeSuggestions(provider: ModelProvider): Suggestion[] {
	const suggestions: Suggestion[] = [];
	const keysCount = provider.keys_count ?? 0;
	const maxRetries = provider.network_config?.max_retries ?? 0;
	const cooldownPolicy = provider.cooldown_policy;
	const cooldownEnabled = !!cooldownPolicy?.rate_limit && cooldownPolicy.rate_limit.enabled !== false;
	const modelsCount = provider.models_count ?? 0;
	const baseUrl = provider.network_config?.base_url;
	const defaultTimeout = provider.network_config?.default_request_timeout_in_seconds;
	const providerName = String(provider.name).toLowerCase();
	const isAnthropicFamily = ANTHROPIC_FAMILY_PROVIDERS.includes(providerName);

	if (keysCount > 1 && maxRetries === 0) {
		suggestions.push({ id: "multipleKeysNoRetries", severity: "warning" });
	}
	if (keysCount > 1 && !cooldownEnabled) {
		suggestions.push({ id: "multipleKeysNoCooldown", severity: "info" });
	}
	if (modelsCount === 0) {
		suggestions.push({ id: "noModels", severity: "warning" });
	}
	if (isAnthropicFamily && !provider.network_config?.beta_header_overrides) {
		suggestions.push({ id: "anthropicNoBetaHeaders", severity: "info" });
	}
	if (baseUrl && (defaultTimeout === undefined || defaultTimeout === 300)) {
		suggestions.push({ id: "customBaseUrlDefaultTimeout", severity: "info" });
	}
	return suggestions;
}

const iconFor = (severity: SuggestionSeverity) => {
	if (severity === "warning") return AlertTriangle;
	return Info;
};

export function ConfigSuggestions({ provider }: ConfigSuggestionsProps) {
	const { t } = useTranslation("providers");
	const suggestions = computeSuggestions(provider);
	if (suggestions.length === 0) return null;

	return (
		<div data-testid="providers2-overview-suggestions" className="space-y-2">
			{suggestions.map((s) => {
				const Icon = iconFor(s.severity);
				return (
					<Alert key={s.id} variant={s.severity} data-testid={`providers2-overview-suggestion-${s.id}`}>
						<Icon className="h-4 w-4" />
						<AlertTitle className="flex items-center gap-1">
							<Lightbulb className="h-3.5 w-3.5" />
							{t(`providers2.suggestions.${s.id}.title`)}
						</AlertTitle>
						<AlertDescription>{t(`providers2.suggestions.${s.id}.description`, { count: provider.keys_count ?? 0 })}</AlertDescription>
					</Alert>
				);
			})}
		</div>
	);
}