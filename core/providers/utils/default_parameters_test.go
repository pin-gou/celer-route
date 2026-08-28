package utils

import (
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestApplyDefaultParameters_InjectReasoningEffort(t *testing.T) {
	req := &schemas.BifrostChatRequest{Model: "deepseek-v4-flash"}
	ApplyDefaultParameters(schemas.Sensenova, "deepseek-v4-flash", req, map[string]interface{}{"reasoning_effort": "high"})
	if req.Params == nil || req.Params.Reasoning == nil || req.Params.Reasoning.Effort == nil {
		t.Fatal("expected Params.Reasoning.Effort to be set")
	}
	if got := *req.Params.Reasoning.Effort; got != "high" {
		t.Errorf("Effort = %q, want high", got)
	}
}

func TestApplyDefaultParameters_DoesNotOverrideExistingEffort(t *testing.T) {
	effort := "medium"
	req := &schemas.BifrostChatRequest{
		Params: &schemas.ChatParameters{Reasoning: &schemas.ChatReasoning{Effort: &effort}},
	}
	ApplyDefaultParameters(schemas.Sensenova, "deepseek-v4-flash", req, map[string]interface{}{"reasoning_effort": "high"})
	if got := *req.Params.Reasoning.Effort; got != "medium" {
		t.Errorf("Effort = %q, want medium (existing value preserved)", got)
	}
}

func TestApplyDefaultParameters_NonStringValueIgnored(t *testing.T) {
	req := &schemas.BifrostChatRequest{}
	ApplyDefaultParameters(schemas.Sensenova, "deepseek-v4-flash", req, map[string]interface{}{"reasoning_effort": 42})
	if req.Params != nil {
		t.Error("Params should remain nil when the reasoning_effort value is not a string")
	}
}

func TestApplyDefaultParameters_UnknownKeyIgnored(t *testing.T) {
	req := &schemas.BifrostChatRequest{}
	ApplyDefaultParameters(schemas.Sensenova, "deepseek-v4-flash", req, map[string]interface{}{"temperature": 0.7})
	if req.Params != nil {
		t.Error("unknown params must not create Params")
	}
}

func TestApplyDefaultParameters_NilRequestAndEmptyDefaults(t *testing.T) {
	ApplyDefaultParameters(schemas.Sensenova, "deepseek-v4-flash", nil, map[string]interface{}{"reasoning_effort": "high"})
	ApplyDefaultParameters(schemas.Sensenova, "deepseek-v4-flash", &schemas.BifrostChatRequest{}, nil)
	ApplyDefaultParameters(schemas.Sensenova, "deepseek-v4-flash", &schemas.BifrostChatRequest{}, map[string]interface{}{})
	// Must not panic; nothing to assert beyond that.
}

func TestApplyDefaultParameters_ExistingReasoningKeepsOtherEffortNil(t *testing.T) {
	req := &schemas.BifrostChatRequest{
		Params: &schemas.ChatParameters{Reasoning: &schemas.ChatReasoning{}},
	}
	ApplyDefaultParameters(schemas.Sensenova, "deepseek-v4-flash", req, map[string]interface{}{"reasoning_effort": "low"})
	if req.Params.Reasoning.Effort == nil || *req.Params.Reasoning.Effort != "low" {
		t.Errorf("Effort = %v, want low", req.Params.Reasoning.Effort)
	}
}

func TestApplyDefaultParameters_UnregisteredProviderIgnored(t *testing.T) {
	// OpenAI has not registered reasoning_effort as a supported default param.
	req := &schemas.BifrostChatRequest{}
	ApplyDefaultParameters(schemas.OpenAI, "deepseek-v4-flash", req, map[string]interface{}{"reasoning_effort": "high"})
	if req.Params != nil {
		t.Error("Params should remain nil when the provider does not support the param")
	}
}

func TestApplyDefaultParameters_UnsupportedModelIgnored(t *testing.T) {
	// sensenova registers reasoning_effort only for deepseek-v4-flash / glm-5.2.
	// A different sensenova model must not get the injected default.
	req := &schemas.BifrostChatRequest{Model: "sensenova-v6"}
	ApplyDefaultParameters(schemas.Sensenova, "sensenova-v6", req, map[string]interface{}{"reasoning_effort": "high"})
	if req.Params != nil {
		t.Error("Params should remain nil when the model does not accept the param")
	}
}
