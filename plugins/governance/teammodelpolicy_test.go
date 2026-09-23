package governance

import (
	"testing"

	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
)

// TestTeamPolicyAllowsModel pins the D6 team-layer predicate. This is the single
// implementation shared by the per-request gate (BudgetResolver.isModelAllowed)
// and the GET /v1/models listing filter, so the cases below are the contract
// both callers rely on.
func TestTeamPolicyAllowsModel(t *testing.T) {
	tests := []struct {
		name    string
		policy  *configstoreTables.TableTeamModelPolicy
		model   string
		allowed bool
	}{
		{
			name:    "nil policy inherits global",
			policy:  nil,
			model:   "gpt-4o",
			allowed: true,
		},
		{
			name: "unrestricted policy inherits global",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider: "openai",
			},
			model:   "gpt-4o",
			allowed: true,
		},
		{
			name: "wildcard allowlist permits any model",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:      "openai",
				AllowedModels: []string{"*"},
			},
			model:   "gpt-4o",
			allowed: true,
		},
		{
			name: "listed model is permitted",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:      "openai",
				AllowedModels: []string{"gpt-4o", "gpt-4o-mini"},
			},
			model:   "gpt-4o-mini",
			allowed: true,
		},
		{
			name: "unlisted model is denied",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:      "openai",
				AllowedModels: []string{"gpt-4o-mini"},
			},
			model:   "gpt-4o",
			allowed: false,
		},
		{
			name: "allowlist matching is case-insensitive",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:      "openai",
				AllowedModels: []string{"GPT-4o"},
			},
			model:   "gpt-4o",
			allowed: true,
		},
		{
			name: "blacklist wins over a wildcard allowlist",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:          "openai",
				AllowedModels:     []string{"*"},
				BlacklistedModels: []string{"o1"},
			},
			model:   "o1",
			allowed: false,
		},
		{
			name: "blacklist wins over an explicit allowlist entry",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:          "openai",
				AllowedModels:     []string{"gpt-4o", "o1"},
				BlacklistedModels: []string{"o1"},
			},
			model:   "o1",
			allowed: false,
		},
		{
			// Documents the deny-by-default consequence of an empty allowlist: a
			// policy that sets ONLY a blacklist blocks the whole provider, not
			// just the listed models. This matches schemas.WhiteList ("empty list
			// means nothing is allowed") and the VK provider-config convention.
			name: "blacklist-only policy denies models outside the blacklist too",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:          "openai",
				BlacklistedModels: []string{"o1"},
			},
			model:   "gpt-4o",
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.allowed, TeamPolicyAllowsModel(tt.policy, tt.model), "model=%s", tt.model)
		})
	}
}

// TestTeamPolicyDeniesAllModels pins the fan-out optimization: a provider whose
// team policy permits nothing is dropped before it is ever queried.
func TestTeamPolicyDeniesAllModels(t *testing.T) {
	tests := []struct {
		name      string
		policy    *configstoreTables.TableTeamModelPolicy
		deniesAll bool
	}{
		{name: "nil policy", policy: nil, deniesAll: false},
		{
			name:      "unrestricted policy",
			policy:    &configstoreTables.TableTeamModelPolicy{Provider: "openai"},
			deniesAll: false,
		},
		{
			name: "non-empty allowlist permits at least its own entries",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:      "openai",
				AllowedModels: []string{"gpt-4o"},
			},
			deniesAll: false,
		},
		{
			name: "wildcard allowlist with blacklist is decided per model",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:          "openai",
				AllowedModels:     []string{"*"},
				BlacklistedModels: []string{"*"},
			},
			deniesAll: false,
		},
		{
			name: "blacklist-only policy denies the whole provider",
			policy: &configstoreTables.TableTeamModelPolicy{
				Provider:          "openai",
				BlacklistedModels: []string{"o1"},
			},
			deniesAll: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.deniesAll, TeamPolicyDeniesAllModels(tt.policy))
		})
	}
}

// TestFilterModelsByTeamPolicy covers the list helper, including the
// zero-allocation "inherit global" fast path.
func TestFilterModelsByTeamPolicy(t *testing.T) {
	models := []string{"gpt-4o", "gpt-4o-mini", "o1"}

	t.Run("nil policy returns the input slice unchanged", func(t *testing.T) {
		got := FilterModelsByTeamPolicy(models, nil)
		assert.Equal(t, models, got)
	})

	t.Run("unrestricted policy returns the input slice unchanged", func(t *testing.T) {
		got := FilterModelsByTeamPolicy(models, &configstoreTables.TableTeamModelPolicy{Provider: "openai"})
		assert.Equal(t, models, got)
	})

	t.Run("allowlist narrows to the intersection", func(t *testing.T) {
		got := FilterModelsByTeamPolicy(models, &configstoreTables.TableTeamModelPolicy{
			Provider:      "openai",
			AllowedModels: []string{"gpt-4o", "o1"},
		})
		assert.Equal(t, []string{"gpt-4o", "o1"}, got)
	})

	t.Run("blacklist removes blocked models", func(t *testing.T) {
		got := FilterModelsByTeamPolicy(models, &configstoreTables.TableTeamModelPolicy{
			Provider:          "openai",
			AllowedModels:     []string{"*"},
			BlacklistedModels: []string{"o1"},
		})
		assert.Equal(t, []string{"gpt-4o", "gpt-4o-mini"}, got)
	})

	t.Run("empty allowlist filters everything out", func(t *testing.T) {
		got := FilterModelsByTeamPolicy(models, &configstoreTables.TableTeamModelPolicy{
			Provider:          "openai",
			BlacklistedModels: []string{"o1"},
		})
		assert.Empty(t, got)
	})
}
