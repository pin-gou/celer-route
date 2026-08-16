package lib

import (
	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

var logger schemas.Logger

// SetLogger sets the logger for the application.
func SetLogger(l schemas.Logger) {
	logger = l
}

// HasDuplicates reports whether the slice contains any repeated element.
// Comparison is exact; callers needing case-insensitive or whitespace-tolerant
// semantics should normalize the slice before calling (e.g. lower-case the
// entries for case-insensitive HTTP header names).
func HasDuplicates[T comparable](items []T) bool {
	if len(items) < 2 {
		return false
	}
	seen := make(map[T]struct{}, len(items))
	for _, it := range items {
		if _, dup := seen[it]; dup {
			return true
		}
		seen[it] = struct{}{}
	}
	return false
}

// StreamLargeResponseBody extracts the large response reader from context and streams
// it directly to the client. Always returns false in OSS (enterprise-only feature).
func StreamLargeResponseBody(ctx *fasthttp.RequestCtx, bifrostCtx *schemas.BifrostContext) bool {
	return false
}
