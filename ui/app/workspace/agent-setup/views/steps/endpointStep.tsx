import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { CodingAgentId } from "@/lib/utils/agentConfigs";
import type { VirtualKey } from "@/lib/types/governance";

export interface EndpointStepProps {
	agent: CodingAgentId;
	baseUrl: string;
	onBaseUrlChange: (value: string) => void;
	protocol: "chat" | "responses";
	onProtocolChange: (value: "chat" | "responses") => void;
	enforceAuth: boolean;
	virtualKeys: VirtualKey[];
	selectedApiKeyId: string;
	selectedApiKeyName: string;
	onSelectedApiKeyIdChange: (id: string) => void;
}

export default function EndpointStep({
	agent,
	baseUrl,
	onBaseUrlChange,
	protocol,
	onProtocolChange,
	enforceAuth,
	virtualKeys,
	selectedApiKeyId,
	selectedApiKeyName,
	onSelectedApiKeyIdChange,
}: EndpointStepProps) {
	const { t } = useTranslation("agent-setup");
	return (
		<div className="space-y-6">
			<section className="space-y-3">
				<Label htmlFor="agent-setup-baseurl">{t("baseUrlLabel")}</Label>
				<Input
					id="agent-setup-baseurl"
					value={baseUrl}
					onChange={(e) => onBaseUrlChange(e.target.value)}
					placeholder="http://localhost:8080"
					className="font-mono"
					data-testid="agent-setup-baseurl"
				/>
				<p className="text-muted-foreground text-xs">{t("baseUrlHint")}</p>
			</section>

			{agent === "opencode" && (
				<section className="space-y-3">
					<div className="flex items-center gap-2">
						<Checkbox
							id="agent-setup-protocol"
							checked={protocol === "responses"}
							onCheckedChange={(v) => onProtocolChange(v ? "responses" : "chat")}
							data-testid="agent-setup-protocol"
						/>
						<label htmlFor="agent-setup-protocol" className="text-muted-foreground text-sm">
							{t("responsesProtocol")}
						</label>
					</div>
					<p className="text-muted-foreground text-xs">{t("stepHint.endpointProtocol")}</p>
				</section>
			)}

			<section className="space-y-3">
				<div className="flex items-center justify-between">
					<Label>{t("apiKeyLabel")}</Label>
					{enforceAuth && virtualKeys.length === 0 && (
						<Link
							to="/workspace/governance/virtual-keys"
							className="text-primary text-xs underline-offset-2 hover:underline"
							data-testid="agent-setup-apikey-create-link"
						>
							{t("apiKeyCreateLink")}
						</Link>
					)}
				</div>
				{enforceAuth ? (
					virtualKeys.length > 0 ? (
						<Select value={selectedApiKeyId} onValueChange={onSelectedApiKeyIdChange}>
							<SelectTrigger className="w-full" data-testid="agent-setup-apikey">
								<SelectValue placeholder={t("apiKeyPlaceholder")} />
							</SelectTrigger>
							<SelectContent>
								{virtualKeys.map((vk) => (
									<SelectItem key={vk.id} value={vk.id} data-testid={`agent-setup-apikey-${vk.name}`}>
										<span className="truncate">{vk.name}</span>
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					) : (
						<p className="text-muted-foreground text-xs">{t("apiKeyEmpty")}</p>
					)
				) : (
					<p className="text-muted-foreground text-xs">{t("apiKeyOptional")}</p>
				)}
				<p className="text-muted-foreground text-xs">{t("stepHint.endpoint")}</p>
			</section>

			{enforceAuth && virtualKeys.length > 0 && selectedApiKeyName && (
				<p className="text-muted-foreground text-xs" data-testid="agent-setup-apikey-summary">
					{t("apiKeySummary", { name: selectedApiKeyName })}
				</p>
			)}
		</div>
	);
}