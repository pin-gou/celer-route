package schemas

import (
	"strings"
	"testing"
)

func TestValidateCooldownPolicy_NilPolicyIsValid(t *testing.T) {
	var p *CooldownPolicy
	if err := p.Validate(); err != nil {
		t.Fatalf("nil policy must validate, got %v", err)
	}
}

func TestValidateCooldownPolicy_NoQuotaRuleIsValid(t *testing.T) {
	p := &CooldownPolicy{
		RateLimit: &CooldownPolicyRule{
			TTLSeconds: 30,
			Match:      []CooldownPolicyMatch{{StatusCode: intPtr(429)}},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("a bare status match in rate_limit is a legitimate catch-all, got %v", err)
	}
}

func TestValidateCooldownPolicy_RejectsBareStatusQuota(t *testing.T) {
	p := &CooldownPolicy{
		Quota: &CooldownPolicyRule{
			TTLSeconds: 600,
			Match:      []CooldownPolicyMatch{{StatusCode: intPtr(429)}},
		},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected an error for a quota rule matching a bare status code")
	}
	// The message must name the field path and the consequence, otherwise the
	// operator cannot tell why the config was rejected.
	for _, want := range []string{"cooldown_policy.quota.match[0]", "429", "rate_limit"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q must mention %q", err.Error(), want)
		}
	}
}

func TestValidateCooldownPolicy_BareStatusInLaterMatchIsRejected(t *testing.T) {
	p := &CooldownPolicy{
		Quota: &CooldownPolicyRule{
			TTLSeconds: 600,
			Match: []CooldownPolicyMatch{
				{MessageContains: []string{"insufficient_quota"}},
				{StatusCode: intPtr(429)},
			},
		},
	}
	if err := p.Validate(); err == nil {
		t.Fatal("expected an error when any quota match is a bare status code")
	}
}

func TestValidateCooldownPolicy_ScopedQuotaMatchesAreValid(t *testing.T) {
	p := &CooldownPolicy{
		Quota: &CooldownPolicyRule{
			TTLSeconds: 600,
			Match: []CooldownPolicyMatch{
				{StatusCode: intPtr(429), Code: []string{"insufficient_quota"}},
				{MessageContains: []string{"token plan entitlement exhausted"}},
				{Type: []string{"quota_exceeded_error"}},
			},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("quota matches with any additional predicate must validate, got %v", err)
	}
}

// TestValidateCooldownPolicy_RetryAfterMode pins the retry_after_mode enum:
// empty and the three documented values (case-insensitive) validate, anything
// else is rejected with the field path in the message.
func TestValidateCooldownPolicy_RetryAfterMode(t *testing.T) {
	valid := []string{"", "off", "cap", "exact", "OFF", "Cap", "Exact"}
	for _, mode := range valid {
		for _, field := range []string{"rate_limit", "quota"} {
			rule := &CooldownPolicyRule{
				TTLSeconds:     60,
				RetryAfterMode: mode,
				Match:          []CooldownPolicyMatch{{MessageContains: []string{"rate limit"}}},
			}
			p := &CooldownPolicy{}
			if field == "quota" {
				p.Quota = rule
			} else {
				p.RateLimit = rule
			}
			if err := p.Validate(); err != nil {
				t.Fatalf("retry_after_mode %q on %s must validate, got %v", mode, field, err)
			}
		}
	}

	for _, mode := range []string{"max", "exactish", "ON"} {
		rule := &CooldownPolicyRule{
			TTLSeconds:     60,
			RetryAfterMode: mode,
			Match:          []CooldownPolicyMatch{{MessageContains: []string{"rate limit"}}},
		}
		p := &CooldownPolicy{RateLimit: rule}
		err := p.Validate()
		if err == nil {
			t.Fatalf("retry_after_mode %q must be rejected", mode)
		}
		for _, want := range []string{"cooldown_policy.rate_limit.retry_after_mode", "off", "cap", "exact"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q must mention %q", err.Error(), want)
			}
		}
	}
}

func TestCooldownPolicyRule_RetryAfterModeOrDefault(t *testing.T) {
	tests := []struct {
		name string
		rule *CooldownPolicyRule
		want string
	}{
		{"nil rule", nil, "off"},
		{"empty", &CooldownPolicyRule{}, "off"},
		{"off", &CooldownPolicyRule{RetryAfterMode: "off"}, "off"},
		{"cap", &CooldownPolicyRule{RetryAfterMode: "cap"}, "cap"},
		{"exact", &CooldownPolicyRule{RetryAfterMode: "exact"}, "exact"},
		{"mixed case", &CooldownPolicyRule{RetryAfterMode: "ExAcT"}, "exact"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.RetryAfterModeOrDefault(); got != tt.want {
				t.Fatalf("RetryAfterModeOrDefault() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDefaultCooldownPolicy_AllProvidersValidate guards the built-in defaults
// against the exact mistake Validate() exists to catch. A default that fails
// its own validation would either brick provider startup or train operators to
// ignore the error.
func TestDefaultCooldownPolicy_AllProvidersValidate(t *testing.T) {
	providers := append([]ModelProvider{}, StandardProviders...)
	providers = append(providers, ModelProvider("some-unknown-custom-provider"))
	for _, p := range providers {
		policy := DefaultCooldownPolicy(p)
		if policy == nil {
			t.Fatalf("DefaultCooldownPolicy(%q) returned nil", p)
		}
		if err := policy.Validate(); err != nil {
			t.Fatalf("DefaultCooldownPolicy(%q) fails its own validation: %v", p, err)
		}
	}
}
