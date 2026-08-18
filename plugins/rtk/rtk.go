// Package rtk provides a Rule-based Tool-output Kompression plugin for Bifrost.
// This is a minimal stub that will be fully implemented in the dev.plugins track.
package rtk

import (
	"context"
	"fmt"

	"github.com/pin-gou/pg-gateway/core/schemas"
)

// PluginName is the canonical name for the RTK compression plugin.
const PluginName = "rtk"

// Plugin implements schemas.LLMPlugin for rule-based tool output compression.
// This is a minimal stub; the full implementation is in the dev.plugins track.
type Plugin struct {
	name   string
	config *Config
	logger schemas.Logger
}

// Init creates a new RTK plugin instance with the given configuration.
// This is a minimal stub; the full implementation is in the dev.plugins track.
// Init validates the config before constructing the plugin to fail fast on
// misconfiguration (malicious input, out-of-range values, etc.).
func Init(ctx context.Context, config *Config, logger schemas.Logger) (*Plugin, error) {
	if config == nil {
		return nil, fmt.Errorf("rtk: config is nil")
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("rtk: invalid config: %w", err)
	}
	return &Plugin{
		name:   PluginName,
		config: config,
		logger: logger,
	}, nil
}

// GetName returns the plugin name.
func (p *Plugin) GetName() string {
	return PluginName
}

// Cleanup performs plugin cleanup.
func (p *Plugin) Cleanup() error {
	return nil
}

// PreRequestHook implements schemas.LLMPlugin (no-op — required for plugin indexing).
func (p *Plugin) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

// PreLLMHook implements schemas.LLMPlugin.
// This is a minimal stub; the full implementation is in the dev.plugins track.
func (p *Plugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}

// PostLLMHook implements schemas.LLMPlugin.
// This is a minimal stub; the full implementation is in the dev.plugins track.
func (p *Plugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
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