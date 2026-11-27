package providercooldown

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Defaults / bounds for CapacityConfig. Values are normalized once at
// construction time so the hot path never re-derives them.
const (
	defaultCapacityWindowSeconds = 60
	minCapacityWindowSeconds     = 10
	maxCapacityWindowSeconds     = 3600

	defaultCapacityHeadroom = 0.9
	minCapacityHeadroom     = 0.1
	maxCapacityHeadroom     = 1.0

	// CapacityScopeProvider shares one budget across every model of a
	// provider; CapacityScopeProviderModel keeps an independent budget per
	// (provider, model) pair.
	CapacityScopeProvider      = "provider"
	CapacityScopeProviderModel = "provider_model"

	// CapacityActionFallback allows the configured fallback chain to take
	// over once the gate engages; CapacityActionReject refuses the request.
	CapacityActionFallback = "fallback"
	CapacityActionReject   = "reject"

	// capacityGateErrorType is the error type the gate stamps on its synthetic
	// 429. It is deliberately distinct from "no_eligible_keys": the client
	// must be able to tell "our own budget gate" apart from "every key is
	// cooling down".
	capacityGateErrorType = "rate_limit_exceeded"

	// capacityGateMessagePrefix prefixes the gate's client-visible message.
	// isCapacityGateError uses it to recognise the synthetic error without
	// adding a wire-visible field.
	capacityGateMessagePrefix = "capacity gate: "
)

// capacityBucket is one second of accounted usage. epoch is the Unix second
// the bucket describes; 0 (or any epoch older than the live window) marks it
// as empty. Buckets are recycled, never reallocated.
type capacityBucket struct {
	epoch    int64
	tokens   int64
	requests int64
}

// capacityWindow is the fixed-length ring of per-second buckets for one
// accounting key. len(buckets) == window_seconds; the bucket for second e
// lives at index e % window_seconds.
type capacityWindow struct {
	buckets []capacityBucket
}

// capacityGate is the retrospective budget gate. It sums real usage over a
// rolling window and refuses *subsequent* requests once the window's usage
// crosses limit*headroom.
//
// Concurrency: the gate has its own mutex rather than sharing
// CooldownState's RWMutex. The cooldown map's lock is held briefly on the
// mark/filter path, but the gate is touched on every terminal request
// (write) and every attempt (read), including per-stream-chunk PostLLMHook
// calls. Keeping the two independent avoids coupling that write-heavy traffic
// to key-pool filtering (AsFilter) latency, and avoids turning the cooldown
// RWMutex into a write-lock bottleneck under load. The gate is nil unless
// capacity.enabled=true and at least one limit is set, so a disabled config
// pays exactly one nil-pointer check per request.
type capacityGate struct {
	// Immutable after construction.
	tokenLimit    int64
	requestLimit  int64
	tokenThresh   int64 // token usage at which the gate engages, 0 when unlimited
	requestThresh int64 // request count at which the gate engages, 0 when unlimited
	windowSeconds int64
	scope         string
	action        string
	headroom      float64

	// now is the clock, shared with the owning CooldownState so tests inject
	// one fake clock for both features. It is a method value, so a clock
	// installed after the gate was built is still honoured.
	now func() time.Time

	mu   sync.Mutex
	keys map[string]*capacityWindow

	// lastSweepSecond is the Unix second of the last opportunistic sweep of
	// expired keys. Sweeping at most once per window keeps the key map
	// bounded by the number of provider/model pairs with live traffic
	// without needing a background goroutine.
	lastSweepSecond int64
}

// capacityVerdict is returned by check when the gate refuses a request.
type capacityVerdict struct {
	usedTokens        int64
	usedRequests      int64
	retryAfterSeconds int64
}

