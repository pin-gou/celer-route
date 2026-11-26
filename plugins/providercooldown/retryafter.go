package providercooldown

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxProviderRetryAfterHint bounds how far into the future a provider-supplied
// retry hint may push a cooldown. Anything above this is treated as "no hint":
// a bogus or mis-parsed header must never park a key for days.
const maxProviderRetryAfterHint = 24 * time.Hour

// providerRetryAfterHint returns the delay the provider asked us to wait, or 0
// when no usable hint is present.
//
// Resolution order (first usable value wins):
//
//  1. Retry-After — integer/float seconds, or an HTTP-date
//  2. x-ratelimit-reset-after — seconds (fractions accepted)
//  3. x-ratelimit-reset-requests / x-ratelimit-reset-tokens — a Go duration
//     string like "6m0s" (OpenAI style), falling back to integer seconds. When
//     both are present the earliest (smallest) value wins.
//  4. anthropic-ratelimit-requests-reset / anthropic-ratelimit-tokens-reset —
//     RFC3339 timestamps; when both are present the earliest wins.
//
// Header names are matched case-insensitively. A result must be strictly
// positive and at most 24h, otherwise 0 is returned. Unparseable values are
// silently ignored — this runs on the error path and must never panic or
// surface a new error.
func providerRetryAfterHint(headers map[string]string) time.Duration {
	if len(headers) == 0 {
		return 0
	}
	lower := make(map[string]string, len(headers))
	for k, v := range headers {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		lower[key] = strings.TrimSpace(v)
	}
	if len(lower) == 0 {
		return 0
	}

	if d, ok := usableHint(parseRetryAfter(lower["retry-after"])); ok {
		return d
	}
	if d, ok := usableHint(parseSeconds(lower["x-ratelimit-reset-after"])); ok {
		return d
	}
	reqReset, reqOK := parseDurationOrSeconds(lower["x-ratelimit-reset-requests"])
	tokReset, tokOK := parseDurationOrSeconds(lower["x-ratelimit-reset-tokens"])
	if d, ok := earliestHint(reqReset, reqOK, tokReset, tokOK); ok {
		return d
	}
	anthropicReq, anthropicReqOK := parseRFC3339Delta(lower["anthropic-ratelimit-requests-reset"])
	anthropicTok, anthropicTokOK := parseRFC3339Delta(lower["anthropic-ratelimit-tokens-reset"])
	if d, ok := earliestHint(anthropicReq, anthropicReqOK, anthropicTok, anthropicTokOK); ok {
		return d
	}
	return 0
}

// usableHint enforces the bounds every parse path must satisfy.
func usableHint(d time.Duration, ok bool) (time.Duration, bool) {
	if !ok || d <= 0 || d > maxProviderRetryAfterHint {
		return 0, false
	}
	return d, true
}

// earliestHint picks the smallest usable candidate from a pair of parses.
// Both candidates are bounds-checked first, so an out-of-range hint falls back
// to its sibling (or to "no hint") instead of leaking through.
func earliestHint(a time.Duration, aOK bool, b time.Duration, bOK bool) (time.Duration, bool) {
	a, aOK = usableHint(a, aOK)
	b, bOK = usableHint(b, bOK)
	if aOK && bOK {
		if b < a {
			return b, true
		}
		return a, true
	}
	if aOK {
		return a, true
	}
	if bOK {
		return b, true
	}
	return 0, false
}

// parseRetryAfter accepts either delta-seconds ("30", "2.5") or an HTTP-date
// (RFC1123 and friends), converting the date into a delta from now.
func parseRetryAfter(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if d, ok := parseSeconds(v); ok {
		return d, true
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	return time.Until(t), true
}

// parseSeconds parses a (possibly fractional) number of seconds.
func parseSeconds(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	secs, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(secs * float64(time.Second)), true
}

// parseDurationOrSeconds accepts a Go duration string first ("6m0s"), then
// falls back to bare seconds ("30").
func parseDurationOrSeconds(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d, true
	}
	return parseSeconds(v)
}

// parseRFC3339Delta converts an absolute RFC3339 timestamp into a delta from
// now. Timestamps in the past yield a non-positive delta, which usableHint
// then rejects.
func parseRFC3339Delta(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return 0, false
	}
	return time.Until(t), true
}
