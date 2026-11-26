package bifrost

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/keyinflight"
	schemas "github.com/pin-gou/celer-route/core/schemas"
)

const inflightTestKeyID = "inflight-key"

// inflightRecorder lets an upstream handler record what the gateway's counter
// looked like while the call was in flight.
type inflightRecorder struct {
	mu     sync.Mutex
	values []int
}

func (r *inflightRecorder) record(v int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values = append(r.values, v)
}

func (r *inflightRecorder) all() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int(nil), r.values...)
}

// waitForInflight polls until the limiter reports want for (provider, key), so
// the assertion is not sensitive to the few instructions between a provider
// goroutine closing its channel and running its deferred finalizer.
func waitForInflight(t *testing.T, l *keyinflight.Limiter, provider schemas.ModelProvider, keyID string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := l.InFlight(string(provider), keyID)
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("InFlight(%s, %s) = %d, want %d", provider, keyID, got, want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestMaxConcurrentRequestsPerKey_DisabledLeavesLimiterUntouched pins the
// default-off contract: with the cap unset (0) the gate must not touch the
// limiter at all, so the hot path is identical to before the feature existed.
func TestMaxConcurrentRequestsPerKey_DisabledLeavesLimiterUntouched(t *testing.T) {
	var rec inflightRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The handler runs while the upstream call is in flight; with the cap
		// disabled nothing may be counted.
		rec.record(probeInflight.Load())
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	account := NewMockAccount()
	account.AddProviderWithBaseURL(schemas.OpenAI, 1, 1, server.URL)
	account.configs[schemas.OpenAI].NetworkConfig.MaxRetries = 0
	account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
		{ID: inflightTestKeyID, Value: *schemas.NewSecretVar("sk-inflight"), Models: schemas.WhiteList{"*"}, Weight: 100},
	})

	client, err := Init(context.Background(), schemas.BifrostConfig{
		Account: account,
		Logger:  NewDefaultLogger(schemas.LogLevelError),
	})
	if err != nil {
		t.Fatalf("failed to initialize bifrost: %v", err)
	}
	defer client.Shutdown()
	probeInflight.Store(func() int { return client.keyInflight.InFlight(string(schemas.OpenAI), inflightTestKeyID) })

	ctx := schemas.NewBifrostContext(context.Background(), time.Now().Add(30*time.Second))
	if _, bifrostErr := client.ChatCompletionRequest(ctx, &schemas.BifrostChatRequest{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o-mini",
		Input: []schemas.ChatMessage{
			{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")}},
		},
	}); bifrostErr != nil {
		t.Fatalf("chat completion failed: %s", bifrostErr.Error.Message)
	}

	probed := rec.all()
	if len(probed) != 1 {
		t.Fatalf("upstream handler ran %d times, want 1", len(probed))
	}
	if probed[0] != 0 {
		t.Fatalf("InFlight observed by the upstream handler = %d with the cap disabled, want 0", probed[0])
	}
	if got := client.keyInflight.InFlight(string(schemas.OpenAI), inflightTestKeyID); got != 0 {
		t.Fatalf("InFlight after the request = %d, want 0", got)
	}
	if got := client.keyInflight.InFlight(string(schemas.OpenAI), "never-used"); got != 0 {
		t.Fatalf("InFlight for an unused key = %d, want 0", got)
	}
}

// probeInflight is set by each test to read the live counter from inside an
// upstream handler.
var probeInflight inflightProbe

type inflightProbe struct {
	mu sync.Mutex
	fn func() int
}

func (p *inflightProbe) Store(fn func() int) { p.mu.Lock(); p.fn = fn; p.mu.Unlock() }
func (p *inflightProbe) Load() int {
	p.mu.Lock()
	fn := p.fn
	p.mu.Unlock()
	if fn == nil {
		return 0
	}
	return fn()
}

func TestMaxConcurrentRequestsPerKey_UnaryAcquireRelease(t *testing.T) {
	var rec inflightRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(probeInflight.Load())
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	account := NewMockAccount()
	account.AddProviderWithBaseURL(schemas.OpenAI, 1, 1, server.URL)
	account.configs[schemas.OpenAI].NetworkConfig.MaxRetries = 0
	account.configs[schemas.OpenAI].MaxConcurrentRequestsPerKey = 1
	account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
		{ID: inflightTestKeyID, Value: *schemas.NewSecretVar("sk-inflight"), Models: schemas.WhiteList{"*"}, Weight: 100},
	})

	client, err := Init(context.Background(), schemas.BifrostConfig{
		Account: account,
		Logger:  NewDefaultLogger(schemas.LogLevelError),
	})
	if err != nil {
		t.Fatalf("failed to initialize bifrost: %v", err)
	}
	defer client.Shutdown()
	probeInflight.Store(func() int { return client.keyInflight.InFlight(string(schemas.OpenAI), inflightTestKeyID) })

	ctx := schemas.NewBifrostContext(context.Background(), time.Now().Add(30*time.Second))
	if _, bifrostErr := client.ChatCompletionRequest(ctx, &schemas.BifrostChatRequest{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o-mini",
		Input: []schemas.ChatMessage{
			{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")}},
		},
	}); bifrostErr != nil {
		t.Fatalf("chat completion failed: %s", bifrostErr.Error.Message)
	}

	probed := rec.all()
	if len(probed) != 1 {
		t.Fatalf("upstream handler ran %d times, want 1", len(probed))
	}
	if probed[0] != 1 {
		t.Fatalf("InFlight observed by the upstream handler = %d, want 1 (slot must be held for the whole attempt)", probed[0])
	}
	waitForInflight(t, client.keyInflight, schemas.OpenAI, inflightTestKeyID, 0)
}

