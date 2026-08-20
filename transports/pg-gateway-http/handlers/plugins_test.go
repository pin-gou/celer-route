package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/framework/configstore"
	configstoreTables "github.com/pin-gou/pg-gateway/framework/configstore/tables"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

// capturePluginsStore records the last config passed to UpdatePlugin so tests
// can assert that config merging occurred correctly.
type capturePluginsStore struct {
	configstore.ConfigStore
	existingPlugin  *configstoreTables.TablePlugin
	capturedConfig  map[string]any
	capturedEnabled bool
}

func (s *capturePluginsStore) GetPlugin(_ context.Context, name string) (*configstoreTables.TablePlugin, error) {
	if s.existingPlugin != nil && s.existingPlugin.Name == name {
		return s.existingPlugin, nil
	}
	return nil, configstore.ErrNotFound
}

func (s *capturePluginsStore) UpdatePlugin(_ context.Context, plugin *configstoreTables.TablePlugin, _ ...*gorm.DB) error {
	if cfg, ok := plugin.Config.(map[string]any); ok {
		s.capturedConfig = cfg
	}
	s.capturedEnabled = plugin.Enabled
	if s.existingPlugin != nil && s.existingPlugin.Name == plugin.Name {
		s.existingPlugin.Config = plugin.Config
		s.existingPlugin.Enabled = plugin.Enabled
	}
	return nil
}

func (s *capturePluginsStore) CreatePlugin(_ context.Context, plugin *configstoreTables.TablePlugin, _ ...*gorm.DB) error {
	s.existingPlugin = plugin
	return nil
}

// noopPluginsLoader satisfies the PluginsLoader interface without doing anything.
type noopPluginsLoader struct{}

func (noopPluginsLoader) ReloadPlugin(_ context.Context, _ string, _ *string, _ any, _ *schemas.PluginPlacement, _ *int) error {
	return nil
}
func (noopPluginsLoader) RemovePlugin(_ context.Context, _ string) error { return nil }
func (noopPluginsLoader) GetPluginStatus(_ context.Context) map[string]schemas.PluginStatus {
	return nil
}
func (noopPluginsLoader) GetLoadedPluginNames() []string { return nil }
func (noopPluginsLoader) NormalizePluginConfig(_ string, _ map[string]any) (map[string]any, error) {
	return nil, nil
}

func (noopPluginsLoader) ExpandPluginConfigForAPI(_ string, _ map[string]any) (map[string]any, error) {
	return nil, nil
}

// buildUpdateRequest creates a PUT /api/plugins/{name} fasthttp context.
func buildUpdateRequest(t *testing.T, body any) *fasthttp.RequestCtx {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("PUT")
	ctx.Request.SetBody(raw)
	ctx.SetUserValue("name", "otel")
	return ctx
}

// buildCreateRequest creates a POST /api/plugins fasthttp context.
func buildCreateRequest(t *testing.T, body any) *fasthttp.RequestCtx {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBody(raw)
	return ctx
}

