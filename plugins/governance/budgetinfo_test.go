package governance

import (
	"context"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
	configstore "github.com/pin-gou/celer-route/framework/configstore"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BudgetInfo is the contract between the resolver (producer) and two
// consumers: EvaluateSoftThresholds (alert_evaluator.go:67, early-returns when
// empty) and the case DecisionBudgetExceeded handler in main.go (which loops
// over it to EnqueueBudgetExceeded).
//
// Before this fix BudgetInfo was declared (resolver.go:44) and read by both
// consumers, but nothing in non-test code ever assigned it. The result: every
// soft-threshold alert rule was dead, and the budget.exceeded webhook enqueue
// path — the only signal an operator gets when a hard block starts answering
// 402 — never fired regardless of how many endpoints subscribed.
//
// These tests pin both the violation-path (M-2) and the allow-path (soft
// threshold) producers, plus the provider/model-scoped path that the live
// webhook reproduction actually exercises.

// TestEvaluateVirtualKeyRequestPopulatesBudgetInfoOnViolation pins M-2: when a
// VK-scoped budget is exceeded, the returned EvaluationResult must carry the
// violated budget so the alert loop can EnqueueBudgetExceeded. Without this
// the consumer (main.go:1118) iterates over an empty slice and no webhook
// job is ever queued.
func TestEvaluateVirtualKeyRequestPopulatesBudgetInfoOnViolation(t *testing.T) {
	logger := NewMockLogger()
	vkBudget := buildBudgetWithUsage("vk-budget1", 100.0, 100.1, "1h") // over the limit
	vk := buildVirtualKeyWithBudget("vk1", "sk-bf-test", "VK with a budget", vkBudget)
	store, err := NewLocalGovernanceStore(context.Background(), logger, nil, &configstore.GovernanceConfig{
		VirtualKeys: []configstoreTables.TableVirtualKey{*vk},
		Budgets:     []configstoreTables.TableBudget{*vkBudget},
	}, nil)
	require.NoError(t, err)

	resolver := NewBudgetResolver(store, nil, logger, nil)
	ctx := &schemas.BifrostContext{}

	result := resolver.EvaluateVirtualKeyRequest(ctx, "sk-bf-test", schemas.OpenAI, "gpt-4", schemas.ChatCompletionRequest, false)

	assertDecision(t, DecisionBudgetExceeded, result)
	require.NotEmpty(t, result.BudgetInfo,
		"BudgetInfo must carry the violated budget so the alert loop can notify; "+
			"a missing slice makes EnqueueBudgetExceeded unreachable (M-2)")
	gotIDs := budgetIDs(result.BudgetInfo)
	assert.Contains(t, gotIDs, vkBudget.ID,
		"the violated VK budget must be present in BudgetInfo; got %v", gotIDs)
}

// TestEvaluateVirtualKeyRequestPopulatesBudgetInfoOnAllow pins the soft
// threshold producer: even when the request is allowed, AlertEvaluator feeds
// the result's BudgetInfo into ListAlertRulesForScope. If the allow path
// leaves the slice empty, sub-100%% alert rules never fire on successful
// requests — which is exactly the case where they are most useful.
func TestEvaluateVirtualKeyRequestPopulatesBudgetInfoOnAllow(t *testing.T) {
	logger := NewMockLogger()
	vkBudget := buildBudgetWithUsage("vk-budget1", 100.0, 50.0, "1h") // half-used, NOT exceeded
	vk := buildVirtualKeyWithBudget("vk1", "sk-bf-test", "VK with a budget", vkBudget)
	store, err := NewLocalGovernanceStore(context.Background(), logger, nil, &configstore.GovernanceConfig{
		VirtualKeys: []configstoreTables.TableVirtualKey{*vk},
		Budgets:     []configstoreTables.TableBudget{*vkBudget},
	}, nil)
	require.NoError(t, err)

	resolver := NewBudgetResolver(store, nil, logger, nil)
	ctx := &schemas.BifrostContext{}

	result := resolver.EvaluateVirtualKeyRequest(ctx, "sk-bf-test", schemas.OpenAI, "gpt-4", schemas.ChatCompletionRequest, false)

	assertDecision(t, DecisionAllow, result)
	require.NotEmpty(t, result.BudgetInfo,
		"BudgetInfo must carry the applicable budgets on the allow path so "+
			"soft-threshold rules can fire on successful requests; an empty "+
			"slice silences sub-100%% alerting entirely")
	gotIDs := budgetIDs(result.BudgetInfo)
	assert.Contains(t, gotIDs, vkBudget.ID,
		"the VK's budget must be present in BudgetInfo; got %v", gotIDs)
}

// TestEvaluateModelAndProviderRequestPopulatesBudgetInfoOnViolation pins the
// model-scoped path: when the violation comes from a model-level budget (the
// path the live gateway exercises), BudgetInfo must still carry the violated
// row. Without this, the 402 response will never trigger the budget.exceeded
// webhook.
func TestEvaluateModelAndProviderRequestPopulatesBudgetInfoOnViolation(t *testing.T) {
	logger := NewMockLogger()
	modelBudget := buildBudgetWithUsage("model-budget1", 50.0, 50.5, "1h") // over the limit
	modelConfig := buildModelConfig("mc1", "gpt-4", nil, modelBudget, nil)
	store, err := NewLocalGovernanceStore(context.Background(), logger, nil, &configstore.GovernanceConfig{
		ModelConfigs: []configstoreTables.TableModelConfig{*modelConfig},
		Budgets:      []configstoreTables.TableBudget{*modelBudget},
	}, nil)
	require.NoError(t, err)

	resolver := NewBudgetResolver(store, nil, logger, nil)
	ctx := &schemas.BifrostContext{}

	result := resolver.EvaluateModelAndProviderRequest(ctx, schemas.OpenAI, "gpt-4")

	assertDecision(t, DecisionBudgetExceeded, result)
	require.NotEmpty(t, result.BudgetInfo,
		"a model-level budget violation must populate BudgetInfo so the alert "+
			"loop can notify; an empty slice is exactly the pre-fix silent failure")
	gotIDs := budgetIDs(result.BudgetInfo)
	assert.Contains(t, gotIDs, modelBudget.ID,
		"the violated model budget must be present; got %v", gotIDs)
}

func budgetIDs(rows []*configstoreTables.TableBudget) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r != nil {
			out = append(out, r.ID)
		}
	}
	return out
}
