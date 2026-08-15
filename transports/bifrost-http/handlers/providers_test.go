package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore"
	configstoreTables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/maximhq/bifrost/framework/modelcatalog"
	"github.com/maximhq/bifrost/framework/modelcatalog/datasheet"
	governanceplugin "github.com/maximhq/bifrost/plugins/governance"
	"github.com/maximhq/bifrost/transports/bifrost-http/lib"
	"github.com/valyala/fasthttp"
)

// mockModelsManager returns stable filtered and unfiltered model lists for handler tests.
// providerKeyRef records a (provider, keyID) pair a refresh was requested for.
type providerKeyRef struct {
	provider schemas.ModelProvider
	keyID    string
}

type mockModelsManager struct {
	filtered             map[schemas.ModelProvider][]string
	unfiltered           map[schemas.ModelProvider][]string
	reloadCalls          []schemas.ModelProvider
	reloadErr            error
	refreshKeyCalls      []providerKeyRef
	refreshProviderCalls []schemas.ModelProvider
	refreshErr           error
}

func (m *mockModelsManager) ReloadProvider(_ context.Context, provider schemas.ModelProvider) (*configstoreTables.TableProvider, error) {
	m.reloadCalls = append(m.reloadCalls, provider)
	if m.reloadErr != nil {
		return nil, m.reloadErr
	}
	return nil, nil
}

func (m *mockModelsManager) RemoveProvider(_ context.Context, _ schemas.ModelProvider) error {
	return nil
}

func (m *mockModelsManager) GetModelsForProvider(provider schemas.ModelProvider) []string {
	models := m.filtered[provider]
	result := make([]string, len(models))
	copy(result, models)
	return result
}

func (m *mockModelsManager) GetUnfilteredModelsForProvider(provider schemas.ModelProvider) []string {
	models := m.unfiltered[provider]
	result := make([]string, len(models))
	copy(result, models)
	return result
}

func (m *mockModelsManager) UpsertModelPricingAttributes(_ context.Context, _ []ModelPricingAttributesEntry) error {
	return nil
}

func (m *mockModelsManager) OnKeyAdded(_ context.Context, _ schemas.ModelProvider, _ schemas.Key) error {
	return nil
}

func (m *mockModelsManager) OnKeyUpdated(_ context.Context, _ schemas.ModelProvider, _ schemas.Key) error {
	return nil
}

func (m *mockModelsManager) OnKeyDeleted(_ context.Context, _ schemas.ModelProvider, _ string) error {
	return nil
}

func (m *mockModelsManager) RefreshLiveModelsForKey(_ context.Context, provider schemas.ModelProvider, keyID string) error {
	m.refreshKeyCalls = append(m.refreshKeyCalls, providerKeyRef{provider: provider, keyID: keyID})
	return m.refreshErr
}

func (m *mockModelsManager) RefreshLiveModelsForAllKeys(_ context.Context, provider schemas.ModelProvider) error {
	m.refreshProviderCalls = append(m.refreshProviderCalls, provider)
	return m.refreshErr
}

// providerHandlerForTest builds a handler with fixed provider config and model sets.
func providerHandlerForTest(provider schemas.ModelProvider, keys []schemas.Key, filtered, unfiltered []string) *ProviderHandler {
	return &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				provider: {
					Keys: keys,
				},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				provider: filtered,
			},
			unfiltered: map[schemas.ModelProvider][]string{
				provider: unfiltered,
			},
		},
	}
}

