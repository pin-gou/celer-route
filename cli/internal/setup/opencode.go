package setup

import (
	"path/filepath"
)

// OpencodePath returns the absolute path the CLI writes to by default.
// Honors $XDG_CONFIG_HOME; falls back to ~/.config/opencode/opencode.json.
func OpencodePath(homeDir string) string {
	if homeDir == "" {
		homeDir = "$HOME"
	}
	return filepath.Join(homeDir, ".config", "opencode", "opencode.json")
}

// RenderOpencode builds the opencode.json that points at celer-route.
// Output is byte-identical (modulo whitespace) to the Web UI's opencode
// template so the two surfaces stay in sync.
func RenderOpencode(in Input) (Output, error) {
	if len(in.Models) == 0 {
		return Output{}, ErrNoModels
	}
	baseURL := OpenAISurface(in.BaseURL)
	defaultID := pickDefaultModel(in.Models, in.DefaultModelID)
	npmPkg := "@ai-sdk/openai-compatible"
	if in.Protocol == "responses" {
		npmPkg = "@ai-sdk/openai"
	}

	modelsMap := map[string]map[string]any{}
	for _, m := range in.Models {
		entry := map[string]any{}
		if m.Name != "" && m.Name != m.ID {
			entry["name"] = m.Name
		}
		if lim := limitMap(m); lim != nil {
			entry["limit"] = lim
		}
		if len(entry) > 0 {
			modelsMap[m.ID] = entry
		} else {
			modelsMap[m.ID] = map[string]any{}
		}
	}

	provider := map[string]any{
		"npm":  npmPkg,
		"name": ProviderKey,
		"options": func() map[string]any {
			opts := map[string]any{"baseURL": baseURL}
			if in.APIKey != "" {
				opts["apiKey"] = in.APIKey
			}
			return opts
		}(),
	}
	if len(modelsMap) > 0 {
		provider["models"] = modelsMap
	}

	cfg := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]any{
			ProviderKey: provider,
		},
	}
	if defaultID != "" {
		cfg["model"] = ProviderKey + "/" + defaultID
	}

	out := Output{
		Files: []File{
			{Path: DisplayPath(platformOrDefault(in), ".config", "opencode", "opencode.json"), Content: JSONMarshalIndent(cfg)},
		},
		Agent: Opencode,
	}
	if defaultID != "" {
		out.DefaultModel = ProviderKey + "/" + defaultID
	}
	return out, nil
}
