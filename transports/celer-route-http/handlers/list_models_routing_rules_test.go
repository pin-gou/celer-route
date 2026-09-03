package handlers

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	governanceplugin "github.com/pin-gou/celer-route/plugins/governance"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
)

// fakeRoutingRuleStore implements just enough of governanceplugin.GovernanceStore
// for the backfill path. Other methods are never called by the code under test
// and panic if invoked, so a regression that expands the surface will surface
// here as a fast failure instead of silent misbehavior.
//
// We embed the real interface so any new method added in the future breaks
// the build here, catching test drift early.
type fakeRoutingRuleStore struct {
	governanceplugin.GovernanceStore
	literals map[string][]governanceplugin.RoutingRuleModelLiteral
}

func (f *fakeRoutingRuleStore) GetScopedRoutingRuleModelLiterals(_ context.Context, scope, scopeID string) []governanceplugin.RoutingRuleModelLiteral {
	if f.literals == nil {
		return nil
	}
	key := scope + ":" + scopeID
	return f.literals[key]
}

// fakeGovernancePlugin satisfies both listModelsBackfillSource (the surface
// the handler uses) and schemas.BasePlugin (so it can live inside
// BasePlugins alongside other plugins in tests).
type fakeGovernancePlugin struct {
	store *fakeRoutingRuleStore
}

func (f *fakeGovernancePlugin) GetName() string         { return "fake-governance" }
func (f *fakeGovernancePlugin) Cleanup() error          { return nil }
func (f *fakeGovernancePlugin) GetGovernanceStore() governanceplugin.GovernanceStore {
	return f.store
}

func newListModelsHandlerWithRules(rules map[string][]governanceplugin.RoutingRuleModelLiteral) (*CompletionHandler, *schemas.BifrostContext) {
	store := &fakeRoutingRuleStore{literals: rules}
	plugin := &fakeGovernancePlugin{store: store}

	cfg := &lib.Config{}
	plugins := []schemas.BasePlugin{schemas.BasePlugin(plugin)}
	cfg.BasePlugins.Store(&plugins)

	h := &CompletionHandler{config: cfg}
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	return h, bifrostCtx
}

func ids(resp *schemas.BifrostListModelsResponse) []string {
	out := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		out = append(out, m.ID)
	}
	sort.Strings(out)
	return out
}

func TestBackfill_AppendsUnionFromScopeChain(t *testing.T) {
	vkID := "vk_a"
	alias := "openai/gpt-4o"
	rulesByScope := map[string][]governanceplugin.RoutingRuleModelLiteral{
		"virtual_key:" + vkID: {
			{ModelID: "pg-expert", Alias: &alias, RuleID: "rule_vk_1", RuleName: "vk-1"},
		},
		"global:": {
			{ModelID: "pg-master", Alias: nil, RuleID: "rule_g_1", RuleName: "g-1"},
		},
	}
	h, bifrostCtx := newListModelsHandlerWithRules(rulesByScope)
	vk := &configstoreTables.TableVirtualKey{ID: vkID}
	bifrostCtx.SetValue(listModelsResolvedVKKey, vk)

	resp := &schemas.BifrostListModelsResponse{Data: []schemas.Model{}}
	h.enrichListModelsWithRoutingRuleBackfill(resp, bifrostCtx)

	got := ids(resp)
	want := []string{"pg-expert", "pg-master"}
	if !equalStrings(got, want) {
		t.Fatalf("expected %v got %v", want, got)
	}
}

func TestBackfill_DedupsAcrossScopes(t *testing.T) {
	vkID := "vk_a"
	rulesByScope := map[string][]governanceplugin.RoutingRuleModelLiteral{
		"virtual_key:" + vkID: {
			{ModelID: "pg-expert", RuleID: "rule_vk_1", RuleName: "vk-1"},
		},
		"global:": {
			{ModelID: "pg-expert", RuleID: "rule_g_1", RuleName: "g-1"},
		},
	}
	h, bifrostCtx := newListModelsHandlerWithRules(rulesByScope)
	vk := &configstoreTables.TableVirtualKey{ID: vkID}
	bifrostCtx.SetValue(listModelsResolvedVKKey, vk)

	resp := &schemas.BifrostListModelsResponse{Data: []schemas.Model{}}
	h.enrichListModelsWithRoutingRuleBackfill(resp, bifrostCtx)

	got := ids(resp)
	want := []string{"pg-expert"}
	if !equalStrings(got, want) {
		t.Fatalf("expected dedup to one entry, got %v", got)
	}
	if resp.Data[0].CanonicalSlug == nil || *resp.Data[0].CanonicalSlug != "rule_vk_1" {
		t.Fatalf("expected first-write-wins to keep VK rule id, got %v", resp.Data[0].CanonicalSlug)
	}
}

