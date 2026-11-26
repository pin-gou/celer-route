package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/router"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// newProviderUsageTestStore builds an in-memory-ish SQLite log store backed by
// a temp file, so the handler test exercises the real aggregation query.
func newProviderUsageTestStore(t *testing.T) logstore.LogStore {
	t.Helper()

	store, err := logstore.NewLogStore(context.Background(), &logstore.Config{
		Enabled: true,
		Type:    logstore.LogStoreTypeSQLite,
		Config: &logstore.SQLiteConfig{
			Path: filepath.Join(t.TempDir(), "provider-usage.db"),
		},
	}, &mockLogger{})
	if err != nil {
		t.Fatalf("NewLogStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	return store
}

func newProviderUsageHandler(store logstore.LogStore) *ProviderHandler {
	return &ProviderHandler{inMemoryStore: &lib.Config{LogsStore: store}}
}

// callProviderUsage invokes the handler with the router-style user value the
// real router sets from the path parameter.
func callProviderUsage(h *ProviderHandler, provider, query string) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	// Wire ctx.s = fakeServer: a zero-value RequestCtx has a nil server, and
	// database/sql calls ctx.Done() (which dereferences ctx.s) while running
	// the aggregation query.
	ctx.Init(&fasthttp.Request{}, nil, nil)
	uri := "/api/providers/" + provider + "/usage"
	if query != "" {
		uri += "?" + query
	}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI(uri)
	ctx.SetUserValue("provider", provider)
	h.getProviderUsage(ctx)
	return ctx
}

func TestProviderUsageRejectsInvalidWindowMinutes(t *testing.T) {
	SetLogger(&mockLogger{})

	h := newProviderUsageHandler(newProviderUsageTestStore(t))

	for _, raw := range []string{"0", "4", "1441", "5000", "abc", "1.5"} {
		t.Run("window_minutes="+raw, func(t *testing.T) {
			ctx := callProviderUsage(h, "sensenova", "window_minutes="+raw)
			if got := ctx.Response.StatusCode(); got != fasthttp.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", got, ctx.Response.Body())
			}
			var bifrostErr map[string]any
			if err := json.Unmarshal(ctx.Response.Body(), &bifrostErr); err != nil {
				t.Fatalf("failed to unmarshal error response: %v", err)
			}
			if !strings.Contains(string(ctx.Response.Body()), "window_minutes") {
				t.Fatalf("expected error message to mention window_minutes, got %s", ctx.Response.Body())
			}
		})
	}
}

