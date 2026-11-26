package utils

import (
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
)

// TestHandleProviderAPIError_CapturesProviderResponseHeaders pins that every
// return branch of HandleProviderAPIError carries the provider's response
// headers (filtered) so downstream consumers — provider-cooldown's
// retry_after_mode — can read Retry-After / x-ratelimit-reset-* on the error
// path. The five subtests map 1:1 to the five return branches.
func TestHandleProviderAPIError_CapturesProviderResponseHeaders(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        []byte
		contentType string
		encoding    string
	}{
		{
			name:        "Decode failure",
			statusCode:  500,
			body:        []byte{0xFF, 0xFE}, // invalid gzip stream
			contentType: "application/json",
			encoding:    "gzip",
		},
		{
			name:        "Empty response",
			statusCode:  502,
			body:        []byte(""),
			contentType: "application/json",
		},
		{
			name:        "Valid JSON error",
			statusCode:  429,
			body:        []byte(`{"error": {"message": "rate limited"}}`),
			contentType: "application/json",
		},
		{
			name:        "HTML error response",
			statusCode:  503,
			body:        []byte(`<html><body>Service Unavailable</body></html>`),
			contentType: "text/html",
		},
		{
			name:        "Unparseable non-HTML response",
			statusCode:  400,
			body:        []byte(`not json, not html`),
			contentType: "text/plain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &fasthttp.Response{}
			resp.SetStatusCode(tt.statusCode)
			resp.Header.Set("Content-Type", tt.contentType)
			if tt.encoding != "" {
				resp.Header.Set("Content-Encoding", tt.encoding)
			}
			resp.Header.Set("Retry-After", "30")
			resp.Header.Set("x-ratelimit-reset-tokens", "1s")
			resp.Header.Set("X-Request-Id", "req-abc")
			// Sensitive / transport-level headers must never leak through.
			resp.Header.Set("Authorization", "Bearer super-secret")
			resp.Header.Set("Set-Cookie", "session=abc")
			resp.Header.Set("Content-Length", "42")
			resp.SetBody(tt.body)

			var errorResp map[string]interface{}
			bifrostErr := HandleProviderAPIError(resp, &errorResp)
			if bifrostErr == nil {
				t.Fatal("HandleProviderAPIError() returned nil")
			}

			headers := bifrostErr.ExtraFields.ProviderResponseHeaders
			if headers == nil {
				t.Fatalf("%s: ProviderResponseHeaders is nil, want captured headers", tt.name)
			}
			// fasthttp canonicalizes header names (e.g. x-ratelimit-reset-tokens
			// becomes X-Ratelimit-Reset-Tokens), so compare case-insensitively.
			lower := make(map[string]string, len(headers))
			for k, v := range headers {
				lower[strings.ToLower(k)] = v
			}
			for key, want := range map[string]string{
				"retry-after":              "30",
				"x-ratelimit-reset-tokens": "1s",
				"x-request-id":             "req-abc",
			} {
				if got := lower[key]; got != want {
					t.Errorf("%s: headers[%q] = %q, want %q", tt.name, key, got, want)
				}
			}
			for _, key := range []string{"authorization", "set-cookie", "content-length", "content-type"} {
				if got, found := lower[key]; found {
					t.Errorf("%s: header %q must be filtered out, got %q", tt.name, key, got)
				}
			}
		})
	}
}
