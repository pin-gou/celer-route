package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

type mockListModelsVKConfigStore struct {
	configstore.ConfigStore
	vk  *configstoreTables.TableVirtualKey
	err error
	// teamPolicies / teamPoliciesErr back ListTeamModelPolicies for the team ACL
	// (Phase 6 / D6) tests. Left nil by the pre-existing VK tests, whose VKs have
	// no team and therefore never reach the lookup.
	teamPolicies    []configstoreTables.TableTeamModelPolicy
	teamPoliciesErr error
}

func (m *mockListModelsVKConfigStore) GetVirtualKeyByValue(_ context.Context, _ string) (*configstoreTables.TableVirtualKey, error) {
	return m.vk, m.err
}

func (m *mockListModelsVKConfigStore) ListTeamModelPolicies(_ context.Context, _ string) ([]configstoreTables.TableTeamModelPolicy, error) {
	return m.teamPolicies, m.teamPoliciesErr
}

// teamScopedVK builds an active VK on team-1 allowing the given providers, for
// the team ACL tests.
func teamScopedVK(secret string, providers ...string) *configstoreTables.TableVirtualKey {
	active := true
	configs := make([]configstoreTables.TableVirtualKeyProviderConfig, 0, len(providers))
	for _, p := range providers {
		configs = append(configs, configstoreTables.TableVirtualKeyProviderConfig{Provider: p})
	}
	return &configstoreTables.TableVirtualKey{
		Value:           *schemas.NewSecretVar(secret),
		IsActive:        &active,
		TeamID:          stringPtr("team-1"),
		Team:            &configstoreTables.TableTeam{ID: "team-1", Name: "Team 1"},
		ProviderConfigs: configs,
	}
}

func TestApplyListModelsVirtualKeyProviderFilterSetsActiveVKProviders(t *testing.T) {
	h := &CompletionHandler{
		config: &lib.Config{
			ConfigStore: &mockListModelsVKConfigStore{vk: &configstoreTables.TableVirtualKey{
				Value:    *schemas.NewSecretVar("sk-bf-active"),
				IsActive: new(true),
				ProviderConfigs: []configstoreTables.TableVirtualKeyProviderConfig{
					{Provider: "openai"},
					{Provider: " anthropic "},
					{Provider: ""},
				},
			}},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Bearer sk-bf-active")
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsVirtualKeyProviderFilter(ctx, bifrostCtx); !ok {
		t.Fatalf("expected active VK to apply provider filter")
	}
	got, ok := bifrostCtx.Value(schemas.BifrostContextKeyAvailableProviders).([]schemas.ModelProvider)
	if !ok {
		t.Fatalf("expected available providers to be set")
	}
	want := []schemas.ModelProvider{schemas.OpenAI, schemas.Anthropic}
	if len(got) != len(want) {
		t.Fatalf("expected providers %#v, got %#v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected providers %#v, got %#v", want, got)
		}
	}
}

func TestApplyListModelsVirtualKeyProviderFilterReturnsErrorOnLookupFailure(t *testing.T) {
	h := &CompletionHandler{
		config: &lib.Config{
			ConfigStore: &mockListModelsVKConfigStore{err: errors.New("database unavailable")},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Bearer sk-bf-active")
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsVirtualKeyProviderFilter(ctx, bifrostCtx); ok {
		t.Fatalf("expected lookup error to fail request")
	}
	if got := ctx.Response.StatusCode(); got != fasthttp.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusInternalServerError, got)
	}
	if body := string(ctx.Response.Body()); !strings.Contains(body, "Failed to resolve virtual key") {
		t.Fatalf("expected virtual key lookup error response, got %q", body)
	}
}

func TestApplyListModelsVirtualKeyProviderFilterReturnsUnavailableWithoutConfigStore(t *testing.T) {
	h := &CompletionHandler{config: &lib.Config{}}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Bearer sk-bf-active")
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsVirtualKeyProviderFilter(ctx, bifrostCtx); ok {
		t.Fatalf("expected missing config store to fail request")
	}
	if got := ctx.Response.StatusCode(); got != fasthttp.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusServiceUnavailable, got)
	}
	if body := string(ctx.Response.Body()); !strings.Contains(body, "database store unavailable") {
		t.Fatalf("expected unavailable response, got %q", body)
	}
}