func TestProviderUsageReturnsAggregates(t *testing.T) {
	SetLogger(&mockLogger{})

	store := newProviderUsageTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	// Rows inside the 60-minute window.
	rows := []*logstore.Log{
		{
			ID: "usage-h1", Timestamp: now.Add(-10 * time.Minute), Object: "chat.completion",
			Provider: "sensenova", Model: "deepseek-flash", SelectedKeyID: "key-1", SelectedKeyName: "simon",
			Status: "success", PromptTokens: 1000, CompletionTokens: 100, CachedReadTokens: 600, TotalTokens: 1100,
		},
		{
			ID: "usage-h2", Timestamp: now.Add(-5 * time.Minute), Object: "chat.completion",
			Provider: "sensenova", Model: "deepseek-flash", SelectedKeyID: "key-1", SelectedKeyName: "simon",
			Status: "error", PromptTokens: 200, CompletionTokens: 20, CachedReadTokens: 0, TotalTokens: 220,
		},
		{
			ID: "usage-h3", Timestamp: now.Add(-3 * time.Minute), Object: "chat.completion",
			Provider: "sensenova", Model: "deepseek-flash", SelectedKeyID: "", SelectedKeyName: "",
			Status: "success", PromptTokens: 10, CompletionTokens: 1, CachedReadTokens: 0, TotalTokens: 11,
		},
		// Other provider, must be excluded.
		{
			ID: "usage-h4", Timestamp: now.Add(-2 * time.Minute), Object: "chat.completion",
			Provider: "openai", Model: "gpt-4o", SelectedKeyID: "key-x", SelectedKeyName: "other",
			Status: "success", PromptTokens: 1, CompletionTokens: 1, CachedReadTokens: 0, TotalTokens: 2,
		},
		// Outside the window, must be excluded.
		{
			ID: "usage-h5", Timestamp: now.Add(-2 * time.Hour), Object: "chat.completion",
			Provider: "sensenova", Model: "deepseek-flash", SelectedKeyID: "key-1", SelectedKeyName: "simon",
			Status: "success", PromptTokens: 9, CompletionTokens: 9, CachedReadTokens: 0, TotalTokens: 99,
		},
	}
	for _, row := range rows {
		if err := store.Create(ctx, row); err != nil {
			t.Fatalf("Create(%s) error = %v", row.ID, err)
		}
	}

	h := newProviderUsageHandler(store)
	responseCtx := callProviderUsage(h, "sensenova", "window_minutes=60")

	if got := responseCtx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", got, responseCtx.Response.Body())
	}
	t.Logf("GET /api/providers/sensenova/usage?window_minutes=60 -> %s", responseCtx.Response.Body())

	// Wire-shape check: every contract field must be present with its exact name.
	var decoded map[string]any
	if err := json.Unmarshal(responseCtx.Response.Body(), &decoded); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	for _, key := range []string{"provider", "window_minutes", "start_time", "end_time", "entries"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("response is missing %q: %s", key, responseCtx.Response.Body())
		}
	}
	if decoded["provider"] != "sensenova" {
		t.Fatalf("expected provider=sensenova, got %v", decoded["provider"])
	}
	if decoded["window_minutes"] != float64(60) {
		t.Fatalf("expected window_minutes=60, got %v", decoded["window_minutes"])
	}
	if !strings.HasSuffix(decoded["start_time"].(string), "Z") || !strings.HasSuffix(decoded["end_time"].(string), "Z") {
		t.Fatalf("expected UTC RFC3339 timestamps, got %v / %v", decoded["start_time"], decoded["end_time"])
	}

	entries, ok := decoded["entries"].([]any)
	if !ok {
		t.Fatalf("entries is not an array: %v", decoded["entries"])
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 groups, got %d: %s", len(entries), responseCtx.Response.Body())
	}
	first, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("entry is not an object: %v", entries[0])
	}
	for _, key := range []string{
		"model", "key_id", "key_name", "requests", "successes", "errors", "cancelled",
		"prompt_tokens", "completion_tokens", "cached_read_tokens", "total_tokens",
		"avg_total_tokens", "requests_per_minute", "tokens_per_minute",
	} {
		if _, ok := first[key]; !ok {
			t.Fatalf("entry is missing %q: %v", key, first)
		}
	}

	// Highest total_tokens first: key-1 (1320) then the keyless group (11).
	if first["model"] != "deepseek-flash" || first["key_id"] != "key-1" || first["key_name"] != "simon" {
		t.Fatalf("unexpected first entry: %v", first)
	}
	if first["requests"] != float64(2) {
		t.Fatalf("expected 2 requests, got %v", first["requests"])
	}
	if first["successes"] != float64(1) || first["errors"] != float64(1) || first["cancelled"] != float64(0) {
		t.Fatalf("unexpected status counters: %v", first)
	}
	if first["prompt_tokens"] != float64(1200) || first["completion_tokens"] != float64(120) {
		t.Fatalf("unexpected token sums: %v", first)
	}
	if first["cached_read_tokens"] != float64(600) || first["total_tokens"] != float64(1320) {
		t.Fatalf("unexpected token sums: %v", first)
	}
	if first["avg_total_tokens"] != float64(660) {
		t.Fatalf("expected avg_total_tokens=660, got %v", first["avg_total_tokens"])
	}
	if first["requests_per_minute"] != float64(2)/60.0 {
		t.Fatalf("expected requests_per_minute=2/60, got %v", first["requests_per_minute"])
	}
	if first["tokens_per_minute"] != float64(1320)/60.0 {
		t.Fatalf("expected tokens_per_minute=1320/60, got %v", first["tokens_per_minute"])
	}

	keyless, ok := entries[1].(map[string]any)
	if !ok {
		t.Fatalf("entry is not an object: %v", entries[1])
	}
	if keyless["key_id"] != "" || keyless["key_name"] != "" {
		t.Fatalf("expected empty key id/name for keyless group, got %v", keyless)
	}
	if keyless["requests"] != float64(1) || keyless["total_tokens"] != float64(11) {
		t.Fatalf("unexpected keyless entry: %v", keyless)
	}
}

