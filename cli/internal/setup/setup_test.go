package setup

import (
	"encoding/json"
	"strings"
	"testing"
)

// fixtureModels returns the same three-model fixture used by the
// TypeScript engine's tests so byte-for-byte parity is easy to audit.
func fixtureModels() []Model {
	return []Model{
		{ID: "minimax/MiniMax-M2.1", Name: "MiniMax-M2.1", ContextLength: 1_000_000, MaxOutput: 8192},
		{ID: "sensenova/glm-5.2", Name: "glm-5.2"},
		{ID: "opencode/big-pickle", Name: "big-pickle"},
	}
}

func TestSurfaceDerivation(t *testing.T) {
	if got := StripV1Suffix("http://localhost:8080"); got != "http://localhost:8080" {
		t.Fatalf("StripV1Suffix plain: %q", got)
	}
	if got := StripV1Suffix("http://localhost:8080/"); got != "http://localhost:8080" {
		t.Fatalf("StripV1Suffix trailing slash: %q", got)
	}
	if got := StripV1Suffix("http://localhost:8080/v1"); got != "http://localhost:8080" {
		t.Fatalf("StripV1Suffix /v1: %q", got)
	}
	if got := OpenAISurface("http://localhost:8080"); got != "http://localhost:8080/v1" {
		t.Errorf("OpenAISurface bare: %q", got)
	}
	if got := OpenAISurface("http://localhost:8080/v1"); got != "http://localhost:8080/v1" {
		t.Errorf("OpenAISurface with /v1: %q", got)
	}
	if got := AnthropicSurface("http://localhost:8080"); got != "http://localhost:8080/anthropic" {
		t.Errorf("AnthropicSurface bare: %q", got)
	}
	if got := AnthropicSurface("http://localhost:8080/v1"); got != "http://localhost:8080/anthropic" {
		t.Errorf("AnthropicSurface from /v1: %q", got)
	}
}

func TestRenderOpencode(t *testing.T) {
	out, err := RenderOpencode(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderOpencode: %v", err)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "~/.config/opencode/opencode.json" {
		t.Fatalf("unexpected files: %+v", out.Files)
	}
	if out.DefaultModel != "celer-route/minimax/MiniMax-M2.1" {
		t.Errorf("default model: %q", out.DefaultModel)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(out.Files[0].Content), &cfg); err != nil {
		t.Fatalf("opencode config is not valid JSON: %v", err)
	}
	if cfg["$schema"] != "https://opencode.ai/config.json" {
		t.Errorf("$schema: %v", cfg["$schema"])
	}
	if cfg["model"] != "celer-route/minimax/MiniMax-M2.1" {
		t.Errorf("model: %v", cfg["model"])
	}
	provider := cfg["provider"].(map[string]any)["celer-route"].(map[string]any)
	if provider["npm"] != "@ai-sdk/openai-compatible" {
		t.Errorf("npm: %v", provider["npm"])
	}
	options := provider["options"].(map[string]any)
	if options["baseURL"] != "http://localhost:8080/v1" {
		t.Errorf("baseURL: %v", options["baseURL"])
	}
	if options["apiKey"] != "sk-bf-abc" {
		t.Errorf("apiKey: %v", options["apiKey"])
	}
	models := provider["models"].(map[string]any)
	mm := models["minimax/MiniMax-M2.1"].(map[string]any)
	if mm["name"] != "MiniMax-M2.1" {
		t.Errorf("model name: %v", mm["name"])
	}
	limit := mm["limit"].(map[string]any)
	if int(limit["context"].(float64)) != 1_000_000 || int(limit["output"].(float64)) != 8192 {
		t.Errorf("model limits: %v", limit)
	}
}