// TestCreatePlugin_RejectsCustomPathWhenAuthBypassed verifies that POST /api/plugins
// refuses to create a custom (non-builtin) plugin with a Path - the .so that would get
// dlopen()'d - when the request was let through by the auth middleware's fail-open
// bypass (dashboard auth disabled/unconfigured), and that no DB write happens.
func TestCreatePlugin_RejectsCustomPathWhenAuthBypassed(t *testing.T) {
	SetLogger(&mockLogger{})

	store := &capturePluginsStore{}
	h := &PluginsHandler{
		pluginsLoader: noopPluginsLoader{},
		configStore:   store,
	}

	path := "/tmp/evil.so"
	ctx := buildCreateRequest(t, map[string]any{
		"name":    "my-custom-test-plugin",
		"enabled": true,
		"path":    path,
	})
	ctx.SetUserValue(schemas.BifrostContextKeyAuthBypassed, true)

	h.createPlugin(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	if store.existingPlugin != nil {
		t.Error("CreatePlugin should not have been called on the config store")
	}
}

// TestCreatePlugin_AllowsCustomPathWhenNotBypassed verifies the same request succeeds
// for a genuinely authenticated caller (BifrostContextKeyAuthBypassed not set).
func TestCreatePlugin_AllowsCustomPathWhenNotBypassed(t *testing.T) {
	SetLogger(&mockLogger{})

	store := &capturePluginsStore{}
	h := &PluginsHandler{
		pluginsLoader: noopPluginsLoader{},
		configStore:   store,
	}

	path := "/opt/bifrost/plugins/trusted.so"
	ctx := buildCreateRequest(t, map[string]any{
		"name":    "my-custom-test-plugin",
		"enabled": false,
		"path":    path,
	})

	h.createPlugin(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	if store.existingPlugin == nil {
		t.Fatal("CreatePlugin was not called on the config store")
	}
	if store.existingPlugin.Path == nil || *store.existingPlugin.Path != path {
		t.Errorf("stored plugin path = %v, want %v", store.existingPlugin.Path, path)
	}
}

// TestUpdatePlugin_RejectsCustomPathWhenAuthBypassed verifies the matching guard on
// PUT /api/plugins/{name}.
func TestUpdatePlugin_RejectsCustomPathWhenAuthBypassed(t *testing.T) {
	SetLogger(&mockLogger{})

	store := &capturePluginsStore{
		existingPlugin: &configstoreTables.TablePlugin{
			Name:     "my-custom-test-plugin",
			Enabled:  false,
			IsCustom: true,
		},
	}
	h := &PluginsHandler{
		pluginsLoader: noopPluginsLoader{},
		configStore:   store,
	}

	ctx := buildUpdateRequest(t, map[string]any{
		"enabled": true,
		"path":    "/tmp/evil.so",
	})
	ctx.SetUserValue("name", "my-custom-test-plugin")
	ctx.SetUserValue(schemas.BifrostContextKeyAuthBypassed, true)

	h.updatePlugin(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	if store.capturedConfig != nil || store.capturedEnabled {
		t.Error("UpdatePlugin should not have been called on the config store")
	}
}

// TestUpdatePlugin_ConfigMerge verifies that updatePlugin merges the incoming
// config over the existing DB config, preserving fields the caller did not send.
// This is critical for the plugin_span_filter field: the OTEL config form in the
// UI does not send plugin_span_filter, so it must survive a save without being wiped.
// TestRestoreRedacted_OTELProfilesHeaders covers the two gaps that broke OTEL header
// round-trips after the multi-profile change: (1) headers live inside the `profiles`
// array (slice traversal), and (2) header values are plain redacted strings, not EnvVar
// objects. Saving a config whose headers came back redacted must not overwrite the
// stored credentials.
func TestRestoreRedacted_OTELProfilesHeaders(t *testing.T) {
	realAuth := "Basic-REAL-SUPER-SECRET-VALUE"
	realVersion := "4"
	maskedAuth := schemas.NewSecretVar(realAuth).Redacted().GetValue()       // long -> first4 + **** + last4
	maskedVersion := schemas.NewSecretVar(realVersion).Redacted().GetValue() // "4" -> "*"

	mkConfig := func(auth, version string) map[string]any {
		return map[string]any{
			"profiles": []any{
				map[string]any{
					"service_name": "langfuse",
					"headers": map[string]any{
						"Authorization":                auth,
						"x-langfuse-ingestion-version": version,
					},
				},
			},
		}
	}

	existing := mkConfig(realAuth, realVersion)
	incoming := mkConfig(maskedAuth, maskedVersion) // what the UI sends back after a redacted GET

	got := restoreRedactedFromExisting(incoming, existing)
	headers := got["profiles"].([]any)[0].(map[string]any)["headers"].(map[string]any)

	if headers["Authorization"] != realAuth {
		t.Errorf("Authorization not restored: got %q, want %q", headers["Authorization"], realAuth)
	}
	if headers["x-langfuse-ingestion-version"] != realVersion {
		t.Errorf("version not restored: got %q, want %q", headers["x-langfuse-ingestion-version"], realVersion)
	}

	// A genuinely changed (non-redacted) header value must pass through untouched.
	changed := mkConfig("Basic-A-BRAND-NEW-KEY-VALUE-1234", "3")
	got2 := restoreRedactedFromExisting(changed, existing)
	headers2 := got2["profiles"].([]any)[0].(map[string]any)["headers"].(map[string]any)
	if headers2["Authorization"] != "Basic-A-BRAND-NEW-KEY-VALUE-1234" {
		t.Errorf("new Authorization should pass through, got %q", headers2["Authorization"])
	}
	if headers2["x-langfuse-ingestion-version"] != "3" {
		t.Errorf("new version should pass through, got %q", headers2["x-langfuse-ingestion-version"])
	}

	// An intentional env.* reference (e.g. credential rotation) must pass through.
	// NewSecretVar parses the "env." prefix as FromEnv=true, which IsRedacted reports as
	// redacted; the IsFromEnv guard must let it through rather than restoring the stored value.
	rotated := mkConfig("env.NEW_TOKEN", "env.NEW_VERSION")
	got3 := restoreRedactedFromExisting(rotated, existing)
	headers3 := got3["profiles"].([]any)[0].(map[string]any)["headers"].(map[string]any)
	if headers3["Authorization"] != "env.NEW_TOKEN" {
		t.Errorf("env.* Authorization should pass through, got %q", headers3["Authorization"])
	}
	if headers3["x-langfuse-ingestion-version"] != "env.NEW_VERSION" {
		t.Errorf("env.* version should pass through, got %q", headers3["x-langfuse-ingestion-version"])
	}
}

// TestRestoreRedacted_KafkaSecretVarObjects covers the Kafka connector shape: secrets are
// stored in the DB as plain strings, but the redacted GET returns plain-text SecretVars as
// value-only objects ({"value": "supe…cret"} — ref/type are omitempty). Saving that back
// must restore the stored string, not persist the mask.
func TestRestoreRedacted_KafkaSecretVarObjects(t *testing.T) {
	realPassword := "REAL-KAFKA-SASL-PASSWORD-123"
	realCACert := "-----BEGIN CERTIFICATE-----\nREAL\n-----END CERTIFICATE-----"
	maskedPassword := schemas.NewSecretVar(realPassword).Redacted().GetValue()
	maskedCACert := schemas.NewSecretVar(realCACert).Redacted().GetValue()

	// What MarshalForStorage stored in the DB.
	existing := map[string]any{
		"brokers": []any{"localhost:9092"},
		"ca_cert": realCACert,
		"sasl": map[string]any{
			"mechanism": "PLAIN",
			"username":  "kafka-user",
			"password":  realPassword,
		},
	}
	// What the UI sends back after a redacted GET, untouched.
	incoming := map[string]any{
		"brokers": []any{"localhost:9092"},
		"ca_cert": map[string]any{"value": maskedCACert},
		"sasl": map[string]any{
			"mechanism": "PLAIN",
			"username":  map[string]any{"value": "kafka-user"},
			"password":  map[string]any{"value": maskedPassword},
		},
	}

	got := restoreRedactedFromExisting(incoming, existing)
	if got["ca_cert"] != realCACert {
		t.Errorf("ca_cert not restored: got %v, want %q", got["ca_cert"], realCACert)
	}
	sasl := got["sasl"].(map[string]any)
	if sasl["password"] != realPassword {
		t.Errorf("sasl.password not restored: got %v, want %q", sasl["password"], realPassword)
	}
	// A non-redacted value-only object (username shown in clear) must pass through.
	if u, ok := sasl["username"].(map[string]any); !ok || u["value"] != "kafka-user" {
		t.Errorf("sasl.username should pass through unchanged, got %v", sasl["username"])
	}

	// A genuinely rotated password ({"value": ..., "ref": ""} as SecretVarInput emits)
	// must pass through, not be clobbered by the stored value.
	rotated := map[string]any{
		"sasl": map[string]any{
			"password": map[string]any{"value": "A-BRAND-NEW-PASSWORD-5678", "ref": ""},
		},
	}
	got2 := restoreRedactedFromExisting(rotated, existing)
	p, ok := got2["sasl"].(map[string]any)["password"].(map[string]any)
	if !ok || p["value"] != "A-BRAND-NEW-PASSWORD-5678" {
		t.Errorf("new password should pass through, got %v", got2["sasl"].(map[string]any)["password"])
	}

	// Switching to an env reference must pass through (intentional update).
	envRef := map[string]any{
		"sasl": map[string]any{
			"password": map[string]any{"value": "", "ref": "env.KAFKA_PASSWORD", "type": "env"},
		},
	}
	got3 := restoreRedactedFromExisting(envRef, existing)
	p3, ok := got3["sasl"].(map[string]any)["password"].(map[string]any)
	if !ok || p3["ref"] != "env.KAFKA_PASSWORD" {
		t.Errorf("env ref password should pass through, got %v", got3["sasl"].(map[string]any)["password"])
	}
}

// TestRestoreRedacted_FullyRedactedSentinel covers the telemetry (Prometheus push gateway)
// password shape: FullyRedacted() returns the fixed "<REDACTED>" sentinel instead of the
// prefix/suffix mask, and the stored value is a plain string.
func TestRestoreRedacted_FullyRedactedSentinel(t *testing.T) {
	realPassword := "REAL-PUSHGATEWAY-PASSWORD"
	sentinel := schemas.NewSecretVar(realPassword).FullyRedacted().GetValue()

	existing := map[string]any{
		"push_gateway": map[string]any{
			"basic_auth": map[string]any{"username": "pgw-user", "password": realPassword},
		},
	}
	incoming := map[string]any{
		"push_gateway": map[string]any{
			"basic_auth": map[string]any{
				"username": map[string]any{"value": "pgw-user"},
				"password": map[string]any{"value": sentinel},
			},
		},
	}

	got := restoreRedactedFromExisting(incoming, existing)
	ba := got["push_gateway"].(map[string]any)["basic_auth"].(map[string]any)
	if ba["password"] != realPassword {
		t.Errorf("sentinel password not restored: got %v, want %q", ba["password"], realPassword)
	}
}

func TestUpdatePlugin_ConfigMerge(t *testing.T) {
	SetLogger(&mockLogger{})

	spanFilter := map[string]any{
		"mode":    "exclude",
		"plugins": []any{"logging", "compat"},
	}
	existingConfig := map[string]any{
		"collector_url":      "localhost:4317",
		"trace_type":         "genai_extension",
		"protocol":           "grpc",
		"plugin_span_filter": spanFilter,
	}

	store := &capturePluginsStore{
		existingPlugin: &configstoreTables.TablePlugin{
			Name:    "otel",
			Enabled: true,
			Config:  existingConfig,
		},
	}

	h := &PluginsHandler{
		pluginsLoader: noopPluginsLoader{},
		configStore:   store,
	}

	// The UI OTEL form sends only the base fields — no plugin_span_filter.
	reqBody := map[string]any{
		"enabled": true,
		"config": map[string]any{
			"collector_url": "new-collector:4317",
			"trace_type":    "open_inference",
			"protocol":      "grpc",
		},
	}

	ctx := buildUpdateRequest(t, reqBody)
	h.updatePlugin(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	// The merged config must contain both the updated base fields AND the preserved filter.
	if store.capturedConfig == nil {
		t.Fatal("UpdatePlugin was not called")
	}
	if got := store.capturedConfig["collector_url"]; got != "new-collector:4317" {
		t.Errorf("collector_url = %v, want new-collector:4317", got)
	}
	if got := store.capturedConfig["trace_type"]; got != "open_inference" {
		t.Errorf("trace_type = %v, want open_inference", got)
	}
	if _, ok := store.capturedConfig["plugin_span_filter"]; !ok {
		t.Error("plugin_span_filter was wiped from the config; merge logic is broken")
	}
}

// TestUpdatePlugin_ConfigMerge_NewPlugin verifies that when no existing plugin
// is found in the DB (first save), the incoming config is used as-is.
func TestUpdatePlugin_ConfigMerge_NewPlugin(t *testing.T) {
	SetLogger(&mockLogger{})

	store := &capturePluginsStore{existingPlugin: nil}
	h := &PluginsHandler{
		pluginsLoader: noopPluginsLoader{},
		configStore:   store,
	}

	reqBody := map[string]any{
		"enabled": true,
		"config": map[string]any{
			"collector_url": "localhost:4317",
			"trace_type":    "genai_extension",
			"protocol":      "grpc",
		},
	}

	ctx := buildUpdateRequest(t, reqBody)
	h.updatePlugin(ctx)

	// Should succeed even when no existing plugin is found (creates then updates).
	if ctx.Response.StatusCode() != 200 {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
}

// namedPluginsLoader is a noopPluginsLoader that returns a fixed set of loaded
// plugin names, used to assert the getLoadedPlugins response contract.
type namedPluginsLoader struct {
	noopPluginsLoader
	names []string
}

func (l namedPluginsLoader) GetLoadedPluginNames() []string { return l.names }

// TestUpdatePlugin_RTKConfigPassThrough verifies that the PATCH (PUT) endpoint
// passes through the new pipeline and min_tokens_to_compress fields in the RTK
// plugin config. The transport layer uses map[string]any for config, so these
// fields must survive the update cycle without being stripped.
func TestUpdatePlugin_RTKConfigPassThrough(t *testing.T) {
	SetLogger(&mockLogger{})

	existingConfig := map[string]any{
		"enabled":                     true,
		"intensity":                   "standard",
		"max_lines_per_result":        120,
		"max_chars_per_result":        12000,
		"dedup_threshold":             3,
		"apply_to_tool_results":       true,
		"apply_to_code_blocks":        false,
		"apply_to_assistant_messages": false,
		"raw_output_retention":        "never",
		"raw_output_max_bytes":        1048576,
	}

	store := &capturePluginsStore{
		existingPlugin: &configstoreTables.TablePlugin{
			Name:    "rtk",
			Enabled: true,
			Config:  existingConfig,
		},
	}

	h := &PluginsHandler{
		pluginsLoader: noopPluginsLoader{},
		configStore:   store,
	}

	// PATCH /api/plugins/rtk with new pipeline and min_tokens_to_compress fields.
	reqBody := map[string]any{
		"enabled": true,
		"config": map[string]any{
			"enabled":                     true,
			"intensity":                   "standard",
			"max_lines_per_result":        120,
			"max_chars_per_result":        12000,
			"dedup_threshold":             3,
			"apply_to_tool_results":       true,
			"apply_to_code_blocks":        false,
			"apply_to_assistant_messages": false,
			"raw_output_retention":        "never",
			"raw_output_max_bytes":        1048576,
			"pipeline": []any{
				map[string]any{
					"id": "rtk",
				},
			},
			"min_tokens_to_compress": 500,
		},
	}

	ctx := buildUpdateRequest(t, reqBody)
	ctx.SetUserValue("name", "rtk")
	h.updatePlugin(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	if store.capturedConfig == nil {
		t.Fatal("UpdatePlugin was not called on the config store")
	}

	// Verify the pipeline field was passed through.
	pipeline, ok := store.capturedConfig["pipeline"]
	if !ok {
		t.Fatal("pipeline field was stripped from the config; pass-through is broken")
	}
	pipelineSlice, ok := pipeline.([]any)
	if !ok {
		t.Fatalf("pipeline is %T, want []any", pipeline)
	}
	if len(pipelineSlice) != 1 {
		t.Fatalf("pipeline has %d entries, want 1", len(pipelineSlice))
	}
	firstEntry, ok := pipelineSlice[0].(map[string]any)
	if !ok {
		t.Fatalf("pipeline[0] is %T, want map[string]any", pipelineSlice[0])
	}
	if firstEntry["id"] != "rtk" {
		t.Errorf("pipeline[0].id = %v, want rtk", firstEntry["id"])
	}

	// Verify the min_tokens_to_compress field was passed through.
	if got, ok := store.capturedConfig["min_tokens_to_compress"]; !ok {
		t.Fatal("min_tokens_to_compress field was stripped from the config; pass-through is broken")
	} else if got != float64(500) {
		t.Errorf("min_tokens_to_compress = %v (type %T), want 500", got, got)
	}

	// Verify the response body also contains the new fields.
	var response struct {
		Plugin struct {
			Config map[string]any `json:"config"`
		} `json:"plugin"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := response.Plugin.Config["pipeline"]; !ok {
		t.Error("pipeline field missing from response body")
	}
	if _, ok := response.Plugin.Config["min_tokens_to_compress"]; !ok {
		t.Error("min_tokens_to_compress field missing from response body")
	}
}

// TestGetLoadedPlugins verifies that getLoadedPlugins returns the loader's plugin
// names under the "plugins" JSON key, locking the response shape the UI depends on.
func TestGetLoadedPlugins(t *testing.T) {
	want := []string{"logging", "telemetry"}
	h := &PluginsHandler{
		pluginsLoader: namedPluginsLoader{names: want},
		configStore:   nil,
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	h.getLoadedPlugins(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var response struct {
		Plugins []string `json:"plugins"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Plugins) != len(want) {
		t.Fatalf("expected %d plugins, got %d: %v", len(want), len(response.Plugins), response.Plugins)
	}
	for i, name := range want {
		if response.Plugins[i] != name {
			t.Errorf("plugins[%d] = %q, want %q", i, response.Plugins[i], name)
		}
	}
}

// statusPluginsLoader returns a fixed status map for getPlugins tests.
type statusPluginsLoader struct {
	noopPluginsLoader
	statuses map[string]schemas.PluginStatus
}

func (l statusPluginsLoader) GetPluginStatus(_ context.Context) map[string]schemas.PluginStatus {
	return l.statuses
}

// emptyPluginsStore is a config store whose GetPlugins returns an empty list.
type emptyPluginsStore struct {
	configstore.ConfigStore
}

func (s emptyPluginsStore) GetPlugins(_ context.Context) ([]*configstoreTables.TablePlugin, error) {
	return nil, nil
}

// TestGetPlugins_NoConfigStore_EnabledReflectsStatus verifies the no-configstore
// branch of getPlugins: every plugin in the status map is returned as a PluginResponse
// with Enabled derived from the status (not hardcoded true) and IsCustom computed
// from IsBuiltinPlugin.
func TestGetPlugins_NoConfigStore_EnabledReflectsStatus(t *testing.T) {
	SetLogger(&mockLogger{})

	statuses := map[string]schemas.PluginStatus{
		"rtk": {
			Name:   "rtk",
			Status: schemas.PluginStatusDisabled,
			Logs:   []string{},
		},
		"telemetry": {
			Name:   "telemetry",
			Status: schemas.PluginStatusActive,
			Logs:   []string{},
		},
		"my-custom-plugin": {
			Name:   "my-custom-plugin",
			Status: schemas.PluginStatusActive,
			Logs:   []string{},
		},
	}

	h := &PluginsHandler{
		pluginsLoader: statusPluginsLoader{statuses: statuses},
		configStore:   nil,
	}

	ctx := &fasthttp.RequestCtx{}
	h.getPlugins(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp struct {
		Plugins []PluginResponse `json:"plugins"`
		Count   int              `json:"count"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	lookup := make(map[string]PluginResponse, len(resp.Plugins))
	for _, p := range resp.Plugins {
		lookup[p.Name] = p
	}

	// rtk: disabled built-in → Enabled=false, IsCustom=false
	rtk, ok := lookup["rtk"]
	if !ok {
		t.Fatal("rtk not found in plugin list")
	}
	if rtk.Enabled {
		t.Errorf("rtk.Enabled = true, want false (status is disabled)")
	}
	if rtk.IsCustom {
		t.Errorf("rtk.IsCustom = true, want false (rtk is a built-in)")
	}

	// telemetry: active built-in → Enabled=true, IsCustom=false
	tel, ok := lookup["telemetry"]
	if !ok {
		t.Fatal("telemetry not found in plugin list")
	}
	if !tel.Enabled {
		t.Errorf("telemetry.Enabled = false, want true (status is active)")
	}
	if tel.IsCustom {
		t.Errorf("telemetry.IsCustom = true, want false (telemetry is a built-in)")
	}

	// my-custom-plugin: active custom → Enabled=true, IsCustom=true
	custom, ok := lookup["my-custom-plugin"]
	if !ok {
		t.Fatal("my-custom-plugin not found in plugin list")
	}
	if !custom.Enabled {
		t.Errorf("my-custom-plugin.Enabled = false, want true (status is active)")
	}
	if !custom.IsCustom {
		t.Errorf("my-custom-plugin.IsCustom = false, want true (custom plugin)")
	}
}

// TestGetPlugins_WithStore_DisabledBuiltinsMerged verifies the store-backed branch
// of getPlugins: a disabled built-in that has no config_plugins row is merged into
// the response with Enabled=false (derived from status, not hardcoded true).
func TestGetPlugins_WithStore_DisabledBuiltinsMerged(t *testing.T) {
	SetLogger(&mockLogger{})

	statuses := map[string]schemas.PluginStatus{
		"rtk": {
			Name:   "rtk",
			Status: schemas.PluginStatusDisabled,
			Logs:   []string{},
		},
		"telemetry": {
			Name:   "telemetry",
			Status: schemas.PluginStatusActive,
			Logs:   []string{},
		},
	}

	h := &PluginsHandler{
		pluginsLoader: statusPluginsLoader{statuses: statuses},
		configStore:   emptyPluginsStore{},
	}

	ctx := &fasthttp.RequestCtx{}
	h.getPlugins(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Fatalf("expected 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp struct {
		Plugins []PluginResponse `json:"plugins"`
		Count   int              `json:"count"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	lookup := make(map[string]PluginResponse, len(resp.Plugins))
	for _, p := range resp.Plugins {
		lookup[p.Name] = p
	}

	// rtk: disabled built-in no DB row → must be merged with Enabled=false
	rtk, ok := lookup["rtk"]
	if !ok {
		t.Fatal("rtk not found in plugin list — disabled built-in must be merged")
	}
	if rtk.Enabled {
		t.Errorf("rtk.Enabled = true, want false (status is disabled)")
	}
	if rtk.IsCustom {
		t.Errorf("rtk.IsCustom = true, want false (rtk is a built-in)")
	}

	// telemetry: active built-in → Enabled=true
	tel, ok := lookup["telemetry"]
	if !ok {
		t.Fatal("telemetry not found in plugin list")
	}
	if !tel.Enabled {
		t.Errorf("telemetry.Enabled = false, want true (status is active)")
	}
}

// TestUpdatePlugin_Governance_RejectsInvalidRoutingChainMaxDepth verifies that
// PUT /api/plugins/governance with routing_chain_max_depth > 100 returns 422
// config_invalid, matching the design contract and frontend zod schema.
func TestUpdatePlugin_Governance_RejectsInvalidRoutingChainMaxDepth(t *testing.T) {
	SetLogger(&mockLogger{})

	store := &capturePluginsStore{
		existingPlugin: &configstoreTables.TablePlugin{
			Name:    "governance",
			Enabled: true,
			Config:  map[string]any{},
		},
	}
	h := &PluginsHandler{
		pluginsLoader: noopPluginsLoader{},
		configStore:   store,
	}

	tests := []struct {
		name  string
		depth any
		code  int
	}{
		{"depth > 100", 999, 422},
		{"depth = 101", 101, 422},
		{"depth = 0 (below min)", 0, 422},
		{"depth = -1 (negative)", -1, 422},
		{"depth = 5.5 (non-integer)", 5.5, 422},
		{"depth = 100 (boundary, valid)", 100, 200},
		{"depth = 1 (boundary, valid)", 1, 200},
		{"depth = 50 (valid)", 50, 200},
		{"depth missing (optional)", nil, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]any{}
			if tt.depth != nil {
				config["routing_chain_max_depth"] = tt.depth
			}
			reqBody := map[string]any{
				"enabled": true,
				"config":  config,
			}
			ctx := buildUpdateRequest(t, reqBody)
			ctx.SetUserValue("name", "governance")

			h.updatePlugin(ctx)

			if ctx.Response.StatusCode() != tt.code {
				t.Errorf("expected status %d, got %d: %s", tt.code, ctx.Response.StatusCode(), ctx.Response.Body())
			}

			if tt.code == 422 {
				var resp struct {
					Error *schemas.ErrorField `json:"error"`
				}
				if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
					t.Fatalf("unmarshal error response: %v", err)
				}
				if resp.Error == nil {
					t.Fatal("expected error field in response body")
				}
				if resp.Error.Code == nil || *resp.Error.Code != "config_invalid" {
					t.Errorf("expected error.code = config_invalid, got %v", resp.Error.Code)
				}
			}
		})
	}
}
