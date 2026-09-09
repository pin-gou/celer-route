package opencode

import (
	"regexp"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

const sessionHeaderRegex = `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`

// newBareProvider returns a fresh bare `opencode` (keyless) provider for
// header-injection tests. The networkConfig is the only field the helper
// touches, so a zero-valued struct is fine.
func newBareProvider(t *testing.T) *opencodeProvider {
	t.Helper()
	cfg := &schemas.ProviderConfig{}
	cfg.CheckAndSetDefaults()
	p, err := NewOpencodeProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewOpencodeProvider: %v", err)
	}
	return p
}

// newPaidProvider returns a fresh OpencodeZen provider so we can assert that
// free-tier injection is suppressed when the provider key is not bare
// `opencode`.
func newPaidProvider(t *testing.T) *opencodeProvider {
	t.Helper()
	cfg := &schemas.ProviderConfig{}
	cfg.CheckAndSetDefaults()
	p, err := NewOpencodeZenProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewOpencodeZenProvider: %v", err)
	}
	return p
}

func TestApplyFreeTierHeaders_BareProviderFreeModel_Injects(t *testing.T) {
	p := newBareProvider(t)
	headers := map[string]string{}
	prepared := &schemas.BifrostChatRequest{Model: "big-pickle"}
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	p.applyFreeTierHeaders(ctx, headers, prepared)

	if got, want := headers["Authorization"], "Bearer public"; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
	sessionID := headers["x-opencode-session"]
	if sessionID == "" {
		t.Fatalf("x-opencode-session was not set")
	}
	if matched, _ := regexp.MatchString(sessionHeaderRegex, sessionID); !matched {
		t.Errorf("x-opencode-session %q does not match opencode client format %s", sessionID, sessionHeaderRegex)
	}
	if got, want := headers["x-opencode-client"], "celer-route"; got != want {
		t.Errorf("x-opencode-client = %q, want %q", got, want)
	}
	if got, want := headers["User-Agent"], opencodeFreeTierUserAgent; got != want {
		t.Errorf("User-Agent = %q, want %q (free-tier gate requires an opencode UA)", got, want)
	}
}

func TestApplyFreeTierHeaders_BareProviderFreeSuffixModel_Injects(t *testing.T) {
	p := newBareProvider(t)
	headers := map[string]string{}
	prepared := &schemas.BifrostChatRequest{Model: "some-experimental-free"}
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	p.applyFreeTierHeaders(ctx, headers, prepared)

	for _, h := range []string{"x-opencode-session", "Authorization", "User-Agent"} {
		if _, ok := headers[h]; !ok {
			t.Errorf("header %q was not set for -free suffixed model", h)
		}
	}
}

func TestApplyFreeTierHeaders_BareProviderPaidModel_NoInject(t *testing.T) {
	p := newBareProvider(t)
	headers := map[string]string{}
	prepared := &schemas.BifrostChatRequest{Model: "gpt-5"}
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	p.applyFreeTierHeaders(ctx, headers, prepared)

	for _, h := range []string{"Authorization", "x-opencode-session", "x-opencode-client", "User-Agent"} {
		if _, ok := headers[h]; ok {
			t.Errorf("header %q should not be set for paid model on keyless provider, got %q", h, headers[h])
		}
	}
}

func TestApplyFreeTierHeaders_ZenProviderFreeModel_NoInject(t *testing.T) {
	p := newPaidProvider(t)
	headers := map[string]string{}
	prepared := &schemas.BifrostChatRequest{Model: "big-pickle"}
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	p.applyFreeTierHeaders(ctx, headers, prepared)

	for _, h := range []string{"Authorization", "x-opencode-session", "x-opencode-client", "User-Agent"} {
		if _, ok := headers[h]; ok {
			t.Errorf("header %q should not be set when provider key is OpencodeZen (paid tier), got %q", h, headers[h])
		}
	}
}

func TestApplyFreeTierHeaders_CallerOverridesRespected(t *testing.T) {
	p := newBareProvider(t)
	headers := map[string]string{
		"Authorization":      "Bearer my-real-key",
		"x-opencode-session": "ses_user_supplied_session",
		"x-opencode-client":  "my-custom-client",
		"User-Agent":         "my-custom-ua/1.0",
	}
	prepared := &schemas.BifrostChatRequest{Model: "big-pickle"}
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	p.applyFreeTierHeaders(ctx, headers, prepared)

	for header, want := range map[string]string{
		"Authorization":      "Bearer my-real-key",
		"x-opencode-session": "ses_user_supplied_session",
		"x-opencode-client":  "my-custom-client",
		"User-Agent":         "my-custom-ua/1.0",
	} {
		if got := headers[header]; got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestApplyFreeTierHeaders_NetworkConfigExtraHeadersUA_Respected(t *testing.T) {
	p := newBareProvider(t)
	p.networkConfig.ExtraHeaders = map[string]string{"user-agent": "corporate-gateway/2.0"}
	headers := map[string]string{}
	prepared := &schemas.BifrostChatRequest{Model: "big-pickle"}
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	p.applyFreeTierHeaders(ctx, headers, prepared)

	if got := headers["User-Agent"]; got != "" {
		t.Errorf("User-Agent should not be injected when NetworkConfig.ExtraHeaders sets one, got %q", got)
	}
	if got := p.networkConfig.ExtraHeaders["user-agent"]; got != "corporate-gateway/2.0" {
		t.Errorf("NetworkConfig.ExtraHeaders user-agent = %q, want corporate-gateway/2.0", got)
	}
}

func TestApplyFreeTierHeaders_CtxExtraHeadersUA_Respected(t *testing.T) {
	p := newBareProvider(t)
	headers := map[string]string{}
	prepared := &schemas.BifrostChatRequest{Model: "big-pickle"}
	ctx := schemas.NewBifrostContext(nil, time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, map[string][]string{"User-Agent": {"ctx-ua/3.0"}})

	p.applyFreeTierHeaders(ctx, headers, prepared)

	if got := headers["User-Agent"]; got != "" {
		t.Errorf("User-Agent should not be injected when ctx ExtraHeaders sets one, got %q", got)
	}
}

func TestApplyFreeTierHeaders_NilRequest_NoInject(t *testing.T) {
	p := newBareProvider(t)
	headers := map[string]string{}
	ctx := schemas.NewBifrostContext(nil, time.Time{})

	p.applyFreeTierHeaders(ctx, headers, nil)

	if len(headers) != 0 {
		t.Errorf("expected empty headers for nil request, got %v", headers)
	}
}

func TestApplyFreeTierHeaders_SessionIDRotates(t *testing.T) {
	p := newBareProvider(t)
	prepared := &schemas.BifrostChatRequest{Model: "big-pickle"}

	const n = 8
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		headers := map[string]string{}
		p.applyFreeTierHeaders(schemas.NewBifrostContext(nil, time.Time{}), headers, prepared)
		id := headers["x-opencode-session"]
		if id == "" {
			t.Fatalf("iteration %d: missing x-opencode-session", i)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("iteration %d: duplicate session id %q", i, id)
		}
		seen[id] = struct{}{}
	}
}