func TestRenderOpencodeProtocolResponses(t *testing.T) {
	out, err := RenderOpencode(Input{
		BaseURL:        "http://localhost:8080/v1",
		APIKey:         "",
		Models:         fixtureModels(),
		DefaultModelID: "opencode/big-pickle",
		Protocol:       "responses",
	})
	if err != nil {
		t.Fatalf("RenderOpencode: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out.Files[0].Content), &cfg); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if cfg["model"] != "celer-route/opencode/big-pickle" {
		t.Errorf("model: %v", cfg["model"])
	}
	provider := cfg["provider"].(map[string]any)["celer-route"].(map[string]any)
	if provider["npm"] != "@ai-sdk/openai" {
		t.Errorf("npm: %v", provider["npm"])
	}
	options := provider["options"].(map[string]any)
	if _, has := options["apiKey"]; has {
		t.Errorf("apiKey should be omitted when empty, got: %v", options["apiKey"])
	}
}

func TestRenderOpencodeEmptyModels(t *testing.T) {
	_, err := RenderOpencode(Input{BaseURL: "http://localhost:8080", APIKey: "k", Models: nil})
	if err != ErrNoModels {
		t.Fatalf("expected ErrNoModels, got %v", err)
	}
}

func TestRenderClaudeCode(t *testing.T) {
	out, err := RenderClaudeCode(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderClaudeCode: %v", err)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "~/.claude/settings.json" {
		t.Fatalf("unexpected files: %+v", out.Files)
	}
	if out.DefaultModel != "minimax/MiniMax-M2.1" {
		t.Errorf("default model: %q", out.DefaultModel)
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(out.Files[0].Content), &settings); err != nil {
		t.Fatalf("claude settings not JSON: %v", err)
	}
	env := settings["env"].(map[string]any)
	if env["ANTHROPIC_BASE_URL"] != "http://localhost:8080/anthropic" {
		t.Errorf("ANTHROPIC_BASE_URL: %v", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "sk-bf-abc" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN: %v", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_MODEL"] != "minimax/MiniMax-M2.1" {
		t.Errorf("ANTHROPIC_MODEL: %v", env["ANTHROPIC_MODEL"])
	}

	// Env recipe should mirror settings.json.
	if !strings.Contains(strings.Join(out.Env.Posix, "\n"), "export ANTHROPIC_BASE_URL=http://localhost:8080/anthropic") {
		t.Errorf("env recipe missing base URL: %v", out.Env)
	}
}

func TestRenderClaudeCodeNoAPIKey(t *testing.T) {
	out, err := RenderClaudeCode(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderClaudeCode: %v", err)
	}
	var settings map[string]any
	_ = json.Unmarshal([]byte(out.Files[0].Content), &settings)
	env := settings["env"].(map[string]any)
	if _, has := env["ANTHROPIC_AUTH_TOKEN"]; has {
		t.Errorf("ANTHROPIC_AUTH_TOKEN should be omitted when no key: %v", env)
	}
}

func TestRenderCodex(t *testing.T) {
	out, err := RenderCodex(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderCodex: %v", err)
	}
	toml := out.Files[0].Content
	if !strings.Contains(toml, `model = "minimax/MiniMax-M2.1"`) {
		t.Errorf("model not in TOML: %s", toml)
	}
	if !strings.Contains(toml, `model_provider = "celer-route"`) {
		t.Errorf("model_provider: %s", toml)
	}
	if !strings.Contains(toml, `[model_providers.celer-route]`) {
		t.Errorf("provider table: %s", toml)
	}
	if !strings.Contains(toml, `base_url = "http://localhost:8080/v1"`) {
		t.Errorf("base_url: %s", toml)
	}
	if !strings.Contains(toml, `env_key = "CELER_ROUTE_API_KEY"`) {
		t.Errorf("env_key: %s", toml)
	}
	if len(out.Env.Posix) != 1 || !strings.Contains(out.Env.Posix[0], "CELER_ROUTE_API_KEY=sk-bf-abc") {
		t.Errorf("env recipe: %v", out.Env)
	}
}

func TestRenderOpenAICompatible(t *testing.T) {
	out, err := RenderOpenAICompatible(Input{
		BaseURL: "http://localhost:8080/v1",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderOpenAICompatible: %v", err)
	}
	content := out.Files[0].Content
	for _, want := range []string{
		"export OPENAI_BASE_URL=http://localhost:8080/v1",
		"export OPENAI_API_KEY=sk-bf-abc",
		"export OPENAI_MODEL=minimax/MiniMax-M2.1",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in: %s", want, content)
		}
	}
}

func TestRenderCursor(t *testing.T) {
	out, err := RenderCursor(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderCursor: %v", err)
	}
	if len(out.Steps) == 0 {
		t.Errorf("expected cursor steps: %+v", out)
	}
	joined := out.Files[0].Content
	if !strings.Contains(joined, "http://localhost:8080/v1") {
		t.Errorf("base URL not in steps: %s", joined)
	}
	if !strings.Contains(joined, "sk-bf-abc") {
		t.Errorf("api key not in steps: %s", joined)
	}
	if !strings.Contains(joined, "minimax/MiniMax-M2.1") {
		t.Errorf("default model not in steps: %s", joined)
	}
}

func TestRenderWorkBuddy(t *testing.T) {
	out, err := RenderWorkBuddy(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderWorkBuddy: %v", err)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "~/.workbuddy/models.json" {
		t.Fatalf("unexpected files: %+v", out.Files)
	}
	if out.DefaultModel != "minimax/MiniMax-M2.1" {
		t.Errorf("default model: %q", out.DefaultModel)
	}

	var doc struct {
		Models []struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Vendor          string `json:"vendor"`
			URL             string `json:"url"`
			APIKey          string `json:"apiKey"`
			MaxInputTokens  int    `json:"maxInputTokens"`
			MaxOutputTokens int    `json:"maxOutputTokens"`
		} `json:"models"`
		AvailableModels []string `json:"availableModels"`
	}
	if err := json.Unmarshal([]byte(out.Files[0].Content), &doc); err != nil {
		t.Fatalf("models.json not valid JSON: %v", err)
	}
	if len(doc.Models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(doc.Models))
	}
	if doc.Models[0].ID != "minimax/MiniMax-M2.1" || doc.Models[0].Vendor != "OpenAI" {
		t.Errorf("first model: %+v", doc.Models[0])
	}
	if doc.Models[0].URL != "http://localhost:8080/v1/chat/completions" {
		t.Errorf("url: %v", doc.Models[0].URL)
	}
	if doc.Models[0].APIKey != "sk-bf-abc" {
		t.Errorf("apiKey: %v", doc.Models[0].APIKey)
	}
	if doc.Models[0].MaxInputTokens != 1_000_000 || doc.Models[0].MaxOutputTokens != 8192 {
		t.Errorf("limits: %+v", doc.Models[0])
	}
	if got := strings.Join(doc.AvailableModels, ","); got != "minimax/MiniMax-M2.1,sensenova/glm-5.2,opencode/big-pickle" {
		t.Errorf("availableModels: %q", got)
	}
}

