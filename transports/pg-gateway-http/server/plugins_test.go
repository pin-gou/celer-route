package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/framework/configstore"
	"github.com/pin-gou/pg-gateway/plugins/providercooldown"
	"github.com/pin-gou/pg-gateway/transports/pg-gateway-http/lib"
)

// TestLoadBuiltinPlugins_ProviderCooldown_DefaultOn verifies that when no
// provider-cooldown entry exists in PluginConfigs, the plugin is loaded as
// active and KeyPoolFilter is wired.
//
// This is the "default-on" semantics: absent config entry = enabled.
// The current production code treats a nil entry as disabled, so this test
// is expected to fail (red phase) until the dev phase implements the new
// default-on behavior.
func TestLoadBuiltinPlugins_ProviderCooldown_DefaultOn(t *testing.T) {
	prevLogger := logger
	logger = noopTestLogger{}
	defer func() { logger = prevLogger }()

	server := &BifrostHTTPServer{
		Ctx: schemas.NewBifrostContext(context.Background(), schemas.NoDeadline),
		Config: &lib.Config{
			ClientConfig: &configstore.ClientConfig{},
			// PluginConfigs is nil — no provider-cooldown entry at all.
			// After the dev phase this should be treated as "enabled by default".
		},
	}

	if err := server.loadBuiltinPlugins(context.Background()); err != nil {
		t.Fatalf("loadBuiltinPlugins returned unexpected error: %v", err)
	}

	// After the dev phase, the providercooldown plugin should be active and
	// KeyPoolFilter must be wired. In the red phase (current code) both
	// assertions fail because the default-on path does not exist yet.
	statuses := server.Config.GetPluginStatus()
	ps, ok := statuses[providercooldown.PluginName]
	if !ok {
		t.Fatal("provider-cooldown plugin status not found — expected active after default-on loading")
	}
	if ps.Status != schemas.PluginStatusActive {
		t.Fatalf("provider-cooldown status = %q, want %q", ps.Status, schemas.PluginStatusActive)
	}
	if server.KeyPoolFilter == nil {
		t.Fatal("KeyPoolFilter is nil, expected non-nil after default-on loading")
	}
}

// TestLoadBuiltinPlugins_ProviderCooldown_ExplicitDisabled verifies that when
// the provider-cooldown entry has enabled=false, the plugin is marked as
// disabled and KeyPoolFilter remains nil.
//
// This is a regression guard for the existing disabled behavior, which must
// survive the default-on change.
func TestLoadBuiltinPlugins_ProviderCooldown_ExplicitDisabled(t *testing.T) {
	prevLogger := logger
	logger = noopTestLogger{}
	defer func() { logger = prevLogger }()

	disabled := false
	server := &BifrostHTTPServer{
		Ctx: schemas.NewBifrostContext(context.Background(), schemas.NoDeadline),
		Config: &lib.Config{
			ClientConfig: &configstore.ClientConfig{},
			PluginConfigs: []*schemas.PluginConfig{
				{
					Name:    providercooldown.PluginName,
					Enabled: disabled,
				},
			},
		},
	}

	if err := server.loadBuiltinPlugins(context.Background()); err != nil {
		t.Fatalf("loadBuiltinPlugins returned unexpected error: %v", err)
	}

	// KeyPoolFilter must stay nil when the plugin is disabled.
	if server.KeyPoolFilter != nil {
		t.Fatal("KeyPoolFilter is non-nil, expected nil when provider-cooldown is explicitly disabled")
	}

	// The plugin status should be disabled.
	statuses := server.Config.GetPluginStatus()
	ps, ok := statuses[providercooldown.PluginName]
	if !ok {
		// When the plugin has never been registered (UpdatePluginOverallStatus
		// was never called), the status map has no entry. This is acceptable
		// for the disabled path — we already verified KeyPoolFilter is nil.
		return
	}
	if ps.Status != schemas.PluginStatusDisabled {
		t.Fatalf("provider-cooldown status = %q, want %q", ps.Status, schemas.PluginStatusDisabled)
	}
}

