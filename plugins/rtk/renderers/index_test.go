package renderers

import (
	"testing"
)

// TestApplyRenderer_UnknownType verifies that an unregistered detection
// type returns the original text with Changed=false.
func TestApplyRenderer_UnknownType(t *testing.T) {
	res := ApplyRenderer("some text", DetectionInfo{Type: "unknown-type"}, RenderConfig{})
	if res.Changed {
		t.Error("Changed=false expected for unknown type")
	}
	if res.Text != "some text" {
		t.Errorf("Text should equal input, got %q", res.Text)
	}
}

// TestApplyRenderer_AllowedRenderersWhitelist verifies that the whitelist
// restricts renderers to the listed detection types.
func TestApplyRenderer_AllowedRenderersWhitelist(t *testing.T) {
	input := `diff --git a/x b/x
@@ -1 +1 @@
-old
+new`
	// type=git-diff but not in whitelist → no-op.
	res := ApplyRenderer(input, DetectionInfo{Type: "git-diff"}, RenderConfig{
		AllowedRenderers: []string{"test-pytest"},
	})
	if res.Changed {
		t.Error("renderer should have been filtered out by whitelist")
	}
	if res.Text != input {
		t.Error("text should be unchanged")
	}
}

// TestApplyRenderer_PanicRecovery verifies that a panic inside a
// registered renderer is caught and the original text returned.
// We register a synthetic renderer that panics, then call ApplyRenderer.
func TestApplyRenderer_PanicRecovery(t *testing.T) {
	prev := registry["panic-test"]
	defer func() { registry["panic-test"] = prev }()
	registry["panic-test"] = func(text string, _ DetectionInfo) (RenderResult, bool) {
		panic("synthetic renderer panic")
	}
	res := ApplyRenderer("safe input", DetectionInfo{Type: "panic-test"}, RenderConfig{})
	if res.Changed {
		t.Error("Changed=false expected when renderer panics")
	}
	if res.Text != "safe input" {
		t.Errorf("Text should equal input on panic, got %q", res.Text)
	}
}

// TestApplyRenderer_DefaultRegistryContainsExpectedTypes verifies that
// the canonical renderer set is registered. If a future refactor
// accidentally drops a renderer, this test surfaces the regression.
func TestApplyRenderer_DefaultRegistryContainsExpectedTypes(t *testing.T) {
	expected := []string{
		"git-diff",
		"test-pytest",
		"test-jest",
		"test-vitest",
		"test-go",
		"build-eslint",
		"terraform-plan",
		"tofu-plan",
		"aws",
		"json-output",
	}
	for _, t1 := range expected {
		if _, ok := registry[t1]; !ok {
			t.Errorf("registry missing %q", t1)
		}
	}
}

// TestRegisteredRenderers verifies the introspection helper returns
// the canonical types.
func TestRegisteredRenderers(t *testing.T) {
	keys := RegisteredRenderers()
	if len(keys) == 0 {
		t.Fatal("RegisteredRenderers returned empty slice")
	}
	want := map[string]bool{
		"git-diff":        true,
		"test-pytest":     true,
		"terraform-plan":  true,
		"aws":             true,
	}
	for _, k := range keys {
		if want[k] {
			delete(want, k)
		}
	}
	if len(want) > 0 {
		t.Errorf("RegisteredRenderers missing keys: %v", want)
	}
}