func TestRenderWorkBuddyDefaultHoisted(t *testing.T) {
	out, err := RenderWorkBuddy(Input{
		BaseURL:        "http://localhost:8080",
		APIKey:         "sk-bf-abc",
		Models:         fixtureModels(),
		DefaultModelID: "opencode/big-pickle",
	})
	if err != nil {
		t.Fatalf("RenderWorkBuddy: %v", err)
	}
	var doc struct {
		Models          []tencentModelEntry `json:"models"`
		AvailableModels []string            `json:"availableModels"`
	}
	if err := json.Unmarshal([]byte(out.Files[0].Content), &doc); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(doc.AvailableModels) == 0 || doc.AvailableModels[0] != "opencode/big-pickle" {
		t.Errorf("default not hoisted: %v", doc.AvailableModels)
	}
	if len(doc.Models) == 0 || doc.Models[0].ID != "opencode/big-pickle" {
		t.Errorf("first entry: %+v", doc.Models)
	}
}

func TestRenderCodeBuddy(t *testing.T) {
	out, err := RenderCodeBuddy(Input{
		BaseURL: "http://localhost:8080/v1",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderCodeBuddy: %v", err)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "~/.codebuddy/models.json" {
		t.Fatalf("unexpected files: %+v", out.Files)
	}
	if !strings.Contains(out.Files[0].Content, "http://localhost:8080/v1/chat/completions") {
		t.Errorf("url missing in: %s", out.Files[0].Content)
	}
}

func TestRenderTrae(t *testing.T) {
	out, err := RenderTrae(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderTrae: %v", err)
	}
	joined := out.Files[0].Content
	for _, want := range []string{
		"http://localhost:8080/v1",
		"sk-bf-abc",
		"minimax/MiniMax-M2.1",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in: %s", want, joined)
		}
	}
}

