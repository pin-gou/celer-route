package rtk

import (
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

// cavemanTestConfig returns a ready-to-use Caveman config with defaults.
func cavemanTestConfig() CavemanConfig {
	c := defaultCavemanConfig()
	c.Enabled = true
	return c
}

// TestCavemanCompressTextShortMessage verifies the min-length gate.
func TestCavemanCompressTextShortMessage(t *testing.T) {
	out := cavemanCompressText("hi", cavemanTestConfig())
	if out.Compressed {
		t.Errorf("expected short message unchanged")
	}
	if out.Text != "hi" {
		t.Errorf("short message mutated: %q", out.Text)
	}
}

// TestCavemanCompressTextLiteIntensity verifies lite intensity compresses
// fillers but preserves substantive words and technical tokens.
func TestCavemanCompressTextLiteIntensity(t *testing.T) {
	cfg := cavemanTestConfig()
	cfg.Intensity = "lite"
	input := "Hi there, could you please check the logs and let me know what you think about the deployment status"
	out := cavemanCompressText(input, cfg)
	if !out.Compressed {
		t.Fatalf("expected compression, got unchanged")
	}
	if len(out.Text) >= len(input) {
		t.Errorf("expected shorter text, got %d >= %d", len(out.Text), len(input))
	}
	// The substantive tokens must remain (case-insensitive — sentence
	// recapitalisation may uppercase the leading word).
	for _, word := range []string{"check", "deployment", "status"} {
		if !strings.Contains(strings.ToLower(out.Text), word) {
			t.Errorf("substantive word %q lost: %q", word, out.Text)
		}
	}
	if !out.ValidationOK {
		t.Errorf("validation should pass, errors=%v", out.ValidationOK)
	}
}

// TestCavemanCompressTextFullIntensity verifies full intensity removes
// articles and leaders.
func TestCavemanCompressTextFullIntensity(t *testing.T) {
	cfg := cavemanTestConfig()
	cfg.Intensity = "full"
	out := cavemanCompressText("Please make sure to read the documentation for the API", cfg)
	if !out.Compressed {
		t.Fatalf("expected compression at full intensity")
	}
	if strings.Contains(out.Text, "make sure to") {
		t.Errorf("redundant phrasing should be rewritten: %q", out.Text)
	}
}

// TestCavemanCompressTextUltraIntensity verifies ultra abbreviations fire.
func TestCavemanCompressTextUltraIntensity(t *testing.T) {
	cfg := cavemanTestConfig()
	cfg.Intensity = "ultra"
	out := cavemanCompressText("The application configuration function is not responding correctly in the database request", cfg)
	if !out.Compressed {
		t.Fatalf("expected compression at ultra intensity")
	}
	if strings.Contains(out.Text, "application configuration function") {
		t.Errorf("ultra abbreviations should rewrite: %q", out.Text)
	}
}

// TestCavemanCompressTextZh verifies Chinese prose compresses.
func TestCavemanCompressTextZh(t *testing.T) {
	cfg := cavemanTestConfig()
	input := "你好，请帮我看看这个配置文件，谢谢你，我觉得应该可以正常运行这个服务，但是我不确定具体参数该怎么配置才最合适"
	out := cavemanCompressText(input, cfg)
	if !out.Compressed {
		t.Fatalf("expected zh compression, got unchanged")
	}
	if !strings.Contains(out.Text, "配置文件") {
		t.Errorf("substantive zh word lost: %q", out.Text)
	}
}

// TestCavemanCompressTextPreservesCode verifies code blocks survive a rules
// pass untouched (integration-level protection + fidelity).
func TestCavemanCompressTextPreservesCode(t *testing.T) {
	cfg := cavemanTestConfig()
	cfg.Intensity = "full"
	input := "Please fix this function please:\n```python\ndef the_thing(x):\n    return x * 2\n```\nThanks a lot!"
	out := cavemanCompressText(input, cfg)
	if !out.ValidationOK {
		t.Fatalf("validation failed: %q (errors omitted)", out.Text)
	}
	if !strings.Contains(out.Text, "def the_thing(x):") {
		t.Errorf("code block lost through compression: %q", out.Text)
	}
}

// TestCavemanCompressTextFallback verifies the fidelity fallback returns the
// original when a protected item would be lost.
func TestCavemanCompressTextFallback(t *testing.T) {
	cfg := cavemanTestConfig()
	// A message with heavy structure that rules might otherwise mangle; the
	// preservation + validation path must keep it intact (validation OK, no
	// fallback needed) OR fall back safely. Either way the output must not
	// silently drop the protected token.
	input := "Please retain The VeryImportantConstantName in your answer"
	cfg.PreservePatterns = []string{`VeryImportantConstantName`}
	out := cavemanCompressText(input, cfg)
	if !strings.Contains(out.Text, "VeryImportantConstantName") {
		t.Errorf("protected token lost: %q", out.Text)
	}
}

// TestCavemanEngineApply verifies the CompressionEngine wrapper.
func TestCavemanEngineApply(t *testing.T) {
	cfg := &Config{Enabled: true}
	cfg.Caveman = defaultCavemanConfig()
	cfg.Caveman.Enabled = true
	cfg.Caveman.Intensity = "lite"
	p := &Plugin{config: cfg}
	eng := &cavemanEngine{plugin: p}

	res, err := eng.Apply(nil, "Hi there, could you please help me check the test result status", EngineConfig{})
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if res.Skipped {
		t.Fatalf("expected engine to compress, skipped: %s", res.Reason)
	}
	if len(res.Techniques) == 0 {
		t.Errorf("expected techniques recorded")
	}
	if res.InputBytes <= res.OutputBytes {
		t.Errorf("expected output smaller, in=%d out=%d", res.InputBytes, res.OutputBytes)
	}

	// Disabled engine must skip.
	cfg2 := &Config{Enabled: true}
	cfg2.Caveman = defaultCavemanConfig()
	cfg2.Caveman.Enabled = false
	p2 := &Plugin{config: cfg2}
	eng2 := &cavemanEngine{plugin: p2}
	res2, _ := eng2.Apply(nil, "the quick brown fox", EngineConfig{})
	if !res2.Skipped {
		t.Errorf("expected disabled engine to skip")
	}
}

// TestCavemanEnginesForRole verifies role-based engine filtering.
func TestCavemanEnginesForRole(t *testing.T) {
	p := &Pipeline{Engines: []string{"rtk", "caveman"}}
	user := enginesForRole(p, string(schemas.ChatMessageRoleUser))
	if len(user.Engines) != 1 || user.Engines[0] != "caveman" {
		t.Errorf("user role pipeline = %v, want [caveman]", user.Engines)
	}
	tool := enginesForRole(p, string(schemas.ChatMessageRoleTool))
	if len(tool.Engines) != 1 || tool.Engines[0] != "rtk" {
		t.Errorf("tool role pipeline = %v, want [rtk]", tool.Engines)
	}
	assist := enginesForRole(p, string(schemas.ChatMessageRoleAssistant))
	if len(assist.Engines) != 1 || assist.Engines[0] != "rtk" {
		t.Errorf("assistant role pipeline = %v, want [rtk]", assist.Engines)
	}
	// Default rtk-only pipeline passes tool messages through unchanged.
	p2 := &Pipeline{Engines: []string{"rtk"}}
	if got := enginesForRole(p2, string(schemas.ChatMessageRoleTool)); len(got.Engines) != 1 {
		t.Errorf("default pipeline should not change for tool role")
	}
	// Caveman-only pipeline on a user message keeps caveman.
	p3 := &Pipeline{Engines: []string{"caveman"}}
	if got := enginesForRole(p3, string(schemas.ChatMessageRoleUser)); len(got.Engines) != 1 || got.Engines[0] != "caveman" {
		t.Errorf("caveman-only pipeline should keep caveman for user")
	}
}

// TestCavemanAppliesToRole verifies the config gating helper.
func TestCavemanAppliesToRole(t *testing.T) {
	cfg := &Config{}
	cfg.Caveman = defaultCavemanConfig()
	if cavemanAppliesToRole(cfg, "user") {
		t.Errorf("caveman disabled should not apply")
	}
	cfg.Caveman.Enabled = true
	if !cavemanAppliesToRole(cfg, "user") {
		t.Errorf("user should apply when enabled and in CompressRoles")
	}
	if cavemanAppliesToRole(cfg, "assistant") {
		t.Errorf("assistant should not apply with default CompressRoles=[user]")
	}
	cfg.Caveman.CompressRoles = []string{"user", "assistant"}
	if !cavemanAppliesToRole(cfg, "assistant") {
		t.Errorf("assistant should apply when whitelisted")
	}
	if cavemanAppliesToRole(cfg, "system") {
		t.Errorf("system should never apply")
	}
}

// TestCavemanConfigValidateRange verifies config validation.
func TestCavemanConfigValidateRange(t *testing.T) {
	if err := (&CavemanConfig{}).Validate(); err != nil {
		t.Errorf("default config should validate: %v", err)
	}
	bad := defaultCavemanConfig()
	bad.Intensity = "extreme"
	if err := bad.Validate(); err == nil {
		t.Errorf("invalid intensity should fail validation")
	}
	bad2 := defaultCavemanConfig()
	bad2.Language = "fr"
	if err := bad2.Validate(); err == nil {
		t.Errorf("unsupported language should fail validation")
	}
}

// TestCavemanPreviewMode verifies /api/compression/preview mode=caveman routes
// through the caveman engine and reports savings.
func TestCavemanPreviewMode(t *testing.T) {
	cfg := &Config{Enabled: true}
	cfg.Caveman = defaultCavemanConfig()
	cfg.Caveman.Enabled = true
	cfg.Caveman.Intensity = "lite"
	p := &Plugin{config: cfg}
	// register engines in the global catalog so the preview runner resolves
	// the "caveman" engine id.
	RegisterEngine(&cavemanEngine{plugin: p})

	resp := p.PreviewCompression(PreviewRequest{
		Mode: CompressionModeCaveman,
		Payload: TestPayload{
			Output: "Hi there, could you please help me check the deployment status of the test infrastructure service",
		},
	})
	if resp.Mode != CompressionModeCaveman {
		t.Errorf("mode = %q, want caveman", resp.Mode)
	}
	if len(resp.EnginesPlanned) != 1 || resp.EnginesPlanned[0] != "caveman" {
		t.Errorf("enginesPlanned = %v, want [caveman]", resp.EnginesPlanned)
	}
	if resp.Result.CompressedTokens >= resp.Result.OriginalTokens {
		t.Errorf("expected savings in caveman preview: %d -> %d", resp.Result.OriginalTokens, resp.Result.CompressedTokens)
	}
	if resp.Result.CompressedText == resp.Result.OriginalText {
		t.Errorf("expected compressed text to differ from original")
	}
}

// TestCavemanStackedPreviewMode verifies mode=stacked plans the configured
// pipeline engines.
func TestCavemanStackedPreviewMode(t *testing.T) {
	cfg := &Config{Enabled: true}
	cfg.Caveman = defaultCavemanConfig()
	cfg.Caveman.Enabled = true
	cfg.Pipeline = []PipelineStep{{ID: "rtk"}, {ID: "caveman"}}
	p := &Plugin{config: cfg}
	RegisterEngine(&cavemanEngine{plugin: p})

	resp := p.PreviewCompression(PreviewRequest{
		Mode: CompressionModeStacked,
		Payload: TestPayload{
			Output: "Hi there, could you please check the deployment status",
		},
	})
	if len(resp.EnginesPlanned) != 2 || resp.EnginesPlanned[0] != "rtk" || resp.EnginesPlanned[1] != "caveman" {
		t.Errorf("enginesPlanned = %v, want [rtk caveman]", resp.EnginesPlanned)
	}
}

// TestCavemanNormalizeConfig serifies defaulting.
func TestCavemanNormalizeConfig(t *testing.T) {
	c := CavemanConfig{}
	normalizeCavemanConfig(&c)
	if c.Intensity != "lite" {
		t.Errorf("intensity should default to lite, got %q", c.Intensity)
	}
	if c.MinMessageLength != 50 {
		t.Errorf("min_message_length should default to 50, got %d", c.MinMessageLength)
	}
	if c.Language != "auto" {
		t.Errorf("language should default to auto, got %q", c.Language)
	}
	if len(c.CompressRoles) != 1 || c.CompressRoles[0] != "user" {
		t.Errorf("compress_roles should default to [user], got %v", c.CompressRoles)
	}
	if c.Enabled {
		t.Errorf("caveman should default to disabled")
	}
}
