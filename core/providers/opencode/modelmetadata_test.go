package opencode

import "testing"

func TestGetModelTargetFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		// Claude-format models
		{"qwen3.7-max", "qwen3.7-max", targetFormatClaude},
		{"qwen3.7-plus", "qwen3.7-plus", targetFormatClaude},
		{"qwen3.6-plus-high", "qwen3.6-plus-high", targetFormatClaude},
		{"qwen3.7-max-max", "qwen3.7-max-max", targetFormatClaude},
		{"minimax-m2.7", "minimax-m2.7", targetFormatClaude},
		{"minimax-m3", "minimax-m3", targetFormatClaude},
		{"glm-5", "glm-5", targetFormatClaude},
		{"glm-5.2", "glm-5.2", targetFormatClaude},

		// OpenAI-format models
		{"gpt-5", "gpt-5", targetFormatOpenAI},
		{"claude-sonnet-4-5", "claude-sonnet-4-5", targetFormatOpenAI},
		{"big-pickle", "big-pickle", targetFormatOpenAI},
		{"kimi-k2.6", "kimi-k2.6", targetFormatOpenAI},
		{"deepseek-v4-flash", "deepseek-v4-flash", targetFormatOpenAI},
		{"empty", "", targetFormatOpenAI},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getModelTargetFormat(tc.in)
			if got != tc.want {
				t.Errorf("getModelTargetFormat(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsClaudeFormatModel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		model string
		want  bool
	}{
		{"qwen3.7-max", true},
		{"minimax-m3", true},
		{"glm-5.1", true},
		{"gpt-5", false},
		{"kimi-k2.6", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			if got := isClaudeFormatModel(tc.model); got != tc.want {
				t.Errorf("isClaudeFormatModel(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestIsTextOnlyModel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		model string
		want  bool
	}{
		{"qwen3.7-max", true},
		{"kimi-k2.6", true},
		{"deepseek-v4-flash", true},
		{"glm-5.2", true},
		{"gpt-5", false},
		{"claude-sonnet-4-5", false},
		{"minimax-m3", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			if got := isTextOnlyModel(tc.model); got != tc.want {
				t.Errorf("isTextOnlyModel(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestIsFreeOpencodeModel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		model string
		want  bool
	}{
		// Explicitly whitelisted free models (mirror of OmniRoute's list).
		{"big-pickle", true},
		// -free suffix models.
		{"deepseek-v4-flash-free", true},
		{"mimo-v2.5-free", true},
		{"hy3-free", true},
		{"nemotron-3-ultra-free", true},
		{"north-mini-code-free", true},
		{"laguna-s-2.1-free", true},
		{"nemotron-3.5-lightning-free", true},
		// Paid models must be excluded.
		{"claude-fable-5", false},
		{"gpt-5.5", false},
		{"glm-5.2", false},
		{"kimi-k3", false},
		{"deepseek-v4-pro", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			if got := isFreeOpencodeModel(tc.model); got != tc.want {
				t.Errorf("isFreeOpencodeModel(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}