// TestMaxConcurrentRequestsPerKey_StreamingReleasedAtStreamEnd is the key
// regression test for the streaming release point: the slot must still be held
// while the stream body is being consumed, and must be gone once the provider
// stream ends. Releasing at channel-return time would fail the first assertion;
// never releasing would fail the second (and leak across requests).
func TestMaxConcurrentRequestsPerKey_StreamingReleasedAtStreamEnd(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n", `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"he"}}]}`)
		if fl != nil {
			fl.Flush()
		}
		startOnce.Do(func() { close(started) })
		<-release
		fmt.Fprintf(w, "data: %s\n\n", `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"llo"}}]}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		fmt.Fprint(w, "data: [DONE]\n\n")
		if fl != nil {
			fl.Flush()
		}
	}))
	defer server.Close()

	account := NewMockAccount()
	account.AddProviderWithBaseURL(schemas.OpenAI, 1, 1, server.URL)
	account.configs[schemas.OpenAI].NetworkConfig.MaxRetries = 0
	account.configs[schemas.OpenAI].MaxConcurrentRequestsPerKey = 2
	account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
		{ID: inflightTestKeyID, Value: *schemas.NewSecretVar("sk-inflight"), Models: schemas.WhiteList{"*"}, Weight: 100},
	})

	client, err := Init(context.Background(), schemas.BifrostConfig{
		Account: account,
		Logger:  NewDefaultLogger(schemas.LogLevelError),
	})
	if err != nil {
		t.Fatalf("failed to initialize bifrost: %v", err)
	}
	defer client.Shutdown()

	ctx := schemas.NewBifrostContext(context.Background(), time.Now().Add(30*time.Second))
	stream, bifrostErr := client.ChatCompletionStreamRequest(ctx, &schemas.BifrostChatRequest{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o-mini",
		Input: []schemas.ChatMessage{
			{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")}},
		},
	})
	if bifrostErr != nil {
		t.Fatalf("stream request failed: %s", bifrostErr.Error.Message)
	}

	// First chunk has been delivered and the upstream is parked mid-stream, so
	// the attempt is definitively still in flight.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream never started streaming")
	}
	if got := client.keyInflight.InFlight(string(schemas.OpenAI), inflightTestKeyID); got != 1 {
		t.Fatalf("InFlight while streaming = %d, want 1", got)
	}

	close(release)
	content, errs := drainChatStream(stream)
	if content != "hello" {
		t.Fatalf("stream content = %q, want %q", content, "hello")
	}
	if len(errs) > 0 {
		t.Fatalf("stream emitted error chunks: %v", errs)
	}
	waitForInflight(t, client.keyInflight, schemas.OpenAI, inflightTestKeyID, 0)
}

// TestMaxConcurrentRequestsPerKey_StreamingRetryReleasesPerAttempt proves each
// retry attempt releases its own slot: with a cap of 1 a leaked first attempt
// would leave one counter behind after the retry completes.
func TestMaxConcurrentRequestsPerKey_StreamingRetryReleasesPerAttempt(t *testing.T) {
	var hits int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		attempt := hits
		mu.Unlock()
		if attempt == 1 {
			sseHandler(`{"error":{"message":"rate limit exceeded, please retry","type":"rate_limit_error"}}`)(w, r)
			return
		}
		sseHandler(
			`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"he"}}]}`,
			`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"llo"}}]}`,
			`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
		)(w, r)
	}))
	defer server.Close()

	account := NewMockAccount()
	account.AddProviderWithBaseURL(schemas.OpenAI, 1, 1, server.URL)
	account.configs[schemas.OpenAI].NetworkConfig.MaxRetries = 1
	account.configs[schemas.OpenAI].NetworkConfig.RetryBackoffInitial = time.Millisecond
	account.configs[schemas.OpenAI].MaxConcurrentRequestsPerKey = 1
	account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
		{ID: inflightTestKeyID, Value: *schemas.NewSecretVar("sk-inflight"), Models: schemas.WhiteList{"*"}, Weight: 100},
	})

	client, err := Init(context.Background(), schemas.BifrostConfig{
		Account: account,
		Logger:  NewDefaultLogger(schemas.LogLevelError),
	})
	if err != nil {
		t.Fatalf("failed to initialize bifrost: %v", err)
	}
	defer client.Shutdown()

	ctx := schemas.NewBifrostContext(context.Background(), time.Now().Add(30*time.Second))
	stream, bifrostErr := client.ChatCompletionStreamRequest(ctx, &schemas.BifrostChatRequest{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o-mini",
		Input: []schemas.ChatMessage{
			{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")}},
		},
	})
	if bifrostErr != nil {
		t.Fatalf("retried stream failed: %s", bifrostErr.Error.Message)
	}
	content, errs := drainChatStream(stream)
	if content != "hello" {
		t.Fatalf("stream content = %q, want %q", content, "hello")
	}
	if len(errs) > 0 {
		t.Fatalf("stream emitted error chunks: %v", errs)
	}
	mu.Lock()
	gotHits := hits
	mu.Unlock()
	if gotHits != 2 {
		t.Fatalf("upstream hits = %d, want 2", gotHits)
	}
	waitForInflight(t, client.keyInflight, schemas.OpenAI, inflightTestKeyID, 0)
}