func TestRenderZCode(t *testing.T) {
	out, err := RenderZCode(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderZCode: %v", err)
	}
	if len(out.Steps) == 0 {
		t.Errorf("expected zcode steps: %+v", out)
	}
	joined := out.Files[0].Content
	if !strings.Contains(joined, "http://localhost:8080/v1") || !strings.Contains(joined, "OpenAI") {
		t.Errorf("zcode steps missing endpoint/protocol: %s", joined)
	}
}

func TestRenderMarsCode(t *testing.T) {
	out, err := RenderMarsCode(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderMarsCode: %v", err)
	}
	content := out.Files[0].Content
	for _, want := range []string{
		"export OPENAI_BASE_URL=http://localhost:8080/v1",
		"export OPENAI_API_KEY=sk-bf-abc",
		"export OPENAI_MODEL=minimax/MiniMax-M2.1",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in: %s", want, content)
		}
	}
}

func TestRenderLingma(t *testing.T) {
	out, err := RenderLingma(Input{
		BaseURL: "http://localhost:8080",
		APIKey:  "sk-bf-abc",
		Models:  fixtureModels(),
	})
	if err != nil {
		t.Fatalf("RenderLingma: %v", err)
	}
	joined := out.Files[0].Content
	for _, want := range []string{
		"通义灵码",
		"http://localhost:8080/v1",
		"sk-bf-abc",
		"minimax/MiniMax-M2.1",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in: %s", want, joined)
		}
	}
}

func TestRenderPlatformWindowsPaths(t *testing.T) {
	windows := Input{BaseURL: "http://localhost:8080", APIKey: "k", Models: fixtureModels(), Platform: PlatformWindows}
	cases := []struct {
		agent Agent
		want  string
	}{
		{Opencode, `%USERPROFILE%\.config\opencode\opencode.json`},
		{ClaudeCode, `%USERPROFILE%\.claude\settings.json`},
		{Codex, `%USERPROFILE%\.codex\config.toml`},
		{WorkBuddy, `%USERPROFILE%\.workbuddy\models.json`},
		{CodeBuddy, `%USERPROFILE%\.codebuddy\models.json`},
	}
	for _, tc := range cases {
		out, err := Dispatch(tc.agent, windows)
		if err != nil {
			t.Fatalf("Dispatch(%s): %v", tc.agent, err)
		}
		if out.Files[0].Path != tc.want {
			t.Errorf("%s path: got %q want %q", tc.agent, out.Files[0].Path, tc.want)
		}
	}
}

func TestRenderPlatformWindowsEnv(t *testing.T) {
	out, err := RenderClaudeCode(Input{
		BaseURL:  "http://localhost:8080",
		APIKey:   "sk-bf-abc",
		Models:   fixtureModels(),
		Platform: PlatformWindows,
	})
	if err != nil {
		t.Fatalf("RenderClaudeCode: %v", err)
	}
	env := out.Env
	if !contains(env.Posix, "export ANTHROPIC_BASE_URL=http://localhost:8080/anthropic") {
		t.Errorf("posix: %v", env.Posix)
	}
	if !contains(env.PowerShell, `$env:ANTHROPIC_BASE_URL = "http://localhost:8080/anthropic"`) {
		t.Errorf("powershell: %v", env.PowerShell)
	}
	if !contains(env.Cmd, "set ANTHROPIC_BASE_URL=http://localhost:8080/anthropic") {
		t.Errorf("cmd: %v", env.Cmd)
	}

	// settings.json content must be identical across platforms.
	mac, err := RenderClaudeCode(Input{BaseURL: "http://localhost:8080", APIKey: "sk-bf-abc", Models: fixtureModels(), Platform: PlatformMacOS})
	if err != nil {
		t.Fatalf("RenderClaudeCode mac: %v", err)
	}
	if mac.Files[0].Content != out.Files[0].Content {
		t.Errorf("settings.json platform drift:\n%q\n!=\n%q", mac.Files[0].Content, out.Files[0].Content)
	}
}

