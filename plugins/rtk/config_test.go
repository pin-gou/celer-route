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
				Enabled:           true,
				Intensity:         "standard",
				MaxLinesPerResult: 120,
				MaxCharsPerResult: 12000,
				DedupThreshold:    3,
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
				Enabled:           true,
				Intensity:         "standard",
				MaxCharsPerResult: -100,
			},
			wantErr: true,
		},
		{
			name: "negative dedup_threshold",
			config: Config{
				Enabled:        true,
				Intensity:      "standard",
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
}

// TestConfigGroupingExplicitValuesPreserved verifies that explicitly-set
// grouping fields are NOT overwritten by applyConfigDefaults.
func TestConfigGroupingExplicitValuesPreserved(t *testing.T) {
	cfg := &Config{
		EnableGrouping:    true,
		GroupingThreshold: 5,
	}
	applyConfigDefaults(cfg)

	if !cfg.EnableGrouping {
		t.Error("applyConfigDefaults() should preserve explicit EnableGrouping=true")
	}
	if cfg.GroupingThreshold != 5 {
		t.Errorf("applyConfigDefaults() GroupingThreshold = %d, want 5 (explicit)", cfg.GroupingThreshold)
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
		Enabled:           true,
		EnableGrouping:    true,
		Intensity:         "bogus-intensity",
		GroupingThreshold: 3,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Config.Validate() should reject invalid intensity even with grouping fields set")
	}

	valid := &Config{
		Enabled:          true,
		EnableGrouping:   true,
		GroupingThreshold: 3,
		Intensity:        "standard",
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
		Enabled:             true,
		Intensity:           "standard",
		EnabledFilters:      []string{"git-status", "npm-install"},
		DisabledFilters:     []string{"npm-install"},
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

// ============================================================================
// Task 6.3: Config new field defaults zero-value safety (red phase)
//
// TDD red phase: Config.Pipeline and Config.MinTokensToCompress do not exist
// yet. All tests referencing them will fail at compile time with "undefined"
// errors.
//
// After dev, applyConfigDefaults must:
//   - Pipeline empty/nil → auto-fill to []PipelineStep{{ID: "rtk"}}
//   - Pipeline non-empty → preserve as-is (no override)
//   - MinTokensToCompress=0 → not skipped (no default override, stays 0)
// ============================================================================

// TestConfigPipelineDefaults verifies that when Pipeline is nil or empty,
// applyConfigDefaults fills it with the default PipelineStep containing
// id="rtk". After dev, this ensures the compression pipeline runs even
// when the config omits the Pipeline field entirely.
func TestConfigPipelineDefaults(t *testing.T) {
	t.Run("nil_pipeline_gets_default", func(t *testing.T) {
		cfg := &Config{Enabled: true}
		applyConfigDefaults(cfg)

		if cfg.Pipeline == nil {
			t.Fatal("applyConfigDefaults() should set Pipeline to non-nil default when nil")
		}
		if len(cfg.Pipeline) != 1 {
			t.Fatalf("applyConfigDefaults() Pipeline len = %d, want 1", len(cfg.Pipeline))
		}
		if cfg.Pipeline[0].ID != "rtk" {
			t.Errorf("applyConfigDefaults() Pipeline[0].ID = %q, want %q", cfg.Pipeline[0].ID, "rtk")
		}
	})

	t.Run("empty_pipeline_gets_default", func(t *testing.T) {
		cfg := &Config{Enabled: true, Pipeline: []PipelineStep{}}
		applyConfigDefaults(cfg)

		if cfg.Pipeline == nil {
			t.Fatal("applyConfigDefaults() should set Pipeline to non-nil default when empty")
		}
		if len(cfg.Pipeline) != 1 {
			t.Fatalf("applyConfigDefaults() Pipeline len = %d, want 1", len(cfg.Pipeline))
		}
		if cfg.Pipeline[0].ID != "rtk" {
			t.Errorf("applyConfigDefaults() Pipeline[0].ID = %q, want %q", cfg.Pipeline[0].ID, "rtk")
		}
	})

	t.Run("existing_pipeline_preserved", func(t *testing.T) {
		cfg := &Config{
			Enabled:  true,
			Pipeline: []PipelineStep{{ID: "custom-engine"}},
		}
		applyConfigDefaults(cfg)

		if len(cfg.Pipeline) != 1 {
			t.Fatalf("applyConfigDefaults() Pipeline len = %d, want 1 (preserved)", len(cfg.Pipeline))
		}
		if cfg.Pipeline[0].ID != "custom-engine" {
			t.Errorf("applyConfigDefaults() Pipeline[0].ID = %q, want %q (preserved)", cfg.Pipeline[0].ID, "custom-engine")
		}
	})

	t.Run("multi_step_pipeline_preserved", func(t *testing.T) {
		cfg := &Config{
			Enabled: true,
			Pipeline: []PipelineStep{
				{ID: "engine-a"},
				{ID: "engine-b"},
				{ID: "engine-c"},
			},
		}
		applyConfigDefaults(cfg)

		if len(cfg.Pipeline) != 3 {
			t.Fatalf("applyConfigDefaults() Pipeline len = %d, want 3 (preserved)", len(cfg.Pipeline))
		}
		if cfg.Pipeline[0].ID != "engine-a" {
			t.Errorf("Pipeline[0].ID = %q, want %q", cfg.Pipeline[0].ID, "engine-a")
		}
		if cfg.Pipeline[1].ID != "engine-b" {
			t.Errorf("Pipeline[1].ID = %q, want %q", cfg.Pipeline[1].ID, "engine-b")
		}
		if cfg.Pipeline[2].ID != "engine-c" {
			t.Errorf("Pipeline[2].ID = %q, want %q", cfg.Pipeline[2].ID, "engine-c")
		}
	})
}

// TestConfigMinTokensToCompressDefault verifies that MinTokensToCompress=0
// (zero value) is kept as-is by applyConfigDefaults — it must NOT be
// overwritten to any positive value. After dev, zero means "no minimum
// threshold, always compress", which is the backward-compatible default.
func TestConfigMinTokensToCompressDefault(t *testing.T) {
	cfg := &Config{Enabled: true}
	applyConfigDefaults(cfg)

	if cfg.MinTokensToCompress != 0 {
		t.Errorf("applyConfigDefaults() MinTokensToCompress = %d, want 0 (zero value preserved)", cfg.MinTokensToCompress)
	}
}

// TestConfigMinTokensToCompressExplicitPreserved verifies that an explicitly
// set MinTokensToCompress value is preserved by applyConfigDefaults (not
// overwritten to 0).
func TestConfigMinTokensToCompressExplicitPreserved(t *testing.T) {
	cfg := &Config{
		Enabled:             true,
		MinTokensToCompress: 500,
	}
	applyConfigDefaults(cfg)

	if cfg.MinTokensToCompress != 500 {
		t.Errorf("applyConfigDefaults() MinTokensToCompress = %d, want 500 (explicit value preserved)", cfg.MinTokensToCompress)
	}
}

// TestConfigPipelineAndMinTokensValidate verifies that the new Pipeline and
// MinTokensToCompress fields do not break Config.Validate(): a valid config
// with the new fields must pass, and an invalid config (bogus intensity) with
// the new fields must still be rejected (fail-fast not weakened).
func TestConfigPipelineAndMinTokensValidate(t *testing.T) {
	t.Run("valid_config_with_new_fields_passes", func(t *testing.T) {
		cfg := &Config{
			Enabled:             true,
			Intensity:           "standard",
			Pipeline:            []PipelineStep{{ID: "rtk"}},
			MinTokensToCompress: 100,
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Config.Validate() should accept valid config with Pipeline and MinTokensToCompress, got: %v", err)
		}
	})

	t.Run("invalid_intensity_still_rejected", func(t *testing.T) {
		cfg := &Config{
			Enabled:             true,
			Intensity:           "bogus-intensity",
			Pipeline:            []PipelineStep{{ID: "rtk"}},
			MinTokensToCompress: 100,
		}
		if err := cfg.Validate(); err == nil {
			t.Error("Config.Validate() should still reject invalid intensity with new fields set")
		}
	})

	t.Run("negative_min_tokens_rejected", func(t *testing.T) {
		cfg := &Config{
			Enabled:             true,
			Intensity:           "standard",
			MinTokensToCompress: -1,
		}
		if err := cfg.Validate(); err == nil {
			t.Error("Config.Validate() should reject negative MinTokensToCompress")
		}
	})
}



// TestLooksLikeAllZero exercises the heuristic that drives the all-zero
// safeguard in applyConfigDefaults. The predicate is intentionally tight:
// it must fire on a fully-default Config (the post-mortem signature) and
// must NOT fire the moment the operator expresses any intent via a
// tunable — otherwise it would override an explicit "enabled: false" that
// happens to ship with all-default tuning values.
func TestLooksLikeAllZero(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{name: "literal zero value", cfg: Config{}, want: true},
		{name: "all booleans false, all ints zero, all strings empty", cfg: Config{
			Enabled: false, EnableGrouping: false,
			CustomFiltersEnabled: false,
			TrustProjectFilters: false, EnableRenderers: false,
		}, want: true},
		{name: "operator set Intensity", cfg: Config{Intensity: "aggressive"}, want: false},
		{name: "operator set MaxLinesPerResult", cfg: Config{MaxLinesPerResult: 50}, want: false},
		{name: "operator set MinTokensToCompress", cfg: Config{MinTokensToCompress: 1024}, want: false},
		{name: "operator enabled EnableGrouping", cfg: Config{EnableGrouping: true}, want: false},
		{name: "operator enabled EnableRenderers", cfg: Config{EnableRenderers: true}, want: false},
		{name: "operator enabled CustomFiltersEnabled", cfg: Config{CustomFiltersEnabled: true}, want: false},
		{name: "operator enabled TrustProjectFilters", cfg: Config{TrustProjectFilters: true}, want: false},
		{name: "operator set RawOutputRetention", cfg: Config{RawOutputRetention: "always"}, want: false},
		{name: "operator set RawOutputMaxBytes", cfg: Config{RawOutputMaxBytes: 4096}, want: false},
		{name: "operator set GroupingThreshold", cfg: Config{GroupingThreshold: 5}, want: false},
		{name: "operator set DedupThreshold", cfg: Config{DedupThreshold: 5}, want: false},
		{name: "operator set MaxCharsPerResult", cfg: Config{MaxCharsPerResult: 5000}, want: false},
		{name: "operator set Pipeline", cfg: Config{Pipeline: []PipelineStep{{ID: "rtk"}}}, want: false}, // Pipeline is handled separately by applyConfigDefaults
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeAllZero(&tc.cfg); got != tc.want {
				t.Errorf("looksLikeAllZero(%+v) = %v, want %v", tc.cfg, got, tc.want)
			}
		})
	}
}

