import { describe, expect, it } from "vitest";

import {
	AGENT_GROUPS,
	AGENT_PROVIDER_KEY,
	CODING_AGENTS,
	CodingAgentId,
	envTabCode,
	generateAgentConfig,
	toAnthropicSurface,
	toOpenAISurface,
} from "@/lib/utils/agentConfigs";

const models = [
	{ id: "minimax/MiniMax-M2.1", name: "MiniMax-M2.1", contextLength: 1_000_000, maxOutputTokens: 8192 },
	{ id: "sensenova/glm-5.2", name: "glm-5.2" },
	{ id: "opencode/big-pickle", name: "big-pickle" },
];

function parseJSON(content: string): Record<string, any> {
	return JSON.parse(content);
}

describe("surface derivation", () => {
	it("appends /v1 to a bare origin", () => {
		expect(toOpenAISurface("http://localhost:8080")).toBe("http://localhost:8080/v1");
	});

	it("does not double the /v1 suffix", () => {
		expect(toOpenAISurface("http://localhost:8080/v1")).toBe("http://localhost:8080/v1");
		expect(toOpenAISurface("http://localhost:8080/v1/")).toBe("http://localhost:8080/v1");
	});

	it("derives the anthropic surface from either form", () => {
		expect(toAnthropicSurface("http://localhost:8080")).toBe("http://localhost:8080/anthropic");
		expect(toAnthropicSurface("http://localhost:8080/v1")).toBe("http://localhost:8080/anthropic");
	});
});

