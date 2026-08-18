package rtk

import (
	"testing"
)

// TestConfigValidate verifies Config.Validate() rejects invalid values and
// accepts valid ones. This is the fail-fast guard for the pattern_consistency
// concern (R-3): invalid configs must not silently produce a running plugin.
func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid standard config",
			config: Config{
				Enabled:              true,
				Intensity:            "standard",
				MaxLinesPerResult:    120,
				MaxCharsPerResult:    12000,
				DedupThreshold:       3,
				PreserveCacheControl: true,
			},
			wantErr: false,
		},
		{
			name: "valid minimal config",
			config: Config{
				Enabled:   true,
				Intensity: "minimal",
			},
			wantErr: false,
		},
		{
			name: "valid aggressive config",
			config: Config{
				Enabled:   true,
				Intensity: "aggressive",
			},
			wantErr: false,
		},
		{
			name: "empty intensity is valid (zero value, optional)",
			config: Config{
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "invalid intensity",
			config: Config{
				Enabled:   true,
				Intensity: "super-aggressive",
			},
			wantErr: true,
		},
		{
			name: "negative max_lines_per_result",
			config: Config{
				Enabled:           true,
				Intensity:         "standard",
				MaxLinesPerResult: -5,
			},
			wantErr: true,
		},
		{
			name: "negative max_chars_per_result",
			config: Config{
				Enabled:          true,
				Intensity:        "standard",
				MaxCharsPerResult: -100,
			},
			wantErr: true,
		},
		{
			name: "negative dedup_threshold",
			config: Config{
				Enabled:       true,
				Intensity:     "standard",
				DedupThreshold: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Config.Validate() expected error for %q, got nil", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Config.Validate() unexpected error for %q: %v", tt.name, err)
			}
		})
	}
}

// TestInitValidatesConfig verifies that Init() calls Validate() and propagates
// the error for invalid configs, including nil config.
func TestInitValidatesConfig(t *testing.T) {
	// Valid config should succeed
	validConfig := &Config{
		Enabled:   true,
		Intensity: "standard",
	}
	plugin, err := Init(nil, validConfig, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Init() with valid config should succeed, got: %v", err)
	}
	if plugin == nil {
		t.Fatal("Init() returned nil plugin")
	}

	// Invalid config should fail
	invalidConfig := &Config{
		Enabled:   true,
		Intensity: "bogus-intensity",
	}
	_, err = Init(nil, invalidConfig, nil, "")
	if err == nil {
		t.Fatal("Init() with invalid intensity should return error, got nil")
	}

	// Nil config should fail
	_, err = Init(nil, nil, nil, "")
	if err == nil {
		t.Fatal("Init() with nil config should return error, got nil")
	}
}

// TestConfigGroupingDefaults verifies the phase-2 grouping fields get their
// documented zero-value defaults from applyConfigDefaults:
//
//	enable_grouping          → false
//	grouping_threshold       → 3
//	apply_to_assistant_messages → false
//
// and that only truly-zero values are defaulted (explicit true values are
// preserved). TDD red phase: the fields do not exist yet (compile error
// expected).
func TestConfigGroupingDefaults(t *testing.T) {
	cfg := &Config{}
	applyConfigDefaults(cfg)

	if cfg.EnableGrouping {
		t.Errorf("applyConfigDefaults() EnableGrouping = true, want false (default off)")
	}
	if cfg.GroupingThreshold != 3 {
		t.Errorf("applyConfigDefaults() GroupingThreshold = %d, want 3 (default)", cfg.GroupingThreshold)
	}
	if cfg.ApplyToAssistantMessages {
		t.Errorf("applyConfigDefaults() ApplyToAssistantMessages = true, want false (default off)")
	}
}

// TestConfigGroupingExplicitValuesPreserved verifies that explicitly-set
// grouping fields are NOT overwritten by applyConfigDefaults.
func TestConfigGroupingExplicitValuesPreserved(t *testing.T) {
	cfg := &Config{
		EnableGrouping:          true,
		GroupingThreshold:       5,
		ApplyToAssistantMessages: true,
	}
	applyConfigDefaults(cfg)

	if !cfg.EnableGrouping {
		t.Error("applyConfigDefaults() should preserve explicit EnableGrouping=true")
	}
	if cfg.GroupingThreshold != 5 {
		t.Errorf("applyConfigDefaults() GroupingThreshold = %d, want 5 (explicit)", cfg.GroupingThreshold)
	}
	if !cfg.ApplyToAssistantMessages {
		t.Error("applyConfigDefaults() should preserve explicit ApplyToAssistantMessages=true")
	}
}

// TestConfigGroupingThresholdClamp verifies that grouping_threshold values
// below 2 are clamped to 2 at runtime (aligning with OmniRoute
// Math.max(2, floor(...)) semantics), while 0 still falls back to the
// default of 3. Values 1 / negative must not cause behaviour drift.
func TestConfigGroupingThresholdClamp(t *testing.T) {
	tests := []struct {
		name      string
		input     int
		wantAfter int
	}{
		{name: "zero_value_gets_default_3", input: 0, wantAfter: 3},
		{name: "one_clamped_to_2", input: 1, wantAfter: 2},
		{name: "negative_clamped_to_2", input: -5, wantAfter: 2},
		{name: "valid_two_kept", input: 2, wantAfter: 2},
		{name: "valid_above_kept", input: 8, wantAfter: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{GroupingThreshold: tt.input}
			applyConfigDefaults(cfg)
			if cfg.GroupingThreshold != tt.wantAfter {
				t.Errorf("applyConfigDefaults() GroupingThreshold = %d, want %d",
					cfg.GroupingThreshold, tt.wantAfter)
			}
		})
	}
}

