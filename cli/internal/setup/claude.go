package setup

import (
	"path/filepath"
	"strings"
)

// ClaudeCodePath returns the absolute path the CLI writes to by default.
func ClaudeCodePath(homeDir string) string {
	if homeDir == "" {
		homeDir = "$HOME"
	}
	return filepath.Join(homeDir, ".claude", "settings.json")
}

// RenderClaudeCode emits ~/.claude/settings.json plus the matching
// ANTHROPIC_* env exports. Claude Code appends /v1/messages to
// ANTHROPIC_BASE_URL, so we point at the gateway's /anthropic surface.
func RenderClaudeCode(in Input) (Output, error) {
	if len(in.Models) == 0 {
		return Output{}, ErrNoModels
	}
	baseURL := AnthropicSurface(in.BaseURL)
	defaultID := pickDefaultModel(in.Models, in.DefaultModelID)

	env := []string{
		"export ANTHROPIC_BASE_URL=" + baseURL,
	}
	if in.APIKey != "" {
		env = append(env, "export ANTHROPIC_AUTH_TOKEN="+in.APIKey)
	}
	if defaultID != "" {
		env = append(env, "export ANTHROPIC_MODEL="+defaultID)
	}

	envMap := map[string]string{}
	for _, line := range env {
		const prefix = "export "
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		body := line[len(prefix):]
		eq := strings.IndexByte(body, '=')
		if eq < 0 {
			continue
		}
		envMap[body[:eq]] = body[eq+1:]
	}

	cfg := map[string]any{"env": envMap}
	content := JSONMarshalIndent(cfg)

	return Output{
		Files: []File{
			{Path: "~/.claude/settings.json", Content: content},
		},
		Env:          env,
		DefaultModel: defaultID,
		Agent:        ClaudeCode,
	}, nil
}
