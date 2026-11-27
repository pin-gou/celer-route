package bifrost

import "testing"

// TestShouldSkipRetryBackoff pins the full truth table of the retry backoff
// decision. The only case that skips the sleep on a key-bound failure is a
// genuine rotation to a different key; same-key retries and transient
// (5xx / network) retries must keep backing off.
func TestShouldSkipRetryBackoff(t *testing.T) {
	tests := []struct {
		name                  string
		keyChanged            bool
		perKeyFailure         bool
		encryptedContentStrip bool
		want                  bool
	}{
		{
			name: "same-key transient failure keeps backoff",
			// 5xx / network error retried on the same key.
			keyChanged:    false,
			perKeyFailure: false,
			want:          false,
		},
		{
			name: "key changed but failure is not key bound keeps backoff",
			// e.g. a transient upstream error that happened to be served by a
			// different key than the previous attempt.
			keyChanged:    true,
			perKeyFailure: false,
			want:          false,
		},
		{
			name: "per-key failure without rotation keeps backoff",
			// 429 pool reset re-picked the same key: its rate-limit window still
			// has to slide. Also covers keyless providers (keyChanged is false).
			keyChanged:    false,
			perKeyFailure: true,
			want:          false,
		},
		{
			name: "key-bound failure with rotation skips backoff",
			// 429/401/402/403: the freshly selected key has its own quota and its
			// own credentials, so waiting buys nothing.
			keyChanged:    true,
			perKeyFailure: true,
			want:          true,
		},
		{
			name: "encrypted content strip skips backoff",
			// The payload changed, so there is nothing to wait out.
			keyChanged:            false,
			perKeyFailure:         false,
			encryptedContentStrip: true,
			want:                  true,
		},
		{
			name:                  "encrypted content strip with rotation skips backoff",
			keyChanged:            true,
			perKeyFailure:         true,
			encryptedContentStrip: true,
			want:                  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipRetryBackoff(tt.keyChanged, tt.perKeyFailure, tt.encryptedContentStrip)
			if got != tt.want {
				t.Fatalf("shouldSkipRetryBackoff(keyChanged=%t, perKeyFailure=%t, encryptedContentStrip=%t) = %t, want %t",
					tt.keyChanged, tt.perKeyFailure, tt.encryptedContentStrip, got, tt.want)
			}
		})
	}
}
