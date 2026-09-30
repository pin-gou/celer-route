package handlers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	governanceplugin "github.com/pin-gou/celer-route/plugins/governance"
	"github.com/valyala/fasthttp"
)

// listModelsResolvedVKKey is the bifrost context key under which
// applyListModelsVirtualKeyProviderFilter stashes the resolved *TableVirtualKey
// for the duration of a single GET /v1/models call. The backfill stage reads
// it to compute the routing-rule scope chain without re-querying the config
// store.
//
// Kept handler-local (not on schemas.BifrostContextKey) because the value
// type is a concrete configstore row and core/schemas must stay free of that
// dependency.
var listModelsResolvedVKKey schemas.BifrostContextKey = "list-models-resolved-virtual-key"

// listModelsTeamPoliciesKey is the bifrost context key under which
// applyListModelsVirtualKeyProviderFilter stashes the resolved team model
// policies (Phase 6 / D6) for the duration of a single GET /v1/models call.
// The response filter reads it back to narrow the model list.
//
// The value is a map[string]tables.TableTeamModelPolicy keyed by lowercased
// provider name. It is bounded by the number of providers an admin wrote a
// policy for — a small, request-independent handle, not per-request payload,
// so keeping it on the context is within the "small handles only" rule.
var listModelsTeamPoliciesKey schemas.BifrostContextKey = "list-models-team-model-policies"

// applyListModelsVirtualKeyProviderFilter narrows provider fan-out for GET /v1/models
// when the request is made with a virtual key. Without this, ListAllModels asks every
// configured provider to list models and governance rejects providers outside the VK,
// creating noisy, expected errors in request logs.
func (h *CompletionHandler) applyListModelsVirtualKeyProviderFilter(ctx *fasthttp.RequestCtx, bifrostCtx *schemas.BifrostContext) bool {
	vkValue := governanceplugin.ParseVirtualKeyFromFastHTTPRequest(ctx)
	if vkValue == nil {
		return true
	}

	trimmedVKValue := strings.TrimSpace(*vkValue)
	if trimmedVKValue == "" {
		return true
	}

	if h.config == nil || h.config.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "database store unavailable")
		return false
	}

	vk, err := h.config.ConfigStore.GetVirtualKeyByValue(ctx, trimmedVKValue)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			return true
		}
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to resolve virtual key: %v", err))
		return false
	}
	if vk == nil || vk.IsActive == nil || !*vk.IsActive {
		return true
	}

	// Team ACL (Phase 6 / D6): resolve the team's per-provider model policies so
	// both the fan-out below and the response filter can honour them.
	teamPolicies, err := h.teamModelPoliciesForVK(ctx, vk)
	if err != nil {
		// Fail closed: without the policies we cannot tell which models the team
		// may see, and listing too much is the unsafe direction.
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to resolve team model policies: %v", err))
		return false
	}

	availableProviders := make([]schemas.ModelProvider, 0, len(vk.ProviderConfigs))
	for _, providerConfig := range vk.ProviderConfigs {
		provider := strings.TrimSpace(providerConfig.Provider)
		if provider == "" {
			continue
		}
		// Drop providers the team cannot use at all. Doing it here rather than
		// after the fan-out is both cheaper (core never queries the provider) and
		// impossible to bypass downstream, which matters because the aggregate
		// response merges every provider's models into one flat list that carries
		// no per-model provider attribution.
		if policy, ok := teamPolicies[strings.ToLower(provider)]; ok {
			if governanceplugin.TeamPolicyDeniesAllModels(&policy) {
				continue
			}
		}
		availableProviders = append(availableProviders, schemas.ModelProvider(provider))
	}

	bifrostCtx.SetValue(schemas.BifrostContextKeyAvailableProviders, availableProviders)
	// Stash the resolved VK for the backfill stage below. We hand the caller
	// back a copy of the pointer so any later state mutation on the store's
	// in-memory copy doesn't desync the snapshot we recorded here.
	if vk != nil {
		bifrostCtx.SetValue(listModelsResolvedVKKey, vk)
	}
	if len(teamPolicies) > 0 {
		bifrostCtx.SetValue(listModelsTeamPoliciesKey, teamPolicies)
	}
	return true
}

// teamModelPoliciesForVK loads the team ACL rows (Phase 6 / D6) for a resolved
// virtual key, keyed by lowercased provider name. It returns nil — not an empty
// map — for every "inherit global" case: no VK, a VK with no team, or a team
// with no policies. That is by far the most common shape, so callers can treat
// nil as "nothing to do" without allocating.
func (h *CompletionHandler) teamModelPoliciesForVK(ctx *fasthttp.RequestCtx, vk *configstoreTables.TableVirtualKey) (map[string]configstoreTables.TableTeamModelPolicy, error) {
	if vk == nil || vk.Team == nil || vk.Team.ID == "" {
		return nil, nil
	}
	policies, err := h.config.ConfigStore.ListTeamModelPolicies(ctx, vk.Team.ID)
	if err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return nil, nil
	}
	keyed := make(map[string]configstoreTables.TableTeamModelPolicy, len(policies))
	for _, policy := range policies {
		keyed[strings.ToLower(strings.TrimSpace(policy.Provider))] = policy
	}
	return keyed, nil
}

