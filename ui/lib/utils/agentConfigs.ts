// Agent config generation engine.
//
// celer-route exposes OpenAI-compatible (/v1), Anthropic-compatible
// (/anthropic/v1/messages) and GenAI surfaces. AI clients rarely discover
// models themselves — most require the model list to be written explicitly
// into their own config file (or hand-entered in an in-app form). This module
// turns the live model catalog (as returned by the celer-route API) + an
// endpoint + a virtual key into ready-to-paste config for the supported AI
// clients: coding agents (opencode, Claude Code, Codex), domestic desktop
// agents & IDEs (WorkBuddy, CodeBuddy, Trae, ZCode, MarsCode, 通义灵码), IDE
// extensions (Cursor) and generic OpenAI-compatible clients.
//
// The functions here are pure: they take plain data and return plain strings.
// The Web UI (workspace/agent-setup) and the `celer-route setup-*`
// CLI both consume them so both surfaces emit byte-identical config.

import type { ClientPlatform } from "@/lib/types/platform";
import { displayPath } from "@/lib/utils/platform";

export type CodingAgentId =
	| "opencode"
	| "claude-code"
	| "codex"
	| "openai-compatible"
	| "cursor"
	| "workbuddy"
	| "codebuddy"
	| "trae"
	| "zcode"
	| "marscode"
	| "lingma";

export interface AgentModelInput {
	/** Full model id as used in requests, e.g. "minimax/MiniMax-M2.1". */
	id: string;
	/** Display name, defaults to `id`. */
	name?: string;
	/** context window in tokens, from the model catalog. */
	contextLength?: number;
	/** max output tokens, from the model catalog. */
	maxOutputTokens?: number;
}

export interface AgentConfigInput {
	agent: CodingAgentId;
	/** Endpoint origin, e.g. "http://localhost:8080" or ".../v1". Normalized internally. */
	baseUrl: string;
	/** Virtual key value (sk-bf-...). May be null when auth is disabled. */
	apiKey: string | null;
	models: AgentModelInput[];
	/** Default model id to select; falls back to the first model. */
	defaultModelId?: string;
	/** opencode only: chat (@ai-sdk/openai-compatible) or responses (@ai-sdk/openai). */
	protocol?: "chat" | "responses";
	/** Target OS; drives config paths, env-var syntax and in-app shortcuts. Defaults to linux. */
	platform?: ClientPlatform;
}

export interface AgentConfigFile {
	/** Human-friendly path shown to the user, e.g. "~/.config/opencode/opencode.json". */
	path: string;
	content: string;
	language: "json" | "toml" | "shell" | "markdown";
}

/**
 * One env recipe expressed in the three shell dialects. The arrays carry no
 * comment headers; display code (envTabCode) layers those on top.
 */
export interface AgentConfigEnv {
	/** POSIX shells: export KEY=value */
	posix: string[];
	/** Windows PowerShell: $env:KEY = "value" */
	powershell: string[];
	/** Windows cmd: set KEY=value */
	cmd: string[];
}

export interface AgentConfigOutput {
	files: AgentConfigFile[];
	/** Optional env-var recipe for tools that read the key from the environment. */
	env?: AgentConfigEnv;
	/** In-app steps for tools that do not take a config file (Cursor/Windsurf). */
	steps?: string[];
	/** Full model reference for the default model, e.g. "celer-route/minimax/MiniMax-M2.1". */
	defaultModelRef?: string;
	modelIds: string[];
}

/** Provider key used inside the generated agent configs (opencode/codex). */
export const AGENT_PROVIDER_KEY = "celer-route";

/** Strip a trailing "/v1" from a base URL so we can derive sibling surfaces. */
export function stripV1Suffix(baseUrl: string): string {
	return baseUrl.replace(/\/+$/, "").replace(/\/v1$/, "");
}

/** OpenAI-compatible surface — append "/v1" unless already present. */
export function toOpenAISurface(baseUrl: string): string {
	const url = stripV1Suffix(baseUrl);
	return `${url}/v1`;
}

/** Anthropic-compatible surface — the gateway serves /anthropic/v1/messages. */
export function toAnthropicSurface(baseUrl: string): string {
	const url = stripV1Suffix(baseUrl);
	return `${url}/anthropic`;
}

function pickDefaultModelId(models: AgentModelInput[], requested?: string): string | undefined {
	if (requested && models.some((m) => m.id === requested)) return requested;
	return models[0]?.id;
}

