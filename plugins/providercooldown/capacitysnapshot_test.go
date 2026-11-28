package providercooldown

import (
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// TestCapacityUsageEmpty pins the empty contract: a nil plugin, a plugin with
// no state and a state with no enabled gate all report a non-nil, empty slice
// (the HTTP layer must then serialize "entries": [] rather than null).
func TestCapacityUsageEmpty(t *testing.T) {
	var nilPlugin *CooldownPlugin
	if got := nilPlugin.CapacityUsage(); got == nil || len(got) != 0 {
		t.Fatalf("nil plugin = %#v, want a non-nil empty slice", got)
	}

	noState := &CooldownPlugin{}
	if got := noState.CapacityUsage(); got == nil || len(got) != 0 {
		t.Fatalf("plugin without state = %#v, want a non-nil empty slice", got)
	}

	inert := &CooldownPlugin{State: NewCooldownState(time.Minute)}
	if got := inert.CapacityUsage(); got == nil || len(got) != 0 {
		t.Fatalf("state without an enabled gate = %#v, want a non-nil empty slice", got)
	}
}

// TestCapacityUsageDefaultGate pins the default-gate snapshot shape: a provider
// that only inherits the plugin-level capacity section is reported with
// Source="default", the effective thresholds (limit*headroom) and the usage
// actually accounted in the window.
func TestCapacityUsageDefaultGate(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		RequestLimit:  100,
		WindowSeconds: 60,
		Headroom:      0.9,
		Scope:         CapacityScopeProviderModel,
	})

	spend(t, p, nil, schemas.OpenAI, "gpt-4o", 120)

	entries := p.CapacityUsage()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want exactly one", entries)
	}
	got := entries[0]
	want := ProviderCapacityUsage{
		Provider:          schemas.OpenAI,
		Model:             "gpt-4o",
		Scope:             CapacityScopeProviderModel,
		WindowSeconds:     60,
		UsedTokens:        120,
		UsedRequests:      1,
		TokenLimit:        1000,
		RequestLimit:      100,
		Headroom:          0.9,
		ThresholdTokens:   900,
		ThresholdRequests: 90,
		LimitSource:       capacityLimitSourceBase,
		RetryAfterSeconds: 0, // below both thresholds
		Source:            capacityUsageSourceDefault,
	}
	if got != want {
		t.Fatalf("entry = %+v, want %+v", got, want)
	}
}

// TestCapacityUsageProviderScopeOmitsModel pins that a provider-scoped budget
// reports an empty model: the usage is shared by every model of the provider,
// so attributing it to one model would be wrong.
func TestCapacityUsageProviderScopeOmitsModel(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		WindowSeconds: 60,
		Scope:         CapacityScopeProvider,
	})

	spend(t, p, nil, schemas.Anthropic, "claude-sonnet-4", 10)

	entries := p.CapacityUsage()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want exactly one", entries)
	}
	if entries[0].Model != "" {
		t.Fatalf("model = %q, want empty for a provider-scoped budget", entries[0].Model)
	}
	if entries[0].Scope != CapacityScopeProvider {
		t.Fatalf("scope = %q, want %q", entries[0].Scope, CapacityScopeProvider)
	}
}

// TestCapacityUsageRetryAfterOnceOverThreshold pins the Retry-After semantic:
// once the window is over a threshold the snapshot reports the seconds until
// the oldest bucket that brings usage back under it slides out; below the
// threshold it stays 0.
func TestCapacityUsageRetryAfterOnceOverThreshold(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		WindowSeconds: 60,
		Headroom:      0.9,
		Scope:         CapacityScopeProviderModel,
	})

	// Spend the whole threshold in one second; that bucket leaves the window a
	// full window from now, so the wait is the window length.
	spend(t, p, nil, schemas.OpenAI, "gpt-4o", 900)

	entries := p.CapacityUsage()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want exactly one", entries)
	}
	if entries[0].RetryAfterSeconds != 60 {
		t.Fatalf("retry_after_seconds = %d, want 60", entries[0].RetryAfterSeconds)
	}

	// Ten seconds later the same bucket releases 10s earlier, so the reported
	// wait shrinks accordingly instead of resetting to the full window.
	clk.Advance(10 * time.Second)
	entries = p.CapacityUsage()
	if len(entries) != 1 || entries[0].RetryAfterSeconds != 50 {
		t.Fatalf("entries after 10s = %#v, want retry_after_seconds 50", entries)
	}
}

