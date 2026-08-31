package rtk

import (
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

// TestCavemanUserRoleE2E exercises the end-to-end chat path: a user message
// flows through applyRtkCompressionWithDefaults and gets compressed when
// Caveman is enabled, but not when disabled. Tool messages stay untouched by
// the user-branch and assistant messages stay untouched by the user-branch.
func TestCavemanUserRoleE2E(t *testing.T) {
	cfg := defaultConfigForCavemanE2E()
	cfg.Enabled = true
	cfg.Caveman.Enabled = true
	cfg.Caveman.Intensity = "lite"
	cfg.Caveman.CompressRoles = []string{"user"}
	cfg.Pipeline = []PipelineStep{{ID: "caveman"}}
	p := &Plugin{config: cfg}
	applyConfigDefaults(cfg)

	orig := "Hi there, could you please help me check the deployment status of the test infrastructure service"
	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: &orig}},
			},
		},
	}
	state := applyRtkCompressionWithDefaults(req, p)
	if !state.Compressed {
		t.Fatalf("expected compression of user message: state=%+v", state)
	}
	if len(req.ChatRequest.Input) != 1 {
		t.Fatalf("input count changed")
	}
	if req.ChatRequest.Input[0].Content == nil || req.ChatRequest.Input[0].Content.ContentStr == nil {
		t.Fatalf("content disappeared")
	}
	got := *req.ChatRequest.Input[0].Content.ContentStr
	if strings.Contains(got, "please") || strings.Contains(got, "Hi there") {
		t.Errorf("user prose not compressed: %q", got)
	}
	for _, tech := range state.Techniques {
		if tech == "caveman-rules" {
			return
		}
	}
	t.Errorf("expected caveman-rules technique, got %v", state.Techniques)
}

// TestCavemanUserRoleDisabledE2E verifies the user branch is a no-op when
// Caveman is disabled (the default).
func TestCavemanUserRoleDisabledE2E(t *testing.T) {
	cfg := defaultConfigForCavemanE2E()
	cfg.Enabled = true
	cfg.Caveman.Enabled = false // explicit
	cfg.Pipeline = []PipelineStep{{ID: "caveman"}}
	p := &Plugin{config: cfg}
	applyConfigDefaults(cfg)

	orig := "Hi there, could you please check the deployment status of the test infrastructure"
	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: &orig}},
			},
		},
	}
	state := applyRtkCompressionWithDefaults(req, p)
	if state.Compressed {
		t.Errorf("user message compressed despite caveman disabled: state=%+v", state)
	}
}

// TestCavemanStackedPipelineE2E verifies a [{rtk, caveman}] stacked pipeline
// routes rtk engines to tool/assistant messages and caveman to user messages,
// not cross-applying.
func TestCavemanStackedPipelineE2E(t *testing.T) {
	cfg := defaultConfigForCavemanE2E()
	cfg.Enabled = true
	cfg.Caveman.Enabled = true
	cfg.Caveman.Intensity = "lite"
	cfg.Caveman.CompressRoles = []string{"user"}
	cfg.Pipeline = []PipelineStep{{ID: "rtk"}, {ID: "caveman"}}
	p := &Plugin{config: cfg}
	applyConfigDefaults(cfg)

	// Two user messages, each compressed by the user-only branch.
	u1 := "Hi there, please help me check the deployment status of the test infrastructure service"
	u2 := "Could you kindly verify the test result and let me know what you think about the configuration"
	req := &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: &u1}},
				{Role: schemas.ChatMessageRoleAssistant, Content: &schemas.ChatMessageContent{ContentStr: strPtr("Sure, here is the status: all green.")}},
				{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: &u2}},
			},
		},
	}
	state := applyRtkCompressionWithDefaults(req, p)
	if !state.Compressed {
		t.Fatalf("expected compression in stacked pipeline, got state=%+v", state)
	}
	// The user-branch mutates the request messages in place: verify the
	// compressed user messages lost their fillers.
	got1 := *req.ChatRequest.Input[0].Content.ContentStr
	got2 := *req.ChatRequest.Input[2].Content.ContentStr
	if strings.Contains(got1, "Hi there") || strings.Contains(got1, "please") {
		t.Errorf("user message 1 not compressed: %q", got1)
	}
	if strings.Contains(got2, "kindly") {
		t.Errorf("user message 2 not compressed: %q", got2)
	}
}

// defaultConfigForCavemanE2E returns a minimal Config suitable for the e2e
// tests. The CavemanConfig is fully defaulted; the rest of the fields are
// zero-valued (RTK Enabled=true is set by the caller).
func defaultConfigForCavemanE2E() *Config {
	return &Config{
		Enabled: true,
		// Tighten the min-length gate to keep these tests fast and stable.
		MaxLinesPerResult: 120,
		MaxCharsPerResult: 12000,
		Pipeline:          []PipelineStep{{ID: "rtk"}},
	}
}
