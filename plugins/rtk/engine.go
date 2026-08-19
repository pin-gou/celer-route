package rtk

import "fmt"

// CompressionEngine defines the interface for compression engines in the pipeline.
// Each engine is registered in the EngineCatalog and executed in order by the
// PipelineRunner. The Compress method takes input text and optional configuration
// and returns the compressed result with processing statistics.
type CompressionEngine interface {
	Compress(text string, opts map[string]any) (string, *ProcessStats, error)
}

// EngineConfig holds configuration for a single engine in the pipeline.
type EngineConfig struct {
	Enabled  bool              `json:"enabled"`
	Settings map[string]any    `json:"settings,omitempty"`
}

// EngineResult holds the result of a single engine's compression pass.
type EngineResult struct {
	Text         string  `json:"text"`
	InputBytes   int     `json:"input_bytes"`
	OutputBytes  int     `json:"output_bytes"`
	CompressedBy float64 `json:"compressed_by"`
	Skipped      bool    `json:"skipped,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

// EngineBreakdown holds per-engine stats for the pipeline result.
type EngineBreakdown struct {
	EngineID     string  `json:"engine_id"`
	InputBytes   int     `json:"input_bytes"`
	OutputBytes  int     `json:"output_bytes"`
	CompressedBy float64 `json:"compressed_by"`
}

// PipelineResult holds the final result of a pipeline run.
type PipelineResult struct {
	FinalText       string            `json:"text"`
	EngineBreakdown []EngineBreakdown `json:"engine_breakdown"`
}

// Pipeline defines a sequence of engine IDs to run in order.
type Pipeline struct {
	Engines []string
}

// PipelineStep defines a single step in the config's pipeline specification.
type PipelineStep struct {
	ID     string         `json:"id"`
	Config map[string]any `json:"config,omitempty"`
}

// EngineCatalog is a registry of named compression engines.
// Engines are registered by ID and can be retrieved, listed, or executed
// by the PipelineRunner.
type EngineCatalog struct {
	engines map[string]CompressionEngine
}

// NewEngineCatalog creates a new empty EngineCatalog.
func NewEngineCatalog() *EngineCatalog {
	return &EngineCatalog{
		engines: make(map[string]CompressionEngine),
	}
}

// RegisterEngine registers a compression engine under the given ID.
// If an engine with the same ID already exists, the last-write-wins
// (the previous engine is replaced).
func (c *EngineCatalog) RegisterEngine(id string, engine CompressionEngine) {
	c.engines[id] = engine
}

// GetEngine retrieves a compression engine by ID. Returns ok=false
// when the ID is not registered.
func (c *EngineCatalog) GetEngine(id string) (CompressionEngine, bool) {
	e, ok := c.engines[id]
	return e, ok
}

// ListEngines returns all registered engine IDs. The order is not
// guaranteed to be stable.
func (c *EngineCatalog) ListEngines() []string {
	ids := make([]string, 0, len(c.engines))
	for id := range c.engines {
		ids = append(ids, id)
	}
	return ids
}

// globalCatalog is the default global engine registry. Engines registered
// here are available to all pipeline runners by default.
var globalCatalog = NewEngineCatalog()

// RegisterEngine registers a compression engine in the global catalog
// under the given ID. This is the top-level registration function called
// by engines during Init. Engines registered here are available to all
// pipeline runners via the global catalog.
func RegisterEngine(id string, engine CompressionEngine) {
	globalCatalog.RegisterEngine(id, engine)
}

// PipelineRunner executes a sequence of compression engines in order.
// Each engine's output feeds into the next (pipeline chaining). Unknown
// engine IDs are logged as warnings and skipped (fail-soft).
type PipelineRunner struct {
	catalog *EngineCatalog
}

// NewPipelineRunner creates a PipelineRunner that uses the given catalog
// to resolve engine IDs.
func NewPipelineRunner(catalog *EngineCatalog) *PipelineRunner {
	return &PipelineRunner{catalog: catalog}
}

// Run executes the pipeline on the input text. Each engine in the pipeline
// is looked up in the catalog and executed in order. The output of each
// engine becomes the input for the next. Unknown engines are skipped with
// a warning. Returns the final text and a breakdown of per-engine stats.
func (r *PipelineRunner) Run(pipeline *Pipeline, input string) (string, []EngineBreakdown) {
	if pipeline == nil || len(pipeline.Engines) == 0 {
		return input, nil
	}

	text := input
	var breakdown []EngineBreakdown

	for _, engineID := range pipeline.Engines {
		engine, ok := r.catalog.GetEngine(engineID)
		if !ok {
			fmt.Printf("WARN: rtk: unknown engine id %q, skipping\n", engineID)
			continue
		}

		result, stats, err := engine.Compress(text, nil)
		if err != nil {
			fmt.Printf("WARN: rtk: engine %q error: %v\n", engineID, err)
			// Still record a breakdown entry so the error engine is visible
			// in the breakdown, allowing the pipeline to continue.
			breakdown = append(breakdown, EngineBreakdown{EngineID: engineID})
			continue
		}

		if result != "" {
			text = result
		}

		entry := EngineBreakdown{EngineID: engineID}
		if stats != nil {
			entry.InputBytes = stats.OriginalTokens
			entry.OutputBytes = stats.CompressedTokens
		}
		breakdown = append(breakdown, entry)
	}

	return text, breakdown
}