// newCapacityGate builds a gate from an already-normalized, Active() config.
func newCapacityGate(cfg *CapacityConfig) *capacityGate {
	g := &capacityGate{
		tokenLimit:    cfg.TokenLimit,
		requestLimit:  cfg.RequestLimit,
		windowSeconds: int64(normalizeCapacityWindowSeconds(cfg.WindowSeconds)),
		scope:         normalizeCapacityScope(cfg.Scope),
		action:        normalizeCapacityAction(cfg.Action),
		headroom:      normalizeCapacityHeadroom(cfg.Headroom),
		keys:          make(map[string]*capacityWindow),
	}
	if g.tokenLimit > 0 {
		g.tokenThresh = capacityThreshold(g.tokenLimit, g.headroom)
	}
	if g.requestLimit > 0 {
		g.requestThresh = capacityThreshold(g.requestLimit, g.headroom)
	}
	return g
}

// capacityThreshold converts limit*headroom into the integer usage count at
// which the gate engages.
//
// The epsilon keeps the boundary exact for headrooms whose float product
// lands a hair above the intended integer (0.9*1000 evaluates to
// 900.0000000000001 in float64, which would otherwise push the gate to 901)
// while still rounding a genuinely fractional budget up (0.9*7 = 6.3 -> 7,
// since usage is always an integer count).
func capacityThreshold(limit int64, headroom float64) int64 {
	t := int64(math.Ceil(float64(limit)*headroom - 1e-9))
	if t < 1 {
		t = 1
	}
	return t
}

// windowKey returns the accounting key for (provider, model) under the
// configured scope.
func (g *capacityGate) windowKey(provider schemas.ModelProvider, model string) string {
	if g.scope == CapacityScopeProviderModel {
		return string(provider) + "|" + model
	}
	return string(provider)
}

// nowSecond returns the current Unix second from the shared clock.
func (g *capacityGate) nowSecond() int64 {
	if g.now != nil {
		return g.now().Unix()
	}
	return time.Now().Unix()
}

// check reports whether the current window has already consumed its budget.
// A nil return means "allow". Reads lazily expire stale buckets and drop the
// key entry entirely once nothing is live, so a window that has fully slid
// away costs no memory.
func (g *capacityGate) check(provider schemas.ModelProvider, model string) *capacityVerdict {
	if g == nil {
		return nil
	}
	if g.scope == CapacityScopeProviderModel && model == "" {
		// Nothing to attribute; never gate on an unattributable pair.
		return nil
	}
	key := g.windowKey(provider, model)
	now := g.nowSecond()

	g.mu.Lock()
	defer g.mu.Unlock()

	w := g.keys[key]
	if w == nil {
		return nil
	}
	tokens, requests := w.sweepLocked(g.windowSeconds, now)
	if tokens == 0 && requests == 0 {
		delete(g.keys, key)
		return nil
	}
	tokenHit := g.tokenThresh > 0 && tokens >= g.tokenThresh
	requestHit := g.requestThresh > 0 && requests >= g.requestThresh
	if !tokenHit && !requestHit {
		return nil
	}
	return &capacityVerdict{
		usedTokens:        tokens,
		usedRequests:      requests,
		retryAfterSeconds: g.retryAfterLocked(w, now, tokens, requests),
	}
}

// record adds one terminal request's real usage to the window. tokens may be
// 0 (usage unavailable or a failure that was not billed) — the request count
// always advances, since it measures traffic we actually sent upstream.
func (g *capacityGate) record(provider schemas.ModelProvider, model string, tokens int64) {
	if g == nil || provider == "" || model == "" {
		return
	}
	if tokens < 0 {
		tokens = 0
	}
	key := g.windowKey(provider, model)
	now := g.nowSecond()

	g.mu.Lock()
	w := g.keys[key]
	if w == nil {
		w = &capacityWindow{buckets: make([]capacityBucket, g.windowSeconds)}
		g.keys[key] = w
	}
	w.addLocked(g.windowSeconds, now, tokens)
	g.sweepKeysLocked(now)
	g.mu.Unlock()
}