// applyListModelsTeamACLForExplicitProvider resolves the team ACL for a
// GET /v1/models?provider=X request and stashes it on bifrostCtx so
// applyListModelsTeamACLFilter can narrow the response.
//
// This exists because the explicit-provider path never runs
// applyListModelsVirtualKeyProviderFilter — that function narrows the aggregate
// fan-out, which is meaningless when the caller already named one provider.
// Calling it anyway would be more than a no-op: it also sets
// AvailableProviders and the resolved-VK handle, and the latter changes the
// routing-rule backfill scope chain for these requests. That is a separate
// behaviour change, deliberately not bundled with the team ACL work.
//
// Unlike the aggregate path, a missing ConfigStore is tolerated here (no
// narrowing possible, and governance still gates the real request downstream)
// rather than answered with a 503, to keep the pre-existing response contract
// of ?provider=X intact.
func (h *CompletionHandler) applyListModelsTeamACLForExplicitProvider(ctx *fasthttp.RequestCtx, bifrostCtx *schemas.BifrostContext) bool {
	vkValue := governanceplugin.ParseVirtualKeyFromFastHTTPRequest(ctx)
	if vkValue == nil {
		return true
	}
	trimmedVKValue := strings.TrimSpace(*vkValue)
	if trimmedVKValue == "" {
		return true
	}
	if h.config == nil || h.config.ConfigStore == nil {
		return true
	}

	vk, err := h.config.ConfigStore.GetVirtualKeyByValue(ctx, trimmedVKValue)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			return true
		}
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to resolve virtual key: %v", err))
		return false
	}
	if vk == nil || vk.IsActive == nil || !*vk.IsActive {
		return true
	}

	teamPolicies, err := h.teamModelPoliciesForVK(ctx, vk)
	if err != nil {
		// Fail closed, same direction as the aggregate path.
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to resolve team model policies: %v", err))
		return false
	}
	if len(teamPolicies) > 0 {
		bifrostCtx.SetValue(listModelsTeamPoliciesKey, teamPolicies)
	}
	return true
}

// resolvedVKFromBifrostContext returns the *TableVirtualKey that
// applyListModelsVirtualKeyProviderFilter stashed on bifrostCtx, or nil
// when the request did not carry a virtual key. Returns a typed *TableVirtualKey
// to avoid an interface{} assertion site at every caller.
func resolvedVKFromBifrostContext(bifrostCtx *schemas.BifrostContext) *configstoreTables.TableVirtualKey {
	if bifrostCtx == nil {
		return nil
	}
	v, _ := bifrostCtx.Value(listModelsResolvedVKKey).(*configstoreTables.TableVirtualKey)
	return v
}

// teamModelPoliciesFromBifrostContext returns the team model policies stashed by
// applyListModelsVirtualKeyProviderFilter, keyed by lowercased provider name, or
// nil when the request carries no virtual key / the VK's team has no policies.
func teamModelPoliciesFromBifrostContext(bifrostCtx *schemas.BifrostContext) map[string]configstoreTables.TableTeamModelPolicy {
	if bifrostCtx == nil {
		return nil
	}
	policies, _ := bifrostCtx.Value(listModelsTeamPoliciesKey).(map[string]configstoreTables.TableTeamModelPolicy)
	return policies
}

// applyListModelsTeamACLFilter narrows a GET /v1/models response by the team ACL
// (Phase 6 / D6), so a member never sees a model their requests would be denied.
// It shares its predicate with the governance resolver's per-request gate
// (governanceplugin.TeamPolicyAllowsModel) — the two must not disagree.
//
// Each entry is attributed to a provider before being checked against that
// provider's policy:
//
//   - the aggregate fan-out merges every provider's models into one flat
//     []schemas.Model, and core stamps schemas.Model.Provider on each entry while
//     collecting (Bifrost.ListAllModels) — the last point at which the origin is
//     unambiguous;
//   - the ?provider=X path goes through ListModelsRequest, which does not stamp,
//     so explicitProvider (the raw query value) is the fallback.
//
// Providers the team cannot use at all are additionally dropped from the fan-out
// up-front in applyListModelsVirtualKeyProviderFilter, so core never even queries
// them.
//
// The policies are stashed on the context by applyListModelsVirtualKeyProviderFilter
// (aggregate path) or applyListModelsTeamACLForExplicitProvider (?provider=X).
//
// A no-op when the request has no team policies, which keeps the unscoped and
// DB-cache paths (neither of which is ever VK-scoped) untouched.
func applyListModelsTeamACLFilter(resp *schemas.BifrostListModelsResponse, bifrostCtx *schemas.BifrostContext, explicitProvider schemas.ModelProvider) {
	if resp == nil || len(resp.Data) == 0 {
		return
	}
	policies := teamModelPoliciesFromBifrostContext(bifrostCtx)
	if len(policies) == 0 {
		return
	}
	explicit := strings.ToLower(strings.TrimSpace(string(explicitProvider)))

	kept := make([]schemas.Model, 0, len(resp.Data))
	for _, model := range resp.Data {
		provider := strings.ToLower(strings.TrimSpace(string(model.Provider)))
		if provider == "" {
			provider = explicit
		}
		if provider == "" {
			// Defensive, unreachable today: aggregate entries are stamped by core,
			// the explicit path always carries a provider, and the cache path is
			// never VK-scoped so it never reaches here. Fail closed rather than
			// advertise a model whose policy could not be resolved.
			continue
		}
		policy, ok := policies[provider]
		if !ok {
			// No policy for this provider = inherit global.
			kept = append(kept, model)
			continue
		}
		// Listed IDs are provider-prefixed ("deepseek/deepseek-flash") while ACL
		// entries are bare model names. Normalize exactly the way governance does
		// before gating a request (plugins/governance/main.go) and the way
		// enrichListModelsResponse does for pricing lookups, so this filter and
		// the per-request gate see the same string. ParseModelString only splits
		// on a *known* provider prefix, so genuinely namespaced IDs
		// ("meta-llama/Llama-3.1-8B") survive intact.
		_, modelName := schemas.ParseModelString(model.ID, "")
		if governanceplugin.TeamPolicyAllowsModel(&policy, modelName) {
			kept = append(kept, model)
		}
	}
	resp.Data = kept
}
