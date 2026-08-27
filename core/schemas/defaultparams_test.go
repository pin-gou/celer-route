package schemas

import "testing"

func TestLookupProviderDefaultParams_Sensenova(t *testing.T) {
	defs := LookupProviderDefaultParams(Sensenova)
	if len(defs) != 1 {
		t.Fatalf("sensenova should register 1 default param, got %d", len(defs))
	}
	d := defs[0]
	if d.Key != "reasoning_effort" {
		t.Errorf("key = %q, want reasoning_effort", d.Key)
	}
	if d.Label == "" {
		t.Error("label should not be empty")
	}
	if len(d.Options) != 4 {
		t.Errorf("expected 4 options, got %d", len(d.Options))
	}
}

func TestLookupProviderDefaultParams_UnknownProviderEmpty(t *testing.T) {
	if defs := LookupProviderDefaultParams(OpenAI); len(defs) != 0 {
		t.Errorf("OpenAI should not register any default params, got %d", len(defs))
	}
	// Custom provider strings are not in the registry.
	if defs := LookupProviderDefaultParams(ModelProvider("custom-provider")); len(defs) != 0 {
		t.Errorf("custom provider should not register any default params, got %d", len(defs))
	}
}

func TestLookupProviderDefaultParams_DefensiveCopy(t *testing.T) {
	defs := LookupProviderDefaultParams(Sensenova)
	if len(defs) == 0 {
		t.Fatal("expected definitions")
	}
	// Mutating the returned slice/options must not affect the global registry.
	defs[0].Options[0] = "mutated"
	defs[0].Key = "mutated"
	again := LookupProviderDefaultParams(Sensenova)
	if again[0].Key != "reasoning_effort" {
		t.Errorf("registry leaked mutation: key = %q", again[0].Key)
	}
	if again[0].Options[0] != "none" {
		t.Errorf("registry leaked mutation: options[0] = %q", again[0].Options[0])
	}
}

func TestProviderSupportsDefaultParam(t *testing.T) {
	if !ProviderSupportsDefaultParam(Sensenova, "reasoning_effort") {
		t.Error("sensenova should support reasoning_effort")
	}
	if ProviderSupportsDefaultParam(Sensenova, "temperature") {
		t.Error("sensenova should not support an unregistered param")
	}
	if ProviderSupportsDefaultParam(OpenAI, "reasoning_effort") {
		t.Error("OpenAI has not registered reasoning_effort")
	}
}

func TestModelMatchesDefaultParamPatterns(t *testing.T) {
	patterns := []string{"deepseek-v4-flash", "glm-5.2"}
	// Empty patterns match everything.
	if !ModelMatchesDefaultParamPatterns("anything", nil) {
		t.Error("empty patterns should match any model")
	}
	// Exact match.
	if !ModelMatchesDefaultParamPatterns("deepseek-v4-flash", patterns) {
		t.Error("exact deepseek-v4-flash should match")
	}
	// Substring match (provider-prefixed or versioned names).
	if !ModelMatchesDefaultParamPatterns("sensenova/deepseek-v4-flash", patterns) {
		t.Error("prefixed deepseek-v4-flash should match")
	}
	if !ModelMatchesDefaultParamPatterns("glm-5.2-20260401", patterns) {
		t.Error("versioned glm-5.2 should match")
	}
	// Case-insensitive.
	if !ModelMatchesDefaultParamPatterns("DeepSeek-V4-Flash", patterns) {
		t.Error("case-insensitive match should hold")
	}
	// Unrelated model.
	if ModelMatchesDefaultParamPatterns("deepseek-v3", patterns) {
		t.Error("deepseek-v3 should NOT match reasoning patterns")
	}
	// Empty model never matches non-empty patterns.
	if ModelMatchesDefaultParamPatterns("", patterns) {
		t.Error("empty model should not match non-empty patterns")
	}
}

func TestProviderModelSupportsDefaultParam(t *testing.T) {
	if !ProviderModelSupportsDefaultParam(Sensenova, "deepseek-v4-flash", "reasoning_effort") {
		t.Error("sensenova deepseek-v4-flash should support reasoning_effort")
	}
	if !ProviderModelSupportsDefaultParam(Sensenova, "glm-5.2", "reasoning_effort") {
		t.Error("sensenova glm-5.2 should support reasoning_effort")
	}
	// A sensenova model that does NOT accept reasoning_effort must be gated out.
	if ProviderModelSupportsDefaultParam(Sensenova, "sensenova-v6", "reasoning_effort") {
		t.Error("unsupported sensenova model should NOT support reasoning_effort")
	}
	if ProviderModelSupportsDefaultParam(Sensenova, "deepseek-v4-flash", "temperature") {
		t.Error("unregistered param should not be supported for any model")
	}
	if ProviderModelSupportsDefaultParam(OpenAI, "gpt-5", "reasoning_effort") {
		t.Error("OpenAI has not registered reasoning_effort")
	}
}