// TestCapacityUsageProviderOverride pins that a provider whose own
// cooldown_policy.capacity section differs from the plugin default gets its own
// entry with Source="provider", while a provider that inherits the default
// keeps Source="default". Entries are sorted by provider then model.
func TestCapacityUsageProviderOverride(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		WindowSeconds: 60,
		Scope:         CapacityScopeProviderModel,
	})

	override := &schemas.CooldownPolicy{Capacity: &schemas.CooldownCapacityConfig{
		Enabled:       true,
		TokenLimit:    5,
		WindowSeconds: 30,
		Headroom:      1,
		Scope:         CapacityScopeProviderModel,
	}}
	spend(t, p, override, schemas.OpenAI, "gpt-4o", 5)
	spend(t, p, nil, schemas.Anthropic, "claude-sonnet-4", 10)

	entries := p.CapacityUsage()
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want two", entries)
	}
	if entries[0].Provider != schemas.Anthropic || entries[0].Source != capacityUsageSourceDefault {
		t.Fatalf("first entry = %+v, want anthropic/default (sorted first)", entries[0])
	}
	if entries[1].Provider != schemas.OpenAI || entries[1].Source != capacityUsageSourceProvider {
		t.Fatalf("second entry = %+v, want openai/provider", entries[1])
	}
	if entries[1].TokenLimit != 5 || entries[1].WindowSeconds != 30 {
		t.Fatalf("override entry = %+v, want token_limit=5 window=30", entries[1])
	}
	if entries[1].RetryAfterSeconds != 30 {
		t.Fatalf("override retry_after_seconds = %d, want 30", entries[1].RetryAfterSeconds)
	}
}

// TestCapacityUsageDropsExpiredWindow pins that a provider whose accounting
// window has fully slid away is not reported.
func TestCapacityUsageDropsExpiredWindow(t *testing.T) {
	clk := newFakeClock()
	p := newCapacityTestPlugin(t, clk, &CapacityConfig{
		Enabled:       true,
		TokenLimit:    1000,
		WindowSeconds: 10,
		Scope:         CapacityScopeProviderModel,
	})

	spend(t, p, nil, schemas.OpenAI, "gpt-4o", 10)
	if entries := p.CapacityUsage(); len(entries) != 1 {
		t.Fatalf("entries = %#v, want one live entry", entries)
	}

	clk.Advance(11 * time.Second)
	entries := p.CapacityUsage()
	if entries == nil || len(entries) != 0 {
		t.Fatalf("entries after the window slid away = %#v, want a non-nil empty slice", entries)
	}
}

// TestSplitCapacityWindowKey pins the key parsing used to attribute a live
// entry back to (provider, key, model), including a model name that itself
// contains the separator.
func TestSplitCapacityWindowKey(t *testing.T) {
	cases := []struct {
		key   string
		scope string
		wantP schemas.ModelProvider
		wantK string
		wantM string
	}{
		{"openai|gpt-4o", CapacityScopeProviderModel, schemas.OpenAI, "", "gpt-4o"},
		{"openai|vendor|model", CapacityScopeProviderModel, schemas.OpenAI, "", "vendor|model"},
		{"openai", CapacityScopeProviderModel, schemas.OpenAI, "", ""},
		{"openai", CapacityScopeProvider, schemas.OpenAI, "", ""},
		// scope=key: "provider|keyID|model", with the model allowed to carry "|".
		{"openai|key-1|gpt-4o", CapacityScopeKey, schemas.OpenAI, "key-1", "gpt-4o"},
		{"openai|key-1|vendor|model", CapacityScopeKey, schemas.OpenAI, "key-1", "vendor|model"},
		// The unattributable fallback bucket has no credential component.
		{"openai|gpt-4o", CapacityScopeKey, schemas.OpenAI, "", "gpt-4o"},
		{"openai", CapacityScopeKey, schemas.OpenAI, "", ""},
	}
	for _, tc := range cases {
		p, k, m := splitCapacityWindowKey(tc.key, tc.scope)
		if p != tc.wantP || k != tc.wantK || m != tc.wantM {
			t.Errorf("splitCapacityWindowKey(%q, %q) = (%q, %q, %q), want (%q, %q, %q)", tc.key, tc.scope, p, k, m, tc.wantP, tc.wantK, tc.wantM)
		}
	}
}
