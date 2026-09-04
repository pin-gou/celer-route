// Agent config generation engine.
//
// celer-route exposes OpenAI-compatible (/v1), Anthropic-compatible
// (/anthropic/v1/messages) and GenAI surfaces. Coding agents never discover
// models themselves — every agent requires the model list to be written
// explicitly into its own config file. This module turns the live model
// catalog (as returned by the celer-route API) + an endpoint + a virtual key
// into ready-to-paste config for the supported coding agents.
//
// The functions here are pure: they take plain data and return plain strings.
// The Web UI (workspace/agent-setup) and the future `celer-route setup-*`
// CLI both consume them so both surfaces emit byte-identical config.

export type CodingAgentId = "opencode" | "claude-code" | "codex" | "openai-compatible" | "cursor";

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
}

export interface AgentConfigFile {
	/** Human-friendly path shown to the user, e.g. "~/.config/opencode/opencode.json". */
	path: string;
	content: string;
	language: "json" | "toml" | "shell" | "markdown";
}

export interface AgentConfigOutput {
	files: AgentConfigFile[];
	/** Optional env-var recipe (export lines) for tools that read the key from the environment. */
	env?: string[];
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

function generateOpenCode(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const models = input.models;
	const defaultModelId = pickDefaultModelId(models, input.defaultModelId) ?? "";
	const defaultModelRef = `${AGENT_PROVIDER_KEY}/${defaultModelId}`;
	const npm = input.protocol === "responses" ? "@ai-sdk/openai" : "@ai-sdk/openai-compatible";

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
		files: [{ path: "~/.config/opencode/opencode.json", content, language: "json" }],
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
	const env: string[] = [
		`export ANTHROPIC_BASE_URL=${baseURL}`,
		...((input.apiKey && [`export ANTHROPIC_AUTH_TOKEN=${input.apiKey}`]) || []),
		...((defaultModelId && [`export ANTHROPIC_MODEL=${defaultModelId}`]) || []),
	];
	const settings: Record<string, Record<string, string>> = { env: {} };
	for (const line of env) {
		const eq = line.indexOf("=");
		if (eq === -1) continue;
		settings.env[line.slice(7, eq)] = line.slice(eq + 1);
	}

	const content = JSON.stringify(settings, null, 2);

	return {
		files: [{ path: "~/.claude/settings.json", content, language: "json" }],
		env,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

function generateCodex(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";
	const envKey = "CELER_ROUTE_API_KEY";

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
		files: [{ path: "~/.codex/config.toml", content, language: "toml" }],
		env: input.apiKey ? [`export ${envKey}=${input.apiKey}`] : undefined,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

function generateOpenAICompatible(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";

	const env: string[] = [
		`export OPENAI_BASE_URL=${baseURL}`,
		...(input.apiKey ? [`export OPENAI_API_KEY=${input.apiKey}`] : []),
		`export OPENAI_MODEL=${defaultModelId}`,
	];

	return {
		files: [
			{
				path: ".env (环境变量接入)",
				content: env.join("\n"),
				language: "shell",
			},
		],
		env,
		defaultModelRef: defaultModelId,
		modelIds: input.models.map((m) => m.id),
	};
}

function generateCursor(input: AgentConfigInput): AgentConfigOutput {
	const baseURL = toOpenAISurface(input.baseUrl);
	const defaultModelId = pickDefaultModelId(input.models, input.defaultModelId) ?? "";
	const providerName = AGENT_PROVIDER_KEY;

	const steps = [
		`打开 Cursor → Settings（⌘,) → Models`,
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

export const CODING_AGENTS: CodingAgentId[] = ["opencode", "claude-code", "codex", "openai-compatible", "cursor"];

/**
 * Generate the ready-to-paste config for a coding agent.
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
	}
}