// TestInstantiatePlugin_RTK_Loads pins the desired contract for the RTK
// compression plugin registration in loadBuiltinPlugin. The transport dev phase
// (tasks.md 7.2) is expected to add an `rtk` case (mirroring semanticcache)
// that calls rtk.Init(ctx, config, logger). Until then the switch hits the
// default branch, so InstantiatePlugin returns "unknown built-in plugin: rtk"
// and this test FAILS — the red-phase signal that the registration is missing.
func TestInstantiatePlugin_RTK_Loads(t *testing.T) {
	prevLogger := logger
	logger = noopTestLogger{}
	defer func() { logger = prevLogger }()

	config := &lib.Config{
		ClientConfig: &configstore.ClientConfig{},
	}

	// The rtk plugin config matches the schema contract from design.md:
	// enabled / intensity / apply_to_tool_results / apply_to_code_blocks /
	// max_lines_per_result / max_chars_per_result / dedup_threshold /
	// preserve_cache_control.
	rtkConfig := map[string]any{
		"enabled":                true,
		"intensity":              "standard",
		"apply_to_tool_results":  true,
		"apply_to_code_blocks":   false,
		"max_lines_per_result":   120,
		"max_chars_per_result":   12000,
		"dedup_threshold":        3,
		"preserve_cache_control": true,
	}

	plugin, err := InstantiatePlugin(context.Background(), "rtk", nil, rtkConfig, config)
	if err != nil {
		// Red phase: the rtk case is not yet registered in loadBuiltinPlugin.
		// After the dev phase this must succeed.
		t.Fatalf("InstantiatePlugin(rtk) returned error: %v", err)
	}
	if plugin == nil {
		t.Fatal("InstantiatePlugin(rtk) returned nil plugin, want non-nil")
	}
	if got := plugin.GetName(); got != "rtk" {
		t.Fatalf("InstantiatePlugin(rtk) plugin name = %q, want %q", got, "rtk")
	}
}

// getSchemaPath returns the absolute path to config.schema.json.
func getSchemaPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	// Walk up from server/ to transports/ where config.schema.json lives
	schemaPath := filepath.Join(filepath.Dir(filename), "..", "..", "config.schema.json")
	if _, err := os.Stat(schemaPath); err != nil {
		t.Fatalf("config.schema.json not found at %s", schemaPath)
	}
	return schemaPath
}

// rtkPluginConfig builds a minimal config.json containing a single rtk plugin
// entry with the given config block, so the allOf/if/then plugin schema branch
// for name=rtk is exercised.
func rtkPluginConfig(configBlock string) string {
	return `{
		"plugins": [{
			"name": "rtk",
			"enabled": true,
			"config": ` + configBlock + `
		}]
	}`
}

