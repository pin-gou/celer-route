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
