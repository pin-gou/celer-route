package setup

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CodexPath returns the absolute path the CLI writes to by default.
func CodexPath(homeDir string) string {
	if homeDir == "" {
		homeDir = "$HOME"
	}
	return filepath.Join(homeDir, ".codex", "config.toml")
}

// CodexEnvKey is the env var Codex CLI reads for the API key.
const CodexEnvKey = "CELER_ROUTE_API_KEY"

// RenderCodex emits ~/.codex/config.toml plus the export line for the
// CodexEnvKey. Codex uses TOML; we hand-build it to avoid pulling a
// dependency on BurntSushi/toml for the entire CLI.
func RenderCodex(in Input) (Output, error) {
	if len(in.Models) == 0 {
		return Output{}, ErrNoModels
	}
	baseURL := OpenAISurface(in.BaseURL)
	defaultID := pickDefaultModel(in.Models, in.DefaultModelID)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("model = %q\n", defaultID))
	b.WriteString(fmt.Sprintf("model_provider = %q\n\n", ProviderKey))
	b.WriteString(fmt.Sprintf("[model_providers.%s]\n", ProviderKey))
	b.WriteString(fmt.Sprintf("name = %q\n", ProviderKey))
	b.WriteString(fmt.Sprintf("base_url = %q\n", baseURL))
	b.WriteString("wire_api = \"chat\"\n")
	b.WriteString(fmt.Sprintf("env_key = %q\n", CodexEnvKey))
	b.WriteString("\n")

	out := Output{
		Files: []File{
			{Path: DisplayPath(platformOrDefault(in), ".codex", "config.toml"), Content: b.String()},
		},
		DefaultModel: defaultID,
		Agent:        Codex,
	}
	if in.APIKey != "" {
		out.Env = BuildEnv([][2]string{{CodexEnvKey, in.APIKey}})
	}
	return out, nil
}