func TestProviderUsageDefaultsToSixtyMinutes(t *testing.T) {
	SetLogger(&mockLogger{})

	h := newProviderUsageHandler(newProviderUsageTestStore(t))
	responseCtx := callProviderUsage(h, "sensenova", "")

	if got := responseCtx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", got, responseCtx.Response.Body())
	}

	var decoded struct {
		WindowMinutes int    `json:"window_minutes"`
		StartTime     string `json:"start_time"`
		EndTime       string `json:"end_time"`
		Entries       []any  `json:"entries"`
	}
	if err := json.Unmarshal(responseCtx.Response.Body(), &decoded); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if decoded.WindowMinutes != 60 {
		t.Fatalf("expected default window_minutes=60, got %d", decoded.WindowMinutes)
	}
	if decoded.Entries == nil {
		t.Fatalf("expected entries to be [] not null: %s", responseCtx.Response.Body())
	}
	if len(decoded.Entries) != 0 {
		t.Fatalf("expected empty entries, got %v", decoded.Entries)
	}

	start, err := time.Parse(time.RFC3339, decoded.StartTime)
	if err != nil {
		t.Fatalf("start_time is not RFC3339: %v", err)
	}
	end, err := time.Parse(time.RFC3339, decoded.EndTime)
	if err != nil {
		t.Fatalf("end_time is not RFC3339: %v", err)
	}
	if end.Sub(start) != time.Hour {
		t.Fatalf("expected a 60-minute window, got %v", end.Sub(start))
	}
}

// TestProviderUsageRouteIsRegistered guards the route wiring: the handler is
// only reachable if RegisterRoutes exposes GET /api/providers/{provider}/usage
// without colliding with the existing /api/providers/{provider} route.
func TestProviderUsageRouteIsRegistered(t *testing.T) {
	SetLogger(&mockLogger{})

	h := newProviderUsageHandler(newProviderUsageTestStore(t))
	r := router.New()
	h.RegisterRoutes(r)

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/providers/sensenova/usage?window_minutes=60")
	r.Handler(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected 200 from the registered route, got %d: %s", got, ctx.Response.Body())
	}
	var decoded struct {
		Provider      string `json:"provider"`
		WindowMinutes int    `json:"window_minutes"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &decoded); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if decoded.Provider != "sensenova" || decoded.WindowMinutes != 60 {
		t.Fatalf("unexpected response: %s", ctx.Response.Body())
	}
}

func TestProviderUsageServiceUnavailableWithoutLogStore(t *testing.T) {
	SetLogger(&mockLogger{})

	// LogsStore nil.
	h := &ProviderHandler{inMemoryStore: &lib.Config{}}
	responseCtx := callProviderUsage(h, "sensenova", "")
	if got := responseCtx.Response.StatusCode(); got != fasthttp.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", got, responseCtx.Response.Body())
	}
	if !strings.Contains(string(responseCtx.Response.Body()), "Logs store") {
		t.Fatalf("expected error message to mention the logs store, got %s", responseCtx.Response.Body())
	}

	// inMemoryStore nil entirely.
	h = &ProviderHandler{}
	responseCtx = callProviderUsage(h, "sensenova", "")
	if got := responseCtx.Response.StatusCode(); got != fasthttp.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", got, responseCtx.Response.Body())
	}
}
