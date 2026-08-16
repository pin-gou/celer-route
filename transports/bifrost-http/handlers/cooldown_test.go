package handlers

import (
	"strings"
	"testing"

	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/plugins/providercooldown"
	"github.com/valyala/fasthttp"
)

// newCooldownCtx builds a fasthttp.RequestCtx with optional user values set,
// mimicking what the router does for path params.
func newCooldownCtx(userValues map[string]string) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	for k, v := range userValues {
		ctx.SetUserValue(k, v)
	}
	return ctx
}

// newCooldownHandlerWithPlugin builds a CooldownHandler whose resolver always
// returns the given plugin instance. Optional keyName resolvers are forwarded.
func newCooldownHandlerWithPlugin(p *providercooldown.CooldownPlugin, keyNameResolvers ...KeyNameResolver) *CooldownHandler {
	return NewCooldownHandler(func() *providercooldown.CooldownPlugin { return p }, keyNameResolvers...)
}

// -----------------------------------------------------------------------------
// getState (GET /api/plugins/provider-cooldown/state)
// -----------------------------------------------------------------------------

func TestGetCooldownState_PluginNotLoaded(t *testing.T) {
	h := NewCooldownHandler(func() *providercooldown.CooldownPlugin { return nil })
	ctx := newCooldownCtx(nil)
	h.getState(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusBadRequest {
		t.Fatalf("expected 400 when plugin not loaded, got %d", got)
	}
	if !strings.Contains(string(ctx.Response.Body()), "provider-cooldown plugin is not loaded") {
		t.Fatalf("expected plugin-not-loaded message, got %s", ctx.Response.Body())
	}
}

func TestGetCooldownState_Empty(t *testing.T) {
	plugin := providercooldown.NewPlugin(nil)
	defer plugin.Cleanup()
	h := newCooldownHandlerWithPlugin(plugin)

	ctx := newCooldownCtx(nil)
	h.getState(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", got, ctx.Response.Body())
	}
	body := string(ctx.Response.Body())
	if !strings.Contains(body, `"count":0`) {
		t.Fatalf("expected count:0 in body, got %s", body)
	}
	if !strings.Contains(body, `"entries":[]`) {
		t.Fatalf("expected empty entries array in body, got %s", body)
	}
}

func TestGetCooldownState_WithEntries(t *testing.T) {
	plugin := providercooldown.NewPlugin(nil)
	defer plugin.Cleanup()
	plugin.State.Mark(schemas.OpenAI, "key-a")
	plugin.State.Mark(schemas.Anthropic, "key-b")

	h := newCooldownHandlerWithPlugin(plugin, func(provider schemas.ModelProvider, keyID string) string {
		switch keyID {
		case "key-a":
			return "prod-openai-key"
		case "key-b":
			return "prod-anthropic-key"
		}
		return ""
	})
	ctx := newCooldownCtx(nil)
	h.getState(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", got, ctx.Response.Body())
	}
	body := string(ctx.Response.Body())
	if !strings.Contains(body, `"count":2`) {
		t.Fatalf("expected count:2, got %s", body)
	}
	for _, want := range []string{
		`"provider":"openai"`, `"key_id":"key-a"`, `"key_name":"prod-openai-key"`,
		`"provider":"anthropic"`, `"key_id":"key-b"`, `"key_name":"prod-anthropic-key"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in body, got %s", want, body)
		}
	}
}

// TestGetCooldownState_KeyNameOmittedWithoutResolver pins that entries carry
// no key_name field when no KeyNameResolver is wired (backward compatibility).
func TestGetCooldownState_KeyNameOmittedWithoutResolver(t *testing.T) {
	plugin := providercooldown.NewPlugin(nil)
	defer plugin.Cleanup()
	plugin.State.Mark(schemas.OpenAI, "key-a")

	h := newCooldownHandlerWithPlugin(plugin)
	ctx := newCooldownCtx(nil)
	h.getState(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", got, ctx.Response.Body())
	}
	body := string(ctx.Response.Body())
	if strings.Contains(body, `"key_name"`) {
		t.Fatalf("expected no key_name field without a resolver, got %s", body)
	}
}

// -----------------------------------------------------------------------------
// clearKey (DELETE /api/plugins/provider-cooldown/state/{provider}/{keyId})
// -----------------------------------------------------------------------------

func TestClearCooldownKey_OK(t *testing.T) {
	plugin := providercooldown.NewPlugin(nil)
	defer plugin.Cleanup()
	plugin.State.Mark(schemas.OpenAI, "key-a")

	h := newCooldownHandlerWithPlugin(plugin)
	ctx := newCooldownCtx(map[string]string{"provider": "openai", "keyId": "key-a"})
	h.clearKey(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", got, ctx.Response.Body())
	}
	if plugin.State.IsCoolingDown(schemas.OpenAI, "key-a") {
		t.Fatal("key must no longer be cooling down after clear")
	}
}

func TestClearCooldownKey_NotFound(t *testing.T) {
	plugin := providercooldown.NewPlugin(nil)
	defer plugin.Cleanup()

	h := newCooldownHandlerWithPlugin(plugin)
	ctx := newCooldownCtx(map[string]string{"provider": "openai", "keyId": "no-such-key"})
	h.clearKey(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusNotFound {
		t.Fatalf("expected 404 for non-existent entry, got %d", got)
	}
}

func TestClearCooldownKey_PluginNotLoaded(t *testing.T) {
	h := NewCooldownHandler(func() *providercooldown.CooldownPlugin { return nil })
	ctx := newCooldownCtx(map[string]string{"provider": "openai", "keyId": "key-a"})
	h.clearKey(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusBadRequest {
		t.Fatalf("expected 400 when plugin not loaded, got %d", got)
	}
}

func TestClearCooldownKey_MissingProvider(t *testing.T) {
	plugin := providercooldown.NewPlugin(nil)
	defer plugin.Cleanup()
	h := newCooldownHandlerWithPlugin(plugin)

	ctx := newCooldownCtx(map[string]string{"keyId": "key-a"})
	h.clearKey(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusBadRequest {
		t.Fatalf("expected 400 for missing provider, got %d", got)
	}
}

func TestClearCooldownKey_MissingKeyID(t *testing.T) {
	plugin := providercooldown.NewPlugin(nil)
	defer plugin.Cleanup()
	h := newCooldownHandlerWithPlugin(plugin)

	ctx := newCooldownCtx(map[string]string{"provider": "openai"})
	h.clearKey(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusBadRequest {
		t.Fatalf("expected 400 for missing keyId, got %d", got)
	}
}

// TestClearCooldownKey_ResolverHonorsReload verifies the resolver pattern:
// a new plugin instance (post-reload) with fresh state returns 404 for a key
// that was cooled down under the OLD instance. This pins the behavior that
// the handler never holds a stale pointer.
func TestClearCooldownKey_ResolverHonorsReload(t *testing.T) {
	oldPlugin := providercooldown.NewPlugin(nil)
	defer oldPlugin.Cleanup()
	oldPlugin.State.Mark(schemas.OpenAI, "key-a")

	newPlugin := providercooldown.NewPlugin(nil)
	defer newPlugin.Cleanup()
	// newPlugin has a fresh (empty) State — simulates reload.

	// Resolver returns the NEW plugin (what the transport would do).
	h := newCooldownHandlerWithPlugin(newPlugin)
	ctx := newCooldownCtx(map[string]string{"provider": "openai", "keyId": "key-a"})
	h.clearKey(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusNotFound {
		t.Fatalf("expected 404 because the new plugin's state is empty, got %d", got)
	}
	// The OLD plugin's state is untouched.
	if !oldPlugin.State.IsCoolingDown(schemas.OpenAI, "key-a") {
		t.Fatal("old plugin state should be unaffected by operations on the new instance")
	}
}