// TestPreferKeyWithInflightHeadroom covers the selection-stage preference that
// the rotating key pool applies when the cap is enabled.
func TestPreferKeyWithInflightHeadroom(t *testing.T) {
	newBifrost := func(keys []schemas.Key) *Bifrost {
		return &Bifrost{
			keyInflight: keyinflight.New(),
			logger:      NewDefaultLogger(schemas.LogLevelError),
			// Deterministic: always the first candidate.
			keySelector: func(_ *schemas.BifrostContext, keys []schemas.Key, _ schemas.ModelProvider, _ string) (schemas.Key, error) {
				return keys[0], nil
			},
		}
	}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	pool := []schemas.Key{{ID: "a"}, {ID: "b"}}

	t.Run("DisabledKeepsOriginalSelection", func(t *testing.T) {
		client := newBifrost(pool)
		// Pre-load key a to prove limit=0 ignores existing counts entirely.
		if !client.keyInflight.Acquire(string(schemas.OpenAI), "a", 1) {
			t.Fatal("seed acquire failed")
		}
		got := client.preferKeyWithInflightHeadroom(ctx, pool[0], pool, schemas.OpenAI, "m", 0)
		if got.ID != "a" {
			t.Fatalf("selected key = %q with cap disabled, want %q", got.ID, "a")
		}
		if n := client.keyInflight.InFlight(string(schemas.OpenAI), "a"); n != 1 {
			t.Fatalf("InFlight(a) = %d, want the pre-seeded 1 (cap disabled must not touch state)", n)
		}
	})

	t.Run("SaturatedKeyIsSkipped", func(t *testing.T) {
		client := newBifrost(pool)
		for i := 0; i < 2; i++ {
			if !client.keyInflight.Acquire(string(schemas.OpenAI), "a", 2) {
				t.Fatal("seed acquire failed")
			}
		}
		got := client.preferKeyWithInflightHeadroom(ctx, pool[0], pool, schemas.OpenAI, "m", 2)
		if got.ID != "b" {
			t.Fatalf("selected key = %q, want the key with headroom %q", got.ID, "b")
		}
	})

	t.Run("AllSaturatedKeepsSelectionFailOpen", func(t *testing.T) {
		client := newBifrost(pool)
		if !client.keyInflight.Acquire(string(schemas.OpenAI), "a", 1) ||
			!client.keyInflight.Acquire(string(schemas.OpenAI), "b", 1) {
			t.Fatal("seed acquire failed")
		}
		got := client.preferKeyWithInflightHeadroom(ctx, pool[0], pool, schemas.OpenAI, "m", 1)
		if got.ID != "a" {
			t.Fatalf("selected key = %q when every key is saturated, want the original %q", got.ID, "a")
		}
	})
}

// TestProviderConfig_NormalizesNegativeInflightCap documents the config
// normalization choice: a negative cap is coerced to "disabled" instead of
// failing provider init, and the gate treats it as disabled regardless.
func TestProviderConfig_NormalizesNegativeInflightCap(t *testing.T) {
	cfg := &schemas.ProviderConfig{MaxConcurrentRequestsPerKey: -3}
	cfg.CheckAndSetDefaults()
	if cfg.MaxConcurrentRequestsPerKey != 0 {
		t.Fatalf("MaxConcurrentRequestsPerKey = %d after defaults, want 0", cfg.MaxConcurrentRequestsPerKey)
	}

	client := &Bifrost{keyInflight: keyinflight.New(), logger: NewDefaultLogger(schemas.LogLevelError)}
	if client.acquireKeyInflight(schemas.OpenAI, "a", -3) {
		t.Fatal("negative cap acquired a slot")
	}
	if n := client.keyInflight.InFlight(string(schemas.OpenAI), "a"); n != 0 {
		t.Fatalf("InFlight = %d after negative-cap acquire, want 0", n)
	}
}