function toLimit(contextLength?: number, maxOutputTokens?: number): Record<string, number> | undefined {
	const limit: Record<string, number> = {};
	if (contextLength && contextLength > 0) limit.context = contextLength;
	if (maxOutputTokens && maxOutputTokens > 0) limit.output = maxOutputTokens;
	return Object.keys(limit).length > 0 ? limit : undefined;
}

function effectivePlatform(input: AgentConfigInput): ClientPlatform {
	return input.platform ?? "linux";
}

/** Build the three shell dialects of an env recipe from (key, value) pairs. */
function buildEnv(entries: Array<[string, string]>): AgentConfigEnv {
	const env: AgentConfigEnv = { posix: [], powershell: [], cmd: [] };
	for (const [key, value] of entries) {
		env.posix.push(`export ${key}=${value}`);
		env.powershell.push(`$env:${key} = "${value}"`);
		env.cmd.push(`set ${key}=${value}`);
	}
	return env;
}

/**
 * Human-facing env recipe for the selected platform. Windows shows both the
 * PowerShell and the cmd block (with comment headers) so users on either
 * shell get a runnable snippet; macOS/Linux show the POSIX export lines.
 */
export function envTabCode(env: AgentConfigEnv, platform: ClientPlatform): string {
	if (platform === "windows") {
		const blocks: string[] = [];
		if (env.powershell.length > 0) blocks.push("# PowerShell:\n" + env.powershell.join("\n"));
		if (env.cmd.length > 0) blocks.push("# cmd:\n" + env.cmd.join("\n"));
		return blocks.join("\n\n");
	}
	return env.posix.join("\n");
}

function generateOpenCode(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const models = input.models;
	const defaultModelId = pickDefaultModelId(models, input.defaultModelId) ?? "";
	const defaultModelRef = `${AGENT_PROVIDER_KEY}/${defaultModelId}`;
	const npm = input.protocol === "responses" ? "@ai-sdk/openai" : "@ai-sdk/openai-compatible";
	const platform = effectivePlatform(input);

	// Build the object so key order is stable and the output is always valid
	// strict JSON (no trailing commas even when apiKey/models are empty).
	const config: Record<string, unknown> = {
		$schema: "https://opencode.ai/config.json",
	};
	if (defaultModelId) config.model = defaultModelRef;
	config.provider = {
		[AGENT_PROVIDER_KEY]: {
			npm,
			name: AGENT_PROVIDER_KEY,
			options: {
				baseURL,
				...(input.apiKey ? { apiKey: input.apiKey } : {}),
			},
			...(models.length > 0 ? { models: opencodeModelsObject(models) } : {}),
		},
	};
	const content = JSON.stringify(config, null, 2);

	return {
		files: [{ path: displayPath(platform, ".config", "opencode", "opencode.json"), content, language: "json" }],
		defaultModelRef: defaultModelId ? defaultModelRef : undefined,
		modelIds: models.map((m) => m.id),
	};
}

function opencodeModelsObject(models: AgentModelInput[]): Record<string, Record<string, unknown>> {
	const out: Record<string, Record<string, unknown>> = {};
	for (const m of models) {
		const entry: Record<string, unknown> = {};
		if (m.name && m.name !== m.id) entry.name = m.name;
		const limit = toLimit(m.contextLength, m.maxOutputTokens);
		if (limit) entry.limit = limit;
		out[m.id] = entry;
	}
	return out;
}