func TestRenderOpenAICompatibleWindows(t *testing.T) {
	out, err := RenderOpenAICompatible(Input{
		BaseURL:  "http://localhost:8080",
		APIKey:   "sk-bf-abc",
		Models:   fixtureModels(),
		Platform: PlatformWindows,
	})
	if err != nil {
		t.Fatalf("RenderOpenAICompatible: %v", err)
	}
	content := out.Files[0].Content
	for _, want := range []string{"# PowerShell:", `$env:OPENAI_BASE_URL = "http://localhost:8080/v1"`, "# cmd:", "set OPENAI_BASE_URL=http://localhost:8080/v1"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in: %s", want, content)
		}
	}
}

func TestRenderCursorShortcut(t *testing.T) {
	mac, err := RenderCursor(Input{BaseURL: "http://localhost:8080", APIKey: "k", Models: fixtureModels(), Platform: PlatformMacOS})
	if err != nil {
		t.Fatalf("RenderCursor mac: %v", err)
	}
	if !strings.Contains(mac.Files[0].Content, "Settings（⌘,）") {
		t.Errorf("mac shortcut missing: %s", mac.Files[0].Content)
	}
	win, err := RenderCursor(Input{BaseURL: "http://localhost:8080", APIKey: "k", Models: fixtureModels(), Platform: PlatformWindows})
	if err != nil {
		t.Fatalf("RenderCursor win: %v", err)
	}
	if !strings.Contains(win.Files[0].Content, "Settings（Ctrl+,）") {
		t.Errorf("windows shortcut missing: %s", win.Files[0].Content)
	}
}

func TestEnvTabCode(t *testing.T) {
	env := BuildEnv([][2]string{{"OPENAI_BASE_URL", "http://localhost:8080/v1"}})
	windows := EnvTabCode(env, PlatformWindows)
	if !strings.Contains(windows, "# PowerShell:") || !strings.Contains(windows, "# cmd:") {
		t.Errorf("windows env tab missing blocks: %q", windows)
	}
	if got := EnvTabCode(env, PlatformLinux); got != "export OPENAI_BASE_URL=http://localhost:8080/v1" {
		t.Errorf("linux env tab: %q", got)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestDispatch(t *testing.T) {
	cases := []Agent{Opencode, ClaudeCode, Codex, OpenAICompatible, Cursor, WorkBuddy, CodeBuddy, Trae, ZCode, MarsCode, Lingma}
	for _, a := range cases {
		_, err := Dispatch(a, Input{
			BaseURL: "http://localhost:8080",
			APIKey:  "k",
			Models:  fixtureModels(),
		})
		if err != nil {
			t.Errorf("Dispatch(%s): %v", a, err)
		}
	}
	if _, err := Dispatch(Agent("bogus"), Input{Models: fixtureModels()}); err == nil {
		t.Errorf("Dispatch with bogus agent should fail")
	}
}

func TestPickDefaultModelFallback(t *testing.T) {
	models := fixtureModels()
	if got := pickDefaultModel(models, "does/not-exist"); got != models[0].ID {
		t.Errorf("fallback: got %q want %q", got, models[0].ID)
	}
	if got := pickDefaultModel(nil, "x"); got != "" {
		t.Errorf("empty models: got %q", got)
	}
}
