package schemas

import (
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
			if *got != tt.want {
				t.Fatalf("Normalized() = %+v, want %+v", *got, tt.want)
			}
			if in != tt.in {
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
