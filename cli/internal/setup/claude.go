package setup

import (
	"path/filepath"
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

	var entries [][2]string
	entries = append(entries, [2]string{"ANTHROPIC_BASE_URL", baseURL})
	if in.APIKey != "" {
		entries = append(entries, [2]string{"ANTHROPIC_AUTH_TOKEN", in.APIKey})
	}
	if defaultID != "" {
		entries = append(entries, [2]string{"ANTHROPIC_MODEL", defaultID})
	}
	env := BuildEnv(entries)

	// settings.json embeds the same values; on every platform it is the same
	// JSON (Claude Code reads env from it regardless of host OS).
	envMap := map[string]string{}
	for _, kv := range entries {
		envMap[kv[0]] = kv[1]
	}
	cfg := map[string]any{"env": envMap}
	content := JSONMarshalIndent(cfg)

	return Output{
		Files: []File{
			{Path: DisplayPath(platformOrDefault(in), ".claude", "settings.json"), Content: content},
		},
		Env:          env,
		DefaultModel: defaultID,
		Agent:        ClaudeCode,
	}, nil
}
