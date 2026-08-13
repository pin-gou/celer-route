package schemas

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGuardrailDebugContextRoundTrip verifies that in OSS the guardrail debug
// surface is a pure no-op: AppendGuardrailJudgeCallOnContext returns false
// and GuardrailDebugFromContext returns (nil, false).
func TestGuardrailDebugContextRoundTrip(t *testing.T) {
	ctx := NewBifrostContext(nil, NoDeadline)
	call := BifrostGuardrailJudgeCall{
		Phase:         "input",
		RuleName:      "pii",
		JudgeProvider: OpenAI,
		JudgeModel:    "gpt-4o-mini",
		PromptTokens:  12,
		TotalTokens:   12,
	}

	assert.False(t, AppendGuardrailJudgeCallOnContext(ctx, call))
	_, ok := GuardrailDebugFromContext(ctx)
	assert.False(t, ok)
}

// TestGuardrailDebugContextReturnsOwnedSnapshot verifies that in OSS
// GuardrailDebugFromContext always returns (nil, false).
func TestGuardrailDebugContextReturnsOwnedSnapshot(t *testing.T) {
	ctx := NewBifrostContext(nil, NoDeadline)

	_, ok := GuardrailDebugFromContext(ctx)
	assert.False(t, ok)
}

// TestGuardrailDebugContextClonesUsageDetails verifies that in OSS no usage
// details are ever stored on context.
func TestGuardrailDebugContextClonesUsageDetails(t *testing.T) {
	ctx := NewBifrostContext(nil, NoDeadline)
	assert.False(t, AppendGuardrailJudgeCallOnContext(ctx, BifrostGuardrailJudgeCall{
		JudgeProvider: OpenAI,
		JudgeModel:    "gpt-4o-mini",
		PromptTokens:  10,
		PromptTokensDetails: &ChatPromptTokensDetails{
			CachedWriteTokenDetails: &ChatCachedWriteTokenDetails{CachedWriteTokens5m: 4},
		},
		CompletionTokens: 5,
		CompletionTokensDetails: &ChatCompletionTokensDetails{
			CitationTokens: func() *int { v := 3; return &v }(),
		},
		TotalTokens: 15,
	}))

	_, ok := GuardrailDebugFromContext(ctx)
	assert.False(t, ok)
}

// TestAppendGuardrailJudgeCallRejectsEmptyUsage verifies that non-billable calls
// are rejected (which in OSS is always, since no guardrail can be stored).
func TestAppendGuardrailJudgeCallRejectsEmptyUsage(t *testing.T) {
	ctx := NewBifrostContext(nil, NoDeadline)
	assert.False(t, AppendGuardrailJudgeCallOnContext(ctx, BifrostGuardrailJudgeCall{}))
	_, ok := GuardrailDebugFromContext(ctx)
	assert.False(t, ok)
}