func TestAddProvider_ReloadsRuntimeEvenWhenModelDiscoveryIsSkipped(t *testing.T) {
	SetLogger(&mockLogger{})
	lib.SetLogger(&mockLogger{})

	modelsManager := &mockModelsManager{}
	h := &ProviderHandler{
		inMemoryStore: &lib.Config{Providers: map[schemas.ModelProvider]configstore.ProviderConfig{}},
		modelsManager: modelsManager,
	}

	body, err := sonic.Marshal(providerCreatePayload{
		Provider: "mock-openai",
		CustomProviderConfig: &schemas.CustomProviderConfig{
			BaseProviderType: schemas.OpenAI,
			IsKeyLess:        true,
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/api/providers")
	ctx.Request.SetBody(body)

	h.addProvider(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}
	if len(modelsManager.reloadCalls) != 1 || modelsManager.reloadCalls[0] != "mock-openai" {
		t.Fatalf("expected provider reload for mock-openai, got %#v", modelsManager.reloadCalls)
	}
	if _, exists := h.inMemoryStore.Providers["mock-openai"]; !exists {
		t.Fatalf("expected provider to be added to in-memory store")
	}
}

func TestAddProvider_ReturnsErrorWhenRuntimeReloadFails(t *testing.T) {
	SetLogger(&mockLogger{})
	lib.SetLogger(&mockLogger{})

	modelsManager := &mockModelsManager{reloadErr: context.DeadlineExceeded}
	h := &ProviderHandler{
		inMemoryStore: &lib.Config{Providers: map[schemas.ModelProvider]configstore.ProviderConfig{}},
		modelsManager: modelsManager,
	}

	body, err := sonic.Marshal(providerCreatePayload{
		Provider: "mock-openai",
		CustomProviderConfig: &schemas.CustomProviderConfig{
			BaseProviderType: schemas.OpenAI,
			IsKeyLess:        true,
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/api/providers")
	ctx.Request.SetBody(body)

	h.addProvider(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}
	if len(modelsManager.reloadCalls) != 1 || modelsManager.reloadCalls[0] != "mock-openai" {
		t.Fatalf("expected single provider reload for mock-openai, got %#v", modelsManager.reloadCalls)
	}
	var bifrostErr schemas.BifrostError
	if err := json.Unmarshal(ctx.Response.Body(), &bifrostErr); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if bifrostErr.Error == nil || bifrostErr.Error.Message == "" {
		t.Fatalf("expected error message in response, got %#v", bifrostErr)
	}
	if bifrostErr.Error.Message != "Failed to initialize provider after add: context deadline exceeded" {
		t.Fatalf("unexpected error message: %q", bifrostErr.Error.Message)
	}
	if _, exists := h.inMemoryStore.Providers["mock-openai"]; exists {
		t.Fatalf("expected provider rollback after reload failure")
	}
}

// TestUpdateProvider_RejectsKeysInBody guards against a silent-discard regression
// where `keys` is decoded into `payload.Keys` but never written to the persisted
// `ProviderConfig`. The endpoint manages provider-level config only; key edits
// must go through PUT /api/providers/{provider}/keys/{key_id}. Without this
// guard, callers (third-party API users, older dashboard bundles, integration
// tests) get HTTP 200 with their `blacklisted_models`/`weight`/etc. silently
// dropped — and the in-memory cache is rewritten with the stale `oldConfigRaw`
// keys, causing list/per-key endpoints to diverge from the DB.
func TestUpdateProvider_RejectsKeysInBody(t *testing.T) {
	SetLogger(&mockLogger{})
	lib.SetLogger(&mockLogger{})

	existingKey := schemas.Key{
		ID:                "key-existing",
		Models:            []string{"*"},
		BlacklistedModels: []string{"gpt-3.5-turbo"},
		Weight:            0.8,
	}
	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI: {Keys: []schemas.Key{existingKey}},
			},
		},
		modelsManager: &mockModelsManager{},
	}

	body, err := sonic.Marshal(struct {
		Keys                     []schemas.Key                    `json:"keys"`
		NetworkConfig            schemas.NetworkConfig            `json:"network_config"`
		ConcurrencyAndBufferSize schemas.ConcurrencyAndBufferSize `json:"concurrency_and_buffer_size"`
	}{
		Keys: []schemas.Key{{
			ID:                "key-existing",
			Models:            []string{"*"},
			BlacklistedModels: []string{"gpt-4o", "o1-preview"},
			Weight:            0.42,
		}},
		ConcurrencyAndBufferSize: schemas.ConcurrencyAndBufferSize{
			Concurrency: 1000,
			BufferSize:  5000,
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPut)
	ctx.Request.SetRequestURI("/api/providers/openai")
	ctx.Request.SetBody(body)
	ctx.SetUserValue("provider", string(schemas.OpenAI))

	h.updateProvider(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var bifrostErr schemas.BifrostError
	if err := json.Unmarshal(ctx.Response.Body(), &bifrostErr); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if bifrostErr.Error == nil || bifrostErr.Error.Message == "" {
		t.Fatalf("expected error message in response, got %#v", bifrostErr)
	}
	if !strings.Contains(bifrostErr.Error.Message, "/keys") {
		t.Fatalf("expected error message to mention the /keys endpoint, got %q", bifrostErr.Error.Message)
	}

	// In-memory cache must NOT have been mutated by the rejected request.
	stored, ok := h.inMemoryStore.Providers[schemas.OpenAI]
	if !ok || len(stored.Keys) != 1 {
		t.Fatalf("expected provider to retain its single existing key, got %#v", stored)
	}
	if stored.Keys[0].Weight != 0.8 || len(stored.Keys[0].BlacklistedModels) != 1 || stored.Keys[0].BlacklistedModels[0] != "gpt-3.5-turbo" {
		t.Fatalf("expected key to be untouched (weight=0.8, blacklisted=[gpt-3.5-turbo]); got weight=%v blacklisted=%v",
			stored.Keys[0].Weight, stored.Keys[0].BlacklistedModels)
	}
}

// TestUpdateProvider_PassesThroughForEmptyOrAbsentKeys locks in the explicit
// promise that the keys-guard only rejects NON-empty `keys` arrays. A future
// refactor that accidentally tightens the guard to `payload.Keys != nil` (or
// silently strips the field with `json:",omitempty"`) would silently break
// provider-level config saves that legitimately include an empty/null `keys`
// field, so we assert the guard does NOT fire for those cases.
//
// We can't easily run the handler all the way through to a 200 here because
// `inMemoryStore.UpdateProviderConfig` requires a real *bifrost.Bifrost client
// that's out of scope for a unit test. Instead, we deliberately send
// `concurrency: 0` so the handler short-circuits with a deterministic 400
// from the concurrency validator that lives AFTER the keys-guard. The
// invariant under test is: the error we get is the concurrency error, not the
// keys-not-accepted error.
func TestUpdateProvider_PassesThroughForEmptyOrAbsentKeys(t *testing.T) {
	SetLogger(&mockLogger{})
	lib.SetLogger(&mockLogger{})

	cases := []struct {
		name string
		body string
	}{
		{
			name: "keys field omitted entirely",
			body: `{
				"network_config": {},
				"concurrency_and_buffer_size": {"concurrency": 0, "buffer_size": 0}
			}`,
		},
		{
			name: "keys explicitly null",
			body: `{
				"keys": null,
				"network_config": {},
				"concurrency_and_buffer_size": {"concurrency": 0, "buffer_size": 0}
			}`,
		},
		{
			name: "keys explicitly empty array",
			body: `{
				"keys": [],
				"network_config": {},
				"concurrency_and_buffer_size": {"concurrency": 0, "buffer_size": 0}
			}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &ProviderHandler{
				inMemoryStore: &lib.Config{
					Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
						schemas.OpenAI: {Keys: []schemas.Key{{ID: "key-existing"}}},
					},
				},
				modelsManager: &mockModelsManager{},
			}

			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(fasthttp.MethodPut)
			ctx.Request.SetRequestURI("/api/providers/openai")
			ctx.Request.SetBody([]byte(tc.body))
			ctx.SetUserValue("provider", string(schemas.OpenAI))

			h.updateProvider(ctx)

			if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
				t.Fatalf("expected 400 (from concurrency validator, NOT keys-guard), got %d: %s",
					ctx.Response.StatusCode(), string(ctx.Response.Body()))
			}

			var bifrostErr schemas.BifrostError
			if err := json.Unmarshal(ctx.Response.Body(), &bifrostErr); err != nil {
				t.Fatalf("failed to unmarshal error response: %v", err)
			}
			if bifrostErr.Error == nil {
				t.Fatalf("expected error in response, got %#v", bifrostErr)
			}
			if strings.Contains(bifrostErr.Error.Message, "keys are not accepted on this endpoint") {
				t.Fatalf("keys-guard should NOT fire for empty/absent keys, got: %s", bifrostErr.Error.Message)
			}
			if !strings.Contains(bifrostErr.Error.Message, "Concurrency") {
				t.Fatalf("expected concurrency error (proves we passed the keys-guard), got: %s", bifrostErr.Error.Message)
			}
		})
	}
}

func modelCatalogForPricingJSON(t *testing.T, pricingJSON []byte) *modelcatalog.ModelCatalog {
	t.Helper()
	pricingPath := filepath.Join(t.TempDir(), "pricing.json")
	if err := os.WriteFile(pricingPath, pricingJSON, 0o600); err != nil {
		t.Fatalf("write pricing testdata: %v", err)
	}
	ds := datasheet.New(nil, nil, datasheet.Config{URL: "file://" + pricingPath})
	if err := ds.LoadFromURLIntoMemory(t.Context()); err != nil {
		t.Fatalf("load pricing testdata: %v", err)
	}
	return modelcatalog.NewTestCatalogWithDatasheet(ds)
}

func TestListModels_UnknownKeysDoNotFilter(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{{ID: "key-a"}},
		[]string{"gpt-4o", "gpt-4o-mini"},
		[]string{"gpt-4o", "gpt-4o-mini"},
	)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=openai&keys=missing")

	h.listModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 2 {
		t.Fatalf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Models) != 2 {
		t.Fatalf("expected all models to be returned, got %#v", resp.Models)
	}
	for _, model := range resp.Models {
		if len(model.AccessibleByKeys) != 0 {
			t.Fatalf("expected no accessible_by_keys annotations, got %#v", resp.Models)
		}
	}
}

func TestListModels_ReturnsExactAccessibleByKeysAndSkipsDisabledKeys(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{
			{ID: "key-a", Models: []string{"gpt-4o"}},
			{ID: "key-b", Models: []string{"gpt-4o", "gpt-4o-mini"}},
			{ID: "key-disabled", Enabled: new(false)},
		},
		[]string{"gpt-4o", "gpt-4o-mini"},
		[]string{"gpt-4o", "gpt-4o-mini"},
	)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=openai&keys=key-a,key-b,key-disabled")

	h.listModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 2 {
		t.Fatalf("expected total=2, got %d", resp.Total)
	}

	got := map[string][]string{}
	for _, model := range resp.Models {
		got[model.Name] = model.AccessibleByKeys
	}

	if len(got["gpt-4o"]) != 2 || got["gpt-4o"][0] != "key-a" || got["gpt-4o"][1] != "key-b" {
		t.Fatalf("expected gpt-4o to be accessible by [key-a key-b], got %#v", got["gpt-4o"])
	}
	if len(got["gpt-4o-mini"]) != 1 || got["gpt-4o-mini"][0] != "key-b" {
		t.Fatalf("expected gpt-4o-mini to be accessible by [key-b], got %#v", got["gpt-4o-mini"])
	}
}

func TestListModels_AppliesQueryAndLimitAfterFiltering(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{{ID: "key-a"}},
		[]string{"gpt-4o", "gpt-4o-mini", "claude-3-5-sonnet"},
		[]string{"gpt-4o", "gpt-4o-mini", "claude-3-5-sonnet"},
	)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=openai&query=gpt&limit=1")

	h.listModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 2 {
		t.Fatalf("expected total=2 after query filtering, got %d", resp.Total)
	}
	if len(resp.Models) != 1 {
		t.Fatalf("expected limit to truncate response to 1 model, got %#v", resp.Models)
	}
	if resp.Models[0].Name != "gpt-4o" {
		t.Fatalf("expected first filtered model to be gpt-4o, got %#v", resp.Models[0])
	}
}

func TestListModels_MarksDeprecatedModelsWithoutFiltering(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{{ID: "key-a"}},
		[]string{"deprecated-model", "current-model", "another-current-model"},
		[]string{"deprecated-model", "current-model", "another-current-model"},
	)

	pricingJSON := []byte(`{
		"deprecated-model": {"provider":"openai","mode":"chat","base_model":"deprecated-model","is_deprecated":true},
		"current-model": {"provider":"openai","mode":"chat","base_model":"current-model"},
		"another-current-model": {"provider":"openai","mode":"chat","base_model":"another-current-model"}
	}`)
	h.inMemoryStore.ModelCatalog = modelCatalogForPricingJSON(t, pricingJSON)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=openai&limit=10")

	h.listModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 3 {
		t.Fatalf("expected total=3 (deprecated models are not filtered), got %d", resp.Total)
	}
	var deprecated *ModelResponse
	for i := range resp.Models {
		if resp.Models[i].Name == "deprecated-model" {
			deprecated = &resp.Models[i]
		}
	}
	if deprecated == nil {
		t.Fatalf("deprecated model should still be returned, got %#v", resp.Models)
	}
	if !deprecated.IsDeprecated {
		t.Fatalf("deprecated model should carry is_deprecated=true, got %#v", *deprecated)
	}
}

func TestListBaseModels_IncludesDeprecatedPricingRows(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{{ID: "key-a"}},
		nil,
		nil,
	)
	h.inMemoryStore.ModelCatalog = modelCatalogForPricingJSON(t, []byte(`{
		"deprecated-model": {"provider":"openai","mode":"chat","base_model":"deprecated-base","is_deprecated":true},
		"current-model": {"provider":"openai","mode":"chat","base_model":"current-base"}
	}`))

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models/base?limit=10")

	h.listBaseModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListBaseModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Total != 2 || !slices.Contains(resp.Models, "current-base") || !slices.Contains(resp.Models, "deprecated-base") {
		t.Fatalf("expected both base models, got %#v", resp)
	}
}

func TestEnrichListModelsResponse_MarksDeprecatedPricingRows(t *testing.T) {
	catalog := modelCatalogForPricingJSON(t, []byte(`{
		"deprecated-model": {"provider":"openai","mode":"chat","base_model":"deprecated-model","is_deprecated":true},
		"current-model": {"provider":"openai","mode":"chat","base_model":"current-model"}
	}`))
	resp := &schemas.BifrostListModelsResponse{Data: []schemas.Model{
		{ID: "openai/deprecated-model"},
		{ID: "openai/current-model"},
		{ID: "openai/provider-deprecated", IsDeprecated: true},
	}}

	enrichListModelsResponse(resp, catalog)

	if len(resp.Data) != 3 {
		t.Fatalf("expected all models retained, got %#v", resp.Data)
	}
	byID := map[string]schemas.Model{}
	for _, m := range resp.Data {
		byID[m.ID] = m
	}
	if !byID["openai/deprecated-model"].IsDeprecated {
		t.Fatalf("catalog-deprecated model should be marked deprecated: %#v", byID["openai/deprecated-model"])
	}
	if byID["openai/current-model"].IsDeprecated {
		t.Fatalf("current model should not be marked deprecated: %#v", byID["openai/current-model"])
	}
	if !byID["openai/provider-deprecated"].IsDeprecated {
		t.Fatalf("provider-deprecated flag should be preserved: %#v", byID["openai/provider-deprecated"])
	}
}

func TestListModels_UnfilteredIgnoresKeys(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{
			{ID: "key-b", Models: []string{"gpt-4o-mini"}},
		},
		[]string{"gpt-4o"},
		[]string{"gpt-4o", "gpt-4o-mini"},
	)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=openai&keys=key-b&unfiltered=true")

	h.listModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 2 || len(resp.Models) != 2 {
		t.Fatalf("expected both unfiltered models, got %#v", resp.Models)
	}

	for _, model := range resp.Models {
		if len(model.AccessibleByKeys) != 0 {
			t.Fatalf("expected no accessible_by_keys when unfiltered bypasses key filtering, got %#v", resp.Models)
		}
	}
}

func TestListModels_UnfilteredWithoutKeysReturnsAllUnfilteredModels(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{
			{ID: "key-b", Models: []string{"gpt-4o-mini"}},
		},
		[]string{"gpt-4o"},
		[]string{"gpt-4o", "gpt-4o-mini"},
	)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=openai&unfiltered=true")

	h.listModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 2 || len(resp.Models) != 2 {
		t.Fatalf("expected both unfiltered models, got %#v", resp.Models)
	}

	for _, model := range resp.Models {
		if len(model.AccessibleByKeys) != 0 {
			t.Fatalf("expected no accessible_by_keys when no key filter is requested, got %#v", resp.Models)
		}
	}
}

func TestListModelDetails_ErrorsWhenModelCatalogUnavailable(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{{ID: "key-a"}},
		[]string{"gpt-4o"},
		[]string{"gpt-4o"},
	)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models/details?provider=openai")

	h.listModelDetails(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}
}

func TestListModelDetails_UnknownKeysDoNotFilter(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{{ID: "key-a"}},
		[]string{"gpt-4o", "gpt-4o-mini"},
		[]string{"gpt-4o", "gpt-4o-mini"},
	)
	h.inMemoryStore.ModelCatalog = modelcatalog.NewTestCatalog(nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models/details?provider=openai&keys=missing")

	h.listModelDetails(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelDetailsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 2 || len(resp.Models) != 2 {
		t.Fatalf("expected all models when keys are unknown, got %#v", resp.Models)
	}
}

func TestListModelDetails_SkipsUnknownKeysAndFiltersWithValid(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{{ID: "key-a", Models: []string{"gpt-4o"}}},
		[]string{"gpt-4o", "gpt-4o-mini"},
		[]string{"gpt-4o", "gpt-4o-mini"},
	)
	h.inMemoryStore.ModelCatalog = modelcatalog.NewTestCatalog(nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models/details?provider=openai&keys=key-a,missing")

	h.listModelDetails(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelDetailsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 1 || len(resp.Models) != 1 {
		t.Fatalf("expected 1 model filtered by valid key, got %#v", resp.Models)
	}
	if resp.Models[0].Name != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", resp.Models[0].Name)
	}
}

func TestListModelDetails_SkipsDisabledKeysAndFiltersWithValid(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{
			{ID: "key-a", Models: []string{"gpt-4o"}},
			{ID: "key-disabled", Enabled: new(false)},
		},
		[]string{"gpt-4o", "gpt-4o-mini"},
		[]string{"gpt-4o", "gpt-4o-mini"},
	)
	h.inMemoryStore.ModelCatalog = modelcatalog.NewTestCatalog(nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models/details?provider=openai&keys=key-a,key-disabled")

	h.listModelDetails(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelDetailsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 1 || len(resp.Models) != 1 {
		t.Fatalf("expected 1 model filtered by valid key, got %#v", resp.Models)
	}
	if resp.Models[0].Name != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", resp.Models[0].Name)
	}
}

func TestListModelDetails_UnfilteredIgnoresKeys(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{
			{ID: "key-b", Models: []string{"gpt-4o-mini"}},
		},
		[]string{"gpt-4o"},
		[]string{"gpt-4o", "gpt-4o-mini"},
	)
	h.inMemoryStore.ModelCatalog = modelcatalog.NewTestCatalog(nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models/details?provider=openai&keys=key-b&unfiltered=true")

	h.listModelDetails(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelDetailsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 2 || len(resp.Models) != 2 {
		t.Fatalf("expected all unfiltered models when unfiltered=true, got %#v", resp.Models)
	}
}

func TestListModelDetails_IncludesPricing(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{{ID: "key-a"}},
		[]string{"gpt-4o"},
		[]string{"gpt-4o"},
	)
	h.inMemoryStore.ModelCatalog = modelCatalogForPricingJSON(t, []byte(`{
		"gpt-4o": {
			"provider": "openai",
			"mode": "chat",
			"input_cost_per_token": 0.0000025,
			"output_cost_per_token": 0.00001,
			"cache_creation_input_token_cost": 0.000003125,
			"cache_read_input_token_cost": 0.00000025,
			"max_input_tokens": 128000,
			"max_output_tokens": 16384
		}
	}`))

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models/details?provider=openai&limit=100")

	h.listModelDetails(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelDetailsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 1 || len(resp.Models) != 1 {
		t.Fatalf("expected one model, got %#v", resp.Models)
	}
	if resp.Models[0].InputCostPerToken == nil || *resp.Models[0].InputCostPerToken != 0.0000025 {
		t.Fatalf("expected input cost 0.0000025, got %#v", resp.Models[0].InputCostPerToken)
	}
	if resp.Models[0].OutputCostPerToken == nil || *resp.Models[0].OutputCostPerToken != 0.00001 {
		t.Fatalf("expected output cost 0.00001, got %#v", resp.Models[0].OutputCostPerToken)
	}
	if resp.Models[0].CacheWriteCost == nil || *resp.Models[0].CacheWriteCost != 0.000003125 {
		t.Fatalf("expected cache write cost 0.000003125, got %#v", resp.Models[0].CacheWriteCost)
	}
	if resp.Models[0].CacheReadCost == nil || *resp.Models[0].CacheReadCost != 0.00000025 {
		t.Fatalf("expected cache read cost 0.00000025, got %#v", resp.Models[0].CacheReadCost)
	}
}

// --- VK-based filtering tests ---

// TestParseVKValueFromRequest verifies that the VK value is extracted from each
// supported header, in priority order, and that non-VK values are ignored.
func TestParseVKValueFromRequest(t *testing.T) {
	const vk = "sk-bf-test-virtual-key"

	cases := []struct {
		name   string
		setup  func(*fasthttp.RequestCtx)
		wantVK string
	}{
		{
			name: "x-bf-vk header",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.Header.Set("x-bf-vk", vk)
			},
			wantVK: vk,
		},
		{
			name: "Authorization Bearer header",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.Header.Set("Authorization", "Bearer "+vk)
			},
			wantVK: vk,
		},
		{
			name: "x-api-key header",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.Header.Set("x-api-key", vk)
			},
			wantVK: vk,
		},
		{
			name: "x-goog-api-key header",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.Header.Set("x-goog-api-key", vk)
			},
			wantVK: vk,
		},
		{
			name:   "no header returns empty string",
			setup:  func(*fasthttp.RequestCtx) {},
			wantVK: "",
		},
		{
			name: "non-VK Bearer token returns empty string",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.Header.Set("Authorization", "Bearer regular-api-key-123")
			},
			wantVK: "",
		},
		{
			name: "x-bf-vk takes priority over Authorization",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.Header.Set("x-bf-vk", vk)
				ctx.Request.Header.Set("Authorization", "Bearer sk-bf-other")
			},
			wantVK: vk,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tc.setup(ctx)
			got := governanceplugin.ParseVirtualKeyFromFastHTTPRequest(ctx)
			gotValue := ""
			if got != nil {
				gotValue = *got
			}
			if gotValue != tc.wantVK {
				t.Fatalf("expected %q, got %q", tc.wantVK, gotValue)
			}
		})
	}
}

// TestListModels_VKFilterRestrictsToAllowedProviderAndModels verifies that when a
// VK filter is active, only providers listed in VKProviderConfigs are returned and
// only models passing AllowedModels are included.
func TestListModels_VKFilterRestrictsToAllowedProviderAndModels(t *testing.T) {
	SetLogger(&mockLogger{})

	// Two providers configured; VK only allows openai with specific models.
	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI:    {Keys: []schemas.Key{{ID: "key-a"}}},
				schemas.Anthropic: {Keys: []schemas.Key{{ID: "key-b"}}},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				schemas.OpenAI:    {"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
				schemas.Anthropic: {"claude-3-5-sonnet", "claude-3-haiku"},
			},
		},
	}

	query := modelListQuery{
		Limit:       100,
		HasVKFilter: true,
		VKProviderConfigs: []configstoreTables.TableVirtualKeyProviderConfig{
			{
				Provider:      "openai",
				AllowedModels: schemas.WhiteList{"gpt-4o", "gpt-4o-mini"},
			},
		},
	}

	models, total, err := h.listManagementModels(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	for _, m := range models {
		if m.Provider != schemas.OpenAI {
			t.Fatalf("expected only openai models, got provider %s", m.Provider)
		}
	}
	names := map[string]bool{}
	for _, m := range models {
		names[m.Name] = true
	}
	if !names["gpt-4o"] || !names["gpt-4o-mini"] {
		t.Fatalf("expected gpt-4o and gpt-4o-mini, got %v", models)
	}
	if names["gpt-3.5-turbo"] {
		t.Fatalf("gpt-3.5-turbo should be denied by AllowedModels")
	}
	if names["claude-3-5-sonnet"] || names["claude-3-haiku"] {
		t.Fatalf("anthropic models should be excluded by VK provider filter")
	}
}

// TestListModels_VKFilterAllowsAllModelsWithWildcard verifies that AllowedModels=["*"]
// passes all provider models through.
func TestListModels_VKFilterAllowsAllModelsWithWildcard(t *testing.T) {
	SetLogger(&mockLogger{})

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI: {Keys: []schemas.Key{{ID: "key-a"}}},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				schemas.OpenAI: {"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
			},
		},
	}

	query := modelListQuery{
		Limit:       100,
		HasVKFilter: true,
		VKProviderConfigs: []configstoreTables.TableVirtualKeyProviderConfig{
			{Provider: "openai", AllowedModels: schemas.WhiteList{"*"}},
		},
	}

	models, total, err := h.listManagementModels(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected all 3 models with wildcard, got total=%d", total)
	}
	_ = models
}

// TestListModels_VKFilterDeniesAllModelsWhenAllowedModelsEmpty verifies deny-by-default:
// a VK that lists a provider but with an empty AllowedModels returns 0 models.
func TestListModels_VKFilterDeniesAllModelsWhenAllowedModelsEmpty(t *testing.T) {
	SetLogger(&mockLogger{})

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI: {Keys: []schemas.Key{{ID: "key-a"}}},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				schemas.OpenAI: {"gpt-4o", "gpt-4o-mini"},
			},
		},
	}

	query := modelListQuery{
		Limit:       100,
		HasVKFilter: true,
		VKProviderConfigs: []configstoreTables.TableVirtualKeyProviderConfig{
			{Provider: "openai", AllowedModels: schemas.WhiteList{}},
		},
	}

	models, total, err := h.listManagementModels(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(models) != 0 {
		t.Fatalf("expected 0 models with empty AllowedModels (deny-by-default), got total=%d %v", total, models)
	}
}

// TestListModels_VKFilterNoProviderConfigsDeniesAll verifies that a VK with no
// ProviderConfigs returns 0 models (deny-by-default at provider level).
func TestListModels_VKFilterNoProviderConfigsDeniesAll(t *testing.T) {
	SetLogger(&mockLogger{})

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI:    {},
				schemas.Anthropic: {},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				schemas.OpenAI:    {"gpt-4o"},
				schemas.Anthropic: {"claude-3-5-sonnet"},
			},
		},
	}

	query := modelListQuery{
		Limit:             100,
		HasVKFilter:       true,
		VKProviderConfigs: []configstoreTables.TableVirtualKeyProviderConfig{}, // empty
	}

	models, total, err := h.listManagementModels(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(models) != 0 {
		t.Fatalf("expected 0 models when VK has no provider configs, got total=%d", total)
	}
}

func TestListModels_VKFilterBlockedExplicitProviderReturnsEmptyResult(t *testing.T) {
	SetLogger(&mockLogger{})

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI:    {},
				schemas.Anthropic: {},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				schemas.OpenAI:    {"gpt-4o"},
				schemas.Anthropic: {"claude-3-5-sonnet"},
			},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=anthropic")
	query, ok := h.parseModelListQuery(ctx, 5)
	if !ok {
		t.Fatalf("expected parseModelListQuery to succeed")
	}
	query.HasVKFilter = true
	query.VKProviderConfigs = []configstoreTables.TableVirtualKeyProviderConfig{
		{Provider: "openai", AllowedModels: schemas.WhiteList{"*"}},
	}

	models, total, err := h.listManagementModels(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(models) != 0 {
		t.Fatalf("expected blocked explicit provider to return no models, got total=%d models=%#v", total, models)
	}
}

func TestParseModelListQuery_VKWithoutDBStoreReturnsServiceUnavailable(t *testing.T) {
	SetLogger(&mockLogger{})

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI: {},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				schemas.OpenAI: {"gpt-4o", "gpt-4o-mini"},
			},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models")
	ctx.Request.Header.Set("x-bf-vk", "sk-bf-test-virtual-key")

	query, ok := h.parseModelListQuery(ctx, 5)
	if ok {
		t.Fatalf("expected parseModelListQuery to fail without dbStore, got query=%#v", query)
	}
	if ctx.Response.StatusCode() != fasthttp.StatusServiceUnavailable {
		t.Fatalf("expected 503 when dbStore is unavailable, got %d", ctx.Response.StatusCode())
	}
}

// TestListModels_NoVKFilterReturnsAll verifies that without a VK filter the endpoint
// returns all providers and models as normal.
func TestListModels_NoVKFilterReturnsAll(t *testing.T) {
	SetLogger(&mockLogger{})

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI:    {},
				schemas.Anthropic: {},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				schemas.OpenAI:    {"gpt-4o"},
				schemas.Anthropic: {"claude-3-5-sonnet"},
			},
		},
	}

	query := modelListQuery{
		Limit:       100,
		HasVKFilter: false, // no filter
	}

	models, total, err := h.listManagementModels(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 || len(models) != 2 {
		t.Fatalf("expected 2 models (one per provider), got total=%d", total)
	}
}

func TestListModels_UsesCatalogAwareAliasMatchingForKeyAllowlist(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{
			{ID: "key-a", Models: []string{"gpt-4o-2024-08-06"}},
		},
		[]string{"gpt-4o"},
		[]string{"gpt-4o"},
	)
	h.inMemoryStore.ModelCatalog = modelcatalog.NewTestCatalog(map[string]string{
		"gpt-4o-2024-08-06": "gpt-4o",
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=openai&keys=key-a")

	h.listModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 1 || len(resp.Models) != 1 || resp.Models[0].Name != "gpt-4o" {
		t.Fatalf("expected gpt-4o to be matched through alias allowlist, got %#v", resp.Models)
	}
}

// TestListModels_KeyModelAllowlistIsCaseInsensitive verifies that key.Models matching
// uses case-insensitive comparison so "GPT-4O" in the allowlist matches "gpt-4o" in the pool.
func TestListModels_KeyModelAllowlistIsCaseInsensitive(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{
			{ID: "key-a", Models: []string{"GPT-4O", "GPT-4O-MINI"}},
		},
		[]string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
		[]string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
	)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=openai&keys=key-a&limit=10")

	h.listModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 2 {
		t.Fatalf("expected total=2 (gpt-4o and gpt-4o-mini matched case-insensitively), got total=%d %v", resp.Total, resp.Models)
	}
	names := map[string]bool{}
	for _, m := range resp.Models {
		names[m.Name] = true
	}
	if !names["gpt-4o"] || !names["gpt-4o-mini"] {
		t.Fatalf("expected gpt-4o and gpt-4o-mini, got %v", resp.Models)
	}
	if names["gpt-3.5-turbo"] {
		t.Fatalf("gpt-3.5-turbo should not be returned (not in key allowlist)")
	}
}

// TestListModels_KeyBlacklistIsCaseInsensitive verifies that key.BlacklistedModels uses
// case-insensitive matching so "GPT-3.5-TURBO" blocks "gpt-3.5-turbo" in the pool.
func TestListModels_KeyBlacklistIsCaseInsensitive(t *testing.T) {
	SetLogger(&mockLogger{})

	h := providerHandlerForTest(
		schemas.OpenAI,
		[]schemas.Key{
			{ID: "key-a", BlacklistedModels: []string{"GPT-3.5-TURBO"}},
		},
		[]string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
		[]string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
	)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/models?provider=openai&keys=key-a&limit=10")

	h.listModels(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 2 {
		t.Fatalf("expected total=2 (gpt-3.5-turbo blocked case-insensitively), got total=%d %v", resp.Total, resp.Models)
	}
	for _, m := range resp.Models {
		if strings.EqualFold(m.Name, "gpt-3.5-turbo") {
			t.Fatalf("gpt-3.5-turbo should be blocked by blacklist, got %v", resp.Models)
		}
	}
}

// TestListProvidersAggregatedFields verifies that GET /api/providers returns the 9 new
// aggregation fields (keys_count, models_count, keys_health_status, today_requests,
// today_errors, last_used_at, last_error_at, uptime, avg_latency_ms) with correct values.
func TestListProvidersAggregatedFields(t *testing.T) {
	SetLogger(&mockLogger{})
	lib.SetLogger(&mockLogger{})

	provider := schemas.OpenAI
	keys := []schemas.Key{
		{ID: "key-1", Enabled: ptr(true)},
		{ID: "key-2", Enabled: ptr(true)},
		{ID: "key-3", Enabled: ptr(true)},
	}
	models := []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo", "o1-preview", "o1-mini"}

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				provider: {
					Keys: keys,
				},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				provider: models,
			},
			unfiltered: map[schemas.ModelProvider][]string{
				provider: models,
			},
		},
		// Mock the logs store so the today_* / last_* / avg_latency aggregates are
		// deterministic: 100 today requests, 3 errors, known timestamps, 312ms avg.
		logStats: &mockProviderLogStats{
			todayRequests: 100,
			todayErrors:   3,
			lastUsedAt:    "2026-08-15T01:42:00Z",
			lastErrorAt:   "2026-08-15T00:15:22Z",
			avgLatencyMs:  312,
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/providers")

	h.listProviders(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListProvidersResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 1 {
		t.Fatalf("expected total=1, got %d", resp.Total)
	}
	if len(resp.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Providers))
	}

	p := resp.Providers[0]
	if p.Name != provider {
		t.Fatalf("expected provider name %q, got %q", provider, p.Name)
	}

	// Assert the 9 new aggregation fields.
	// These fields do not exist yet on ProviderResponse — this is the TDD red phase.
	if p.KeysCount != 3 {
		t.Fatalf("expected keys_count=3, got %d", p.KeysCount)
	}
	if p.ModelsCount != 5 {
		t.Fatalf("expected models_count=5, got %d", p.ModelsCount)
	}
	if p.KeysHealthStatus != "healthy" {
		t.Fatalf("expected keys_health_status=\"healthy\", got %q", p.KeysHealthStatus)
	}
	if !p.KeysEnabled {
		t.Fatalf("expected keys_enabled=true when all keys are enabled, got %v", p.KeysEnabled)
	}
	if p.TodayRequests != 100 {
		t.Fatalf("expected today_requests=100, got %d", p.TodayRequests)
	}
	if p.TodayErrors != 3 {
		t.Fatalf("expected today_errors=3, got %d", p.TodayErrors)
	}
	if p.LastUsedAt == "" {
		t.Fatalf("expected non-empty last_used_at")
	}
	if p.LastErrorAt == "" {
		t.Fatalf("expected non-empty last_error_at")
	}
	if p.Uptime < 0 || p.Uptime > 1 {
		t.Fatalf("expected uptime in [0,1], got %f", p.Uptime)
	}
	if p.AvgLatencyMs <= 0 {
		t.Fatalf("expected positive avg_latency_ms, got %d", p.AvgLatencyMs)
	}
}

// TestListProviders_UsesBatchLogStatsAggregation verifies that the providers
// list path aggregates log stats through a single AggregateAllProviderLogStats
// batch call — never falling back to a per-provider AggregateProviderLogStats
// loop (the design.md performance contract forbids the N+1 pattern).
func TestListProviders_UsesBatchLogStatsAggregation(t *testing.T) {
	SetLogger(&mockLogger{})
	lib.SetLogger(&mockLogger{})

	singleCalls, batchCalls := 0, 0

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI:    {Keys: []schemas.Key{{ID: "key-1", Enabled: ptr(true)}}},
				schemas.Anthropic: {Keys: []schemas.Key{{ID: "key-2", Enabled: ptr(true)}}},
			},
		},
		modelsManager: &mockModelsManager{
			filtered: map[schemas.ModelProvider][]string{
				schemas.OpenAI:    {"gpt-4o"},
				schemas.Anthropic: {"claude-3-5-sonnet"},
			},
		},
		logStats: &mockProviderLogStats{
			todayRequests:   50,
			todayErrors:     2,
			lastUsedAt:      "2026-08-15T01:42:00Z",
			lastErrorAt:     "2026-08-15T00:15:22Z",
			avgLatencyMs:    200,
			singleCallCount: &singleCalls,
			batchCallCount:  &batchCalls,
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/providers")

	h.listProviders(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	// Exactly one batch call for the whole provider set; no per-provider loop.
	if batchCalls != 1 {
		t.Fatalf("expected exactly 1 AggregateAllProviderLogStats call, got %d", batchCalls)
	}
	if singleCalls != 0 {
		t.Fatalf("expected 0 per-provider AggregateProviderLogStats calls, got %d (N+1 loop)", singleCalls)
	}

	var resp ListProvidersResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected total=2, got %d", resp.Total)
	}

	// Both providers received the batch-derived values.
	for _, p := range resp.Providers {
		if p.TodayRequests != 50 || p.TodayErrors != 2 {
			t.Fatalf("provider %s: expected batch stats (today_requests=50, today_errors=2), got (%d, %d)",
				p.Name, p.TodayRequests, p.TodayErrors)
		}
		if p.LastUsedAt == "" || p.LastErrorAt == "" || p.AvgLatencyMs != 200 {
			t.Fatalf("provider %s: expected non-empty timestamps and avg_latency_ms=200, got %#v", p.Name, p)
		}
	}
}

// TestBatchUpdateProviderKeys verifies the happy path of POST /api/providers/{provider}/keys/batch:
// mock provider with 3 keys, POST {"key_ids":[...], "enabled":true} returns 200 + updated=3,
// and all 3 keys are enabled in the DB.
// This test is expected to fail at compile time because batchUpdateProviderKeys and
// BatchUpdateProviderKeysResponse do not exist yet (TDD red phase).
func TestBatchUpdateProviderKeys(t *testing.T) {
	SetLogger(&mockLogger{})
	lib.SetLogger(&mockLogger{})

	keys := []schemas.Key{
		{ID: "k1", Enabled: ptr(false)},
		{ID: "k2", Enabled: ptr(false)},
		{ID: "k3", Enabled: ptr(false)},
	}

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI: {Keys: keys},
			},
		},
		modelsManager: &mockModelsManager{},
	}

	body, err := sonic.Marshal(BatchUpdateProviderKeysRequest{
		KeyIDs:  []string{"k1", "k2", "k3"},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetRequestURI("/api/providers/openai/keys/batch")
	ctx.Request.SetBody(body)
	ctx.SetUserValue("provider", string(schemas.OpenAI))

	h.batchUpdateProviderKeys(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp BatchUpdateProviderKeysResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Updated != 3 {
		t.Fatalf("expected updated=3, got %d", resp.Updated)
	}
	if len(resp.KeyIDs) != 3 {
		t.Fatalf("expected 3 key_ids, got %d", len(resp.KeyIDs))
	}

	// Verify all 3 keys are now enabled=true in the in-memory store.
	cfg, err := h.inMemoryStore.GetProviderConfigRaw(schemas.OpenAI)
	if err != nil {
		t.Fatalf("failed to get provider config: %v", err)
	}
	for _, keyID := range []string{"k1", "k2", "k3"} {
		found := false
		for _, k := range cfg.Keys {
			if k.ID == keyID {
				found = true
				if k.Enabled == nil || !*k.Enabled {
					t.Fatalf("key %s should be enabled=true, got %v", keyID, k.Enabled)
				}
				break
			}
		}
		if !found {
			t.Fatalf("key %s not found in store", keyID)
		}
	}
}

// TestBatchUpdateProviderKeys_RollbackOnMissing verifies that POST /api/providers/{provider}/keys/batch
// with a non-existent key_id returns 400 + missing_key_ids, and the transaction rolls back so
// no key is updated.
// This test is expected to fail at compile time because the batch types and handler do not
// exist yet (TDD red phase).
func TestBatchUpdateProviderKeys_RollbackOnMissing(t *testing.T) {
	SetLogger(&mockLogger{})
	lib.SetLogger(&mockLogger{})

	keys := []schemas.Key{
		{ID: "k1", Enabled: ptr(false)},
		{ID: "k2", Enabled: ptr(false)},
		{ID: "k3", Enabled: ptr(false)},
	}

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				schemas.OpenAI: {Keys: keys},
			},
		},
		modelsManager: &mockModelsManager{},
	}

	// Request includes "k-bad" which does not exist in the provider.
	body, err := sonic.Marshal(BatchUpdateProviderKeysRequest{
		KeyIDs:  []string{"k1", "k2", "k3", "k-bad"},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetRequestURI("/api/providers/openai/keys/batch")
	ctx.Request.SetBody(body)
	ctx.SetUserValue("provider", string(schemas.OpenAI))

	h.batchUpdateProviderKeys(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var errResp BatchUpdateProviderKeysErrorResponse
	if err := json.Unmarshal(ctx.Response.Body(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}

	if len(errResp.MissingKeyIDs) != 1 || errResp.MissingKeyIDs[0] != "k-bad" {
		t.Fatalf("expected missing_key_ids=[\"k-bad\"], got %v", errResp.MissingKeyIDs)
	}

	// Verify transaction rollback: no key was updated (all remain disabled).
	cfg, err := h.inMemoryStore.GetProviderConfigRaw(schemas.OpenAI)
	if err != nil {
		t.Fatalf("failed to get provider config: %v", err)
	}
	for _, k := range cfg.Keys {
		if k.Enabled != nil && *k.Enabled {
			t.Fatalf("key %s should remain disabled after rollback, got enabled=true", k.ID)
		}
	}
}

// TestProviderResponse_BackwardCompat verifies that the existing ProviderResponse wire
// contract is not broken by the 9 new aggregation fields. It unmarshals a fixture JSON
// response (containing only the original fields) into ListProvidersResponse and asserts
// all original fields are correctly parsed, and the 9 new fields default to zero values
// (omitempty compatibility).
// This test is expected to fail at compile time because the 9 new fields do not exist
// on ProviderResponse yet (TDD red phase).
func TestProviderResponse_BackwardCompat(t *testing.T) {
	// A fixture JSON representing a provider response with only the original fields.
	// This matches the wire format before the 9 aggregation fields are added.
	fixtureJSON := `{
		"providers": [
			{
				"name": "openai",
				"network_config": {
					"base_url": "https://api.openai.com",
					"max_conns_per_host": 5000,
					"max_idle_conn_duration": "30s"
				},
				"concurrency_and_buffer_size": {
					"concurrency": 1000,
					"buffer_size": 5000
				},
				"proxy_config": null,
				"send_back_raw_request": false,
				"send_back_raw_response": false,
				"store_raw_request_response": false,
				"custom_provider_config": null,
				"openai_config": null,
				"provider_status": "active",
				"status": "",
				"description": "OpenAI production provider",
				"config_hash": "abc123"
			},
			{
				"name": "anthropic",
				"network_config": {
					"base_url": "https://api.anthropic.com",
					"max_conns_per_host": 5000,
					"max_idle_conn_duration": "30s"
				},
				"concurrency_and_buffer_size": {
					"concurrency": 500,
					"buffer_size": 2500
				},
				"proxy_config": null,
				"send_back_raw_request": false,
				"send_back_raw_response": false,
				"store_raw_request_response": false,
				"custom_provider_config": null,
				"openai_config": null,
				"provider_status": "active",
				"status": "list_models_failed",
				"description": "Anthropic provider",
				"config_hash": "def456"
			}
		],
		"total": 2
	}`

	var resp ListProvidersResponse
	if err := json.Unmarshal([]byte(fixtureJSON), &resp); err != nil {
		t.Fatalf("failed to unmarshal fixture JSON: %v", err)
	}

	if resp.Total != 2 {
		t.Fatalf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(resp.Providers))
	}

	// Assert all original fields are still present and correctly parsed.
	p := resp.Providers[0]
	if p.Name != schemas.OpenAI {
		t.Fatalf("expected name=openai, got %q", p.Name)
	}
	if p.NetworkConfig.BaseURL != "https://api.openai.com" {
		t.Fatalf("expected network_config.base_url, got %q", p.NetworkConfig.BaseURL)
	}
	if p.ConcurrencyAndBufferSize.Concurrency != 1000 {
		t.Fatalf("expected concurrency=1000, got %d", p.ConcurrencyAndBufferSize.Concurrency)
	}
	if p.ProviderStatus != ProviderStatusActive {
		t.Fatalf("expected provider_status=active, got %q", p.ProviderStatus)
	}
	if p.ConfigHash != "abc123" {
		t.Fatalf("expected config_hash=abc123, got %q", p.ConfigHash)
	}

	// Assert the 9 new fields default to zero values when absent from the wire.
	// These fields have `omitempty` JSON tags and should be zero-valued when missing.
	if p.KeysCount != 0 {
		t.Fatalf("expected keys_count=0 (zero value when absent), got %d", p.KeysCount)
	}
	if p.ModelsCount != 0 {
		t.Fatalf("expected models_count=0 (zero value when absent), got %d", p.ModelsCount)
	}
	if p.KeysHealthStatus != "" {
		t.Fatalf("expected keys_health_status=\"\" (zero value when absent), got %q", p.KeysHealthStatus)
	}
	if p.TodayRequests != 0 {
		t.Fatalf("expected today_requests=0 (zero value when absent), got %d", p.TodayRequests)
	}
	if p.TodayErrors != 0 {
		t.Fatalf("expected today_errors=0 (zero value when absent), got %d", p.TodayErrors)
	}
	if p.LastUsedAt != "" {
		t.Fatalf("expected last_used_at=\"\" (zero value when absent), got %q", p.LastUsedAt)
	}
	if p.LastErrorAt != "" {
		t.Fatalf("expected last_error_at=\"\" (zero value when absent), got %q", p.LastErrorAt)
	}
	if p.Uptime != 0 {
		t.Fatalf("expected uptime=0 (zero value when absent), got %f", p.Uptime)
	}
	if p.AvgLatencyMs != 0 {
		t.Fatalf("expected avg_latency_ms=0 (zero value when absent), got %d", p.AvgLatencyMs)
	}

	// Same checks for the second provider to ensure consistency.
	p2 := resp.Providers[1]
	if p2.Name != schemas.Anthropic {
		t.Fatalf("expected name=anthropic, got %q", p2.Name)
	}
	if p2.ProviderStatus != ProviderStatusActive {
		t.Fatalf("expected provider_status=active, got %q", p2.ProviderStatus)
	}
	if p2.Status != "list_models_failed" {
		t.Fatalf("expected status=list_models_failed, got %q", p2.Status)
	}
	if p2.KeysCount != 0 {
		t.Fatalf("expected keys_count=0 (zero value when absent), got %d", p2.KeysCount)
	}
	if p2.AvgLatencyMs != 0 {
		t.Fatalf("expected avg_latency_ms=0 (zero value when absent), got %d", p2.AvgLatencyMs)
	}
}

// TestComputeKeysEnabled verifies computeKeysEnabled for all key states:
// all enabled, some disabled, all disabled, empty, and nil Enabled.
func TestComputeKeysEnabled(t *testing.T) {
	tests := []struct {
		name string
		keys []schemas.Key
		want bool
	}{
		{
			name: "all keys enabled",
			keys: []schemas.Key{
				{ID: "k1", Enabled: ptr(true)},
				{ID: "k2", Enabled: ptr(true)},
			},
			want: true,
		},
		{
			name: "some keys disabled",
			keys: []schemas.Key{
				{ID: "k1", Enabled: ptr(true)},
				{ID: "k2", Enabled: ptr(false)},
			},
			want: true,
		},
		{
			name: "all keys disabled",
			keys: []schemas.Key{
				{ID: "k1", Enabled: ptr(false)},
				{ID: "k2", Enabled: ptr(false)},
			},
			want: false,
		},
		{
			name: "empty keys",
			keys: []schemas.Key{},
			want: false,
		},
		{
			name: "nil Enabled (default enabled)",
			keys: []schemas.Key{
				{ID: "k1", Enabled: nil},
				{ID: "k2", Enabled: nil},
			},
			want: true,
		},
		{
			name: "mixed nil and disabled",
			keys: []schemas.Key{
				{ID: "k1", Enabled: nil},
				{ID: "k2", Enabled: ptr(false)},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeKeysEnabled(tt.keys)
			if got != tt.want {
				t.Fatalf("computeKeysEnabled(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestListProvidersAggregatedFields_DisabledKeys verifies that keys_enabled=false
// when all keys are disabled.
func TestListProvidersAggregatedFields_DisabledKeys(t *testing.T) {
	SetLogger(&mockLogger{})
	lib.SetLogger(&mockLogger{})

	provider := schemas.OpenAI
	keys := []schemas.Key{
		{ID: "key-1", Enabled: ptr(false)},
		{ID: "key-2", Enabled: ptr(false)},
		{ID: "key-3", Enabled: ptr(false)},
	}

	h := &ProviderHandler{
		inMemoryStore: &lib.Config{
			Providers: map[schemas.ModelProvider]configstore.ProviderConfig{
				provider: {
					Keys: keys,
				},
			},
		},
		modelsManager: &mockModelsManager{},
		logStats:      &mockProviderLogStats{},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/providers")

	h.listProviders(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp ListProvidersResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Providers) == 0 {
		t.Fatalf("expected at least 1 provider")
	}

	p := resp.Providers[0]
	if p.KeysEnabled {
		t.Fatalf("expected keys_enabled=false when all keys are disabled, got true")
	}
}

// ptr returns a pointer to the given value. Useful for *bool literals in test setup.
func ptr[T any](v T) *T { return &v }

// mockProviderLogStats implements ProviderLogStats with fixed values, letting the
// handler tests exercise the aggregated fields without a real logs store.
type mockProviderLogStats struct {
	todayRequests int
	todayErrors   int
	lastUsedAt    string
	lastErrorAt   string
	avgLatencyMs  int
	err           error
	// When non-nil, tracks which method was called (for test assertions).
	singleCallCount *int
	batchCallCount  *int
}

func (m *mockProviderLogStats) AggregateProviderLogStats(_ context.Context, _ schemas.ModelProvider) (int, int, string, string, int, error) {
	if m.singleCallCount != nil {
		*m.singleCallCount++
	}
	if m.err != nil {
		return 0, 0, "", "", 0, m.err
	}
	return m.todayRequests, m.todayErrors, m.lastUsedAt, m.lastErrorAt, m.avgLatencyMs, nil
}

func (m *mockProviderLogStats) AggregateAllProviderLogStats(_ context.Context, providerNames []schemas.ModelProvider) (map[schemas.ModelProvider]LogAgg, error) {
	if m.batchCallCount != nil {
		*m.batchCallCount++
	}
	if m.err != nil {
		return nil, m.err
	}
	out := make(map[schemas.ModelProvider]LogAgg, len(providerNames))
	for _, p := range providerNames {
		out[p] = LogAgg{
			TodayRequests: m.todayRequests,
			TodayErrors:   m.todayErrors,
			LastUsedAt:    m.lastUsedAt,
			LastErrorAt:   m.lastErrorAt,
			AvgLatencyMs:  m.avgLatencyMs,
		}
	}
	return out, nil
}
