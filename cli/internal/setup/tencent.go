package setup

// Tencent WorkBuddy / CodeBuddy share the same local model registry: a
// `~/.{workbuddy,codebuddy}/models.json` containing an OpenAI-compatible
// `models[]` list plus an `availableModels[]` allow-list. Each model entry
// points at a full chat-completions URL, so celer-route's `/v1` surface maps
// to `{origin}/v1/chat/completions`. The first `availableModels` entry is the
// client's default model, so the picker's default is hoisted to the front.
//
// Structs (not maps) keep the JSON key order identical to the TypeScript
// engine — id, name, vendor, url, apiKey, maxInputTokens, maxOutputTokens.

type tencentModelEntry struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Vendor          string `json:"vendor"`
	URL             string `json:"url"`
	APIKey          string `json:"apiKey,omitempty"`
	MaxInputTokens  int    `json:"maxInputTokens,omitempty"`
	MaxOutputTokens int    `json:"maxOutputTokens,omitempty"`
}

type tencentModelsDoc struct {
	Models          []tencentModelEntry `json:"models"`
	AvailableModels []string            `json:"availableModels"`
}

// RenderTencentModelsJSON renders the shared WorkBuddy/CodeBuddy models.json.
// path is the per-client config path (~/.workbuddy/models.json etc.).
func RenderTencentModelsJSON(in Input, path string, agent Agent) (Output, error) {
	if len(in.Models) == 0 {
		return Output{}, ErrNoModels
	}
	baseURL := OpenAISurface(in.BaseURL) + "/chat/completions"
	defaultID := pickDefaultModel(in.Models, in.DefaultModelID)

	ordered := make([]Model, len(in.Models))
	copy(ordered, in.Models)
	if defaultID != "" {
		for i, m := range ordered {
			if m.ID == defaultID && i > 0 {
				ordered[0], ordered[i] = ordered[i], ordered[0]
				break
			}
		}
	}

	entries := make([]tencentModelEntry, 0, len(ordered))
	for _, m := range ordered {
		name := m.Name
		if name == "" || name == m.ID {
			name = m.ID
		}
		entries = append(entries, tencentModelEntry{
			ID:              m.ID,
			Name:            name,
			Vendor:          "OpenAI",
			URL:             baseURL,
			APIKey:          in.APIKey,
			MaxInputTokens:  m.ContextLength,
			MaxOutputTokens: m.MaxOutput,
		})
	}
	available := make([]string, 0, len(ordered))
	for _, m := range ordered {
		available = append(available, m.ID)
	}

	out := Output{
		Files: []File{
			{Path: path, Content: JSONMarshalIndent(tencentModelsDoc{Models: entries, AvailableModels: available})},
		},
		DefaultModel: defaultID,
		Agent:        agent,
	}
	return out, nil
}

// RenderWorkBuddy writes ~/.workbuddy/models.json for Tencent WorkBuddy.
func RenderWorkBuddy(in Input) (Output, error) {
	return RenderTencentModelsJSON(in, DisplayPath(platformOrDefault(in), ".workbuddy", "models.json"), WorkBuddy)
}

// RenderCodeBuddy writes ~/.codebuddy/models.json for Tencent CodeBuddy.
func RenderCodeBuddy(in Input) (Output, error) {
	return RenderTencentModelsJSON(in, DisplayPath(platformOrDefault(in), ".codebuddy", "models.json"), CodeBuddy)
}
