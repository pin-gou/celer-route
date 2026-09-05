import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scrollArea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { AgentModelInput, CodingAgentId } from "@/lib/utils/agentConfigs";
import type { V1Model } from "@/lib/hooks/useV1Models";

export interface ModelsStepProps {
	agent: CodingAgentId;
	showModelSubset: boolean;
	v1Models: V1Model[];
	isLoadingModels: boolean;
	v1Error: string | null;
	onRefetch: () => void;
	search: string;
	onSearchChange: (value: string) => void;
	selectedIds: Set<string>;
	onToggleModel: (id: string) => void;
	onToggleAllVisible: () => void;
	allVisibleSelected: boolean;
	defaultModelId: string;
	onDefaultModelIdChange: (id: string) => void;
	defaultModelPool: AgentModelInput[];
	toAgentModel: (m: V1Model) => AgentModelInput;
}

export default function ModelsStep(props: ModelsStepProps) {
	const { t } = useTranslation("agent-setup");
	const {
		agent,
		showModelSubset,
		v1Models,
		isLoadingModels,
		v1Error,
		onRefetch,
		search,
		onSearchChange,
		selectedIds,
		onToggleModel,
		onToggleAllVisible,
		allVisibleSelected,
		defaultModelId,
		onDefaultModelIdChange,
		defaultModelPool,
	} = props;

	const filteredModels = useMemo(() => {
		const q = search.trim().toLowerCase();
		if (!q) return v1Models;
		return v1Models.filter((m) => m.id.toLowerCase().includes(q) || (m.name ?? "").toLowerCase().includes(q));
	}, [v1Models, search]);

	return (
		<div className="space-y-6">
			{showModelSubset ? (
				<section className="space-y-3">
					<div className="flex items-center justify-between gap-2">
						<Label>{t("modelsLabel")}</Label>
						<div className="flex items-center gap-3">
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={onToggleAllVisible}
								disabled={filteredModels.length === 0}
								data-testid="agent-setup-models-toggle"
							>
								{allVisibleSelected ? t("modelsClear") : t("modelsSelectAll")}
							</Button>
							<Button
								type="button"
								variant="ghost"
								size="sm"
								onClick={onRefetch}
								disabled={isLoadingModels}
								data-testid="agent-setup-models-refresh"
								aria-label={t("modelsRefresh")}
							>
								{t("modelsRefresh")}
							</Button>
							<span className="text-muted-foreground text-xs" data-testid="agent-setup-models-count">
								{t("modelsSelected", { count: selectedIds.size })}
							</span>
						</div>
					</div>
					<div className="relative">
						<Search className="text-muted-foreground absolute top-1/2 left-2 h-4 w-4 -translate-y-1/2" />
						<Input
							value={search}
							onChange={(e) => onSearchChange(e.target.value)}
							placeholder={t("modelsSearch")}
							className="pl-8"
							data-testid="agent-setup-models-search"
						/>
					</div>
					<ScrollArea className="h-64 rounded-md border" data-testid="agent-setup-models-list">
						{isLoadingModels && v1Models.length === 0 ? (
							<div className="text-muted-foreground p-4 text-sm">{t("loading")}</div>
						) : v1Error ? (
							<div className="space-y-2 p-4">
								<p className="text-destructive text-sm">{v1Error}</p>
								<Button size="sm" variant="outline" onClick={onRefetch} data-testid="agent-setup-models-retry">
									{t("modelsRetry")}
								</Button>
							</div>
						) : filteredModels.length === 0 ? (
							<div className="text-muted-foreground p-4 text-sm">{t("modelsEmpty")}</div>
						) : (
							<div className="p-2">
								{filteredModels.map((m) => {
									const id = m.id;
									const checked = selectedIds.has(id);
									return (
										<label
											key={id}
											className="hover:bg-muted/50 flex cursor-pointer items-center gap-2 rounded px-2 py-1.5"
											data-testid={`agent-setup-model-${id}`}
										>
											<Checkbox checked={checked} onCheckedChange={() => onToggleModel(id)} />
											<span className="truncate font-mono text-xs">{id}</span>
										</label>
									);
								})}
							</div>
						)}
					</ScrollArea>
					<p className="text-muted-foreground text-xs">{t("stepHint.modelsSubset", { agent: t(`agent.${agentKey(agent)}`) })}</p>
				</section>
			) : (
				<section className="space-y-3">
					<Label>{t("defaultModelLabel")}</Label>
					<p className="text-muted-foreground text-xs">{t("defaultModelHint")}</p>
				</section>
			)}

			<section className="space-y-2">
				<Label htmlFor="agent-setup-default-model">{t("defaultModelLabel")}</Label>
				<Select value={defaultModelId} onValueChange={onDefaultModelIdChange} disabled={defaultModelPool.length === 0}>
					<SelectTrigger id="agent-setup-default-model" className="w-full" data-testid="agent-setup-default-model">
						<SelectValue placeholder={t("defaultModelPlaceholder")} />
					</SelectTrigger>
					<SelectContent>
						{defaultModelPool.map((m) => (
							<SelectItem key={m.id} value={m.id} data-testid={`agent-setup-default-model-${m.id}`}>
								<span className="truncate font-mono">{m.id}</span>
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				<p className="text-muted-foreground text-xs">{t("stepHint.modelsDefault")}</p>
			</section>
		</div>
	);
}

function agentKey(agent: CodingAgentId): string {
	switch (agent) {
		case "claude-code":
			return "claudeCode";
		case "openai-compatible":
			return "openaiCompatible";
		default:
			return agent;
	}
}