package providercooldown

import (
	"net/http"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestProviderRetryAfterHint(t *testing.T) {
	now := time.Now().UTC()
	httpDateIn30s := now.Add(30 * time.Second).Format(http.TimeFormat)
	httpDateInPast := now.Add(-time.Minute).Format(http.TimeFormat)
	rfc3339In45s := now.Add(45 * time.Second).Format(time.RFC3339)

	const tolerance = 5 * time.Second

	tests := []struct {
		name    string
		headers map[string]string
		want    time.Duration
		// approx compares want against the actual value with a tolerance,
		// for hints derived from absolute timestamps (whole-second truncation).
		approx bool
	}{
		{
			name:    "Retry-After integer seconds",
			headers: map[string]string{"Retry-After": "30"},
			want:    30 * time.Second,
		},
		{
			name:    "Retry-After HTTP-date in the future",
			headers: map[string]string{"Retry-After": httpDateIn30s},
			want:    30 * time.Second,
			approx:  true,
		},
		{
			name:    "Retry-After zero is not a hint",
			headers: map[string]string{"Retry-After": "0"},
			want:    0,
		},
		{
			name:    "Retry-After garbage is not a hint",
			headers: map[string]string{"Retry-After": "garbage"},
			want:    0,
		},
		{
			name:    "Retry-After HTTP-date in the past is not a hint",
			headers: map[string]string{"Retry-After": httpDateInPast},
			want:    0,
		},
		{
			name:    "x-ratelimit-reset-after accepts fractions",
			headers: map[string]string{"x-ratelimit-reset-after": "2.5"},
			want:    2500 * time.Millisecond,
		},
		{
			name:    "x-ratelimit-reset-tokens parses a Go duration",
			headers: map[string]string{"x-ratelimit-reset-tokens": "6m0s"},
			want:    6 * time.Minute,
		},
		{
			name:    "x-ratelimit-reset-requests takes the earliest of the pair",
			headers: map[string]string{"x-ratelimit-reset-requests": "30s", "x-ratelimit-reset-tokens": "6m0s"},
			want:    30 * time.Second,
		},
		{
			name:    "anthropic RFC3339 timestamp",
			headers: map[string]string{"anthropic-ratelimit-tokens-reset": rfc3339In45s},
			want:    45 * time.Second,
			approx:  true,
		},
		{
			name:    "header names are case-insensitive",
			headers: map[string]string{"rEtRy-AfTeR": "10", "X-RaTeLiMiT-ReSeT-AfTeR": "1"},
			want:    10 * time.Second,
		},
		{
			name:    "empty map",
			headers: map[string]string{},
			want:    0,
		},
		{
			name:    "nil map",
			headers: nil,
			want:    0,
		},
		{
			name:    "hint above 24h is rejected",
			headers: map[string]string{"Retry-After": "90000"},
			want:    0,
		},
		{
			name:    "anthropic timestamp above 24h is rejected",
			headers: map[string]string{"anthropic-ratelimit-requests-reset": now.Add(48 * time.Hour).Format(time.RFC3339)},
			want:    0,
		},
		{
			name:    "unparseable values are ignored, later sources still win",
			headers: map[string]string{"Retry-After": "soon", "x-ratelimit-reset-after": "3"},
			want:    3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerRetryAfterHint(tt.headers)
			if tt.approx {
				if got <= 0 || got < tt.want-tolerance || got > tt.want+tolerance {
					t.Fatalf("providerRetryAfterHint() = %v, want ~%v", got, tt.want)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("providerRetryAfterHint() = %v, want %v", got, tt.want)
			}
		})
	}
}

// retryModePolicy builds a single-rule policy whose only match is the message
// "rate limited", so tests can isolate the retry-after mode behaviour.
func retryModePolicy(ttlSeconds int, mode string) *schemas.CooldownPolicy {
	return &schemas.CooldownPolicy{
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode:      "any",
			TTLSeconds:     ttlSeconds,
			RetryAfterMode: mode,
			Match:          []schemas.CooldownPolicyMatch{{MessageContains: []string{"rate limited"}}},
		},
	}
}

