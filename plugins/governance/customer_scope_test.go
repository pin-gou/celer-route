package governance

import (
	"context"
	"testing"

	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/framework/configstore"
	configstoreTables "github.com/pin-gou/pg-gateway/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollectHierarchy_ScopedCustomerSkipsScalarTeamCustomer verifies that in OSS
// mode (without enterprise scoped customer), the scalar team customer is always charged.
func TestCollectHierarchy_ScopedCustomerSkipsScalarTeamCustomer(t *testing.T) {
	logger := NewMockLogger()

	teamBudget := buildBudgetWithUsage("team-budget", 500.0, 0.0, "1d")
	customerBudget := buildBudgetWithUsage("customer-budget", 1000.0, 0.0, "1d")
	vkBudget := buildBudgetWithUsage("vk-budget", 100.0, 0.0, "1d")

	customerRL := buildRateLimit("customer-rl", 1000, 1000)

	team := buildTeam("team1", "Team 1", teamBudget)
	customer := buildCustomer("customer1", "Customer 1", customerBudget)
	customer.RateLimitID = &customerRL.ID
	team.CustomerID = &customer.ID
	team.Customer = customer

	vk := buildVirtualKeyWithBudget("vk1", "sk-bf-test", "Test VK", vkBudget)
	vk.TeamID = &team.ID
	vk.Team = team

	store, err := NewLocalGovernanceStore(context.Background(), logger, nil, &configstore.GovernanceConfig{
		VirtualKeys: []configstoreTables.TableVirtualKey{*vk},
		Budgets:     []configstoreTables.TableBudget{*vkBudget, *teamBudget, *customerBudget},
		RateLimits:  []configstoreTables.TableRateLimit{*customerRL},
		Teams:       []configstoreTables.TableTeam{*team},
		Customers:   []configstoreTables.TableCustomer{*customer},
	}, nil)
	require.NoError(t, err)

	vk, _ = store.GetVirtualKey(context.Background(), "sk-bf-test")

	hasCustomerBudget := func(ctx context.Context) bool {
		for _, b := range store.collectBudgetsFromHierarchy(ctx, vk, schemas.OpenAI)["Customer"] {
			if b.ID == "customer-budget" {
				return true
			}
		}
		return false
	}
	hasCustomerRateLimit := func(ctx context.Context) bool {
		for _, rl := range store.collectRateLimitsFromHierarchy(ctx, vk, schemas.OpenAI)["Customer"] {
			if rl.ID == "customer-rl" {
				return true
			}
		}
		return false
	}

	t.Run("no scope charges the scalar team customer", func(t *testing.T) {
		assert.True(t, hasCustomerBudget(context.Background()))
		assert.True(t, hasCustomerRateLimit(context.Background()))
	})

	t.Run("scoped context (enterprise-only, no-op in OSS) charges the scalar team customer", func(t *testing.T) {
		assert.True(t, hasCustomerBudget(context.Background()))
		assert.True(t, hasCustomerRateLimit(context.Background()))
	})
}

// TestEvaluateGovernanceRequest_ScopedCustomerSkipsScalarTeamCustomerEnforcement verifies
// that in OSS mode (without enterprise scoped customer), the scalar team customer budget
// enforcement always applies.
func TestEvaluateGovernanceRequest_ScopedCustomerSkipsScalarTeamCustomerEnforcement(t *testing.T) {
	logger := NewMockLogger()

	teamBudget := buildBudgetWithUsage("team-budget", 1000.0, 0.0, "1d")
	customerBudget := buildBudgetWithUsage("customer-budget", 50.0, 100.0, "1d") // exceeded
	vkBudget := buildBudgetWithUsage("vk-budget", 1000.0, 0.0, "1d")

	team := buildTeam("team1", "Team 1", teamBudget)
	customer := buildCustomer("customer1", "Customer 1", customerBudget)
	team.CustomerID = &customer.ID
	team.Customer = customer

	vk := buildVirtualKeyWithBudget("vk1", "sk-bf-test", "Test VK", vkBudget)
	vk.TeamID = &team.ID
	vk.Team = team

	store, err := NewLocalGovernanceStore(context.Background(), logger, nil, &configstore.GovernanceConfig{
		VirtualKeys: []configstoreTables.TableVirtualKey{*vk},
		Budgets:     []configstoreTables.TableBudget{*vkBudget, *teamBudget, *customerBudget},
		Teams:       []configstoreTables.TableTeam{*team},
		Customers:   []configstoreTables.TableCustomer{*customer},
	}, nil)
	require.NoError(t, err)

	p := &GovernancePlugin{
		logger:   logger,
		store:    store,
		resolver: NewBudgetResolver(store, nil, logger, nil),
	}

	evaluate := func() *EvaluationResult {
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		res, _ := p.EvaluateGovernanceRequest(ctx, &EvaluationRequest{
			VirtualKey: "sk-bf-test",
			Provider:   schemas.OpenAI,
			Model:      "gpt-4o",
		}, schemas.ChatCompletionRequest)
		return res
	}

	t.Run("no scope enforces the scalar team customer", func(t *testing.T) {
		assert.Equal(t, DecisionBudgetExceeded, evaluate().Decision)
	})
}