function generateClaudeCode(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toAnthropicSurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId);
	const platform = effectivePlatform(input);

	const entries: Array<[string, string]> = [
		["ANTHROPIC_BASE_URL", baseURL],
		...(input.apiKey ? ([["ANTHROPIC_AUTH_TOKEN", input.apiKey]] as Array<[string, string]>) : []),
		...(defaultModelId ? ([["ANTHROPIC_MODEL", defaultModelId]] as Array<[string, string]>) : []),
	];
	const env = buildEnv(entries);

	const settings: Record<string, Record<string, string>> = { env: {} };
	for (const [key, value] of entries) {
		settings.env[key] = value;
	}

	const content = JSON.stringify(settings, null, 2);

	return {
		files: [{ path: displayPath(platform, ".claude", "settings.json"), content, language: "json" }],
		env,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

function generateCodex(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";
	const envKey = "CELER_ROUTE_API_KEY";
	const platform = effectivePlatform(input);

	const content = [
		`model = ${JSON.stringify(defaultModelId)}`,
		`model_provider = ${JSON.stringify(AGENT_PROVIDER_KEY)}`,
		"",
		`[model_providers.${AGENT_PROVIDER_KEY}]`,
		`name = ${JSON.stringify(AGENT_PROVIDER_KEY)}`,
		`base_url = ${JSON.stringify(baseURL)}`,
		'wire_api = "chat"',
		`env_key = ${JSON.stringify(envKey)}`,
		"",
	].join("\n");

	return {
		files: [{ path: displayPath(platform, ".codex", "config.toml"), content, language: "toml" }],
		env: input.apiKey ? buildEnv([[envKey, input.apiKey]]) : undefined,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

/** Shared renderer for env-recipe-only clients (openai-compatible, MarsCode). */
function generateEnvRecipe(input: AgentConfigInput, entries: Array<[string, string]>): AgentConfigOutput {
	const platform = effectivePlatform(input);
	const env = buildEnv(entries);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";

	return {
		files: [
			{
				path: ".env (环境变量接入)",
				content: envTabCode(env, platform),
				language: "shell",
			},
		],
		env,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

function generateOpenAICompatible(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";

	const entries: Array<[string, string]> = [
		["OPENAI_BASE_URL", baseURL],
		...(input.apiKey ? ([["OPENAI_API_KEY", input.apiKey]] as Array<[string, string]>) : []),
		["OPENAI_MODEL", defaultModelId],
	];

	return generateEnvRecipe(input, entries);
}

function generateCursor(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";
	const providerName = AGENT_PROVIDER_KEY;
	const platform = effectivePlatform(input);
	const settingsShortcut = platform === "macos" ? "⌘," : "Ctrl+,";

	const steps = [
		`打开 Cursor → Settings（${settingsShortcut}）→ Models`,
		`在 "Model Provider" 下点击 "+ Add" → 选择 "OpenAI"`,
		`Name 填 ${providerName}`,
		`Base URL 填 ${baseURL}`,
		input.apiKey ? `API Key 填 ${input.apiKey}` : `API Key 留空（celer-route 未开启强制鉴权）`,
		`点击 "Verify"，选择默认模型：${defaultModelId}`,
		`确认后可在模型下拉中切换到任意已配置模型`,
	];

	const content = steps.map((s, i) => `${i + 1}. ${s}`).join("\n");

	return {
		files: [{ path: "Cursor 内操作步骤", content, language: "markdown" }],
		steps,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

/**
 * Tencent WorkBuddy / CodeBuddy share the same local model registry: a
 * `~/.{workbuddy,codebuddy}/models.json` containing an OpenAI-compatible
 * `models[]` list plus an `availableModels[]` allow-list. Each model entry
 * points at a full chat-completions URL, so celer-route's `/v1` surface maps
 * to `{origin}/v1/chat/completions`. The first `availableModels` entry is the
 * client's default model, so the picker's default is hoisted to the front.
 */
function generateTencentModelsJson(input: AgentConfigInput, path: string): AgentConfigOutput {
	const baseURL = `${toOpenAISurface(input.baseUrl)}/chat/completions`;
	const models = input.models;
	const defaultModelId = pickDefaultModelId(models, input.defaultModelId) ?? "";

	const ordered = [...models];
	if (defaultModelId) {
		const i = ordered.findIndex((m) => m.id === defaultModelId);
		if (i > 0) [ordered[0], ordered[i]] = [ordered[i], ordered[0]];
	}

	const entries = ordered.map((m) => {
		const entry: Record<string, string | number> = {
			id: m.id,
			name: m.name && m.name !== m.id ? m.name : m.id,
			vendor: "OpenAI",
			url: baseURL,
		};
		if (input.apiKey) entry.apiKey = input.apiKey;
		if (m.contextLength && m.contextLength > 0) entry.maxInputTokens = m.contextLength;
		if (m.maxOutputTokens && m.maxOutputTokens > 0) entry.maxOutputTokens = m.maxOutputTokens;
		return entry;
	});

	const doc: Record<string, unknown> = { models: entries };
	if (ordered.length > 0) doc.availableModels = ordered.map((m) => m.id);

	return {
		files: [{ path, content: `${JSON.stringify(doc, null, 2)}\n`, language: "json" }],
		defaultModelRef: defaultModelId || undefined,
		modelIds: models.map((m) => m.id),
	};
}

function generateWorkBuddy(input: AgentConfigInput): AgentConfigOutput {
	return generateTencentModelsJson(input, displayPath(effectivePlatform(input), ".workbuddy", "models.json"));
}

function generateCodeBuddy(input: AgentConfigInput): AgentConfigOutput {
	return generateTencentModelsJson(input, displayPath(effectivePlatform(input), ".codebuddy", "models.json"));
}

function generateTrae(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";
	const providerName = AGENT_PROVIDER_KEY;

	const steps = [
		`打开 Trae → 设置 → 模型 → 自定义模型`,
		`添加模型，API 格式选择「OpenAI」（若你的服务只提供 Anthropic 协议再换）`,
		`Name 填 ${providerName}`,
		`Base URL 填 ${baseURL}（务必填完整路径，含 /v1）`,
		input.apiKey ? `API Key 填 ${input.apiKey}` : `API Key 留空（celer-route 未开启强制鉴权）`,
		`Model ID 填 ${defaultModelId}（模型目录里的完整 ID）`,
		`点击连接/校验后，即可在模型下拉中切换到该模型`,
	];

	const content = steps.map((s, i) => `${i + 1}. ${s}`).join("\n");

	return {
		files: [{ path: "Trae 内操作步骤", content, language: "markdown" }],
		steps,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

function generateZCode(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";

	const steps = [
		`打开 ZCode → 设置 → 模型接入`,
		`选择「自定义 OpenAI 兼容接口」`,
		`Base URL 填 ${baseURL}（含 /v1）`,
		input.apiKey ? `API Key 填 ${input.apiKey}` : `API Key 留空（celer-route 未开启强制鉴权）`,
		`Model ID 填 ${defaultModelId}（模型目录里的完整 ID）`,
		`保存后即可在模型列表中切换到该模型`,
	];

	const content = steps.map((s, i) => `${i + 1}. ${s}`).join("\n");

	return {
		files: [{ path: "ZCode 内操作步骤", content, language: "markdown" }],
		steps,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

function generateMarsCode(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";

	const entries: Array<[string, string]> = [
		["OPENAI_BASE_URL", baseURL],
		...(input.apiKey ? ([["OPENAI_API_KEY", input.apiKey]] as Array<[string, string]>) : []),
		["OPENAI_MODEL", defaultModelId],
	];

	return generateEnvRecipe(input, entries);
}

function generateLingma(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";

	const steps = [
		`安装通义灵码插件（VS Code / JetBrains 扩展市场搜索「通义灵码」）`,
		`打开设置 → 模型服务 → 自定义端点`,
		`协议选择「OpenAI 兼容（Chat Completions）」`,
		`Base URL 填 ${baseURL}（含 /v1）`,
		input.apiKey ? `API Key 填 ${input.apiKey}` : `API Key 留空（celer-route 未开启强制鉴权）`,
		`模型（Model）填 ${defaultModelId}（模型目录里的完整 ID）`,
		`保存并重载窗口后生效`,
	];

	const content = steps.map((s, i) => `${i + 1}. ${s}`).join("\n");

	return {
		files: [{ path: "通义灵码 内操作步骤", content, language: "markdown" }],
		steps,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

/**
 * Category groups for the agent dropdown. Order is display order; the group
 * labels live in the i18n `agent.group.*` keys.
 */
export type AgentGroupId = "coding" | "domestic" | "ide" | "generic";

export interface AgentGroup {
	id: AgentGroupId;
	agents: CodingAgentId[];
}

export const AGENT_GROUPS: AgentGroup[] = [
	{ id: "coding", agents: ["opencode", "claude-code", "codex", "marscode"] },
	{ id: "domestic", agents: ["workbuddy", "zcode"] },
	{ id: "ide", agents: ["codebuddy", "cursor", "trae", "lingma"] },
	{ id: "generic", agents: ["openai-compatible"] },
];

export const CODING_AGENTS: CodingAgentId[] = AGENT_GROUPS.flatMap((g) => g.agents);

/**
 * Generate the ready-to-paste config for an AI client.
 * Pure — no I/O, no side effects.
 */
export function generateAgentConfig(input: AgentConfigInput): AgentConfigOutput {
	switch (input.agent) {
		case "opencode":
			return generateOpenCode(input);
		case "claude-code":
			return generateClaudeCode(input);
		case "codex":
			return generateCodex(input);
		case "openai-compatible":
			return generateOpenAICompatible(input);
		case "cursor":
			return generateCursor(input);
		case "workbuddy":
			return generateWorkBuddy(input);
		case "codebuddy":
			return generateCodeBuddy(input);
		case "trae":
			return generateTrae(input);
		case "zcode":
			return generateZCode(input);
		case "marscode":
			return generateMarsCode(input);
		case "lingma":
			return generateLingma(input);
	}
}