// TestClassifyForMarker_RetryAfterModes pins the end-to-end TTL selection for
// the three modes through classifyForMarker (the marker path the retry loop
// uses), including the no-hint fallback in each mode.
func TestClassifyForMarker_RetryAfterModes(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		ttlSeconds int
		headers    map[string]string
		wantTTL    time.Duration
		wantKind   CooldownKind
		wantOK     bool
	}{
		{
			name:       "off (empty) uses ttl_seconds and ignores the hint",
			mode:       "",
			ttlSeconds: 60,
			headers:    map[string]string{"Retry-After": "30"},
			wantTTL:    60 * time.Second,
			wantKind:   CooldownKindRateLimit,
			wantOK:     true,
		},
		{
			name:       "off explicit also ignores the hint",
			mode:       "off",
			ttlSeconds: 60,
			headers:    map[string]string{"Retry-After": "30"},
			wantTTL:    60 * time.Second,
			wantKind:   CooldownKindRateLimit,
			wantOK:     true,
		},
		{
			name:       "cap takes the smaller hint",
			mode:       "cap",
			ttlSeconds: 60,
			headers:    map[string]string{"Retry-After": "30"},
			wantTTL:    30 * time.Second,
			wantKind:   CooldownKindRateLimit,
			wantOK:     true,
		},
		{
			name:       "cap keeps ttl_seconds when the hint is larger",
			mode:       "cap",
			ttlSeconds: 60,
			headers:    map[string]string{"Retry-After": "120"},
			wantTTL:    60 * time.Second,
			wantKind:   CooldownKindRateLimit,
			wantOK:     true,
		},
		{
			name:       "cap falls back to ttl_seconds without a hint",
			mode:       "cap",
			ttlSeconds: 60,
			headers:    nil,
			wantTTL:    60 * time.Second,
			wantKind:   CooldownKindRateLimit,
			wantOK:     true,
		},
		{
			name:       "exact uses the hint",
			mode:       "exact",
			ttlSeconds: 60,
			headers:    map[string]string{"Retry-After": "30"},
			wantTTL:    30 * time.Second,
			wantKind:   CooldownKindRateLimit,
			wantOK:     true,
		},
		{
			name:       "exact can extend beyond ttl_seconds",
			mode:       "exact",
			ttlSeconds: 60,
			headers:    map[string]string{"Retry-After": "120"},
			wantTTL:    120 * time.Second,
			wantKind:   CooldownKindRateLimit,
			wantOK:     true,
		},
		{
			name:       "exact falls back to ttl_seconds without a hint",
			mode:       "exact",
			ttlSeconds: 60,
			headers:    nil,
			wantTTL:    60 * time.Second,
			wantKind:   CooldownKindRateLimit,
			wantOK:     true,
		},
		{
			name:       "mode is case-insensitive",
			mode:       "CAP",
			ttlSeconds: 60,
			headers:    map[string]string{"x-ratelimit-reset-tokens": "1s"},
			wantTTL:    1 * time.Second,
			wantKind:   CooldownKindRateLimit,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := makeCtx(t, retryModePolicy(tt.ttlSeconds, tt.mode))
			err := newErrOn(429, "rate_limit_error", "", "rate limited", string(schemas.OpenAI))
			err.ExtraFields.ProviderResponseHeaders = tt.headers

			ttl, keyID, _, kind, scope, ok := classifyForMarker(ctx, schemas.OpenAI, "k1", "key-1", err)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ttl != tt.wantTTL {
				t.Fatalf("ttl = %v, want %v", ttl, tt.wantTTL)
			}
			if kind != tt.wantKind {
				t.Fatalf("kind = %q, want %q", kind, tt.wantKind)
			}
			if scope != schemas.CooldownScopeKey {
				t.Fatalf("scope = %q, want %q", scope, schemas.CooldownScopeKey)
			}
			if keyID != "k1" {
				t.Fatalf("keyID = %q, want k1", keyID)
			}
		})
	}
}

// TestClassifyForMarker_RetryAfterModePerRule pins that quota and rate_limit
// each consult their OWN rule's mode: an "off" quota rule keeps its long TTL
// even when the provider hint is short, while an "exact" sibling rule follows
// the hint.
func TestClassifyForMarker_RetryAfterModePerRule(t *testing.T) {
	policy := &schemas.CooldownPolicy{
		Quota: &schemas.CooldownPolicyRule{
			MatchMode:      "any",
			TTLSeconds:     600,
			RetryAfterMode: "off",
			Match:          []schemas.CooldownPolicyMatch{{MessageContains: []string{"quota exhausted"}}},
		},
		RateLimit: &schemas.CooldownPolicyRule{
			MatchMode:      "any",
			TTLSeconds:     60,
			RetryAfterMode: "exact",
			Match:          []schemas.CooldownPolicyMatch{{MessageContains: []string{"rate limited"}}},
		},
	}

	t.Run("quota rule with off ignores the hint", func(t *testing.T) {
		ctx := makeCtx(t, policy)
		err := newErrOn(429, "quota_exceeded_error", "", "quota exhausted", string(schemas.OpenAI))
		err.ExtraFields.ProviderResponseHeaders = map[string]string{"Retry-After": "30"}

		ttl, _, _, kind, _, ok := classifyForMarker(ctx, schemas.OpenAI, "k1", "key-1", err)
		if !ok || kind != CooldownKindQuota {
			t.Fatalf("expected quota match, got ok=%v kind=%q", ok, kind)
		}
		if want := 600 * time.Second; ttl != want {
			t.Fatalf("quota ttl = %v, want %v (off must ignore the hint)", ttl, want)
		}
	})

	t.Run("rate_limit rule with exact follows the hint", func(t *testing.T) {
		ctx := makeCtx(t, policy)
		err := newErrOn(429, "rate_limit_error", "", "rate limited", string(schemas.OpenAI))
		err.ExtraFields.ProviderResponseHeaders = map[string]string{"Retry-After": "30"}

		ttl, _, _, kind, _, ok := classifyForMarker(ctx, schemas.OpenAI, "k1", "key-1", err)
		if !ok || kind != CooldownKindRateLimit {
			t.Fatalf("expected rate_limit match, got ok=%v kind=%q", ok, kind)
		}
		if want := 30 * time.Second; ttl != want {
			t.Fatalf("rate_limit ttl = %v, want %v (exact must follow the hint)", ttl, want)
		}
	})
}

// TestEffectiveCooldownTTL_NilGuards pins that the helper never panics on the
// defensive branches the marker can reach.
func TestEffectiveCooldownTTL_NilGuards(t *testing.T) {
	if got := effectiveCooldownTTL(nil, &schemas.BifrostError{}); got != 0 {
		t.Fatalf("nil rule = %v, want 0", got)
	}
	rule := &schemas.CooldownPolicyRule{TTLSeconds: 60, RetryAfterMode: "exact"}
	if got := effectiveCooldownTTL(rule, nil); got != time.Minute {
		t.Fatalf("nil error = %v, want 1m", got)
	}
}
