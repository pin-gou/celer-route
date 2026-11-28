package schemas

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCooldownCapacityConfigNormalized(t *testing.T) {
	if got := (*CooldownCapacityConfig)(nil).Normalized(); got != nil {
		t.Fatalf("nil config Normalized() = %+v, want nil", got)
	}

	if got := (&CooldownCapacityConfig{}).Normalized(); got.WindowSeconds != DefaultCapacityWindowSeconds ||
		got.Headroom != DefaultCapacityHeadroom ||
		got.Scope != CapacityScopeProvider ||
		got.Action != CapacityActionFallback {
		t.Fatalf("zero config normalized to %+v, want defaults", got)
	}

	tests := []struct {
		name string
		in   CooldownCapacityConfig
		want CooldownCapacityConfig
	}{
		{
			name: "negative window takes default",
			in:   CooldownCapacityConfig{WindowSeconds: -5},
			want: CooldownCapacityConfig{WindowSeconds: DefaultCapacityWindowSeconds, Headroom: DefaultCapacityHeadroom, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		},
		{
			name: "window below minimum is clamped",
			in:   CooldownCapacityConfig{WindowSeconds: 5},
			want: CooldownCapacityConfig{WindowSeconds: MinCapacityWindowSeconds, Headroom: DefaultCapacityHeadroom, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		},
		{
			name: "window above maximum is clamped",
			in:   CooldownCapacityConfig{WindowSeconds: 100000},
			want: CooldownCapacityConfig{WindowSeconds: MaxCapacityWindowSeconds, Headroom: DefaultCapacityHeadroom, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		},
		{
			name: "window inside bounds is kept",
			in:   CooldownCapacityConfig{WindowSeconds: 120},
			want: CooldownCapacityConfig{WindowSeconds: 120, Headroom: DefaultCapacityHeadroom, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		},
		{
			name: "negative headroom takes default",
			in:   CooldownCapacityConfig{Headroom: -0.5},
			want: CooldownCapacityConfig{WindowSeconds: DefaultCapacityWindowSeconds, Headroom: DefaultCapacityHeadroom, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		},
		{
			name: "headroom below minimum is clamped",
			in:   CooldownCapacityConfig{Headroom: 0.05},
			want: CooldownCapacityConfig{WindowSeconds: DefaultCapacityWindowSeconds, Headroom: MinCapacityHeadroom, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		},
		{
			name: "headroom above maximum is clamped",
			in:   CooldownCapacityConfig{Headroom: 2},
			want: CooldownCapacityConfig{WindowSeconds: DefaultCapacityWindowSeconds, Headroom: MaxCapacityHeadroom, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		},
		{
			name: "headroom inside bounds is kept",
			in:   CooldownCapacityConfig{Headroom: 0.5},
			want: CooldownCapacityConfig{WindowSeconds: DefaultCapacityWindowSeconds, Headroom: 0.5, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		},
		{
			name: "scope and action are lower-cased and trimmed",
			in:   CooldownCapacityConfig{Scope: "  PROVIDER_MODEL ", Action: " Reject "},
			want: CooldownCapacityConfig{WindowSeconds: DefaultCapacityWindowSeconds, Headroom: DefaultCapacityHeadroom, Scope: CapacityScopeProviderModel, Action: CapacityActionReject},
		},
		{
			name: "unknown scope and action fall back to defaults",
			in:   CooldownCapacityConfig{Scope: "bogus", Action: "bogus"},
			want: CooldownCapacityConfig{WindowSeconds: DefaultCapacityWindowSeconds, Headroom: DefaultCapacityHeadroom, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			got := in.Normalized()
			if got == nil {
				t.Fatal("Normalized() = nil")
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Fatalf("Normalized() = %+v, want %+v", *got, tt.want)
			}
			if !reflect.DeepEqual(in, tt.in) {
				t.Fatalf("Normalized() mutated the receiver: %+v", in)
			}
		})
	}
}

func TestCooldownCapacityConfigActive(t *testing.T) {
	if (*CooldownCapacityConfig)(nil).Active() {
		t.Fatal("nil capacity config must not be active")
	}
	if (&CooldownCapacityConfig{Enabled: true}).Active() {
		t.Fatal("enabled config without a limit must not be active")
	}
	if (&CooldownCapacityConfig{Enabled: false, TokenLimit: 100}).Active() {
		t.Fatal("disabled config must not be active")
	}
	if !(&CooldownCapacityConfig{Enabled: true, TokenLimit: 100}).Active() {
		t.Fatal("enabled config with a token limit must be active")
	}
	if !(&CooldownCapacityConfig{Enabled: true, RequestLimit: 10}).Active() {
		t.Fatal("enabled config with a request limit must be active")
	}
}

func TestCooldownCapacityConfigValidate(t *testing.T) {
	valid := []CooldownCapacityConfig{
		{},
		{Enabled: true, TokenLimit: 1000, RequestLimit: 10, WindowSeconds: 60, Headroom: 0.9, Scope: CapacityScopeProvider, Action: CapacityActionFallback},
		{WindowSeconds: MinCapacityWindowSeconds, Headroom: MinCapacityHeadroom},
		{WindowSeconds: MaxCapacityWindowSeconds, Headroom: MaxCapacityHeadroom},
		{Scope: "PROVIDER_MODEL", Action: "Reject"},
		{Scope: CapacityScopeKey},
		{Scope: "KEY"},
		{
			Enabled:    true,
			Scope:      CapacityScopeKey,
			KeyLimits:  map[string]CooldownCapacityKeyLimit{"small": {TokenLimit: ptrInt64(100)}, "wide": {RequestLimit: ptrInt64(0)}},
			TokenLimit: 1000,
		},
	}
	for i := range valid {
		if err := valid[i].Validate(); err != nil {
			t.Fatalf("valid config %+v rejected: %v", valid[i], err)
		}
	}
	// Every scope normalization accepts "key"; an unknown value keeps falling
	// back to "provider".
	if got := NormalizeCapacityScope(" Key "); got != CapacityScopeKey {
		t.Fatalf("NormalizeCapacityScope(%q) = %q, want %q", " Key ", got, CapacityScopeKey)
	}
	if got := NormalizeCapacityScope("tenant"); got != CapacityScopeProvider {
		t.Fatalf("NormalizeCapacityScope(%q) = %q, want the provider fallback", "tenant", got)
	}

	invalid := []struct {
		name string
		cfg  CooldownCapacityConfig
		want string
	}{
		{"window below minimum", CooldownCapacityConfig{WindowSeconds: MinCapacityWindowSeconds - 1}, "capacity.window_seconds"},
		{"window above maximum", CooldownCapacityConfig{WindowSeconds: MaxCapacityWindowSeconds + 1}, "capacity.window_seconds"},
		{"negative window", CooldownCapacityConfig{WindowSeconds: -1}, "capacity.window_seconds"},
		{"headroom below minimum", CooldownCapacityConfig{Headroom: MinCapacityHeadroom - 0.01}, "capacity.headroom"},
		{"headroom above maximum", CooldownCapacityConfig{Headroom: MaxCapacityHeadroom + 0.01}, "capacity.headroom"},
		{"negative headroom", CooldownCapacityConfig{Headroom: -0.1}, "capacity.headroom"},
		{"negative token limit", CooldownCapacityConfig{TokenLimit: -1}, "capacity.token_limit"},
		{"negative request limit", CooldownCapacityConfig{RequestLimit: -1}, "capacity.request_limit"},
		{"unknown scope", CooldownCapacityConfig{Scope: "tenant"}, "capacity.scope"},
		{"unknown action", CooldownCapacityConfig{Action: "warn"}, "capacity.action"},
		{
			"key_limits under the provider scope",
			CooldownCapacityConfig{Scope: CapacityScopeProvider, KeyLimits: map[string]CooldownCapacityKeyLimit{"k": {TokenLimit: ptrInt64(1)}}},
			"capacity.key_limits requires capacity.scope",
		},
		{
			"key_limits under the provider_model scope",
			CooldownCapacityConfig{Scope: CapacityScopeProviderModel, KeyLimits: map[string]CooldownCapacityKeyLimit{"k": {TokenLimit: ptrInt64(1)}}},
			"capacity.key_limits requires capacity.scope",
		},
		{
			"empty key name",
			CooldownCapacityConfig{Scope: CapacityScopeKey, KeyLimits: map[string]CooldownCapacityKeyLimit{"  ": {TokenLimit: ptrInt64(1)}}},
			"empty key name",
		},
		{
			"entry with no limit",
			CooldownCapacityConfig{Scope: CapacityScopeKey, KeyLimits: map[string]CooldownCapacityKeyLimit{"k": {}}},
			"must set token_limit or request_limit",
		},
		{
			"negative per-key token limit",
			CooldownCapacityConfig{Scope: CapacityScopeKey, KeyLimits: map[string]CooldownCapacityKeyLimit{"k": {TokenLimit: ptrInt64(-1)}}},
			"token_limit must not be negative",
		},
		{
			"negative per-key request limit",
			CooldownCapacityConfig{Scope: CapacityScopeKey, KeyLimits: map[string]CooldownCapacityKeyLimit{"k": {RequestLimit: ptrInt64(-1)}}},
			"request_limit must not be negative",
		},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatalf("config %+v must be rejected", tt.cfg)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}

	if err := (*CooldownCapacityConfig)(nil).Validate(); err != nil {
		t.Fatalf("nil config must validate, got %v", err)
	}
}

func TestCooldownEscalationConfigNormalized(t *testing.T) {
	if got := (*CooldownEscalationConfig)(nil).Normalized(); got != nil {
		t.Fatalf("nil config Normalized() = %+v, want nil", got)
	}

	tests := []struct {
		in   int
		want int
	}{
		{-3, DefaultEscalationMaxMultiplier},
		{0, DefaultEscalationMaxMultiplier},
		{1, 1},
		{4, 4},
		{MaxEscalationMultiplier, MaxEscalationMultiplier},
		{MaxEscalationMultiplier + 1, MaxEscalationMultiplier},
	}
	for _, tt := range tests {
		got := (&CooldownEscalationConfig{MaxMultiplier: tt.in}).Normalized()
		if got.MaxMultiplier != tt.want {
			t.Fatalf("Normalized(%d).MaxMultiplier = %d, want %d", tt.in, got.MaxMultiplier, tt.want)
		}
	}
}

func TestCooldownEscalationConfigActive(t *testing.T) {
	if (*CooldownEscalationConfig)(nil).Active() {
		t.Fatal("nil escalation config must not be active")
	}
	if (&CooldownEscalationConfig{}).Active() {
		t.Fatal("disabled escalation config must not be active")
	}
	if !(&CooldownEscalationConfig{Enabled: true}).Active() {
		t.Fatal("enabled escalation config must be active")
	}
}

func TestCooldownEscalationConfigValidate(t *testing.T) {
	for _, cfg := range []CooldownEscalationConfig{
		{},
		{MaxMultiplier: 0},
		{MaxMultiplier: 1},
		{MaxMultiplier: MaxEscalationMultiplier},
	} {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("valid config %+v rejected: %v", cfg, err)
		}
	}

	invalid := []struct {
		name string
		cfg  CooldownEscalationConfig
	}{
		{"negative multiplier", CooldownEscalationConfig{MaxMultiplier: -1}},
		{"multiplier above maximum", CooldownEscalationConfig{MaxMultiplier: MaxEscalationMultiplier + 1}},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatalf("config %+v must be rejected", tt.cfg)
			}
			if !strings.Contains(err.Error(), "escalation.max_multiplier") {
				t.Fatalf("error %q does not mention escalation.max_multiplier", err)
			}
		})
	}

	if err := (*CooldownEscalationConfig)(nil).Validate(); err != nil {
		t.Fatalf("nil config must validate, got %v", err)
	}

	// A chronic block is validated in place, and an enabled chronic block under
	// a disabled escalation section is rejected (it could never take effect).
	if err := (&CooldownEscalationConfig{
		Enabled: true,
		Chronic: &CooldownChronicConfig{Enabled: true},
	}).Validate(); err != nil {
		t.Fatalf("enabled chronic under enabled escalation rejected: %v", err)
	}
	if err := (&CooldownEscalationConfig{
		Enabled: false,
		Chronic: &CooldownChronicConfig{Enabled: true},
	}).Validate(); err == nil {
		t.Fatal("escalation.chronic.enabled with escalation.enabled=false must be rejected")
	}
	if err := (&CooldownEscalationConfig{
		Enabled: true,
		Chronic: &CooldownChronicConfig{Enabled: true, SuccessRateThreshold: 1.5},
	}).Validate(); err == nil {
		t.Fatal("an out-of-range chronic threshold must be rejected")
	}
}

func TestCooldownChronicConfigNormalized(t *testing.T) {
	if (*CooldownChronicConfig)(nil).Normalized() != nil {
		t.Fatal("nil chronic config Normalized() must be nil")
	}
	got := (&CooldownChronicConfig{Enabled: true}).Normalized()
	if got.SuccessRateThreshold != DefaultChronicSuccessRateThreshold ||
		got.MinSamples != DefaultChronicMinSamples ||
		got.ChronicMaxTTLSeconds != DefaultChronicMaxTTLSeconds ||
		got.RecoverySuccesses != DefaultChronicRecoverySuccesses {
		t.Fatalf("defaults = %+v, want %v/%d/%d/%d", got,
			DefaultChronicSuccessRateThreshold, DefaultChronicMinSamples,
			DefaultChronicMaxTTLSeconds, DefaultChronicRecoverySuccesses)
	}
	// jitter 0 means "no jitter" and must survive normalization; a negative
	// value takes the recommended default.
	if (&CooldownChronicConfig{}).Normalized().JitterRatio != 0 {
		t.Fatal("jitter_ratio=0 must be preserved as \"no jitter\"")
	}
	if got := (&CooldownChronicConfig{JitterRatio: -1}).Normalized().JitterRatio; got != DefaultChronicJitterRatio {
		t.Fatalf("negative jitter_ratio normalized to %v, want %v", got, DefaultChronicJitterRatio)
	}

	clamps := []struct {
		in   CooldownChronicConfig
		want CooldownChronicConfig
	}{
		{
			in:   CooldownChronicConfig{SuccessRateThreshold: 2, MinSamples: -5, ChronicMaxTTLSeconds: 1 << 20, JitterRatio: 9, RecoverySuccesses: -2},
			want: CooldownChronicConfig{SuccessRateThreshold: MaxChronicSuccessRateThreshold, MinSamples: DefaultChronicMinSamples, ChronicMaxTTLSeconds: MaxChronicMaxTTLSeconds, JitterRatio: MaxChronicJitterRatio, RecoverySuccesses: DefaultChronicRecoverySuccesses},
		},
		{
			in:   CooldownChronicConfig{SuccessRateThreshold: 0.001, ChronicMaxTTLSeconds: 1},
			want: CooldownChronicConfig{SuccessRateThreshold: MinChronicSuccessRateThreshold, MinSamples: DefaultChronicMinSamples, ChronicMaxTTLSeconds: MinChronicMaxTTLSeconds, RecoverySuccesses: DefaultChronicRecoverySuccesses},
		},
	}
	for _, tt := range clamps {
		got := tt.in.Normalized()
		if got.SuccessRateThreshold != tt.want.SuccessRateThreshold ||
			got.MinSamples != tt.want.MinSamples ||
			got.ChronicMaxTTLSeconds != tt.want.ChronicMaxTTLSeconds ||
			got.JitterRatio != tt.want.JitterRatio ||
			got.RecoverySuccesses != tt.want.RecoverySuccesses {
			t.Fatalf("Normalized(%+v) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestCooldownChronicConfigActiveAndValidate(t *testing.T) {
	if (*CooldownChronicConfig)(nil).Active() {
		t.Fatal("nil chronic config must not be active")
	}
	if (&CooldownChronicConfig{}).Active() {
		t.Fatal("disabled chronic config must not be active")
	}
	if !(&CooldownChronicConfig{Enabled: true}).Active() {
		t.Fatal("enabled chronic config must be active")
	}

	for _, cfg := range []CooldownChronicConfig{
		{},
		{SuccessRateThreshold: 0.5, MinSamples: 10, ChronicMaxTTLSeconds: 1800, JitterRatio: 0.2, RecoverySuccesses: 3},
		{JitterRatio: 0},
		{JitterRatio: 1},
	} {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("valid chronic config %+v rejected: %v", cfg, err)
		}
	}
	invalid := []struct {
		name string
		cfg  CooldownChronicConfig
		want string
	}{
		{"threshold must stay inside (0,1)", CooldownChronicConfig{SuccessRateThreshold: 1}, "escalation.chronic.success_rate_threshold"},
		{"negative threshold", CooldownChronicConfig{SuccessRateThreshold: -0.2}, "escalation.chronic.success_rate_threshold"},
		{"min_samples too small", CooldownChronicConfig{MinSamples: -1}, "escalation.chronic.min_samples"},
		{"ttl too large", CooldownChronicConfig{ChronicMaxTTLSeconds: MaxChronicMaxTTLSeconds + 1}, "escalation.chronic.chronic_max_ttl_seconds"},
		{"jitter above one", CooldownChronicConfig{JitterRatio: 1.5}, "escalation.chronic.jitter_ratio"},
		{"recovery too large", CooldownChronicConfig{RecoverySuccesses: MaxChronicRecoverySuccesses + 1}, "escalation.chronic.recovery_successes"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatalf("config %+v must be rejected", tt.cfg)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestCooldownEscalationNormalizedDeepCopiesChronic(t *testing.T) {
	chronic := &CooldownChronicConfig{Enabled: true, MinSamples: 7}
	cfg := &CooldownEscalationConfig{Enabled: true, MaxMultiplier: 4, Chronic: chronic}
	got := cfg.Normalized()
	if got.Chronic == chronic {
		t.Fatal("Normalized aliased the caller's chronic block; it must be deep-copied")
	}
	if chronic.MinSamples != 7 {
		t.Fatalf("the caller's chronic block was mutated: %+v", chronic)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("enabled chronic under enabled escalation rejected: %v", err)
	}
}

func TestCooldownLearningConfigNormalized(t *testing.T) {
	if got := (*CooldownLearningConfig)(nil).Normalized(); got != nil {
		t.Fatalf("nil config Normalized() = %+v, want nil", got)
	}

	got := (&CooldownLearningConfig{}).Normalized()
	if got.WindowSeconds != DefaultLearningWindowSeconds ||
		got.MaxSamples != DefaultLearningMaxSamples ||
		got.MinSamples != DefaultLearningMinSamples {
		t.Fatalf("zero config normalized to %+v, want defaults", got)
	}

	tests := []struct {
		name string
		in   CooldownLearningConfig
		want CooldownLearningConfig
	}{
		{
			name: "negative window takes default",
			in:   CooldownLearningConfig{WindowSeconds: -1},
			want: CooldownLearningConfig{WindowSeconds: DefaultLearningWindowSeconds, MaxSamples: DefaultLearningMaxSamples, MinSamples: DefaultLearningMinSamples},
		},
		{
			name: "window below minimum is clamped",
			in:   CooldownLearningConfig{WindowSeconds: MinLearningWindowSeconds - 1},
			want: CooldownLearningConfig{WindowSeconds: MinLearningWindowSeconds, MaxSamples: DefaultLearningMaxSamples, MinSamples: DefaultLearningMinSamples},
		},
		{
			name: "window above maximum is clamped",
			in:   CooldownLearningConfig{WindowSeconds: MaxLearningWindowSeconds + 1},
			want: CooldownLearningConfig{WindowSeconds: MaxLearningWindowSeconds, MaxSamples: DefaultLearningMaxSamples, MinSamples: DefaultLearningMinSamples},
		},
		{
			name: "max samples below minimum is clamped",
			in:   CooldownLearningConfig{MaxSamples: MinLearningMaxSamples - 1},
			want: CooldownLearningConfig{WindowSeconds: DefaultLearningWindowSeconds, MaxSamples: MinLearningMaxSamples, MinSamples: DefaultLearningMinSamples},
		},
		{
			name: "max samples above maximum is clamped",
			in:   CooldownLearningConfig{MaxSamples: MaxLearningMaxSamples + 1},
			want: CooldownLearningConfig{WindowSeconds: DefaultLearningWindowSeconds, MaxSamples: MaxLearningMaxSamples, MinSamples: DefaultLearningMinSamples},
		},
		{
			name: "min samples below minimum is clamped",
			in:   CooldownLearningConfig{MinSamples: -1},
			want: CooldownLearningConfig{WindowSeconds: DefaultLearningWindowSeconds, MaxSamples: DefaultLearningMaxSamples, MinSamples: DefaultLearningMinSamples},
		},
		{
			name: "min samples above maximum is clamped",
			in:   CooldownLearningConfig{MinSamples: MaxLearningMinSamples + 1},
			want: CooldownLearningConfig{WindowSeconds: DefaultLearningWindowSeconds, MaxSamples: DefaultLearningMaxSamples, MinSamples: DefaultLearningMaxSamples},
		},
		{
			name: "min samples above max samples is pulled down",
			in:   CooldownLearningConfig{MaxSamples: 20, MinSamples: 15},
			want: CooldownLearningConfig{WindowSeconds: DefaultLearningWindowSeconds, MaxSamples: 20, MinSamples: 15},
		},
		{
			name: "min samples above max samples cap is pulled down",
			in:   CooldownLearningConfig{MaxSamples: 10, MinSamples: 100},
			want: CooldownLearningConfig{WindowSeconds: DefaultLearningWindowSeconds, MaxSamples: 10, MinSamples: 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			got := in.Normalized()
			if got == nil {
				t.Fatal("Normalized() = nil")
			}
			if *got != tt.want {
				t.Fatalf("Normalized() = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestCooldownLearningConfigActive(t *testing.T) {
	if (*CooldownLearningConfig)(nil).Active() {
		t.Fatal("nil learning config must not be active")
	}
	if (&CooldownLearningConfig{}).Active() {
		t.Fatal("disabled learning config must not be active")
	}
	if !(&CooldownLearningConfig{Enabled: true}).Active() {
		t.Fatal("enabled learning config must be active")
	}
}

func TestCooldownLearningConfigValidate(t *testing.T) {
	for _, cfg := range []CooldownLearningConfig{
		{},
		{Enabled: true},
		{WindowSeconds: MinLearningWindowSeconds, MaxSamples: MinLearningMaxSamples, MinSamples: MinLearningMinSamples},
		{WindowSeconds: MaxLearningWindowSeconds, MaxSamples: MaxLearningMaxSamples, MinSamples: MaxLearningMinSamples},
		{MaxSamples: 10, MinSamples: 10},
		{MaxSamples: 100, MinSamples: 60},
	} {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("valid config %+v rejected: %v", cfg, err)
		}
	}

	invalid := []struct {
		name string
		cfg  CooldownLearningConfig
		want string
	}{
		{"window below minimum", CooldownLearningConfig{WindowSeconds: MinLearningWindowSeconds - 1}, "learning.window_seconds"},
		{"window above maximum", CooldownLearningConfig{WindowSeconds: MaxLearningWindowSeconds + 1}, "learning.window_seconds"},
		{"negative window", CooldownLearningConfig{WindowSeconds: -1}, "learning.window_seconds"},
		{"max samples below minimum", CooldownLearningConfig{MaxSamples: MinLearningMaxSamples - 1}, "learning.max_samples"},
		{"max samples above maximum", CooldownLearningConfig{MaxSamples: MaxLearningMaxSamples + 1}, "learning.max_samples"},
		{"min samples below minimum", CooldownLearningConfig{MinSamples: -1}, "learning.min_samples"},
		{"min samples above maximum", CooldownLearningConfig{MinSamples: MaxLearningMinSamples + 1}, "learning.min_samples"},
		{"min samples above max samples", CooldownLearningConfig{MaxSamples: 10, MinSamples: 20}, "learning.min_samples must be <= learning.max_samples"},
		{"min samples above defaulted max samples", CooldownLearningConfig{MinSamples: DefaultLearningMaxSamples + 1}, "learning.min_samples must be <= learning.max_samples"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatalf("config %+v must be rejected", tt.cfg)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}

	if err := (*CooldownLearningConfig)(nil).Validate(); err != nil {
		t.Fatalf("nil config must validate, got %v", err)
	}
}

func TestCooldownHealthConfigNormalized(t *testing.T) {
	if got := (*CooldownHealthConfig)(nil).Normalized(); got != nil {
		t.Fatalf("nil config Normalized() = %+v, want nil", got)
	}

	got := (&CooldownHealthConfig{}).Normalized()
	if got.HalfLifeSeconds != DefaultHealthHalfLifeSeconds ||
		got.MinSamples != DefaultHealthMinSamples ||
		got.MinWeightFactor != DefaultHealthMinWeightFactor {
		t.Fatalf("zero config normalized to %+v, want defaults", got)
	}

	tests := []struct {
		name string
		in   CooldownHealthConfig
		want CooldownHealthConfig
	}{
		{
			name: "negative half life takes default",
			in:   CooldownHealthConfig{HalfLifeSeconds: -1},
			want: CooldownHealthConfig{HalfLifeSeconds: DefaultHealthHalfLifeSeconds, MinSamples: DefaultHealthMinSamples, MinWeightFactor: DefaultHealthMinWeightFactor},
		},
		{
			name: "half life below minimum is clamped",
			in:   CooldownHealthConfig{HalfLifeSeconds: MinHealthHalfLifeSeconds - 1},
			want: CooldownHealthConfig{HalfLifeSeconds: MinHealthHalfLifeSeconds, MinSamples: DefaultHealthMinSamples, MinWeightFactor: DefaultHealthMinWeightFactor},
		},
		{
			name: "half life above maximum is clamped",
			in:   CooldownHealthConfig{HalfLifeSeconds: MaxHealthHalfLifeSeconds + 1},
			want: CooldownHealthConfig{HalfLifeSeconds: MaxHealthHalfLifeSeconds, MinSamples: DefaultHealthMinSamples, MinWeightFactor: DefaultHealthMinWeightFactor},
		},
		{
			name: "min samples below minimum is clamped",
			in:   CooldownHealthConfig{MinSamples: -1},
			want: CooldownHealthConfig{HalfLifeSeconds: DefaultHealthHalfLifeSeconds, MinSamples: DefaultHealthMinSamples, MinWeightFactor: DefaultHealthMinWeightFactor},
		},
		{
			name: "min samples above maximum is clamped",
			in:   CooldownHealthConfig{MinSamples: MaxHealthMinSamples + 1},
			want: CooldownHealthConfig{HalfLifeSeconds: DefaultHealthHalfLifeSeconds, MinSamples: MaxHealthMinSamples, MinWeightFactor: DefaultHealthMinWeightFactor},
		},
		{
			name: "zero weight factor takes default",
			in:   CooldownHealthConfig{MinWeightFactor: 0},
			want: CooldownHealthConfig{HalfLifeSeconds: DefaultHealthHalfLifeSeconds, MinSamples: DefaultHealthMinSamples, MinWeightFactor: DefaultHealthMinWeightFactor},
		},
		{
			name: "weight factor below floor is raised",
			in:   CooldownHealthConfig{MinWeightFactor: MinHealthMinWeightFactor / 2},
			want: CooldownHealthConfig{HalfLifeSeconds: DefaultHealthHalfLifeSeconds, MinSamples: DefaultHealthMinSamples, MinWeightFactor: MinHealthMinWeightFactor},
		},
		{
			name: "weight factor above one is clamped",
			in:   CooldownHealthConfig{MinWeightFactor: 2},
			want: CooldownHealthConfig{HalfLifeSeconds: DefaultHealthHalfLifeSeconds, MinSamples: DefaultHealthMinSamples, MinWeightFactor: MaxHealthMinWeightFactor},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			got := in.Normalized()
			if got == nil {
				t.Fatal("Normalized() = nil")
			}
			if *got != tt.want {
				t.Fatalf("Normalized() = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestCooldownHealthConfigActive(t *testing.T) {
	if (*CooldownHealthConfig)(nil).Active() {
		t.Fatal("nil health config must not be active")
	}
	if (&CooldownHealthConfig{}).Active() {
		t.Fatal("disabled health config must not be active")
	}
	if !(&CooldownHealthConfig{Enabled: true}).Active() {
		t.Fatal("enabled health config must be active")
	}
}

func TestCooldownHealthConfigValidate(t *testing.T) {
	for _, cfg := range []CooldownHealthConfig{
		{},
		{Enabled: true},
		{HalfLifeSeconds: MinHealthHalfLifeSeconds, MinSamples: MinHealthMinSamples, MinWeightFactor: MinHealthMinWeightFactor},
		{HalfLifeSeconds: MaxHealthHalfLifeSeconds, MinSamples: MaxHealthMinSamples, MinWeightFactor: MaxHealthMinWeightFactor},
	} {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("valid config %+v rejected: %v", cfg, err)
		}
	}

	invalid := []struct {
		name string
		cfg  CooldownHealthConfig
		want string
	}{
		{"half life below minimum", CooldownHealthConfig{HalfLifeSeconds: MinHealthHalfLifeSeconds - 1}, "health.half_life_seconds"},
		{"half life above maximum", CooldownHealthConfig{HalfLifeSeconds: MaxHealthHalfLifeSeconds + 1}, "health.half_life_seconds"},
		{"negative half life", CooldownHealthConfig{HalfLifeSeconds: -1}, "health.half_life_seconds"},
		{"min samples below minimum", CooldownHealthConfig{MinSamples: -1}, "health.min_samples"},
		{"min samples above maximum", CooldownHealthConfig{MinSamples: MaxHealthMinSamples + 1}, "health.min_samples"},
		{"weight factor below floor", CooldownHealthConfig{MinWeightFactor: MinHealthMinWeightFactor / 2}, "health.min_weight_factor"},
		{"weight factor above one", CooldownHealthConfig{MinWeightFactor: 2}, "health.min_weight_factor"},
		{"negative weight factor", CooldownHealthConfig{MinWeightFactor: -1}, "health.min_weight_factor"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatalf("config %+v must be rejected", tt.cfg)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}

	if err := (*CooldownHealthConfig)(nil).Validate(); err != nil {
		t.Fatalf("nil config must validate, got %v", err)
	}
}

func TestCooldownPolicyValidateCapabilitySections(t *testing.T) {
	// Nil capability sections are always valid.
	if err := (&CooldownPolicy{}).Validate(); err != nil {
		t.Fatalf("policy without capability sections must validate, got %v", err)
	}
	if err := (*CooldownPolicy)(nil).Validate(); err != nil {
		t.Fatalf("nil policy must validate, got %v", err)
	}

	valid := &CooldownPolicy{
		Capacity:   &CooldownCapacityConfig{Enabled: true, TokenLimit: 1000, Scope: CapacityScopeProvider},
		Escalation: &CooldownEscalationConfig{Enabled: true, MaxMultiplier: 4},
		Learning:   &CooldownLearningConfig{Enabled: true, WindowSeconds: 60, MaxSamples: 50, MinSamples: 5},
		Health:     &CooldownHealthConfig{Enabled: true, HalfLifeSeconds: 120, MinSamples: 5, MinWeightFactor: 0.05},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid capability sections rejected: %v", err)
	}

	invalid := []struct {
		name   string
		policy CooldownPolicy
		want   string
	}{
		{
			name:   "invalid capacity",
			policy: CooldownPolicy{Capacity: &CooldownCapacityConfig{WindowSeconds: 1}},
			want:   "cooldown_policy.capacity.window_seconds",
		},
		{
			name:   "invalid escalation",
			policy: CooldownPolicy{Escalation: &CooldownEscalationConfig{MaxMultiplier: 99}},
			want:   "cooldown_policy.escalation.max_multiplier",
		},
		{
			name:   "invalid learning",
			policy: CooldownPolicy{Learning: &CooldownLearningConfig{MaxSamples: 1}},
			want:   "cooldown_policy.learning.max_samples",
		},
		{
			name:   "invalid health",
			policy: CooldownPolicy{Health: &CooldownHealthConfig{MinWeightFactor: 2}},
			want:   "cooldown_policy.health.min_weight_factor",
		},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if err == nil {
				t.Fatalf("policy %+v must be rejected", tt.policy)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
			// The write path reports this error verbatim (behind a message that
			// no longer repeats "cooldown policy"), so the field path must be
			// anchored exactly once, not double-prefixed.
			if n := strings.Count(err.Error(), "cooldown_policy"); n != 1 {
				t.Fatalf("error %q must contain the cooldown_policy prefix exactly once, got %d", err, n)
			}
		})
	}

	// The pre-existing rule validation must still run and still take
	// precedence: an invalid retry_after_mode is reported regardless of the
	// capability sections.
	policy := &CooldownPolicy{
		RateLimit: &CooldownPolicyRule{RetryAfterMode: "sometimes"},
		Capacity:  &CooldownCapacityConfig{WindowSeconds: 1},
	}
	if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), "cooldown_policy.rate_limit.retry_after_mode") {
		t.Fatalf("expected rate_limit error to win, got %v", err)
	}
}

// ptrInt64 is a local helper for the key_limits cases above.
func ptrInt64(v int64) *int64 { return &v }

// TestCooldownCapacityConfigNormalizedDeepCopiesKeyLimits pins that a
// normalized config cannot be mutated through the caller's pointer fields.
func TestCooldownCapacityConfigNormalizedDeepCopiesKeyLimits(t *testing.T) {
	token := int64(100)
	in := CooldownCapacityConfig{
		Enabled:   true,
		Scope:     CapacityScopeKey,
		KeyLimits: map[string]CooldownCapacityKeyLimit{"k": {TokenLimit: &token}},
	}
	out := in.Normalized()
	token = 5
	if got := *out.KeyLimits["k"].TokenLimit; got != 100 {
		t.Fatalf("normalized copy followed the caller's pointer: got %d, want 100", got)
	}
	outToken := *out.KeyLimits["k"].TokenLimit
	if &outToken == in.KeyLimits["k"].TokenLimit {
		t.Fatal("normalized copy aliases the caller's pointer")
	}
}

// TestCooldownCapacityConfigActiveWithKeyLimits pins that per-key budgets can
// be the only configured budget.
func TestCooldownCapacityConfigActiveWithKeyLimits(t *testing.T) {
	cfg := &CooldownCapacityConfig{
		Enabled:   true,
		Scope:     CapacityScopeKey,
		KeyLimits: map[string]CooldownCapacityKeyLimit{"k": {RequestLimit: ptrInt64(10)}},
	}
	if !cfg.Active() {
		t.Fatal("a key_limits-only section must be active")
	}
	zero := &CooldownCapacityConfig{
		Enabled:   true,
		Scope:     CapacityScopeKey,
		KeyLimits: map[string]CooldownCapacityKeyLimit{"k": {RequestLimit: ptrInt64(0)}},
	}
	if zero.Active() {
		t.Fatal("an all-zero key_limits section is a no-op and must not be active")
	}
}

// TestCooldownCapacityKeyLimitsJSONRoundTrip pins that per-key overrides survive
// the config store's JSON round-trip, and that an explicit 0 stays an explicit
// 0 instead of collapsing into "inherit".
func TestCooldownCapacityKeyLimitsJSONRoundTrip(t *testing.T) {
	policy := CooldownPolicy{
		Capacity: &CooldownCapacityConfig{
			Enabled:    true,
			Scope:      CapacityScopeKey,
			TokenLimit: 1000,
			KeyLimits: map[string]CooldownCapacityKeyLimit{
				"key-a": {TokenLimit: ptrInt64(0)},
				"key-b": {RequestLimit: ptrInt64(20)},
			},
		},
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back CooldownPolicy
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Capacity == nil || len(back.Capacity.KeyLimits) != 2 {
		t.Fatalf("per-key overrides lost in the round-trip: %s", raw)
	}
	a := back.Capacity.KeyLimits["key-a"]
	if a.TokenLimit == nil || *a.TokenLimit != 0 || a.RequestLimit != nil {
		t.Fatalf("key-a round-trip = %+v, want an explicit token_limit=0 and no request limit", a)
	}
	b := back.Capacity.KeyLimits["key-b"]
	if b.RequestLimit == nil || *b.RequestLimit != 20 || b.TokenLimit != nil {
		t.Fatalf("key-b round-trip = %+v, want request_limit=20 and no token limit", b)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("round-tripped policy must validate: %v", err)
	}

	// No overrides => the field is omitted entirely (byte-compatible with a
	// config written before this feature existed).
	bare, err := json.Marshal(&CooldownCapacityConfig{Enabled: true, Scope: CapacityScopeKey, TokenLimit: 1})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(bare), "key_limits") {
		t.Fatalf("empty key_limits must be omitted, got %s", bare)
	}
}

// TestCooldownCapacityConfigValidateKeyModelLimits pins the nested layer's
// write-path rules: a models-only credential is valid, empty model names and
// empty model entries are not, and the scope requirement still applies.
func TestCooldownCapacityConfigValidateKeyModelLimits(t *testing.T) {
	valid := []CooldownCapacityConfig{
		{
			Scope: CapacityScopeKey,
			KeyLimits: map[string]CooldownCapacityKeyLimit{
				"k": {Models: map[string]CooldownCapacityModelLimit{
					"gpt-4o": {TokenLimit: ptrInt64(100)},
				}},
			},
		},
		{
			Scope: CapacityScopeKey,
			KeyLimits: map[string]CooldownCapacityKeyLimit{
				"k": {TokenLimit: ptrInt64(1000), Models: map[string]CooldownCapacityModelLimit{
					"gpt-4o": {RequestLimit: ptrInt64(0)},
				}},
			},
		},
	}
	for i := range valid {
		if err := valid[i].Validate(); err != nil {
			t.Fatalf("valid config %+v rejected: %v", valid[i], err)
		}
	}

	invalid := []struct {
		name string
		cfg  CooldownCapacityConfig
		want string
	}{
		{
			"models under the provider_model scope",
			CooldownCapacityConfig{Scope: CapacityScopeProviderModel, KeyLimits: map[string]CooldownCapacityKeyLimit{
				"k": {Models: map[string]CooldownCapacityModelLimit{"gpt-4o": {TokenLimit: ptrInt64(1)}}},
			}},
			"capacity.key_limits requires capacity.scope",
		},
		{
			"empty model name",
			CooldownCapacityConfig{Scope: CapacityScopeKey, KeyLimits: map[string]CooldownCapacityKeyLimit{
				"k": {Models: map[string]CooldownCapacityModelLimit{"  ": {TokenLimit: ptrInt64(1)}}},
			}},
			"empty model name",
		},
		{
			"model entry with no limit",
			CooldownCapacityConfig{Scope: CapacityScopeKey, KeyLimits: map[string]CooldownCapacityKeyLimit{
				"k": {Models: map[string]CooldownCapacityModelLimit{"gpt-4o": {}}},
			}},
			"must set token_limit or request_limit",
		},
		{
			"negative model token limit",
			CooldownCapacityConfig{Scope: CapacityScopeKey, KeyLimits: map[string]CooldownCapacityKeyLimit{
				"k": {Models: map[string]CooldownCapacityModelLimit{"gpt-4o": {TokenLimit: ptrInt64(-1)}}},
			}},
			"token_limit must not be negative",
		},
		{
			"negative model request limit",
			CooldownCapacityConfig{Scope: CapacityScopeKey, KeyLimits: map[string]CooldownCapacityKeyLimit{
				"k": {Models: map[string]CooldownCapacityModelLimit{"gpt-4o": {RequestLimit: ptrInt64(-1)}}},
			}},
			"request_limit must not be negative",
		},
	}
	for _, tt := range invalid {
		err := tt.cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("%s: err = %v, want it to contain %q", tt.name, err, tt.want)
		}
	}
}

// TestCooldownCapacityConfigNormalizedDeepCopiesModelLimits pins that
// normalizing never aliases the caller's nested map or pointer fields.
func TestCooldownCapacityConfigNormalizedDeepCopiesModelLimits(t *testing.T) {
	token := int64(100)
	in := &CooldownCapacityConfig{
		Scope: CapacityScopeKey,
		KeyLimits: map[string]CooldownCapacityKeyLimit{
			"k": {Models: map[string]CooldownCapacityModelLimit{
				"gpt-4o": {TokenLimit: &token},
			}},
		},
	}
	out := in.Normalized()
	if out == nil {
		t.Fatal("Normalized returned nil")
	}
	got := out.KeyLimits["k"].Models["gpt-4o"].TokenLimit
	if got == nil || *got != 100 {
		t.Fatalf("normalized model token limit = %v, want 100", got)
	}
	if got == in.KeyLimits["k"].Models["gpt-4o"].TokenLimit {
		t.Fatal("normalized config must not share the caller's pointer")
	}
	// Mutating the output must not leak back into the input.
	*got = 1
	if *in.KeyLimits["k"].Models["gpt-4o"].TokenLimit != 100 {
		t.Fatal("mutating the normalized copy changed the caller's value")
	}
	delete(out.KeyLimits["k"].Models, "gpt-4o")
	if _, ok := in.KeyLimits["k"].Models["gpt-4o"]; !ok {
		t.Fatal("mutating the normalized map changed the caller's map")
	}
}
