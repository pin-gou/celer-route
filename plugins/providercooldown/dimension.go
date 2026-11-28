package providercooldown

import (
	"strings"

	"github.com/pin-gou/celer-route/core/schemas"
)

// limiterDimension identifies which upstream budget a rate-limit rejection hit:
// the request rate (RPM/RPS) or the token rate (TPM).
//
// The distinction matters because the two budgets recover on different
// timescales. A request-rate rejection caused by traffic outside this gateway
// (the production SenseNova case) does not clear until the external consumer
// stops, so backing off for the token-rate TTL just probes it again a few
// seconds later. Deriving the dimension from the error body lets the cooldown
// rule choose a longer TTL for RPM marks (rpm_ttl_seconds) and keeps the
// learned token limit from being corrupted by RPM samples.
//
// dimensionUnknown means "the error did not identify the budget". It is the
// safe default: the rule's plain ttl_seconds applies, and the rps/rpm samples
// are still recorded for diagnostics but only unknown + tpm samples feed the
// token-limit estimate (see learnSample).
type limiterDimension string

const (
	dimensionRPM     limiterDimension = "rpm"
	dimensionTPM     limiterDimension = "tpm"
	dimensionUnknown limiterDimension = "unknown"
)

// limiterDimensionPattern is one ordered matcher. The table is evaluated in
// order and the first hit wins, so more specific patterns must come first.
//
// notContains guards a generic pattern against a more specific budget: the
// bare "rate limit exceeded" phrase is treated as request-rate only when the
// text does not mention tokens or TPM at all. Without that guard a generic
// message paired with a token-budget note would be misfiled as RPM.
type limiterDimensionPattern struct {
	dimension   limiterDimension
	pattern     string
	notContains []string
}

// limiterDimensionPatterns is the ordered matcher table. Token patterns are
// checked before request patterns so a body that carries both a generic rate
// limit phrase and a token budget signal resolves to tpm.
//
// "rps" (requests per second) folds into the rpm bucket: it is a request-rate
// limit too, and it recovers once the request pressure drops — the same
// dynamics as rpm. Keeping a separate dimension would add no operational
// difference.
var limiterDimensionPatterns = []limiterDimensionPattern{
	// Token-rate patterns. The exact provider codes come from the SenseNova
	// production 429 samples (EndpointTPMExceeded /
	// ModelAccountTpmRateLimitExceeded) plus the message forms the same
	// provider emits.
	{dimension: dimensionTPM, pattern: "endpointtpmexceeded"},
	{dimension: dimensionTPM, pattern: "modelaccounttpmratelimitexceeded"},
	{dimension: dimensionTPM, pattern: "tpm exhausted"},
	{dimension: dimensionTPM, pattern: "token rate limit"},
	{dimension: dimensionTPM, pattern: "tokens per minute"},

	// Request-rate patterns (specific codes first, then messages).
	{dimension: dimensionRPM, pattern: "endpointrpmexceeded"},
	{dimension: dimensionRPM, pattern: "modelaccountrpmratelimitexceeded"},
	{dimension: dimensionRPM, pattern: "rpm exhausted"},
	{dimension: dimensionRPM, pattern: "rps exhausted"},
	{dimension: dimensionRPM, pattern: "requestbursttoofast"},
	{dimension: dimensionRPM, pattern: "throttling.burstrate"},

	// Generic fallback. Only request-rate when nothing token-related is
	// present; otherwise leave it to the tpm patterns above (or unknown).
	{
		dimension:   dimensionRPM,
		pattern:     "rate limit exceeded",
		notContains: []string{"token", "tpm"},
	},
}

// limiterDimensionOf derives the rate-limit dimension from a provider error.
//
// It scans the error's code and type first, then the rendered message
// (GetErrorString). Both are folded into one lower-case haystack because the
// ordered pattern table handles precedence: a tpm pattern wins over a generic
// rpm phrase regardless of which field carried it.
//
// This runs on the failure path only — GetErrorString may render a fallback
// string and the lower-casing allocates. It must never be called from a
// success/hot path.
func limiterDimensionOf(err *schemas.BifrostError) limiterDimension {
	if err == nil {
		return dimensionUnknown
	}
	var b strings.Builder
	if err.Error != nil {
		if err.Error.Code != nil {
			b.WriteString(*err.Error.Code)
			b.WriteByte(' ')
		}
		if err.Error.Type != nil {
			b.WriteString(*err.Error.Type)
			b.WriteByte(' ')
		}
	}
	b.WriteString(err.GetErrorString())
	return LimiterDimensionOfText(b.String())
}

// LimiterDimensionOfFields derives the dimension from the individual pieces an
// aggregated error pattern carries (the /logs/error-patterns buckets), so the
// UI can echo what the automatic classifier would decide for that error shape
// without re-implementing the pattern table on the client.
//
// It is the field-level entry point of the same classifier limiterDimensionOf
// uses; the pattern table stays the single source of truth.
func LimiterDimensionOfFields(errType string, errCode string, message string) string {
	var b strings.Builder
	if errCode != "" {
		b.WriteString(errCode)
		b.WriteByte(' ')
	}
	if errType != "" {
		b.WriteString(errType)
		b.WriteByte(' ')
	}
	b.WriteString(message)
	return string(LimiterDimensionOfText(b.String()))
}

// LimiterDimensionOfText is the public, text-level entry point of the
// classifier. It lower-cases the haystack and applies the ordered pattern
// table; "" is impossible (unmatched text yields "unknown").
//
// Exported so the error-patterns handler can annotate its buckets with the
// same decision the mark path would make.
func LimiterDimensionOfText(text string) limiterDimension {
	return matchLimiterDimension(strings.ToLower(text))
}

// matchLimiterDimension applies the ordered pattern table to an already
// lower-cased haystack.
func matchLimiterDimension(haystack string) limiterDimension {
	for _, p := range limiterDimensionPatterns {
		if !strings.Contains(haystack, p.pattern) {
			continue
		}
		skip := false
		for _, banned := range p.notContains {
			if strings.Contains(haystack, banned) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		return p.dimension
	}
	return dimensionUnknown
}

// isTokenBudgetDimension reports whether a sample dimension may contribute to
// the learned token-limit estimate. RPM rejections are recorded for
// diagnostics but excluded from the estimate.
func isTokenBudgetDimension(d limiterDimension) bool {
	return d != dimensionRPM
}

// resolveDimension decides which budget a rate-limit rejection is attributed
// to. An operator-pinned match dimension always wins; otherwise the classifier
// reads the provider error body.
//
// The pin is match-scoped on purpose: a policy can carry a predicate that is
// known to target the request rate (e.g. code=EndpointRPMExceeded) next to a
// generic status_code=429 catch-all, and only the specific one should force
// the longer RPM TTL.
//
// This runs on the failure path (marks and learning samples), never on the
// success path.
func resolveDimension(rule *schemas.CooldownPolicyRule, err *schemas.BifrostError) limiterDimension {
	if err == nil {
		return dimensionUnknown
	}
	if rule != nil {
		if _, explicit := rule.MatchDimension(err); explicit != "" {
			return limiterDimension(explicit)
		}
	}
	return limiterDimensionOf(err)
}

// rateLimitRuleOf returns the policy's rate-limit rule, or nil when there is
// no policy at all (plugin defaults, or a caller that never stamped one). The
// nil-safe accessor keeps the learning path from dereferencing a nil policy.
func rateLimitRuleOf(policy *schemas.CooldownPolicy) *schemas.CooldownPolicyRule {
	if policy == nil {
		return nil
	}
	return policy.RateLimit
}