describe("opencode", () => {
	it("generates a valid strict-JSON config with models + limits", () => {
		const out = generateAgentConfig({ agent: "opencode", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		expect(out.files).toHaveLength(1);
		expect(out.files[0].path).toBe("~/.config/opencode/opencode.json");
		expect(out.defaultModelRef).toBe("celer-route/minimax/MiniMax-M2.1");

		const cfg = parseJSON(out.files[0].content);
		expect(cfg.$schema).toBe("https://opencode.ai/config.json");
		expect(cfg.model).toBe("celer-route/minimax/MiniMax-M2.1");
		expect(cfg.provider["celer-route"].npm).toBe("@ai-sdk/openai-compatible");
		expect(cfg.provider["celer-route"].options.baseURL).toBe("http://localhost:8080/v1");
		expect(cfg.provider["celer-route"].options.apiKey).toBe("sk-bf-abc");

		const m = cfg.provider["celer-route"].models;
		expect(m["minimax/MiniMax-M2.1"]).toEqual({ name: "MiniMax-M2.1", limit: { context: 1_000_000, output: 8192 } });
		expect(m["sensenova/glm-5.2"]).toEqual({ name: "glm-5.2" });
	});

	it("honors an explicit default model + responses protocol", () => {
		const out = generateAgentConfig({
			agent: "opencode",
			baseUrl: "http://localhost:8080/v1",
			apiKey: null,
			models,
			defaultModelId: "opencode/big-pickle",
			protocol: "responses",
		});
		const cfg = parseJSON(out.files[0].content);
		expect(cfg.model).toBe("celer-route/opencode/big-pickle");
		expect(cfg.provider["celer-route"].npm).toBe("@ai-sdk/openai");
		expect(cfg.provider["celer-route"].options.apiKey).toBeUndefined();
	});

	it("omits models block cleanly when no models are selected (no trailing commas)", () => {
		const out = generateAgentConfig({ agent: "opencode", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models: [] });
		const cfg = parseJSON(out.files[0].content);
		expect(cfg.provider["celer-route"].models).toBeUndefined();
		expect(cfg.model).toBeUndefined();
		expect(out.modelIds).toEqual([]);
	});
});

describe("claude-code", () => {
	it("writes the anthropic surface into settings.json and env recipe", () => {
		const out = generateAgentConfig({ agent: "claude-code", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		const settings = parseJSON(out.files[0].content);
		expect(settings.env.ANTHROPIC_BASE_URL).toBe("http://localhost:8080/anthropic");
		expect(settings.env.ANTHROPIC_AUTH_TOKEN).toBe("sk-bf-abc");
		expect(settings.env.ANTHROPIC_MODEL).toBe("minimax/MiniMax-M2.1");
		expect(out.env!.posix).toContain("export ANTHROPIC_BASE_URL=http://localhost:8080/anthropic");
	});

	it("omits auth env line when apiKey is null", () => {
		const out = generateAgentConfig({ agent: "claude-code", baseUrl: "http://localhost:8080", apiKey: null, models });
		const settings = parseJSON(out.files[0].content);
		expect(settings.env.ANTHROPIC_AUTH_TOKEN).toBeUndefined();
	});
});

describe("codex", () => {
	it("writes a config.toml with the celer-route provider", () => {
		const out = generateAgentConfig({ agent: "codex", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		const toml = out.files[0].content;
		expect(out.files[0].path).toBe("~/.codex/config.toml");
		expect(toml).toContain('model = "minimax/MiniMax-M2.1"');
		expect(toml).toContain('model_provider = "celer-route"');
		expect(toml).toContain("[model_providers.celer-route]");
		expect(toml).toContain('base_url = "http://localhost:8080/v1"');
		expect(toml).toContain('env_key = "CELER_ROUTE_API_KEY"');
		expect(out.env!.posix).toEqual(["export CELER_ROUTE_API_KEY=sk-bf-abc"]);
	});
});

describe("openai-compatible", () => {
	it("emits OPENAI_* env recipe for generic agents (hermes/openclaw)", () => {
		const out = generateAgentConfig({ agent: "openai-compatible", baseUrl: "http://localhost:8080/v1", apiKey: "sk-bf-abc", models });
		const content = out.files[0].content;
		expect(content).toContain("export OPENAI_BASE_URL=http://localhost:8080/v1");
		expect(content).toContain("export OPENAI_API_KEY=sk-bf-abc");
		expect(content).toContain("export OPENAI_MODEL=minimax/MiniMax-M2.1");
		expect(out.defaultModelRef).toBe("minimax/MiniMax-M2.1");
	});
});

describe("cursor", () => {
	it("produces in-app steps, not a config file", () => {
		const out = generateAgentConfig({ agent: "cursor", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		expect(out.steps).toBeDefined();
		expect(out.steps!.length).toBeGreaterThan(0);
		expect(out.files[0].path).toBe("Cursor 内操作步骤");
		const joined = out.files[0].content;
		expect(joined).toContain("http://localhost:8080/v1");
		expect(joined).toContain("minimax/MiniMax-M2.1");
	});
});

describe("workbuddy", () => {
	it("writes a models.json with OpenAI-compatible chat-completions URL", () => {
		const out = generateAgentConfig({ agent: "workbuddy", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		expect(out.files[0].path).toBe("~/.workbuddy/models.json");
		expect(out.defaultModelRef).toBe("minimax/MiniMax-M2.1");
		const doc = parseJSON(out.files[0].content);
		expect(doc.availableModels).toEqual(["minimax/MiniMax-M2.1", "sensenova/glm-5.2", "opencode/big-pickle"]);
		const [first, second] = doc.models;
		expect(first).toEqual({
			id: "minimax/MiniMax-M2.1",
			name: "MiniMax-M2.1",
			vendor: "OpenAI",
			url: "http://localhost:8080/v1/chat/completions",
			apiKey: "sk-bf-abc",
			maxInputTokens: 1_000_000,
			maxOutputTokens: 8192,
		});
		expect(second).toEqual({
			id: "sensenova/glm-5.2",
			name: "glm-5.2",
			vendor: "OpenAI",
			url: "http://localhost:8080/v1/chat/completions",
			apiKey: "sk-bf-abc",
		});
	});

	it("hoists the default model to the front of availableModels", () => {
		const out = generateAgentConfig({
			agent: "workbuddy",
			baseUrl: "http://localhost:8080",
			apiKey: "sk-bf-abc",
			models,
			defaultModelId: "opencode/big-pickle",
		});
		const doc = parseJSON(out.files[0].content);
		expect(doc.availableModels).toEqual(["opencode/big-pickle", "sensenova/glm-5.2", "minimax/MiniMax-M2.1"]);
		expect(doc.models[0].id).toBe("opencode/big-pickle");
	});

	it("omits apiKey when auth is disabled", () => {
		const out = generateAgentConfig({ agent: "workbuddy", baseUrl: "http://localhost:8080", apiKey: null, models });
		const doc = parseJSON(out.files[0].content);
		expect(doc.models[0].apiKey).toBeUndefined();
	});
});

describe("codebuddy", () => {
	it("writes ~/.codebuddy/models.json with the same shape", () => {
		const out = generateAgentConfig({ agent: "codebuddy", baseUrl: "http://localhost:8080/v1", apiKey: "sk-bf-abc", models });
		expect(out.files[0].path).toBe("~/.codebuddy/models.json");
		const doc = parseJSON(out.files[0].content);
		expect(doc.models[0].url).toBe("http://localhost:8080/v1/chat/completions");
		expect(doc.availableModels).toHaveLength(models.length);
	});
});

describe("trae", () => {
	it("produces in-app steps with the full /v1 base URL", () => {
		const out = generateAgentConfig({ agent: "trae", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		expect(out.steps!.length).toBeGreaterThan(0);
		const joined = out.files[0].content;
		expect(joined).toContain("http://localhost:8080/v1");
		expect(joined).toContain("sk-bf-abc");
		expect(joined).toContain("minimax/MiniMax-M2.1");
	});
});

describe("zcode", () => {
	it("produces in-app steps for a custom OpenAI-compatible endpoint", () => {
		const out = generateAgentConfig({ agent: "zcode", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		expect(out.steps!.length).toBeGreaterThan(0);
		const joined = out.files[0].content;
		expect(joined).toContain("http://localhost:8080/v1");
		expect(joined).toContain("OpenAI");
	});
});

describe("marscode", () => {
	it("emits the OPENAI_* env recipe", () => {
		const out = generateAgentConfig({ agent: "marscode", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		const content = out.files[0].content;
		expect(content).toContain("export OPENAI_BASE_URL=http://localhost:8080/v1");
		expect(content).toContain("export OPENAI_API_KEY=sk-bf-abc");
		expect(content).toContain("export OPENAI_MODEL=minimax/MiniMax-M2.1");
	});
});

describe("lingma", () => {
	it("produces Tongyi Lingma in-app steps", () => {
		const out = generateAgentConfig({ agent: "lingma", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		expect(out.steps!.length).toBeGreaterThan(0);
		const joined = out.files[0].content;
		expect(joined).toContain("通义灵码");
		expect(joined).toContain("http://localhost:8080/v1");
		expect(joined).toContain("sk-bf-abc");
	});
});

describe("platforms", () => {
	it("renders Windows config paths with %USERPROFILE% and backslashes", () => {
		expect(
			generateAgentConfig({ agent: "opencode", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "windows" }).files[0].path,
		).toBe("%USERPROFILE%\\.config\\opencode\\opencode.json");
		expect(
			generateAgentConfig({ agent: "claude-code", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "windows" }).files[0]
				.path,
		).toBe("%USERPROFILE%\\.claude\\settings.json");
		expect(
			generateAgentConfig({ agent: "codex", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "windows" }).files[0].path,
		).toBe("%USERPROFILE%\\.codex\\config.toml");
		expect(
			generateAgentConfig({ agent: "workbuddy", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "windows" }).files[0].path,
		).toBe("%USERPROFILE%\\.workbuddy\\models.json");
		expect(
			generateAgentConfig({ agent: "codebuddy", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "windows" }).files[0].path,
		).toBe("%USERPROFILE%\\.codebuddy\\models.json");
	});

	it("keeps POSIX paths on macOS and Linux", () => {
		expect(
			generateAgentConfig({ agent: "opencode", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "macos" }).files[0].path,
		).toBe("~/.config/opencode/opencode.json");
		expect(
			generateAgentConfig({ agent: "claude-code", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "linux" }).files[0].path,
		).toBe("~/.claude/settings.json");
	});

	it("emits PowerShell and cmd env lines for Windows", () => {
		const out = generateAgentConfig({
			agent: "claude-code",
			baseUrl: "http://localhost:8080",
			apiKey: "sk-bf-abc",
			models,
			platform: "windows",
		});
		expect(out.env!.posix).toContain("export ANTHROPIC_BASE_URL=http://localhost:8080/anthropic");
		expect(out.env!.powershell).toContain('$env:ANTHROPIC_BASE_URL = "http://localhost:8080/anthropic"');
		expect(out.env!.cmd).toContain("set ANTHROPIC_BASE_URL=http://localhost:8080/anthropic");
	});

	it("renders the settings.json env identically on every platform", () => {
		const mac = generateAgentConfig({ agent: "claude-code", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "macos" });
		const win = generateAgentConfig({ agent: "claude-code", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "windows" });
		expect(mac.files[0].content).toBe(win.files[0].content);
	});

	it("adapts the Cursor settings shortcut to the platform", () => {
		expect(
			generateAgentConfig({ agent: "cursor", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "macos" }).files[0].content,
		).toContain("Settings（⌘,）");
		expect(
			generateAgentConfig({ agent: "cursor", baseUrl: "http://localhost:8080", apiKey: "k", models, platform: "windows" }).files[0].content,
		).toContain("Settings（Ctrl+,）");
	});

	it("envTabCode layers PowerShell + cmd blocks on Windows and plain exports on POSIX", () => {
		const env = generateAgentConfig({
			agent: "openai-compatible",
			baseUrl: "http://localhost:8080",
			apiKey: "k",
			models,
			platform: "windows",
		}).env!;
		const windows = envTabCode(env, "windows");
		expect(windows).toContain("# PowerShell:");
		expect(windows).toContain('$env:OPENAI_BASE_URL = "http://localhost:8080/v1"');
		expect(windows).toContain("# cmd:");
		expect(windows).toContain("set OPENAI_BASE_URL=http://localhost:8080/v1");
		expect(envTabCode(env, "linux")).toContain("export OPENAI_BASE_URL=http://localhost:8080/v1");
	});

	it("defaults to the linux (POSIX) flavor when platform is omitted", () => {
		const out = generateAgentConfig({ agent: "codex", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		expect(out.files[0].path).toBe("~/.codex/config.toml");
		expect(out.env!.posix).toEqual(["export CELER_ROUTE_API_KEY=sk-bf-abc"]);
		expect(out.env!.powershell).toEqual(['$env:CELER_ROUTE_API_KEY = "sk-bf-abc"']);
	});
});

describe("agent groups", () => {
	it("covers every agent id exactly once", () => {
		const all = AGENT_GROUPS.flatMap((g) => g.agents);
		expect(new Set(all).size).toBe(all.length);
		expect(new Set(all).size).toBe(CODING_AGENTS.length);
	});
});

describe("generateAgentConfig", () => {
	it("returns modelIds mirroring the input order", () => {
		const out = generateAgentConfig({ agent: "codex", baseUrl: "http://localhost:8080", apiKey: "sk-bf-abc", models });
		expect(out.modelIds).toEqual(["minimax/MiniMax-M2.1", "sensenova/glm-5.2", "opencode/big-pickle"]);
	});

	it("falls back to the first model when defaultModelId is unknown", () => {
		const out = generateAgentConfig({
			agent: "codex",
			baseUrl: "http://localhost:8080",
			apiKey: "sk-bf-abc",
			models,
			defaultModelId: "does/not-exist",
		});
		expect(out.defaultModelRef).toBe("minimax/MiniMax-M2.1");
	});

	it("handles every agent id without throwing", () => {
		const agents: CodingAgentId[] = [
			"opencode",
			"claude-code",
			"codex",
			"openai-compatible",
			"cursor",
			"workbuddy",
			"codebuddy",
			"trae",
			"zcode",
			"marscode",
			"lingma",
		];
		for (const agent of agents) {
			expect(() => generateAgentConfig({ agent, baseUrl: "http://localhost:8080", apiKey: null, models })).not.toThrow();
		}
	});

	it("uses the shared provider key consistently", () => {
		expect(AGENT_PROVIDER_KEY).toBe("celer-route");
	});
});