func TestApplyListModelsVirtualKeyProviderFilterSkipsWhenVKNotFound(t *testing.T) {
	h := &CompletionHandler{
		config: &lib.Config{
			ConfigStore: &mockListModelsVKConfigStore{},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Bearer sk-bf-missing")
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsVirtualKeyProviderFilter(ctx, bifrostCtx); !ok {
		t.Fatalf("expected missing VK to be ignored without failing request")
	}
	if got := bifrostCtx.Value(schemas.BifrostContextKeyAvailableProviders); got != nil {
		t.Fatalf("expected missing VK not to set available providers, got %#v", got)
	}
}

func TestApplyListModelsVirtualKeyProviderFilterSkipsInactiveVK(t *testing.T) {
	h := &CompletionHandler{
		config: &lib.Config{
			ConfigStore: &mockListModelsVKConfigStore{vk: &configstoreTables.TableVirtualKey{
				Value:    *schemas.NewSecretVar("sk-bf-inactive"),
				IsActive: new(false),
				ProviderConfigs: []configstoreTables.TableVirtualKeyProviderConfig{
					{Provider: "openai"},
				},
			}},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Bearer sk-bf-inactive")
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsVirtualKeyProviderFilter(ctx, bifrostCtx); !ok {
		t.Fatalf("expected inactive VK to be ignored without failing request")
	}
	if got := bifrostCtx.Value(schemas.BifrostContextKeyAvailableProviders); got != nil {
		t.Fatalf("expected inactive VK not to set available providers, got %#v", got)
	}
}

// TestApplyListModelsVirtualKeyProviderFilterDropsProviderDeniedByTeamACL covers
// the team ACL (Phase 6 / D6) fan-out narrowing: a provider whose team policy
// permits no models is removed from the fan-out entirely, so core never queries
// it. This is the only granularity available on the aggregate /v1/models path,
// where every provider's models are merged into one flat list with no per-model
// provider attribution.
func TestApplyListModelsVirtualKeyProviderFilterDropsProviderDeniedByTeamACL(t *testing.T) {
	h := &CompletionHandler{
		config: &lib.Config{
			ConfigStore: &mockListModelsVKConfigStore{
				vk: teamScopedVK("sk-bf-team", "openai", "anthropic"),
				teamPolicies: []configstoreTables.TableTeamModelPolicy{
					{
						TeamID:   "team-1",
						Provider: "openai",
						// Empty allowlist + a blacklist denies every model.
						BlacklistedModels: []string{"o1"},
					},
				},
			},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Bearer sk-bf-team")
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsVirtualKeyProviderFilter(ctx, bifrostCtx); !ok {
		t.Fatalf("expected team-scoped VK to apply provider filter")
	}

	got, ok := bifrostCtx.Value(schemas.BifrostContextKeyAvailableProviders).([]schemas.ModelProvider)
	if !ok {
		t.Fatalf("expected available providers to be set")
	}
	if len(got) != 1 || got[0] != schemas.Anthropic {
		t.Fatalf("expected only anthropic to survive the team ACL, got %#v", got)
	}

	policies := teamModelPoliciesFromBifrostContext(bifrostCtx)
	if len(policies) != 1 {
		t.Fatalf("expected the team policy to be stashed for the response filter, got %#v", policies)
	}
	if _, found := policies["openai"]; !found {
		t.Fatalf("expected policies keyed by lowercased provider, got %#v", policies)
	}
}

// TestApplyListModelsVirtualKeyProviderFilterFailsClosedOnTeamPolicyLookupError
// pins the fail-closed direction: if the team policies cannot be read, the
// request errors rather than listing models the team may not be allowed to see.
func TestApplyListModelsVirtualKeyProviderFilterFailsClosedOnTeamPolicyLookupError(t *testing.T) {
	h := &CompletionHandler{
		config: &lib.Config{
			ConfigStore: &mockListModelsVKConfigStore{
				vk:              teamScopedVK("sk-bf-team", "openai"),
				teamPoliciesErr: errors.New("database unavailable"),
			},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Bearer sk-bf-team")
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsVirtualKeyProviderFilter(ctx, bifrostCtx); ok {
		t.Fatalf("expected team policy lookup error to fail the request")
	}
	if got := ctx.Response.StatusCode(); got != fasthttp.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusInternalServerError, got)
	}
	if body := string(ctx.Response.Body()); !strings.Contains(body, "Failed to resolve team model policies") {
		t.Fatalf("expected team policy lookup error response, got %q", body)
	}
}

// TestApplyListModelsTeamACLFilterNarrowsModelsForExplicitProvider covers
// model-level team ACL narrowing on both listing paths: ?provider=X (attribution
// from the query value) and the aggregate fan-out (attribution from the Provider
// field core stamps while merging).
func TestApplyListModelsTeamACLFilterNarrowsModelsForExplicitProvider(t *testing.T) {
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	bifrostCtx.SetValue(listModelsTeamPoliciesKey, map[string]configstoreTables.TableTeamModelPolicy{
		"openai": {
			TeamID:        "team-1",
			Provider:      "openai",
			AllowedModels: []string{"gpt-4o", "gpt-4o-mini"},
		},
	})

	newResp := func() *schemas.BifrostListModelsResponse {
		return &schemas.BifrostListModelsResponse{Data: []schemas.Model{
			{ID: "gpt-4o"},
			{ID: "gpt-4o-mini"},
			{ID: "o1"},
		}}
	}

	t.Run("models outside the team allowlist are dropped", func(t *testing.T) {
		resp := newResp()
		applyListModelsTeamACLFilter(resp, bifrostCtx, schemas.OpenAI)
		got := make([]string, 0, len(resp.Data))
		for _, m := range resp.Data {
			got = append(got, m.ID)
		}
		if len(got) != 2 || got[0] != "gpt-4o" || got[1] != "gpt-4o-mini" {
			t.Fatalf("expected [gpt-4o gpt-4o-mini], got %#v", got)
		}
	})

	t.Run("aggregate entries are filtered by their stamped provider", func(t *testing.T) {
		// The aggregate fan-out carries no ?provider=, so attribution comes from
		// the Provider field core stamps while merging (Bifrost.ListAllModels).
		resp := &schemas.BifrostListModelsResponse{Data: []schemas.Model{
			{ID: "openai/gpt-4o", Provider: schemas.OpenAI},
			{ID: "openai/o1", Provider: schemas.OpenAI},
			{ID: "anthropic/claude-opus-4-7", Provider: schemas.Anthropic},
		}}
		applyListModelsTeamACLFilter(resp, bifrostCtx, "")
		got := make([]string, 0, len(resp.Data))
		for _, m := range resp.Data {
			got = append(got, m.ID)
		}
		// openai is narrowed to the allowlist; anthropic has no policy so it
		// inherits global and survives untouched.
		want := []string{"openai/gpt-4o", "anthropic/claude-opus-4-7"}
		if len(got) != len(want) {
			t.Fatalf("expected %#v, got %#v", want, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("expected %#v, got %#v", want, got)
			}
		}
	})

	t.Run("unattributed entry with no explicit provider fails closed", func(t *testing.T) {
		// Defensive branch: unreachable today (core stamps the aggregate path,
		// ?provider=X supplies the fallback, and the cache path is never
		// VK-scoped). Pinned so a future path that forgets attribution surfaces
		// as an empty list rather than silently advertising blocked models.
		resp := newResp()
		applyListModelsTeamACLFilter(resp, bifrostCtx, "")
		if len(resp.Data) != 0 {
			t.Fatalf("expected unattributed entries to be dropped, got %#v", resp.Data)
		}
	})

	t.Run("provider without a policy inherits global", func(t *testing.T) {
		resp := newResp()
		applyListModelsTeamACLFilter(resp, bifrostCtx, schemas.Anthropic)
		if len(resp.Data) != 3 {
			t.Fatalf("expected untouched response for a provider with no policy, got %#v", resp.Data)
		}
	})

	t.Run("nil response and empty context are no-ops", func(t *testing.T) {
		applyListModelsTeamACLFilter(nil, bifrostCtx, schemas.OpenAI)
		resp := newResp()
		applyListModelsTeamACLFilter(resp, nil, schemas.OpenAI)
		if len(resp.Data) != 3 {
			t.Fatalf("expected untouched response without a context, got %#v", resp.Data)
		}
	})
}

// TestApplyListModelsTeamACLFilterNormalizesProviderPrefixedIDs pins the ID-form
// mismatch between the listing and the ACL: GET /v1/models advertises
// provider-prefixed IDs ("openai/gpt-4o") while policy entries are bare model
// names. Governance normalizes the request model with schemas.ParseModelString
// before gating it, so this filter must normalize identically — comparing the
// raw listed ID silently drops every model and empties the list.
func TestApplyListModelsTeamACLFilterNormalizesProviderPrefixedIDs(t *testing.T) {
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	bifrostCtx.SetValue(listModelsTeamPoliciesKey, map[string]configstoreTables.TableTeamModelPolicy{
		"openai": {
			TeamID:        "team-1",
			Provider:      "openai",
			AllowedModels: []string{"gpt-4o", "gpt-4o-mini"},
		},
	})

	resp := &schemas.BifrostListModelsResponse{Data: []schemas.Model{
		{ID: "openai/gpt-4o"},
		{ID: "openai/o1"},
		{ID: "gpt-4o-mini"},
	}}
	applyListModelsTeamACLFilter(resp, bifrostCtx, schemas.OpenAI)

	got := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		got = append(got, m.ID)
	}
	// Prefixed and bare spellings of an allowed model both survive; the listed ID
	// is preserved verbatim so clients can send it straight back.
	want := []string{"openai/gpt-4o", "gpt-4o-mini"}
	if len(got) != len(want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %#v, got %#v", want, got)
		}
	}
}

// TestApplyListModelsTeamACLForExplicitProviderStashesPolicies covers the
// ?provider=X path, which never runs applyListModelsVirtualKeyProviderFilter and
// would otherwise list models the caller's team may not use.
func TestApplyListModelsTeamACLForExplicitProviderStashesPolicies(t *testing.T) {
	h := &CompletionHandler{
		config: &lib.Config{
			ConfigStore: &mockListModelsVKConfigStore{
				vk: teamScopedVK("sk-bf-team", "openai"),
				teamPolicies: []configstoreTables.TableTeamModelPolicy{
					{TeamID: "team-1", Provider: "OpenAI", AllowedModels: []string{"gpt-4o"}},
				},
			},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Bearer sk-bf-team")
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsTeamACLForExplicitProvider(ctx, bifrostCtx); !ok {
		t.Fatalf("expected explicit-provider team ACL resolution to succeed")
	}

	policies := teamModelPoliciesFromBifrostContext(bifrostCtx)
	if len(policies) != 1 {
		t.Fatalf("expected one stashed policy, got %#v", policies)
	}
	// Keyed by lowercased provider so the ?provider= lookup matches regardless of
	// the casing the policy row was written with.
	if _, found := policies["openai"]; !found {
		t.Fatalf("expected policies keyed by lowercased provider, got %#v", policies)
	}
	// The provider fan-out must be left alone on this path.
	if v := bifrostCtx.Value(schemas.BifrostContextKeyAvailableProviders); v != nil {
		t.Fatalf("expected AvailableProviders to stay unset, got %#v", v)
	}
}

// TestApplyListModelsTeamACLForExplicitProviderSkipsWithoutVK verifies the
// unscoped ?provider=X request is untouched — no lookup, nothing stashed.
func TestApplyListModelsTeamACLForExplicitProviderSkipsWithoutVK(t *testing.T) {
	h := &CompletionHandler{config: &lib.Config{ConfigStore: &mockListModelsVKConfigStore{}}}

	ctx := &fasthttp.RequestCtx{}
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsTeamACLForExplicitProvider(ctx, bifrostCtx); !ok {
		t.Fatalf("expected a request without a virtual key to pass through")
	}
	if policies := teamModelPoliciesFromBifrostContext(bifrostCtx); len(policies) != 0 {
		t.Fatalf("expected no stashed policies, got %#v", policies)
	}
}

// TestApplyListModelsTeamACLForExplicitProviderFailsClosedOnLookupError mirrors
// the aggregate path's fail-closed direction.
func TestApplyListModelsTeamACLForExplicitProviderFailsClosedOnLookupError(t *testing.T) {
	h := &CompletionHandler{
		config: &lib.Config{
			ConfigStore: &mockListModelsVKConfigStore{
				vk:              teamScopedVK("sk-bf-team", "openai"),
				teamPoliciesErr: errors.New("database unavailable"),
			},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Bearer sk-bf-team")
	bifrostCtx := schemas.NewBifrostContext(context.Background(), time.Time{})

	if ok := h.applyListModelsTeamACLForExplicitProvider(ctx, bifrostCtx); ok {
		t.Fatalf("expected team policy lookup error to fail the request")
	}
	if got := ctx.Response.StatusCode(); got != fasthttp.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusInternalServerError, got)
	}
}
