import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PlatformSelect } from "@/components/ui/platformSelect";
import { ScrollArea } from "@/components/ui/scrollArea";
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select";
import { TestCommandTabs, type TestCommandTab } from "@/components/testCommandPanel";
import { Button } from "@/components/ui/button";
import type { ClientPlatform } from "@/lib/types/platform";
import { useGetCoreConfigQuery } from "@/lib/store";
import { useGetVirtualKeysQuery } from "@/lib/store/apis/governanceApi";
import { useV1Models, type V1Model } from "@/lib/hooks/useV1Models";
import {
	AGENT_GROUPS,
	buildApplyCommand,
	envTabCode,
	generateAgentConfig,
	toOpenAISurface,
	type AgentConfigOutput,
	type AgentGroupId,
	type AgentModelInput,
	type CodingAgentId,
} from "@/lib/utils/agentConfigs";
import { detectPlatform } from "@/lib/utils/platform";
import { buildExamples, resolveEndpointUrl } from "@/lib/utils/testCommandSnippets";
import { parseAsStringLiteral, useQueryStates } from "nuqs";
import { Search, TerminalSquare, WandSparkles } from "lucide-react";

const PLATFORMS = ["macos", "windows", "linux"] as const;

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

// requiresModelSubset is the set of agents whose config embeds an explicit
// list of models. opencode writes a `models` block under `provider`;
// WorkBuddy / CodeBuddy write the `models[]` array into models.json. The
// other agents only carry a single default-model reference (ANTHROPIC_MODEL,
// codex `model`, OPENAI_MODEL, in-app step), so the picker collapses to a
// single dropdown sourced from the live /v1/models catalog.
const REQUIRES_MODEL_SUBSET: ReadonlySet<CodingAgentId> = new Set(["opencode", "workbuddy", "codebuddy"]);

function requiresModelSubset(agent: CodingAgentId): boolean {
	return REQUIRES_MODEL_SUBSET.has(agent);
}

function toAgentModel(m: V1Model): AgentModelInput {
	const ctx = m.context_length ?? m.max_input_tokens;
	return {
		// /v1/models returns `id` already in `provider/name` form — write it
		// straight into the agent config. No reassembly needed.
		id: m.id,
		name: m.name,
		contextLength: ctx,
		maxOutputTokens: m.max_output_tokens,
	};
}

