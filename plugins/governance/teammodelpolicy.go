package governance

import (
	"github.com/pin-gou/celer-route/core/schemas"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
)

// TeamPolicyAllowsModel is the single implementation of the D6 team layer: it
// answers "does this team's policy for one provider permit this model?".
//
// Both the per-request gate (BudgetResolver.isModelAllowed) and the GET
// /v1/models listing filter go through it, so the model list a member sees can
// never advertise a model their requests would be denied. Keeping the predicate
// in one place is the whole point — the two callers must not be free to drift.
//
// Semantics:
//   - nil policy or IsUnrestricted() -> allowed ("inherit global")
//   - model in BlacklistedModels     -> denied (blacklist wins over any allow)
//   - otherwise                      -> allowed iff AllowedModels permits it
//
// An empty AllowedModels denies everything, matching the deny-by-default
// convention VK provider configs already use (schemas.WhiteList documents "empty
// list means nothing is allowed"). A consequence worth knowing: a policy that
// sets only BlacklistedModels blocks the entire provider, not just the listed
// models. See TeamPolicyDeniesAllModels.
func TeamPolicyAllowsModel(policy *configstoreTables.TableTeamModelPolicy, model string) bool {
	if policy == nil || policy.IsUnrestricted() {
		return true
	}
	if schemas.BlackList(policy.BlacklistedModels).IsBlocked(model) {
		return false
	}
	return schemas.WhiteList(policy.AllowedModels).IsAllowed(model)
}

// TeamPolicyDeniesAllModels reports whether a policy makes every model on its
// provider unreachable. Callers use it to drop the provider from a fan-out
// entirely instead of querying it and discarding the whole result.
//
// Only an empty allowlist can deny wholesale: a non-empty allowlist always
// permits its own entries. A wildcard allowlist combined with a blacklist can
// still block everything in principle, but that is decided per model, so this
// conservatively returns false and the caller falls back to per-model filtering.
func TeamPolicyDeniesAllModels(policy *configstoreTables.TableTeamModelPolicy) bool {
	if policy == nil || policy.IsUnrestricted() {
		return false
	}
	return len(policy.AllowedModels) == 0
}

// FilterModelsByTeamPolicy narrows a candidate model list to the models the
// team policy permits. The input slice is returned untouched when the policy
// places no restriction, so the common "inherit global" case allocates nothing.
func FilterModelsByTeamPolicy(models []string, policy *configstoreTables.TableTeamModelPolicy) []string {
	if policy == nil || policy.IsUnrestricted() {
		return models
	}
	kept := make([]string, 0, len(models))
	for _, model := range models {
		if TeamPolicyAllowsModel(policy, model) {
			kept = append(kept, model)
		}
	}
	return kept
}
