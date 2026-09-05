import { useTranslation } from "react-i18next";
import { WandSparkles } from "lucide-react";
import { Label } from "@/components/ui/label";
import { TestCommandTabs, type TestCommandTab } from "@/components/testCommandPanel";
import type { AgentConfigOutput } from "@/lib/utils/agentConfigs";

export interface OutputStepProps {
	output: AgentConfigOutput | null;
	tabs: TestCommandTab[];
	noModelPickedError: boolean;
}

export default function OutputStep({ output, tabs, noModelPickedError }: OutputStepProps) {
	const { t } = useTranslation("agent-setup");
	return (
		<div className="space-y-6">
			{output ? (
				<section className="space-y-3">
					<div className="flex items-center gap-2">
						<WandSparkles className="text-muted-foreground h-4 w-4" />
						<Label>{t("outputTitle")}</Label>
					</div>
					<TestCommandTabs tabs={tabs} defaultTab={tabs[0]?.id} testIdPrefix="agent-setup-output" />
					{output.defaultModelRef && (
						<p className="text-muted-foreground text-xs">{t("defaultModelRef", { ref: output.defaultModelRef })}</p>
					)}
				</section>
			) : noModelPickedError ? (
				<div className="text-muted-foreground rounded-md border border-dashed p-4 text-sm" data-testid="agent-setup-no-output">
					{t("noOutput")}
				</div>
			) : null}
		</div>
	);
}