// sweepKeysLocked drops accounting keys whose window has fully expired. It
// runs at most once per window so the key map stays bounded by the number of
// provider/model pairs with live traffic, without a background goroutine.
func (g *capacityGate) sweepKeysLocked(now int64) {
	if g.lastSweepSecond != 0 && now-g.lastSweepSecond < g.windowSeconds {
		return
	}
	g.lastSweepSecond = now
	for key, w := range g.keys {
		tokens, requests := w.sweepLocked(g.windowSeconds, now)
		if tokens == 0 && requests == 0 {
			delete(g.keys, key)
		}
	}
}

// retryAfterLocked estimates how many seconds must pass before both limits
// are back below their thresholds. Each dimension is evaluated independently
// by dropping its oldest live buckets (which is exactly what window sliding
// does) and taking the release time of the bucket that brings usage back
// under the threshold; the larger of the two wins, because both must hold.
// Falls back to a full window when nothing usable can be computed.
func (g *capacityGate) retryAfterLocked(w *capacityWindow, now, tokens, requests int64) int64 {
	wait := int64(0)
	if g.tokenThresh > 0 && tokens >= g.tokenThresh {
		if d := g.secondsUntilBelow(w, now, tokens, g.tokenThresh, true); d > wait {
			wait = d
		}
	}
	if g.requestThresh > 0 && requests >= g.requestThresh {
		if d := g.secondsUntilBelow(w, now, requests, g.requestThresh, false); d > wait {
			wait = d
		}
	}
	if wait < 1 {
		wait = g.windowSeconds
	}
	return wait
}

// secondsUntilBelow walks live buckets from oldest to newest (the order the
// window releases them) and returns the seconds until usage drops below
// threshold. 0 means "no further release needed".
//
// A bucket for second e leaves the window at second e+window_seconds, so
// once the bucket that brings usage under the threshold is identified, the
// wait is exactly e+window_seconds-now.
func (g *capacityGate) secondsUntilBelow(w *capacityWindow, now, used, threshold int64, tokens bool) int64 {
	if w == nil {
		return g.windowSeconds
	}
	remaining := used
	for epoch := now - g.windowSeconds + 1; epoch <= now; epoch++ {
		b := &w.buckets[capacitySlot(epoch, g.windowSeconds)]
		if b.epoch != epoch {
			// Never written (or already recycled) — contributes nothing.
			continue
		}
		if tokens {
			remaining -= b.tokens
		} else {
			remaining -= b.requests
		}
		if remaining < threshold {
			release := epoch + g.windowSeconds - now
			if release < 1 {
				return 1
			}
			return release
		}
	}
	return g.windowSeconds
}

// sweepLocked zeroes every expired bucket and returns the live totals. A
// bucket with epoch e is live while e > now-windowSeconds (its release time
// is e+windowSeconds), so the live range is [now-window+1, now].
func (w *capacityWindow) sweepLocked(windowSeconds, now int64) (tokens, requests int64) {
	oldestLive := now - windowSeconds + 1
	for i := range w.buckets {
		b := &w.buckets[i]
		if b.epoch < oldestLive {
			// Includes epoch 0 (never written) and any bucket that has aged
			// out of the window. Reset in place; the ring slot is reused.
			b.epoch = 0
			b.tokens = 0
			b.requests = 0
			continue
		}
		tokens += b.tokens
		requests += b.requests
	}
	return tokens, requests
}

// addLocked accounts one request (and tokens) into the bucket for second now,
// recycling the slot when it belongs to an earlier second.
func (w *capacityWindow) addLocked(windowSeconds, now, tokens int64) {
	b := &w.buckets[capacitySlot(now, windowSeconds)]
	if b.epoch != now {
		b.epoch = now
		b.tokens = 0
		b.requests = 0
	}
	b.tokens += tokens
	b.requests++
}

