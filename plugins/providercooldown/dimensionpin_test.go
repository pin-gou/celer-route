package providercooldown

import (
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// genericRateLimitErr is a 429 whose body names no budget: the automatic
// classifier returns unknown, which is exactly the case a match-level pin is
// meant to resolve.
func genericRateLimitErr() *schemas.BifrostError {
	return &schemas.BifrostError{
		StatusCode: intPtr(429),
		Error: &schemas.ErrorField{
			Message: "rate limited, try again later",
			Type:    strPtr("rate_limit_error"),
		},
	}
}

func pinnedRule(dimension string) *schemas.CooldownPolicyRule {
	return &schemas.CooldownPolicyRule{
		TTLSeconds:    60,
		RPMTTLSeconds: 90,
		TPMTTLSeconds: 15,
		Match: []schemas.CooldownPolicyMatch{{
			StatusCode: intPtr(429),
			Dimension:  dimension,
		}},
	}
}

func TestResolveDimension_PinWinsOverAuto(t *testing.T) {
	// The body says TPM, the operator pinned RPM: the pin is authoritative.
	if got := resolveDimension(pinnedRule(schemas.CooldownDimensionRPM), tpmRateLimitErr()); got != dimensionRPM {
		t.Fatalf("pinned rpm must override the automatic tpm, got %q", got)
	}
	// The reverse direction also holds.
	if got := resolveDimension(pinnedRule(schemas.CooldownDimensionTPM), rpmRateLimitErr()); got != dimensionTPM {
		t.Fatalf("pinned tpm must override the automatic rpm, got %q", got)
	}
}

func TestResolveDimension_AutoWhenUnpinned(t *testing.T) {
	if got := resolveDimension(pinnedRule(""), tpmRateLimitErr()); got != dimensionTPM {
		t.Fatalf("unpinned match must fall back to the body classifier, got %q", got)
	}
	if got := resolveDimension(pinnedRule(""), genericRateLimitErr()); got != dimensionUnknown {
		t.Fatalf("a body naming no budget must resolve to unknown, got %q", got)
	}
	// A nil rule (plugin defaults with no per-provider policy) still classifies.
	if got := resolveDimension(nil, rpmRateLimitErr()); got != dimensionRPM {
		t.Fatalf("nil rule must still use the body classifier, got %q", got)
	}
}

func TestEffectiveCooldownTTL_PinnedDimensionSelectsItsTTL(t *testing.T) {
	// The body names no budget (auto → unknown → 60s), but the match pins rpm,
	// so the 90s request-rate TTL applies.
	if got := effectiveCooldownTTL(pinnedRule(schemas.CooldownDimensionRPM), genericRateLimitErr()); got != 90*time.Second {
		t.Fatalf("pinned rpm TTL = %v, want 90s", got)
	}
	if got := effectiveCooldownTTL(pinnedRule(schemas.CooldownDimensionTPM), genericRateLimitErr()); got != 15*time.Second {
		t.Fatalf("pinned tpm TTL = %v, want 15s", got)
	}
	// Without a pin the generic body falls back to ttl_seconds.
	if got := effectiveCooldownTTL(pinnedRule(""), genericRateLimitErr()); got != 60*time.Second {
		t.Fatalf("unpinned unknown TTL = %v, want 60s", got)
	}
}

func TestEffectiveCooldownTTL_PinnedDimensionWithoutTTLFallsBack(t *testing.T) {
	rule := &schemas.CooldownPolicyRule{
		TTLSeconds: 60,
		Match: []schemas.CooldownPolicyMatch{{
			StatusCode: intPtr(429),
			Dimension:  schemas.CooldownDimensionRPM,
		}},
	}
	// The pin decides the dimension, but with no rpm_ttl_seconds configured the
	// base TTL still applies — pinning never invents a duration.
	if got := effectiveCooldownTTL(rule, genericRateLimitErr()); got != 60*time.Second {
		t.Fatalf("pinned rpm with no rpm TTL = %v, want 60s", got)
	}
}

func TestEffectiveQuotaCooldownTTL_IgnoresPin(t *testing.T) {
	// Quota rules cannot pin a dimension (Validate rejects it), but even a
	// hand-built one must not borrow the rate-limit TTL split: a quota
	// exhaustion is not a rate window.
	rule := &schemas.CooldownPolicyRule{
		TTLSeconds:    600,
		RPMTTLSeconds: 90,
		TPMTTLSeconds: 15,
		Match: []schemas.CooldownPolicyMatch{{
			MessageContains: []string{"quota"},
			Dimension:       schemas.CooldownDimensionRPM,
		}},
	}
	if got := effectiveQuotaCooldownTTL(rule, genericRateLimitErr()); got != 600*time.Second {
		t.Fatalf("quota TTL = %v, want 600s (dimension split must not apply)", got)
	}
}

func TestLimiterDimensionOfFields_MatchesErrorLevelClassifier(t *testing.T) {
	// The error-patterns endpoint annotates buckets from the 4 aggregate
	// fields; that decision must equal the one the mark path makes from a
	// live error with the same content.
	for _, tc := range []struct {
		name string
		err  *schemas.BifrostError
	}{
		{"rpm", rpmRateLimitErr()},
		{"tpm", tpmRateLimitErr()},
		{"generic", genericRateLimitErr()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, typ, msg := "", "", ""
			if tc.err.Error != nil {
				if tc.err.Error.Code != nil {
					code = *tc.err.Error.Code
				}
				if tc.err.Error.Type != nil {
					typ = *tc.err.Error.Type
				}
				msg = tc.err.Error.Message
			}
			fromFields := LimiterDimensionOfFields(typ, code, msg)
			fromError := string(limiterDimensionOf(tc.err))
			if fromFields != fromError {
				t.Fatalf("fields=%q error=%q — the two entry points diverged", fromFields, fromError)
			}
		})
	}
}

// TestResolveDimension_PinFlowsIntoLearning guards the coupling that made the
// pin worth adding: a 429 whose body is ambiguous must be sampled into the
// right budget bucket, because an RPM rejection must never feed the token
// limit estimate.
func TestResolveDimension_PinFlowsIntoLearning(t *testing.T) {
	rule := pinnedRule(schemas.CooldownDimensionRPM)
	if dim := resolveDimension(rule, genericRateLimitErr()); isTokenBudgetDimension(dim) {
		t.Fatalf("a pinned-rpm sample must be excluded from the token estimate, got %q", dim)
	}
	if dim := resolveDimension(pinnedRule(schemas.CooldownDimensionTPM), genericRateLimitErr()); !isTokenBudgetDimension(dim) {
		t.Fatalf("a pinned-tpm sample must feed the token estimate, got %q", dim)
	}
}
