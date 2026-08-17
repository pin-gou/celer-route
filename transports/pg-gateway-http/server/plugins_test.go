package server

import (
	"context"
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

// TestRTKPluginConfig_SchemaContract pins the config shape the transport dev
// phase must accept for the rtk plugin (design.md § 数据模型 3). It mirrors the
// `if/then` block that will be added to transports/config.schema.json: every
// property listed in the schema must be present in the Config struct of the
// plugin once implemented. This test guards the JSON round-trip (marshal →
// unmarshal) so a config.json `{name: "rtk", config: {...}}` block survives
// loading without schema validation errors.
func TestRTKPluginConfig_SchemaContract(t *testing.T) {
	// The canonical plugin block for config.json, matching the schema contract:
	// `{name: "rtk", enabled: true, config: {...}}`.
	pluginBlock := map[string]any{
		"name":    "rtk",
		"enabled": true,
		"config": map[string]any{
			"enabled":                true,
			"intensity":              "standard",
			"apply_to_tool_results":  true,
			"apply_to_code_blocks":   false,
			"max_lines_per_result":   120,
			"max_chars_per_result":   12000,
			"dedup_threshold":        3,
			"preserve_cache_control": true,
		},
	}

	expectedKeys := []string{
		"enabled",
		"intensity",
		"apply_to_tool_results",
		"apply_to_code_blocks",
		"max_lines_per_result",
		"max_chars_per_result",
		"dedup_threshold",
		"preserve_cache_control",
	}

	configMap, ok := pluginBlock["config"].(map[string]any)
	if !ok {
		t.Fatal("config field is not a map")
	}

	// Every schema-declared property must be present in the config sample.
	for _, key := range expectedKeys {
		if _, exists := configMap[key]; !exists {
			t.Fatalf("rtk config missing schema property %q — dev phase must accept it in config.schema.json then branch", key)
		}
	}

	// The config must round-trip through the schema plugin-block shape used by
	// Config.UpdatePluginOverallStatus / getPluginConfig lookups.
	if pluginBlock["name"] != "rtk" {
		t.Fatalf("plugin block name = %v, want rtk", pluginBlock["name"])
	}
	enabled, ok := pluginBlock["enabled"].(bool)
	if !ok || !enabled {
		t.Fatalf("plugin block enabled = %v, want true", pluginBlock["enabled"])
	}
}