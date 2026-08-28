package rtk

import "github.com/pin-gou/celer-route/core/schemas"

// CompressionState holds per-request compression state, stored in the Plugin's
// sync.Map keyed by request ID. Populated by PreLLMHook and consumed by PostLLMHook.
type CompressionState struct {
	OriginalTokens    int
	CompressedTokens  int
	Compressed        bool
	Techniques        []string
	FilterMatched     string
	RawOutputPointers []*RtkRawOutputPointer
	// OriginalSnapshot captures the pre-compression text for every tool
	// message that the pipeline either compressed or considered compressing.
	// It is not serialised — it lives only in the per-request state and is
	// converted to JSON in PostLLMHook.
	OriginalSnapshot []SnapshotEntry
}

// NewCompressionState creates a new CompressionState with default values.
func NewCompressionState() *CompressionState {
	return &CompressionState{
		Techniques:        make([]string, 0),
		RawOutputPointers: make([]*RtkRawOutputPointer, 0),
	}
}

// setState stores the compression state for the given context's request ID.
func (p *Plugin) setState(ctx *schemas.BifrostContext, state *CompressionState) {
	reqID := ctx.Value(schemas.BifrostContextKeyRequestID)
	if reqID == nil {
		// Fallback: use a placeholder if no request ID is available
		p.stateStore.Store("default", state)
		return
	}
	id, ok := reqID.(string)
	if !ok {
		p.stateStore.Store("default", state)
		return
	}
	p.stateStore.Store(id, state)
}

// getState retrieves the compression state for the given context's request ID.
func (p *Plugin) getState(ctx *schemas.BifrostContext) *CompressionState {
	reqID := ctx.Value(schemas.BifrostContextKeyRequestID)
	if reqID == nil {
		v, ok := p.stateStore.Load("default")
		if !ok {
			return nil
		}
		state, _ := v.(*CompressionState)
		return state
	}
	id, ok := reqID.(string)
	if !ok {
		v, ok := p.stateStore.Load("default")
		if !ok {
			return nil
		}
		state, _ := v.(*CompressionState)
		return state
	}
	v, ok := p.stateStore.Load(id)
	if !ok {
		return nil
	}
	state, _ := v.(*CompressionState)
	return state
}

// getCompressionState retrieves the compression state for PostLLMHook.
// Returns nil if no compression state exists for this request.
func (p *Plugin) getCompressionState(ctx *schemas.BifrostContext) *CompressionState {
	return p.getState(ctx)
}

// clearCompressionState removes the compression state for the given context's request ID.
func (p *Plugin) clearCompressionState(ctx *schemas.BifrostContext) {
	reqID := ctx.Value(schemas.BifrostContextKeyRequestID)
	if reqID == nil {
		p.stateStore.Delete("default")
		return
	}
	id, ok := reqID.(string)
	if !ok {
		p.stateStore.Delete("default")
		return
	}
	p.stateStore.Delete(id)
}
