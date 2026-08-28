// Package rtk provides a Rule-based Tool-output Kompression plugin for Bifrost.
// It implements custom filter loading (project > global > builtin), dual-format
// JSON parsing, and trust-based SHA256 verification.
package rtk

import (
	"context"
	"fmt"
	"sync"

	"github.com/pin-gou/celer-route/core/schemas"
)

// PluginName is the canonical name for the RTK compression plugin.
const PluginName = "rtk"

// Plugin implements schemas.LLMPlugin for rule-based tool output compression.
type Plugin struct {
	name       string
	config     *Config
	logger     schemas.Logger
	stateStore sync.Map // map[string]*CompressionState, keyed by requestID
	loader     *FilterLoader
	appDir     string
	metrics    *CompressionMetrics
}

// Init creates a new RTK plugin instance with the given configuration.
// Init validates the config before constructing the plugin to fail fast on
// misconfiguration (malicious input, out-of-range values, etc.). The appDir
// parameter is used to locate project/global custom filter files.
// If loading custom filters fails, Init logs a warning but does not fail
// (fail-open strategy — builtin filters still work).
func Init(ctx context.Context, config *Config, logger schemas.Logger, appDir string) (*Plugin, error) {
	if config == nil {
		return nil, fmt.Errorf("rtk: config is nil")
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("rtk: invalid config: %w", err)
	}
	// Apply defaults for zero-value fields so the compression pipeline has
	// sane limits even when the config omits them.
	applyConfigDefaults(config)
	p := &Plugin{
		name:       PluginName,
		config:     config,
		logger:     logger,
		stateStore: sync.Map{},
		loader:     NewFilterLoader(config),
		appDir:     appDir,
		metrics:    &CompressionMetrics{},
	}
	// Load custom filters — fail-open: warn on error, continue with builtins.
	if err := p.loader.Load(appDir); err != nil {
		if logger != nil {
			logger.Warn("rtk", "filter loader warning: %v", err)
		}
	}

	// Observability: log Init summary and diagnostics as required by design.md.
	if logger != nil {
		loader := p.loader
		builtinCount := len(loader.builtins)
		projectCount := len(loader.projects)
		globalCount := len(loader.globals)
		totalCount := len(loader.cachedFilters)
		diagCount := len(loader.diagnostics)
		logger.Info("rtk: filter loader initialized, total=%d (project=%d global=%d builtin=%d), diagnostics=%d",
			totalCount, projectCount, globalCount, builtinCount, diagCount)

		for _, d := range loader.diagnostics {
			switch d.Level {
			case "error":
				logger.Error("rtk", "filter diagnostic: [%s/%s] %s — %s", d.Source, d.Format, d.Path, d.Message)
			case "warning":
				logger.Warn("rtk", "filter diagnostic: [%s/%s] %s — %s", d.Source, d.Format, d.Path, d.Message)
			default:
				logger.Info("rtk", "filter diagnostic: [%s/%s] %s — %s", d.Source, d.Format, d.Path, d.Message)
			}
		}
	}
	// Register the RTK engine in the global EngineCatalog so it can be
	// used as a compression step in the pipeline. The engine wraps the
	// plugin's config and filter loader, allowing the existing compression
	// pipeline (processRtkText) to be called through the CompressionEngine
	// interface.
	RegisterEngine(&rtkEngine{plugin: p})

	return p, nil
}

// GetName returns the plugin name.
func (p *Plugin) GetName() string {
	return PluginName
}

// GetAppDir returns the application directory passed to Init. It is used by
// handlers to resolve relative paths (e.g. raw-output files) consistently
// with the loader's project/global filter discovery.
func (p *Plugin) GetAppDir() string {
	if p == nil {
		return ""
	}
	return p.appDir
}

// Metrics returns the cross-request compression metrics so the admin HTTP
// handler can surface a Monitoring panel. Returns nil for an uninitialised
// plugin (e.g. during tests that bypass Init) — callers must guard.
func (p *Plugin) Metrics() *CompressionMetrics {
	if p == nil {
		return nil
	}
	return p.metrics
}

// Stats is the public Snapshot view of the compression metrics. Implemented
// as a method on *Plugin (rather than only on *CompressionMetrics) so the
// handlers package can satisfy RtkPluginAccessor.Stats() without having to
// reach through a separate accessor path.
func (p *Plugin) Stats() MetricsSnapshot {
	if p == nil || p.metrics == nil {
		return MetricsSnapshot{}
	}
	return p.metrics.Snapshot()
}

// Histogram returns time-bucketed compression metrics within [start, end)
// aligned to the requested bucketSizeSeconds. Satisfies the RtkPluginAccessor
// interface so the handler can serve GET /api/context/rtk/stats/histogram.
func (p *Plugin) Histogram(start, end, bucketSize int64) []RtkHistogramBucket {
	if p == nil || p.metrics == nil {
		return nil
	}
	return p.metrics.Histogram(start, end, bucketSize)
}

// Loader returns the plugin's FilterLoader so handlers can inspect the
// loaded filter catalog and diagnostics.
func (p *Plugin) Loader() *FilterLoader {
	if p == nil {
		return nil
	}
	return p.loader
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

// PreProviderHook implements schemas.LLMPlugin (no-op passthrough). RTK
// decides what to compress in PreLLMHook once a provider is pinned, so
// there is nothing to gate here.
func (p *Plugin) PreProviderHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
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