// TestApplyConfigDefaults_ZeroConfigEnables verifies the post-mortem fix:
// a config_json that round-trips to all-zero values (e.g. null stored in
// the config_plugins row) must default to Enabled=true, not stay false.
// Otherwise PreLLMHook short-circuits and the plugin becomes a silent
// no-op on every request — which is exactly the production incident that
// motivated this guard.
func TestApplyConfigDefaults_ZeroConfigEnables(t *testing.T) {
	t.Run("literal zero config flips Enabled to true", func(t *testing.T) {
		cfg := Config{}
		applyConfigDefaults(&cfg)
		if !cfg.Enabled {
			t.Fatalf("applyConfigDefaults on zero-value Config left Enabled=false; want true (zero-detect safeguard)")
		}
	})
	t.Run("explicit Enabled=false on zero config still flips to true", func(t *testing.T) {
		// This mirrors the production incident: storage serialised to an
		// all-zero map, the operator never had a chance to express intent,
		// but the JSON round-trip produced Enabled=false. We must promote
		// it back to true to recover the plugin.
		cfg := Config{Enabled: false}
		applyConfigDefaults(&cfg)
		if !cfg.Enabled {
			t.Fatalf("applyConfigDefaults on zero-value Config{Enabled:false} left Enabled=false; want true")
		}
	})
	t.Run("explicit Enabled=false with any operator-tunable is preserved", func(t *testing.T) {
		// If the operator set ANY other field, they touched the config
		// deliberately. Honoring their explicit Enabled=false is the
		// principle of least surprise — the guard exists to recover
		// never-saved configs, not to second-guess operators.
		cfg := Config{Enabled: false, Intensity: "aggressive"}
		applyConfigDefaults(&cfg)
		if cfg.Enabled {
			t.Fatalf("applyConfigDefaults overrode explicit Enabled=false when Intensity is set; want false (operator intent)")
		}
		if cfg.Intensity != "aggressive" {
			t.Fatalf("applyConfigDefaults clobbered explicit Intensity=%q", cfg.Intensity)
		}
	})
	t.Run("explicit Enabled=true on zero config stays true", func(t *testing.T) {
		cfg := Config{Enabled: true}
		applyConfigDefaults(&cfg)
		if !cfg.Enabled {
			t.Fatalf("applyConfigDefaults flipped an explicit Enabled=true to false")
		}
	})
	t.Run("config_json simulation via JSON round-trip enables", func(t *testing.T) {
		// End-to-end simulation of the bug: storage held null, the handler
		// does MarshalPluginConfig(nil) which gives a zero Config, then
		// Init calls applyConfigDefaults. The result must be Enabled=true.
		cfg := Config{}
		applyConfigDefaults(&cfg)
		if !cfg.Enabled {
			t.Fatalf("post-round-trip config left Enabled=false")
		}
		// And applyConfigDefaults should still fill the other sensible defaults.
		if cfg.Intensity != "standard" {
			t.Errorf("Intensity = %q, want standard", cfg.Intensity)
		}
		if cfg.MaxLinesPerResult != 120 {
			t.Errorf("MaxLinesPerResult = %d, want 120", cfg.MaxLinesPerResult)
		}
	})
}
