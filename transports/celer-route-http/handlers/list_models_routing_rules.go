package handlers

import (
	"context"

	"github.com/pin-gou/celer-route/core/schemas"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	governanceplugin "github.com/pin-gou/celer-route/plugins/governance"
)

// listModelsBackfillSource models the narrow surface of the governance plugin
// the routing-rule backfill needs. It mirrors governanceplugin.BaseGovernancePlugin
// for just the method we call.
//
// Declared here rather than depending on the concrete plugin type so the
// handler file documents the contact explicitly and so we can keep the
// dependency surface minimal.
type listModelsBackfillSource interface {
	GetGovernanceStore() governanceplugin.GovernanceStore
}

// virtualModelOwner is the value exposed on the backfilled Model entries. It
// is intentionally a small, stable sentinel string so OpenAI-compatible
// clients can recognise gateway-level virtual models without parsing the
// Alias or CanonicalSlug fields.
const virtualModelOwner = "celer-route"

// enrichListModelsWithRoutingRuleBackfill appends virtual model entries
// derived from enabled routing rules to resp.Data. The set is the union of
// every scope reachable from the current request (virtual_key → team →
// customer → global).
//
// Behavior:
//   - Provider-owned models take precedence: if resp.Data already contains an
//     entry with the same ID, the rule-derived entry is dropped.
//   - Multiple rules in the same or different scopes that map to the same
//     virtual model name collapse into a single entry; the first rule's
//     metadata (Alias, CanonicalSlug) wins for wire stability.
//   - When the request carries `?provider=`, the backfill still runs —
//     virtual models are a gateway-level abstraction, not a provider-level one.
//
// The function is a no-op when no governance plugin is registered (OSS
// builds, or governance disabled in config).
func (h *CompletionHandler) enrichListModelsWithRoutingRuleBackfill(
	resp *schemas.BifrostListModelsResponse,
	bifrostCtx *schemas.BifrostContext,
) {
	if resp == nil || h == nil || h.config == nil || bifrostCtx == nil {
		return
	}
	src := h.findGovernancePlugin()
	if src == nil {
		return
	}
	store := src.GetGovernanceStore()
	if store == nil {
		return
	}

	chain := buildListModelsScopeChain(bifrostCtx)
	if len(chain) == 0 {
		return
	}

	// Track IDs already present from provider fan-out so backfill doesn't
	// shadow a real provider model with the same name.
	taken := make(map[string]struct{}, len(resp.Data))
	for i := range resp.Data {
		taken[resp.Data[i].ID] = struct{}{}
	}

	// First write wins for the rule's metadata, so the wire field stays
	// stable as scopes are processed in precedence order.
	aggregated := make(map[string]schemas.Model, len(resp.Data))

	// Use a context.Background()-shaped cancelable for the store calls so we
	// never block on a slow in-memory map. The bifrostCtx itself carries a
	// deadline; in practice the store lookup is microseconds, so the
	// explicit cancelable is just hygiene.
	storeCtx, cancel := context.WithCancel(bifrostCtx)
	defer cancel()

	for _, scope := range chain {
		entries := store.GetScopedRoutingRuleModelLiterals(storeCtx, scope.ScopeName, scope.ScopeID)
		for _, e := range entries {
			if e.ModelID == "" {
				continue
			}
			if _, dup := taken[e.ModelID]; dup {
				continue
			}
			if _, exists := aggregated[e.ModelID]; exists {
				continue
			}
			m := schemas.Model{
				ID:      e.ModelID,
				OwnedBy: stringPtr(virtualModelOwner),
				Name:    stringPtr(e.ModelID),
				Alias:   cloneStringPtr(e.Alias),
			}
			if e.RuleID != "" {
				m.CanonicalSlug = stringPtr(e.RuleID)
			}
			aggregated[e.ModelID] = m
		}
	}

	if len(aggregated) == 0 {
		return
	}

	// Append in a deterministic order so two requests against the same rule
	// set always see the same response shape. We sort by ID alphabetically —
	// provider models already came in their own order, we only sort the
	// backfill chunk.
	out := make([]schemas.Model, 0, len(aggregated))
	for _, m := range aggregated {
		out = append(out, m)
	}
	sortModelsByID(out)
	resp.Data = append(resp.Data, out...)
}

// buildListModelsScopeChain mirrors plugins/governance.buildScopeChain —
// virtual_key > team > customer > global — but is reconstructed from values
// available in the transport (the resolved VK on bifrostCtx). Keeping this
// in the handler avoids a transports -> plugins/governance dependency
// direction change.
//
// An empty ScopeID for a non-global scope is treated as "this scope does not
// apply" and the entry is skipped — global is always appended last.
//
// user-scope is intentionally not consulted in the OSS build: the
// enterprise-only BifrostContextKeyUserID would trip the OSS stripper. A
// future PR can add it without re-shaping this function.
func buildListModelsScopeChain(bifrostCtx *schemas.BifrostContext) []listModelsScope {
	var chain []listModelsScope

	if vk := resolvedVKFromBifrostContext(bifrostCtx); vk != nil && vk.ID != "" {
		chain = append(chain, listModelsScope{"virtual_key", vk.ID})
		if vk.Team != nil && vk.Team.ID != "" {
			chain = append(chain, listModelsScope{"team", vk.Team.ID})
			if vk.Team.Customer != nil && vk.Team.Customer.ID != "" {
				chain = append(chain, listModelsScope{"customer", vk.Team.Customer.ID})
			}
		} else if vk.Customer != nil && vk.Customer.ID != "" {
			chain = append(chain, listModelsScope{"customer", vk.Customer.ID})
		}
	}

	chain = append(chain, listModelsScope{"global", ""})
	return chain
}

type listModelsScope struct {
	ScopeName string
	ScopeID   string
}

// findGovernancePlugin returns the first registered plugin that satisfies the
// listModelsBackfillSource interface, or nil if none is wired. The shape
// mirrors what RealtimeClientSecretsHandler does for the same plugin, but
// typed here against the narrower surface the backfill needs.
func (h *CompletionHandler) findGovernancePlugin() listModelsBackfillSource {
	if h == nil || h.config == nil {
		return nil
	}
	base := h.config.BasePlugins.Load()
	if base == nil {
		return nil
	}
	for _, p := range *base {
		if gp, ok := p.(listModelsBackfillSource); ok {
			return gp
		}
	}
	return nil
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func cloneStringPtr(s *string) *string {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}

func sortModelsByID(ms []schemas.Model) {
	if len(ms) < 2 {
		return
	}
	// Insertion sort is fine here — backfill adds at most a few hundred
	// entries in pathological rule sets, and the slice is short-lived.
	for i := 1; i < len(ms); i++ {
		for j := i; j > 0 && ms[j-1].ID > ms[j].ID; j-- {
			ms[j-1], ms[j] = ms[j], ms[j-1]
		}
	}
}

// _ keeps configstoreTables referenced so an unused-import error doesn't
// creep in if a future refactor removes the only direct use; the type is
// used transitively via resolvedVKFromBifrostContext below.
var _ = (*configstoreTables.TableVirtualKey)(nil)