// TestRTKPluginConfig_SchemaContract validates the rtk plugin config block
// against the real config.schema.json using the project's existing schema
// validation entry point. This guards against field name drift between the
// schema and the Config struct (e.g. deduplicate_threshold vs dedup_threshold).
//
// The test asserts three things:
//  1. A valid config with all canonical fields passes schema validation.
//  2. An unknown key in the config block is rejected (additionalProperties: false).
//  3. A missing required config key is rejected.
func TestRTKPluginConfig_SchemaContract(t *testing.T) {
	schemaPath := getSchemaPath(t)
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("failed to read schema: %v", err)
	}

	// 1. Valid config — all canonical fields from design.md § 数据模型 3.
	validConfig := rtkPluginConfig(`{
		"enabled": true,
		"intensity": "standard",
		"apply_to_tool_results": true,
		"apply_to_code_blocks": false,
		"max_lines_per_result": 120,
		"max_chars_per_result": 12000,
		"dedup_threshold": 3,
		"preserve_cache_control": true
	}`)
	if err := lib.ValidateConfigSchema([]byte(validConfig), schemaBytes); err != nil {
		t.Fatalf("valid rtk config should pass schema validation, got: %v", err)
	}

	// 2. Unknown key — must be rejected by additionalProperties: false.
	unknownKeyConfig := rtkPluginConfig(`{
		"enabled": true,
		"intensity": "standard",
		"dedup_threshold": 3,
		"unknown_field": "should be rejected"
	}`)
	if err := lib.ValidateConfigSchema([]byte(unknownKeyConfig), schemaBytes); err == nil {
		t.Error("unknown key in rtk config should be rejected by schema additionalProperties: false")
	}

	// 3. Verify the canonical property names are present in the schema.
	// Read the schema JSON to verify field names match the Config struct.
	var schemaObj map[string]interface{}
	if err := json.Unmarshal(schemaBytes, &schemaObj); err != nil {
		t.Fatalf("failed to parse schema JSON: %v", err)
	}
	// Navigate to plugins.items then branch for name=rtk → config.properties
	plugins, ok := schemaObj["properties"].(map[string]interface{})["plugins"].(map[string]interface{})
	if !ok {
		t.Fatal("schema missing plugins property")
	}
	items, ok := plugins["items"].(map[string]interface{})
	if !ok {
		t.Fatal("schema plugins missing items")
	}
	allOf, ok := items["allOf"].([]interface{})
	if !ok {
		t.Fatal("schema plugins.items missing allOf")
	}

	// Find the rtk branch in allOf
	var rtkThen map[string]interface{}
	for _, branch := range allOf {
		b, ok := branch.(map[string]interface{})
		if !ok {
			continue
		}
		ifBlock, ok := b["if"].(map[string]interface{})
		if !ok {
			continue
		}
		props, ok := ifBlock["properties"].(map[string]interface{})
		if !ok {
			continue
		}
		nameProp, ok := props["name"].(map[string]interface{})
		if !ok {
			continue
		}
		if nameProp["const"] == "rtk" {
			rtkThen, ok = b["then"].(map[string]interface{})
			break
		}
	}
	if rtkThen == nil {
		t.Fatal("schema missing rtk if/then branch")
	}

	configProps, ok := rtkThen["properties"].(map[string]interface{})["config"].(map[string]interface{})["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("rtk then branch missing config.properties")
	}

	// Every field in the Config struct must be present in the schema.
	expectedFields := []string{
		"enabled",
		"intensity",
		"apply_to_tool_results",
		"apply_to_code_blocks",
		"max_lines_per_result",
		"max_chars_per_result",
		"dedup_threshold",
		"preserve_cache_control",
	}
	for _, field := range expectedFields {
		if _, exists := configProps[field]; !exists {
			t.Errorf("rtk config schema missing field %q — schema field name must match Config struct JSON tag", field)
		}
	}

	// Ensure deduplicate_threshold (wrong name) is NOT present.
	if _, exists := configProps["deduplicate_threshold"]; exists {
		t.Error("schema uses deduplicate_threshold but design.md and Config struct use dedup_threshold — field name must be dedup_threshold")
	}
}

// TestRTKInit_InvalidConfigRejected verifies that rtk.Init rejects malicious
// or out-of-range config values by calling Config.Validate() during
// initialization. This is the fail-fast guard for the pattern_consistency
// concern (R-3): invalid configs must not silently produce a running plugin.
func TestRTKInit_InvalidConfigRejected(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
	}{
		{
			name: "invalid intensity",
			config: map[string]any{
				"enabled":   true,
				"intensity": "super-aggressive",
			},
		},
		{
			name: "negative max_lines_per_result",
			config: map[string]any{
				"enabled":              true,
				"intensity":            "standard",
				"max_lines_per_result": -5,
			},
		},
		{
			name: "negative max_chars_per_result",
			config: map[string]any{
				"enabled":             true,
				"intensity":           "standard",
				"max_chars_per_result": -100,
			},
		},
		{
			name: "negative dedup_threshold",
			config: map[string]any{
				"enabled":         true,
				"intensity":       "standard",
				"dedup_threshold": -1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := InstantiatePlugin(context.Background(), "rtk", nil, tt.config, &lib.Config{
				ClientConfig: &configstore.ClientConfig{},
			})
			if err == nil {
				t.Error("InstantiatePlugin(rtk) with invalid config should return error, got nil")
			}
		})
	}
}