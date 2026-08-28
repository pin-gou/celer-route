package opencode

import (
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestParseEffortLevel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		in        string
		wantBase  string
		wantTier  string
		wantMatch bool
	}{
		// Known effort tiers
		{"deepseek-v4-pro-low", "deepseek-v4-pro-low", "deepseek-v4-pro", "low", true},
		{"deepseek-v4-pro-max", "deepseek-v4-pro-max", "deepseek-v4-pro", "max", true},
		{"deepseek-v4-flash-high", "deepseek-v4-flash-high", "deepseek-v4-flash", "high", true},
		{"glm-5.2-high", "glm-5.2-high", "glm-5.2", "high", true},
		{"qwen3.7-max-max", "qwen3.7-max-max", "qwen3.7-max", "max", true},
		{"hy3-none", "hy3-none", "hy3", "none", true},
		{"grok-4.5-medium", "grok-4.5-medium", "grok-4.5", "medium", true},

		// No tier (canonical id, no suffix)
		{"deepseek-v4-pro base", "deepseek-v4-pro", "deepseek-v4-pro", "", false},
		{"plain model", "gpt-5", "gpt-5", "", false},
		{"empty", "", "", "", false},

		// Unknown tier — must NOT match
		{"unknown tier", "deepseek-v4-pro-extreme", "deepseek-v4-pro-extreme", "", false},
		{"unknown base", "totally-fake-low", "totally-fake-low", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, tier, ok := parseEffortLevel(tc.in)
			if base != tc.wantBase || tier != tc.wantTier || ok != tc.wantMatch {
				t.Errorf("parseEffortLevel(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.in, base, tier, ok, tc.wantBase, tc.wantTier, tc.wantMatch)
			}
		})
	}
}

func TestPrepareOpencodeRequest(t *testing.T) {
	t.Parallel()

	t.Run("nil request returns nil", func(t *testing.T) {
		if got := prepareOpencodeRequest(nil); got != nil {
			t.Errorf("expected nil, got %#v", got)
		}
	})

	t.Run("strips client_metadata from ExtraParams", func(t *testing.T) {
		req := &schemas.BifrostChatRequest{
			Model: "gpt-5",
			Params: &schemas.ChatParameters{
				ExtraParams: map[string]interface{}{
					"client_metadata": map[string]string{"k": "v"},
					"other":           "keep",
				},
			},
		}
		out := prepareOpencodeRequest(req)
		if _, ok := out.Params.ExtraParams["client_metadata"]; ok {
			t.Errorf("client_metadata still in ExtraParams")
		}
		if out.Params.ExtraParams["other"] != "keep" {
			t.Errorf("other ExtraParams disturbed")
		}
	})

	t.Run("rewrites effort-tier model and injects reasoning_effort", func(t *testing.T) {
		req := &schemas.BifrostChatRequest{
			Model: "deepseek-v4-pro-high",
		}
		out := prepareOpencodeRequest(req)
		if out.Model != "deepseek-v4-pro" {
			t.Errorf("model not rewritten: %s", out.Model)
		}
		if out.Params == nil || out.Params.Reasoning == nil || out.Params.Reasoning.Effort == nil {
			t.Fatalf("reasoning_effort not set")
		}
		if *out.Params.Reasoning.Effort != "high" {
			t.Errorf("reasoning_effort = %s, want high", *out.Params.Reasoning.Effort)
		}
	})

	t.Run("preserves caller reasoning_effort", func(t *testing.T) {
		callerEffort := "low"
		req := &schemas.BifrostChatRequest{
			Model: "deepseek-v4-pro-high",
			Params: &schemas.ChatParameters{
				Reasoning: &schemas.ChatReasoning{
					Effort: &callerEffort,
				},
			},
		}
		out := prepareOpencodeRequest(req)
		if out.Params.Reasoning == nil || out.Params.Reasoning.Effort == nil || *out.Params.Reasoning.Effort != "low" {
			t.Errorf("caller reasoning_effort overwritten")
		}
	})

	t.Run("truncates tools above 128", func(t *testing.T) {
		tools := make([]schemas.ChatTool, maxToolsCount+10)
		for i := range tools {
			tools[i] = schemas.ChatTool{
				Type: "function",
				Function: &schemas.ChatToolFunction{
					Name: "tool",
				},
			}
		}
		req := &schemas.BifrostChatRequest{
			Model: "gpt-5",
			Params: &schemas.ChatParameters{
				Tools: tools,
			},
		}
		out := prepareOpencodeRequest(req)
		if len(out.Params.Tools) != maxToolsCount {
			t.Errorf("tools = %d, want %d", len(out.Params.Tools), maxToolsCount)
		}
	})

	t.Run("downgrades deepseek-v4-flash-free json_schema", func(t *testing.T) {
		reqFormat := map[string]interface{}{
			"type":        "json_schema",
			"json_schema": map[string]interface{}{"schema": map[string]interface{}{}},
		}
		wrapped := interface{}(reqFormat)
		req := &schemas.BifrostChatRequest{
			Model: "deepseek-v4-flash-free",
			Params: &schemas.ChatParameters{
				ResponseFormat: &wrapped,
			},
		}
		out := prepareOpencodeRequest(req)
		if out.Params.ResponseFormat == nil {
			t.Fatalf("response_format nil")
		}
		got, ok := (*out.Params.ResponseFormat).(map[string]interface{})
		if !ok {
			t.Fatalf("response_format not a map: %T", *out.Params.ResponseFormat)
		}
		if got["type"] != "json_object" {
			t.Errorf("response_format.type = %v, want json_object", got["type"])
		}
	})

	t.Run("plain model passes through unchanged", func(t *testing.T) {
		req := &schemas.BifrostChatRequest{Model: "gpt-5"}
		out := prepareOpencodeRequest(req)
		if out.Model != "gpt-5" {
			t.Errorf("model changed: %s", out.Model)
		}
	})

	t.Run("no client_metadata leaves ExtraParams untouched", func(t *testing.T) {
		req := &schemas.BifrostChatRequest{
			Model: "gpt-5",
			Params: &schemas.ChatParameters{
				ExtraParams: map[string]interface{}{
					"safe_field": "v",
				},
			},
		}
		out := prepareOpencodeRequest(req)
		if out.Params.ExtraParams["safe_field"] != "v" {
			t.Errorf("ExtraParams disturbed")
		}
	})

	t.Run("does not mutate caller request", func(t *testing.T) {
		origExtra := map[string]interface{}{
			"client_metadata": "x",
			"keep":            "y",
		}
		req := &schemas.BifrostChatRequest{
			Model: "deepseek-v4-pro-high",
			Params: &schemas.ChatParameters{
				ExtraParams: origExtra,
			},
		}
		_ = prepareOpencodeRequest(req)

		// The caller's model must be unchanged.
		if req.Model != "deepseek-v4-pro-high" {
			t.Errorf("caller model mutated: %s", req.Model)
		}
		// The caller's ExtraParams must still carry client_metadata.
		if _, ok := req.Params.ExtraParams["client_metadata"]; !ok {
			t.Errorf("caller ExtraParams mutated (client_metadata removed)")
		}
		// The caller must not have gained a reasoning effort.
		if req.Params.Reasoning != nil && req.Params.Reasoning.Effort != nil {
			t.Errorf("caller Reasoning mutated")
		}
	})
}
