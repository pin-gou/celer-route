package server

import (
	"context"
	"testing"

	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/framework/configstore"
	"github.com/pin-gou/pg-gateway/plugins/providercooldown"
	"github.com/pin-gou/pg-gateway/transports/bifrost-http/lib"
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