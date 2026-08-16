package utils

import (
	"io"
	"math"

	"github.com/bytedance/sonic"
	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// LargeResponseReader wraps an io.Reader and releases the fasthttp response on Close.
// Used by providers to keep the response alive while the transport streams it to the client.
// ctx is held to check BifrostContextKeyConnectionClosed in Close, so a mid-stream
// cancellation that already tore down the underlying fasthttp conn does not double-release.
type LargeResponseReader struct {
	io.Reader
	Resp     *fasthttp.Response
	ctx      *schemas.BifrostContext
	cleanup  func()
	consumed bool // true after Read returns io.EOF, body fully consumed through Reader chain
}

// Read delegates to the wrapped Reader and tracks EOF so Close() can skip
// a redundant (and potentially blocking) drain of the body stream.
func (r *LargeResponseReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.consumed = true
	}
	return n, err
}

// Close drains any unconsumed body stream and releases the underlying fasthttp
// response back to the pool. Draining prevents "whitespace in header" errors on
// connection reuse when the client disconnects before the full response is consumed
// (see: fasthttp#1743).
//
// When the body was already fully consumed through the Reader chain (consumed == true),
// the drain is skipped. For identity-encoded responses (no Content-Length), the body
// stream is a fasthttp closeReader that blocks until the TCP connection closes — which
// can take minutes if the upstream server keeps the connection alive.
func (r *LargeResponseReader) Close() error {
	if r == nil || r.Resp == nil {
		return nil
	}
	// Run cleanup first so SetupStreamCancellation's goroutine settles (close(done); <-closed)
	// before we read BifrostContextKeyConnectionClosed. The goroutine's done-branch can set the
	// flag when ctx.Err() != nil, so checking it before cleanup would miss that interleaving and
	// fall through to fasthttp.ReleaseResponse on an already-torn-down conn (nil-deref in connsCleaner).
	if r.cleanup != nil {
		r.cleanup()
		r.cleanup = nil
	}
	if r.ctx != nil {
		if closed, ok := r.ctx.Value(schemas.BifrostContextKeyConnectionClosed).(bool); ok && closed {
			r.Resp = nil
			return nil
		}
	}
	if !r.consumed {
		if bodyStream := r.Resp.BodyStream(); bodyStream != nil {
			_, _ = io.Copy(io.Discard, bodyStream)
			if closer, ok := bodyStream.(io.Closer); ok {
				_ = closer.Close()
			}
		}
	}
	fasthttp.ReleaseResponse(r.Resp)
	r.Resp = nil
	return nil
}

// BuildLargeResponseClient creates a streaming-enabled fasthttp client for large response detection.
// The client caps buffering at the threshold and enables response body streaming.
//
// ReadTimeout/WriteTimeout/MaxConnDuration are zeroed: large-response bodies may take arbitrarily
// long to download, and fasthttp's ReadTimeout bounds *full* body read — not idle. Idle detection
// on stalled streams is handled separately (see NewIdleTimeoutReader / SetupStreamingPassthrough).
func BuildLargeResponseClient(base *fasthttp.Client, responseThreshold int64) *fasthttp.Client {
	client := CloneFastHTTPClientConfig(base)
	if responseThreshold > 0 && responseThreshold <= int64(math.MaxInt) {
		client.MaxResponseBodySize = int(responseThreshold)
	}
	client.StreamResponseBody = true
	client.ReadTimeout = 0
	client.WriteTimeout = 0
	client.MaxConnDuration = 0
	return client
}

// PrepareResponseStreaming is a no-op in OSS: there is no large response
// threshold set from context, so the original client is returned unchanged.
func PrepareResponseStreaming(ctx *schemas.BifrostContext, client *fasthttp.Client, resp *fasthttp.Response) *fasthttp.Client {
	return client
}

// MaterializeStreamErrorBody is a no-op in OSS: response streaming is not
// active, so all error bodies are already materialized in the response.
func MaterializeStreamErrorBody(ctx *schemas.BifrostContext, resp *fasthttp.Response) {
}

// FinalizeResponseWithLargeDetection reads the response body from the stream. In OSS
// there is no large response threshold, so the response is always buffered normally.
// Returns (body, false, nil) on success.
func FinalizeResponseWithLargeDetection(
	ctx *schemas.BifrostContext,
	resp *fasthttp.Response,
	logger schemas.Logger,
) ([]byte, bool, *schemas.BifrostError) {
	body, err := CheckAndDecodeBody(resp)
	if err != nil {
		return nil, false, NewBifrostOperationError(schemas.ErrProviderResponseDecode, err)
	}
	// Copy body before caller releases resp
	return append([]byte(nil), body...), false, nil
}

// ParseOpenAIUsageFromBytes parses OpenAI-format usage from raw JSON bytes into BifrostLLMUsage.
// Handles both Chat Completions (prompt_tokens/completion_tokens) and Responses API
// (input_tokens/output_tokens) field names. Expects the "usage" object bytes directly,
// not the full response body.
func ParseOpenAIUsageFromBytes(data []byte) *schemas.BifrostLLMUsage {
	var usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		// Responses API uses different field names
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	}
	if err := sonic.Unmarshal(data, &usage); err != nil {
		return nil
	}

	result := &schemas.BifrostLLMUsage{}
	if usage.PromptTokens > 0 {
		result.PromptTokens = usage.PromptTokens
	} else if usage.InputTokens > 0 {
		result.PromptTokens = usage.InputTokens
	}
	if usage.CompletionTokens > 0 {
		result.CompletionTokens = usage.CompletionTokens
	} else if usage.OutputTokens > 0 {
		result.CompletionTokens = usage.OutputTokens
	}
	if usage.TotalTokens > 0 {
		result.TotalTokens = usage.TotalTokens
	} else {
		result.TotalTokens = result.PromptTokens + result.CompletionTokens
	}

	if result.TotalTokens == 0 {
		return nil
	}
	return result
}

// SetupStreamingPassthrough is a no-op in OSS: large payload mode is never
// activated, so streaming passthrough is never set up.
func SetupStreamingPassthrough(ctx *schemas.BifrostContext, resp *fasthttp.Response) bool {
	return false
}
