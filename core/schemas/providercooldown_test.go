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
