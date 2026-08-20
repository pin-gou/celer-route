package compat

import (
	"encoding/json"
	"testing"
)

func TestConfig_UnmarshalJSON_AllAbsent(t *testing.T) {
	// All 4 fields absent — each should default to true
	data := `{}`
	var cfg Config
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if !cfg.ConvertTextToChat {
		t.Error("ConvertTextToChat: expected true when absent")
	}
	if !cfg.ConvertChatToResponses {
		t.Error("ConvertChatToResponses: expected true when absent")
	}
	if !cfg.ShouldDropParams {
		t.Error("ShouldDropParams: expected true when absent")
	}
	if !cfg.ShouldConvertParams {
		t.Error("ShouldConvertParams: expected true when absent")
	}
}

func TestConfig_UnmarshalJSON_AllExplicitlyTrue(t *testing.T) {
	data := `{
		"convert_text_to_chat": true,
		"convert_chat_to_responses": true,
		"should_drop_params": true,
		"should_convert_params": true
	}`
	var cfg Config
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if !cfg.ConvertTextToChat {
		t.Error("ConvertTextToChat: expected true")
	}
	if !cfg.ConvertChatToResponses {
		t.Error("ConvertChatToResponses: expected true")
	}
	if !cfg.ShouldDropParams {
		t.Error("ShouldDropParams: expected true")
	}
	if !cfg.ShouldConvertParams {
		t.Error("ShouldConvertParams: expected true")
	}
}

func TestConfig_UnmarshalJSON_AllExplicitlyFalse(t *testing.T) {
	data := `{
		"convert_text_to_chat": false,
		"convert_chat_to_responses": false,
		"should_drop_params": false,
		"should_convert_params": false
	}`
	var cfg Config
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if cfg.ConvertTextToChat {
		t.Error("ConvertTextToChat: expected false")
	}
	if cfg.ConvertChatToResponses {
		t.Error("ConvertChatToResponses: expected false")
	}
	if cfg.ShouldDropParams {
		t.Error("ShouldDropParams: expected false")
	}
	if cfg.ShouldConvertParams {
		t.Error("ShouldConvertParams: expected false")
	}
}

func TestConfig_UnmarshalJSON_Mixed(t *testing.T) {
	data := `{
		"convert_text_to_chat": true,
		"convert_chat_to_responses": false,
		"should_drop_params": true,
		"should_convert_params": false
	}`
	var cfg Config
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if !cfg.ConvertTextToChat {
		t.Error("ConvertTextToChat: expected true")
	}
	if cfg.ConvertChatToResponses {
		t.Error("ConvertChatToResponses: expected false")
	}
	if !cfg.ShouldDropParams {
		t.Error("ShouldDropParams: expected true")
	}
	if cfg.ShouldConvertParams {
		t.Error("ShouldConvertParams: expected false")
	}
}

func TestIsEnabled_AllTrue(t *testing.T) {
	cfg := Config{ConvertTextToChat: true, ConvertChatToResponses: true, ShouldDropParams: true, ShouldConvertParams: true}
	if !cfg.IsEnabled() {
		t.Error("IsEnabled: expected true when all are true")
	}
}

func TestIsEnabled_OneTrue(t *testing.T) {
	cfg := Config{ConvertTextToChat: true, ConvertChatToResponses: false, ShouldDropParams: false, ShouldConvertParams: false}
	if !cfg.IsEnabled() {
		t.Error("IsEnabled: expected true when ConvertTextToChat is true")
	}

	cfg = Config{ConvertTextToChat: false, ConvertChatToResponses: true, ShouldDropParams: false, ShouldConvertParams: false}
	if !cfg.IsEnabled() {
		t.Error("IsEnabled: expected true when ConvertChatToResponses is true")
	}

	cfg = Config{ConvertTextToChat: false, ConvertChatToResponses: false, ShouldDropParams: true, ShouldConvertParams: false}
	if !cfg.IsEnabled() {
		t.Error("IsEnabled: expected true when ShouldDropParams is true")
	}

	cfg = Config{ConvertTextToChat: false, ConvertChatToResponses: false, ShouldDropParams: false, ShouldConvertParams: true}
	if !cfg.IsEnabled() {
		t.Error("IsEnabled: expected true when ShouldConvertParams is true")
	}
}

func TestIsEnabled_AllFalse(t *testing.T) {
	cfg := Config{ConvertTextToChat: false, ConvertChatToResponses: false, ShouldDropParams: false, ShouldConvertParams: false}
	if cfg.IsEnabled() {
		t.Error("IsEnabled: expected false when all are false")
	}
}

func TestIsEnabled_AfterUnmarshalEmptyJSON(t *testing.T) {
	// After UnmarshalJSON with empty JSON, all fields default to true
	data := `{}`
	var cfg Config
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if !cfg.IsEnabled() {
		t.Error("IsEnabled: expected true after unmarshaling empty JSON (all default to true)")
	}
}

func TestIsEnabled_ZeroValue(t *testing.T) {
	// A zero-value Config struct (not unmarshaled) has all fields false
	cfg := Config{}
	if cfg.IsEnabled() {
		t.Error("IsEnabled: expected false for zero-value Config (all fields are false)")
	}
}