// TestConfigGroupingFieldsValidate verifies the grouping fields participate in
// Config.Validate(): a config carrying the new fields with an invalid
// intensity is still rejected (fail-fast). TDD red phase: the fields do not
// exist yet (compile error expected).
func TestConfigGroupingFieldsValidates(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		EnableGrouping:  true,
		Intensity:       "bogus-intensity",
		GroupingThreshold: 3,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Config.Validate() should reject invalid intensity even with grouping fields set")
	}

	valid := &Config{
		Enabled:                 true,
		EnableGrouping:          true,
		GroupingThreshold:       3,
		ApplyToAssistantMessages: true,
		Intensity:               "standard",
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("Config.Validate() unexpected error for valid grouping config: %v", err)
	}
}
// ============================================================================
// Phase 3: Custom filter config fields (V-plugins-2/3)
// The 4 new fields (CustomFiltersEnabled / TrustProjectFilters /
// EnabledFilters / DisabledFilters) must validate — empty values are valid,
// non-empty values are valid, and the mutual-exclusion combo is accepted
// (validation only checks types, per design.md).
// ============================================================================

// TestConfigCustomFilterFieldsEmpty validates that empty/nil values for the
// 4 new fields pass Validate (all optional).
func TestConfigCustomFilterFieldsEmpty(t *testing.T) {
	cfg := &Config{
		Enabled:   true,
		Intensity: "standard",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Config.Validate() should accept empty custom-filter fields, got: %v", err)
	}
	if cfg.CustomFiltersEnabled {
		t.Error("zero-value CustomFiltersEnabled should be false before defaults are applied")
	}
	if cfg.TrustProjectFilters {
		t.Error("zero-value TrustProjectFilters should be false")
	}
	if len(cfg.EnabledFilters) != 0 {
		t.Errorf("zero-value EnabledFilters should be empty, got %v", cfg.EnabledFilters)
	}
	if len(cfg.DisabledFilters) != 0 {
		t.Errorf("zero-value DisabledFilters should be empty, got %v", cfg.DisabledFilters)
	}
}

// TestConfigCustomFilterFieldsNonEmpty verifies that non-empty values for the
// 4 new fields pass Validate and are preserved verbatim.
func TestConfigCustomFilterFieldsNonEmpty(t *testing.T) {
	cfg := &Config{
		Enabled:              true,
		Intensity:            "standard",
		CustomFiltersEnabled: true,
		TrustProjectFilters:  true,
		EnabledFilters:       []string{"git-status", "npm-install"},
		DisabledFilters:      []string{"generic-output"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Config.Validate() should accept non-empty custom-filter fields, got: %v", err)
	}
	if len(cfg.EnabledFilters) != 2 {
		t.Errorf("EnabledFilters = %v, want 2 entries preserved", cfg.EnabledFilters)
	}
	if len(cfg.DisabledFilters) != 1 {
		t.Errorf("DisabledFilters = %v, want 1 entry preserved", cfg.DisabledFilters)
	}
}

// TestConfigCustomFilterMutualExclusion verifies that setting both
// EnabledFilters and DisabledFilters simultaneously is accepted by Validate
// (the loader applies whitelist first then blacklist). The config validator
// does not reject the combination — only type-checks the fields.
func TestConfigCustomFilterMutualExclusion(t *testing.T) {
	cfg := &Config{
		Enabled:            true,
		Intensity:          "standard",
		EnabledFilters:     []string{"git-status", "npm-install"},
		DisabledFilters:    []string{"npm-install"},
		TrustProjectFilters: true,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Config.Validate() should accept EnabledFilters + DisabledFilters together, got: %v", err)
	}
}

// TestConfigCustomFiltersInvalidIntensityStillRejected verifies the new fields
// do not weaken fail-fast validation: an invalid intensity combined with the
// new fields is still rejected.
func TestConfigCustomFiltersInvalidIntensityStillRejected(t *testing.T) {
	cfg := &Config{
		Enabled:              true,
		Intensity:            "bogus",
		CustomFiltersEnabled: true,
		EnabledFilters:       []string{"git-status"},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Config.Validate() should still reject invalid intensity with custom-filter fields set")
	}
}

// TestConfigCustomFiltersDefaults verifies the documented defaults via
// applyConfigDefaults: CustomFiltersEnabled stays false at the Config level
// (the loader treats zero-value as enabled-by-design), TrustProjectFilters
// stays false, and the filter lists stay empty.
func TestConfigCustomFiltersDefaults(t *testing.T) {
	cfg := &Config{Enabled: true}
	applyConfigDefaults(cfg)

	if cfg.TrustProjectFilters {
		t.Error("applyConfigDefaults() should leave TrustProjectFilters false (default)")
	}
	if len(cfg.EnabledFilters) != 0 {
		t.Errorf("applyConfigDefaults() should leave EnabledFilters empty, got %v", cfg.EnabledFilters)
	}
	if len(cfg.DisabledFilters) != 0 {
		t.Errorf("applyConfigDefaults() should leave DisabledFilters empty, got %v", cfg.DisabledFilters)
	}
}