// capacitySlot maps a Unix second to its ring index. Go's % keeps the sign of
// the dividend, so a pre-1970 clock would otherwise index negatively.
func capacitySlot(epoch, windowSeconds int64) int64 {
	idx := epoch % windowSeconds
	if idx < 0 {
		idx += windowSeconds
	}
	return idx
}

// --- observability helpers (used by tests and diagnostics) ---

// trackedKeys reports how many accounting keys currently hold state. A
// disabled gate is nil, so this is only reachable on an active gate.
func (g *capacityGate) trackedKeys() int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.keys)
}

// liveBuckets reports how many buckets for (provider, model) still carry a
// live epoch. Used by tests to assert stale buckets were zeroed.
func (g *capacityGate) liveBuckets(provider schemas.ModelProvider, model string) int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	w := g.keys[g.windowKey(provider, model)]
	if w == nil {
		return 0
	}
	n := 0
	for i := range w.buckets {
		if w.buckets[i].epoch != 0 {
			n++
		}
	}
	return n
}

// usage returns the live window totals for (provider, model) without
// gating. Used by tests.
func (g *capacityGate) usage(provider schemas.ModelProvider, model string) (tokens, requests int64) {
	if g == nil {
		return 0, 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	w := g.keys[g.windowKey(provider, model)]
	if w == nil {
		return 0, 0
	}
	return w.sweepLocked(g.windowSeconds, g.nowSecond())
}

// --- CooldownState / plugin wiring ---

// enableCapacity installs an active gate on the state. Like enableEscalation,
// this is normally called while the state is being built (AsState), but the
// atomic store makes a mid-flight reconfiguration race-free too.
func (c *CooldownState) enableCapacity(cfg *CapacityConfig) {
	if c == nil || cfg == nil || !cfg.Active() {
		return
	}
	g := newCapacityGate(cfg)
	g.now = c.nowTime
	c.capacity.Store(g)
}

// capacityGate returns the gate installed on the state, or nil when the
// feature is off.
func (c *CooldownState) capacityGate() *capacityGate {
	if c == nil {
		return nil
	}
	return c.capacity.Load()
}

// capacityShortCircuit evaluates the gate for (provider, model) and, when the
// window's budget is spent, builds the synthetic 429 that short-circuits the
// attempt. Returns nil when the request may proceed.
func (p *CooldownPlugin) capacityShortCircuit(ctx *schemas.BifrostContext, gate *capacityGate, provider schemas.ModelProvider, model string) *schemas.LLMPluginShortCircuit {
	verdict := gate.check(provider, model)
	if verdict == nil {
		return nil
	}
	retryAfter := verdict.retryAfterSeconds
	statusCode := 429
	errType := capacityGateErrorType
	message := fmt.Sprintf(
		"%s%s/%s exceeded its %ds budget; retry in %ds",
		capacityGateMessagePrefix, provider, model, gate.windowSeconds, retryAfter,
	)
	// "fallback" lets the configured fallback chain pick the request up;
	// "reject" returns this error immediately. nil==true by default, so both
	// branches set the field explicitly.
	allowFallbacks := gate.action != CapacityActionReject
	if ctx != nil && retryAfter > 0 {
		ctx.SetValue(schemas.BifrostContextKeyRetryAfterSeconds, retryAfter)
	}
	if p.logger != nil {
		p.logger.Info(
			"[provider-cooldown] capacity gate short-circuit on %s/%s — used %d token(s) / %d request(s) in %ds window (token_limit=%d, request_limit=%d, headroom=%.2f, scope=%s, action=%s, retry-after %ds)",
			provider, model, verdict.usedTokens, verdict.usedRequests, gate.windowSeconds,
			gate.tokenLimit, gate.requestLimit, gate.headroom, gate.scope, gate.action, retryAfter,
		)
	}
	return &schemas.LLMPluginShortCircuit{
		Error: &schemas.BifrostError{
			IsBifrostError: false,
			StatusCode:     &statusCode,
			Type:           &errType,
			Error: &schemas.ErrorField{
				Type:    &errType,
				Message: message,
			},
			ExtraFields: schemas.BifrostErrorExtraFields{
				RetryAfterSeconds: retryAfter,
			},
			AllowFallbacks: &allowFallbacks,
		},
		Silent: true,
	}
}

// recordCapacity accounts one terminal request's real usage into the gate.
//
// It runs for both success and failure because it measures traffic, not
// outcomes: the request count advances on every terminal outcome, and tokens
// are added only when the provider reported them (response usage on success,
// ExtraFields.BilledUsage on a failed-but-billed attempt). When usage cannot
// be determined the request is still counted, tokens are 0 — the documented
// degradation, since guessing would defeat the whole point of a gate that
// only trusts real numbers.
//
// Two guards keep rejected requests out of the budget:
//
//   - Attempts that never reached the provider are skipped. The framework
//     marks those with BifrostContextKeySilentLog (our own short-circuit,
//     the "no eligible keys" short-circuit, and the worker's synthetic
//     no_eligible_keys error), and plugin-produced responses (e.g. a cache
//     hit) carry no RoutingInfo.Provider.
//   - Streaming calls PostLLMHook once per chunk, so only the terminal chunk
//     is accounted.
func (p *CooldownPlugin) recordCapacity(ctx *schemas.BifrostContext, gate *capacityGate, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) {
	if gate == nil || ctx == nil {
		return
	}
	// The framework sets this flag for attempts that were rejected before
	// reaching a provider. Counting them would let rejections extend the
	// very window that rejected them.
	if silent, _ := ctx.Value(schemas.BifrostContextKeySilentLog).(bool); silent {
		return
	}

	var requestType schemas.RequestType
	var provider schemas.ModelProvider
	var model string
	var tokens int64
	if bifrostErr != nil {
		requestType = bifrostErr.ExtraFields.RequestType
		routing := bifrostErr.ExtraFields.RoutingInfo
		provider = routing.Provider
		model = routing.Model
		tokens = usageTotal(bifrostErr.ExtraFields.BilledUsage)
	} else if resp != nil {
		extra := resp.GetExtraFields()
		if extra == nil {
			return
		}
		requestType = extra.RequestType
		routing := extra.RoutingInfo
		provider = routing.Provider
		model = routing.Model
		tokens = responseTokenUsage(resp)
	} else {
		return
	}

	// A response/error without routing info did not come from a provider
	// attempt (e.g. a semantic-cache hit produced by a plugin).
	if provider == "" || model == "" {
		return
	}

	// Streaming calls PostLLMHook once per chunk; account only the terminal
	// one. Providers set BifrostContextKeyStreamEndIndicator on every
	// terminal path — a normal final chunk, a truncated stream, a cancellation
	// and an upstream read error all set it before running post hooks — so
	// this both avoids counting every chunk and keeps one stream from being
	// billed twice. A stream that dies without ever setting the marker is not
	// accounted (tokens 0, request 0): under-counting is preferred to
	// double-counting a stream's tokens.
	if isStreamingRequest(requestType) && !isFinalChunk(ctx) {
		return
	}

	gate.record(provider, model, tokens)
}

// isCapacityGateError recognises the synthetic error built by
// capacityShortCircuit. Keeping the cooldown marker away from it matters: the
// rejection never reached a provider, and an operator's custom quota pattern
// (e.g. a bare "exceeded") could otherwise match the message and cool a key
// for a request we refused ourselves.
func isCapacityGateError(err *schemas.BifrostError) bool {
	if err == nil || err.Error == nil {
		return false
	}
	if err.Type == nil || *err.Type != capacityGateErrorType {
		return false
	}
	return strings.HasPrefix(err.Error.Message, capacityGateMessagePrefix)
}

// isStreamingRequest reports whether the given request type is a streaming
// one, mirroring bifrost.IsStreamRequestType. The realtime request type is
// excluded: a realtime turn is one completed exchange, not a chunk stream.
func isStreamingRequest(t schemas.RequestType) bool {
	switch t {
	case schemas.TextCompletionStreamRequest,
		schemas.ChatCompletionStreamRequest,
		schemas.ResponsesStreamRequest,
		schemas.ResponsesRetrieveStreamRequest,
		schemas.SpeechStreamRequest,
		schemas.TranscriptionStreamRequest,
		schemas.ImageGenerationStreamRequest,
		schemas.ImageEditStreamRequest,
		schemas.PassthroughStreamRequest,
		schemas.WebSocketResponsesRequest:
		return true
	default:
		return false
	}
}

// isFinalChunk mirrors bifrost.IsFinalChunk without importing the whole core
// package (the plugin only needs schemas).
func isFinalChunk(ctx *schemas.BifrostContext) bool {
	if ctx == nil {
		return false
	}
	v, ok := ctx.Value(schemas.BifrostContextKeyStreamEndIndicator).(bool)
	return ok && v
}

// usageTotal returns the total token count of a usage block, falling back to
// prompt+completion when the provider omitted the total.
func usageTotal(u *schemas.BifrostLLMUsage) int64 {
	if u == nil {
		return 0
	}
	if u.TotalTokens > 0 {
		return int64(u.TotalTokens)
	}
	return int64(u.PromptTokens) + int64(u.CompletionTokens)
}

// responseTokenUsage extracts the real token usage from any supported
// response shape, mirroring the logging plugin's extraction order. Returns 0
// when the shape carries no usage; callers treat that as "unknown" and still
// count the request.
func responseTokenUsage(resp *schemas.BifrostResponse) int64 {
	if resp == nil {
		return 0
	}
	switch {
	case resp.TextCompletionResponse != nil:
		return usageTotal(resp.TextCompletionResponse.Usage)
	case resp.ChatResponse != nil:
		return usageTotal(resp.ChatResponse.Usage)
	case resp.ResponsesResponse != nil && resp.ResponsesResponse.Usage != nil:
		return usageTotal(resp.ResponsesResponse.Usage.ToBifrostLLMUsage())
	case resp.ResponsesStreamResponse != nil && resp.ResponsesStreamResponse.Response != nil:
		return usageTotal(resp.ResponsesStreamResponse.Response.Usage.ToBifrostLLMUsage())
	case resp.CompactionResponse != nil && resp.CompactionResponse.Usage != nil:
		return usageTotal(resp.CompactionResponse.Usage.ToBifrostLLMUsage())
	case resp.EmbeddingResponse != nil:
		return usageTotal(resp.EmbeddingResponse.Usage)
	case resp.RerankResponse != nil:
		return usageTotal(resp.RerankResponse.Usage)
	case resp.TranscriptionResponse != nil && resp.TranscriptionResponse.Usage != nil:
		u := resp.TranscriptionResponse.Usage
		if u.TotalTokens != nil && *u.TotalTokens > 0 {
			return int64(*u.TotalTokens)
		}
		var total int64
		if u.InputTokens != nil {
			total += int64(*u.InputTokens)
		}
		if u.OutputTokens != nil {
			total += int64(*u.OutputTokens)
		}
		return total
	case resp.ImageGenerationResponse != nil && resp.ImageGenerationResponse.Usage != nil:
		u := resp.ImageGenerationResponse.Usage
		if u.TotalTokens > 0 {
			return int64(u.TotalTokens)
		}
		return int64(u.InputTokens) + int64(u.OutputTokens)
	case resp.PassthroughResponse != nil && resp.PassthroughResponse.PassthroughUsage != nil:
		return usageTotal(resp.PassthroughResponse.PassthroughUsage.LLMUsage)
	}
	return 0
}
