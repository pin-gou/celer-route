package schemas

import (
	"strings"
	"testing"
)

func rpmErr() *BifrostError {
	return &BifrostError{
		StatusCode: intPtr(429),
		Error: &ErrorField{
			Message: "rpm exhausted",
			Type:    strPtr("rate_limit_error"),
			Code:    strPtr("EndpointRPMExceeded"),
		},
	}
}

func tpmErr() *BifrostError {
	return &BifrostError{
		StatusCode: intPtr(429),
		Error: &ErrorField{
			Message: "tpm exhausted",
			Type:    strPtr("rate_limit_error"),
			Code:    strPtr("EndpointTPMExceeded"),
		},
	}
}

func strPtr(s string) *string { return &s }

func TestCooldownPolicyMatch_NormalizedDimension(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"", ""},
		{"rpm", CooldownDimensionRPM},
		{"RPM", CooldownDimensionRPM},
		{" tpm ", CooldownDimensionTPM},
	} {
		m := CooldownPolicyMatch{Dimension: tc.in}
		if got := m.NormalizedDimension(); got != tc.want {
			t.Fatalf("NormalizedDimension(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCooldownPolicyRule_MatchDimension(t *testing.T) {
	any := CooldownPolicyRule{
		TTLSeconds: 30,
		MatchMode:  "any",
		Match: []CooldownPolicyMatch{
			// Fires on the RPM error but pins nothing: it must not stop the
			// scan, because a later fired match may pin a dimension.
			{StatusCode: intPtr(429)},
			{Code: []string{"EndpointRPMExceeded"}, Dimension: CooldownDimensionRPM},
			{Code: []string{"EndpointTPMExceeded"}, Dimension: CooldownDimensionTPM},
		},
	}

	fired, dim := any.MatchDimension(rpmErr())
	if !fired {
		t.Fatal("rate-limit error must fire the rule")
	}
	if dim != CooldownDimensionRPM {
		t.Fatalf("MatchDimension = %q, want %q (first FIRED match that pins one)", dim, CooldownDimensionRPM)
	}

	// An error that only satisfies the unpinned catch-all fires, but pins
	// nothing → auto.
	other := &BifrostError{StatusCode: intPtr(429), Error: &ErrorField{Message: "boom"}}
	fired, dim = any.MatchDimension(other)
	if !fired || dim != "" {
		t.Fatalf("catch-all-only match: fired=%v dim=%q, want fired=true dim=\"\"", fired, dim)
	}

	// Disabled rule never fires.
	disabled := false
	any.Enabled = &disabled
	if fired, dim := any.MatchDimension(rpmErr()); fired || dim != "" {
		t.Fatalf("disabled rule must not fire, got fired=%v dim=%q", fired, dim)
	}
}

func TestCooldownPolicyRule_MatchDimensionAllMode(t *testing.T) {
	all := CooldownPolicyRule{
		TTLSeconds: 30,
		MatchMode:  "all",
		Match: []CooldownPolicyMatch{
			{StatusCode: intPtr(429)},
			{Code: []string{"EndpointTPMExceeded"}, Dimension: CooldownDimensionTPM},
		},
	}
	if fired, dim := all.MatchDimension(tpmErr()); !fired || dim != CooldownDimensionTPM {
		t.Fatalf("all-mode fired=%v dim=%q, want true/%q", fired, dim, CooldownDimensionTPM)
	}
	// "all" fails when any predicate misses.
	if fired, _ := all.MatchDimension(rpmErr()); fired {
		t.Fatal("all-mode must not fire when a predicate misses")
	}
}

func TestValidateCooldownPolicy_Dimension(t *testing.T) {
	ok := &CooldownPolicy{
		RateLimit: &CooldownPolicyRule{
			TTLSeconds: 60,
			Match:      []CooldownPolicyMatch{{Code: []string{"EndpointRPMExceeded"}, Dimension: "rpm"}},
		},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a pinned rpm dimension on rate_limit must validate, got %v", err)
	}

	bad := &CooldownPolicy{
		RateLimit: &CooldownPolicyRule{
			TTLSeconds: 60,
			Match:      []CooldownPolicyMatch{{StatusCode: intPtr(429), Dimension: "rps"}},
		},
	}
	err := bad.Validate()
	if err == nil {
		t.Fatal("expected an error for an unknown dimension value")
	}
	for _, want := range []string{"cooldown_policy.rate_limit.match[0].dimension", "rpm", "tpm"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q must mention %q", err.Error(), want)
		}
	}

	quota := &CooldownPolicy{
		Quota: &CooldownPolicyRule{
			TTLSeconds: 600,
			Match: []CooldownPolicyMatch{{
				MessageContains: []string{"quota"},
				Dimension:       CooldownDimensionRPM,
			}},
		},
	}
	if err := quota.Validate(); err == nil || !strings.Contains(err.Error(), "cooldown_policy.quota.match[0].dimension") {
		t.Fatalf("expected a quota dimension rejection, got %v", err)
	}
}
