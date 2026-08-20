package mocker

import (
	"testing"
)

func TestInit_DefaultBehavior_Passthrough(t *testing.T) {
	plugin, err := Init(MockerConfig{Enabled: true})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if plugin.config.DefaultBehavior != DefaultBehaviorPassthrough {
		t.Errorf("DefaultBehavior: expected %q, got %q", DefaultBehaviorPassthrough, plugin.config.DefaultBehavior)
	}
}

func TestInit_DefaultBehavior_Explicit(t *testing.T) {
	plugin, err := Init(MockerConfig{Enabled: true, DefaultBehavior: DefaultBehaviorError})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if plugin.config.DefaultBehavior != DefaultBehaviorError {
		t.Errorf("DefaultBehavior: expected %q, got %q", DefaultBehaviorError, plugin.config.DefaultBehavior)
	}
}

func TestInit_DefaultBehavior_EmptyConfig(t *testing.T) {
	// Disabled plugin with empty config — Init should not error since no rules are enforced
	plugin, err := Init(MockerConfig{})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if plugin.config.DefaultBehavior != DefaultBehaviorPassthrough {
		t.Errorf("DefaultBehavior: expected %q, got %q", DefaultBehaviorPassthrough, plugin.config.DefaultBehavior)
	}
}

func TestValidateRule_Priority_LowerBound(t *testing.T) {
	// Priority -1000 is allowed
	err := validateRule(MockRule{
		Name:        "test",
		Priority:    -1000,
		Probability: 1.0,
		Responses: []Response{
			{Type: ResponseTypeSuccess, Weight: 1.0, Content: &SuccessResponse{Message: "ok"}},
		},
	})
	if err != nil {
		t.Errorf("expected no error for Priority=-1000, got: %v", err)
	}
}

func TestValidateRule_Priority_AboveUpperBound(t *testing.T) {
	// Priority 1001 is out of range
	err := validateRule(MockRule{
		Name:        "test",
		Priority:    1001,
		Probability: 1.0,
		Responses: []Response{
			{Type: ResponseTypeSuccess, Weight: 1.0, Content: &SuccessResponse{Message: "ok"}},
		},
	})
	if err == nil {
		t.Error("expected error for Priority=1001")
	}
}

func TestValidateRule_Priority_BelowLowerBound(t *testing.T) {
	err := validateRule(MockRule{
		Name:        "test",
		Priority:    -1001,
		Probability: 1.0,
		Responses: []Response{
			{Type: ResponseTypeSuccess, Weight: 1.0, Content: &SuccessResponse{Message: "ok"}},
		},
	})
	if err == nil {
		t.Error("expected error for Priority=-1001")
	}
}

func TestValidateRule_Priority_UpperBound(t *testing.T) {
	// Priority 1000 is allowed
	err := validateRule(MockRule{
		Name:        "test",
		Priority:    1000,
		Probability: 1.0,
		Responses: []Response{
			{Type: ResponseTypeSuccess, Weight: 1.0, Content: &SuccessResponse{Message: "ok"}},
		},
	})
	if err != nil {
		t.Errorf("expected no error for Priority=1000, got: %v", err)
	}
}

func TestValidateRule_Probability_Zero(t *testing.T) {
	err := validateRule(MockRule{
		Name:        "test",
		Priority:    0,
		Probability: 0.0,
		Responses: []Response{
			{Type: ResponseTypeSuccess, Weight: 1.0, Content: &SuccessResponse{Message: "ok"}},
		},
	})
	if err != nil {
		t.Errorf("expected no error for Probability=0.0, got: %v", err)
	}
}

func TestValidateRule_Probability_One(t *testing.T) {
	err := validateRule(MockRule{
		Name:        "test",
		Priority:    0,
		Probability: 1.0,
		Responses: []Response{
			{Type: ResponseTypeSuccess, Weight: 1.0, Content: &SuccessResponse{Message: "ok"}},
		},
	})
	if err != nil {
		t.Errorf("expected no error for Probability=1.0, got: %v", err)
	}
}

func TestValidateRule_Probability_BelowZero(t *testing.T) {
	err := validateRule(MockRule{
		Name:        "test",
		Priority:    0,
		Probability: -0.1,
		Responses: []Response{
			{Type: ResponseTypeSuccess, Weight: 1.0, Content: &SuccessResponse{Message: "ok"}},
		},
	})
	if err == nil {
		t.Error("expected error for Probability=-0.1")
	}
}

func TestValidateRule_Probability_AboveOne(t *testing.T) {
	err := validateRule(MockRule{
		Name:        "test",
		Priority:    0,
		Probability: 1.1,
		Responses: []Response{
			{Type: ResponseTypeSuccess, Weight: 1.0, Content: &SuccessResponse{Message: "ok"}},
		},
	})
	if err == nil {
		t.Error("expected error for Probability=1.1")
	}
}

func TestValidateRule_EmptyName(t *testing.T) {
	err := validateRule(MockRule{
		Name:        "",
		Priority:    0,
		Probability: 1.0,
		Responses: []Response{
			{Type: ResponseTypeSuccess, Weight: 1.0, Content: &SuccessResponse{Message: "ok"}},
		},
	})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestValidateRule_EmptyResponses(t *testing.T) {
	err := validateRule(MockRule{
		Name:        "test",
		Priority:    0,
		Probability: 1.0,
		Responses:   []Response{},
	})
	if err == nil {
		t.Error("expected error for empty responses")
	}
}

func TestValidateErrorResponse_StatusCode_LowerBound(t *testing.T) {
	code := 100
	err := validateErrorResponse(ErrorResponse{
		Message:    "error",
		StatusCode: &code,
	})
	if err != nil {
		t.Errorf("expected no error for StatusCode=100, got: %v", err)
	}
}

func TestValidateErrorResponse_StatusCode_UpperBound(t *testing.T) {
	code := 599
	err := validateErrorResponse(ErrorResponse{
		Message:    "error",
		StatusCode: &code,
	})
	if err != nil {
		t.Errorf("expected no error for StatusCode=599, got: %v", err)
	}
}

func TestValidateErrorResponse_StatusCode_BelowRange(t *testing.T) {
	code := 99
	err := validateErrorResponse(ErrorResponse{
		Message:    "error",
		StatusCode: &code,
	})
	if err == nil {
		t.Error("expected error for StatusCode=99")
	}
}

func TestValidateErrorResponse_StatusCode_AboveRange(t *testing.T) {
	code := 600
	err := validateErrorResponse(ErrorResponse{
		Message:    "error",
		StatusCode: &code,
	})
	if err == nil {
		t.Error("expected error for StatusCode=600")
	}
}

func TestValidateErrorResponse_EmptyMessage(t *testing.T) {
	err := validateErrorResponse(ErrorResponse{
		Message: "",
	})
	if err == nil {
		t.Error("expected error for empty message")
	}
}