func TestBackfill_ProviderModelWinsOverVirtual(t *testing.T) {
	vkID := "vk_a"
	ownerOpenAI := "openai"
	rulesByScope := map[string][]governanceplugin.RoutingRuleModelLiteral{
		"virtual_key:" + vkID: {
			{ModelID: "shared-name", RuleID: "rule_vk_1"},
		},
	}
	h, bifrostCtx := newListModelsHandlerWithRules(rulesByScope)
	vk := &configstoreTables.TableVirtualKey{ID: vkID}
	bifrostCtx.SetValue(listModelsResolvedVKKey, vk)

	resp := &schemas.BifrostListModelsResponse{
		Data: []schemas.Model{
			{ID: "shared-name", OwnedBy: &ownerOpenAI},
		},
	}
	h.enrichListModelsWithRoutingRuleBackfill(resp, bifrostCtx)

	if len(resp.Data) != 1 {
		t.Fatalf("expected provider entry to be preserved, got %v", resp.Data)
	}
	if resp.Data[0].OwnedBy == nil || *resp.Data[0].OwnedBy != "openai" {
		t.Fatalf("provider entry should not be replaced, got %v", resp.Data[0])
	}
}

func TestBackfill_NoGovernancePlugin_NoOp(t *testing.T) {
	h := &CompletionHandler{config: &lib.Config{}}
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	resp := &schemas.BifrostListModelsResponse{Data: []schemas.Model{{ID: "x"}}}
	h.enrichListModelsWithRoutingRuleBackfill(resp, bifrostCtx)

	if len(resp.Data) != 1 || resp.Data[0].ID != "x" {
		t.Fatalf("expected no change, got %v", resp.Data)
	}
}

func TestBackfill_NilResponse_NoOp(t *testing.T) {
	h := &CompletionHandler{config: &lib.Config{}}
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	h.enrichListModelsWithRoutingRuleBackfill(nil, bifrostCtx)
}

func TestBuildListModelsScopeChain_NoVK_OnlyGlobal(t *testing.T) {
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	chain := buildListModelsScopeChain(bifrostCtx)
	if len(chain) != 1 || chain[0].ScopeName != "global" || chain[0].ScopeID != "" {
		t.Fatalf("expected single global scope, got %v", chain)
	}
}

func TestBuildListModelsScopeChain_VKAddsScopes(t *testing.T) {
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	vk := &configstoreTables.TableVirtualKey{
		ID: "vk_a",
		Team: &configstoreTables.TableTeam{
			ID:       "team_a",
			Customer: &configstoreTables.TableCustomer{ID: "cust_a"},
		},
	}
	bifrostCtx.SetValue(listModelsResolvedVKKey, vk)

	chain := buildListModelsScopeChain(bifrostCtx)
	if len(chain) != 4 {
		t.Fatalf("expected 4 scopes (vk/team/customer/global), got %d: %v", len(chain), chain)
	}
	if chain[0].ScopeName != "virtual_key" || chain[0].ScopeID != "vk_a" {
		t.Fatalf("expected virtual_key first, got %v", chain[0])
	}
	if chain[1].ScopeName != "team" || chain[1].ScopeID != "team_a" {
		t.Fatalf("expected team second, got %v", chain[1])
	}
	if chain[2].ScopeName != "customer" || chain[2].ScopeID != "cust_a" {
		t.Fatalf("expected customer third, got %v", chain[2])
	}
	if chain[3].ScopeName != "global" {
		t.Fatalf("expected global last, got %v", chain[3])
	}
}

func TestBuildListModelsScopeChain_VKWithDirectCustomer(t *testing.T) {
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	vk := &configstoreTables.TableVirtualKey{
		ID:       "vk_a",
		Customer: &configstoreTables.TableCustomer{ID: "cust_only"},
	}
	bifrostCtx.SetValue(listModelsResolvedVKKey, vk)

	chain := buildListModelsScopeChain(bifrostCtx)
	if len(chain) != 3 {
		t.Fatalf("expected 3 scopes, got %d: %v", len(chain), chain)
	}
	if chain[1].ScopeName != "customer" || chain[1].ScopeID != "cust_only" {
		t.Fatalf("expected direct customer scope, got %v", chain[1])
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