export default function AgentSetupView() {
	const { t } = useTranslation("agent-setup");

	const [agent, setAgent] = useState<CodingAgentId>("opencode");
	const [baseUrl, setBaseUrl] = useState<string>(() => resolveEndpointUrl());
	const [protocol, setProtocol] = useState<"chat" | "responses">("chat");
	const [search, setSearch] = useState("");
	const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
	const [defaultModelId, setDefaultModelId] = useState<string>("");
	const [selectedApiKeyId, setSelectedApiKeyId] = useState<string>("");

	// Target OS: auto-detected first, overridable, persisted in the URL so a
	// shared link keeps the same shell/path conventions.
	const [urlPlatform, setUrlPlatform] = useQueryStates({ platform: parseAsStringLiteral(PLATFORMS) }, { history: "replace" });
	const platform: ClientPlatform = urlPlatform.platform ?? detectPlatform();
	const setPlatform = (p: ClientPlatform) => setUrlPlatform({ platform: p });

	const showModelSubset = requiresModelSubset(agent);

	const { data: bifrostConfig, isSuccess: configSettled, isError: configFailed } = useGetCoreConfigQuery({});
	// The auth mode is only known once the core-config query resolves. Until
	// then `enforce_auth_on_inference` reads as falsy and gating the models
	// probe on it alone would still fire an unauthenticated request.
	const authDecided = configSettled || configFailed;
	const enforceAuth = authDecided && !!bifrostConfig?.client_config?.enforce_auth_on_inference;
	const { data: vksResponse } = useGetVirtualKeysQuery(undefined, { skip: !enforceAuth });
	const vks = useMemo(() => vksResponse?.virtual_keys ?? [], [vksResponse]);

	const selectedApiKey = useMemo(() => vks.find((v) => v.id === selectedApiKeyId) ?? vks[0] ?? null, [vks, selectedApiKeyId]);
	// On enforce-auth-on gateways we must send the user's selected API key
	// (the virtual key value); on open gateways (auth off) we omit the
	// header entirely so the result matches an unauthenticated `curl /v1/models`.
	const apiKey = enforceAuth ? (selectedApiKey?.value ?? null) : null;

	// Hold off on the /v1/models probe until the auth mode is settled AND, on
	// enforce-auth gateways, a virtual key is actually available. Otherwise the
	// hook fires an unauthenticated request, then a second authenticated one
	// once the key resolves — two calls to /v1/models.
	const modelsSkip = !authDecided || (enforceAuth && !selectedApiKey);

	const { models: v1Models, isLoading: isLoadingModels, error: v1Error, refetch } = useV1Models(baseUrl, apiKey, modelsSkip);

	// Reset the default model + selection when the catalog changes (e.g.
	// user switched API key and the catalog no longer contains the picked id).
	useEffect(() => {
		setSelectedIds((prev) => {
			const valid = new Set<string>();
			for (const id of prev) if (v1Models.some((m) => m.id === id)) valid.add(id);
			return valid;
		});
		setDefaultModelId((prev) => (v1Models.some((m) => m.id === prev) ? prev : ""));
	}, [v1Models]);

	const filteredModels = useMemo(() => {
		const q = search.trim().toLowerCase();
		if (!q) return v1Models;
		return v1Models.filter((m) => m.id.toLowerCase().includes(q) || (m.name ?? "").toLowerCase().includes(q));
	}, [v1Models, search]);

	const allVisibleSelected = useMemo(
		() => filteredModels.length > 0 && filteredModels.every((m) => selectedIds.has(m.id)),
		[filteredModels, selectedIds],
	);

	const toggleModel = (id: string) => {
		setSelectedIds((prev) => {
			const next = new Set(prev);
			if (next.has(id)) next.delete(id);
			else next.add(id);
			return next;
		});
	};

	const toggleAllVisible = () => {
		setSelectedIds((prev) => {
			const next = new Set(prev);
			for (const m of filteredModels) {
				if (allVisibleSelected) next.delete(m.id);
				else next.add(m.id);
			}
			return next;
		});
	};

	const selectedModelInputs = useMemo(() => v1Models.filter((m) => selectedIds.has(m.id)).map(toAgentModel), [v1Models, selectedIds]);

	// For agents that don't embed a models block, the default-model picker
	// should list the entire /v1/models catalog (scoped by the selected API
	// key), not the user's hand-picked subset.
	const defaultModelPool = useMemo(
		() => (showModelSubset ? selectedModelInputs : v1Models.map(toAgentModel)),
		[showModelSubset, selectedModelInputs, v1Models],
	);

	// Keep a valid default model: prefer the user's pick, else first available.
	const effectiveDefaultModelId = useMemo(() => {
		if (defaultModelId && defaultModelPool.some((m) => m.id === defaultModelId)) return defaultModelId;
		return defaultModelPool[0]?.id ?? "";
	}, [defaultModelPool, defaultModelId]);

	const output: AgentConfigOutput | null = useMemo(() => {
		// For opencode we need at least one selected model; the other agents
		// only need a default model picked from the catalog.
		if (showModelSubset && selectedModelInputs.length === 0) return null;
		if (!showModelSubset && v1Models.length === 0) return null;
		return generateAgentConfig({
			agent,
			baseUrl,
			apiKey,
			models: showModelSubset ? selectedModelInputs : v1Models.map(toAgentModel),
			defaultModelId: effectiveDefaultModelId,
			protocol: agent === "opencode" ? protocol : undefined,
			platform,
		});
	}, [agent, baseUrl, apiKey, selectedModelInputs, v1Models, showModelSubset, effectiveDefaultModelId, protocol, platform]);

	const tabs = useMemo(() => {
		if (!output) return [];
		const result: TestCommandTab[] = [];
		const apply = buildApplyCommand(output, platform);
		if (apply) {
			result.push({
				id: "apply",
				label: t("applyTab"),
				code: apply,
				copyLabel: t("copy"),
				copiedLabel: t("copied"),
				copyText: t("copy"),
				copySuccessMessage: t("copySuccess"),
			});
		}
		for (const [i, f] of output.files.entries()) {
			result.push({
				id: `file-${i}`,
				label: f.path,
				code: f.content,
				copyLabel: t("copy"),
				copiedLabel: t("copied"),
				copyText: t("copy"),
				copySuccessMessage: t("copySuccess"),
			});
		}
		const envCode = output.env ? envTabCode(output.env, platform) : "";
		if (envCode.length > 0 && !output.files.some((f) => f.content === envCode)) {
			result.push({
				id: "env",
				label: t("envTab"),
				code: envCode,
				copyLabel: t("copy"),
				copiedLabel: t("copied"),
				copyText: t("copy"),
				copySuccessMessage: t("copySuccess"),
			});
		}
		if (effectiveDefaultModelId) {
			const probe = buildExamples(toOpenAISurface(baseUrl), effectiveDefaultModelId, apiKey, platform);
			result.push({
				id: "test",
				label: t("testTab"),
				code: probe.curl,
				copyLabel: t("copy"),
				copiedLabel: t("copied"),
				copyText: t("copy"),
				copySuccessMessage: t("copySuccess"),
			});
		}
		return result;
	}, [output, t, effectiveDefaultModelId, baseUrl, apiKey, platform]);

	const noModelPickedError = showModelSubset && selectedModelInputs.length === 0;

	return (
		<div className="space-y-6">
			<div className="space-y-1">
				<h2 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
					<TerminalSquare className="text-muted-foreground h-5 w-5" />
					{t("title")}
				</h2>
				<p className="text-muted-foreground text-sm">{t("subtitle")}</p>
			</div>

			<div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
				{/* Left column: platform + agent + endpoint + key */}
				<div className="space-y-6">
					<section className="space-y-3">
						<Label>{t("platformLabel")}</Label>
						<PlatformSelect platform={platform} onPlatformChange={setPlatform} testIdPrefix="agent-setup" />
						<p className="text-muted-foreground text-xs">{t("platformHint")}</p>
					</section>

					<section className="space-y-3">
						<Label>{t("agentLabel")}</Label>
						<Select value={agent} onValueChange={(v) => setAgent(v as CodingAgentId)}>
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
						{agent === "opencode" && (
							<div className="flex items-center gap-2">
								<Checkbox
									id="agent-setup-protocol"
									checked={protocol === "responses"}
									onCheckedChange={(v) => setProtocol(v ? "responses" : "chat")}
									data-testid="agent-setup-protocol"
								/>
								<label htmlFor="agent-setup-protocol" className="text-muted-foreground text-sm">
									{t("responsesProtocol")}
								</label>
							</div>
						)}
					</section>

					<section className="space-y-3">
						<Label htmlFor="agent-setup-baseurl">{t("baseUrlLabel")}</Label>
						<Input
							id="agent-setup-baseurl"
							value={baseUrl}
							onChange={(e) => setBaseUrl(e.target.value)}
							placeholder="http://localhost:8080"
							className="font-mono"
							data-testid="agent-setup-baseurl"
						/>
						<p className="text-muted-foreground text-xs">{t("baseUrlHint")}</p>
					</section>

					<section className="space-y-3">
						<div className="flex items-center justify-between">
							<Label>{t("apiKeyLabel")}</Label>
							{enforceAuth && vks.length === 0 && (
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
							vks.length > 0 ? (
								<Select value={selectedApiKey?.id ?? ""} onValueChange={(v) => setSelectedApiKeyId(v)}>
									<SelectTrigger className="w-full" data-testid="agent-setup-apikey">
										<SelectValue placeholder={t("apiKeyPlaceholder")} />
									</SelectTrigger>
									<SelectContent>
										{vks.map((vk) => (
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
					</section>
				</div>

				{/* Right column: default model + (opencode only) model subset */}
				<section className="space-y-3">
					{showModelSubset ? (
						<>
							<div className="flex items-center justify-between gap-2">
								<Label>{t("modelsLabel")}</Label>
								<div className="flex items-center gap-3">
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={toggleAllVisible}
										disabled={filteredModels.length === 0}
										data-testid="agent-setup-models-toggle"
									>
										{allVisibleSelected ? t("modelsClear") : t("modelsSelectAll")}
									</Button>
									<Button
										type="button"
										variant="ghost"
										size="sm"
										onClick={refetch}
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
									onChange={(e) => setSearch(e.target.value)}
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
										<Button size="sm" variant="outline" onClick={refetch} data-testid="agent-setup-models-retry">
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
													<Checkbox checked={checked} onCheckedChange={() => toggleModel(id)} />
													<span className="truncate font-mono text-xs">{id}</span>
												</label>
											);
										})}
									</div>
								)}
							</ScrollArea>
						</>
					) : (
						<div className="space-y-1">
							<Label>{t("defaultModelLabel")}</Label>
							<p className="text-muted-foreground text-xs">{t("defaultModelHint")}</p>
						</div>
					)}

					<section className="space-y-2">
						{!showModelSubset && <div />}
						<Label htmlFor="agent-setup-default-model">{t("defaultModelLabel")}</Label>
						<Select value={effectiveDefaultModelId} onValueChange={(v) => setDefaultModelId(v)} disabled={defaultModelPool.length === 0}>
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
					</section>
				</section>
			</div>

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