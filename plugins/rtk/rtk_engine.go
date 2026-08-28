package rtk

import (
	"encoding/json"

	"github.com/pin-gou/celer-route/core/schemas"
)

// rtkEngine implements CompressionEngine by wrapping the existing RTK
// compression pipeline (processRtkText). It is registered as "rtk"
// in the EngineCatalog during Init and can be used as a compression step
// in a pipeline.
type rtkEngine struct {
	plugin *Plugin
}

// Id returns the engine identifier ("rtk").
func (e *rtkEngine) Id() string {
	return "rtk"
}

// Apply applies the RTK compression pipeline to the input text.
// It delegates to processRtkText for the actual compression logic.
// When the plugin is nil or disabled, the input is returned unchanged.
func (e *rtkEngine) Apply(ctx *schemas.BifrostContext, text string, cfg EngineConfig) (EngineResult, error) {
	if e.plugin == nil || e.plugin.config == nil {
		return EngineResult{
			Text:         text,
			InputBytes:   len(text),
			OutputBytes:  len(text),
			CompressedBy: 0,
			Skipped:      true,
			Reason:       "plugin disabled",
		}, nil
	}

	// Respect cfg.Enabled (preview passes true to force-enable) while
	// falling back to the plugin config for production pipeline runs
	// where cfg is the zero value (Enabled: false).
	if !e.plugin.config.Enabled && !cfg.Enabled {
		return EngineResult{
			Text:         text,
			InputBytes:   len(text),
			OutputBytes:  len(text),
			CompressedBy: 0,
			Skipped:      true,
			Reason:       "disabled by config",
		}, nil
	}

	config := e.plugin.config
	loader := e.plugin.loader

	commandHint := cfg.CommandHint
	result, stats := processRtkTextWithCommand(text, config, loader, commandHint)
	if stats == nil {
		return EngineResult{
			Text:         result,
			InputBytes:   len(text),
			OutputBytes:  len(result),
			CompressedBy: calcCompressedBy(len(text), len(result)),
		}, nil
	}

	inputBytes := len(text)
	outputBytes := len(result)
	compressedBy := calcCompressedBy(inputBytes, outputBytes)

	return EngineResult{
		Text:              result,
		InputBytes:        inputBytes,
		OutputBytes:       outputBytes,
		CompressedBy:      compressedBy,
		Techniques:        stats.Techniques,
		FilterMatched:     stats.FilterMatched,
		rawOutputPointers: stats.RawOutputPointers,
	}, nil
}

// HealthCheck returns nil (always healthy for the rtk engine).
func (e *rtkEngine) HealthCheck() error {
	return nil
}

// IsEnabled returns true when the plugin's config has Enabled set to true.
func (e *rtkEngine) IsEnabled() bool {
	return e.plugin != nil && e.plugin.config != nil && e.plugin.config.Enabled
}

// Schema returns a JSON schema describing the engine's config options.
func (e *rtkEngine) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

// calcCompressedBy calculates the compression ratio (0.0 = no compression, 1.0 = fully compressed).
func calcCompressedBy(inputBytes, outputBytes int) float64 {
	if inputBytes <= 0 {
		return 0
	}
	return 1.0 - float64(outputBytes)/float64(inputBytes)
}
