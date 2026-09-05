import { useTranslation } from "react-i18next";
import { Label } from "@/components/ui/label";
import { PlatformSelect } from "@/components/ui/platformSelect";
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select";
import { AGENT_GROUPS, type AgentGroupId, type CodingAgentId } from "@/lib/utils/agentConfigs";
import type { ClientPlatform } from "@/lib/types/platform";

const AGENT_LABEL_KEY: Record<CodingAgentId, string> = {
	opencode: "agent.opencode",
	"claude-code": "agent.claudeCode",
	codex: "agent.codex",
	"openai-compatible": "agent.openaiCompatible",
	cursor: "agent.cursor",
	workbuddy: "agent.workbuddy",
	codebuddy: "agent.codebuddy",
	trae: "agent.trae",
	zcode: "agent.zcode",
	marscode: "agent.marscode",
	lingma: "agent.lingma",
};

const AGENT_GROUP_LABEL_KEY: Record<AgentGroupId, string> = {
	coding: "agent.group.coding",
	domestic: "agent.group.domestic",
	ide: "agent.group.ide",
	generic: "agent.group.generic",
};

export interface ClientStepProps {
	agent: CodingAgentId;
	onAgentChange: (agent: CodingAgentId) => void;
	platform: ClientPlatform;
	onPlatformChange: (platform: ClientPlatform) => void;
}

export default function ClientStep({ agent, onAgentChange, platform, onPlatformChange }: ClientStepProps) {
	const { t } = useTranslation("agent-setup");
	return (
		<div className="space-y-6">
			<section className="space-y-3">
				<Label>{t("platformLabel")}</Label>
				<PlatformSelect platform={platform} onPlatformChange={onPlatformChange} testIdPrefix="agent-setup" />
				<p className="text-muted-foreground text-xs">{t("platformHint")}</p>
			</section>

			<section className="space-y-3">
				<Label>{t("agentLabel")}</Label>
				<Select value={agent} onValueChange={(v) => onAgentChange(v as CodingAgentId)}>
					<SelectTrigger className="w-full" data-testid="agent-setup-agent">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{AGENT_GROUPS.map((group) => (
							<SelectGroup key={group.id}>
								<SelectLabel className="text-muted-foreground px-2 py-1 text-xs">{t(AGENT_GROUP_LABEL_KEY[group.id])}</SelectLabel>
								{group.agents.map((id) => (
									<SelectItem key={id} value={id} data-testid={`agent-setup-agent-${id}`}>
										{t(AGENT_LABEL_KEY[id])}
									</SelectItem>
								))}
							</SelectGroup>
						))}
					</SelectContent>
				</Select>
				<p className="text-muted-foreground text-xs">{t("stepHint.client")}</p>
			</section>
		</div>
	);
}