package setup

// RenderMarsCode emits the same OPENAI_* env recipe the generic
// OpenAI-compatible template uses, under the MarsCode name so the product is
// identifiable in the client picker. MarsCode CLI reads OpenAI-compatible
// environment variables from the shell it launches under.
func RenderMarsCode(in Input) (Output, error) {
	if len(in.Models) == 0 {
		return Output{}, ErrNoModels
	}
	baseURL := OpenAISurface(in.BaseURL)
	defaultID := pickDefaultModel(in.Models, in.DefaultModelID)

	entries := [][2]string{{"OPENAI_BASE_URL", baseURL}}
	if in.APIKey != "" {
		entries = append(entries, [2]string{"OPENAI_API_KEY", in.APIKey})
	}
	entries = append(entries, [2]string{"OPENAI_MODEL", defaultID})
	env := BuildEnv(entries)

	return Output{
		Files: []File{
			{Path: ".env (environment)", Content: EnvTabCode(env, platformOrDefault(in)) + "\n"},
		},
		Env:          env,
		DefaultModel: defaultID,
		Agent:        MarsCode,
	}, nil
}
