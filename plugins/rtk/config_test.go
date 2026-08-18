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
	plugin, err := Init(nil, validConfig, nil)
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
	_, err = Init(nil, invalidConfig, nil)
	if err == nil {
		t.Fatal("Init() with invalid intensity should return error, got nil")
	}

	// Nil config should fail
	_, err = Init(nil, nil, nil)
	if err == nil {
		t.Fatal("Init() with nil config should return error, got nil")
	}
}