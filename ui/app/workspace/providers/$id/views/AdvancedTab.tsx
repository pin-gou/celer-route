import { useTranslation } from "react-i18next";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { BetaHeadersFormFragment } from "@/app/workspace/providers/fragments/betaHeadersFormFragment";
import { DebuggingFormFragment } from "@/app/workspace/providers/fragments/debuggingFormFragment";
import { GovernanceFormFragment } from "@/app/workspace/providers/fragments/governanceFormFragment";
import { OpenAIConfigFormFragment } from "@/app/workspace/providers/fragments/openaiConfigFormFragment";
import { PerformanceFormFragment } from "@/app/workspace/providers/fragments/performanceFormFragment";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { ModelProvider } from "@/lib/types/config";

const ANTHROPIC_FAMILY_PROVIDERS = ["anthropic", "vertex", "bedrock", "bedrock_mantle", "azure"];

interface AdvancedTabProps {
	provider: ModelProvider;
}

export function AdvancedTab({ provider }: AdvancedTabProps) {
	const { t } = useTranslation("providers");
	const hasGovernanceAccess = useRbac(RbacResource.Governance, RbacOperation.View);
	const providerName = String(provider.name).toLowerCase();
	const isAnthropicFamily = ANTHROPIC_FAMILY_PROVIDERS.includes(providerName);
	const isOpenAI = providerName === "openai";

	return (
		<div data-testid="providers2-advanced-tab">
			<Accordion type="multiple" defaultValue={["performance"]}>
				<AccordionItem value="performance">
					<AccordionTrigger data-testid="providers2-advanced-performance-trigger">{t("providers2.overview.performance")}</AccordionTrigger>
					<AccordionContent>
						<PerformanceFormFragment provider={provider} />
					</AccordionContent>
				</AccordionItem>

				{hasGovernanceAccess && (
					<AccordionItem value="governance">
						<AccordionTrigger data-testid="providers2-advanced-governance-trigger">{t("providers2.overview.governance")}</AccordionTrigger>
						<AccordionContent>
							<GovernanceFormFragment provider={provider} />
						</AccordionContent>
					</AccordionItem>
				)}

				{isAnthropicFamily && (
					<AccordionItem value="beta-headers">
						<AccordionTrigger data-testid="providers2-advanced-beta-headers-trigger">
							{t("providers2.overview.betaHeaders")}
						</AccordionTrigger>
						<AccordionContent>
							<BetaHeadersFormFragment provider={provider} />
						</AccordionContent>
					</AccordionItem>
				)}

				{isOpenAI && (
					<AccordionItem value="openai-config">
						<AccordionTrigger data-testid="providers2-advanced-openai-config-trigger">
							{t("providers2.overview.openaiConfig")}
						</AccordionTrigger>
						<AccordionContent>
							<OpenAIConfigFormFragment provider={provider} />
						</AccordionContent>
					</AccordionItem>
				)}

				<AccordionItem value="debugging">
					<AccordionTrigger data-testid="providers2-advanced-debugging-trigger">{t("providers2.overview.debugging")}</AccordionTrigger>
					<AccordionContent>
						<DebuggingFormFragment provider={provider} />
					</AccordionContent>
				</AccordionItem>
			</Accordion>
		</div>
	);
}