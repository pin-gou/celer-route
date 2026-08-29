// Package rtk provides a Rule-based Tool-output Kompression plugin for Bifrost.
// It implements custom filter loading (project > global > builtin), dual-format
// JSON parsing, and trust-based SHA256 verification.
package rtk

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	bifrost "github.com/pin-gou/celer-route/core"
	"github.com/pin-gou/celer-route/core/schemas"
)

// PluginName is the canonical name for the RTK compression plugin.
const PluginName = "rtk"

// Plugin implements schemas.LLMPlugin for rule-based tool output compression.
type Plugin struct {
	name         string
	config       *Config
	logger       schemas.Logger
	stateStore   sync.Map // map[string]*CompressionState, keyed by requestID
	loader       *FilterLoader
	appDir       string
	metrics      *CompressionMetrics
	rawOutputDir string               // resolved on-disk root for raw-output persistence
	janitor      *RtkRawOutputJanitor // TTL reaper for persisted raw outputs (nil when TTL=0)
	bifrost      *bifrost.Bifrost     // core instance, attached post-Init for MCP tool registration (nil when unattached)
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
	// Resolve the on-disk root for raw-output persistence. Dir takes
	// priority; AppDir is the historical fallback so existing callers
	// stay source-compatible.
	rawDir := config.RawOutputDir
	if rawDir == "" {
		rawDir = filepath.Join(appDir, "rtk", "raw-output")
	}
	p := &Plugin{
		name:         PluginName,
		config:       config,
		logger:       logger,
		stateStore:   sync.Map{},
		loader:       NewFilterLoader(config),
		appDir:       appDir,
		metrics:      &CompressionMetrics{},
		rawOutputDir: rawDir,
	}
	// Load custom filters — fail-open: warn on error, continue with builtins.
	if err := p.loader.Load(appDir); err != nil {
		if logger != nil {
			logger.Warn("rtk", "filter loader warning: %v", err)
		}
	}

	// Start the raw-output janitor if a non-zero TTL is configured. The
	// janitor is a background goroutine that reaps files older than
	// TTL on a 30-minute tick. It is intentionally separate from the
	// compression hot path so a misbehaving filesystem cannot stall
	// the pipeline.
	//
	// A nil ctx (some legacy test callers pass nil to Init) is treated as
	// "no janitor wanted" — the goroutine has no cancellation source
	// other than Stop(), and tests rarely call Cleanup(). Production
	// paths always pass a real context.
	if ctx != nil {
		if ttl := time.Duration(config.RawOutputTTLHours) * time.Hour; ttl > 0 {
			p.janitor = NewRtkRawOutputJanitor(rawDir, ttl, logger)
			p.janitor.Start(ctx)
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

// RawOutputDir returns the on-disk root used for raw-output persistence.
// It is the explicit config.RawOutputDir when set, otherwise the
// derived <appDir>/rtk/raw-output. The handler uses this when serving
// GET /api/context/rtk/raw-output/{id} so the read path matches where
// the janitor is reaping.
func (p *Plugin) RawOutputDir() string {
	if p == nil {
		return ""
	}
	return p.rawOutputDir
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

// AttachBifrost wires the core bifrost instance into the plugin and registers
// the rtk_fetch_raw_output MCP tool when the plugin is enabled, the config
// opts in (InjectFetchTool default true), and the core exposes an MCP manager.
//
// It is called by the transports host AFTER bifrost.Init (the core instance
// does not exist during loadBuiltinPlugins), matching the provider-cooldown
// post-init wiring pattern (server.server.go). It is safe to call multiple
// times: RegisterTool on an already-registered name returns an error which is
// logged at WARN and swallowed (the design's error table for RegisterTool).
func (p *Plugin) AttachBifrost(b *bifrost.Bifrost) {
	if p == nil {
		return
	}
	p.bifrost = b
	p.registerFetchToolIfEnabled()
}

// registerFetchToolIfEnabled registers the raw-output fetch tool with the
// MCP manager when the plugin is enabled and InjectFetchTool is true. No-op
// when the plugin is disabled, when the opt-in flag is false, when no bifrost
// instance is attached, or when the core has no MCP manager (e.g. unit tests
// that bypass MCP entirely). A RegisterTool failure (name already registered)
// is logged at WARN and swallowed so a tool conflict cannot take down the
// gateway — the recovery hint text still works via plain GET as a fallback.
func (p *Plugin) registerFetchToolIfEnabled() {
	if p == nil || p.config == nil {
		return
	}
	if !p.config.Enabled || !IsTruePtr(p.config.InjectFetchTool) {
		return
	}
	if p.bifrost == nil {
		return
	}
	mgr := p.bifrost.GetMCPManager()
	if mgr == nil {
		return
	}
	if err := mgr.RegisterTool(
		"rtk_fetch_raw_output", // internal name (no hyphen); RegisterTool prefixes it
		rtkFetchRawOutputToolDescription,
		p.rawOutputReadHandlerMCPTool, // MCPToolFunction[any] adapter
		RtkFetchRawOutputTool,         // byte-stable schema (prefixed name)
	); err != nil {
		if p.logger != nil {
			p.logger.Warn("rtk: failed to register rtk_fetch_raw_output MCP tool: %v", err)
		}
		return
	}
	if p.logger != nil {
		p.logger.Info("rtk: registered rtk_fetch_raw_output MCP tool")
	}
}

// rawOutputReadHandlerMCPTool adapts RawOutputReadHandler to the
// MCPToolFunction[any] shape RegisterTool expects. The MCP layer passes args
// as map[string]any (deserialised from the JSON Schema declaration), and the
// handler is a pure (ctx, args) → (string, error) function per design.md.
func (p *Plugin) rawOutputReadHandlerMCPTool(args any) (string, error) {
	return p.RawOutputReadHandler(context.Background(), args)
}

// Cleanup performs plugin cleanup — drains the state store and stops
// the raw-output janitor. The bifrost client may call this on graceful
// shutdown; the reload path skips Cleanup because ReloadPlugin constructs
// a fresh Plugin (the new instance gets its own janitor).
func (p *Plugin) Cleanup() error {
	if p.janitor != nil {
		p.janitor.Stop()
		p.janitor = nil
	}
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