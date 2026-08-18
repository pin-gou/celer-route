// Package rtk provides a Rule-based Tool-output Kompression plugin for Bifrost.
// This is a minimal stub that will be fully implemented in the dev.plugins track.
package rtk

import (
	"context"
	"fmt"
	"sync"

	"github.com/pin-gou/pg-gateway/core/schemas"
)

// PluginName is the canonical name for the RTK compression plugin.
const PluginName = "rtk"

// Plugin implements schemas.LLMPlugin for rule-based tool output compression.
type Plugin struct {
	name       string
	config     *Config
	logger     schemas.Logger
	stateStore sync.Map // map[string]*CompressionState, keyed by requestID
}

// Init creates a new RTK plugin instance with the given configuration.
// Init validates the config before constructing the plugin to fail fast on
// misconfiguration (malicious input, out-of-range values, etc.).
func Init(ctx context.Context, config *Config, logger schemas.Logger) (*Plugin, error) {
	if config == nil {
		return nil, fmt.Errorf("rtk: config is nil")
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("rtk: invalid config: %w", err)
	}
	// Apply defaults for zero-value fields so the compression pipeline has
	// sane limits even when the config omits them.
	applyConfigDefaults(config)
	return &Plugin{
		name:       PluginName,
		config:     config,
		logger:     logger,
		stateStore: sync.Map{},
	}, nil
}

// GetName returns the plugin name.
func (p *Plugin) GetName() string {
	return PluginName
}

// Cleanup performs plugin cleanup — drains the state store.
func (p *Plugin) Cleanup() error {
	p.stateStore.Range(func(k, _ interface{}) bool {
		p.stateStore.Delete(k)
		return true
	})
	return nil
}

// PreRequestHook implements schemas.LLMPlugin (no-op — required for plugin indexing).
func (p *Plugin) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

// HTTPTransportPreHook implements schemas.HTTPTransportPlugin (no-op).
func (p *Plugin) HTTPTransportPreHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	return nil, nil
}

// HTTPTransportPostHook implements schemas.HTTPTransportPlugin (no-op).
func (p *Plugin) HTTPTransportPostHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error {
	return nil
}

// HTTPTransportStreamChunkHook implements schemas.HTTPTransportPlugin (pass-through).
func (p *Plugin) HTTPTransportStreamChunkHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest, chunk *schemas.BifrostStreamChunk) (*schemas.BifrostStreamChunk, error) {
	return chunk, nil
}