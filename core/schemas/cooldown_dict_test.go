package schemas

import (
	"slices"
	"testing"
)

func TestLookupProviderErrorCatalog_KnownProvider(t *testing.T) {
	c := LookupProviderErrorCatalog(OpenAI)
	if c.IsEmpty() {
		t.Fatalf("expected non-empty catalog for OpenAI")
	}
	if !slices.Contains(c.Types, "rate_limit_error") {
		t.Errorf("expected OpenAI catalog to contain rate_limit_error, got %v", c.Types)
	}
	if !slices.Contains(c.Codes, "insufficient_quota") {
		t.Errorf("expected OpenAI catalog to contain insufficient_quota, got %v", c.Codes)
	}
}

func TestLookupProviderErrorCatalog_UnknownProviderFallsBack(t *testing.T) {
	unknown := ModelProvider("totally-made-up-provider")
	c := LookupProviderErrorCatalog(unknown)
	if c.IsEmpty() {
		t.Fatalf("expected generic fallback for unknown provider, got empty catalog")
	}
	// Generic fallback must include common types so the dropdown is never blank.
	if !slices.Contains(c.Types, "rate_limit_error") {
		t.Errorf("generic fallback missing rate_limit_error, got %v", c.Types)
	}
}

func TestLookupProviderErrorCatalog_DefensiveCopy(t *testing.T) {
	c1 := LookupProviderErrorCatalog(OpenAI)
	c2 := LookupProviderErrorCatalog(OpenAI)
	// Mutate the first; the second must remain unchanged so callers can't
	// stomp on the global catalog through a returned reference.
	c1.Types[0] = "MUTATED"
	if c2.Types[0] == "MUTATED" {
		t.Fatalf("catalog returned shared backing array — defensive copy missing")
	}
}

func TestLookupProviderErrorCatalog_AllStandardProvidersPresent(t *testing.T) {
	// Coverage check: every StandardProvider (except those unlikely to surface
	// error responses, like keyless opencode variants) has a catalog entry so
	// the UI dropdown isn't a dead-letter when an operator edits one.
	knownExceptions := map[ModelProvider]bool{} // no exceptions — every provider gets a catalog
	for _, p := range StandardProviders {
		if knownExceptions[p] {
			continue
		}
		c := LookupProviderErrorCatalog(p)
		if c.IsEmpty() {
			t.Errorf("provider %q has empty catalog — add it to providerErrorCatalog", p)
		}
	}
}

func TestIsKnownProviderErrorType(t *testing.T) {
	if !IsKnownProviderErrorType(OpenAI, "rate_limit_error") {
		t.Error("expected OpenAI/rate_limit_error to be known")
	}
	if IsKnownProviderErrorType(OpenAI, "totally_made_up_type") {
		t.Error("expected OpenAI/totally_made_up_type to be unknown")
	}
	// Unknown provider falls back to generic, which still contains
	// rate_limit_error, so the helper should report known.
	if !IsKnownProviderErrorType(ModelProvider("unknown"), "rate_limit_error") {
		t.Error("expected generic fallback to recognise rate_limit_error")
	}
}

func TestIsKnownProviderErrorCode(t *testing.T) {
	if !IsKnownProviderErrorCode(Sensenvola(t), "insufficient_quota") {
		t.Fatal("expected sensenova/insufficient_quota to be known")
	}
	if IsKnownProviderErrorCode(Sensenvola(t), "made_up_code") {
		t.Fatal("expected sensenova/made_up_code to be unknown")
	}
}

// Sensenvola is a tiny helper so the sensenova literal reads cleanly in the
// assertions above (avoids a typo hard-coding the constant name).
func Sensenvola(t *testing.T) ModelProvider {
	t.Helper()
	return Sensenova
}