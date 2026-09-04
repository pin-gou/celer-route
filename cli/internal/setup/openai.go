package setup

// RenderOpenAICompatible emits an OPENAI_* env recipe for any coding
// agent that follows the OpenAI CLI convention (hermes, openclaw,
// aider, goose, qwen, …). The file is a plain shell snippet; most
// agents accept the values via environment variables.
func RenderOpenAICompatible(in Input) (Output, error) {
	if len(in.Models) == 0 {
		return Output{}, ErrNoModels
	}
	baseURL := OpenAISurface(in.BaseURL)
	defaultID := pickDefaultModel(in.Models, in.DefaultModelID)

	env := []string{
		"export OPENAI_BASE_URL=" + baseURL,
	}
	if in.APIKey != "" {
		env = append(env, "export OPENAI_API_KEY="+in.APIKey)
	}
	env = append(env, "export OPENAI_MODEL="+defaultID)

	return Output{
		Files: []File{
			{Path: ".env (environment)", Content: joinLines(env) + "\n"},
		},
		Env:          env,
		DefaultModel: defaultID,
		Agent:        OpenAICompatible,
	}, nil
}

// joinLines concatenates ss with '\n' separators.
func joinLines(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += "\n" + s
	}
	return out
}
